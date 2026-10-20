package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateEscalationPolicySendsBearerAuthAndDecodesResponse(t *testing.T) {
	var gotAuth string
	var gotMethod, gotPath string
	var gotBody EscalationPolicy

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
		resp.ID = "policy-123"
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	created, err := c.CreateEscalationPolicy(context.Background(), EscalationPolicy{
		GroupID: "group-1",
		Name:    "Primary on-call",
		Steps: []EscalationStep{
			{ChannelIDs: []string{"chan-1", "chan-2"}, WaitMinutes: 15},
		},
	})
	if err != nil {
		t.Fatalf("CreateEscalationPolicy returned an error: %v", err)
	}

	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer test-key")
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/v1/customer/escalation-policies" {
		t.Errorf("path = %q, want /api/v1/customer/escalation-policies", gotPath)
	}
	if len(gotBody.Steps) != 1 || len(gotBody.Steps[0].ChannelIDs) != 2 ||
		gotBody.Steps[0].ChannelIDs[0] != "chan-1" || gotBody.Steps[0].ChannelIDs[1] != "chan-2" {
		t.Errorf("request body steps not encoded correctly: %+v", gotBody.Steps)
	}
	if created.ID != "policy-123" {
		t.Errorf("created.ID = %q, want %q", created.ID, "policy-123")
	}
}

func TestUpdateEscalationPolicySendsPUTToThePolicyID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %q, want PUT", r.Method)
		}
		if r.URL.Path != "/api/v1/customer/escalation-policies/policy-1" {
			t.Errorf("path = %q", r.URL.Path)
		}
		var body EscalationPolicy
		_ = json.NewDecoder(r.Body).Decode(&body)
		body.ID = "policy-1"
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	updated, err := c.UpdateEscalationPolicy(context.Background(), "policy-1", EscalationPolicy{
		GroupID: "group-1",
		Name:    "renamed",
	})
	if err != nil {
		t.Fatalf("UpdateEscalationPolicy returned an error: %v", err)
	}
	if updated.Name != "renamed" {
		t.Errorf("updated.Name = %q, want %q", updated.Name, "renamed")
	}
}

func TestGetEscalationPolicyReturnsAPIErrorOnNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "no such escalation policy"})
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	_, err := c.GetEscalationPolicy(context.Background(), "missing-id")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !IsNotFound(err) {
		t.Errorf("IsNotFound(err) = false for a 404 response, want true")
	}
}

func TestDeleteEscalationPolicySucceedsOn204WithNoBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %q, want DELETE", r.Method)
		}
		if r.URL.Path != "/api/v1/customer/escalation-policies/policy-1" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	if err := c.DeleteEscalationPolicy(context.Background(), "policy-1"); err != nil {
		t.Fatalf("DeleteEscalationPolicy returned an error for a 204 response: %v", err)
	}
}
