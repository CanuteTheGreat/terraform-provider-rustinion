package client

import (
	"context"
	"fmt"
	"net/http"
)

// EscalationStep mirrors `routes::customer_api::EscalationStepInput`/
// `CustomerEscalationStep` on the wire — one struct for both
// directions, same convention as `AlertRuleCondition`. `step_number` is
// deliberately not tracked here: the server assigns it from this
// step's index in the parent policy's own array on every write (a full
// replace, not a diff), the same "order is the position" convention
// `DashboardWidget` already uses for the same reason.
//
// ChannelIDs is a list, not a single value — a step fans out to every
// channel in it independently (see escalation.rs's own notify_step doc
// comment in the main repo), backed by escalation_step_channels since
// migration 0077_escalation_step_fanout.sql. Order is preserved
// (position column) but has no delivery-timing significance; it's kept
// stable purely so a Terraform-managed list doesn't show spurious diffs.
type EscalationStep struct {
	ID          string   `json:"id,omitempty"`
	ChannelIDs  []string `json:"channel_ids"`
	WaitMinutes int64    `json:"wait_minutes"`
}

// EscalationPolicy mirrors `routes::customer_api::
// EscalationPolicyInput`/`CustomerEscalationPolicy`. `OnCallScheduleID`
// is purely for message enrichment server-side — escalation timing
// never depends on it — but it must still belong to the same group as
// this policy, enforced by the server on every write.
type EscalationPolicy struct {
	ID               string           `json:"id,omitempty"`
	GroupID          string           `json:"group_id"`
	Name             string           `json:"name"`
	OnCallScheduleID *string          `json:"on_call_schedule_id,omitempty"`
	Steps            []EscalationStep `json:"steps"`
}

func (c *Client) CreateEscalationPolicy(ctx context.Context, p EscalationPolicy) (*EscalationPolicy, error) {
	var out EscalationPolicy
	if err := c.do(ctx, http.MethodPost, "/api/v1/customer/escalation-policies", p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetEscalationPolicy(ctx context.Context, id string) (*EscalationPolicy, error) {
	var out EscalationPolicy
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/escalation-policies/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateEscalationPolicy(ctx context.Context, id string, p EscalationPolicy) (*EscalationPolicy, error) {
	var out EscalationPolicy
	path := fmt.Sprintf("/api/v1/customer/escalation-policies/%s", id)
	if err := c.do(ctx, http.MethodPut, path, p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteEscalationPolicy(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/customer/escalation-policies/"+id, nil, nil)
}
