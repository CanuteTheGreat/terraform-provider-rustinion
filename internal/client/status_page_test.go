package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPutStatusPageSendsBearerAuthToTheFixedPathAndDecodesResponse(t *testing.T) {
	var gotAuth string
	var gotMethod, gotPath string
	var gotBody StatusPage

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotMethod = r.Method
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decoding request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		resp := gotBody
		resp.ID = "page-123"
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	created, err := c.PutStatusPage(context.Background(), StatusPage{
		Slug:  "status",
		Title: "Our status",
		Components: []StatusPageComponent{
			{SyntheticCheckID: "check-1", DisplayName: "API"},
		},
	})
	if err != nil {
		t.Fatalf("PutStatusPage returned an error: %v", err)
	}

	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer test-key")
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method = %q, want PUT", gotMethod)
	}
	if gotPath != "/api/v1/customer/status-page" {
		t.Errorf("path = %q, want /api/v1/customer/status-page (no id — singleton)", gotPath)
	}
	if len(gotBody.Components) != 1 || gotBody.Components[0].SyntheticCheckID != "check-1" {
		t.Errorf("request body components not encoded correctly: %+v", gotBody.Components)
	}
	if created.ID != "page-123" {
		t.Errorf("created.ID = %q, want %q", created.ID, "page-123")
	}
}

func TestGetStatusPageReturnsAPIErrorOnNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/customer/status-page" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "this tenant has no status page configured"})
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	_, err := c.GetStatusPage(context.Background())
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !IsNotFound(err) {
		t.Errorf("IsNotFound(err) = false for a 404 response, want true")
	}
}

func TestDeleteStatusPageSucceedsOn204WithNoBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %q, want DELETE", r.Method)
		}
		if r.URL.Path != "/api/v1/customer/status-page" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	if err := c.DeleteStatusPage(context.Background()); err != nil {
		t.Fatalf("DeleteStatusPage returned an error for a 204 response: %v", err)
	}
}
