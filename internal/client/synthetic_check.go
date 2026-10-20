package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// SyntheticCheckStep mirrors `routes::customer_api::
// SyntheticCheckStepInput`/`CustomerSyntheticCheckStep`. Only meaningful
// (and only accepted by the server) when the parent check's `CheckType`
// is `"http_multistep"` — see `SyntheticCheck`'s own doc comment.
type SyntheticCheckStep struct {
	ID                string          `json:"id,omitempty"`
	Name              string          `json:"name"`
	Method            string          `json:"method"`
	URL               string          `json:"url"`
	Headers           json.RawMessage `json:"headers,omitempty"`
	Body              *string         `json:"body,omitempty"`
	ExpectedStatusMin int64           `json:"expected_status_min"`
	ExpectedStatusMax int64           `json:"expected_status_max"`
	AssertBodyRegex   *string         `json:"assert_body_regex,omitempty"`
	AssertJSONPath    *string         `json:"assert_json_path,omitempty"`
	AssertJSONValue   *string         `json:"assert_json_value,omitempty"`
	CaptureVariable   *string         `json:"capture_variable,omitempty"`
	CaptureSource     *string         `json:"capture_source,omitempty"`
	CapturePath       *string         `json:"capture_path,omitempty"`
}

// SyntheticCheckLocation mirrors `routes::customer_api::
// SyntheticCheckLocationInput`/`CustomerSyntheticCheckLocation` — one of
// the check's own group's enrolled devices, relaying this check from
// wherever that device actually is. Only meaningful for a `relay_mode =
// "relay"` check; see `SyntheticCheck.MinFailingLocations`'s own doc
// comment for the aggregation this activates.
type SyntheticCheckLocation struct {
	ID       string  `json:"id,omitempty"`
	DeviceID string  `json:"device_id"`
	Label    *string `json:"label,omitempty"`
}

// SyntheticCheck mirrors `routes::customer_api::SyntheticCheckInput`/
// `CustomerSyntheticCheck`. `CheckType` is one of `"http"`, `"tcp"`,
// `"tls"`, `"icmp"`, `"dns"` or `"http_multistep"` — see that Rust module's own doc comment for why
// these have no dedicated enum anywhere in the main app either, and why
// this provider validates against the same literal set rather than a
// stricter one. `Steps` is only valid (and only meaningful) for
// `"http_multistep"`; the server rejects a non-empty `Steps` on any
// other check type.
type SyntheticCheck struct {
	ID                string `json:"id,omitempty"`
	GroupID           string `json:"group_id"`
	Name              string `json:"name"`
	CheckType         string `json:"check_type"`
	Target            string `json:"target"`
	HTTPMethod        string `json:"http_method"`
	ExpectedStatusMin int64  `json:"expected_status_min"`
	ExpectedStatusMax int64  `json:"expected_status_max"`
	TimeoutMs         int64  `json:"timeout_ms"`
	IntervalSeconds   int64  `json:"interval_seconds"`
	RelayMode         string `json:"relay_mode"`
	Enabled           bool   `json:"enabled"`
	// How many of this check's own *configured* Locations must be
	// currently failing before it's considered down — 1 (the default)
	// means "any location failing," N (the full count) means "every
	// location must agree." Meaningless until Locations is non-empty; a
	// check with none keeps using the server's existing dynamic
	// single-relay-device behavior exactly as before this field existed.
	MinFailingLocations int64 `json:"min_failing_locations"`
	// DNS checks: record type (A, AAAA, CNAME, MX, NS, TXT; nil = A), the
	// server to ask (nil = the runner's resolver) and the comma-separated
	// values the answer must contain (nil = any answer).
	DNSRecordType  *string `json:"dns_record_type"`
	DNSServer      *string `json:"dns_server"`
	ExpectedAnswer *string `json:"expected_answer"`
	// TLS certificate checks: an untrusted chain or name mismatch fails.
	TLSVerify bool `json:"tls_verify"`
	// Any check: slower than this counts as a failure (nil = no limit).
	MaxResponseTimeMs *int64 `json:"max_response_time_ms"`
	// TLS and https checks: certificate_expiring warning/critical windows.
	CertWarningDays  int64 `json:"cert_warning_days"`
	CertCriticalDays int64 `json:"cert_critical_days"`
	// Alert after this many failed runs in a row.
	FailuresBeforeAlert int64                    `json:"failures_before_alert"`
	Steps               []SyntheticCheckStep     `json:"steps"`
	Locations           []SyntheticCheckLocation `json:"locations"`
}

func (c *Client) CreateSyntheticCheck(ctx context.Context, check SyntheticCheck) (*SyntheticCheck, error) {
	var out SyntheticCheck
	if err := c.do(ctx, http.MethodPost, "/api/v1/customer/synthetic-checks", check, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetSyntheticCheck(ctx context.Context, id string) (*SyntheticCheck, error) {
	var out SyntheticCheck
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/synthetic-checks/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateSyntheticCheck(ctx context.Context, id string, check SyntheticCheck) (*SyntheticCheck, error) {
	var out SyntheticCheck
	path := fmt.Sprintf("/api/v1/customer/synthetic-checks/%s", id)
	if err := c.do(ctx, http.MethodPut, path, check, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteSyntheticCheck(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/customer/synthetic-checks/"+id, nil, nil)
}
