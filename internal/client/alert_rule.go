package client

import (
	"context"
	"fmt"
	"net/http"
)

// AlertRuleCondition mirrors `routes::customer_api::
// AlertRuleConditionInput`/`CustomerAlertRuleCondition` on the wire —
// one struct for both directions, since every field round-trips
// losslessly (the server never adds a field on read that wasn't valid
// to send, other than the condition's own server-assigned `id`, which
// this provider never sets on write and never depends on for identity —
// see `resource_alert_rule.go`'s own doc comment on why conditions are
// a plain nested list, not a resource of their own).
type AlertRuleCondition struct {
	ID                    string  `json:"id,omitempty"`
	MetricType            string  `json:"metric_type"`
	ConditionKind         string  `json:"condition_kind"`
	Comparator            string  `json:"comparator"`
	Threshold             float64 `json:"threshold"`
	WindowMinutes         int64   `json:"window_minutes"`
	AnomalyMethod         *string `json:"anomaly_method,omitempty"`
	BaselineWindowMinutes *int64  `json:"baseline_window_minutes,omitempty"`
	// ForecastWithinHours is required for condition_kind = "forecast":
	// how soon (in hours) the metric's current linear trend must be
	// projected to cross Threshold for this condition to breach. See
	// alerting::hours_until_threshold_crossing in the main repo.
	ForecastWithinHours *int64 `json:"forecast_within_hours,omitempty"`
	// Aggregation is avg (the server's own default)/sum/min/max, or a
	// percentile (p50/p90/p95/p99) — see custom_metrics::Aggregation's
	// own doc comment in the main repo. Only meaningful for a
	// `custom:<name>` MetricType; sent unconditionally since the
	// resource schema always resolves it to a concrete value (Optional
	// + Computed + a default of "avg") before a request is built.
	Aggregation string `json:"aggregation"`
	// TagKey/TagValue narrow this condition to samples carrying this
	// exact tag key=value pair — both-or-neither, enforced server-side
	// (see custom_metrics::tags_are_valid / routes::customer_api's own
	// validate_alert_rule_input in the main repo). Only meaningful for a
	// `custom:<name>` MetricType. Omitted from the request entirely when
	// absent (not sent as an explicit null), same as AnomalyMethod above.
	TagKey   *string `json:"tag_key,omitempty"`
	TagValue *string `json:"tag_value,omitempty"`
	// CompositeRuleID/CompositeExpectFiring are required together for
	// condition_kind = "composite" — the other, same-tenant rule whose
	// current firing state this condition reads instead of a metric
	// (see alerting::MetricRef::Composite in the main repo). The server
	// enforces the same-tenant boundary itself (resolve_condition in
	// routes::customer_api); this client never re-validates it.
	CompositeRuleID       *string `json:"composite_rule_id,omitempty"`
	CompositeExpectFiring *bool   `json:"composite_expect_firing,omitempty"`
	// MwLongWindowMinutes/MwLongThreshold are required together for
	// condition_kind = "slo_burn_rate_multi_window" — the LONG window's
	// own lookback (minutes) and burn-rate threshold, paired with
	// WindowMinutes/Threshold above (reused as the SHORT window's own
	// parameters). See alerting::ConditionOutcome::
	// evaluate_slo_multi_window_burn_rate in the main repo.
	MwLongWindowMinutes *int64   `json:"mw_long_window_minutes,omitempty"`
	MwLongThreshold     *float64 `json:"mw_long_threshold,omitempty"`
}

// AlertRule mirrors `routes::customer_api::AlertRuleInput` (for
// Create/Update request bodies) and `CustomerAlertRule` (for every
// response) — a single struct for both, since the request/response
// shapes only differ by `ID` being empty on the way in.
type AlertRule struct {
	ID                 string               `json:"id,omitempty"`
	GroupID            string               `json:"group_id"`
	Name               string               `json:"name"`
	BoolOp             string               `json:"bool_op"`
	Severity           string               `json:"severity"`
	BreachesToFire     int64                `json:"breaches_to_fire"`
	ClearsToResolve    int64                `json:"clears_to_resolve"`
	Enabled            bool                 `json:"enabled"`
	EscalationPolicyID *string              `json:"escalation_policy_id,omitempty"`
	Conditions         []AlertRuleCondition `json:"conditions"`
}

func (c *Client) CreateAlertRule(ctx context.Context, rule AlertRule) (*AlertRule, error) {
	var out AlertRule
	if err := c.do(ctx, http.MethodPost, "/api/v1/customer/alert-rules", rule, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetAlertRule(ctx context.Context, id string) (*AlertRule, error) {
	var out AlertRule
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/alert-rules/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateAlertRule(ctx context.Context, id string, rule AlertRule) (*AlertRule, error) {
	var out AlertRule
	path := fmt.Sprintf("/api/v1/customer/alert-rules/%s", id)
	if err := c.do(ctx, http.MethodPut, path, rule, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteAlertRule(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/customer/alert-rules/"+id, nil, nil)
}
