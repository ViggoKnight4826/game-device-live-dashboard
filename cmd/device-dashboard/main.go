package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"example.com/infrai-game-device-dashboard/internal/game"
	"example.com/infrai-game-device-dashboard/internal/infrai"
)

const channel = "game-device-status"

type server struct {
	client      *infrai.Client
	now         func() time.Time
	mu          sync.Mutex
	subscribers map[chan game.LiveEvent]struct{}
}

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	s := &server{client: infrai.New(key), now: time.Now, subscribers: make(map[chan game.LiveEvent]struct{})}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /devices/status", s.recordStatus)
	mux.HandleFunc("GET /dashboard/events", s.streamDashboard)
	addr := envOr("ADDR", ":8080")
	log.Printf("device dashboard listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func (s *server) recordStatus(w http.ResponseWriter, r *http.Request) {
	var status game.DeviceStatus
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&status); err != nil || status.PlayerID == "" || status.DeviceID == "" || status.State == "" {
		http.Error(w, "player_id, device_id, and state are required", http.StatusBadRequest)
		return
	}
	event := game.PrepareStatus(status, s.now())
	requestID := requestKey(event)
	if err := s.client.Publish(r.Context(), channel, event.Type, event.Status, status.PlayerID, requestID); err != nil {
		writeClientError(w, err)
		return
	}
	s.broadcast(event)
	writeJSON(w, http.StatusAccepted, event)
}

func (s *server) streamDashboard(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	events := s.subscribe()
	defer s.unsubscribe(events)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	if err := s.writeSnapshot(r.Context(), w, flusher); err != nil {
		return
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-events:
			writeSSE(w, flusher, "device.status", event)
		case <-ticker.C:
			if err := s.writeSnapshot(r.Context(), w, flusher); err != nil {
				return
			}
		}
	}
}

func (s *server) writeSnapshot(ctx context.Context, w http.ResponseWriter, flusher http.Flusher) error {
	snapshot, err := s.dashboardSnapshot(ctx)
	if err != nil {
		return err
	}
	writeSSE(w, flusher, "dashboard", snapshot)
	return nil
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, eventType string, value any) {
	encoded, _ := json.Marshal(value)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, encoded)
	flusher.Flush()
}

func (s *server) subscribe() chan game.LiveEvent {
	updates := make(chan game.LiveEvent, 8)
	s.mu.Lock()
	s.subscribers[updates] = struct{}{}
	s.mu.Unlock()
	return updates
}

func (s *server) unsubscribe(updates chan game.LiveEvent) {
	s.mu.Lock()
	delete(s.subscribers, updates)
	s.mu.Unlock()
}

func (s *server) broadcast(event game.LiveEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for updates := range s.subscribers {
		select {
		case updates <- event:
		default:
		}
	}
}

func (s *server) dashboardSnapshot(ctx context.Context) (map[string]any, error) {
	var presence any
	if err := s.client.Presence(ctx, channel, &presence); err != nil {
		return nil, err
	}
	var metrics any
	if err := s.client.Metrics(ctx, "realtime.publish", "count", &metrics); err != nil {
		return nil, err
	}
	return map[string]any{"channel": channel, "presence": presence, "metrics": metrics}, nil
}

func requestKey(event game.LiveEvent) string {
	raw, _ := json.Marshal(event)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func writeClientError(w http.ResponseWriter, err error) {
	var apiErr *infrai.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
		writeJSON(w, apiErr.StatusCode, map[string]string{"error": apiErr.Message})
		return
	}
	http.Error(w, "upstream request failed", http.StatusBadGateway)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func envOr(name string, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
