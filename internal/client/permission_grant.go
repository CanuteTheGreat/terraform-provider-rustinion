package client

import (
	"context"
	"net/http"
)

// PermissionGrant mirrors `routes::customer_api::PermissionGrantInput`/
// `CustomerPermissionGrant` on the wire — one struct for both
// directions, same convention as ProcessWatchRule and LogAlertRule.
//
// GroupID is a *string rather than a string because a grant's scope is
// genuinely nullable: `null` means tenant-wide (every group), matching
// `user_roles.group_id`'s own convention. An empty string would be a
// different thing entirely — an invalid uuid the server would reject —
// so the nullability has to survive the round trip.
//
// There is deliberately no UpdatePermissionGrant. A grant's identity is
// its (UserID, GroupID, ResourceType) tuple — that is exactly what the
// server's two partial unique indexes enforce — so the only mutable
// field is PermissionLevel, and CreatePermissionGrant already changes
// it: the server upserts and returns the SAME ID with the new level.
// See resource_permission_grant.go's own Update, which relies on this.
type PermissionGrant struct {
	ID string `json:"id,omitempty"`
	// UserID is `users.id` — the value `rustinion_user` exposes as its
	// own `user_id` attribute, NOT that resource's `id` (which is a
	// user_roles row id). A grant is per user, not per role row.
	UserID string `json:"user_id"`
	// GroupID nil = tenant-wide.
	GroupID *string `json:"group_id"`
	// ResourceType is one of "alert_rules", "dashboards",
	// "synthetic_checks", "notification_channels".
	ResourceType string `json:"resource_type"`
	// PermissionLevel is one of "read", "write", "admin".
	PermissionLevel string `json:"permission_level"`
}

// CreatePermissionGrant creates a grant, or updates the level of the
// one already covering this exact scope, returning the same ID either
// way — the server side is an upsert, not a plain insert.
func (c *Client) CreatePermissionGrant(ctx context.Context, grant PermissionGrant) (*PermissionGrant, error) {
	var out PermissionGrant
	if err := c.do(ctx, http.MethodPost, "/api/v1/customer/permission-grants", grant, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetPermissionGrant(ctx context.Context, id string) (*PermissionGrant, error) {
	var out PermissionGrant
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/permission-grants/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeletePermissionGrant(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/customer/permission-grants/"+id, nil, nil)
}

// ListPermissionGrants returns every fine-grained grant in the tenant —
// the server's list endpoint is tenant-wide with no filter, matching
// every other list shape in this client.
func (c *Client) ListPermissionGrants(ctx context.Context) ([]PermissionGrant, error) {
	var out []PermissionGrant
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/permission-grants", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}
