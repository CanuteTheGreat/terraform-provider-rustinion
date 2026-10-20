package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

func TestAccDashboardResource(t *testing.T) {
	groupID := testAccGroupID(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDashboardDestroy,
		Steps: []resource.TestStep{
			// Create + Read: one widget.
			{
				Config: fmt.Sprintf(`
resource "rustinion_dashboard" "test" {
  group_id = %[1]q
  name     = "tf-acc fleet overview"

  widget {
    title       = "CPU"
    widget_type = "kpi_tile"
    config      = jsonencode({ metric_type = "cpu_avg_pct", window_minutes = 60 })
  }
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rustinion_dashboard.test", "id"),
					resource.TestCheckResourceAttr("rustinion_dashboard.test", "name", "tf-acc fleet overview"),
					resource.TestCheckResourceAttr("rustinion_dashboard.test", "widget.#", "1"),
					resource.TestCheckResourceAttr("rustinion_dashboard.test", "widget.0.title", "CPU"),
					resource.TestCheckResourceAttr("rustinion_dashboard.test", "widget.0.widget_type", "kpi_tile"),
				),
			},
			// Import.
			{
				ResourceName:      "rustinion_dashboard.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update: reorder to two widgets, in a new order — exercises
			// the server's full-widget-replace PUT semantics and that
			// widget order (== position) is taken from list order.
			{
				Config: fmt.Sprintf(`
resource "rustinion_dashboard" "test" {
  group_id = %[1]q
  name     = "tf-acc fleet overview v2"

  widget {
    title       = "Alerts"
    widget_type = "alert_rule_status"
    # config deliberately omitted - alert_rule_status needs none, and
    # this exercises that an unconfigured, Computed config attribute
    # does not drift on a second, no-op plan (a real bug this test
    # previously did not catch because every widget always set config
    # explicitly).
  }

  widget {
    title       = "CPU"
    widget_type = "kpi_tile"
    config      = jsonencode({ metric_type = "cpu_avg_pct", window_minutes = 60 })
  }
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_dashboard.test", "name", "tf-acc fleet overview v2"),
					resource.TestCheckResourceAttr("rustinion_dashboard.test", "widget.#", "2"),
					resource.TestCheckResourceAttr("rustinion_dashboard.test", "widget.0.widget_type", "alert_rule_status"),
					resource.TestCheckResourceAttr("rustinion_dashboard.test", "widget.1.widget_type", "kpi_tile"),
					// Neither widget sets a width, so both must land on
					// the schema default rather than drifting to empty.
					resource.TestCheckResourceAttr("rustinion_dashboard.test", "widget.0.width", "full"),
					resource.TestCheckResourceAttr("rustinion_dashboard.test", "widget.1.width", "full"),
					resource.TestCheckNoResourceAttr("rustinion_dashboard.test", "default_time_range_days"),
				),
			},
			// Widths + a shared dashboard time range, plus the widget
			// config opt-in that makes the shared range take effect.
			{
				Config: fmt.Sprintf(`
resource "rustinion_dashboard" "test" {
  group_id                = %[1]q
  name                    = "tf-acc fleet overview v3"
  default_time_range_days = 30

  widget {
    title       = "Alerts"
    widget_type = "alert_rule_status"
    width       = "half"
  }

  widget {
    title       = "Errors"
    widget_type = "log_search"
    width       = "third"
    config      = jsonencode({ days = 7, use_dashboard_time_range = true })
  }
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_dashboard.test", "default_time_range_days", "30"),
					resource.TestCheckResourceAttr("rustinion_dashboard.test", "widget.0.width", "half"),
					resource.TestCheckResourceAttr("rustinion_dashboard.test", "widget.1.width", "third"),
				),
			},
			// Re-import at the new shape: catches a width or shared
			// range that writes but doesn't read back.
			{
				ResourceName:      "rustinion_dashboard.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Dropping both back off again must really clear them, not
			// silently keep the previous values.
			{
				Config: fmt.Sprintf(`
resource "rustinion_dashboard" "test" {
  group_id = %[1]q
  name     = "tf-acc fleet overview v4"

  widget {
    title       = "Alerts"
    widget_type = "alert_rule_status"
  }
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("rustinion_dashboard.test", "default_time_range_days"),
					resource.TestCheckResourceAttr("rustinion_dashboard.test", "widget.0.width", "full"),
				),
			},
		},
	})
}

// TestDashboardToClientWidth covers the plumbing the acceptance test
// can't reach without a live server: an unset width must be sent as
// omitted (the server then defaults it), and an absent shared range
// must stay a nil pointer rather than serializing as 0.
func TestDashboardToClientWidth(t *testing.T) {
	m := dashboardResourceModel{
		GroupID:              types.StringValue("g"),
		Name:                 types.StringValue("d"),
		DefaultTimeRangeDays: types.Int64Null(),
		Widget: []dashboardWidgetModel{
			{
				Title:      types.StringValue("a"),
				WidgetType: types.StringValue("alert_rule_status"),
				Config:     jsontypes.NewNormalizedValue("{}"),
				Width:      types.StringNull(),
			},
			{
				Title:      types.StringValue("b"),
				WidgetType: types.StringValue("log_search"),
				Config:     jsontypes.NewNormalizedValue("{}"),
				Width:      types.StringValue("half"),
			},
		},
	}
	d := dashboardToClient(m)
	if d.DefaultTimeRangeDays != nil {
		t.Fatalf("expected a nil shared range, got %v", *d.DefaultTimeRangeDays)
	}
	if d.Widgets[0].Width != "" {
		t.Fatalf("an unset width should serialize as omitted, got %q", d.Widgets[0].Width)
	}
	if d.Widgets[1].Width != "half" {
		t.Fatalf("expected half, got %q", d.Widgets[1].Width)
	}

	body, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "default_time_range_days") {
		t.Fatalf("an unset shared range must not appear in the request body: %s", body)
	}
	if strings.Contains(string(body), `"width":""`) {
		t.Fatalf("an unset width must not appear in the request body: %s", body)
	}
}

