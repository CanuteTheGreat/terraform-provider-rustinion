package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccGroupDetectionResource(t *testing.T) {
	groupID := testAccGroupID(t)
	check := func(enabled bool, level string) resource.TestCheckFunc {
		return func(_ *terraform.State) error {
			p, err := testAccAPIClient().GetGroupDetection(context.Background(), groupID)
			if err != nil {
				return err
			}
			if p.Enabled != enabled || p.MinLevel != level {
				return fmt.Errorf("server has %+v, want enabled=%v min_level=%s", *p, enabled, level)
			}
			return nil
		}
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             check(false, "medium"),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_group_detection" "test" {
  group_id        = %q
  min_level       = "high"
  disabled_rules  = ["rustinion-web-not-found"]
  rule_thresholds = { "rustinion-ssh-brute-force" = 8 }
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_group_detection.test", "id", groupID),
					resource.TestCheckResourceAttr("rustinion_group_detection.test", "enabled", "true"),
					resource.TestCheckResourceAttr("rustinion_group_detection.test", "disabled_rules.0", "rustinion-web-not-found"),
					check(true, "high"),
				),
			},
			{
				ResourceName:      "rustinion_group_detection.test",
				ImportState:       true,
				ImportStateId:     groupID,
				ImportStateVerify: true,
			},
		},
	})
}
