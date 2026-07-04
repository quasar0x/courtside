package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"
)

type Member struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	Region string `json:"region"`
}

type store struct {
	mu      sync.RWMutex
	members map[string]Member
	seq     int
}

func newStore() *store {
	s := &store{members: make(map[string]Member)}
	s.create(Member{Name: "Ada Lovelace", Email: "ada@example.com", Region: "eu-west"})
	s.create(Member{Name: "Grace Hopper", Email: "grace@example.com", Region: "us-east"})
	return s
}

func (s *store) create(m Member) Member {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	m.ID = strconv.Itoa(s.seq)
	s.members[m.ID] = m
	return m
}

func (s *store) list() []Member {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Member, 0, len(s.members))
	for _, m := range s.members {
		out = append(out, m)
	}
	return out
}

func (s *store) get(id string) (Member, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.members[id]
	return m, ok
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	st := newStore()
	port := getenv("PORT", "8080")

	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("GET /members", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, st.list())
	})

	mux.HandleFunc("GET /members/{id}", func(w http.ResponseWriter, r *http.Request) {
		m, ok := st.get(r.PathValue("id"))
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
		created := st.create(m)
		slog.Info("member created", "id", created.ID, "region", created.Region)
		writeJSON(w, http.StatusCreated, created)
	})

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           logRequests(mux),
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
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
