package client

import (
	"context"
	"fmt"
	"net/http"
)

// DeviceDisplayName mirrors `GET/PUT /api/v1/customer/devices/{id}/display-name`:
// the admin-set name shown instead of the agent-reported hostname in lists,
// alerts and digests. DisplayName nil = the hostname is used. Name is the
// resulting label (display name, plus the hostname in brackets unless it
// already contains it).
type DeviceDisplayName struct {
	ID          string  `json:"id,omitempty"`
	Hostname    *string `json:"hostname,omitempty"`
	DisplayName *string `json:"display_name"`
	Name        *string `json:"name,omitempty"`
}

func deviceDisplayNamePath(deviceID string) string {
	return fmt.Sprintf("/api/v1/customer/devices/%s/display-name", deviceID)
}

func (c *Client) GetDeviceDisplayName(ctx context.Context, deviceID string) (*DeviceDisplayName, error) {
	var out DeviceDisplayName
	if err := c.do(ctx, http.MethodGet, deviceDisplayNamePath(deviceID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PutDeviceDisplayName sets the display name; nil clears it.
func (c *Client) PutDeviceDisplayName(ctx context.Context, deviceID string, displayName *string) (*DeviceDisplayName, error) {
	var out DeviceDisplayName
	body := map[string]any{"display_name": displayName}
	if err := c.do(ctx, http.MethodPut, deviceDisplayNamePath(deviceID), body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
