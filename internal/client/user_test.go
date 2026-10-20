package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateUserSendsBearerAuthAndDecodesResponse(t *testing.T) {
	var gotAuth string
	var gotMethod, gotPath string
	var gotBody User

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
		resp.ID = "role-123"
		resp.UserID = "user-456"
		if resp.AccessLevel == "" {
			resp.AccessLevel = "admin"
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	created, err := c.CreateUser(context.Background(), User{Email: "new-admin@example.com"})
	if err != nil {
		t.Fatalf("CreateUser returned an error: %v", err)
	}

	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer test-key")
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/v1/customer/users" {
		t.Errorf("path = %q, want /api/v1/customer/users", gotPath)
	}
	if gotBody.Email != "new-admin@example.com" {
		t.Errorf("request body email = %q, want %q", gotBody.Email, "new-admin@example.com")
	}
	if created.ID != "role-123" {
		t.Errorf("created.ID = %q, want %q", created.ID, "role-123")
	}
	if created.UserID != "user-456" {
		t.Errorf("created.UserID = %q, want %q", created.UserID, "user-456")
	}
}

func TestUpdateUserAccessLevelSendsPUTWithJustTheAccessLevel(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %q, want PUT", r.Method)
		}
		if r.URL.Path != "/api/v1/customer/users/role-1" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(User{ID: "role-1", Email: "a@example.com", AccessLevel: "read_only"})
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	updated, err := c.UpdateUserAccessLevel(context.Background(), "role-1", "read_only")
	if err != nil {
		t.Fatalf("UpdateUserAccessLevel returned an error: %v", err)
	}
	if gotBody["access_level"] != "read_only" {
		t.Errorf("request body access_level = %v, want %q", gotBody["access_level"], "read_only")
	}
	if _, present := gotBody["email"]; present {
		t.Errorf("request body unexpectedly included email: %v", gotBody)
	}
	if updated.AccessLevel != "read_only" {
		t.Errorf("updated.AccessLevel = %q, want %q", updated.AccessLevel, "read_only")
	}
}

func TestGetUserReturnsAPIErrorOnNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "no such user"})
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	_, err := c.GetUser(context.Background(), "missing-id")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !IsNotFound(err) {
		t.Errorf("IsNotFound(err) = false for a 404 response, want true")
	}
}

func TestDeleteUserSucceedsOn204WithNoBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %q, want DELETE", r.Method)
		}
		if r.URL.Path != "/api/v1/customer/users/role-1" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	if err := c.DeleteUser(context.Background(), "role-1"); err != nil {
		t.Fatalf("DeleteUser returned an error for a 204 response: %v", err)
	}
}
