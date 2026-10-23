package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

// httpAuthModel is the `auth` / `http_auth` object of a scrape target and
// of an HTTP check: request headers, bearer token or Basic credentials, a
// custom CA, an explicit skip-TLS-verification switch and a client
// certificate. The secrets (headers, bearer_token, basic_auth_password,
// tls_client_key_pem) are write-only on the server: it never returns them,
// so the provider keeps the configured values in state (marked sensitive)
// and only compares which ones the server says are set.
type httpAuthModel struct {
	Headers               types.Map    `tfsdk:"headers"`
	BearerToken           types.String `tfsdk:"bearer_token"`
	BasicAuthUsername     types.String `tfsdk:"basic_auth_username"`
	BasicAuthPassword     types.String `tfsdk:"basic_auth_password"`
	TLSCAPEM              types.String `tfsdk:"tls_ca_pem"`
	TLSInsecureSkipVerify types.Bool   `tfsdk:"tls_insecure_skip_verify"`
	TLSClientCertPEM      types.String `tfsdk:"tls_client_cert_pem"`
	TLSClientKeyPEM       types.String `tfsdk:"tls_client_key_pem"`
}

func httpAuthAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"headers": schema.MapAttribute{
			Optional:            true,
			Sensitive:           true,
			ElementType:         types.StringType,
			MarkdownDescription: "Extra request headers (name => value). The values are secrets: stored encrypted, never returned by the API or shown in the UI.",
		},
		"bearer_token": schema.StringAttribute{
			Optional:            true,
			Sensitive:           true,
			MarkdownDescription: "Sent as `Authorization: Bearer <token>` (for Horcrux: a JWT; a Horcrux API key goes in `headers` as `X-API-Key`). Stored encrypted, never returned. Not combinable with Basic auth.",
		},
		"basic_auth_username": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "HTTP Basic auth user name (no colon). Not combinable with `bearer_token`.",
		},
		"basic_auth_password": schema.StringAttribute{
			Optional:            true,
			Sensitive:           true,
			MarkdownDescription: "HTTP Basic auth password. Stored encrypted, never returned.",
		},
		"tls_ca_pem": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "PEM certificate(s) of a private CA to trust *instead of* the system roots, for endpoints with a private or self-signed CA (for Horcrux: its `/api/tls/ca.pem`). Preferred over `tls_insecure_skip_verify`. https URLs only.",
			Validators:          []validator.String{stringvalidator.LengthAtMost(65536)},
		},
		"tls_insecure_skip_verify": schema.BoolAttribute{
			Optional:            true,
			Computed:            true,
			Default:             booldefault.StaticBool(false),
			MarkdownDescription: "**Insecure.** Do not verify the server certificate at all: anyone on the network path can impersonate the endpoint and read the credentials sent to it. Use `tls_ca_pem` instead where you can. Not combinable with `tls_ca_pem`. Default false.",
		},
		"tls_client_cert_pem": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "Client certificate (PEM) for mutual TLS. Needs `tls_client_key_pem`.",
		},
		"tls_client_key_pem": schema.StringAttribute{
			Optional:            true,
			Sensitive:           true,
			MarkdownDescription: "Private key (PEM) of the client certificate. Stored encrypted, never returned. PKCS#8 (`BEGIN PRIVATE KEY`) is accepted everywhere; relayed checks and scrape targets also accept RSA/EC keys.",
		},
	}
}

func optStringKeep(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	s := v.ValueString()
	return &s
}

// httpAuthToClient: a nil model means "nothing configured" and clears
// everything on the server.
func httpAuthToClient(ctx context.Context, m *httpAuthModel) (client.HTTPAuth, diag.Diagnostics) {
	var diags diag.Diagnostics
	out := client.HTTPAuth{}
	if m == nil {
		return out, diags
	}
	if !m.Headers.IsNull() && !m.Headers.IsUnknown() {
		diags.Append(m.Headers.ElementsAs(ctx, &out.Headers, false)...)
	}
	out.BearerToken = optStringKeep(m.BearerToken)
	out.BasicAuthUser = optStringKeep(m.BasicAuthUsername)
	out.BasicAuthPassword = optStringKeep(m.BasicAuthPassword)
	out.TLSCAPEM = optStringKeep(m.TLSCAPEM)
	out.TLSSkipVerify = !m.TLSInsecureSkipVerify.IsNull() && !m.TLSInsecureSkipVerify.IsUnknown() && m.TLSInsecureSkipVerify.ValueBool()
	out.TLSClientCertPEM = optStringKeep(m.TLSClientCertPEM)
	out.TLSClientKeyPEM = optStringKeep(m.TLSClientKeyPEM)
	return out, diags
}

func sameStringSet(a []string, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for _, n := range a {
		if _, ok := b[n]; !ok {
			return false
		}
	}
	return true
}

// httpAuthFromClient builds the state object from the server's secret-free
// view. Secret values come from `prior` (plan or state) and are kept only
// while the server confirms they are still set, so removing a credential
// out-of-band shows up as a plan diff.
func httpAuthFromClient(ctx context.Context, a *client.HTTPAuth, prior *httpAuthModel) (*httpAuthModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	if a == nil || a.IsEmpty() {
		// An `auth = {}` the user wrote stays as written.
		if prior != nil && httpAuthIsBlank(prior) {
			return prior, diags
		}
		return nil, diags
	}
	m := &httpAuthModel{
		Headers:               types.MapNull(types.StringType),
		BearerToken:           types.StringNull(),
		BasicAuthUsername:     types.StringPointerValue(a.BasicAuthUser),
		BasicAuthPassword:     types.StringNull(),
		TLSCAPEM:              types.StringPointerValue(a.TLSCAPEM),
		TLSInsecureSkipVerify: types.BoolValue(a.TLSSkipVerify),
		TLSClientCertPEM:      types.StringPointerValue(a.TLSClientCertPEM),
		TLSClientKeyPEM:       types.StringNull(),
	}
	if prior != nil {
		if len(a.HeaderNames) > 0 && !prior.Headers.IsNull() && !prior.Headers.IsUnknown() {
			var have map[string]string
			diags.Append(prior.Headers.ElementsAs(ctx, &have, false)...)
			if sameStringSet(a.HeaderNames, have) {
				m.Headers = prior.Headers
			}
		}
		if a.HasBearerToken {
			m.BearerToken = prior.BearerToken
		}
		if a.HasBasicAuthPassword {
			m.BasicAuthPassword = prior.BasicAuthPassword
		}
		if a.HasTLSClientKey {
			m.TLSClientKeyPEM = prior.TLSClientKeyPEM
		}
	}
	return m, diags
}

func httpAuthIsBlank(m *httpAuthModel) bool {
	return (m.Headers.IsNull() || len(m.Headers.Elements()) == 0) &&
		m.BearerToken.IsNull() && m.BasicAuthUsername.IsNull() && m.BasicAuthPassword.IsNull() &&
		m.TLSCAPEM.IsNull() && !m.TLSInsecureSkipVerify.ValueBool() &&
		m.TLSClientCertPEM.IsNull() && m.TLSClientKeyPEM.IsNull()
}
