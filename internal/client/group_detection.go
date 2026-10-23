package client

import (
	"context"
	"fmt"
	"net/http"
	"sort"
)

// DetectionRuleTuning is one rule's per-group tuning.
type DetectionRuleTuning struct {
	Enabled   *bool   `json:"enabled,omitempty"`
	Level     *string `json:"level,omitempty"`
	Threshold *int64  `json:"threshold,omitempty"`
}

// GroupDetection mirrors `GET/PUT /api/v1/customer/groups/{id}/detection`
// (detection rules of a group).
type GroupDetection struct {
	Enabled      bool                           `json:"enabled"`
	CriticalPush bool                           `json:"critical_push"`
	MinLevel     string                         `json:"min_level"`
	Rules        map[string]DetectionRuleTuning `json:"rules"`
}

// DefaultGroupDetection: off, findings from medium up, no tuning.
func DefaultGroupDetection() GroupDetection {
	return GroupDetection{Enabled: false, MinLevel: "medium", Rules: map[string]DetectionRuleTuning{}}
}

func groupDetectionPath(groupID string) string {
	return fmt.Sprintf("/api/v1/customer/groups/%s/detection", groupID)
}

func normalizeDetection(p *GroupDetection) *GroupDetection {
	if p.Rules == nil {
		p.Rules = map[string]DetectionRuleTuning{}
	}
	return p
}

func (c *Client) GetGroupDetection(ctx context.Context, groupID string) (*GroupDetection, error) {
	var out GroupDetection
	if err := c.do(ctx, http.MethodGet, groupDetectionPath(groupID), nil, &out); err != nil {
		return nil, err
	}
	return normalizeDetection(&out), nil
}

func (c *Client) PutGroupDetection(ctx context.Context, groupID string, p GroupDetection) (*GroupDetection, error) {
	normalizeDetection(&p)
	var out GroupDetection
	if err := c.do(ctx, http.MethodPut, groupDetectionPath(groupID), p, &out); err != nil {
		return nil, err
	}
	return normalizeDetection(&out), nil
}

// DisabledRules lists the rules switched off, sorted.
func (p *GroupDetection) DisabledRules() []string {
	out := []string{}
	for id, t := range p.Rules {
		if t.Enabled != nil && !*t.Enabled {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}
