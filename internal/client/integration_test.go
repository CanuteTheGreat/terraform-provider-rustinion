package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateIntegrationSendsBearerAuthAndDecodesResponse(t *testing.T) {
	var gotAuth string
	var gotMethod, gotPath string
	var gotBody Integration

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotMethod = r.Method
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decoding request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		dashboardID := "dash-1"
		resp := gotBody
		resp.ID = "integration-123"
		resp.DashboardID = &dashboardID
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	created, err := c.CreateIntegration(context.Background(), Integration{
		Kind:    "nginx",
		GroupID: "group-1",
		Config:  json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("CreateIntegration returned an error: %v", err)
	}

	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer test-key")
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/v1/customer/integrations" {
		t.Errorf("path = %q, want /api/v1/customer/integrations", gotPath)
	}
	if gotBody.Kind != "nginx" {
		t.Errorf("request body kind = %q, want %q", gotBody.Kind, "nginx")
	}
	if created.ID != "integration-123" {
		t.Errorf("created.ID = %q, want %q", created.ID, "integration-123")
	}
	if created.DashboardID == nil || *created.DashboardID != "dash-1" {
		t.Errorf("created.DashboardID = %v, want a pointer to %q", created.DashboardID, "dash-1")
	}
}

func TestUpdateIntegrationOnlySendsConfigOnTheWayInButDecodesEveryField(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %q, want PUT", r.Method)
		}
		if r.URL.Path != "/api/v1/customer/integrations/int-1" {
			t.Errorf("path = %q", r.URL.Path)
		}
		alertRuleID := "alert-1"
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Integration{
			ID:          "int-1",
			Kind:        "postgresql",
			GroupID:     "group-1",
			Config:      json.RawMessage(`{"target":"db.internal:5432"}`),
			AlertRuleID: &alertRuleID,
		})
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	updated, err := c.UpdateIntegration(context.Background(), "int-1", Integration{
		Config: json.RawMessage(`{"target":"db.internal:5432"}`),
	})
	if err != nil {
		t.Fatalf("UpdateIntegration returned an error: %v", err)
	}
	if string(updated.Config) != `{"target":"db.internal:5432"}` {
		t.Errorf("updated.Config = %s, want passed through unmodified", updated.Config)
	}
	if updated.AlertRuleID == nil || *updated.AlertRuleID != "alert-1" {
		t.Errorf("updated.AlertRuleID = %v, want a pointer to %q", updated.AlertRuleID, "alert-1")
	}
}

func TestGetIntegrationReturnsAPIErrorOnNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "no such integration"})
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	_, err := c.GetIntegration(context.Background(), "missing-id")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !IsNotFound(err) {
		t.Errorf("IsNotFound(err) = false for a 404 response, want true")
	}
}

func TestDeleteIntegrationSucceedsOn204WithNoBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %q, want DELETE", r.Method)
		}
		if r.URL.Path != "/api/v1/customer/integrations/int-1" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	if err := c.DeleteIntegration(context.Background(), "int-1"); err != nil {
		t.Fatalf("DeleteIntegration returned an error for a 204 response: %v", err)
	}
}
