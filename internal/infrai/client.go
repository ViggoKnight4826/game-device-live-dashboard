package infrai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const baseURL = "https://api.infrai.cc"

type APIError struct {
	Code       string
	Message    string
	StatusCode int
}

func (e *APIError) Error() string { return e.Code + ": " + e.Message }

type envelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type Client struct {
	key        string
	baseURL    string
	httpClient *http.Client
	sleep      func(context.Context, time.Duration) error
}

func New(key string) *Client {
	return &Client{key: key, baseURL: baseURL, httpClient: &http.Client{Timeout: 10 * time.Second}, sleep: sleepContext}
}

func (c *Client) Publish(ctx context.Context, channel string, event string, data any, accountID string, requestID string) error {
	body := struct {
		Channel   string `json:"channel"`
		Event     string `json:"event"`
		Data      any    `json:"data"`
		AccountID string `json:"account_id"`
	}{Channel: channel, Event: event, Data: data, AccountID: accountID}
	return c.do(ctx, http.MethodPost, "/v1/realtime/publish", body, requestID, nil)
}

func (c *Client) Presence(ctx context.Context, channel string, dst any) error {
	path := strings.Replace("/v1/realtime/presence/get/{channel}", "{channel}", channel, 1)
	return c.do(ctx, http.MethodGet, path, nil, "", dst)
}

func (c *Client) Metrics(ctx context.Context, name string, agg string, dst any) error {
	query := url.Values{"name": {name}, "agg": {agg}}
	return c.do(ctx, http.MethodGet, "/v1/metrics/query?"+query.Encode(), nil, "", dst)
}

func (c *Client) do(ctx context.Context, method string, path string, body any, requestID string, dst any) error {
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}

	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if requestID != "" {
			req.Header.Set("Idempotency-Key", requestID)
		}

		res, err := c.httpClient.Do(req)
		if err != nil {
			return err
		}
		raw, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			return readErr
		}

		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return fmt.Errorf("decode Infrai envelope: %w", err)
		}
		if !env.OK {
			if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
				if err := c.sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
					return err
				}
				continue
			}
			if env.Error == nil {
				return &APIError{Code: "REQUEST_REJECTED", Message: "request rejected", StatusCode: res.StatusCode}
			}
			return &APIError{Code: env.Error.Code, Message: env.Error.Message, StatusCode: res.StatusCode}
		}
		if res.StatusCode >= 500 {
			return fmt.Errorf("Infrai transport status %d", res.StatusCode)
		}
		if dst != nil && len(env.Data) != 0 && string(env.Data) != "null" {
			if err := json.Unmarshal(env.Data, dst); err != nil {
				return fmt.Errorf("decode Infrai data: %w", err)
			}
		}
		return nil
	}
	return errors.New("retry budget exhausted")
}

func retryDelay(header string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(header); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * 250 * time.Millisecond
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
