package client

import (
	"context"
	"fmt"
	"net/http"
)

// AwsSecurityAccount mirrors `/api/v1/customer/aws/accounts` (an AWS account
// whose CloudTrail events and GuardDuty findings Rustinion reads through a
// read-only cross-account role).
type AwsSecurityAccount struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	AccountID  string   `json:"account_id"`
	RoleName   string   `json:"role_name"`
	RoleARN    string   `json:"role_arn"`
	ExternalID string   `json:"external_id"`
	Regions    []string `json:"regions"`
	DeviceID   string   `json:"device_id"`
	CloudTrail bool     `json:"cloudtrail"`
	GuardDuty  bool     `json:"guardduty"`
	Enabled    bool     `json:"enabled"`
	LastError  *string  `json:"last_error"`
}

type awsSecurityAccountCreate struct {
	Name       string   `json:"name"`
	AccountID  string   `json:"account_id"`
	RoleName   string   `json:"role_name,omitempty"`
	Regions    []string `json:"regions"`
	DeviceID   string   `json:"device_id"`
	CloudTrail bool     `json:"cloudtrail"`
	GuardDuty  bool     `json:"guardduty"`
}

type awsSecurityAccountUpdate struct {
	Name       string   `json:"name"`
	Enabled    bool     `json:"enabled"`
	CloudTrail bool     `json:"cloudtrail"`
	GuardDuty  bool     `json:"guardduty"`
	Regions    []string `json:"regions"`
}

func awsSecurityAccountPath(id string) string {
	return fmt.Sprintf("/api/v1/customer/aws/accounts/%s", id)
}

func (c *Client) CreateAwsSecurityAccount(ctx context.Context, a AwsSecurityAccount) (*AwsSecurityAccount, error) {
	body := awsSecurityAccountCreate{
		Name: a.Name, AccountID: a.AccountID, RoleName: a.RoleName, Regions: a.Regions,
		DeviceID: a.DeviceID, CloudTrail: a.CloudTrail, GuardDuty: a.GuardDuty,
	}
	var out AwsSecurityAccount
	if err := c.do(ctx, http.MethodPost, "/api/v1/customer/aws/accounts", body, &out); err != nil {
		return nil, err
	}
	// The server creates accounts enabled; apply a requested "disabled".
	if !a.Enabled {
		return c.UpdateAwsSecurityAccount(ctx, out.ID, a)
	}
	return &out, nil
}

func (c *Client) GetAwsSecurityAccount(ctx context.Context, id string) (*AwsSecurityAccount, error) {
	var out AwsSecurityAccount
	if err := c.do(ctx, http.MethodGet, awsSecurityAccountPath(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateAwsSecurityAccount(ctx context.Context, id string, a AwsSecurityAccount) (*AwsSecurityAccount, error) {
	body := awsSecurityAccountUpdate{
		Name: a.Name, Enabled: a.Enabled, CloudTrail: a.CloudTrail, GuardDuty: a.GuardDuty, Regions: a.Regions,
	}
	var out AwsSecurityAccount
	if err := c.do(ctx, http.MethodPut, awsSecurityAccountPath(id), body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteAwsSecurityAccount(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, awsSecurityAccountPath(id), nil, nil)
}