// TestDashboardFromClientWidthDefault: a server response with no width
// (an older server, or a field the API stops sending) must read back as
// the schema default, not as an empty string that would show up as a
// permanent diff.
func TestDashboardFromClientWidthDefault(t *testing.T) {
	days := int64(7)
	d := &client.Dashboard{
		ID:                   "id",
		GroupID:              "g",
		Name:                 "d",
		DefaultTimeRangeDays: &days,
		Widgets: []client.DashboardWidget{
			{Title: "a", WidgetType: "alert_rule_status", Config: json.RawMessage("{}")},
			{Title: "b", WidgetType: "log_search", Config: json.RawMessage("{}"), Width: "third"},
		},
	}
	m := dashboardFromClient(d)
	if got := m.Widget[0].Width.ValueString(); got != "full" {
		t.Fatalf("expected full, got %q", got)
	}
	if got := m.Widget[1].Width.ValueString(); got != "third" {
		t.Fatalf("expected third, got %q", got)
	}
	if got := m.DefaultTimeRangeDays.ValueInt64(); got != 7 {
		t.Fatalf("expected 7, got %d", got)
	}

	d.DefaultTimeRangeDays = nil
	if m := dashboardFromClient(d); !m.DefaultTimeRangeDays.IsNull() {
		t.Fatalf("expected a null shared range, got %v", m.DefaultTimeRangeDays)
	}
}

func testAccCheckDashboardDestroy(s *terraform.State) error {
	c := testAccAPIClient()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "rustinion_dashboard" {
			continue
		}
		_, err := c.GetDashboard(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("dashboard %s still exists", rs.Primary.ID)
		}
		if !client.IsNotFound(err) {
			return err
		}
	}
	return nil
}
