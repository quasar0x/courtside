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
)

type Member struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	Region string `json:"region"`
}

type Club struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	City      string   `json:"city"`
	Region    string   `json:"region"`
	MemberIDs []string `json:"member_ids"`
}

type EnrichedClub struct {
	Club
	Members []Member `json:"members"`
}

type store struct{ pool *pgxpool.Pool }

func newStore(ctx context.Context, pool *pgxpool.Pool) (*store, error) {
	s := &store{pool: pool}
	if _, err := pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS clubs (
    id         SERIAL PRIMARY KEY,
    name       TEXT NOT NULL,
    city       TEXT NOT NULL,
    region     TEXT NOT NULL,
    member_ids TEXT[] NOT NULL DEFAULT '{}'
)`); err != nil {
		return nil, fmt.Errorf("create schema: %w", err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM clubs`).Scan(&count); err != nil {
		return nil, err
	}
	if count == 0 {
		_, _ = s.create(ctx, Club{Name: "Riverside FC", City: "London", Region: "eu-west", MemberIDs: []string{"1", "2"}})
		_, _ = s.create(ctx, Club{Name: "Bay Area Hoops", City: "San Francisco", Region: "us-east", MemberIDs: []string{"2"}})
		slog.Info("seeded initial clubs")
	}
	return s, nil
}

func (s *store) create(ctx context.Context, c Club) (Club, error) {
	var id int
	err := s.pool.QueryRow(ctx,
		`INSERT INTO clubs (name,city,region,member_ids) VALUES ($1,$2,$3,$4) RETURNING id`,
		c.Name, c.City, c.Region, c.MemberIDs).Scan(&id)
	if err != nil {
		return Club{}, err
	}
	c.ID = strconv.Itoa(id)
	return c, nil
}

func (s *store) list(ctx context.Context) ([]Club, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,name,city,region,member_ids FROM clubs ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Club{}
	for rows.Next() {
		var id int
		var c Club
		if err := rows.Scan(&id, &c.Name, &c.City, &c.Region, &c.MemberIDs); err != nil {
			return nil, err
		}
		c.ID = strconv.Itoa(id)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *store) get(ctx context.Context, idStr string) (Club, bool, error) {
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return Club{}, false, nil
	}
	var c Club
	var iid int
	err = s.pool.QueryRow(ctx,
		`SELECT id,name,city,region,member_ids FROM clubs WHERE id=$1`, id).
		Scan(&iid, &c.Name, &c.City, &c.Region, &c.MemberIDs)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Club{}, false, nil
		}
		return Club{}, false, err
	}
	c.ID = strconv.Itoa(iid)
	return c, true, nil
}

type membersClient struct {
	baseURL string
	http    *http.Client
}

func (mc *membersClient) getMember(ctx context.Context, id string) (Member, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, mc.baseURL+"/members/"+id, nil)
	resp, err := mc.http.Do(req)
	if err != nil {
		return Member{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Member{}, fmt.Errorf("members service returned %d for id %s", resp.StatusCode, id)
	}
	var m Member
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return Member{}, err
	}
	return m, nil
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

	port := getenv("PORT", "8080")
	mc := &membersClient{
		baseURL: getenv("MEMBERS_URL", "http://members.courtside:8080"),
		http:    &http.Client{Timeout: 3 * time.Second},
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("GET /clubs", func(w http.ResponseWriter, r *http.Request) {
		cs, err := st.list(r.Context())
		if err != nil {
			slog.Error("list failed", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		writeJSON(w, http.StatusOK, cs)
	})

	mux.HandleFunc("GET /clubs/{id}", func(w http.ResponseWriter, r *http.Request) {
		c, ok, err := st.get(r.Context(), r.PathValue("id"))
		if err != nil {
			slog.Error("get failed", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "club not found"})
			return
		}
		enriched := EnrichedClub{Club: c}
		for _, mid := range c.MemberIDs {
			m, err := mc.getMember(r.Context(), mid)
			if err != nil {
				slog.Warn("could not fetch member", "member_id", mid, "err", err)
				continue
			}
			enriched.Members = append(enriched.Members, m)
		}
		writeJSON(w, http.StatusOK, enriched)
	})

	mux.HandleFunc("POST /clubs", func(w http.ResponseWriter, r *http.Request) {
		var c Club
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		created, err := st.create(r.Context(), c)
		if err != nil {
			slog.Error("create failed", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		slog.Info("club created", "id", created.ID, "region", created.Region)
		writeJSON(w, http.StatusCreated, created)
	})

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           logRequests(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		slog.Info("clubs service listening", "port", port, "members_url", mc.baseURL)
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
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		getenv("PGUSER", "courtside"),
		os.Getenv("PGPASSWORD"),
		getenv("PGHOST", "postgres.courtside"),
		getenv("PGPORT", "5432"),
		getenv("PGDATABASE", "clubs"),
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
