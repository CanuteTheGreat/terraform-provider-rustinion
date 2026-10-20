package client

import (
	"context"
	"net/http"
)

// RetentionPolicy mirrors `retention::RetentionPolicy`. A tenant
// singleton: there is exactly one per tenant, it always exists, and
// every field is nullable — a nil pointer means "use the platform-wide
// default for this tier" rather than "leave unchanged", because PUT is
// a full replace.
//
// Every field is a *int64 rather than an int64 so that "unset" stays
// distinguishable from a literal 0 on the wire; the server treats a
// non-positive value as unset too, but sending 0 where the user meant
// "default" would be relying on that rather than saying it.
type RetentionPolicy struct {
	CustomMetricRawRetentionDays      *int64 `json:"custom_metric_raw_retention_days"`
	CustomMetricMinutelyRetentionDays *int64 `json:"custom_metric_minutely_retention_days"`
	CustomMetricHourlyRetentionDays   *int64 `json:"custom_metric_hourly_retention_days"`
	DeviceMetricsRawRetentionDays     *int64 `json:"device_metrics_raw_retention_days"`
	DeviceMetricsHourlyRetentionDays  *int64 `json:"device_metrics_hourly_retention_days"`
	DeviceLogRetentionDays            *int64 `json:"device_log_retention_days"`
}

func (c *Client) GetRetentionPolicy(ctx context.Context) (*RetentionPolicy, error) {
	var out RetentionPolicy
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/retention-policy", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) PutRetentionPolicy(ctx context.Context, p RetentionPolicy) (*RetentionPolicy, error) {
	var out RetentionPolicy
	if err := c.do(ctx, http.MethodPut, "/api/v1/customer/retention-policy", p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
