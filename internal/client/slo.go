package client

import (
	"context"
	"net/http"
)

// Slo mirrors `routes::customer_api::SloInput`/`CustomerSlo` — one
// struct for both directions, same convention as every other resource
// in this package. There is no update endpoint at all (see
// `resource_slo.go`'s own doc comment) — only create/read/delete.
type Slo struct {
	ID               string   `json:"id,omitempty"`
	GroupID          string   `json:"group_id"`
	Name             string   `json:"name"`
	MetricType       string   `json:"metric_type"`
	TargetPercentage float64  `json:"target_percentage"`
	WindowDays       int64    `json:"window_days,omitempty"`
	FreshnessMinutes int64    `json:"freshness_minutes,omitempty"`
	CustomMetricName *string  `json:"custom_metric_name,omitempty"`
	Comparator       *string  `json:"comparator,omitempty"`
	GoodThreshold    *float64 `json:"good_threshold,omitempty"`
	SliAlertRuleID   *string  `json:"sli_alert_rule_id,omitempty"`
	Enabled          bool     `json:"enabled"`
}

func (c *Client) CreateSlo(ctx context.Context, s Slo) (*Slo, error) {
	var out Slo
	if err := c.do(ctx, http.MethodPost, "/api/v1/customer/slos", s, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetSlo(ctx context.Context, id string) (*Slo, error) {
	var out Slo
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/slos/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteSlo(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/customer/slos/"+id, nil, nil)
}
