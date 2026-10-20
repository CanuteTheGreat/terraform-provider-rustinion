package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

// TestAccLogAlertRuleResource exercises create, read, update, import,
// and (via CheckDestroy) delete against a real running rustinion-web
// instance — see README.md's "Running acceptance tests". Skipped unless
// TF_ACC=1 is set, same convention as every other TestAcc* here.
func TestAccLogAlertRuleResource(t *testing.T) {
	groupID := testAccGroupID(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckLogAlertRuleDestroy,
		Steps: []resource.TestStep{
			// Create + Read.
			{
				Config: fmt.Sprintf(`
resource "rustinion_log_alert_rule" "test" {
  group_id     = %[1]q
  name         = "tf-acc ssh failures"
  log_severity = "error"
  log_source   = "journal"
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rustinion_log_alert_rule.test", "id"),
					resource.TestCheckResourceAttr("rustinion_log_alert_rule.test", "group_id", groupID),
					resource.TestCheckResourceAttr("rustinion_log_alert_rule.test", "name", "tf-acc ssh failures"),
					resource.TestCheckResourceAttr("rustinion_log_alert_rule.test", "log_severity", "error"),
					resource.TestCheckResourceAttr("rustinion_log_alert_rule.test", "log_source", "journal"),
					resource.TestCheckResourceAttr("rustinion_log_alert_rule.test", "severity", "critical"),
					resource.TestCheckResourceAttr("rustinion_log_alert_rule.test", "threshold_count", "1"),
					resource.TestCheckResourceAttr("rustinion_log_alert_rule.test", "window_minutes", "5"),
					resource.TestCheckResourceAttr("rustinion_log_alert_rule.test", "enabled", "true"),
				),
			},
			// Import.
			{
				ResourceName:      "rustinion_log_alert_rule.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update in place: rename, widen the window, change severity,
			// add a message_pattern filter.
			{
				Config: fmt.Sprintf(`
resource "rustinion_log_alert_rule" "test" {
  group_id        = %[1]q
  name            = "tf-acc ssh failures renamed"
  message_pattern = "Failed password"
  log_severity    = "warning"
  severity        = "warning"
  threshold_count = 3
  window_minutes  = 15
  enabled         = false
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_log_alert_rule.test", "name", "tf-acc ssh failures renamed"),
					resource.TestCheckResourceAttr("rustinion_log_alert_rule.test", "message_pattern", "Failed password"),
					resource.TestCheckResourceAttr("rustinion_log_alert_rule.test", "log_severity", "warning"),
					resource.TestCheckNoResourceAttr("rustinion_log_alert_rule.test", "log_source"),
					resource.TestCheckResourceAttr("rustinion_log_alert_rule.test", "severity", "warning"),
					resource.TestCheckResourceAttr("rustinion_log_alert_rule.test", "threshold_count", "3"),
					resource.TestCheckResourceAttr("rustinion_log_alert_rule.test", "window_minutes", "15"),
					resource.TestCheckResourceAttr("rustinion_log_alert_rule.test", "enabled", "false"),
				),
			},
		},
	})
}

func testAccCheckLogAlertRuleDestroy(s *terraform.State) error {
	c := testAccAPIClient()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "rustinion_log_alert_rule" {
			continue
		}
		_, err := c.GetLogAlertRule(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("log alert rule %s still exists", rs.Primary.ID)
		}
		if !client.IsNotFound(err) {
			return err
		}
	}
	return nil
}
