package client

import (
	"context"
	"fmt"
	"net/http"
)

// GroupFileIntegrity mirrors `GET/PUT /api/v1/customer/groups/{id}/file-integrity`
// (file integrity monitoring policy of a group).
type GroupFileIntegrity struct {
	Enabled      bool     `json:"enabled"`
	UseDefaults  bool     `json:"use_defaults"`
	Paths        []string `json:"paths"`
	Exclusions   []string `json:"exclusions"`
	ScanMinutes  int64    `json:"scan_minutes"`
	Realtime     bool     `json:"realtime"`
	CriticalPush bool     `json:"critical_push"`
}

// DefaultGroupFileIntegrity: off, built-in list, 30 minute scans.
func DefaultGroupFileIntegrity() GroupFileIntegrity {
	return GroupFileIntegrity{Enabled: false, UseDefaults: true, Paths: []string{}, Exclusions: []string{}, ScanMinutes: 30, Realtime: true}
}

func groupFileIntegrityPath(groupID string) string {
	return fmt.Sprintf("/api/v1/customer/groups/%s/file-integrity", groupID)
}

func (c *Client) GetGroupFileIntegrity(ctx context.Context, groupID string) (*GroupFileIntegrity, error) {
	var out GroupFileIntegrity
	if err := c.do(ctx, http.MethodGet, groupFileIntegrityPath(groupID), nil, &out); err != nil {
		return nil, err
	}
	return normalizeFim(&out), nil
}

func (c *Client) PutGroupFileIntegrity(ctx context.Context, groupID string, p GroupFileIntegrity) (*GroupFileIntegrity, error) {
	var out GroupFileIntegrity
	if err := c.do(ctx, http.MethodPut, groupFileIntegrityPath(groupID), p, &out); err != nil {
		return nil, err
	}
	return normalizeFim(&out), nil
}

func normalizeFim(p *GroupFileIntegrity) *GroupFileIntegrity {
	if p.Paths == nil {
		p.Paths = []string{}
	}
	if p.Exclusions == nil {
		p.Exclusions = []string{}
	}
	return p
}
