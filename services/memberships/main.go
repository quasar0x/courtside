package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	kafka "github.com/segmentio/kafka-go"
)

type Membership struct {
	ID       string `json:"id"`
	MemberID string `json:"member_id"`
	ClubID   string `json:"club_id"`
	Plan     string `json:"plan"`
	Status   string `json:"status"`
}

// MembershipCreated is the event we publish to Kafka. event_id is what
// consumers use for idempotency (dedup) via Redis.
type MembershipCreated struct {
	EventID      string    `json:"event_id"`
	Type         string    `json:"type"`
	MembershipID string    `json:"membership_id"`
	MemberID     string    `json:"member_id"`
	ClubID       string    `json:"club_id"`
	Plan         string    `json:"plan"`
	OccurredAt   time.Time `json:"occurred_at"`
}

type store struct{ pool *pgxpool.Pool }

func newStore(ctx context.Context, pool *pgxpool.Pool) (*store, error) {
	s := &store{pool: pool}
	if _, err := pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS memberships (
    id         SERIAL PRIMARY KEY,
    member_id  TEXT NOT NULL,
    club_id    TEXT NOT NULL,
    plan       TEXT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`); err != nil {
		return nil, fmt.Errorf("create schema: %w", err)
	}
	return s, nil
}

func (s *store) create(ctx context.Context, m Membership) (Membership, error) {
	if m.Status == "" {
		m.Status = "active"
	}
	var id int
	err := s.pool.QueryRow(ctx,
		`INSERT INTO memberships (member_id,club_id,plan,status) VALUES ($1,$2,$3,$4) RETURNING id`,
		m.MemberID, m.ClubID, m.Plan, m.Status).Scan(&id)
	if err != nil {
		return Membership{}, err
	}
	m.ID = strconv.Itoa(id)
	return m, nil
}

func (s *store) list(ctx context.Context) ([]Membership, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,member_id,club_id,plan,status FROM memberships ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Membership{}
	for rows.Next() {
		var id int
		var m Membership
		if err := rows.Scan(&id, &m.MemberID, &m.ClubID, &m.Plan, &m.Status); err != nil {
			return nil, err
		}
		m.ID = strconv.Itoa(id)
		out = append(out, m)
	}
	return out, rows.Err()
}

type publisher struct{ w *kafka.Writer }

func newPublisher(broker string) *publisher {
	return &publisher{w: &kafka.Writer{
		Addr:     kafka.TCP(broker),
		Topic:    "membership.created",
		Balancer: &kafka.LeastBytes{},
	}}
}

func (p *publisher) publish(ctx context.Context, ev MembershipCreated) error {
	payload, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	return p.w.WriteMessages(ctx, kafka.Message{
		Key:   []byte(ev.MembershipID),
		Value: payload,
	})
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, buildDSN())
	if err != nil {
		slog.Error("cannot create db pool", "err", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := waitForDB(ctx, pool); err != nil {
		slog.Error("database never became ready", "err", err)
		os.Exit(1)
	}

	st, err := newStore(ctx, pool)
	if err != nil {
		slog.Error("store init failed", "err", err)
		os.Exit(1)
	}

	pub := newPublisher(getenv("KAFKA_BROKER", "kafka.courtside:9092"))
	defer pub.w.Close()

	port := getenv("PORT", "8080")
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("GET /memberships", func(w http.ResponseWriter, r *http.Request) {
		ms, err := st.list(r.Context())
		if err != nil {
			slog.Error("list failed", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		writeJSON(w, http.StatusOK, ms)
	})

	mux.HandleFunc("POST /memberships", func(w http.ResponseWriter, r *http.Request) {
		var m Membership
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		created, err := st.create(r.Context(), m)
		if err != nil {
			slog.Error("create failed", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}

		ev := MembershipCreated{
			EventID:      newID(),
			Type:         "membership.created",
			MembershipID: created.ID,
			MemberID:     created.MemberID,
			ClubID:       created.ClubID,
			Plan:         created.Plan,
			OccurredAt:   time.Now().UTC(),
		}
		// Inline publish (dual-write). Production: transactional outbox.
		if err := pub.publish(r.Context(), ev); err != nil {
			slog.Error("event publish failed", "membership_id", created.ID, "err", err)
		} else {
			slog.Info("published membership.created", "event_id", ev.EventID, "membership_id", created.ID)
		}

		writeJSON(w, http.StatusCreated, created)
	})

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           logRequests(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		slog.Info("memberships service listening", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	<-stop
	slog.Info("shutting down")
	shCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shCtx)
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func buildDSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		getenv("PGUSER", "courtside"),
		os.Getenv("PGPASSWORD"),
		getenv("PGHOST", "postgres.courtside"),
		getenv("PGPORT", "5432"),
		getenv("PGDATABASE", "memberships"),
		getenv("PGSSLMODE", "require"),
	)
}

func waitForDB(ctx context.Context, pool *pgxpool.Pool) error {
	for i := 0; i < 30; i++ {
		if err := pool.Ping(ctx); err == nil {
			return nil
		}
		slog.Info("waiting for database...", "attempt", i+1)
		time.Sleep(2 * time.Second)
	}
	return errors.New("timed out waiting for database")
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.Info("request", "method", r.Method, "path", r.URL.Path,
			"dur_ms", time.Since(start).Milliseconds())
	})
}
