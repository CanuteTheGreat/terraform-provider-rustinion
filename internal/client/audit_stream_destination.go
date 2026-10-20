package client

import (
	"context"
	"net/http"
)

// AuditStreamDestination mirrors
// `routes::customer_api::AuditStreamDestinationInput`/
// `audit_stream::AuditStreamDestinationRow`. Three real delivery types,
// `webhook` (the original), `splunk_hec`, and `syslog`, coexist in one
// flexible shape — the same "one struct, kind-specific fields coexist"
// pattern `AlertRuleCondition`'s own `condition_kind` already
// established.
//
// `WebhookURL`/`HECEndpoint`/`HECToken`/`HECIndex`/`HECSourcetype`/
// `SyslogHost`/`SyslogPort`/`SyslogUseTLS` are all pointer types
// (`*string`/`*int64`/`*bool`), never plain value types — the Rust
// side's own `#[serde(skip_serializing_if = "Option::is_none")]` means
// a field that doesn't apply to a given destination's own type is
// OMITTED from the JSON entirely, not sent as `null`; a plain Go value
// type can't tell "absent from the response" apart from "explicitly
// empty/zero", which caused a real "Provider produced inconsistent
// result after apply" failure (Terraform requires a `null` state for an
// unset `Optional` attribute, never `""`/`0`/`false`) on a real
// acceptance-test run before this was fixed for the HEC fields. Matches
// the exact same pointer convention `AlertRuleCondition`'s own
// `TagKey`/`TagValue`/`CompositeRuleID` already established for this
// identical problem — applied here to `SyslogUseTLS` too, even though a
// `bool`'s own zero value (`false`) might look "safe" at first glance:
// it isn't, for the exact same absent-vs-explicit reason.
//
// `SigningSecret` is server-generated and only ever present on the
// response to `CreateAuditStreamDestination` — every other response
// (including `GetAuditStreamDestination`, called on every
// `terraform plan`/refresh) omits it entirely, by deliberate design on
// the Rust side (see that module's own doc comment). `HECToken`, by
// contrast, is a customer-*supplied* secret with no such "shown once"
// property at all — it's freely present on every real response,
// including a later GET, for either create/update/list/get. `syslog`
// has no secret/token concept at all — its own "authentication" is the
// TLS transport itself (`SyslogUseTLS`) or bare network trust.
type AuditStreamDestination struct {
	ID                   string  `json:"id,omitempty"`
	Name                 string  `json:"name"`
	DestinationType      string  `json:"destination_type,omitempty"`
	WebhookURL           *string `json:"webhook_url,omitempty"`
	Enabled              bool    `json:"enabled"`
	SigningSecret        string  `json:"signing_secret,omitempty"`
	HECEndpoint          *string `json:"hec_endpoint,omitempty"`
	HECToken             *string `json:"hec_token,omitempty"`
	HECIndex             *string `json:"hec_index,omitempty"`
	HECSourcetype        *string `json:"hec_sourcetype,omitempty"`
	SyslogHost           *string `json:"syslog_host,omitempty"`
	SyslogPort           *int64  `json:"syslog_port,omitempty"`
	SyslogUseTLS         *bool   `json:"syslog_use_tls,omitempty"`
	LastDeliveredAt      string  `json:"last_delivered_created_at,omitempty"`
	LastDeliveryFailedAt string  `json:"last_delivery_failed_at,omitempty"`
	LastDeliveryError    string  `json:"last_delivery_error,omitempty"`
}

// AuditStreamDestinationUpdate is the body `PUT
// /api/v1/customer/audit-stream-destinations/:id` accepts — a pure
// outbound request shape (never decoded from a response, unlike
// `AuditStreamDestination` above), so plain value types are sufficient
// here; there's no absent-vs-empty ambiguity to resolve on the way out.
// `destination_type` can't be changed after creation (delete and
// recreate to switch types), and `hec_token` is never updatable here
// either (no rotation without recreate, a real stated v1 scope cut,
// matching the pre-existing `signing_secret` one) — `syslog` has no
// equivalent secret to omit in the first place.
type AuditStreamDestinationUpdate struct {
	Name          string `json:"name"`
	WebhookURL    string `json:"webhook_url,omitempty"`
	HECEndpoint   string `json:"hec_endpoint,omitempty"`
	HECIndex      string `json:"hec_index,omitempty"`
	HECSourcetype string `json:"hec_sourcetype,omitempty"`
	SyslogHost    string `json:"syslog_host,omitempty"`
	SyslogPort    int64  `json:"syslog_port,omitempty"`
	SyslogUseTLS  bool   `json:"syslog_use_tls"`
	Enabled       bool   `json:"enabled"`
}

func (c *Client) CreateAuditStreamDestination(ctx context.Context, d AuditStreamDestination) (*AuditStreamDestination, error) {
	var out AuditStreamDestination
	body := struct {
		Name            string  `json:"name"`
		DestinationType string  `json:"destination_type,omitempty"`
		WebhookURL      *string `json:"webhook_url,omitempty"`
		HECEndpoint     *string `json:"hec_endpoint,omitempty"`
		HECToken        *string `json:"hec_token,omitempty"`
		HECIndex        *string `json:"hec_index,omitempty"`
		HECSourcetype   *string `json:"hec_sourcetype,omitempty"`
		SyslogHost      *string `json:"syslog_host,omitempty"`
		SyslogPort      *int64  `json:"syslog_port,omitempty"`
		SyslogUseTLS    *bool   `json:"syslog_use_tls,omitempty"`
	}{
		Name:            d.Name,
		DestinationType: d.DestinationType,
		WebhookURL:      d.WebhookURL,
		HECEndpoint:     d.HECEndpoint,
		HECToken:        d.HECToken,
		HECIndex:        d.HECIndex,
		HECSourcetype:   d.HECSourcetype,
		SyslogHost:      d.SyslogHost,
		SyslogPort:      d.SyslogPort,
		SyslogUseTLS:    d.SyslogUseTLS,
	}
	if err := c.do(ctx, http.MethodPost, "/api/v1/customer/audit-stream-destinations", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetAuditStreamDestination(ctx context.Context, id string) (*AuditStreamDestination, error) {
	var out AuditStreamDestination
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/audit-stream-destinations/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateAuditStreamDestination(ctx context.Context, id string, d AuditStreamDestinationUpdate) (*AuditStreamDestination, error) {
	var out AuditStreamDestination
	if err := c.do(ctx, http.MethodPut, "/api/v1/customer/audit-stream-destinations/"+id, d, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteAuditStreamDestination(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/customer/audit-stream-destinations/"+id, nil, nil)
}
