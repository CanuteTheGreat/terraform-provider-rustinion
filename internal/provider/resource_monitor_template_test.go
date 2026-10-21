package provider

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

// TestAccMonitorTemplate exercises a template inheriting from the
// built-in Linux pack (data source), its link to the test group, a
// device macro override, an in-place update that keeps the rule's id,
// import, and delete (CheckDestroy) against a real rustinion-web.
func TestAccMonitorTemplate(t *testing.T) {
	groupID := testAccGroupID(t)
	deviceID := testAccDeviceID(t)

	config := func(threshold, overrideValue string) string {
		return fmt.Sprintf(`
data "rustinion_monitor_template" "linux" {
  builtin_key = "linux"
}

resource "rustinion_monitor_template" "test" {
  name        = "tf-acc data disks"
  description = "data volumes"
  parent_ids  = [data.rustinion_monitor_template.linux.id]

  macro {
    macro = "{$DISK_CRIT}"
    value = %[3]q
  }
  macro {
    macro   = "{$DISK_CRIT}"
    context = "/srv"
    value   = "80"
  }

  rule {
    name       = "Data disk full"
    severity   = "critical"
    item_key   = "vfs.fs.size[/data,pused]"
    comparator = "gt"
    threshold  = "{$DISK_CRIT}"
  }
  rule {
    name             = "Average CPU"
    func             = "avg"
    window_minutes   = 15
    item_key         = "system.cpu.util"
    comparator       = "gte"
    threshold        = "95"
    breaches_to_fire = 3
    defer_to_builtin = "cpu_high"
  }
}

resource "rustinion_monitor_template_link" "group" {
  template_id = rustinion_monitor_template.test.id
  group_id    = %[1]q
}

resource "rustinion_monitor_macro_override" "device" {
  macro     = "{$DISK_CRIT}"
  value     = %[4]q
  device_id = %[2]q
}
`, groupID, deviceID, threshold, overrideValue)
	}

	var ruleID string
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMonitorTemplateDestroy,
		Steps: []resource.TestStep{
			{
				Config: config("90", "99"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.rustinion_monitor_template.linux", "name", "Linux server"),
					resource.TestCheckResourceAttr("data.rustinion_monitor_template.linux", "built_in", "true"),
					resource.TestCheckResourceAttrSet("rustinion_monitor_template.test", "id"),
					resource.TestCheckResourceAttr("rustinion_monitor_template.test", "rule.#", "2"),
					resource.TestCheckResourceAttr("rustinion_monitor_template.test", "rule.0.func", "last"),
					resource.TestCheckResourceAttr("rustinion_monitor_template.test", "rule.1.defer_to_builtin", "cpu_high"),
					resource.TestCheckResourceAttr("rustinion_monitor_template.test", "macro.1.context", "/srv"),
					resource.TestCheckResourceAttr("rustinion_monitor_template.test", "parent_ids.#", "1"),
					resource.TestCheckResourceAttr("rustinion_monitor_template_link.group", "group_id", groupID),
					resource.TestCheckResourceAttr("rustinion_monitor_macro_override.device", "value", "99"),
					func(s *terraform.State) error {
						id := s.RootModule().Resources["rustinion_monitor_template.test"].Primary.ID
						tpl, err := testAccAPIClient().GetMonitorTemplate(context.Background(), id)
						if err != nil {
							return err
						}
						ruleID = *tpl.Rules[0].ID
						return nil
					},
				),
			},
			{
				ResourceName:      "rustinion_monitor_template.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				ResourceName:      "rustinion_monitor_template_link.group",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				ResourceName:      "rustinion_monitor_macro_override.device",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update in place: the macro default and the override value
			// change; the rule keeps its id (and so its device state).
			{
				Config: config("85", "97"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_monitor_template.test", "macro.0.value", "85"),
					resource.TestCheckResourceAttr("rustinion_monitor_macro_override.device", "value", "97"),
					func(s *terraform.State) error {
						id := s.RootModule().Resources["rustinion_monitor_template.test"].Primary.ID
						tpl, err := testAccAPIClient().GetMonitorTemplate(context.Background(), id)
						if err != nil {
							return err
						}
						if got := *tpl.Rules[0].ID; got != ruleID {
							return fmt.Errorf("rule id changed on update: %s -> %s", ruleID, got)
						}
						return nil
					},
				),
			},
		},
	})
}

func testAccCheckMonitorTemplateDestroy(s *terraform.State) error {
	c := testAccAPIClient()
	for name, rs := range s.RootModule().Resources {
		// The built-in pack read through the data source has the same
		// type and of course outlives the test.
		if strings.HasPrefix(name, "data.") {
			continue
		}
		var err error
		switch rs.Type {
		case "rustinion_monitor_template":
			_, err = c.GetMonitorTemplate(context.Background(), rs.Primary.ID)
		case "rustinion_monitor_template_link":
			_, err = c.GetMonitorTemplateLink(context.Background(), rs.Primary.ID)
		case "rustinion_monitor_macro_override":
			_, err = c.GetMonitorMacroOverride(context.Background(), rs.Primary.ID)
		default:
			continue
		}
		if err == nil {
			return fmt.Errorf("%s %s still exists", rs.Type, rs.Primary.ID)
		}
		if !client.IsNotFound(err) {
			return err
		}
	}
	return nil
}

// TestAccAlertRuleMultiAlert: a per-device (multi-alert) alert rule.
func TestAccAlertRuleMultiAlert(t *testing.T) {
	groupID := testAccGroupID(t)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAlertRuleDestroy,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_alert_rule" "per_host" {
  group_id    = %[1]q
  name        = "tf-acc per-host CPU"
  multi_alert = true
  condition {
    metric_type = "cpu_avg_pct"
    comparator  = "gt"
    threshold   = 90
  }
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_alert_rule.per_host", "multi_alert", "true"),
				),
			},
			{
				ResourceName:      "rustinion_alert_rule.per_host",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
