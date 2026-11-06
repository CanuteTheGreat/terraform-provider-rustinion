package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// RemoteAccessDLP mirrors GET/PUT /api/v1/customer/{remote-access,groups/{id}/remote-access,devices/{id}/remote-access}/dlp:
// the data-loss policy stored at one level (nil Policy = nothing set there) and the effective result.
type RemoteAccessDLP struct {
	Scope     string          `json:"scope"`
	ScopeID   string          `json:"scope_id"`
	Preset    string          `json:"preset"`
	Policy    json.RawMessage `json:"policy"`
	Effective json.RawMessage `json:"effective"`
}

func remoteAccessDLPPath(scope, scopeID string) (string, error) {
	switch scope {
	case "tenant":
		return "/api/v1/customer/remote-access/dlp", nil
	case "group":
		return fmt.Sprintf("/api/v1/customer/groups/%s/remote-access/dlp", scopeID), nil
	case "device":
		return fmt.Sprintf("/api/v1/customer/devices/%s/remote-access/dlp", scopeID), nil
	}
	return "", fmt.Errorf("scope must be tenant, group or device, got %q", scope)
}

func (c *Client) GetRemoteAccessDLP(ctx context.Context, scope, scopeID string) (*RemoteAccessDLP, error) {
	path, err := remoteAccessDLPPath(scope, scopeID)
	if err != nil {
		return nil, err
	}
	var out RemoteAccessDLP
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PutRemoteAccessDLP sets the level to a preset, or to a full policy (JSON), or clears it (both empty).
func (c *Client) PutRemoteAccessDLP(ctx context.Context, scope, scopeID, preset string, policy json.RawMessage) (*RemoteAccessDLP, error) {
	path, err := remoteAccessDLPPath(scope, scopeID)
	if err != nil {
		return nil, err
	}
	body := map[string]any{}
	switch {
	case preset != "":
		body["preset"] = preset
	case len(policy) > 0:
		body["policy"] = policy
	default:
		body["policy"] = nil
	}
	var out RemoteAccessDLP
	if err := c.do(ctx, http.MethodPut, path, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
