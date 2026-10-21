package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/CanuteTheGreat/terraform-provider-rustinion/internal/client"
)

func TestAccDeviceDisplayNameResource(t *testing.T) {
	deviceID := testAccDeviceID(t)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDeviceDisplayNameOnServer(deviceID, nil),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_device_display_name" "test" {
  device_id    = %q
  display_name = "tf display name"
}
`, deviceID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_device_display_name.test", "id", deviceID),
					resource.TestCheckResourceAttr("rustinion_device_display_name.test", "display_name", "tf display name"),
					resource.TestCheckResourceAttrSet("rustinion_device_display_name.test", "name"),
					testAccCheckDeviceDisplayNameOnServer(deviceID, ddnStrPtr("tf display name")),
				),
			},
			{
				ResourceName:      "rustinion_device_display_name.test",
				ImportState:       true,
				ImportStateId:     deviceID,
				ImportStateVerify: true,
			},
		},
	})
}

func ddnStrPtr(s string) *string { return &s }

func testAccCheckDeviceDisplayNameOnServer(deviceID string, want *string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		d, err := testAccAPIClient().GetDeviceDisplayName(context.Background(), deviceID)
		if err != nil {
			return err
		}
		if (d.DisplayName == nil) != (want == nil) || (want != nil && *d.DisplayName != *want) {
			return fmt.Errorf("server has display_name=%v, want %v", d.DisplayName, want)
		}
		return nil
	}
}

func TestDeviceDisplayNameClientRoundTrip(t *testing.T) {
	var calls []string
	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		var shown any
		if r.Method == http.MethodPut {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decoding body: %v", err)
			}
			bodies = append(bodies, body)
			shown = body["display_name"]
		}
		if r.URL.Path != "/api/v1/customer/devices/d1/display-name" {
			http.NotFound(w, r)
			return
		}
		name := "GL-AX1800"
		if s, ok := shown.(string); ok {
			name = s
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "d1", "hostname": "GL-AX1800", "display_name": shown, "name": name,
		})
	}))
	defer server.Close()

	c := client.New(server.URL, "k")
	ctx := context.Background()
	d, err := c.PutDeviceDisplayName(ctx, "d1", ddnStrPtr("GL-AX1800 (10.66.69.10)"))
	if err != nil {
		t.Fatal(err)
	}
	m := deviceDisplayNameFromClient("d1", d)
	if m.DisplayName.ValueString() != "GL-AX1800 (10.66.69.10)" || m.Hostname.ValueString() != "GL-AX1800" ||
		m.Name.ValueString() != "GL-AX1800 (10.66.69.10)" {
		t.Fatalf("model %+v", m)
	}
	// Clearing sends an explicit null.
	d, err = c.PutDeviceDisplayName(ctx, "d1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := bodies[1]["display_name"]; !ok || v != nil {
		t.Fatalf("clear body %v", bodies[1])
	}
	if d.DisplayName != nil || !deviceDisplayNameFromClient("d1", d).DisplayName.IsNull() {
		t.Fatalf("cleared %+v", d)
	}
	if _, err := c.GetDeviceDisplayName(ctx, "d1"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"PUT /api/v1/customer/devices/d1/display-name",
		"PUT /api/v1/customer/devices/d1/display-name",
		"GET /api/v1/customer/devices/d1/display-name",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls %v", calls)
	}
}
