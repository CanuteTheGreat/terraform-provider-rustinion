// Package client is a small, hand-written HTTP client for Rustinion's
// customer-facing config-as-code API (see
// web/src/routes/customer_api.rs in the main repo) — the same REST
// surface a `read_write`-scoped API key already gets for Terraform-style
// automation. It exists purely to give the Terraform provider a typed
// Go interface over that JSON API; it is not a general-purpose SDK.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Client is a thin wrapper around net/http, authenticated with a single
// bearer API key for the lifetime of the provider — the same key used
// for every resource type, since a Rustinion API key is tenant-wide,
// not scoped to one resource kind.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// New builds a Client. baseURL is the scheme+host (and optional path
// prefix) of a running rustinion-web instance, e.g.
// "https://app.rustinion.com" or "http://localhost:8090" for local
// acceptance testing — no trailing slash required, trimmed if present.
func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		httpClient: &http.Client{
			// A generous but bounded timeout — matches the spirit of
			// every server-side outbound HTTP call in the main Rust
			// app (see e.g. notifications.rs's own short_timeout_
			// client) that a hung request must not block forever,
			// scaled up because this is a synchronous CLI tool a
			// human is watching run `terraform apply`, not a
			// background sweep.
			Timeout: 30 * time.Second,
		},
	}
}

// apiError is the exact `{"error": "..."}` shape every customer_api.rs
// handler returns on a non-2xx response.
type apiError struct {
	Error string `json:"error"`
}

// APIError is returned for any non-2xx response from the Rustinion API,
// carrying the HTTP status so callers (in particular the provider's own
// resource Create/Read/Update/Delete methods) can distinguish "not
// found" (404 — the resource was deleted out of band, Terraform should
// drop it from state) from every other failure (which should surface as
// a real diagnostic).
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("rustinion API error (HTTP %d): %s", e.StatusCode, e.Message)
}

// IsNotFound reports whether err is an *APIError for a 404 response —
// every resource's Read method uses this to decide "remove from state"
// vs. "fail the read with a diagnostic."
func IsNotFound(err error) bool {
	var apiErr *APIError
	if e, ok := err.(*APIError); ok {
		apiErr = e
	}
	return apiErr != nil && apiErr.StatusCode == http.StatusNotFound
}

// IsConflict reports whether err is an *APIError for a 409 response —
// `AcknowledgeEscalation`'s own "nothing active to acknowledge" case.
func IsConflict(err error) bool {
	var apiErr *APIError
	if e, ok := err.(*APIError); ok {
		apiErr = e
	}
	return apiErr != nil && apiErr.StatusCode == http.StatusConflict
}

// maxRateLimitWait bounds the total time one call spends waiting out
// 429 responses before giving up and surfacing the error. The server's
// per-key quota is 60 requests per rolling 60s (web/src/rate_limit.rs),
// so a few minutes covers even a large `terraform apply` that drains the
// bucket repeatedly, while still failing in bounded time if the server
// keeps refusing for some other reason.
var maxRateLimitWait = 5 * time.Minute

// retryDelay returns how long to wait before retrying a 429: the
// server's own Retry-After (whole seconds) when present and sane,
// otherwise an exponential fallback capped at 30s.
func retryDelay(resp *http.Response, attempt int) time.Duration {
	if secs, err := strconv.Atoi(strings.TrimSpace(resp.Header.Get("Retry-After"))); err == nil && secs >= 0 {
		if secs > 60 {
			secs = 60
		}
		if secs == 0 {
			secs = 1
		}
		return time.Duration(secs) * time.Second
	}
	delay := time.Second << attempt
	if delay > 30*time.Second || delay <= 0 {
		delay = 30 * time.Second
	}
	return delay
}

// do sends one request. reqBody is marshaled as the JSON request body
// if non-nil (used for POST/PUT); respBody, if non-nil, receives the
// decoded JSON response body on success. A 204 No Content response (all
// four resources' own DELETE) is not an error just because there's
// nothing to decode into respBody — callers pass nil for respBody in
// that case.
//
// A 429 (the server's per-key rate limit) is retried after the delay the
// server asks for, up to maxRateLimitWait in total. Without this, any
// configuration with more than ~60 resources failed partway through
// `terraform apply` with "rate limit exceeded", leaving a half-applied
// state for the user to clean up. A 429 is always safe to retry: the
// server rejects it before doing any work.
func (c *Client) do(ctx context.Context, method, path string, reqBody, respBody any) error {
	var encoded []byte
	if reqBody != nil {
		var err error
		encoded, err = json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("encoding request body: %w", err)
		}
	}

	var waited time.Duration
	for attempt := 0; ; attempt++ {
		var bodyReader io.Reader
		if encoded != nil {
			bodyReader = bytes.NewReader(encoded)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
		if err != nil {
			return fmt.Errorf("building request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		if encoded != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("performing request: %w", err)
		}
		raw, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return fmt.Errorf("reading response body: %w", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			delay := retryDelay(resp, attempt)
			if waited+delay <= maxRateLimitWait {
				timer := time.NewTimer(delay)
				select {
				case <-ctx.Done():
					timer.Stop()
					return ctx.Err()
				case <-timer.C:
				}
				waited += delay
				continue
			}
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			message := string(raw)
			var decoded apiError
			if json.Unmarshal(raw, &decoded) == nil && decoded.Error != "" {
				message = decoded.Error
			}
			return &APIError{StatusCode: resp.StatusCode, Message: message}
		}

		if respBody != nil && len(raw) > 0 {
			if err := json.Unmarshal(raw, respBody); err != nil {
				return fmt.Errorf("decoding response body: %w", err)
			}
		}
		return nil
	}
}
