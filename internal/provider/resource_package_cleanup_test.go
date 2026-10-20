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

func TestAccGroupPackageCleanupResource(t *testing.T) {
	groupID := testAccGroupID(t)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGroupPackageCleanupOnServer(groupID, true),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "rustinion_group_package_cleanup" "test" {
  group_id = %q
  enabled  = false
}
`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_group_package_cleanup.test", "id", groupID),
					resource.TestCheckResourceAttr("rustinion_group_package_cleanup.test", "enabled", "false"),
					testAccCheckGroupPackageCleanupOnServer(groupID, false),
				),
			},
			{
				ResourceName:      "rustinion_group_package_cleanup.test",
				ImportState:       true,
				ImportStateId:     groupID,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccCheckGroupPackageCleanupOnServer(groupID string, want bool) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		p, err := testAccAPIClient().GetGroupPackageCleanup(context.Background(), groupID)
		if err != nil {
			return err
		}
		if p.Enabled != want {
			return fmt.Errorf("server has enabled=%v, want %v", p.Enabled, want)
		}
		return nil
	}
}

// Omitted enabled means the default (on), never false.
func TestGroupPackageCleanupToClientFillsDefaults(t *testing.T) {
	p := groupPackageCleanupToClient(groupPackageCleanupResourceModel{GroupID: types.StringValue("g1"), Enabled: types.BoolNull()})
	if !p.Enabled {
		t.Fatalf("got %+v, want enabled", p)
	}
	p = groupPackageCleanupToClient(groupPackageCleanupResourceModel{GroupID: types.StringValue("g1"), Enabled: types.BoolValue(false)})
	if p.Enabled {
		t.Fatalf("got %+v, want disabled", p)
	}
	if devicePackageCleanupMode(devicePackageCleanupResourceModel{Mode: types.StringNull()}) != "inherit" {
		t.Fatal("null mode must mean inherit")
	}
}

func TestPackageCleanupClientRoundTrip(t *testing.T) {
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
		switch r.URL.Path {
		case "/api/v1/customer/groups/g1/package-cleanup":
			_ = json.NewEncoder(w).Encode(map[string]any{"group_id": "g1", "enabled": false})
		case "/api/v1/customer/devices/d1/package-cleanup":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "d1", "mode": "off", "enabled": false, "source": "device_setting", "explanation": "Off — set on this device by api-key:terraform",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	c := client.New(server.URL, "k")
	ctx := context.Background()
	g, err := c.PutGroupPackageCleanup(ctx, "g1", client.GroupPackageCleanup{Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	if g.Enabled || bodies[0]["enabled"] != false {
		t.Fatalf("group %+v body %v", g, bodies[0])
	}
	d, err := c.PutDevicePackageCleanup(ctx, "d1", "off")
	if err != nil {
		t.Fatal(err)
	}
	if bodies[1]["mode"] != "off" {
		t.Fatalf("body %v", bodies[1])
	}
	m := devicePackageCleanupFromClient("d1", d)
	if m.Mode.ValueString() != "off" || m.Enabled.ValueBool() || m.Source.ValueString() != "device_setting" {
		t.Fatalf("model %+v", m)
	}
	if _, err := c.GetDevicePackageCleanup(ctx, "d1"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetGroupPackageCleanup(ctx, "g1"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"PUT /api/v1/customer/groups/g1/package-cleanup",
		"PUT /api/v1/customer/devices/d1/package-cleanup",
		"GET /api/v1/customer/devices/d1/package-cleanup",
		"GET /api/v1/customer/groups/g1/package-cleanup",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls %v", calls)
	}
}
