package client

import (
	"context"
	"fmt"
	"net/http"
)

// DeviceAgentUpdate mirrors `GET/PUT /api/v1/customer/devices/{id}/agent-update`:
// one device's own agent self-update setting. Mode is "follow_group" (the
// group's agent-update policy), "auto" (always install new releases),
// "paused" or "pinned" (PinnedVersion, a published agent version: updated up
// to it and kept there, never downgraded). The rest is read-only.
type DeviceAgentUpdate struct {
	Mode          string  `json:"mode"`
	PinnedVersion *string `json:"pinned_version,omitempty"`
	Reason        *string `json:"reason,omitempty"`
	Source        string  `json:"source,omitempty"`
	Explanation   string  `json:"explanation,omitempty"`
	AgentVersion  *string `json:"agent_version,omitempty"`
	LatestRelease *string `json:"latest_release,omitempty"`
}

func deviceAgentUpdatePath(deviceID string) string {
	return fmt.Sprintf("/api/v1/customer/devices/%s/agent-update", deviceID)
}

func (c *Client) GetDeviceAgentUpdate(ctx context.Context, deviceID string) (*DeviceAgentUpdate, error) {
	var out DeviceAgentUpdate
	if err := c.do(ctx, http.MethodGet, deviceAgentUpdatePath(deviceID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PutDeviceAgentUpdate sets the device's setting; pinnedVersion and reason
// may be empty.
func (c *Client) PutDeviceAgentUpdate(ctx context.Context, deviceID, mode, pinnedVersion, reason string) (*DeviceAgentUpdate, error) {
	body := map[string]any{"mode": mode}
	if pinnedVersion != "" {
		body["pinned_version"] = pinnedVersion
	}
	if reason != "" {
		body["reason"] = reason
	}
	var out DeviceAgentUpdate
	if err := c.do(ctx, http.MethodPut, deviceAgentUpdatePath(deviceID), body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
