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
	"syscall"
	"time"

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

type Notification struct {
	MembershipID string    `json:"membership_id"`
	MemberID     string    `json:"member_id"`
	Channel      string    `json:"channel"`
	Message      string    `json:"message"`
	SentAt       time.Time `json:"sent_at"`
}

const recentKey = "notifications:recent"

func consume(ctx context.Context, reader *kafka.Reader, rdb *redis.Client) {
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

		key := "notifications:processed:" + ev.EventID
		firstTime, err := rdb.SetNX(ctx, key, "1", 24*time.Hour).Result()
		if err != nil {
			slog.Error("redis error", "err", err)
			continue
		}
		if !firstTime {
			slog.Info("duplicate event skipped", "event_id", ev.EventID)
			continue
		}

		n := Notification{
			MembershipID: ev.MembershipID,
			MemberID:     ev.MemberID,
			Channel:      "email",
			Message:      fmt.Sprintf("Welcome! Your %s membership (#%s) is now active.", ev.Plan, ev.MembershipID),
			SentAt:       time.Now().UTC(),
		}
		payload, _ := json.Marshal(n)
		if err := rdb.LPush(ctx, recentKey, payload).Err(); err != nil {
			slog.Error("redis lpush failed", "err", err)
			rdb.Del(ctx, key)
			continue
		}
		rdb.LTrim(ctx, recentKey, 0, 49)
		slog.Info("notification sent", "member_id", ev.MemberID, "membership_id", ev.MembershipID, "channel", n.Channel)
	}
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rdb := redis.NewClient(&redis.Options{Addr: getenv("REDIS_ADDR", "redis.courtside:6379")})
	defer rdb.Close()
	if err := waitForRedis(ctx, rdb); err != nil {
		slog.Error("redis never became ready", "err", err)
		os.Exit(1)
	}

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     []string{getenv("KAFKA_BROKER", "kafka.courtside:9092")},
		Topic:       "membership.created",
		GroupID:     "notifications",
		StartOffset: kafka.FirstOffset,
	})
	defer reader.Close()

	go consume(ctx, reader, rdb)

	port := getenv("PORT", "8080")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /notifications", func(w http.ResponseWriter, r *http.Request) {
		vals, err := rdb.LRange(r.Context(), recentKey, 0, 49).Result()
		if err != nil {
			slog.Error("lrange failed", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		out := []Notification{}
		for _, v := range vals {
			var n Notification
			if err := json.Unmarshal([]byte(v), &n); err == nil {
				out = append(out, n)
			}
		}
		writeJSON(w, http.StatusOK, out)
	})

	srv := &http.Server{Addr: ":" + port, Handler: logRequests(mux), ReadHeaderTimeout: 5 * time.Second}

	go func() {
		slog.Info("notifications service listening", "port", port)
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

func waitForRedis(ctx context.Context, rdb *redis.Client) error {
	for i := 0; i < 30; i++ {
		if err := rdb.Ping(ctx).Err(); err == nil {
			return nil
		}
		slog.Info("waiting for redis...", "attempt", i+1)
		time.Sleep(2 * time.Second)
	}
	return errors.New("timed out waiting for redis")
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
