package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// A 429 from the server's per-key rate limiter must be waited out and
// retried with the identical request, not surfaced as a failed apply.
func TestRateLimitedRequestIsRetriedAfterRetryAfter(t *testing.T) {
	var calls atomic.Int32
	var bodies []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got AlertRule
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decoding request body on call %d: %v", calls.Load()+1, err)
		}
		bodies = append(bodies, got.Name)
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"rate limit exceeded, try again shortly"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		got.ID = "rule-after-retry"
		_ = json.NewEncoder(w).Encode(got)
	}))
	defer server.Close()

	start := time.Now()
	created, err := New(server.URL, "k").CreateAlertRule(context.Background(), AlertRule{GroupID: "g", Name: "retried"})
	if err != nil {
		t.Fatalf("CreateAlertRule after one 429 returned an error: %v", err)
	}
	if created.ID != "rule-after-retry" {
		t.Errorf("created.ID = %q, want rule-after-retry", created.ID)
	}
	if n := calls.Load(); n != 2 {
		t.Errorf("server saw %d calls, want 2 (one 429, one retry)", n)
	}
	if len(bodies) != 2 || bodies[0] != "retried" || bodies[1] != "retried" {
		t.Errorf("retry must resend the same body, got %q", bodies)
	}
	if elapsed := time.Since(start); elapsed < 900*time.Millisecond {
		t.Errorf("retried after %v, want it to honour Retry-After: 1", elapsed)
	}
}

// Past the total wait budget the 429 is reported, so a server that keeps
// refusing can't hang `terraform apply` forever.
func TestRateLimitGivesUpAfterTheWaitBudget(t *testing.T) {
	old := maxRateLimitWait
	maxRateLimitWait = 1500 * time.Millisecond
	defer func() { maxRateLimitWait = old }()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"rate limit exceeded, try again shortly"}`))
	}))
	defer server.Close()

	_, err := New(server.URL, "k").GetAlertRule(context.Background(), "x")
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("err = %v, want an *APIError with HTTP 429", err)
	}
	if n := calls.Load(); n != 2 {
		t.Errorf("server saw %d calls, want 2 (one retry fits a 1.5s budget)", n)
	}
}

// Cancelling the context (Ctrl-C on `terraform apply`) must stop the wait.
func TestRateLimitWaitStopsWhenContextIsCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := New(server.URL, "k").GetAlertRule(ctx, "x")
	if err != context.DeadlineExceeded {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("cancelled wait took %v", elapsed)
	}
}
