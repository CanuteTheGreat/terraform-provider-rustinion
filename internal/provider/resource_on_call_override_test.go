package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

// TestAccOnCallOverrideResource exercises create, read, import, a
// forced replace (every attribute is RequiresReplace — there is no
// in-place update, matching the server's own create/delete-only API
// for overrides), and (via CheckDestroy) delete, against a real running
// rustinion-web instance — see README.md's "Running acceptance tests".
func TestAccOnCallOverrideResource(t *testing.T) {
	groupID := testAccGroupID(t)
	userID := testAccUserID(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckOnCallOverrideDestroy,
		Steps: []resource.TestStep{
			// Create + Read.
			{
				Config: fmt.Sprintf(`
resource "rustinion_on_call_schedule" "test" {
  group_id = %[1]q
  name     = "tf-acc override rotation"
}

resource "rustinion_on_call_override" "test" {
  schedule_id = rustinion_on_call_schedule.test.id
  user_id     = %[2]q
  starts_at   = "2030-01-01T00:00:00Z"
  ends_at     = "2030-01-02T00:00:00Z"
}
`, groupID, userID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rustinion_on_call_override.test", "id"),
					resource.TestCheckResourceAttrPair(
						"rustinion_on_call_override.test", "schedule_id",
						"rustinion_on_call_schedule.test", "id",
					),
					resource.TestCheckResourceAttr("rustinion_on_call_override.test", "user_id", userID),
					resource.TestCheckResourceAttr("rustinion_on_call_override.test", "starts_at", "2030-01-01T00:00:00Z"),
					resource.TestCheckResourceAttr("rustinion_on_call_override.test", "ends_at", "2030-01-02T00:00:00Z"),
				),
			},
			// Import — "schedule_id/override_id", the one resource in
			// this provider whose import id isn't just the bare
			// server-assigned id (see this resource's own ImportState
			// doc comment for why).
			{
				ResourceName: "rustinion_on_call_override.test",
				ImportState:  true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs, ok := s.RootModule().Resources["rustinion_on_call_override.test"]
					if !ok {
						return "", fmt.Errorf("resource not found in state")
					}
					return rs.Primary.Attributes["schedule_id"] + "/" + rs.Primary.ID, nil
				},
				ImportStateVerify: true,
			},
			// Changing ends_at forces a real replace (every attribute is
			// RequiresReplace) — proves this resource never attempts an
			// in-place update against an endpoint that doesn't support one.
			{
				Config: fmt.Sprintf(`
resource "rustinion_on_call_schedule" "test" {
  group_id = %[1]q
  name     = "tf-acc override rotation"
}

resource "rustinion_on_call_override" "test" {
  schedule_id = rustinion_on_call_schedule.test.id
  user_id     = %[2]q
  starts_at   = "2030-01-01T00:00:00Z"
  ends_at     = "2030-01-03T00:00:00Z"
}
`, groupID, userID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_on_call_override.test", "ends_at", "2030-01-03T00:00:00Z"),
				),
			},
		},
	})
}

func testAccCheckOnCallOverrideDestroy(s *terraform.State) error {
	c := testAccAPIClient()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "rustinion_on_call_override" {
			continue
		}
		scheduleID := rs.Primary.Attributes["schedule_id"]
		_, err := c.GetOnCallOverride(context.Background(), scheduleID, rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("on-call override %s still exists", rs.Primary.ID)
		}
		if !client.IsNotFound(err) {
			return err
		}
	}
	return nil
}
