package main

import (
	"context"
	"encoding/json"
	"fmt"
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

type Club struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	City      string   `json:"city"`
	Region    string   `json:"region"`
	MemberIDs []string `json:"member_ids"`
}

// EnrichedClub is a Club with its members resolved from the members service.
type EnrichedClub struct {
	Club
	Members []Member `json:"members"`
}

type store struct {
	mu    sync.RWMutex
	clubs map[string]Club
	seq   int
}

func newStore() *store {
	s := &store{clubs: make(map[string]Club)}
	s.create(Club{Name: "Riverside FC", City: "London", Region: "eu-west", MemberIDs: []string{"1", "2"}})
	s.create(Club{Name: "Bay Area Hoops", City: "San Francisco", Region: "us-east", MemberIDs: []string{"2"}})
	return s
}

func (s *store) create(c Club) Club {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	c.ID = strconv.Itoa(s.seq)
	s.clubs[c.ID] = c
	return c
}

func (s *store) list() []Club {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Club, 0, len(s.clubs))
	for _, c := range s.clubs {
		out = append(out, c)
	}
	return out
}

func (s *store) get(id string) (Club, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.clubs[id]
	return c, ok
}

// membersClient talks to the members service over HTTP.
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

	st := newStore()
	port := getenv("PORT", "8080")

	// Where to find the members service. Defaults to in-cluster DNS.
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
		writeJSON(w, http.StatusOK, st.list())
	})

	// The cross-service call: resolve each member via the members service.
	mux.HandleFunc("GET /clubs/{id}", func(w http.ResponseWriter, r *http.Request) {
		c, ok := st.get(r.PathValue("id"))
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
		created := st.create(c)
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
