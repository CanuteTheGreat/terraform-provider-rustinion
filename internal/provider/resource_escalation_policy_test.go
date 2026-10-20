package provider

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

// TestAccEscalationPolicyResource exercises create, read, update,
// import, and (via CheckDestroy) delete against a real running
// rustinion-web instance — see README.md's "Running acceptance tests".
func TestAccEscalationPolicyResource(t *testing.T) {
	groupID := testAccGroupID(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEscalationPolicyDestroy,
		Steps: []resource.TestStep{
			// Create + Read.
			{
				Config: fmt.Sprintf(`
resource "rustinion_notification_channel" "primary" {
  name         = "tf-acc primary"
  channel_type = "webhook"
  config       = jsonencode({ url = "https://example.com/hook" })
}

resource "rustinion_escalation_policy" "test" {
  group_id = %[1]q
  name     = "tf-acc primary escalation"

  step {
    channel_ids = [rustinion_notification_channel.primary.id]
  }
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rustinion_escalation_policy.test", "id"),
					resource.TestCheckResourceAttr("rustinion_escalation_policy.test", "group_id", groupID),
					resource.TestCheckResourceAttr("rustinion_escalation_policy.test", "name", "tf-acc primary escalation"),
					resource.TestCheckResourceAttr("rustinion_escalation_policy.test", "step.#", "1"),
					resource.TestCheckResourceAttr("rustinion_escalation_policy.test", "step.0.wait_minutes", "15"),
					resource.TestCheckResourceAttr("rustinion_escalation_policy.test", "step.0.channel_ids.#", "1"),
					resource.TestCheckResourceAttrPair(
						"rustinion_escalation_policy.test", "step.0.channel_ids.0",
						"rustinion_notification_channel.primary", "id",
					),
				),
			},
			// Import.
			{
				ResourceName:      "rustinion_escalation_policy.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update in place: rename, fan step 1 out to *two* channels,
			// add a second single-channel step — exercises both the
			// server's own full-replace PUT semantics for the nested
			// step list and real multi-channel fan-out on one step.
			{
				Config: fmt.Sprintf(`
resource "rustinion_notification_channel" "primary" {
  name         = "tf-acc primary"
  channel_type = "webhook"
  config       = jsonencode({ url = "https://example.com/hook" })
}

resource "rustinion_notification_channel" "secondary" {
  name         = "tf-acc secondary"
  channel_type = "webhook"
  config       = jsonencode({ url = "https://example.com/hook2" })
}

resource "rustinion_notification_channel" "tertiary" {
  name         = "tf-acc tertiary"
  channel_type = "webhook"
  config       = jsonencode({ url = "https://example.com/hook3" })
}

resource "rustinion_escalation_policy" "test" {
  group_id = %[1]q
  name     = "tf-acc primary escalation renamed"

  step {
    channel_ids  = [rustinion_notification_channel.primary.id, rustinion_notification_channel.secondary.id]
    wait_minutes = 5
  }

  step {
    channel_ids  = [rustinion_notification_channel.tertiary.id]
    wait_minutes = 30
  }
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_escalation_policy.test", "name", "tf-acc primary escalation renamed"),
					resource.TestCheckResourceAttr("rustinion_escalation_policy.test", "step.#", "2"),
					resource.TestCheckResourceAttr("rustinion_escalation_policy.test", "step.0.wait_minutes", "5"),
					resource.TestCheckResourceAttr("rustinion_escalation_policy.test", "step.0.channel_ids.#", "2"),
					resource.TestCheckResourceAttrPair(
						"rustinion_escalation_policy.test", "step.0.channel_ids.0",
						"rustinion_notification_channel.primary", "id",
					),
					resource.TestCheckResourceAttrPair(
						"rustinion_escalation_policy.test", "step.0.channel_ids.1",
						"rustinion_notification_channel.secondary", "id",
					),
					resource.TestCheckResourceAttr("rustinion_escalation_policy.test", "step.1.wait_minutes", "30"),
					resource.TestCheckResourceAttr("rustinion_escalation_policy.test", "step.1.channel_ids.#", "1"),
				),
			},
		},
	})
}

// TestAccEscalationPolicyResource_stepRequiresAChannel proves a step
// with an empty channel_ids list is rejected before it ever reaches the
// server — the schema's own listvalidator.SizeAtLeast(1) (mirroring
// routes::customer_api::validate_escalation_policy_input's identical
// server-side check) catches it at plan time.
func TestAccEscalationPolicyResource_stepRequiresAChannel(t *testing.T) {
	groupID := testAccGroupID(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_escalation_policy" "empty_step" {
  group_id = %[1]q
  name     = "tf-acc empty step"

  step {
    channel_ids = []
  }
}
`, groupID),
				ExpectError: regexp.MustCompile("at least 1 elements"),
			},
		},
	})
}

func testAccCheckEscalationPolicyDestroy(s *terraform.State) error {
	c := testAccAPIClient()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "rustinion_escalation_policy" {
			continue
		}
		_, err := c.GetEscalationPolicy(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("escalation policy %s still exists", rs.Primary.ID)
		}
		if !client.IsNotFound(err) {
			return err
		}
	}
	return nil
}
