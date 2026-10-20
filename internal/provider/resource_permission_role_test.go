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

// TestAccPermissionRoleResource exercises create, read, import, an
// in-place update that both renames the role and fully replaces its
// permission set, and (via CheckDestroy) delete against a real running
// rustinion-web — see README.md's "Running acceptance tests".
//
// Role names are unique per tenant case-insensitively and this suite
// runs repeatedly against a persistent dev Postgres, so the name is
// made unique per run the same way TestAccUserResource handles its own
// database-wide unique email.
func TestAccPermissionRoleResource(t *testing.T) {
	name := fmt.Sprintf("tf-acc-role-%d", time.Now().UnixNano())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPermissionRoleDestroy,
		Steps: []resource.TestStep{
			// Create + Read, with two permissions.
			{
				Config: fmt.Sprintf(`
resource "rustinion_permission_role" "test" {
  name        = %[1]q
  description = "created by acceptance test"

  permission {
    resource_type    = "alert_rules"
    permission_level = "admin"
  }

  permission {
    resource_type    = "dashboards"
    permission_level = "read"
  }
}
`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rustinion_permission_role.test", "id"),
					resource.TestCheckResourceAttr("rustinion_permission_role.test", "name", name),
					resource.TestCheckResourceAttr("rustinion_permission_role.test", "description", "created by acceptance test"),
					resource.TestCheckResourceAttr("rustinion_permission_role.test", "permission.#", "2"),
					resource.TestCheckResourceAttr("rustinion_permission_role.test", "permission.0.resource_type", "alert_rules"),
					resource.TestCheckResourceAttr("rustinion_permission_role.test", "permission.0.permission_level", "admin"),
				),
			},
			// Import.
			{
				ResourceName:      "rustinion_permission_role.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update in place: rename, and fully replace the permission
			// set with a single different entry. PUT is a full replace,
			// so both original permissions must be gone afterwards.
			{
				Config: fmt.Sprintf(`
resource "rustinion_permission_role" "test" {
  name = "%[1]s-renamed"

  permission {
    resource_type    = "synthetic_checks"
    permission_level = "write"
  }
}
`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_permission_role.test", "name", name+"-renamed"),
					resource.TestCheckNoResourceAttr("rustinion_permission_role.test", "description"),
					resource.TestCheckResourceAttr("rustinion_permission_role.test", "permission.#", "1"),
					resource.TestCheckResourceAttr("rustinion_permission_role.test", "permission.0.resource_type", "synthetic_checks"),
				),
			},
		},
	})
}

// TestAccPermissionRoleResourceNoPermissions covers the legal-but-easy-
// to-get-wrong shape: a role with no permission blocks at all. It must
// round trip as an empty set rather than being treated as "leave
// unchanged".
func TestAccPermissionRoleResourceNoPermissions(t *testing.T) {
	name := fmt.Sprintf("tf-acc-role-empty-%d", time.Now().UnixNano())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPermissionRoleDestroy,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_permission_role" "test" {
  name = %[1]q
}
`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rustinion_permission_role.test", "id"),
					resource.TestCheckResourceAttr("rustinion_permission_role.test", "permission.#", "0"),
				),
			},
			{
				ResourceName:      "rustinion_permission_role.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccPermissionRoleAssignmentResource exercises assigning a role to
// the tenant member setup_test_tenant.sh provisions, importing it with
// its role_id/assignment_id composite id, and destroying it.
func TestAccPermissionRoleAssignmentResource(t *testing.T) {
	userID := testAccUserID(t)
	name := fmt.Sprintf("tf-acc-role-assign-%d", time.Now().UnixNano())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPermissionRoleDestroy,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_permission_role" "test" {
  name = %[1]q

  permission {
    resource_type    = "alert_rules"
    permission_level = "write"
  }
}

resource "rustinion_permission_role_assignment" "test" {
  role_id = rustinion_permission_role.test.id
  user_id = %[2]q
}
`, name, userID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rustinion_permission_role_assignment.test", "id"),
					resource.TestCheckResourceAttrPair(
						"rustinion_permission_role_assignment.test", "role_id",
						"rustinion_permission_role.test", "id",
					),
					resource.TestCheckResourceAttr("rustinion_permission_role_assignment.test", "user_id", userID),
					resource.TestCheckNoResourceAttr("rustinion_permission_role_assignment.test", "group_id"),
				),
			},
			{
				ResourceName:      "rustinion_permission_role_assignment.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs, ok := s.RootModule().Resources["rustinion_permission_role_assignment.test"]
					if !ok {
						return "", fmt.Errorf("resource not found in state")
					}
					return rs.Primary.Attributes["role_id"] + "/" + rs.Primary.ID, nil
				},
			},
		},
	})
}

func testAccCheckPermissionRoleDestroy(s *terraform.State) error {
	c := testAccAPIClient()
	for _, rs := range s.RootModule().Resources {
		switch rs.Type {
		case "rustinion_permission_role":
			_, err := c.GetPermissionRole(context.Background(), rs.Primary.ID)
			if err == nil {
				return fmt.Errorf("permission role %s still exists", rs.Primary.ID)
			}
			if !client.IsNotFound(err) {
				return err
			}
		case "rustinion_permission_role_assignment":
			// The parent role is destroyed in the same sweep and
			// cascades its assignments away, so a 404 on either the
			// role or the assignment is the expected outcome here.
			_, err := c.GetRoleAssignment(context.Background(), rs.Primary.Attributes["role_id"], rs.Primary.ID)
			if err == nil {
				return fmt.Errorf("permission role assignment %s still exists", rs.Primary.ID)
			}
			if !client.IsNotFound(err) {
				return err
			}
		}
	}
	return nil
}
