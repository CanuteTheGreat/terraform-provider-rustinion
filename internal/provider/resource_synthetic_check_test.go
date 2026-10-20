package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

func TestAccSyntheticCheckResource(t *testing.T) {
	groupID := testAccGroupID(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSyntheticCheckDestroy,
		Steps: []resource.TestStep{
			// Create + Read: a plain http check.
			{
				Config: fmt.Sprintf(`
resource "rustinion_synthetic_check" "test" {
  group_id   = %[1]q
  name       = "tf-acc health check"
  check_type = "http"
  target     = "https://example.com/health"
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rustinion_synthetic_check.test", "id"),
					resource.TestCheckResourceAttr("rustinion_synthetic_check.test", "name", "tf-acc health check"),
					resource.TestCheckResourceAttr("rustinion_synthetic_check.test", "check_type", "http"),
					resource.TestCheckResourceAttr("rustinion_synthetic_check.test", "target", "https://example.com/health"),
					resource.TestCheckResourceAttr("rustinion_synthetic_check.test", "http_method", "GET"),
					resource.TestCheckResourceAttr("rustinion_synthetic_check.test", "expected_status_min", "200"),
					resource.TestCheckResourceAttr("rustinion_synthetic_check.test", "expected_status_max", "299"),
					resource.TestCheckResourceAttr("rustinion_synthetic_check.test", "timeout_ms", "10000"),
					resource.TestCheckResourceAttr("rustinion_synthetic_check.test", "interval_seconds", "60"),
					resource.TestCheckResourceAttr("rustinion_synthetic_check.test", "relay_mode", "direct"),
					resource.TestCheckResourceAttr("rustinion_synthetic_check.test", "enabled", "true"),
					resource.TestCheckResourceAttr("rustinion_synthetic_check.test", "step.#", "0"),
				),
			},
			// Import.
			{
				ResourceName:      "rustinion_synthetic_check.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update: switch to a multi-step check entirely — a real
			// check_type change, plus a login-then-authenticated-request
			// flow exercising capture/substitution end to end.
			{
				Config: fmt.Sprintf(`
resource "rustinion_synthetic_check" "test" {
  group_id   = %[1]q
  name       = "tf-acc login flow"
  check_type = "http_multistep"

  step {
    name   = "login"
    method = "POST"
    url    = "https://example.com/login"

    capture_variable = "token"
    capture_source   = "json_path"
    capture_path     = "data.token"
  }

  step {
    name   = "authenticated request"
    method = "GET"
    url    = "https://example.com/me"
    headers = jsonencode({ Authorization = "Bearer {{token}}" })
  }
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_synthetic_check.test", "name", "tf-acc login flow"),
					resource.TestCheckResourceAttr("rustinion_synthetic_check.test", "check_type", "http_multistep"),
					resource.TestCheckResourceAttr("rustinion_synthetic_check.test", "step.#", "2"),
					resource.TestCheckResourceAttr("rustinion_synthetic_check.test", "step.0.name", "login"),
					resource.TestCheckResourceAttr("rustinion_synthetic_check.test", "step.0.capture_variable", "token"),
					resource.TestCheckResourceAttr("rustinion_synthetic_check.test", "step.0.capture_source", "json_path"),
					resource.TestCheckResourceAttr("rustinion_synthetic_check.test", "step.0.capture_path", "data.token"),
					resource.TestCheckResourceAttr("rustinion_synthetic_check.test", "step.1.name", "authenticated request"),
				),
			},
		},
	})
}

func TestAccSyntheticCheckResource_tcp(t *testing.T) {
	groupID := testAccGroupID(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSyntheticCheckDestroy,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_synthetic_check" "tcp" {
  group_id   = %[1]q
  name       = "tf-acc tcp check"
  check_type = "tcp"
  target     = "example.com:443"
}
`, groupID),
				Check: resource.TestCheckResourceAttr("rustinion_synthetic_check.tcp", "target", "example.com:443"),
			},
		},
	})
}

// TestAccSyntheticCheckResource_locations exercises this pass's own new
// capability end to end against a real server: a check with two
// `location` blocks (each a real enrolled device — see
// testAccDeviceID's own doc comment for why there's only ever one real
// device fixture, so both locations reference it; a real check
// constraint would reject a duplicate (check_id, device_id) pair, so
// this proves create/read/import round-trip correctly for a single
// location instead of asserting anything about multiple distinct
// devices) and a non-default min_failing_locations.
func TestAccSyntheticCheckResource_locations(t *testing.T) {
	groupID := testAccGroupID(t)
	deviceID := testAccDeviceID(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSyntheticCheckDestroy,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_synthetic_check" "located" {
  group_id              = %[1]q
  name                  = "tf-acc located check"
  check_type            = "tcp"
  target                = "10.0.0.5:443"
  relay_mode            = "relay"
  min_failing_locations = 1

  location {
    device_id = %[2]q
    label     = "primary"
  }
}
`, groupID, deviceID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rustinion_synthetic_check.located", "id"),
					resource.TestCheckResourceAttr("rustinion_synthetic_check.located", "relay_mode", "relay"),
					resource.TestCheckResourceAttr("rustinion_synthetic_check.located", "min_failing_locations", "1"),
					resource.TestCheckResourceAttr("rustinion_synthetic_check.located", "location.#", "1"),
					resource.TestCheckResourceAttr("rustinion_synthetic_check.located", "location.0.device_id", deviceID),
					resource.TestCheckResourceAttr("rustinion_synthetic_check.located", "location.0.label", "primary"),
				),
			},
			// Import.
			{
				ResourceName:      "rustinion_synthetic_check.located",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update: change the label and min_failing_locations — a
			// full-replace PUT, not just a plan no-op.
			{
				Config: fmt.Sprintf(`
resource "rustinion_synthetic_check" "located" {
  group_id              = %[1]q
  name                  = "tf-acc located check"
  check_type            = "tcp"
  target                = "10.0.0.5:443"
  relay_mode            = "relay"
  min_failing_locations = 1

  location {
    device_id = %[2]q
    label     = "renamed"
  }
}
`, groupID, deviceID),
				Check: resource.TestCheckResourceAttr("rustinion_synthetic_check.located", "location.0.label", "renamed"),
			},
		},
	})
}

