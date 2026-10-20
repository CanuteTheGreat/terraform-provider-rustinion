package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

// TestAccIntegrationResource exercises create, read, update (config
// only, in place), import, and (via CheckDestroy) delete against a real
// running rustinion-web instance — see README.md's "Running acceptance
// tests". Uses "nginx" (a SyntheticTemplate-kind integration) since it
// populates synthetic_check_id, dashboard_id, and alert_rule_id all at
// once, giving the widest real coverage of the computed attributes in
// one test — see integrations.rs's own doc comment for what each kind
// creates.
func TestAccIntegrationResource(t *testing.T) {
	groupID := testAccGroupID(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckIntegrationDestroy,
		Steps: []resource.TestStep{
			// Create + Read.
			{
				Config: fmt.Sprintf(`
resource "rustinion_integration" "test" {
  kind     = "nginx"
  group_id = %[1]q
  config   = jsonencode({ target = "https://nginx.example.com/health" })
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rustinion_integration.test", "id"),
					resource.TestCheckResourceAttr("rustinion_integration.test", "kind", "nginx"),
					resource.TestCheckResourceAttr("rustinion_integration.test", "group_id", groupID),
					resource.TestCheckResourceAttrSet("rustinion_integration.test", "dashboard_id"),
					resource.TestCheckResourceAttrSet("rustinion_integration.test", "alert_rule_id"),
					resource.TestCheckResourceAttrSet("rustinion_integration.test", "synthetic_check_id"),
					resource.TestCheckNoResourceAttr("rustinion_integration.test", "notification_channel_id"),
				),
			},
			// Import.
			{
				ResourceName:      "rustinion_integration.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update: config only, in place — kind/group_id are
			// RequiresReplace and unchanged here, so this must not
			// produce a new id.
			{
				Config: fmt.Sprintf(`
resource "rustinion_integration" "test" {
  kind     = "nginx"
  group_id = %[1]q
  config   = jsonencode({ target = "https://nginx.example.com/health2" })
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrWith("rustinion_integration.test", "config", func(value string) error {
						if value != `{"target":"https://nginx.example.com/health2"}` {
							return fmt.Errorf("config = %s, want the updated target", value)
						}
						return nil
					}),
				),
			},
		},
	})
}

// TestAccIntegrationResource_noConfigKind proves a kind that needs no
// setup at all (SetupKind::AgentConfigSnippet — see integrations.rs)
// installs cleanly with config left at its schema default.
func TestAccIntegrationResource_noConfigKind(t *testing.T) {
	groupID := testAccGroupID(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckIntegrationDestroy,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_integration" "docker" {
  kind     = "docker"
  group_id = %[1]q
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_integration.docker", "config", "{}"),
					resource.TestCheckResourceAttrSet("rustinion_integration.docker", "dashboard_id"),
					resource.TestCheckResourceAttrSet("rustinion_integration.docker", "alert_rule_id"),
					resource.TestCheckNoResourceAttr("rustinion_integration.docker", "synthetic_check_id"),
					resource.TestCheckNoResourceAttr("rustinion_integration.docker", "notification_channel_id"),
				),
			},
		},
	})
}

func testAccCheckIntegrationDestroy(s *terraform.State) error {
	c := testAccAPIClient()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "rustinion_integration" {
			continue
		}
		_, err := c.GetIntegration(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("integration %s still exists", rs.Primary.ID)
		}
		if !client.IsNotFound(err) {
			return err
		}
	}
	return nil
}
