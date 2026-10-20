package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAcknowledgeEscalationSendsPOSTToTheAcknowledgePath(t *testing.T) {
	var gotMethod, gotPath, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	if err := c.AcknowledgeEscalation(context.Background(), "alert_rule", "rule-1"); err != nil {
		t.Fatalf("AcknowledgeEscalation returned an error: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/v1/customer/escalations/alert_rule/rule-1/acknowledge" {
		t.Errorf("path = %q", gotPath)
	}
	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer test-key")
	}
}

func TestAcknowledgeEscalationReturnsConflictOn409(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "no active, unacknowledged escalation for this rule"})
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	err := c.AcknowledgeEscalation(context.Background(), "alert_rule", "rule-1")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !IsConflict(err) {
		t.Errorf("IsConflict(err) = false for a 409 response, want true")
	}
	if IsNotFound(err) {
		t.Errorf("IsNotFound(err) = true for a 409 response, want false")
	}
}

func TestAcknowledgeEscalationReturnsNotFoundOn404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "no such alert_rule or log_alert_rule in this tenant"})
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	err := c.AcknowledgeEscalation(context.Background(), "alert_rule", "rule-1")
	if !IsNotFound(err) {
		t.Errorf("IsNotFound(err) = false for a 404 response, want true")
	}
}

func TestGetEscalationStatusDecodesTheResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/customer/escalations/alert_rule/rule-1" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		ackedAt := "2026-01-01T00:00:00Z"
		_ = json.NewEncoder(w).Encode(EscalationStatus{
			RuleKind:        "alert_rule",
			RuleID:          "rule-1",
			CurrentStep:     2,
			StartedAt:       "2026-01-01T00:00:00Z",
			LastEscalatedAt: "2026-01-01T00:00:00Z",
			AcknowledgedAt:  &ackedAt,
		})
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	status, err := c.GetEscalationStatus(context.Background(), "alert_rule", "rule-1")
	if err != nil {
		t.Fatalf("GetEscalationStatus returned an error: %v", err)
	}
	if status.CurrentStep != 2 {
		t.Errorf("CurrentStep = %d, want 2", status.CurrentStep)
	}
	if status.AcknowledgedAt == nil || *status.AcknowledgedAt != "2026-01-01T00:00:00Z" {
		t.Errorf("AcknowledgedAt = %v, want a timestamp", status.AcknowledgedAt)
	}
}

func TestGetEscalationStatusReturnsNotFoundWhenTheRuleHasNeverEscalated(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "no escalation has ever run for this rule"})
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	_, err := c.GetEscalationStatus(context.Background(), "alert_rule", "rule-1")
	if !IsNotFound(err) {
		t.Errorf("IsNotFound(err) = false for a 404 response, want true")
	}
}
