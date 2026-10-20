package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreatePermissionRoleMappingSendsTupleAndNullGroup(t *testing.T) {
	var raw map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Fatalf("decoding body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(PermissionRoleMapping{
			ID:            "map-1",
			Source:        "oidc",
			ExternalGroup: "sre",
			RoleID:        "role-1",
		})
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	created, err := c.CreatePermissionRoleMapping(context.Background(), PermissionRoleMapping{
		Source:        "oidc",
		ExternalGroup: "sre",
		RoleID:        "role-1",
	})
	if err != nil {
		t.Fatalf("CreatePermissionRoleMapping returned an error: %v", err)
	}
	if raw["source"] != "oidc" || raw["external_group"] != "sre" || raw["role_id"] != "role-1" {
		t.Fatalf("request body = %#v", raw)
	}
	// Same null-vs-empty-string trap as the other scoped resources: an
	// empty string would reach the server as an invalid uuid.
	if gid, present := raw["group_id"]; !present || gid != nil {
		t.Fatalf("group_id = %#v, want explicit null", gid)
	}
	if created.ID != "map-1" || created.GroupID != nil {
		t.Fatalf("created = %+v", created)
	}
}

func TestListAndDeletePermissionRoleMappings(t *testing.T) {
	var deletedPath string
	group := "group-1"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]PermissionRoleMapping{
				{ID: "map-1", Source: "saml", ExternalGroup: "sre", RoleID: "role-1"},
				{ID: "map-2", Source: "oidc", ExternalGroup: "billing", RoleID: "role-2", GroupID: &group},
			})
		case http.MethodDelete:
			deletedPath = r.URL.Path
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	mappings, err := c.ListPermissionRoleMappings(context.Background())
	if err != nil {
		t.Fatalf("ListPermissionRoleMappings returned an error: %v", err)
	}
	if len(mappings) != 2 || mappings[0].Source != "saml" {
		t.Fatalf("mappings = %+v", mappings)
	}
	if mappings[1].GroupID == nil || *mappings[1].GroupID != "group-1" {
		t.Fatalf("scoped mapping lost its group: %+v", mappings[1])
	}

	if err := c.DeletePermissionRoleMapping(context.Background(), "map-1"); err != nil {
		t.Fatalf("DeletePermissionRoleMapping returned an error: %v", err)
	}
	if deletedPath != "/api/v1/customer/permission-role-mappings/map-1" {
		t.Fatalf("delete path = %q", deletedPath)
	}
}
