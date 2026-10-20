package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// DashboardWidget mirrors `routes::customer_api::DashboardWidgetInput`/
// `dashboards::Widget`. `Config`'s shape depends on `WidgetType`
// (metric_chart/kpi_tile/alert_rule_status/log_search — validated
// server-side by `dashboards::validate_widget_config`), so it's left as
// raw JSON here for the same reason `NotificationChannel.Config` is —
// see that type's own doc comment.
type DashboardWidget struct {
	ID         string          `json:"id,omitempty"`
	Title      string          `json:"title"`
	WidgetType string          `json:"widget_type"`
	Config     json.RawMessage `json:"config"`
	// Width is "full" (the default), "half" or "third" — how much of a
	// row the widget occupies on the rendered dashboard. Empty is sent
	// as omitted, which the server reads as "full".
	Width string `json:"width,omitempty"`
}

// Dashboard mirrors `routes::customer_api::DashboardInput`/
// `CustomerDashboard`. `Widgets` order is significant — the server
// assigns each widget's `position` from its index in this array on
// every write (a full replace, not a diff — see `routes::customer_api`'s
// own "config-as-code: dashboards" section doc comment), so reordering
// this list in Terraform config and re-applying genuinely reorders the
// dashboard.
type Dashboard struct {
	ID      string `json:"id,omitempty"`
	GroupID string `json:"group_id"`
	Name    string `json:"name"`
	// DefaultTimeRangeDays is the dashboard-wide time range applied to
	// the widgets that opted into it via their config's
	// `use_dashboard_time_range`. A nil pointer (omitted from the
	// request, absent from the response) means each widget keeps its
	// own configured range — a pointer rather than a plain int so that
	// "unset" is distinguishable from a hypothetical 0.
	DefaultTimeRangeDays *int64            `json:"default_time_range_days,omitempty"`
	Widgets              []DashboardWidget `json:"widgets"`
}

func (c *Client) CreateDashboard(ctx context.Context, d Dashboard) (*Dashboard, error) {
	var out Dashboard
	if err := c.do(ctx, http.MethodPost, "/api/v1/customer/dashboards", d, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetDashboard(ctx context.Context, id string) (*Dashboard, error) {
	var out Dashboard
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/dashboards/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateDashboard(ctx context.Context, id string, d Dashboard) (*Dashboard, error) {
	var out Dashboard
	path := fmt.Sprintf("/api/v1/customer/dashboards/%s", id)
	if err := c.do(ctx, http.MethodPut, path, d, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteDashboard(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/customer/dashboards/"+id, nil, nil)
}
