package infrai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPublishBoundary(t *testing.T) {
	tests := []struct {
		name       string
		responses  []string
		statuses   []int
		wantCode   string
		wantCalls  int
		wantHeader string
	}{
		{name: "business rejection keeps envelope error", responses: []string{`{"ok":false,"error":{"code":"INVALID_INPUT","message":"device_id is required"},"data":null,"metadata":{}}`}, statuses: []int{400}, wantCode: "INVALID_INPUT", wantCalls: 1, wantHeader: "req-7"},
		{name: "rate limit retries idempotently", responses: []string{`{"ok":false,"error":{"code":"RATE_LIMITED","message":"retry later"},"data":null,"metadata":{}}`, `{"ok":true,"data":{},"error":null,"metadata":{}}`}, statuses: []int{429, 200}, wantCalls: 2, wantHeader: "req-7"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/v1/realtime/publish" {
					t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				if got := r.Header.Get("Idempotency-Key"); got != tt.wantHeader {
					t.Fatalf("idempotency key = %q", got)
				}
				idx := calls
				calls++
				w.WriteHeader(tt.statuses[idx])
				_, _ = w.Write([]byte(tt.responses[idx]))
			}))
			defer server.Close()

			client := New("test-key")
			client.baseURL = server.URL
			client.httpClient = server.Client()
			client.sleep = func(context.Context, time.Duration) error { return nil }
			err := client.Publish(context.Background(), "game-ops", "device.status", map[string]string{"state": "online"}, "player-42", "req-7")
			if calls != tt.wantCalls {
				t.Fatalf("calls = %d, want %d", calls, tt.wantCalls)
			}
			if tt.wantCode == "" && err != nil {
				t.Fatal(err)
			}
			if tt.wantCode != "" {
				apiErr, ok := err.(*APIError)
				if !ok || apiErr.Code != tt.wantCode {
					t.Fatalf("error = %#v, want code %s", err, tt.wantCode)
				}
			}
		})
	}
}

func TestMetricsIncludesRequiredQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/metrics/query" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("name"); got != "realtime.publish" {
			t.Fatalf("name = %q", got)
		}
		if got := r.URL.Query().Get("agg"); got != "count" {
			t.Fatalf("agg = %q", got)
		}
		_, _ = w.Write([]byte(`{"ok":true,"data":{},"error":null,"metadata":{}}`))
	}))
	defer server.Close()

	client := New("test-key")
	client.baseURL = server.URL
	client.httpClient = server.Client()
	if err := client.Metrics(context.Background(), "realtime.publish", "count", nil); err != nil {
		t.Fatal(err)
	}
}
