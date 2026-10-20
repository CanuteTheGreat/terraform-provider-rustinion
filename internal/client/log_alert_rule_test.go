package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateLogAlertRuleSendsBearerAuthAndDecodesResponse(t *testing.T) {
	var gotAuth string
	var gotMethod, gotPath string
	var gotBody LogAlertRule

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
	created, err := c.CreateLogAlertRule(context.Background(), LogAlertRule{
		GroupID:        "group-1",
		Name:           "ssh failures",
		ThresholdCount: 1,
		WindowMinutes:  5,
		Severity:       "critical",
		Enabled:        true,
	})
	if err != nil {
		t.Fatalf("CreateLogAlertRule returned an error: %v", err)
	}

	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer test-key")
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/v1/customer/log-alert-rules" {
		t.Errorf("path = %q, want /api/v1/customer/log-alert-rules", gotPath)
	}
	if gotBody.Name != "ssh failures" {
		t.Errorf("request body name = %q, want %q", gotBody.Name, "ssh failures")
	}
	if created.ID != "rule-123" {
		t.Errorf("created.ID = %q, want %q", created.ID, "rule-123")
	}
}

func TestCreateLogAlertRuleOmitsNilOptionalFilters(t *testing.T) {
	var raw map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Fatalf("decoding request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(LogAlertRule{ID: "rule-1"})
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	_, err := c.CreateLogAlertRule(context.Background(), LogAlertRule{
		GroupID: "group-1",
		Name:    "no filters",
	})
	if err != nil {
		t.Fatalf("CreateLogAlertRule returned an error: %v", err)
	}

	for _, field := range []string{"message_pattern", "log_severity", "log_source", "unit_pattern", "escalation_policy_id"} {
		if _, present := raw[field]; present {
			t.Errorf("request body unexpectedly included %q: %v", field, raw[field])
		}
	}
}

func TestGetLogAlertRuleReturnsAPIErrorOnNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "no such log alert rule"})
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	_, err := c.GetLogAlertRule(context.Background(), "missing-id")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !IsNotFound(err) {
		t.Errorf("IsNotFound(err) = false for a 404 response, want true")
	}
}

func TestDeleteLogAlertRuleSucceedsOn204WithNoBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %q, want DELETE", r.Method)
		}
		if r.URL.Path != "/api/v1/customer/log-alert-rules/rule-1" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	if err := c.DeleteLogAlertRule(context.Background(), "rule-1"); err != nil {
		t.Fatalf("DeleteLogAlertRule returned an error for a 204 response: %v", err)
	}
}

func TestUpdateLogAlertRuleSendsPUTToTheRuleID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %q, want PUT", r.Method)
		}
		if r.URL.Path != "/api/v1/customer/log-alert-rules/rule-1" {
			t.Errorf("path = %q", r.URL.Path)
		}
		var body LogAlertRule
		_ = json.NewDecoder(r.Body).Decode(&body)
		body.ID = "rule-1"
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	updated, err := c.UpdateLogAlertRule(context.Background(), "rule-1", LogAlertRule{
		GroupID: "group-1",
		Name:    "renamed",
	})
	if err != nil {
		t.Fatalf("UpdateLogAlertRule returned an error: %v", err)
	}
	if updated.Name != "renamed" {
		t.Errorf("updated.Name = %q, want %q", updated.Name, "renamed")
	}
}
