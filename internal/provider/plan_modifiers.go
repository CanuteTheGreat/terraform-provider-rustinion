package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// syntheticCheckTargetPlanModifier predicts the server's own forced
// override so a plan never disagrees with what apply actually returns.
// `routes::customer_api::create_synthetic_check`/`update_synthetic_check`
// (main repo) always store `target` as the fixed placeholder
// "(multi-step)" when `check_type` is "http_multistep" — a multi-step
// check's real target is its own ordered steps, not this field —
// silently ignoring whatever the caller sent. Without this, a plan that
// omits `target` on an `http_multistep` check computes an empty-string
// default, then the server-applied "(multi-step)" value trips
// Terraform's "provider produced inconsistent result after apply" check.
func syntheticCheckTargetPlanModifier() planmodifier.String {
	return syntheticCheckTargetModifier{}
}

type syntheticCheckTargetModifier struct{}

func (m syntheticCheckTargetModifier) Description(_ context.Context) string {
	return `Predicts "(multi-step)" as the stored target when check_type is "http_multistep", matching the server's own forced override.`
}

func (m syntheticCheckTargetModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m syntheticCheckTargetModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	var checkType types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("check_type"), &checkType)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if checkType.ValueString() == "http_multistep" {
		resp.PlanValue = types.StringValue("(multi-step)")
	}
}
