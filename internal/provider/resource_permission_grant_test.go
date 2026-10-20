package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

// TestAccPermissionGrantResource exercises create, read, import, an
// in-place permission_level update, and (via CheckDestroy) delete
// against a real running rustinion-web instance — see README.md's
// "Running acceptance tests".
//
// Uses the tenant member scripts/setup_test_tenant.sh already
// provisions (RUSTINION_TEST_USER_ID) rather than creating one with
// rustinion_user. A grant needs a user who already holds a role in the
// tenant — the API refuses a grant for a non-member — and that fixture
// user is exactly that. Creating one for real would send a genuine
// password-set email, the same side effect setup_test_tenant.sh's own
// comment gives for why it provisions this user directly instead. The
// fixture id IS a users.id, which is what a grant is keyed on, so this
// covers the same wiring `rustinion_user.example.user_id` produces.
func TestAccPermissionGrantResource(t *testing.T) {
	groupID := testAccGroupID(t)
	userID := testAccUserID(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPermissionGrantDestroy,
		Steps: []resource.TestStep{
			// Create + Read, scoped to one group.
			{
				Config: fmt.Sprintf(`
resource "rustinion_permission_grant" "test" {
  user_id          = %[1]q
  group_id         = %[2]q
  resource_type    = "alert_rules"
  permission_level = "write"
}
`, userID, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rustinion_permission_grant.test", "id"),
					resource.TestCheckResourceAttr("rustinion_permission_grant.test", "user_id", userID),
					resource.TestCheckResourceAttr("rustinion_permission_grant.test", "group_id", groupID),
					resource.TestCheckResourceAttr("rustinion_permission_grant.test", "resource_type", "alert_rules"),
					resource.TestCheckResourceAttr("rustinion_permission_grant.test", "permission_level", "write"),
				),
			},
			// Import.
			{
				ResourceName:      "rustinion_permission_grant.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update in place: permission_level only. The id must
			// survive — the API upserts on the grant's own scope rather
			// than recreating, which is exactly what stops a level
			// change from briefly restoring unrestricted access.
			{
				Config: fmt.Sprintf(`
resource "rustinion_permission_grant" "test" {
  user_id          = %[1]q
  group_id         = %[2]q
  resource_type    = "alert_rules"
  permission_level = "read"
}
`, userID, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_permission_grant.test", "permission_level", "read"),
				),
			},
		},
	})
}

// TestAccPermissionGrantResourceTenantWide covers the other scope
// shape: group_id omitted entirely, meaning every group. Worth its own
// case because null-vs-empty-string is a real failure mode on this
// attribute — an empty string would reach the API as an invalid uuid.
// Uses a different resource_type from the group-scoped test above so
// the two can never collide on the server's own
// (user_id, group_id, resource_type) uniqueness if they interleave.
func TestAccPermissionGrantResourceTenantWide(t *testing.T) {
	userID := testAccUserID(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPermissionGrantDestroy,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_permission_grant" "test" {
  user_id          = %[1]q
  resource_type    = "notification_channels"
  permission_level = "read"
}
`, userID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rustinion_permission_grant.test", "id"),
					resource.TestCheckNoResourceAttr("rustinion_permission_grant.test", "group_id"),
					resource.TestCheckResourceAttr("rustinion_permission_grant.test", "resource_type", "notification_channels"),
					resource.TestCheckResourceAttr("rustinion_permission_grant.test", "permission_level", "read"),
				),
			},
			{
				ResourceName:      "rustinion_permission_grant.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccCheckPermissionGrantDestroy(s *terraform.State) error {
	c := testAccAPIClient()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "rustinion_permission_grant" {
			continue
		}
		_, err := c.GetPermissionGrant(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("permission grant %s still exists", rs.Primary.ID)
		}
		if !client.IsNotFound(err) {
			return err
		}
	}
	return nil
}
