package client

// HTTPAuth mirrors the authentication and TLS options of an OpenMetrics
// scrape target (flattened into the target's JSON) and of an HTTP uptime
// check (the `http_auth` object): custom request headers, bearer token or
// Basic credentials, a custom CA certificate, an explicit skip-TLS-
// verification switch and an optional client certificate.
//
// Request side: every field is sent on every write (null/false/{} clears
// it), so Terraform stays declarative. Response side: the server never
// returns a secret; it only says which are set (`HeaderNames`,
// `HasBearerToken`, `HasBasicAuthPassword`, `HasTLSClientKey`). The
// provider keeps the secret values from the configuration.
type HTTPAuth struct {
	// Request fields.
	Headers           map[string]string `json:"headers"`
	BearerToken       *string           `json:"bearer_token"`
	BasicAuthUser     *string           `json:"basic_auth_user"`
	BasicAuthPassword *string           `json:"basic_auth_password"`
	TLSCAPEM          *string           `json:"tls_ca_pem"`
	TLSSkipVerify     bool              `json:"tls_skip_verify"`
	TLSClientCertPEM  *string           `json:"tls_client_cert_pem"`
	TLSClientKeyPEM   *string           `json:"tls_client_key_pem"`

	// Response-only view (never sent).
	HeaderNames          []string `json:"header_names,omitempty"`
	HasBearerToken       bool     `json:"has_bearer_token,omitempty"`
	HasBasicAuthPassword bool     `json:"has_basic_auth_password,omitempty"`
	HasTLSClientKey      bool     `json:"has_tls_client_key,omitempty"`
}

// ForRequest returns a copy without the response-only view fields.
func (a HTTPAuth) ForRequest() HTTPAuth {
	a.HeaderNames = nil
	a.HasBearerToken = false
	a.HasBasicAuthPassword = false
	a.HasTLSClientKey = false
	return a
}

// IsEmpty reports whether the server shows nothing configured.
func (a HTTPAuth) IsEmpty() bool {
	return len(a.HeaderNames) == 0 && !a.HasBearerToken && a.BasicAuthUser == nil &&
		!a.HasBasicAuthPassword && a.TLSCAPEM == nil && !a.TLSSkipVerify &&
		a.TLSClientCertPEM == nil && !a.HasTLSClientKey
}
