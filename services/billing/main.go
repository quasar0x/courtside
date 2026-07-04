package main

import (
	"context"
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
	"github.com/redis/go-redis/v9"
	kafka "github.com/segmentio/kafka-go"
)

type MembershipCreated struct {
	EventID      string    `json:"event_id"`
	Type         string    `json:"type"`
	MembershipID string    `json:"membership_id"`
	MemberID     string    `json:"member_id"`
	ClubID       string    `json:"club_id"`
	Plan         string    `json:"plan"`
	OccurredAt   time.Time `json:"occurred_at"`
}

type Invoice struct {
	ID           string  `json:"id"`
	MembershipID string  `json:"membership_id"`
	MemberID     string  `json:"member_id"`
	Plan         string  `json:"plan"`
	Amount       float64 `json:"amount"`
	Status       string  `json:"status"`
}

var planPrices = map[string]float64{"premium": 100, "standard": 50, "basic": 20}

type store struct{ pool *pgxpool.Pool }

func newStore(ctx context.Context, pool *pgxpool.Pool) (*store, error) {
	s := &store{pool: pool}
	if _, err := pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS invoices (
    id            SERIAL PRIMARY KEY,
    membership_id TEXT NOT NULL,
    member_id     TEXT NOT NULL,
    plan          TEXT NOT NULL,
    amount        DOUBLE PRECISION NOT NULL,
    status        TEXT NOT NULL DEFAULT 'issued',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
)`); err != nil {
		return nil, fmt.Errorf("create schema: %w", err)
	}
	return s, nil
}

func (s *store) create(ctx context.Context, inv Invoice) (Invoice, error) {
	var id int
	err := s.pool.QueryRow(ctx,
		`INSERT INTO invoices (membership_id,member_id,plan,amount,status) VALUES ($1,$2,$3,$4,$5) RETURNING id`,
		inv.MembershipID, inv.MemberID, inv.Plan, inv.Amount, inv.Status).Scan(&id)
	if err != nil {
		return Invoice{}, err
	}
	inv.ID = strconv.Itoa(id)
	return inv, nil
}

func (s *store) list(ctx context.Context) ([]Invoice, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,membership_id,member_id,plan,amount,status FROM invoices ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Invoice{}
	for rows.Next() {
		var id int
		var inv Invoice
		if err := rows.Scan(&id, &inv.MembershipID, &inv.MemberID, &inv.Plan, &inv.Amount, &inv.Status); err != nil {
			return nil, err
		}
		inv.ID = strconv.Itoa(id)
		out = append(out, inv)
	}
	return out, rows.Err()
}

func consume(ctx context.Context, reader *kafka.Reader, rdb *redis.Client, st *store) {
	for {
		msg, err := reader.ReadMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			slog.Error("kafka read error", "err", err)
			time.Sleep(time.Second)
			continue
		}
		var ev MembershipCreated
		if err := json.Unmarshal(msg.Value, &ev); err != nil {
			slog.Error("bad event payload", "err", err)
			continue
		}

		key := "billing:processed:" + ev.EventID
		firstTime, err := rdb.SetNX(ctx, key, "1", 24*time.Hour).Result()
		if err != nil {
			slog.Error("redis error", "err", err)
			continue
		}
		if !firstTime {
			slog.Info("duplicate event skipped", "event_id", ev.EventID)
			continue
		}

		inv, err := st.create(ctx, Invoice{
			MembershipID: ev.MembershipID,
			MemberID:     ev.MemberID,
			Plan:         ev.Plan,
			Amount:       planPrices[ev.Plan],
			Status:       "issued",
		})
		if err != nil {
			slog.Error("invoice create failed", "err", err)
			rdb.Del(ctx, key)
			continue
		}
		slog.Info("invoice created", "invoice_id", inv.ID, "event_id", ev.EventID,
			"amount", inv.Amount, "plan", ev.Plan)
	}
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	rdb := redis.NewClient(&redis.Options{Addr: getenv("REDIS_ADDR", "redis.courtside:6379")})
	defer rdb.Close()

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     []string{getenv("KAFKA_BROKER", "kafka.courtside:9092")},
		Topic:       "membership.created",
		GroupID:     "billing",
		StartOffset: kafka.FirstOffset,
	})
	defer reader.Close()

	go consume(ctx, reader, rdb, st)

	port := getenv("PORT", "8080")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /invoices", func(w http.ResponseWriter, r *http.Request) {
		invs, err := st.list(r.Context())
		if err != nil {
			slog.Error("list failed", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		writeJSON(w, http.StatusOK, invs)
	})

	srv := &http.Server{Addr: ":" + port, Handler: logRequests(mux), ReadHeaderTimeout: 5 * time.Second}

	go func() {
		slog.Info("billing service listening", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	<-stop
	slog.Info("shutting down")
	cancel()
	shCtx, shCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shCancel()
	_ = srv.Shutdown(shCtx)
}

func buildDSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		getenv("PGUSER", "courtside"),
		os.Getenv("PGPASSWORD"),
		getenv("PGHOST", "postgres.courtside"),
		getenv("PGPORT", "5432"),
		getenv("PGDATABASE", "billing"),
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
		slog.Info("request", "method", r.Method, "path", r.URL.Path, "dur_ms", time.Since(start).Milliseconds())
	})
}
