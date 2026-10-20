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

// TestAccPermissionRoleMappingResource exercises create, read, import
// and (via CheckDestroy) delete of an IdP group → role mapping against
// a real running rustinion-web, alongside the role it points at.
func TestAccPermissionRoleMappingResource(t *testing.T) {
	groupID := testAccGroupID(t)
	name := fmt.Sprintf("tf-acc-role-map-%d", time.Now().UnixNano())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPermissionRoleMappingDestroy,
		Steps: []resource.TestStep{
			// Tenant-wide OIDC mapping.
			{
				Config: fmt.Sprintf(`
resource "rustinion_permission_role" "test" {
  name = %[1]q

  permission {
    resource_type    = "alert_rules"
    permission_level = "write"
  }
}

resource "rustinion_permission_role_mapping" "test" {
  source         = "oidc"
  external_group = "sre"
  role_id        = rustinion_permission_role.test.id
}
`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rustinion_permission_role_mapping.test", "id"),
					resource.TestCheckResourceAttr("rustinion_permission_role_mapping.test", "source", "oidc"),
					resource.TestCheckResourceAttr("rustinion_permission_role_mapping.test", "external_group", "sre"),
					resource.TestCheckResourceAttrPair(
						"rustinion_permission_role_mapping.test", "role_id",
						"rustinion_permission_role.test", "id",
					),
					resource.TestCheckNoResourceAttr("rustinion_permission_role_mapping.test", "group_id"),
				),
			},
			{
				ResourceName:      "rustinion_permission_role_mapping.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Changing source forces replacement (every attribute does);
			// this also covers the group-scoped shape and the third
			// source, SAML.
			{
				Config: fmt.Sprintf(`
resource "rustinion_permission_role" "test" {
  name = %[1]q

  permission {
    resource_type    = "alert_rules"
    permission_level = "write"
  }
}

resource "rustinion_permission_role_mapping" "test" {
  source         = "saml"
  external_group = "sre"
  role_id        = rustinion_permission_role.test.id
  group_id       = %[2]q
}
`, name, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_permission_role_mapping.test", "source", "saml"),
					resource.TestCheckResourceAttr("rustinion_permission_role_mapping.test", "group_id", groupID),
				),
			},
		},
	})
}

func testAccCheckPermissionRoleMappingDestroy(s *terraform.State) error {
	c := testAccAPIClient()
	for _, rs := range s.RootModule().Resources {
		switch rs.Type {
		case "rustinion_permission_role_mapping":
			_, err := c.GetPermissionRoleMapping(context.Background(), rs.Primary.ID)
			if err == nil {
				return fmt.Errorf("permission role mapping %s still exists", rs.Primary.ID)
			}
			if !client.IsNotFound(err) {
				return err
			}
		case "rustinion_permission_role":
			_, err := c.GetPermissionRole(context.Background(), rs.Primary.ID)
			if err == nil {
				return fmt.Errorf("permission role %s still exists", rs.Primary.ID)
			}
			if !client.IsNotFound(err) {
				return err
			}
		}
	}
	return nil
}
