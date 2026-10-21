package client

import (
	"context"
	"fmt"
	"net/http"
)

// GroupNotifications mirrors `GET/PUT /api/v1/customer/groups/{id}/notifications`:
// whether findings on the group's devices may be pushed to people.
// Mode is "normal", "digest_only" (never pushed for any reason or severity,
// emergencies included; still listed in the email digest) or "silent"
// (not emailed either). A per-group singleton that always exists
// (default "normal").
type GroupNotifications struct {
	Mode string `json:"mode"`
	// Pushes and Emails are read-only consequences of Mode.
	Pushes *bool `json:"pushes,omitempty"`
	Emails *bool `json:"emails,omitempty"`
}

// DefaultGroupNotificationsMode is what a group that never changed it has.
const DefaultGroupNotificationsMode = "normal"

func groupNotificationsPath(groupID string) string {
	return fmt.Sprintf("/api/v1/customer/groups/%s/notifications", groupID)
}

func (c *Client) GetGroupNotifications(ctx context.Context, groupID string) (*GroupNotifications, error) {
	var out GroupNotifications
	if err := c.do(ctx, http.MethodGet, groupNotificationsPath(groupID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) PutGroupNotifications(ctx context.Context, groupID string, mode string) (*GroupNotifications, error) {
	var out GroupNotifications
	if err := c.do(ctx, http.MethodPut, groupNotificationsPath(groupID), map[string]any{"mode": mode}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
