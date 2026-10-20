// Package provider implements the Terraform provider for Rustinion's
// customer-facing config-as-code API (web/src/routes/customer_api.rs in
// the main repo) — full CRUD for alert rules, notification channels,
// dashboards, synthetic checks, log alert rules, integrations, the
// tenant's one status page, escalation policies (with multi-channel
// fan-out per step), on-call schedules (plus overrides), groups, SLOs,
// silences, users, process-watch rules, and escalation acknowledgements
// — every resource that API exposes today — plus one data source,
// rustinion_process_watch_rules, the first in this provider. See the
// top-level README.md for setup/auth and the main repo's
// docs/roadmap.md for what this provider deliberately does not cover
// yet.
package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

var _ provider.Provider = &RustinionProvider{}

// RustinionProvider is the top-level `provider "rustinion" {}` block.
type RustinionProvider struct {
	// version is injected by main.go — "dev" for a local build, a real
	// semver for a tagged release (via -ldflags). Surfaced only through
	// resp.Version in Metadata; nothing in this provider's own behavior
	// varies by version today.
	version string
}

// rustinionProviderModel is the schema.Schema below, decoded.
type rustinionProviderModel struct {
	BaseURL types.String `tfsdk:"base_url"`
	APIKey  types.String `tfsdk:"api_key"`
}

// New returns a provider.Provider factory, the shape both
// providerserver.Serve (main.go) and terraform-plugin-testing's own
// ProtoV6ProviderFactories (acceptance tests) expect.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &RustinionProvider{version: version}
	}
}

func (p *RustinionProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "rustinion"
	resp.Version = p.version
}

func (p *RustinionProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages Rustinion (https://rustinion.com) configuration as code through its customer API: groups and their update, restart, alerting and scan policies, alert rules, log alerts and pipelines, notification channels and routes, escalation policies, on-call schedules, dashboards, SLOs, silences, maintenance windows, synthetic checks, SNMP targets, device tags, integrations, RUM applications, scheduled reports, status pages, users and roles. Authenticate with a read+write API key from the organization's admin panel.",
		Attributes: map[string]schema.Attribute{
			"base_url": schema.StringAttribute{
				Optional:    true,
				Description: "Base URL of the Rustinion instance to manage, e.g. https://rustinion.com. Defaults to the RUSTINION_BASE_URL environment variable.",
			},
			"api_key": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "A read-write-scoped Rustinion API key (Tenant admin panel -> API keys -> Read + write). Defaults to the RUSTINION_API_KEY environment variable. Every resource this provider manages is a write, so a read-only key is rejected by the API with a 403 on the first apply.",
			},
		},
	}
}

func (p *RustinionProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config rustinionProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	baseURL := os.Getenv("RUSTINION_BASE_URL")
	if !config.BaseURL.IsNull() && config.BaseURL.ValueString() != "" {
		baseURL = config.BaseURL.ValueString()
	}
	apiKey := os.Getenv("RUSTINION_API_KEY")
	if !config.APIKey.IsNull() && config.APIKey.ValueString() != "" {
		apiKey = config.APIKey.ValueString()
	}

	if baseURL == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("base_url"),
			"Missing Rustinion base_url",
			"Set base_url in the provider configuration block or the RUSTINION_BASE_URL environment variable.",
		)
	}
	if apiKey == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_key"),
			"Missing Rustinion api_key",
			"Set api_key in the provider configuration block or the RUSTINION_API_KEY environment variable.",
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	c := client.New(baseURL, apiKey)
	resp.DataSourceData = c
	resp.ResourceData = c
}

func (p *RustinionProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewAlertRuleResource,
		NewNotificationChannelResource,
		NewNotificationRouteResource,
		NewDashboardResource,
		NewSyntheticCheckResource,
		NewLogAlertRuleResource,
		NewLogMetricDefinitionResource,
		NewLogPipelineProcessorResource,
		NewRetentionPolicyResource,
		NewAuditStreamDestinationResource,
		NewLogArchiveDestinationResource,
		NewIntegrationResource,
		NewStatusPageResource,
		NewStatusPageIncidentResource,
		NewEscalationPolicyResource,
		NewEscalationAcknowledgementResource,
		NewOnCallScheduleResource,
		NewOnCallOverrideResource,
		NewGroupResource,
		NewGroupPortablePolicyResource,
		NewGroupRestartPolicyResource,
		NewGroupOfflineAlertsResource,
		NewGroupHostHealthResource,
		NewGroupCustomMetricsResource,
		NewAlertRoutingResource,
		NewGroupPackageCleanupResource,
		NewDevicePackageCleanupResource,
		NewGroupMalwareScanResource,
		NewMalwareAllowlistRuleResource,
		NewGroupAlertTuningResource,
		NewSloResource,
		NewSilenceResource,
		NewSNMPTargetResource,
		NewDeviceTagResource,
		NewMaintenanceWindowResource,
		NewRUMApplicationResource,
		NewReportScheduleResource,
		NewUserResource,
		NewProcessWatchRuleResource,
		NewPermissionGrantResource,
		NewPermissionRoleResource,
		NewPermissionRoleAssignmentResource,
		NewPermissionRoleMappingResource,
	}
}

// rustinion_process_watch_rules is this provider's first-ever data
// source (see its own doc comment for why one didn't exist before
// now). Every other resource this API exposes is also writable, so
// there had been no need for a read-only companion until process-watch
// rules — which the admin UI itself can now suggest without creating
// (see process_watch.rs's own "Suggested rules" feature in the main
// repo) — gave a genuine reason to look some up without managing them.
func (p *RustinionProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewProcessWatchRulesDataSource,
	}
}
