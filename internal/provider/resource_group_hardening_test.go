package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccGroupHardeningResource(t *testing.T) {
	groupID := testAccGroupID(t)
	check := func(enabled bool, minScore int64) resource.TestCheckFunc {
		return func(_ *terraform.State) error {
			p, err := testAccAPIClient().GetGroupHardening(context.Background(), groupID)
			if err != nil {
				return err
			}
			got := int64(-1)
			if p.MinScore != nil {
				got = *p.MinScore
			}
			if p.Enabled != enabled || got != minScore {
				return fmt.Errorf("server has %+v, want enabled=%v min=%d", *p, enabled, minScore)
			}
			return nil
		}
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             check(false, -1),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_group_hardening" "test" {
  group_id       = %q
  interval_hours = 12
  min_score      = 80
  policies       = ["rustinion_debian_ubuntu_hardening"]
  accepted_risks = ["rustinion_debian_ubuntu_hardening | net-ip-forward | container host"]
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_group_hardening.test", "id", groupID),
					resource.TestCheckResourceAttr("rustinion_group_hardening.test", "min_score", "80"),
					check(true, 80),
				),
			},
			{
				ResourceName:      "rustinion_group_hardening.test",
				ImportState:       true,
				ImportStateId:     groupID,
				ImportStateVerify: true,
			},
		},
	})
}
