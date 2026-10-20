package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// NotificationChannel mirrors `routes::customer_api::
// NotificationChannelInput`/`notifications::NotificationChannel` on the
// wire. `Config` is left as `json.RawMessage` rather than a Go struct —
// its shape depends entirely on `ChannelType` (Slack/webhook/email/
// PagerDuty/Opsgenie each expect different fields, validated server-side
// by `notifications::validate_channel_config`), and duplicating that
// five-way shape here would be a second copy of validation this
// provider has no business re-implementing; it just passes the caller's
// JSON straight through.
type NotificationChannel struct {
	ID                    string          `json:"id,omitempty"`
	Name                  string          `json:"name"`
	ChannelType           string          `json:"channel_type"`
	Config                json.RawMessage `json:"config"`
	Enabled               bool            `json:"enabled"`
	GroupingEnabled       bool            `json:"grouping_enabled"`
	GroupingDimension     string          `json:"grouping_dimension"`
	GroupingWindowMinutes int64           `json:"grouping_window_minutes"`
	DigestIntervalMinutes int64           `json:"digest_interval_minutes"`
	// Custom notification content. Pointers, not plain strings,
	// because the server distinguishes "no template, use the built-in
	// format" (null) from a template that happens to be empty — and
	// omitempty on a plain string would make those indistinguishable
	// on the wire.
	SubjectTemplate *string `json:"subject_template"`
	BodyTemplate    *string `json:"body_template"`
	// Webhook channels only: the entire JSON request body.
	PayloadTemplate *string `json:"payload_template"`
	// Finding severities this channel gets (critical, warning, info). nil
	// (JSON null) = the channel type's default: pagerduty, opsgenie, sms
	// and voice get critical only, the others every severity.
	Severities []string `json:"severities"`
}

func (c *Client) CreateNotificationChannel(ctx context.Context, ch NotificationChannel) (*NotificationChannel, error) {
	var out NotificationChannel
	if err := c.do(ctx, http.MethodPost, "/api/v1/customer/notification-channels", ch, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetNotificationChannel(ctx context.Context, id string) (*NotificationChannel, error) {
	var out NotificationChannel
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/notification-channels/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateNotificationChannel(ctx context.Context, id string, ch NotificationChannel) (*NotificationChannel, error) {
	var out NotificationChannel
	path := fmt.Sprintf("/api/v1/customer/notification-channels/%s", id)
	if err := c.do(ctx, http.MethodPut, path, ch, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteNotificationChannel(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/customer/notification-channels/"+id, nil, nil)
}
