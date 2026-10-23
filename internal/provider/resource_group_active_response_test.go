package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccGroupActiveResponseResource(t *testing.T) {
	groupID := testAccGroupID(t)
	check := func(enabled bool, minutes int64) resource.TestCheckFunc {
		return func(_ *terraform.State) error {
			p, err := testAccAPIClient().GetGroupActiveResponse(context.Background(), groupID)
			if err != nil {
				return err
			}
			if p.Enabled != enabled || p.BlockMinutes != minutes || !p.DryRun {
				return fmt.Errorf("server has %+v, want enabled=%v block_minutes=%d dry_run=true", *p, enabled, minutes)
			}
			return nil
		}
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             check(false, 60),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_group_active_response" "test" {
  group_id      = %q
  block_minutes = 15
  allowlist     = ["203.0.113.0/24"]
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_group_active_response.test", "dry_run", "true"),
					resource.TestCheckResourceAttr("rustinion_group_active_response.test", "allowlist.0", "203.0.113.0/24"),
					check(true, 15),
				),
			},
			{
				ResourceName:      "rustinion_group_active_response.test",
				ImportState:       true,
				ImportStateId:     groupID,
				ImportStateVerify: true,
			},
		},
	})
}
