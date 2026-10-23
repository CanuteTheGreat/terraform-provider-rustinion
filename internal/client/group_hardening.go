package client

import (
	"context"
	"fmt"
	"net/http"
)

// GroupHardening mirrors `GET/PUT /api/v1/customer/groups/{id}/hardening`
// (hardening benchmarks of a group).
type GroupHardening struct {
	Enabled       bool           `json:"enabled"`
	IntervalHours int64          `json:"interval_hours"`
	Policies      []string       `json:"policies"`
	MinScore      *int64         `json:"min_score"`
	AcceptedRisks []AcceptedRisk `json:"accepted_risks"`
}

type AcceptedRisk struct {
	PolicyID string `json:"policy_id"`
	CheckID  string `json:"check_id"`
	Reason   string `json:"reason"`
}

// groupHardeningPut is the PUT body: min_score -1 clears the minimum.
type groupHardeningPut struct {
	Enabled       bool           `json:"enabled"`
	IntervalHours int64          `json:"interval_hours"`
	Policies      []string       `json:"policies"`
	MinScore      int64          `json:"min_score"`
	AcceptedRisks []AcceptedRisk `json:"accepted_risks"`
}

// DefaultGroupHardening: off, every shipped policy, daily, no minimum.
func DefaultGroupHardening() GroupHardening {
	return GroupHardening{Enabled: false, IntervalHours: 24, Policies: []string{}, AcceptedRisks: []AcceptedRisk{}}
}

func groupHardeningPath(groupID string) string {
	return fmt.Sprintf("/api/v1/customer/groups/%s/hardening", groupID)
}

func normalizeHardening(p *GroupHardening) *GroupHardening {
	if p.Policies == nil {
		p.Policies = []string{}
	}
	if p.AcceptedRisks == nil {
		p.AcceptedRisks = []AcceptedRisk{}
	}
	return p
}

func (c *Client) GetGroupHardening(ctx context.Context, groupID string) (*GroupHardening, error) {
	var out GroupHardening
	if err := c.do(ctx, http.MethodGet, groupHardeningPath(groupID), nil, &out); err != nil {
		return nil, err
	}
	return normalizeHardening(&out), nil
}

func (c *Client) PutGroupHardening(ctx context.Context, groupID string, p GroupHardening) (*GroupHardening, error) {
	body := groupHardeningPut{
		Enabled: p.Enabled, IntervalHours: p.IntervalHours, Policies: p.Policies,
		MinScore: -1, AcceptedRisks: p.AcceptedRisks,
	}
	if body.Policies == nil {
		body.Policies = []string{}
	}
	if body.AcceptedRisks == nil {
		body.AcceptedRisks = []AcceptedRisk{}
	}
	if p.MinScore != nil {
		body.MinScore = *p.MinScore
	}
	var out GroupHardening
	if err := c.do(ctx, http.MethodPut, groupHardeningPath(groupID), body, &out); err != nil {
		return nil, err
	}
	return normalizeHardening(&out), nil
}
