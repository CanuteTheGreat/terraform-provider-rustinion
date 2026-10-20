package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

// TestAccSloResource exercises create, read, import, and (via
// CheckDestroy) delete against a real running rustinion-web instance —
// see README.md's "Running acceptance tests". No update step: every
// attribute is RequiresReplace, since the API itself has no update
// endpoint for an SLO — see resource_slo.go's own doc comment.
func TestAccSloResource(t *testing.T) {
	groupID := testAccGroupID(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSloDestroy,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_slo" "test" {
  group_id           = %[1]q
  name               = "tf-acc fleet uptime"
  metric_type        = "checkin_freshness"
  target_percentage  = 99.9
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rustinion_slo.test", "id"),
					resource.TestCheckResourceAttr("rustinion_slo.test", "group_id", groupID),
					resource.TestCheckResourceAttr("rustinion_slo.test", "name", "tf-acc fleet uptime"),
					resource.TestCheckResourceAttr("rustinion_slo.test", "metric_type", "checkin_freshness"),
					resource.TestCheckResourceAttr("rustinion_slo.test", "target_percentage", "99.9"),
					resource.TestCheckResourceAttr("rustinion_slo.test", "window_days", "30"),
					resource.TestCheckResourceAttr("rustinion_slo.test", "freshness_minutes", "60"),
					resource.TestCheckResourceAttr("rustinion_slo.test", "enabled", "true"),
				),
			},
			{
				ResourceName:      "rustinion_slo.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccSloResource_customMetric proves a custom_metric SLO's own
// extra fields (custom_metric_name/comparator/good_threshold) round-
// trip correctly.
func TestAccSloResource_customMetric(t *testing.T) {
	groupID := testAccGroupID(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSloDestroy,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_slo" "custom" {
  group_id           = %[1]q
  name               = "tf-acc queue depth"
  metric_type        = "custom_metric"
  target_percentage  = 95
  custom_metric_name = "queue_depth"
  comparator         = "lte"
  good_threshold     = 100
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_slo.custom", "custom_metric_name", "queue_depth"),
					resource.TestCheckResourceAttr("rustinion_slo.custom", "comparator", "lte"),
					resource.TestCheckResourceAttr("rustinion_slo.custom", "good_threshold", "100"),
				),
			},
		},
	})
}

func testAccCheckSloDestroy(s *terraform.State) error {
	c := testAccAPIClient()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "rustinion_slo" {
			continue
		}
		_, err := c.GetSlo(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("slo %s still exists", rs.Primary.ID)
		}
		if !client.IsNotFound(err) {
			return err
		}
	}
	return nil
}
