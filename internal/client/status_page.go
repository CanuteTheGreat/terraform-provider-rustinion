package client

import (
	"context"
	"net/http"
)

// StatusPageComponent mirrors `routes::customer_api::
// StatusPageComponentInput`/`CustomerStatusPageComponent`. The response
// additionally carries a server-assigned `position`, taken from the
// component's index in the parent page's own `Components` array on
// every `PUT` (a full replace, not a diff — same convention as an alert
// rule's conditions or a dashboard's widgets) — this provider doesn't
// track it here since it's fully implied by list order, same choice
// `client.DashboardWidget` already makes for the same reason.
type StatusPageComponent struct {
	ID               string `json:"id,omitempty"`
	SyntheticCheckID string `json:"synthetic_check_id"`
	DisplayName      string `json:"display_name"`
}

// StatusPage mirrors `routes::customer_api::StatusPageInput`/
// `CustomerStatusPage`. Unlike every other resource in this package,
// there is exactly one status page per tenant
// (`status_pages.tenant_id` is unique) — see `resource_status_page.go`'s
// own doc comment for how that singleton shape is modeled in Terraform.
type StatusPage struct {
	ID         string                `json:"id,omitempty"`
	Slug       string                `json:"slug"`
	Title      string                `json:"title"`
	Components []StatusPageComponent `json:"components"`
}

func (c *Client) GetStatusPage(ctx context.Context) (*StatusPage, error) {
	var out StatusPage
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/status-page", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) PutStatusPage(ctx context.Context, page StatusPage) (*StatusPage, error) {
	var out StatusPage
	if err := c.do(ctx, http.MethodPut, "/api/v1/customer/status-page", page, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteStatusPage(ctx context.Context) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/customer/status-page", nil, nil)
}
