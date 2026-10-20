package client

import (
	"context"
	"fmt"
	"net/http"
)

// LogAlertRule mirrors `routes::customer_api::LogAlertRuleInput`/
// `CustomerLogAlertRule` on the wire — one struct for both directions,
// same convention as AlertRule. Unlike a metric alert rule, a log alert
// rule has no nested conditions sub-table: every filter field lives
// directly on the row.
type LogAlertRule struct {
	ID                 string  `json:"id,omitempty"`
	GroupID            string  `json:"group_id"`
	Name               string  `json:"name"`
	MessagePattern     *string `json:"message_pattern,omitempty"`
	LogSeverity        *string `json:"log_severity,omitempty"`
	LogSource          *string `json:"log_source,omitempty"`
	UnitPattern        *string `json:"unit_pattern,omitempty"`
	ThresholdCount     int64   `json:"threshold_count"`
	WindowMinutes      int64   `json:"window_minutes"`
	Severity           string  `json:"severity"`
	Enabled            bool    `json:"enabled"`
	EscalationPolicyID *string `json:"escalation_policy_id,omitempty"`
}

func (c *Client) CreateLogAlertRule(ctx context.Context, rule LogAlertRule) (*LogAlertRule, error) {
	var out LogAlertRule
	if err := c.do(ctx, http.MethodPost, "/api/v1/customer/log-alert-rules", rule, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetLogAlertRule(ctx context.Context, id string) (*LogAlertRule, error) {
	var out LogAlertRule
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/log-alert-rules/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateLogAlertRule(ctx context.Context, id string, rule LogAlertRule) (*LogAlertRule, error) {
	var out LogAlertRule
	path := fmt.Sprintf("/api/v1/customer/log-alert-rules/%s", id)
	if err := c.do(ctx, http.MethodPut, path, rule, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteLogAlertRule(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/customer/log-alert-rules/"+id, nil, nil)
}
