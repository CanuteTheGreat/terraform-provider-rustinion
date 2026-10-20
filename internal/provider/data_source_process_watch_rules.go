package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

var (
	_ datasource.DataSource              = &processWatchRulesDataSource{}
	_ datasource.DataSourceWithConfigure = &processWatchRulesDataSource{}
)

// NewProcessWatchRulesDataSource is this provider's first-ever data
// source — no other read-only, non-managing lookup exists anywhere in
// this provider to mirror (provider.go's own DataSources() returned an
// empty slice before this; see its own doc comment for why). Every
// other resource this provider exposes is also writable, so there was
// never a need for one until now. Looking up an existing group's
// process-watch rules without having to manage them (e.g. to read
// suggestions already created via the admin UI's own "Suggested
// rules" — see process_watch.rs's own doc comment in the main repo)
// is a genuine, real read-only use case, unlike anything else this
// provider has needed before.
func NewProcessWatchRulesDataSource() datasource.DataSource {
	return &processWatchRulesDataSource{}
}

type processWatchRulesDataSource struct {
	client *client.Client
}

type processWatchRuleDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	ProcessName types.String `tfsdk:"process_name"`
	Enabled     types.Bool   `tfsdk:"enabled"`
}

type processWatchRulesDataSourceModel struct {
	GroupID types.String                      `tfsdk:"group_id"`
	Rules   []processWatchRuleDataSourceModel `tfsdk:"rules"`
}

func (d *processWatchRulesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_process_watch_rules"
}

func (d *processWatchRulesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Looks up every process-watch rule belonging to one group, without managing them. The server's own list endpoint (GET /api/v1/customer/process-watch-rules) is tenant-wide with no group filter, matching rustinion_log_alert_rule's own list shape — this data source filters that result client-side to group_id, a deliberate, simple design choice rather than a workaround.",
		Attributes: map[string]schema.Attribute{
			"group_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Only rules belonging to this group are returned.",
			},
			"rules": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Every process-watch rule in group_id.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":           schema.StringAttribute{Computed: true},
						"process_name": schema.StringAttribute{Computed: true},
						"enabled":      schema.BoolAttribute{Computed: true},
					},
				},
			},
		},
	}
}

func (d *processWatchRulesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Data Source Configure Type", fmt.Sprintf("Expected *client.Client, got: %T", req.ProviderData))
		return
	}
	d.client = c
}

func (d *processWatchRulesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config processWatchRulesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	all, err := d.client.ListProcessWatchRules(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error listing process watch rules", err.Error())
		return
	}

	groupID := config.GroupID.ValueString()
	rules := make([]processWatchRuleDataSourceModel, 0, len(all))
	for _, rule := range all {
		if rule.GroupID != groupID {
			continue
		}
		rules = append(rules, processWatchRuleDataSourceModel{
			ID:          types.StringValue(rule.ID),
			ProcessName: types.StringValue(rule.ProcessName),
			Enabled:     types.BoolValue(rule.Enabled),
		})
	}
	config.Rules = rules

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