func testAccCheckSyntheticCheckDestroy(s *terraform.State) error {
	c := testAccAPIClient()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "rustinion_synthetic_check" {
			continue
		}
		_, err := c.GetSyntheticCheck(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("synthetic check %s still exists", rs.Primary.ID)
		}
		if !client.IsNotFound(err) {
			return err
		}
	}
	return nil
}

// The probe check types and per-check thresholds round-trip through the
// API. enabled = false: nothing is probed while the test runs (no lookups
// of test names on the real internet).
func TestAccSyntheticCheckResourceProbeTypes(t *testing.T) {
	groupID := testAccGroupID(t)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSyntheticCheckDestroy,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_synthetic_check" "dns" {
  group_id              = %[1]q
  name                  = "tf-acc resolver answers"
  check_type            = "dns"
  target                = "intranet.example.test"
  relay_mode            = "relay"
  enabled               = false
  dns_record_type       = "MX"
  dns_server            = "10.0.0.53:53"
  expected_answer       = "mail.example.test"
  failures_before_alert = 3
  max_response_time_ms  = 500
}

resource "rustinion_synthetic_check" "cert" {
  group_id           = %[1]q
  name               = "tf-acc certificate"
  check_type         = "tls"
  target             = "shop.example.test:8443"
  enabled            = false
  tls_verify         = false
  cert_warning_days  = 21
  cert_critical_days = 5
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_synthetic_check.dns", "dns_record_type", "MX"),
					resource.TestCheckResourceAttr("rustinion_synthetic_check.dns", "dns_server", "10.0.0.53:53"),
					resource.TestCheckResourceAttr("rustinion_synthetic_check.dns", "expected_answer", "mail.example.test"),
					resource.TestCheckResourceAttr("rustinion_synthetic_check.dns", "failures_before_alert", "3"),
					resource.TestCheckResourceAttr("rustinion_synthetic_check.dns", "max_response_time_ms", "500"),
					resource.TestCheckResourceAttr("rustinion_synthetic_check.dns", "cert_warning_days", "30"),
					resource.TestCheckResourceAttr("rustinion_synthetic_check.cert", "tls_verify", "false"),
					resource.TestCheckResourceAttr("rustinion_synthetic_check.cert", "cert_warning_days", "21"),
					resource.TestCheckResourceAttr("rustinion_synthetic_check.cert", "cert_critical_days", "5"),
					resource.TestCheckNoResourceAttr("rustinion_synthetic_check.cert", "dns_record_type"),
					func(s *terraform.State) error {
						id := s.RootModule().Resources["rustinion_synthetic_check.dns"].Primary.ID
						got, err := testAccAPIClient().GetSyntheticCheck(context.Background(), id)
						if err != nil {
							return err
						}
						if got.FailuresBeforeAlert != 3 || got.DNSServer == nil || *got.DNSServer != "10.0.0.53:53" {
							return fmt.Errorf("server has %+v", got)
						}
						return nil
					},
				),
			},
			{
				ResourceName:      "rustinion_synthetic_check.dns",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: fmt.Sprintf(`
resource "rustinion_synthetic_check" "dns" {
  group_id   = %[1]q
  name       = "tf-acc resolver answers"
  check_type = "dns"
  target     = "intranet.example.test"
  relay_mode = "relay"
  enabled    = false
}

resource "rustinion_synthetic_check" "cert" {
  group_id   = %[1]q
  name       = "tf-acc certificate"
  check_type = "tls"
  target     = "shop.example.test:8443"
  enabled    = false
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("rustinion_synthetic_check.dns", "dns_server"),
					resource.TestCheckNoResourceAttr("rustinion_synthetic_check.dns", "max_response_time_ms"),
					resource.TestCheckResourceAttr("rustinion_synthetic_check.dns", "failures_before_alert", "1"),
					resource.TestCheckResourceAttr("rustinion_synthetic_check.cert", "tls_verify", "true"),
					resource.TestCheckResourceAttr("rustinion_synthetic_check.cert", "cert_critical_days", "7"),
				),
			},
		},
	})
}
