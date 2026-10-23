package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccGroupFileIntegrityResource(t *testing.T) {
	groupID := testAccGroupID(t)
	check := func(enabled bool, scan int64) resource.TestCheckFunc {
		return func(_ *terraform.State) error {
			p, err := testAccAPIClient().GetGroupFileIntegrity(context.Background(), groupID)
			if err != nil {
				return err
			}
			if p.Enabled != enabled || p.ScanMinutes != scan {
				return fmt.Errorf("server has %+v, want enabled=%v scan=%d", *p, enabled, scan)
			}
			return nil
		}
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             check(false, 30),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_group_file_integrity" "test" {
  group_id      = %q
  paths         = ["/srv/app/config"]
  exclusions    = ["/etc/cups/**"]
  scan_minutes  = 10
  critical_push = true
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_group_file_integrity.test", "id", groupID),
					resource.TestCheckResourceAttr("rustinion_group_file_integrity.test", "enabled", "true"),
					resource.TestCheckResourceAttr("rustinion_group_file_integrity.test", "paths.0", "/srv/app/config"),
					check(true, 10),
				),
			},
			{
				ResourceName:      "rustinion_group_file_integrity.test",
				ImportState:       true,
				ImportStateId:     groupID,
				ImportStateVerify: true,
			},
		},
	})
}
