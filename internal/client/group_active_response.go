package client

import (
	"context"
	"fmt"
	"net/http"
)

// GroupActiveResponse mirrors `GET/PUT /api/v1/customer/groups/{id}/active-response`.
type GroupActiveResponse struct {
	Enabled        bool     `json:"enabled"`
	DryRun         bool     `json:"dry_run"`
	MinLevel       string   `json:"min_level"`
	BlockIP        bool     `json:"block_ip"`
	BlockMinutes   int64    `json:"block_minutes"`
	KillProcess    bool     `json:"kill_process"`
	DisableUser    bool     `json:"disable_user"`
	DisableHours   int64    `json:"disable_hours"`
	Allowlist      []string `json:"allowlist"`
	BlockPrivate   bool     `json:"block_private"`
	ProtectedUsers []string `json:"protected_users"`
}

// DefaultGroupActiveResponse: off, dry run, high and above, block for an hour.
func DefaultGroupActiveResponse() GroupActiveResponse {
	return GroupActiveResponse{
		Enabled: false, DryRun: true, MinLevel: "high", BlockIP: true, BlockMinutes: 60,
		DisableHours: 24, Allowlist: []string{}, ProtectedUsers: []string{},
	}
}

func groupActiveResponsePath(groupID string) string {
	return fmt.Sprintf("/api/v1/customer/groups/%s/active-response", groupID)
}

func normalizeActiveResponse(p *GroupActiveResponse) *GroupActiveResponse {
	if p.Allowlist == nil {
		p.Allowlist = []string{}
	}
	if p.ProtectedUsers == nil {
		p.ProtectedUsers = []string{}
	}
	return p
}

func (c *Client) GetGroupActiveResponse(ctx context.Context, groupID string) (*GroupActiveResponse, error) {
	var out GroupActiveResponse
	if err := c.do(ctx, http.MethodGet, groupActiveResponsePath(groupID), nil, &out); err != nil {
		return nil, err
	}
	return normalizeActiveResponse(&out), nil
}

func (c *Client) PutGroupActiveResponse(ctx context.Context, groupID string, p GroupActiveResponse) (*GroupActiveResponse, error) {
	normalizeActiveResponse(&p)
	var out GroupActiveResponse
	if err := c.do(ctx, http.MethodPut, groupActiveResponsePath(groupID), p, &out); err != nil {
		return nil, err
	}
	return normalizeActiveResponse(&out), nil
}
