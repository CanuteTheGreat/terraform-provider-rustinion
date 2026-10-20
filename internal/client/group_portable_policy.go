package client

import (
	"context"
	"fmt"
	"net/http"
)

// GroupPortablePolicy mirrors `portable::GroupPortablePolicy`: whether a
// group's devices are portable by default (laptops: shown as "Away"
// instead of alerting when they stop checking in), how long that lasts
// before one low-severity notice, and how long a device that announced
// sleep/shutdown shows as "Asleep". A per-group singleton that always
// exists (the defaults are false / 14 days / 48 hours).
type GroupPortablePolicy struct {
	PortableDevices   bool  `json:"portable_devices"`
	PortableGraceDays int64 `json:"portable_grace_days"`
	AsleepGraceHours  int64 `json:"asleep_grace_hours"`
}

// Defaults the server applies to a group that never set these.
var DefaultGroupPortablePolicy = GroupPortablePolicy{
	PortableDevices:   false,
	PortableGraceDays: 14,
	AsleepGraceHours:  48,
}

func groupPortablePath(groupID string) string {
	return fmt.Sprintf("/api/v1/customer/groups/%s/portable", groupID)
}

func (c *Client) GetGroupPortablePolicy(ctx context.Context, groupID string) (*GroupPortablePolicy, error) {
	var out GroupPortablePolicy
	if err := c.do(ctx, http.MethodGet, groupPortablePath(groupID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) PutGroupPortablePolicy(ctx context.Context, groupID string, p GroupPortablePolicy) (*GroupPortablePolicy, error) {
	var out GroupPortablePolicy
	if err := c.do(ctx, http.MethodPut, groupPortablePath(groupID), p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
