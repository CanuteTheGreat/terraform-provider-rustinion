package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// Integration mirrors `routes::customer_api::InstallIntegrationInput`/
// `ConfigureIntegrationInput`/`CustomerIntegration` on the wire — one
// struct for every direction, same convention as the other resources.
// `Kind`/`GroupID` are fixed at install time (the server only ever
// updates `Config` on a `PUT`, see `resource_integration.go`'s own doc
// comment on why both are `RequiresReplace`); `Config`'s shape depends
// on `Kind` (see `integrations.rs`'s own module doc comment for what
// each kind expects), so it's left as raw JSON for the same reason
// `NotificationChannel.Config` is. `DashboardID`/`AlertRuleID`/
// `SyntheticCheckID`/`NotificationChannelID` are server-assigned,
// read-only pointers to whatever real resources `install` created for
// this integration's own `kind` (never all four at once — see that
// module's doc comment for which kind creates which).
type Integration struct {
	ID                    string          `json:"id,omitempty"`
	Kind                  string          `json:"kind"`
	GroupID               string          `json:"group_id"`
	Config                json.RawMessage `json:"config"`
	DashboardID           *string         `json:"dashboard_id,omitempty"`
	AlertRuleID           *string         `json:"alert_rule_id,omitempty"`
	SyntheticCheckID      *string         `json:"synthetic_check_id,omitempty"`
	NotificationChannelID *string         `json:"notification_channel_id,omitempty"`
}

func (c *Client) CreateIntegration(ctx context.Context, i Integration) (*Integration, error) {
	var out Integration
	if err := c.do(ctx, http.MethodPost, "/api/v1/customer/integrations", i, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetIntegration(ctx context.Context, id string) (*Integration, error) {
	var out Integration
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/integrations/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateIntegration only ever pushes `Config` — the server's own
// `ConfigureIntegrationInput` reads no other field, matching
// `resource_integration.go`'s "kind/group_id are RequiresReplace"
// schema.
func (c *Client) UpdateIntegration(ctx context.Context, id string, i Integration) (*Integration, error) {
	var out Integration
	path := fmt.Sprintf("/api/v1/customer/integrations/%s", id)
	if err := c.do(ctx, http.MethodPut, path, i, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteIntegration(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/customer/integrations/"+id, nil, nil)
}
