package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

// Pauses the real e2e device's agent updates, then destroy puts it back to
// follow_group (CheckDestroy reads the server).
func TestAccDeviceAgentUpdateResource(t *testing.T) {
	deviceID := testAccDeviceID(t)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDeviceAgentUpdateOnServer(deviceID, "follow_group"),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_device_agent_update" "test" {
  device_id = %q
  mode      = "paused"
  reason    = "tf acceptance test"
}
`, deviceID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_device_agent_update.test", "id", deviceID),
					resource.TestCheckResourceAttr("rustinion_device_agent_update.test", "mode", "paused"),
					resource.TestCheckResourceAttr("rustinion_device_agent_update.test", "source", "device_setting"),
					resource.TestCheckResourceAttr("rustinion_device_agent_update.test", "reason", "tf acceptance test"),
					testAccCheckDeviceAgentUpdateOnServer(deviceID, "paused"),
				),
			},
			{
				ResourceName:      "rustinion_device_agent_update.test",
				ImportState:       true,
				ImportStateId:     deviceID,
				ImportStateVerify: true,
			},
			{
				Config: fmt.Sprintf(`
resource "rustinion_device_agent_update" "test" {
  device_id = %q
  mode      = "auto"
}
`, deviceID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_device_agent_update.test", "mode", "auto"),
					resource.TestCheckNoResourceAttr("rustinion_device_agent_update.test", "reason"),
					testAccCheckDeviceAgentUpdateOnServer(deviceID, "auto"),
				),
			},
		},
	})
}

func testAccCheckDeviceAgentUpdateOnServer(deviceID, want string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		p, err := testAccAPIClient().GetDeviceAgentUpdate(context.Background(), deviceID)
		if err != nil {
			return err
		}
		if p.Mode != want {
			return fmt.Errorf("server has mode=%q, want %q", p.Mode, want)
		}
		return nil
	}
}

func TestDeviceAgentUpdateConfigRules(t *testing.T) {
	cases := []struct {
		mode, pinned, reason string
		ok                   bool
	}{
		{"pinned", "0.1.230", "", true},
		{"pinned", "", "", false},
		{"paused", "0.1.230", "", false},
		{"paused", "", "flash full", true},
		{"follow_group", "", "why", false},
		{"follow_group", "", "", true},
	}
	for _, c := range cases {
		got := deviceAgentUpdateConfigError(c.mode, c.pinned, c.reason) == ""
		if got != c.ok {
			t.Errorf("%+v: ok=%v", c, got)
		}
	}
	if deviceAgentUpdateMode(deviceAgentUpdateResourceModel{Mode: types.StringNull()}) != "follow_group" {
		t.Fatal("null mode must mean follow_group")
	}
}

func TestDeviceAgentUpdateClientRoundTrip(t *testing.T) {
	var calls []string
	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodPut {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decoding body: %v", err)
			}
			bodies = append(bodies, body)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/customer/devices/d1/agent-update" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "d1", "mode": "pinned", "pinned_version": "0.1.230", "reason": nil,
			"source": "device_setting", "explanation": "Pinned to 0.1.230 — set on this device by api-key:tf",
		})
	}))
	defer server.Close()

	c := client.New(server.URL, "k")
	ctx := context.Background()
	d, err := c.PutDeviceAgentUpdate(ctx, "d1", "pinned", "0.1.230", "")
	if err != nil {
		t.Fatal(err)
	}
	if bodies[0]["mode"] != "pinned" || bodies[0]["pinned_version"] != "0.1.230" {
		t.Fatalf("body %v", bodies[0])
	}
	if _, has := bodies[0]["reason"]; has {
		t.Fatalf("empty reason must not be sent: %v", bodies[0])
	}
	m := deviceAgentUpdateFromClient("d1", d)
	if m.Mode.ValueString() != "pinned" || m.PinnedVersion.ValueString() != "0.1.230" || !m.Reason.IsNull() || m.Source.ValueString() != "device_setting" {
		t.Fatalf("model %+v", m)
	}
	if _, err := c.PutDeviceAgentUpdate(ctx, "d1", "paused", "", "flash full"); err != nil {
		t.Fatal(err)
	}
	if bodies[1]["reason"] != "flash full" {
		t.Fatalf("body %v", bodies[1])
	}
	if _, err := c.GetDeviceAgentUpdate(ctx, "d1"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"PUT /api/v1/customer/devices/d1/agent-update",
		"PUT /api/v1/customer/devices/d1/agent-update",
		"GET /api/v1/customer/devices/d1/agent-update",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls %v", calls)
	}
}
