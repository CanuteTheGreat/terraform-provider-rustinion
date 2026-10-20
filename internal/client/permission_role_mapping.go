package client

import (
	"context"
	"net/http"
)

// PermissionRoleMapping maps an identity-provider group name onto a
// permission role, so directory membership decides who holds it.
//
// Source is "scim" (applied when the IdP pushes a group membership
// change), "oidc" (applied at each sign-in, from the ID token's groups
// claim) or "saml" (applied at each sign-in, from the assertion's
// group attribute). A tenant may use any combination, with different
// group naming in each, which is why source is part of a mapping's
// identity.
//
// There is no update method: a mapping's identity IS its (Source,
// ExternalGroup, RoleID, GroupID) tuple — exactly what the server's two
// partial unique indexes enforce — so it has no mutable field at all.
//
// GroupID nil = the resulting assignment is tenant-wide.
type PermissionRoleMapping struct {
	ID            string  `json:"id,omitempty"`
	Source        string  `json:"source"`
	ExternalGroup string  `json:"external_group"`
	RoleID        string  `json:"role_id"`
	GroupID       *string `json:"group_id"`
}

func (c *Client) CreatePermissionRoleMapping(ctx context.Context, mapping PermissionRoleMapping) (*PermissionRoleMapping, error) {
	var out PermissionRoleMapping
	if err := c.do(ctx, http.MethodPost, "/api/v1/customer/permission-role-mappings", mapping, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetPermissionRoleMapping(ctx context.Context, id string) (*PermissionRoleMapping, error) {
	var out PermissionRoleMapping
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/permission-role-mappings/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeletePermissionRoleMapping(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/customer/permission-role-mappings/"+id, nil, nil)
}

func (c *Client) ListPermissionRoleMappings(ctx context.Context) ([]PermissionRoleMapping, error) {
	var out []PermissionRoleMapping
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/permission-role-mappings", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}
