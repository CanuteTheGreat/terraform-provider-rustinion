package client

import (
	"context"
	"net/http"
)

// AlertDependency mirrors `routes::alert_dependencies`' customer API
// (`/api/v1/customer/alert-dependencies`): while the upstream (another
// device, an SNMP target or an uptime check) is down, the alerts of the
// child (one device, or every device of a group) are suppressed. Exactly
// one of DeviceID/GroupID and exactly one of the three Parent* fields is
// set. There is no update endpoint; every field forces a replace.
type AlertDependency struct {
	ID                     string  `json:"id,omitempty"`
	DeviceID               *string `json:"device_id,omitempty"`
	GroupID                *string `json:"group_id,omitempty"`
	ParentDeviceID         *string `json:"parent_device_id,omitempty"`
	ParentSnmpTargetID     *string `json:"parent_snmp_target_id,omitempty"`
	ParentSyntheticCheckID *string `json:"parent_synthetic_check_id,omitempty"`
	CreatedAt              string  `json:"created_at,omitempty"`
}

func (c *Client) CreateAlertDependency(ctx context.Context, d AlertDependency) (*AlertDependency, error) {
	var out AlertDependency
	if err := c.do(ctx, http.MethodPost, "/api/v1/customer/alert-dependencies", d, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetAlertDependency(ctx context.Context, id string) (*AlertDependency, error) {
	var out AlertDependency
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/alert-dependencies/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteAlertDependency(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/customer/alert-dependencies/"+id, nil, nil)
}
