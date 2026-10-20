package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateSilenceSendsBearerAuthAndDecodesResponse(t *testing.T) {
	var gotAuth string
	var gotMethod, gotPath string
	var gotBody Silence

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
		resp.ID = "silence-123"
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	tag := "env:staging"
	created, err := c.CreateSilence(context.Background(), Silence{
		Name:     "known issue",
		Tag:      &tag,
		StartsAt: "2026-01-01T00:00:00Z",
		EndsAt:   "2026-01-02T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("CreateSilence returned an error: %v", err)
	}

	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer test-key")
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/v1/customer/silences" {
		t.Errorf("path = %q, want /api/v1/customer/silences", gotPath)
	}
	if gotBody.Tag == nil || *gotBody.Tag != "env:staging" {
		t.Errorf("request body tag = %v, want a pointer to %q", gotBody.Tag, "env:staging")
	}
	if created.ID != "silence-123" {
		t.Errorf("created.ID = %q, want %q", created.ID, "silence-123")
	}
}

func TestGetSilenceReturnsAPIErrorOnNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "no such silence"})
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	_, err := c.GetSilence(context.Background(), "missing-id")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !IsNotFound(err) {
		t.Errorf("IsNotFound(err) = false for a 404 response, want true")
	}
}

func TestDeleteSilenceSendsDeleteToTheCancelEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %q, want DELETE", r.Method)
		}
		if r.URL.Path != "/api/v1/customer/silences/silence-1" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	if err := c.DeleteSilence(context.Background(), "silence-1"); err != nil {
		t.Fatalf("DeleteSilence returned an error for a 204 response: %v", err)
	}
}
