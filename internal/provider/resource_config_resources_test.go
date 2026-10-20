package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

// Acceptance tests for the SNMP target, device tag, maintenance window,
// RUM application and report schedule resources against a real
// rustinion-web (live-e2e runs them with TF_ACC=1). Each checks
// create, an in-place update where the API has one, import, and that
// destroy really removed/cancelled the object server-side.

func TestAccSNMPTargetResource(t *testing.T) {
	groupID := testAccGroupID(t)
	config := func(name string, port int, community string) string {
		return fmt.Sprintf(`
resource "rustinion_snmp_target" "test" {
  group_id  = %[1]q
  name      = %[2]q
  host      = "192.0.2.10"
  port      = %[3]d
  community = %[4]q
}
`, groupID, name, port, community)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckGoneOnServer("rustinion_snmp_target", func(c *client.Client, id string) error {
			_, err := c.GetSNMPTarget(context.Background(), id)
			return err
		}),
		Steps: []resource.TestStep{
			{
				Config: config("tf-acc core switch", 161, "tf-acc-community"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rustinion_snmp_target.test", "id"),
					resource.TestCheckResourceAttr("rustinion_snmp_target.test", "version", "v2c"),
					resource.TestCheckResourceAttr("rustinion_snmp_target.test", "poll_interval_seconds", "300"),
					resource.TestCheckResourceAttr("rustinion_snmp_target.test", "enabled", "true"),
					resource.TestCheckResourceAttr("rustinion_snmp_target.test", "has_community", "true"),
				),
			},
			{
				Config: config("tf-acc core switch 2", 1161, "tf-acc-community"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_snmp_target.test", "name", "tf-acc core switch 2"),
					resource.TestCheckResourceAttr("rustinion_snmp_target.test", "port", "1161"),
					resource.TestCheckResourceAttr("rustinion_snmp_target.test", "has_community", "true"),
				),
			},
			{
				ResourceName:            "rustinion_snmp_target.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"community", "v3_auth_key", "v3_priv_key"},
			},
		},
	})
}

func TestAccSNMPTargetResource_v3(t *testing.T) {
	groupID := testAccGroupID(t)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_snmp_target" "v3" {
  group_id         = %[1]q
  name             = "tf-acc ups"
  host             = "192.0.2.11"
  version          = "v3"
  v3_username      = "monitor"
  v3_auth_protocol = "sha2"
  v3_auth_key      = "tf-acc-auth-key"
  v3_priv_protocol = "aes128"
  v3_priv_key      = "tf-acc-priv-key"
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_snmp_target.v3", "has_v3_auth_key", "true"),
					resource.TestCheckResourceAttr("rustinion_snmp_target.v3", "has_v3_priv_key", "true"),
					resource.TestCheckResourceAttr("rustinion_snmp_target.v3", "has_community", "false"),
				),
			},
		},
	})
}

