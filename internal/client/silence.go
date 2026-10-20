package client

import (
	"context"
	"net/http"
)

// Silence mirrors `routes::customer_api::SilenceInput`/
// `CustomerSilence` — one struct for both directions. There is no
// update endpoint at all (see `resource_silence.go`'s own doc comment)
// — only create/read/cancel (`DeleteSilence`, a `DELETE` that maps to
// the server's own "cancel" semantics — `cancelled_at = now()`, not a
// row delete).
type Silence struct {
	ID          string  `json:"id,omitempty"`
	Name        string  `json:"name"`
	GroupID     *string `json:"group_id,omitempty"`
	DeviceID    *string `json:"device_id,omitempty"`
	AlertRuleID *string `json:"alert_rule_id,omitempty"`
	Tag         *string `json:"tag,omitempty"`
	StartsAt    string  `json:"starts_at"`
	EndsAt      string  `json:"ends_at"`
	CancelledAt *string `json:"cancelled_at,omitempty"`
	Reason      *string `json:"reason,omitempty"`
}

func (c *Client) CreateSilence(ctx context.Context, s Silence) (*Silence, error) {
	var out Silence
	if err := c.do(ctx, http.MethodPost, "/api/v1/customer/silences", s, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetSilence(ctx context.Context, id string) (*Silence, error) {
	var out Silence
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/silences/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteSilence cancels the silence (`cancelled_at = now()`) — the
// server never deletes the row, keeping its history queryable/
// auditable. Idempotent: cancelling an already-cancelled silence still
// succeeds.
func (c *Client) DeleteSilence(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/customer/silences/"+id, nil, nil)
}
