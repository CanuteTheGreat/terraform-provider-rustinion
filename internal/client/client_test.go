package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// These are plain unit tests against an httptest server — they verify
// this package's own request construction, auth header, and error/404
// handling in isolation, fast and without any real infrastructure. They
// are a complement to, not a substitute for, this provider's real
// acceptance tests (internal/provider/*_test.go, TF_ACC=1) which
// exercise the actual customer API end to end.

func TestCreateAlertRuleSendsBearerAuthAndDecodesResponse(t *testing.T) {
	var gotAuth string
	var gotMethod, gotPath string
	var gotBody AlertRule

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotMethod = r.Method
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decoding request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		resp := gotBody
		resp.ID = "rule-123"
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	created, err := c.CreateAlertRule(context.Background(), AlertRule{
		GroupID:  "group-1",
		Name:     "high cpu",
		BoolOp:   "and",
		Severity: "critical",
		Enabled:  true,
		Conditions: []AlertRuleCondition{
			{MetricType: "cpu_avg_pct", ConditionKind: "threshold", Comparator: "gt", Threshold: 90, WindowMinutes: 60},
		},
	})
	if err != nil {
		t.Fatalf("CreateAlertRule returned an error: %v", err)
	}

	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer test-key")
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/v1/customer/alert-rules" {
		t.Errorf("path = %q, want /api/v1/customer/alert-rules", gotPath)
	}
	if gotBody.Name != "high cpu" {
		t.Errorf("request body name = %q, want %q", gotBody.Name, "high cpu")
	}
	if len(gotBody.Conditions) != 1 || gotBody.Conditions[0].MetricType != "cpu_avg_pct" {
		t.Errorf("request body conditions not encoded correctly: %+v", gotBody.Conditions)
	}
	if created.ID != "rule-123" {
		t.Errorf("created.ID = %q, want %q", created.ID, "rule-123")
	}
}

func TestGetAlertRuleReturnsAPIErrorOnNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "this API key is read-only"})
	}))
	defer server.Close()

	c := New(server.URL, "read-only-key")
	_, err := c.GetAlertRule(context.Background(), "some-id")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusForbidden {
		t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusForbidden)
	}
	if apiErr.Message != "this API key is read-only" {
		t.Errorf("Message = %q, want the server's own error text", apiErr.Message)
	}
}

func TestIsNotFoundDistinguishes404FromOtherErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "no such alert rule"})
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	_, err := c.GetAlertRule(context.Background(), "missing-id")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !IsNotFound(err) {
		t.Errorf("IsNotFound(err) = false for a 404 response, want true")
	}

	if IsNotFound(nil) {
		t.Error("IsNotFound(nil) = true, want false")
	}
}

func TestDeleteAlertRuleSucceedsOn204WithNoBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %q, want DELETE", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	if err := c.DeleteAlertRule(context.Background(), "rule-1"); err != nil {
		t.Fatalf("DeleteAlertRule returned an error for a 204 response: %v", err)
	}
}

func TestUpdateNotificationChannelPassesConfigThroughUnmodified(t *testing.T) {
	var gotBody NotificationChannel
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %q, want PUT", r.Method)
		}
		if r.URL.Path != "/api/v1/customer/notification-channels/chan-1" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(gotBody)
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	config := json.RawMessage(`{"webhook_url":"https://hooks.slack.com/x"}`)
	updated, err := c.UpdateNotificationChannel(context.Background(), "chan-1", NotificationChannel{
		Name:        "ops-slack",
		ChannelType: "slack",
		Config:      config,
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("UpdateNotificationChannel returned an error: %v", err)
	}
	if string(updated.Config) != string(config) {
		t.Errorf("Config = %s, want %s (passed through unmodified)", updated.Config, config)
	}
}