func TestAccDeviceTagResource(t *testing.T) {
	deviceID := testAccDeviceID(t)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(s *terraform.State) error {
			tags, err := testAccAPIClient().ListDeviceTags(context.Background(), deviceID)
			if err != nil {
				return err
			}
			for _, tag := range tags.Tags {
				if tag == "tf-acc-web" {
					return fmt.Errorf("tag tf-acc-web still on device %s after destroy", deviceID)
				}
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_device_tag" "test" {
  device_id = %[1]q
  tag       = "tf-acc-web"
}
`, deviceID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_device_tag.test", "id", deviceID+"/tf-acc-web"),
					func(s *terraform.State) error {
						tags, err := testAccAPIClient().ListDeviceTags(context.Background(), deviceID)
						if err != nil {
							return err
						}
						for _, tag := range tags.Tags {
							if tag == "tf-acc-web" {
								return nil
							}
						}
						return fmt.Errorf("tag not on the device: %v", tags.Tags)
					},
				),
			},
			{
				ResourceName:      "rustinion_device_tag.test",
				ImportState:       true,
				ImportStateId:     deviceID + "/tf-acc-web",
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccMaintenanceWindowResource(t *testing.T) {
	groupID := testAccGroupID(t)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(s *terraform.State) error {
			for _, rs := range s.RootModule().Resources {
				if rs.Type != "rustinion_maintenance_window" {
					continue
				}
				w, err := testAccAPIClient().GetMaintenanceWindow(context.Background(), rs.Primary.ID)
				if err != nil {
					return err
				}
				if w.CancelledAt == nil {
					return fmt.Errorf("maintenance window %s was not cancelled on destroy", rs.Primary.ID)
				}
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_maintenance_window" "weekly" {
  name       = "tf-acc Sunday patching"
  group_id   = %[1]q
  starts_at  = "2030-01-06T02:00:00Z"
  ends_at    = "2030-01-06T04:00:00Z"
  recurrence = "weekly"
}

resource "rustinion_maintenance_window" "org" {
  name      = "tf-acc org freeze"
  starts_at = "2030-02-01T00:00:00Z"
  ends_at   = "2030-02-03T00:00:00Z"
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_maintenance_window.weekly", "scope", "group"),
					resource.TestCheckResourceAttr("rustinion_maintenance_window.weekly", "recurrence", "weekly"),
					resource.TestCheckResourceAttr("rustinion_maintenance_window.org", "scope", "tenant"),
					resource.TestCheckResourceAttr("rustinion_maintenance_window.org", "recurrence", "none"),
					resource.TestCheckNoResourceAttr("rustinion_maintenance_window.org", "cancelled_at"),
				),
			},
			{
				ResourceName:      "rustinion_maintenance_window.weekly",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccRUMApplicationResource(t *testing.T) {
	config := func(name string) string {
		return fmt.Sprintf(`
resource "rustinion_rum_application" "test" {
  name = %q
}
`, name)
	}
	var token string
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckGoneOnServer("rustinion_rum_application", func(c *client.Client, id string) error {
			_, err := c.GetRUMApplication(context.Background(), id)
			return err
		}),
		Steps: []resource.TestStep{
			{
				Config: config("tf-acc shop"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rustinion_rum_application.test", "client_token"),
					func(s *terraform.State) error {
						token = s.RootModule().Resources["rustinion_rum_application.test"].Primary.Attributes["client_token"]
						return nil
					},
				),
			},
			{
				Config: config("tf-acc storefront"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_rum_application.test", "name", "tf-acc storefront"),
					func(s *terraform.State) error {
						if got := s.RootModule().Resources["rustinion_rum_application.test"].Primary.Attributes["client_token"]; got != token {
							return fmt.Errorf("renaming changed the client token (%s -> %s)", token, got)
						}
						return nil
					},
				),
			},
			{
				ResourceName:      "rustinion_rum_application.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccReportScheduleResource(t *testing.T) {
	groupID := testAccGroupID(t)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckGoneOnServer("rustinion_report_schedule", func(c *client.Client, id string) error {
			_, err := c.GetReportSchedule(context.Background(), id)
			return err
		}),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_report_schedule" "test" {
  name        = "tf-acc weekly uptime"
  report_type = "uptime_summary"
  group_id    = %[1]q
  cadence     = "weekly"
  recipients  = ["ops@errant.solutions"]
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_report_schedule.test", "format", "html"),
					resource.TestCheckResourceAttr("rustinion_report_schedule.test", "enabled", "true"),
					resource.TestCheckResourceAttr("rustinion_report_schedule.test", "recipients.#", "1"),
				),
			},
			{
				Config: `
resource "rustinion_report_schedule" "test" {
  name        = "tf-acc monthly SLOs"
  report_type = "slo_report"
  cadence     = "monthly"
  format      = "csv"
  enabled     = false
  recipients  = ["ops@errant.solutions", "reports@errant.solutions"]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_report_schedule.test", "report_type", "slo_report"),
					resource.TestCheckNoResourceAttr("rustinion_report_schedule.test", "group_id"),
					resource.TestCheckResourceAttr("rustinion_report_schedule.test", "enabled", "false"),
					resource.TestCheckResourceAttr("rustinion_report_schedule.test", "recipients.1", "reports@errant.solutions"),
				),
			},
			{
				ResourceName:      "rustinion_report_schedule.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// testAccCheckGoneOnServer: every resource of `kind` in state must now
// answer 404 from the API.
func testAccCheckGoneOnServer(kind string, get func(*client.Client, string) error) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testAccAPIClient()
		for _, rs := range s.RootModule().Resources {
			if rs.Type != kind {
				continue
			}
			err := get(c, rs.Primary.ID)
			if err == nil {
				return fmt.Errorf("%s %s still exists after destroy", kind, rs.Primary.ID)
			}
			if !client.IsNotFound(err) {
				return err
			}
		}
		return nil
	}
}
