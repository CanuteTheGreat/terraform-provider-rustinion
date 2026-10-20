package provider

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

// TestAccUserResource exercises create, read, import, an in-place
// access_level update, and (via CheckDestroy) delete against a real
// running rustinion-web instance — see README.md's "Running acceptance
// tests". Uses a time-based unique email each run since `users.email`
// is unique across the whole database, not just this test's own
// tenant, and this suite runs repeatedly against a persistent dev
// Postgres.
func TestAccUserResource(t *testing.T) {
	groupID := testAccGroupID(t)
	email := fmt.Sprintf("tf-acc-%d@example.com", time.Now().UnixNano())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckUserDestroy,
		Steps: []resource.TestStep{
			// Create + Read.
			{
				Config: fmt.Sprintf(`
resource "rustinion_user" "test" {
  email    = %[1]q
  group_id = %[2]q
}
`, email, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rustinion_user.test", "id"),
					resource.TestCheckResourceAttrSet("rustinion_user.test", "user_id"),
					resource.TestCheckResourceAttr("rustinion_user.test", "email", email),
					resource.TestCheckResourceAttr("rustinion_user.test", "group_id", groupID),
					resource.TestCheckResourceAttr("rustinion_user.test", "access_level", "admin"),
				),
			},
			// Import.
			{
				ResourceName:      "rustinion_user.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update in place: access_level only.
			{
				Config: fmt.Sprintf(`
resource "rustinion_user" "test" {
  email        = %[1]q
  group_id     = %[2]q
  access_level = "read_only"
}
`, email, groupID),
				Check: resource.TestCheckResourceAttr("rustinion_user.test", "access_level", "read_only"),
			},
		},
	})
}

func testAccCheckUserDestroy(s *terraform.State) error {
	c := testAccAPIClient()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "rustinion_user" {
			continue
		}
		_, err := c.GetUser(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("user %s still exists", rs.Primary.ID)
		}
		if !client.IsNotFound(err) {
			return err
		}
	}
	return nil
}
