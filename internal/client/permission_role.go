package client

import (
	"context"
	"net/http"
)

// RolePermission is one line of a role's definition: what level it
// confers on one resource type. At most one per resource type per role,
// enforced server-side.
type RolePermission struct {
	// ResourceType is one of "alert_rules", "dashboards",
	// "synthetic_checks", "notification_channels".
	ResourceType string `json:"resource_type"`
	// PermissionLevel is one of "read", "write", "admin".
	PermissionLevel string `json:"permission_level"`
}

// PermissionRole mirrors `routes::customer_api::PermissionRoleInput`/
// `CustomerPermissionRole` on the wire — one struct for both
// directions, same convention as the other resources in this client.
//
// Permissions are a nested array on the role itself rather than a
// sub-resource, and PUT replaces the whole set — matching the server's
// own "PUT is a full resource replace" convention and the nested-array
// shape AlertRule.Conditions and Dashboard.Widgets already use. An
// empty array is a real value meaning "permits nothing", never a
// "leave unchanged" signal.
//
// Description is a *string because the server genuinely distinguishes
// a null description from an empty one.
type PermissionRole struct {
	ID          string           `json:"id,omitempty"`
	Name        string           `json:"name"`
	Description *string          `json:"description"`
	Permissions []RolePermission `json:"permissions"`
}

func (c *Client) CreatePermissionRole(ctx context.Context, role PermissionRole) (*PermissionRole, error) {
	var out PermissionRole
	if err := c.do(ctx, http.MethodPost, "/api/v1/customer/permission-roles", role, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetPermissionRole(ctx context.Context, id string) (*PermissionRole, error) {
	var out PermissionRole
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/permission-roles/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdatePermissionRole(ctx context.Context, id string, role PermissionRole) (*PermissionRole, error) {
	var out PermissionRole
	if err := c.do(ctx, http.MethodPut, "/api/v1/customer/permission-roles/"+id, role, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeletePermissionRole(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/customer/permission-roles/"+id, nil, nil)
}

func (c *Client) ListPermissionRoles(ctx context.Context) ([]PermissionRole, error) {
	var out []PermissionRole
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/permission-roles", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// RoleAssignment is who holds a role, over what scope. A genuine
// sub-resource of a role rather than a nested array, mirroring
// NotificationRoute under NotificationChannel: who holds a role is
// independent of what the role permits and changes on a different
// cadence, so adding a person must not diff the role definition.
//
// GroupID nil = tenant-wide (every group).
type RoleAssignment struct {
	ID      string  `json:"id,omitempty"`
	RoleID  string  `json:"role_id,omitempty"`
	UserID  string  `json:"user_id"`
	GroupID *string `json:"group_id"`
}

// CreateRoleAssignment assigns a role. Assigning the same user and
// scope twice is idempotent server-side, returning the existing
// assignment rather than duplicating.
func (c *Client) CreateRoleAssignment(ctx context.Context, roleID string, assignment RoleAssignment) (*RoleAssignment, error) {
	var out RoleAssignment
	if err := c.do(ctx, http.MethodPost, "/api/v1/customer/permission-roles/"+roleID+"/assignments", assignment, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListRoleAssignments returns every holder of one role. The server has
// no per-assignment GET, so a caller wanting one (this provider's own
// Read) filters this result by id — the same shape
// GetNotificationRoute uses for the same reason.
func (c *Client) ListRoleAssignments(ctx context.Context, roleID string) ([]RoleAssignment, error) {
	var out []RoleAssignment
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/permission-roles/"+roleID+"/assignments", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetRoleAssignment finds one assignment by id. The server has no
// per-assignment GET endpoint, so this filters the role's own list —
// the same shape GetNotificationRoute uses for the same reason.
func (c *Client) GetRoleAssignment(ctx context.Context, roleID, assignmentID string) (*RoleAssignment, error) {
	assignments, err := c.ListRoleAssignments(ctx, roleID)
	if err != nil {
		return nil, err
	}
	for _, a := range assignments {
		if a.ID == assignmentID {
			return &a, nil
		}
	}
	return nil, &APIError{StatusCode: http.StatusNotFound, Message: "no such role assignment"}
}

func (c *Client) DeleteRoleAssignment(ctx context.Context, roleID, assignmentID string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/customer/permission-roles/"+roleID+"/assignments/"+assignmentID, nil, nil)
}
