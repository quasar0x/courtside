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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/quasar0x/courtside/pkg/telemetry"
)

type Member struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	Region string `json:"region"`
}

type store struct{ pool *pgxpool.Pool }

func newStore(ctx context.Context, pool *pgxpool.Pool) (*store, error) {
	s := &store{pool: pool}
	if _, err := pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS members (
    id     SERIAL PRIMARY KEY,
    name   TEXT NOT NULL,
    email  TEXT NOT NULL,
    region TEXT NOT NULL
)`); err != nil {
		return nil, fmt.Errorf("create schema: %w", err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM members`).Scan(&count); err != nil {
		return nil, err
	}
	if count == 0 {
		_, _ = s.create(ctx, Member{Name: "Ada Lovelace", Email: "ada@example.com", Region: "eu-west"})
		_, _ = s.create(ctx, Member{Name: "Grace Hopper", Email: "grace@example.com", Region: "us-east"})
		slog.Info("seeded initial members")
	}
	return s, nil
}

func (s *store) create(ctx context.Context, m Member) (Member, error) {
	var id int
	err := s.pool.QueryRow(ctx,
		`INSERT INTO members (name,email,region) VALUES ($1,$2,$3) RETURNING id`,
		m.Name, m.Email, m.Region).Scan(&id)
	if err != nil {
		return Member{}, err
	}
	m.ID = strconv.Itoa(id)
	return m, nil
}

func (s *store) list(ctx context.Context) ([]Member, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,name,email,region FROM members ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Member{}
	for rows.Next() {
		var id int
		var m Member
		if err := rows.Scan(&id, &m.Name, &m.Email, &m.Region); err != nil {
			return nil, err
		}
		m.ID = strconv.Itoa(id)
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *store) get(ctx context.Context, idStr string) (Member, bool, error) {
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return Member{}, false, nil
	}
	var m Member
	var iid int
	err = s.pool.QueryRow(ctx,
		`SELECT id,name,email,region FROM members WHERE id=$1`, id).
		Scan(&iid, &m.Name, &m.Email, &m.Region)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Member{}, false, nil
		}
		return Member{}, false, err
	}
	m.ID = strconv.Itoa(iid)
	return m, true, nil
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	ctx := context.Background()

	if shutdown, err := telemetry.InitTracer(ctx, "members", getenv("OTEL_ENDPOINT", "tempo.monitoring.svc.cluster.local:4318")); err != nil {
		slog.Error("tracer init failed", "err", err)
	} else {
		defer func() { _ = shutdown(context.Background()) }()
	}

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

	port := getenv("PORT", "8080")
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("GET /members", func(w http.ResponseWriter, r *http.Request) {
		ms, err := st.list(r.Context())
		if err != nil {
			slog.Error("list failed", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		writeJSON(w, http.StatusOK, ms)
	})

	mux.HandleFunc("GET /members/{id}", func(w http.ResponseWriter, r *http.Request) {
		m, ok, err := st.get(r.Context(), r.PathValue("id"))
		if err != nil {
			slog.Error("get failed", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "member not found"})
			return
		}
		writeJSON(w, http.StatusOK, m)
	})

	mux.HandleFunc("POST /members", func(w http.ResponseWriter, r *http.Request) {
		var m Member
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
		slog.Info("member created", "id", created.ID, "region", created.Region)
		writeJSON(w, http.StatusCreated, created)
	})

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           otelhttp.NewHandler(logRequests(mux), "members"),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		slog.Info("members service listening", "port", port)
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

func buildDSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		getenv("PGUSER", "courtside"),
		os.Getenv("PGPASSWORD"),
		getenv("PGHOST", "postgres.courtside"),
		getenv("PGPORT", "5432"),
		getenv("PGDATABASE", "members"),
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
