package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccEscalationAcknowledgementResource_noActiveEscalation proves
// the real, deterministic error path this resource can be tested for
// without direct database access: applying it against a real alert
// rule that has never fired (so the server genuinely has no active
// escalation_runs row for it) surfaces a clear Terraform apply error,
// not a silent no-op success. See this resource's own Schema doc
// comment — its "did acknowledging actually work" success path (a rule
// with a real, currently-firing escalation) can't be manufactured from
// an acceptance test's own credentials (a tenant-scoped API key, not
// the device-secret-authenticated metrics ingestion path a rule needs
// real data flowing through to ever fire) — that path is proven for
// real instead by the Rust integration test
// acknowledge_escalation_halts_further_steps_via_the_real_endpoint in
// web/src/routes/customer_api.rs, which has full database access to
// fabricate a firing rule and a real escalation run.
func TestAccEscalationAcknowledgementResource_noActiveEscalation(t *testing.T) {
	groupID := testAccGroupID(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_alert_rule" "never_fires" {
  group_id = %[1]q
  name     = "tf-acc never fires"

  condition {
    metric_type = "cpu_avg_pct"
    comparator  = "gt"
    threshold   = 90
  }
}

resource "rustinion_escalation_acknowledgement" "test" {
  rule_kind = "alert_rule"
  rule_id   = rustinion_alert_rule.never_fires.id
}
`, groupID),
				ExpectError: regexp.MustCompile("No active escalation to acknowledge"),
			},
		},
	})
}

// TestAccEscalationAcknowledgementResource_unknownRule proves the
// "rule not found" error path — acknowledging a rule_id that was never
// created (or belongs to another tenant) is a real, distinct failure
// from "no active escalation."
func TestAccEscalationAcknowledgementResource_unknownRule(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "rustinion_escalation_acknowledgement" "test" {
  rule_kind = "alert_rule"
  rule_id   = "00000000-0000-0000-0000-000000000000"
}
`,
				ExpectError: regexp.MustCompile("Rule not found"),
			},
		},
	})
}

// TestAccEscalationAcknowledgementResource_rejectsAnUnknownRuleKind
// proves the schema's own stringvalidator.OneOf catches a bad
// rule_kind at plan time, before any request is sent.
func TestAccEscalationAcknowledgementResource_rejectsAnUnknownRuleKind(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "rustinion_escalation_acknowledgement" "test" {
  rule_kind = "not_a_real_kind"
  rule_id   = "00000000-0000-0000-0000-000000000000"
}
`,
				ExpectError: regexp.MustCompile("(?i)value must be one of"),
			},
		},
	})
}
