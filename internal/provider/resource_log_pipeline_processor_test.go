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

func TestAccLogPipelineProcessorResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckLogPipelineProcessorDestroy,
		Steps: []resource.TestStep{
			// Create + Read: a grok_parse stage with everything else
			// left at its default.
			{
				Config: `
resource "rustinion_log_pipeline_processor" "test" {
  name           = "tf_acc_parse_access_log"
  processor_type = "grok_parse"
  config         = jsonencode({ pattern = "%%{IPV4:client_ip} %%{HTTPMETHOD:method} %%{URIPATH:path} %%{INT:status:int}" })
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rustinion_log_pipeline_processor.test", "id"),
					resource.TestCheckResourceAttr("rustinion_log_pipeline_processor.test", "name", "tf_acc_parse_access_log"),
					resource.TestCheckResourceAttr("rustinion_log_pipeline_processor.test", "processor_type", "grok_parse"),
					resource.TestCheckResourceAttr("rustinion_log_pipeline_processor.test", "position", "0"),
					resource.TestCheckResourceAttr("rustinion_log_pipeline_processor.test", "enabled", "true"),
					resource.TestCheckNoResourceAttr("rustinion_log_pipeline_processor.test", "source_filter"),
				),
			},
			// Import.
			{
				ResourceName:      "rustinion_log_pipeline_processor.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update: switch the whole resource to a remap stage, set a
			// source filter and a real position, and disable it.
			{
				Config: `
resource "rustinion_log_pipeline_processor" "test" {
  name           = "tf_acc_rename_status"
  processor_type = "remap"
  config         = jsonencode({ from = "status", to = "http_status" })
  source_filter  = "nginx"
  position       = 5
  enabled        = false
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rustinion_log_pipeline_processor.test", "name", "tf_acc_rename_status"),
					resource.TestCheckResourceAttr("rustinion_log_pipeline_processor.test", "processor_type", "remap"),
					resource.TestCheckResourceAttr("rustinion_log_pipeline_processor.test", "source_filter", "nginx"),
					resource.TestCheckResourceAttr("rustinion_log_pipeline_processor.test", "position", "5"),
					resource.TestCheckResourceAttr("rustinion_log_pipeline_processor.test", "enabled", "false"),
				),
			},
			// Re-import at the new shape: catches a field that writes
			// but doesn't read back.
			{
				ResourceName:      "rustinion_log_pipeline_processor.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccCheckLogPipelineProcessorDestroy(s *terraform.State) error {
	c := testAccAPIClient()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "rustinion_log_pipeline_processor" {
			continue
		}
		_, err := c.GetLogPipelineProcessor(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("log pipeline processor %s still exists", rs.Primary.ID)
		}
		if !client.IsNotFound(err) {
			return err
		}
	}
	return nil
}

// TestLogPipelineProcessorToClientSourceFilter covers what the
// acceptance test can't reach without a live server: an unset
// source_filter must serialize as an explicit null (the server reads
// that as "every source"), never as an empty string that would
// otherwise mean "a source literally named ”".
func TestLogPipelineProcessorToClientSourceFilter(t *testing.T) {
	m := logPipelineProcessorResourceModel{
		Name:          types.StringValue("p"),
		ProcessorType: types.StringValue("grok_parse"),
		Config:        jsontypes.NewNormalizedValue(`{"pattern":"%{WORD:a}"}`),
		SourceFilter:  types.StringNull(),
		Position:      types.Int64Value(3),
		Enabled:       types.BoolValue(true),
	}
	p := logPipelineProcessorToClient(m)
	if p.SourceFilter != nil {
		t.Fatalf("expected a nil source filter, got %q", *p.SourceFilter)
	}
	body, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"source_filter":null`) {
		t.Fatalf("an unset source filter must serialize as null: %s", body)
	}

	m.SourceFilter = types.StringValue("nginx")
	p = logPipelineProcessorToClient(m)
	if p.SourceFilter == nil || *p.SourceFilter != "nginx" {
		t.Fatalf("expected nginx, got %v", p.SourceFilter)
	}
}

// TestLogPipelineProcessorToClientDefaultsConfig: an omitted config
// must still be valid JSON on the wire, so the server's own validation
// (not a JSON parse error) is what rejects it.
func TestLogPipelineProcessorToClientDefaultsConfig(t *testing.T) {
	m := logPipelineProcessorResourceModel{
		Name:          types.StringValue("p"),
		ProcessorType: types.StringValue("grok_parse"),
		Config:        jsontypes.NewNormalizedValue(""),
	}
	if got := string(logPipelineProcessorToClient(m).Config); got != "{}" {
		t.Fatalf("expected {}, got %q", got)
	}
}

// TestLogPipelineProcessorFromClientBlankSourceFilter: the server
// normalises a blank source_filter to SQL NULL, and an older server
// might return "" — both must read back as unset, or Terraform would
// show a permanent diff against a config that never set the field.
func TestLogPipelineProcessorFromClientBlankSourceFilter(t *testing.T) {
	blank := ""
	p := &client.LogPipelineProcessor{
		ID:            "id",
		Name:          "p",
		ProcessorType: "grok_parse",
		Config:        json.RawMessage(`{"pattern":"%{WORD:a}"}`),
		SourceFilter:  &blank,
		Position:      2,
		Enabled:       true,
	}
	if m := logPipelineProcessorFromClient(p); !m.SourceFilter.IsNull() {
		t.Fatalf("a blank source filter should read back as null, got %v", m.SourceFilter)
	}

	p.SourceFilter = nil
	if m := logPipelineProcessorFromClient(p); !m.SourceFilter.IsNull() {
		t.Fatalf("a nil source filter should read back as null, got %v", m.SourceFilter)
	}

	filter := "nginx"
	p.SourceFilter = &filter
	m := logPipelineProcessorFromClient(p)
	if got := m.SourceFilter.ValueString(); got != "nginx" {
		t.Fatalf("expected nginx, got %q", got)
	}
	if got := m.Position.ValueInt64(); got != 2 {
		t.Fatalf("expected position 2, got %d", got)
	}
}
