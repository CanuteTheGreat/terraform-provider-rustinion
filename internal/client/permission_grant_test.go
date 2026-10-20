package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestCreatePermissionGrantUpsertsAndKeepsTheSameID proves the
// property the Terraform resource's own Update depends on: re-POSTing
// the same (user_id, group_id, resource_type) scope with a new level
// returns the SAME id, so a level change never has to destroy and
// recreate the grant. There is no PUT endpoint at all, by design — see
// PermissionGrant's own doc comment.
func TestCreatePermissionGrantUpsertsAndKeepsTheSameID(t *testing.T) {
	var got []PermissionGrant
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method %s", r.Method)
		}
		var in PermissionGrant
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Fatalf("decoding request body: %v", err)
		}
		got = append(got, in)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		out := in
		out.ID = "grant-1"
		_ = json.NewEncoder(w).Encode(out)
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	groupID := "group-1"

	first, err := c.CreatePermissionGrant(context.Background(), PermissionGrant{
		UserID:          "user-1",
		GroupID:         &groupID,
		ResourceType:    "alert_rules",
		PermissionLevel: "read",
	})
	if err != nil {
		t.Fatalf("CreatePermissionGrant returned an error: %v", err)
	}
	second, err := c.CreatePermissionGrant(context.Background(), PermissionGrant{
		UserID:          "user-1",
		GroupID:         &groupID,
		ResourceType:    "alert_rules",
		PermissionLevel: "write",
	})
	if err != nil {
		t.Fatalf("CreatePermissionGrant (upsert) returned an error: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("got %d requests, want 2", len(got))
	}
	if got[0].PermissionLevel != "read" || got[1].PermissionLevel != "write" {
		t.Fatalf("request levels = %q, %q; want read, write", got[0].PermissionLevel, got[1].PermissionLevel)
	}
	if got[1].GroupID == nil || *got[1].GroupID != "group-1" {
		t.Fatalf("group_id = %v, want group-1", got[1].GroupID)
	}
	if first.ID != second.ID {
		t.Fatalf("ids differ across an upsert: %q vs %q", first.ID, second.ID)
	}
	if second.PermissionLevel != "write" {
		t.Fatalf("second.PermissionLevel = %q, want write", second.PermissionLevel)
	}
}

// TestPermissionGrantTenantWideSendsNullGroupID pins the nullability
// of the scope down to the wire: a tenant-wide grant must send JSON
// null, never "" — an empty string is an invalid uuid the server
// rejects, so collapsing the two would turn "every group" into a 400.
func TestPermissionGrantTenantWideSendsNullGroupID(t *testing.T) {
	var raw map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Fatalf("decoding request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(PermissionGrant{
			ID:              "grant-1",
			UserID:          "user-1",
			ResourceType:    "dashboards",
			PermissionLevel: "admin",
		})
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	created, err := c.CreatePermissionGrant(context.Background(), PermissionGrant{
		UserID:          "user-1",
		ResourceType:    "dashboards",
		PermissionLevel: "admin",
	})
	if err != nil {
		t.Fatalf("CreatePermissionGrant returned an error: %v", err)
	}

	v, present := raw["group_id"]
	if !present {
		t.Fatal("group_id key is absent from the request body; want an explicit null")
	}
	if v != nil {
		t.Fatalf("group_id = %#v, want null", v)
	}
	if created.GroupID != nil {
		t.Fatalf("created.GroupID = %v, want nil for a tenant-wide grant", created.GroupID)
	}
}

func TestListAndDeletePermissionGrants(t *testing.T) {
	var deletedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]PermissionGrant{
				{ID: "grant-1", UserID: "user-1", ResourceType: "alert_rules", PermissionLevel: "write"},
				{ID: "grant-2", UserID: "user-2", ResourceType: "dashboards", PermissionLevel: "read"},
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
	grants, err := c.ListPermissionGrants(context.Background())
	if err != nil {
		t.Fatalf("ListPermissionGrants returned an error: %v", err)
	}
	if len(grants) != 2 || grants[0].ID != "grant-1" || grants[1].PermissionLevel != "read" {
		t.Fatalf("grants = %+v, want the two seeded grants", grants)
	}

	if err := c.DeletePermissionGrant(context.Background(), "grant-1"); err != nil {
		t.Fatalf("DeletePermissionGrant returned an error: %v", err)
	}
	if deletedPath != "/api/v1/customer/permission-grants/grant-1" {
		t.Fatalf("delete path = %q", deletedPath)
	}
}
