package client

import (
	"context"
	"net/http"
)

// LogMetricDefinition mirrors `routes::customer_api::LogMetricDefinitionInput`/
// `custom_metrics::LogMetricDefinitionRow`. Tenant-wide — there is no
// `group_id` on this resource at all (see the Rust side's own
// `0091_log_metric_definitions.sql` doc comment).
type LogMetricDefinition struct {
	ID         string `json:"id,omitempty"`
	Name       string `json:"name"`
	Pattern    string `json:"pattern"`
	Extraction string `json:"extraction"`
	Enabled    bool   `json:"enabled"`
}

func (c *Client) CreateLogMetricDefinition(ctx context.Context, d LogMetricDefinition) (*LogMetricDefinition, error) {
	var out LogMetricDefinition
	if err := c.do(ctx, http.MethodPost, "/api/v1/customer/log-metrics", d, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetLogMetricDefinition(ctx context.Context, id string) (*LogMetricDefinition, error) {
	var out LogMetricDefinition
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/log-metrics/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateLogMetricDefinition(ctx context.Context, id string, d LogMetricDefinition) (*LogMetricDefinition, error) {
	var out LogMetricDefinition
	if err := c.do(ctx, http.MethodPut, "/api/v1/customer/log-metrics/"+id, d, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteLogMetricDefinition(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/customer/log-metrics/"+id, nil, nil)
}
