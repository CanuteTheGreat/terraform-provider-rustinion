package client

import (
	"context"
	"fmt"
	"net/http"
)

// ScrapeTarget is one Prometheus/OpenMetrics endpoint the agents of a group
// scrape (fetched by each device itself, so localhost = the device).
type ScrapeTarget struct {
	URL          string            `json:"url"`
	Metrics      []string          `json:"metrics"`
	Namespace    *string           `json:"namespace"`
	IntervalSecs int64             `json:"interval_secs"`
	MaxSeries    int64             `json:"max_series"`
	Tags         map[string]string `json:"tags"`
}

// GroupCustomMetrics mirrors `GET/PUT /api/v1/customer/groups/{id}/custom-metrics`:
// the agent's DogStatsD/StatsD listener and OpenMetrics scrape targets.
// `monitoring_enabled` is read-only (custom metrics are only stored while the
// group's basic monitoring is on) and never sent.
type GroupCustomMetrics struct {
	Enabled           bool           `json:"enabled"`
	StatsdEnabled     bool           `json:"statsd_enabled"`
	StatsdAddress     string         `json:"statsd_address"`
	StatsdSocket      *string        `json:"statsd_socket"`
	FlushIntervalSecs int64          `json:"flush_interval_secs"`
	ScrapeTargets     []ScrapeTarget `json:"scrape_targets"`
	MonitoringEnabled bool           `json:"monitoring_enabled,omitempty"`
}

// DefaultGroupCustomMetrics: off, listener on 127.0.0.1:8125 when switched
// on, 60 s flush, no scrape targets.
func DefaultGroupCustomMetrics() GroupCustomMetrics {
	return GroupCustomMetrics{
		Enabled:           false,
		StatsdEnabled:     true,
		StatsdAddress:     "127.0.0.1:8125",
		FlushIntervalSecs: 60,
		ScrapeTargets:     []ScrapeTarget{},
	}
}

func groupCustomMetricsPath(groupID string) string {
	return fmt.Sprintf("/api/v1/customer/groups/%s/custom-metrics", groupID)
}

func normalizeCustomMetrics(p *GroupCustomMetrics) {
	if p.ScrapeTargets == nil {
		p.ScrapeTargets = []ScrapeTarget{}
	}
	for i := range p.ScrapeTargets {
		if p.ScrapeTargets[i].Metrics == nil {
			p.ScrapeTargets[i].Metrics = []string{}
		}
		if p.ScrapeTargets[i].Tags == nil {
			p.ScrapeTargets[i].Tags = map[string]string{}
		}
	}
}

func (c *Client) GetGroupCustomMetrics(ctx context.Context, groupID string) (*GroupCustomMetrics, error) {
	var out GroupCustomMetrics
	if err := c.do(ctx, http.MethodGet, groupCustomMetricsPath(groupID), nil, &out); err != nil {
		return nil, err
	}
	normalizeCustomMetrics(&out)
	return &out, nil
}

func (c *Client) PutGroupCustomMetrics(ctx context.Context, groupID string, p GroupCustomMetrics) (*GroupCustomMetrics, error) {
	normalizeCustomMetrics(&p)
	// Read-only on the server; never send it.
	p.MonitoringEnabled = false
	var out GroupCustomMetrics
	if err := c.do(ctx, http.MethodPut, groupCustomMetricsPath(groupID), p, &out); err != nil {
		return nil, err
	}
	normalizeCustomMetrics(&out)
	return &out, nil
}
