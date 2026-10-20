package provider

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

func TestAccRetentionPolicyResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckRetentionPolicyCleared,
		Steps: []resource.TestStep{
			// Create: set two tiers, leave the other four on the
			// platform defaults.
			{
				Config: `
resource "rustinion_retention_policy" "test" {
  device_log_retention_days         = 45
  device_metrics_raw_retention_days = 15
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_retention_policy.test", "id", "retention-policy"),
					resource.TestCheckResourceAttr("rustinion_retention_policy.test", "device_log_retention_days", "45"),
					resource.TestCheckResourceAttr("rustinion_retention_policy.test", "device_metrics_raw_retention_days", "15"),
					resource.TestCheckNoResourceAttr("rustinion_retention_policy.test", "custom_metric_raw_retention_days"),
				),
			},
			// Import: the id is a placeholder, so this also pins that
			// Read() populates every attribute from the server rather
			// than relying on prior state.
			{
				ResourceName:      "rustinion_retention_policy.test",
				ImportState:       true,
				ImportStateId:     "retention-policy",
				ImportStateVerify: true,
			},
			// Update: change one tier and DROP the other from the
			// config entirely. The dropped one must clear back to the
			// platform default, not linger — the full-replace property
			// that makes this resource declarative.
			{
				Config: `
resource "rustinion_retention_policy" "test" {
  device_log_retention_days = 90
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_retention_policy.test", "device_log_retention_days", "90"),
					resource.TestCheckNoResourceAttr("rustinion_retention_policy.test", "device_metrics_raw_retention_days"),
				),
			},
		},
	})
}

// testAccCheckRetentionPolicyCleared: the policy row is the tenant
// itself and can't be deleted, so "destroyed" means every override is
// back to null.
func testAccCheckRetentionPolicyCleared(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "rustinion_retention_policy" {
			continue
		}
		policy, err := testAccAPIClient().GetRetentionPolicy(context.Background())
		if err != nil {
			if client.IsNotFound(err) {
				return nil
			}
			return err
		}
		if policy.DeviceLogRetentionDays != nil || policy.DeviceMetricsRawRetentionDays != nil {
			return errRetentionPolicyNotCleared
		}
	}
	return nil
}

var errRetentionPolicyNotCleared = &retentionPolicyNotClearedError{}

type retentionPolicyNotClearedError struct{}

func (e *retentionPolicyNotClearedError) Error() string {
	return "retention policy overrides were not cleared on destroy"
}

// TestRetentionPolicyToClientNullsUnsetTiers is the property the whole
// resource rests on: an attribute absent from the config must reach the
// server as an explicit JSON null (its "use the platform default"
// signal), never as 0 and never omitted — a PUT is a full replace, so a
// missing field would silently mean something different.
func TestRetentionPolicyToClientNullsUnsetTiers(t *testing.T) {
	m := retentionPolicyResourceModel{
		DeviceLogRetentionDays: types.Int64Value(45),
		// Every other tier left null, as an omitted attribute is.
	}
	p := retentionPolicyToClient(m)
	if p.DeviceLogRetentionDays == nil || *p.DeviceLogRetentionDays != 45 {
		t.Fatalf("expected 45, got %v", p.DeviceLogRetentionDays)
	}
	if p.CustomMetricRawRetentionDays != nil {
		t.Fatalf("an unset tier must be nil, got %v", *p.CustomMetricRawRetentionDays)
	}

	body, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"custom_metric_raw_retention_days":null`) {
		t.Fatalf("an unset tier must serialize as an explicit null: %s", body)
	}
	if strings.Contains(string(body), `"custom_metric_raw_retention_days":0`) {
		t.Fatalf("an unset tier must never serialize as 0: %s", body)
	}
}

// An unknown value (a tier interpolated from something not yet computed)
// must behave like unset rather than panicking or sending a zero.
func TestRetentionPolicyToClientTreatsUnknownAsUnset(t *testing.T) {
	m := retentionPolicyResourceModel{
		DeviceLogRetentionDays: types.Int64Unknown(),
	}
	if p := retentionPolicyToClient(m); p.DeviceLogRetentionDays != nil {
		t.Fatalf("an unknown tier must be nil, got %v", *p.DeviceLogRetentionDays)
	}
}

func TestRetentionPolicyFromClientRoundTrip(t *testing.T) {
	days := int64(400)
	p := &client.RetentionPolicy{
		CustomMetricHourlyRetentionDays: &days,
	}
	m := retentionPolicyFromClient(p)
	if got := m.ID.ValueString(); got != retentionPolicyID {
		t.Fatalf("expected the fixed singleton id, got %q", got)
	}
	if got := m.CustomMetricHourlyRetentionDays.ValueInt64(); got != 400 {
		t.Fatalf("expected 400, got %d", got)
	}
	// A tier the server reports as null must read back as null, so a
	// config that never set it shows no diff.
	if !m.DeviceLogRetentionDays.IsNull() {
		t.Fatalf("expected null, got %v", m.DeviceLogRetentionDays)
	}
}
