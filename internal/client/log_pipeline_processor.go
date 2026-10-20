package client

import (
	"context"
	"encoding/json"
	"net/http"
)

// LogPipelineProcessor mirrors
// `routes::customer_api::LogPipelineProcessorInput`/
// `log_pipeline::LogPipelineProcessorRow`. Tenant-wide — there is no
// `group_id` on this resource at all, the same as LogMetricDefinition
// (see the Rust side's own `0116_log_pipeline_processors.sql` comment).
//
// `Config`'s shape depends on `ProcessorType` (grok_parse takes
// {"pattern": ...}, remap takes {"from": ..., "to": ...}, both validated
// server-side by `log_pipeline::validate_processor_input`), so it's left
// as raw JSON here for the same reason `DashboardWidget.Config` is.
type LogPipelineProcessor struct {
	ID            string          `json:"id,omitempty"`
	Name          string          `json:"name"`
	ProcessorType string          `json:"processor_type"`
	Config        json.RawMessage `json:"config"`
	// SourceFilter restricts the processor to log lines whose `source`
	// matches exactly. A nil pointer means every source — a pointer, not
	// a plain string, so "unset" stays distinguishable from "match the
	// empty source".
	SourceFilter *string `json:"source_filter"`
	// Position orders the pipeline: processors run in ascending
	// position, so a remap can act on what an earlier grok_parse
	// extracted.
	Position int64 `json:"position"`
	Enabled  bool  `json:"enabled"`
}

func (c *Client) CreateLogPipelineProcessor(ctx context.Context, p LogPipelineProcessor) (*LogPipelineProcessor, error) {
	var out LogPipelineProcessor
	if err := c.do(ctx, http.MethodPost, "/api/v1/customer/log-pipeline-processors", p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetLogPipelineProcessor(ctx context.Context, id string) (*LogPipelineProcessor, error) {
	var out LogPipelineProcessor
	if err := c.do(ctx, http.MethodGet, "/api/v1/customer/log-pipeline-processors/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateLogPipelineProcessor(ctx context.Context, id string, p LogPipelineProcessor) (*LogPipelineProcessor, error) {
	var out LogPipelineProcessor
	if err := c.do(ctx, http.MethodPut, "/api/v1/customer/log-pipeline-processors/"+id, p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteLogPipelineProcessor(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/customer/log-pipeline-processors/"+id, nil, nil)
}
