package client

import (
	"context"
	"net/http"
)

// ProcessWatchRule mirrors `routes::customer_api::ProcessWatchRuleInput`/
// `CustomerProcessWatchRule` on the wire — one struct for both
// directions, same convention as LogAlertRule. Unlike the admin UI's
// own `routes::process_watch::create_rule`/`remove_rule` (add/remove
// only, no update path at all), this API can also update `Enabled`/
// `ProcessName`/`GroupID` via a full replace — a real, additive
// capability, not a mismatch with the admin UI to paper over.
type ProcessWatchRule struct {
	ID          string `json:"id,omitempty"`
	GroupID     string `json:"group_id"`
	ProcessName string `json:"process_name"`
	Enabled     bool   `json:"enabled"`
}

func (c *Client) CreateProcessWatchRule(ctx context.Context, rule ProcessWatchRule) (*ProcessWatchRule, error) {
	var out ProcessWatchRule
	if err := c.do(ctx, http.MethodPost, "/api/v1/customer/process-watch-rules", rule, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetProcessWatchRule(ctx context.Context, id string) (*ProcessWatchRule, error) {
	var out ProcessWatchRule
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/process-watch-rules/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateProcessWatchRule(ctx context.Context, id string, rule ProcessWatchRule) (*ProcessWatchRule, error) {
	var out ProcessWatchRule
	if err := c.do(ctx, http.MethodPut, "/api/v1/customer/process-watch-rules/"+id, rule, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteProcessWatchRule(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/customer/process-watch-rules/"+id, nil, nil)
}

// ListProcessWatchRules returns every process watch rule across every
// group in the tenant — the server's own list endpoint is tenant-wide
// with no group filter (matching `log-alert-rules`'s own list shape
// exactly), so a caller that wants one group's rules (the data source
// below) filters this result client-side.
func (c *Client) ListProcessWatchRules(ctx context.Context) ([]ProcessWatchRule, error) {
	var out []ProcessWatchRule
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/process-watch-rules", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}
