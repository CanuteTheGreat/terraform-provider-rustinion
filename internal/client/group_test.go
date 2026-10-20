package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateGroupSendsBearerAuthAndDecodesResponse(t *testing.T) {
	var gotAuth string
	var gotMethod, gotPath string
	var gotBody Group

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotMethod = r.Method
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decoding request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(Group{ID: "group-123", Name: gotBody.Name, ManifestVersion: 1})
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	created, err := c.CreateGroup(context.Background(), Group{Name: "Engineering", TemplateKey: "linux_web_server"})
	if err != nil {
		t.Fatalf("CreateGroup returned an error: %v", err)
	}

	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer test-key")
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/v1/customer/groups" {
		t.Errorf("path = %q, want /api/v1/customer/groups", gotPath)
	}
	if gotBody.TemplateKey != "linux_web_server" {
		t.Errorf("request body template_key = %q, want %q", gotBody.TemplateKey, "linux_web_server")
	}
	if created.ID != "group-123" {
		t.Errorf("created.ID = %q, want %q", created.ID, "group-123")
	}
	if created.TemplateKey != "" {
		t.Errorf("created.TemplateKey = %q, want empty — the server never returns it", created.TemplateKey)
	}
}

func TestApplyGroupTemplateSendsPUTWithJustTheTemplateKey(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %q, want PUT", r.Method)
		}
		if r.URL.Path != "/api/v1/customer/groups/group-1" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Group{ID: "group-1", Name: "g", ManifestVersion: 2})
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	updated, err := c.ApplyGroupTemplate(context.Background(), "group-1", "macos_workstation")
	if err != nil {
		t.Fatalf("ApplyGroupTemplate returned an error: %v", err)
	}
	if gotBody["template_key"] != "macos_workstation" {
		t.Errorf("request body template_key = %v, want %q", gotBody["template_key"], "macos_workstation")
	}
	if _, present := gotBody["name"]; present {
		t.Errorf("request body unexpectedly included name: %v", gotBody)
	}
	if updated.ManifestVersion != 2 {
		t.Errorf("updated.ManifestVersion = %d, want 2", updated.ManifestVersion)
	}
}

func TestGetGroupReturnsAPIErrorOnNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "no such group"})
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	_, err := c.GetGroup(context.Background(), "missing-id")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !IsNotFound(err) {
		t.Errorf("IsNotFound(err) = false for a 404 response, want true")
	}
}

func TestDeleteGroupSurfacesAForeignKeyConflictAsA400(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %q, want DELETE", r.Method)
		}
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "cannot delete a group that still has devices or outstanding enrollment tokens"})
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	err := c.DeleteGroup(context.Background(), "group-1")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusBadRequest)
	}
}
