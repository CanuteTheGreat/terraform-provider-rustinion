package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestPermissionRoleRoundTrip proves a real create-then-update round
// trip, including that PUT carries the whole replacement permission set
// rather than a patch.
func TestPermissionRoleRoundTrip(t *testing.T) {
	var gotCreate, gotUpdate PermissionRole
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
			if err := json.NewDecoder(r.Body).Decode(&gotCreate); err != nil {
				t.Fatalf("decoding create body: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
			out := gotCreate
			out.ID = "role-1"
			_ = json.NewEncoder(w).Encode(out)
		case http.MethodPut:
			if err := json.NewDecoder(r.Body).Decode(&gotUpdate); err != nil {
				t.Fatalf("decoding update body: %v", err)
			}
			out := gotUpdate
			out.ID = "role-1"
			_ = json.NewEncoder(w).Encode(out)
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	desc := "on call"
	created, err := c.CreatePermissionRole(context.Background(), PermissionRole{
		Name:        "SRE",
		Description: &desc,
		Permissions: []RolePermission{
			{ResourceType: "alert_rules", PermissionLevel: "admin"},
			{ResourceType: "dashboards", PermissionLevel: "read"},
		},
	})
	if err != nil {
		t.Fatalf("CreatePermissionRole returned an error: %v", err)
	}
	if created.ID != "role-1" || len(created.Permissions) != 2 {
		t.Fatalf("created = %+v, want id=role-1 with 2 permissions", created)
	}
	if gotCreate.Description == nil || *gotCreate.Description != "on call" {
		t.Fatalf("create body description = %v, want \"on call\"", gotCreate.Description)
	}

	updated, err := c.UpdatePermissionRole(context.Background(), "role-1", PermissionRole{
		Name:        "SRE",
		Permissions: []RolePermission{{ResourceType: "synthetic_checks", PermissionLevel: "write"}},
	})
	if err != nil {
		t.Fatalf("UpdatePermissionRole returned an error: %v", err)
	}
	if len(gotUpdate.Permissions) != 1 || gotUpdate.Permissions[0].ResourceType != "synthetic_checks" {
		t.Fatalf("update body permissions = %+v, want the single replacement", gotUpdate.Permissions)
	}
	if len(updated.Permissions) != 1 {
		t.Fatalf("updated = %+v, want 1 permission", updated)
	}
}

// TestPermissionRoleEmptyPermissionsSendsArrayNotNull pins down the
// distinction the server actually cares about: a role with no
// permissions must serialize as [], not null. A null would be
// indistinguishable from "field omitted" and could be read as
// leave-unchanged rather than the real "permits nothing" value.
func TestPermissionRoleEmptyPermissionsSendsArrayNotNull(t *testing.T) {
	var raw map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Fatalf("decoding body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(PermissionRole{ID: "role-1", Name: "Empty", Permissions: []RolePermission{}})
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	if _, err := c.CreatePermissionRole(context.Background(), PermissionRole{
		Name:        "Empty",
		Permissions: []RolePermission{},
	}); err != nil {
		t.Fatalf("CreatePermissionRole returned an error: %v", err)
	}

	perms, present := raw["permissions"]
	if !present {
		t.Fatal("permissions key absent from request body; want an explicit []")
	}
	list, ok := perms.([]any)
	if !ok {
		t.Fatalf("permissions = %#v, want a JSON array", perms)
	}
	if len(list) != 0 {
		t.Fatalf("permissions = %#v, want an empty array", list)
	}
	if desc, present := raw["description"]; !present || desc != nil {
		t.Fatalf("description = %#v, want explicit null", desc)
	}
}

func TestRoleAssignmentCreateListGetDelete(t *testing.T) {
	var createBody map[string]any
	var deletedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			if err := json.NewDecoder(r.Body).Decode(&createBody); err != nil {
				t.Fatalf("decoding create body: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(RoleAssignment{ID: "a-1", RoleID: "role-1", UserID: "user-1"})
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]RoleAssignment{
				{ID: "a-1", RoleID: "role-1", UserID: "user-1"},
				{ID: "a-2", RoleID: "role-1", UserID: "user-2"},
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
	if _, err := c.CreateRoleAssignment(context.Background(), "role-1", RoleAssignment{UserID: "user-1"}); err != nil {
		t.Fatalf("CreateRoleAssignment returned an error: %v", err)
	}
	// A tenant-wide assignment must send explicit null, same
	// null-vs-empty-string trap as the permission grant resource.
	if gid, present := createBody["group_id"]; !present || gid != nil {
		t.Fatalf("group_id = %#v, want explicit null", gid)
	}

	got, err := c.GetRoleAssignment(context.Background(), "role-1", "a-2")
	if err != nil {
		t.Fatalf("GetRoleAssignment returned an error: %v", err)
	}
	if got.UserID != "user-2" {
		t.Fatalf("got = %+v, want the a-2 row", got)
	}

	if _, err := c.GetRoleAssignment(context.Background(), "role-1", "nope"); !IsNotFound(err) {
		t.Fatalf("GetRoleAssignment for a missing id: err = %v, want a 404", err)
	}

	if err := c.DeleteRoleAssignment(context.Background(), "role-1", "a-1"); err != nil {
		t.Fatalf("DeleteRoleAssignment returned an error: %v", err)
	}
	if deletedPath != "/api/v1/customer/permission-roles/role-1/assignments/a-1" {
		t.Fatalf("delete path = %q", deletedPath)
	}
}
