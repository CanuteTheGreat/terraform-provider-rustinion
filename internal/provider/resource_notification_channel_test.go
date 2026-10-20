package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

func TestAccNotificationChannelResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNotificationChannelDestroy,
		Steps: []resource.TestStep{
			// Create + Read: a Slack channel.
			{
				Config: `
resource "rustinion_notification_channel" "test" {
  name         = "tf-acc ops-slack"
  channel_type = "slack"
  config       = jsonencode({ webhook_url = "https://hooks.slack.com/services/tf-acc" })
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rustinion_notification_channel.test", "id"),
					resource.TestCheckResourceAttr("rustinion_notification_channel.test", "name", "tf-acc ops-slack"),
					resource.TestCheckResourceAttr("rustinion_notification_channel.test", "channel_type", "slack"),
					resource.TestCheckResourceAttr("rustinion_notification_channel.test", "enabled", "true"),
					resource.TestCheckResourceAttr("rustinion_notification_channel.test", "grouping_enabled", "false"),
					resource.TestCheckResourceAttr("rustinion_notification_channel.test", "grouping_dimension", "rule"),
				),
			},
			// Import.
			{
				ResourceName:      "rustinion_notification_channel.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update: switch channel_type entirely (webhook instead of
			// Slack — a real config-shape change, not just a field
			// tweak) and turn on grouping.
			{
				Config: `
resource "rustinion_notification_channel" "test" {
  name             = "tf-acc ops-webhook"
  channel_type     = "webhook"
  config           = jsonencode({ url = "https://example.com/hook", secret = "s3cr3t" })
  grouping_enabled = true
  grouping_dimension = "device"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_notification_channel.test", "name", "tf-acc ops-webhook"),
					resource.TestCheckResourceAttr("rustinion_notification_channel.test", "channel_type", "webhook"),
					resource.TestCheckResourceAttr("rustinion_notification_channel.test", "grouping_enabled", "true"),
					resource.TestCheckResourceAttr("rustinion_notification_channel.test", "grouping_dimension", "device"),
				),
			},
			// Switch again to an sms channel — a real Twilio-backed
			// channel_type, not just the original five.
			{
				Config: `
resource "rustinion_notification_channel" "test" {
  name         = "tf-acc ops-sms"
  channel_type = "sms"
  config = jsonencode({
    account_sid = "ACtfacc0000000000000000000000000"
    auth_token  = "tf-acc-token"
    from_number = "+15551234567"
    to_numbers  = ["+15557654321"]
  })
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_notification_channel.test", "name", "tf-acc ops-sms"),
					resource.TestCheckResourceAttr("rustinion_notification_channel.test", "channel_type", "sms"),
				),
			},
		},
	})
}

func testAccCheckNotificationChannelDestroy(s *terraform.State) error {
	c := testAccAPIClient()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "rustinion_notification_channel" {
			continue
		}
		_, err := c.GetNotificationChannel(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("notification channel %s still exists", rs.Primary.ID)
		}
		if !client.IsNotFound(err) {
			return err
		}
	}
	return nil
}

// TestAccNotificationChannelTemplates covers the three custom-content
// fields end to end: set them, confirm they round-trip through import,
// then clear them and confirm the channel goes back to the built-in
// format rather than keeping a stale template.
func TestAccNotificationChannelTemplates(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNotificationChannelDestroy,
		Steps: []resource.TestStep{
			// A webhook channel carrying all three templates.
			{
				Config: `
resource "rustinion_notification_channel" "tpl" {
  name             = "tf-acc templated"
  channel_type     = "webhook"
  config           = jsonencode({ url = "https://example.com/tf-acc-hook" })
  subject_template = "[{{severity}}] {{reason}}"
  body_template    = "{{reason}} on {{hostname}} in {{group}}"
  payload_template = "{\"alert\": \"{{reason}}\", \"host\": \"{{hostname}}\"}"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_notification_channel.tpl", "subject_template", "[{{severity}}] {{reason}}"),
					resource.TestCheckResourceAttr("rustinion_notification_channel.tpl", "body_template", "{{reason}} on {{hostname}} in {{group}}"),
					resource.TestCheckResourceAttr("rustinion_notification_channel.tpl", "payload_template", `{"alert": "{{reason}}", "host": "{{hostname}}"}`),
				),
			},
			{
				ResourceName:      "rustinion_notification_channel.tpl",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Clearing them must really clear them — the server stores
			// NULL for an omitted template, and a stale one left behind
			// would keep formatting alerts a way the config no longer
			// asks for.
			{
				Config: `
resource "rustinion_notification_channel" "tpl" {
  name         = "tf-acc templated"
  channel_type = "webhook"
  config       = jsonencode({ url = "https://example.com/tf-acc-hook" })
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("rustinion_notification_channel.tpl", "subject_template"),
					resource.TestCheckNoResourceAttr("rustinion_notification_channel.tpl", "body_template"),
					resource.TestCheckNoResourceAttr("rustinion_notification_channel.tpl", "payload_template"),
				),
			},
		},
	})
}
