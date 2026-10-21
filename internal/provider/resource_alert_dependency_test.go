package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

// TestAccAlertDependencyResource: a device and a whole group depend on an
// uptime check (an uplink probe), create/read/import/delete against a
// real rustinion-web. No update step: every attribute forces a replace.
func TestAccAlertDependencyResource(t *testing.T) {
	groupID := testAccGroupID(t)
	deviceID := testAccDeviceID(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAlertDependencyDestroy,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_synthetic_check" "uplink" {
  group_id   = %[1]q
  name       = "tf-acc uplink"
  check_type = "tcp"
  target     = "127.0.0.1:9"
}

resource "rustinion_alert_dependency" "device" {
  device_id                 = %[2]q
  parent_synthetic_check_id = rustinion_synthetic_check.uplink.id
}

resource "rustinion_alert_dependency" "group" {
  group_id                  = %[1]q
  parent_synthetic_check_id = rustinion_synthetic_check.uplink.id
}
`, groupID, deviceID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rustinion_alert_dependency.device", "id"),
					resource.TestCheckResourceAttr("rustinion_alert_dependency.device", "device_id", deviceID),
					resource.TestCheckNoResourceAttr("rustinion_alert_dependency.device", "group_id"),
					resource.TestCheckResourceAttrPair("rustinion_alert_dependency.device", "parent_synthetic_check_id",
						"rustinion_synthetic_check.uplink", "id"),
					resource.TestCheckResourceAttr("rustinion_alert_dependency.group", "group_id", groupID),
					resource.TestCheckResourceAttrSet("rustinion_alert_dependency.group", "created_at"),
				),
			},
			{
				ResourceName:      "rustinion_alert_dependency.device",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccCheckAlertDependencyDestroy(s *terraform.State) error {
	c := testAccAPIClient()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "rustinion_alert_dependency" {
			continue
		}
		_, err := c.GetAlertDependency(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("alert dependency %s still exists after destroy", rs.Primary.ID)
		}
		if !client.IsNotFound(err) {
			return fmt.Errorf("unexpected error checking alert dependency %s: %w", rs.Primary.ID, err)
		}
	}
	return nil
}

// TestAccAlertRuleResource_noDataScoped: a no-data condition on a rule
// limited to one host tag round-trips (create, update of the scope,
// import).
func TestAccAlertRuleResource_noDataScoped(t *testing.T) {
	groupID := testAccGroupID(t)
	config := func(tag string) string {
		return fmt.Sprintf(`
resource "rustinion_alert_rule" "nodata" {
  group_id  = %[1]q
  name      = "tf-acc web hosts stopped reporting"
  scope_tag = %[2]q

  condition {
    metric_type    = "cpu_avg_pct"
    condition_kind = "no_data"
    comparator     = "gt"
    threshold      = 0
    window_minutes = 15
  }
}
`, groupID, tag)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAlertRuleDestroy,
		Steps: []resource.TestStep{
			{
				Config: config("role:web"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_alert_rule.nodata", "scope_tag", "role:web"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.nodata", "condition.0.condition_kind", "no_data"),
					resource.TestCheckResourceAttr("rustinion_alert_rule.nodata", "condition.0.window_minutes", "15"),
				),
			},
			{
				Config: config("env:ci"),
				Check:  resource.TestCheckResourceAttr("rustinion_alert_rule.nodata", "scope_tag", "env:ci"),
			},
			{
				ResourceName:      "rustinion_alert_rule.nodata",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
