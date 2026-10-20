package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

// TestAccOnCallScheduleResource exercises create, read, update, import,
// and (via CheckDestroy) delete against a real running rustinion-web
// instance — see README.md's "Running acceptance tests".
func TestAccOnCallScheduleResource(t *testing.T) {
	groupID := testAccGroupID(t)
	userID := testAccUserID(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckOnCallScheduleDestroy,
		Steps: []resource.TestStep{
			// Create + Read.
			{
				Config: fmt.Sprintf(`
resource "rustinion_on_call_schedule" "test" {
  group_id = %[1]q
  name     = "tf-acc weekly rotation"

  member {
    user_id = %[2]q
  }
}
`, groupID, userID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rustinion_on_call_schedule.test", "id"),
					resource.TestCheckResourceAttrSet("rustinion_on_call_schedule.test", "started_at"),
					resource.TestCheckResourceAttr("rustinion_on_call_schedule.test", "group_id", groupID),
					resource.TestCheckResourceAttr("rustinion_on_call_schedule.test", "name", "tf-acc weekly rotation"),
					resource.TestCheckResourceAttr("rustinion_on_call_schedule.test", "rotation_days", "7"),
					resource.TestCheckResourceAttr("rustinion_on_call_schedule.test", "member.#", "1"),
					resource.TestCheckResourceAttr("rustinion_on_call_schedule.test", "member.0.user_id", userID),
				),
			},
			// Import.
			{
				ResourceName:      "rustinion_on_call_schedule.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update in place: rename, change rotation_days. Members stay
			// the same (only one real user is provisioned for this test
			// tenant) but the replace-wholesale path is already covered
			// by this provider's own unit-level equivalent
			// (update_on_call_schedule_replaces_members_wholesale_...
			// in the main repo's customer_api.rs tests).
			{
				Config: fmt.Sprintf(`
resource "rustinion_on_call_schedule" "test" {
  group_id      = %[1]q
  name          = "tf-acc weekly rotation renamed"
  rotation_days = 14

  member {
    user_id = %[2]q
  }
}
`, groupID, userID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_on_call_schedule.test", "name", "tf-acc weekly rotation renamed"),
					resource.TestCheckResourceAttr("rustinion_on_call_schedule.test", "rotation_days", "14"),
				),
			},
		},
	})
}

func testAccCheckOnCallScheduleDestroy(s *terraform.State) error {
	c := testAccAPIClient()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "rustinion_on_call_schedule" {
			continue
		}
		_, err := c.GetOnCallSchedule(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("on-call schedule %s still exists", rs.Primary.ID)
		}
		if !client.IsNotFound(err) {
			return err
		}
	}
	return nil
}
