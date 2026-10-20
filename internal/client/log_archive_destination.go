package client

import (
	"context"
	"net/http"
)

// LogArchiveDestination mirrors
// `routes::customer_api::LogArchiveDestinationInput`/
// `s3_archive::LogArchiveDestinationRow`. One destination per tenant
// (`log_archive_destinations`'s own `unique(tenant_id)` constraint).
//
// `SecretAccessKey` is a customer-supplied AWS secret key the server
// never echoes back at any point, ever -- not even once, a third,
// stricter secret-handling convention than either one
// `audit_stream_destinations` has (not `signing_secret`'s "shown once,"
// not `hec_token`'s "always freely returned"). It's write-only here:
// sent on create, absent from every real response body (list/get/create
// response alike), so this struct has no field for it at all -- there is
// nothing to decode.
type LogArchiveDestination struct {
	ID          string `json:"id,omitempty"`
	Bucket      string `json:"bucket"`
	Region      string `json:"region"`
	AccessKeyID string `json:"access_key_id,omitempty"`
	Prefix      string `json:"prefix,omitempty"`
	Enabled     bool   `json:"enabled"`
	CreatedAt   string `json:"created_at,omitempty"`
}

// LogArchiveDestinationCreate is the body `POST
// /api/v1/customer/log-archive-destinations` accepts.
type LogArchiveDestinationCreate struct {
	Bucket          string `json:"bucket"`
	Region          string `json:"region"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	Prefix          string `json:"prefix,omitempty"`
}

// LogArchiveDestinationUpdate is the body `PUT .../:id` accepts --
// `bucket`/`region`/`prefix`/`enabled` only. `access_key_id`/
// `secret_access_key` are not updatable here (a real, stated v1 scope
// cut -- rotate credentials by deleting and recreating the destination).
type LogArchiveDestinationUpdate struct {
	Bucket  string `json:"bucket"`
	Region  string `json:"region"`
	Prefix  string `json:"prefix,omitempty"`
	Enabled bool   `json:"enabled"`
}

func (c *Client) CreateLogArchiveDestination(ctx context.Context, d LogArchiveDestinationCreate) (*LogArchiveDestination, error) {
	var out LogArchiveDestination
	if err := c.do(ctx, http.MethodPost, "/api/v1/customer/log-archive-destinations", d, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetLogArchiveDestination(ctx context.Context, id string) (*LogArchiveDestination, error) {
	var out LogArchiveDestination
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/log-archive-destinations/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateLogArchiveDestination(ctx context.Context, id string, d LogArchiveDestinationUpdate) (*LogArchiveDestination, error) {
	var out LogArchiveDestination
	if err := c.do(ctx, http.MethodPut, "/api/v1/customer/log-archive-destinations/"+id, d, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteLogArchiveDestination(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/customer/log-archive-destinations/"+id, nil, nil)
}
