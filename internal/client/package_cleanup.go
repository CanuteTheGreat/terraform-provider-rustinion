package client

import (
	"context"
	"fmt"
	"net/http"
)

// GroupPackageCleanup mirrors `GET/PUT /api/v1/customer/groups/{id}/package-cleanup`:
// whether devices in the group remove packages that are no longer needed
// (old kernels, orphaned dependencies) and clean their package cache after
// every OS update and at most daily. On by default.
type GroupPackageCleanup struct {
	Enabled bool `json:"enabled"`
}

// DefaultGroupPackageCleanup is what a group that never changed it has.
var DefaultGroupPackageCleanup = GroupPackageCleanup{Enabled: true}

func groupPackageCleanupPath(groupID string) string {
	return fmt.Sprintf("/api/v1/customer/groups/%s/package-cleanup", groupID)
}

func (c *Client) GetGroupPackageCleanup(ctx context.Context, groupID string) (*GroupPackageCleanup, error) {
	var out GroupPackageCleanup
	if err := c.do(ctx, http.MethodGet, groupPackageCleanupPath(groupID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) PutGroupPackageCleanup(ctx context.Context, groupID string, p GroupPackageCleanup) (*GroupPackageCleanup, error) {
	var out GroupPackageCleanup
	if err := c.do(ctx, http.MethodPut, groupPackageCleanupPath(groupID), map[string]any{"enabled": p.Enabled}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DevicePackageCleanup mirrors `GET/PUT /api/v1/customer/devices/{id}/package-cleanup`:
// the per-device override ("on", "off" or "inherit"). Enabled and Source
// are the effective, read-only result (the machine owner's local
// `--no-package-cleanup` still wins).
type DevicePackageCleanup struct {
	Mode    string `json:"mode"`
	Enabled bool   `json:"enabled,omitempty"`
	Source  string `json:"source,omitempty"`
}

func devicePackageCleanupPath(deviceID string) string {
	return fmt.Sprintf("/api/v1/customer/devices/%s/package-cleanup", deviceID)
}

func (c *Client) GetDevicePackageCleanup(ctx context.Context, deviceID string) (*DevicePackageCleanup, error) {
	var out DevicePackageCleanup
	if err := c.do(ctx, http.MethodGet, devicePackageCleanupPath(deviceID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) PutDevicePackageCleanup(ctx context.Context, deviceID, mode string) (*DevicePackageCleanup, error) {
	var out DevicePackageCleanup
	if err := c.do(ctx, http.MethodPut, devicePackageCleanupPath(deviceID), map[string]any{"mode": mode}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
