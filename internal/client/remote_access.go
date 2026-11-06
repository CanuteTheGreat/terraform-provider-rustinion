package client

import (
	"context"
	"fmt"
	"net/http"
)

// RemoteAccessChannels lists every channel the server knows (delegated remote access control). The
// terminal is a separate setting from the desktop; every other channel also needs the desktop.
var RemoteAccessChannels = []string{
	"terminal", "desktop", "clipboard", "files", "drive", "print", "usb", "audio", "mic",
	"recording", "unattended", "secure_desktop",
}

// RemoteAccessCell is one scope's own setting for one channel.
type RemoteAccessCell struct {
	Value  string `json:"value"`
	Locked bool   `json:"locked"`
	SetBy  string `json:"set_by"`
	// DefaultFromLegacy: the organization has never decided (the column default "not enabled"). It is
	// shown as a locked off but is not something a configuration set.
	DefaultFromLegacy bool `json:"default_from_legacy"`
}

// RemoteAccessChannel is one channel of GET /api/v1/customer/.../remote-access.
type RemoteAccessChannel struct {
	Channel   string                      `json:"channel"`
	Cells     map[string]RemoteAccessCell `json:"cells"`
	Effective bool                        `json:"effective"`
	Why       string                      `json:"why"`
	Locked    bool                        `json:"locked"`
}

// RemoteAccessView is the server's answer for a scope.
type RemoteAccessView struct {
	Scope    string                `json:"scope"`
	ScopeID  string                `json:"scope_id"`
	Channels []RemoteAccessChannel `json:"channels"`
}

// RemoteAccessInput is one channel of a PUT. A nil entry clears the setting (inherit).
type RemoteAccessInput struct {
	Value  string `json:"value"`
	Locked bool   `json:"locked"`
}

// Cell returns the scope's own cell for a channel.
func (v *RemoteAccessView) Cell(channel string) (RemoteAccessCell, bool) {
	for _, ch := range v.Channels {
		if ch.Channel == channel {
			c, ok := ch.Cells[v.Scope]
			return c, ok
		}
	}
	return RemoteAccessCell{}, false
}

func remoteAccessPath(scope, scopeID string) (string, error) {
	switch scope {
	case "tenant":
		return "/api/v1/customer/remote-access", nil
	case "group":
		return fmt.Sprintf("/api/v1/customer/groups/%s/remote-access", scopeID), nil
	case "device":
		return fmt.Sprintf("/api/v1/customer/devices/%s/remote-access", scopeID), nil
	}
	return "", fmt.Errorf("scope must be tenant, group or device, got %q", scope)
}

func (c *Client) GetRemoteAccess(ctx context.Context, scope, scopeID string) (*RemoteAccessView, error) {
	path, err := remoteAccessPath(scope, scopeID)
	if err != nil {
		return nil, err
	}
	var out RemoteAccessView
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PutRemoteAccess sets the listed channels at the scope; a nil input clears that channel (inherit).
// Channels not listed are untouched.
func (c *Client) PutRemoteAccess(ctx context.Context, scope, scopeID string, channels map[string]*RemoteAccessInput) (*RemoteAccessView, error) {
	path, err := remoteAccessPath(scope, scopeID)
	if err != nil {
		return nil, err
	}
	body := map[string]any{"channels": channels}
	var out RemoteAccessView
	if err := c.do(ctx, http.MethodPut, path, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RemoteAccessOperator mirrors one row of /api/v1/customer/remote-access/operators: who may start sessions.
type RemoteAccessOperator struct {
	ID      string `json:"id"`
	Scope   string `json:"scope"`
	ScopeID string `json:"scope_id"`
	UserID  string `json:"user_id"`
	Email   string `json:"email"`
	Channel string `json:"channel"`
	Effect  string `json:"effect"`
	SetBy   string `json:"set_by"`
}

type remoteAccessOperatorList struct {
	Operators []RemoteAccessOperator `json:"operators"`
}

func (c *Client) ListRemoteAccessOperators(ctx context.Context) ([]RemoteAccessOperator, error) {
	var out remoteAccessOperatorList
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/remote-access/operators", nil, &out); err != nil {
		return nil, err
	}
	return out.Operators, nil
}

// GetRemoteAccessOperator finds one row by id (the API has no single-row GET); a *APIError 404 when gone.
func (c *Client) GetRemoteAccessOperator(ctx context.Context, id string) (*RemoteAccessOperator, error) {
	rows, err := c.ListRemoteAccessOperators(ctx)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		if rows[i].ID == id {
			return &rows[i], nil
		}
	}
	return nil, &APIError{StatusCode: http.StatusNotFound, Message: "no such operator row"}
}

// PutRemoteAccessOperator adds or replaces the row for (scope, person, channel).
func (c *Client) PutRemoteAccessOperator(ctx context.Context, scope, scopeID, userID, email, channel, effect string) (*RemoteAccessOperator, error) {
	body := map[string]any{"scope": scope, "channel": channel, "effect": effect}
	if scopeID != "" {
		body["scope_id"] = scopeID
	}
	if userID != "" {
		body["user_id"] = userID
	}
	if email != "" {
		body["email"] = email
	}
	var out RemoteAccessOperator
	if err := c.do(ctx, http.MethodPut, "/api/v1/customer/remote-access/operators", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteRemoteAccessOperator(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/customer/remote-access/operators/"+id, nil, nil)
}
