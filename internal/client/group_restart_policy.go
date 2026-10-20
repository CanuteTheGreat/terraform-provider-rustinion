package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// GroupRestartPolicy is a group's restart policy as stored by the server
// (`rustinion_manifest::restart::RestartSettings`): every key is optional
// and an absent key means the built-in default. It is passed through as raw
// JSON so new policy keys (windows, rings, drain/verify hooks, ...) never
// need a provider release.
type GroupRestartPolicy struct {
	GroupID       string          `json:"group_id"`
	RestartPolicy json.RawMessage `json:"restart_policy"`
	// Why the group's restarts are paused (a device that didn't come back or
	// isn't healthy after a coordinated restart); empty = running.
	Paused *string `json:"paused"`
}

func groupRestartPolicyPath(groupID string) string {
	return fmt.Sprintf("/api/v1/customer/groups/%s/restart-policy", groupID)
}

func (c *Client) GetGroupRestartPolicy(ctx context.Context, groupID string) (*GroupRestartPolicy, error) {
	var out GroupRestartPolicy
	if err := c.do(ctx, http.MethodGet, groupRestartPolicyPath(groupID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PutGroupRestartPolicy replaces the group's whole restart policy with
// `policy` (a JSON object).
func (c *Client) PutGroupRestartPolicy(ctx context.Context, groupID string, policy json.RawMessage) (*GroupRestartPolicy, error) {
	var out GroupRestartPolicy
	if err := c.do(ctx, http.MethodPut, groupRestartPolicyPath(groupID), policy, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
