package client

import (
	"context"
	"encoding/json"
	"net/http"
)

// GroupLogCollection is a group's log collection settings
// (GET/PUT /api/v1/customer/groups/{id}/log-collection).
type GroupLogCollection struct {
	Settings  json.RawMessage `json:"settings"`
	Effective json.RawMessage `json:"effective"`
}

// DeviceLogCollection is a device's override plus its effective settings
// (GET/PUT /api/v1/customer/devices/{id}/log-collection).
type DeviceLogCollection struct {
	Override  json.RawMessage `json:"override"`
	Effective json.RawMessage `json:"effective"`
}

func groupLogCollectionPath(id string) string {
	return "/api/v1/customer/groups/" + id + "/log-collection"
}

func deviceLogCollectionPath(id string) string {
	return "/api/v1/customer/devices/" + id + "/log-collection"
}

func (c *Client) GetGroupLogCollection(ctx context.Context, groupID string) (*GroupLogCollection, error) {
	var out GroupLogCollection
	if err := c.do(ctx, http.MethodGet, groupLogCollectionPath(groupID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) PutGroupLogCollection(ctx context.Context, groupID string, settings json.RawMessage) (*GroupLogCollection, error) {
	var out GroupLogCollection
	if err := c.do(ctx, http.MethodPut, groupLogCollectionPath(groupID), settings, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetDeviceLogCollection(ctx context.Context, deviceID string) (*DeviceLogCollection, error) {
	var out DeviceLogCollection
	if err := c.do(ctx, http.MethodGet, deviceLogCollectionPath(deviceID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PutDeviceLogCollection sets the device override; `null` = inherit.
func (c *Client) PutDeviceLogCollection(ctx context.Context, deviceID string, override json.RawMessage) (*DeviceLogCollection, error) {
	var out DeviceLogCollection
	if err := c.do(ctx, http.MethodPut, deviceLogCollectionPath(deviceID), override, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
