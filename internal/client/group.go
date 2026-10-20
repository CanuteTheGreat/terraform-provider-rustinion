package client

import (
	"context"
	"fmt"
	"net/http"
)

// Group mirrors `routes::customer_api::GroupInput`/`CustomerGroup` —
// with one deliberate exception: `TemplateKey` is accepted on
// create/`ApplyGroupTemplate` but the server never returns it in any
// response (a group's manifest doesn't remember which template
// produced it), so it always decodes as empty on a `GetGroup`/
// `CreateGroup` response regardless of what was last applied. Callers
// that need to preserve it (see `resource_group.go`'s own doc comment)
// must track it themselves rather than trusting a round-trip through
// this struct.
type Group struct {
	ID              string `json:"id,omitempty"`
	Name            string `json:"name"`
	TemplateKey     string `json:"template_key,omitempty"`
	ManifestVersion int64  `json:"manifest_version,omitempty"`
}

type applyGroupTemplateRequest struct {
	TemplateKey string `json:"template_key"`
}

func (c *Client) CreateGroup(ctx context.Context, g Group) (*Group, error) {
	var out Group
	if err := c.do(ctx, http.MethodPost, "/api/v1/customer/groups", g, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetGroup(ctx context.Context, id string) (*Group, error) {
	var out Group
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/groups/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ApplyGroupTemplate is the only update this API offers for a group —
// it (re-)seeds the manifest/OS-update policy/compliance policy from a
// named starter template; it never touches `Name`. See
// `resource_group.go`'s own doc comment for why a group has no other
// update path at all.
func (c *Client) ApplyGroupTemplate(ctx context.Context, id, templateKey string) (*Group, error) {
	var out Group
	path := fmt.Sprintf("/api/v1/customer/groups/%s", id)
	if err := c.do(ctx, http.MethodPut, path, applyGroupTemplateRequest{TemplateKey: templateKey}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteGroup(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/customer/groups/"+id, nil, nil)
}
