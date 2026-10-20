package client

import (
	"context"
	"fmt"
	"net/http"
)

// GroupOfflineAlerts mirrors `GET/PUT /api/v1/customer/groups/{id}/offline-alerts`:
// the "device not responding" alert (`device_unreachable`). A device whose
// agent hasn't reached Rustinion for WarningAfterMinutes raises a warning,
// after CriticalAfterMinutes a critical. A per-group singleton that always
// exists (defaults: on, 10 and 30 minutes).
type GroupOfflineAlerts struct {
	Enabled              bool  `json:"enabled"`
	WarningAfterMinutes  int64 `json:"warning_after_minutes"`
	CriticalAfterMinutes int64 `json:"critical_after_minutes"`
	// AlertsEnabled is the group's master alert switch (read-only here).
	AlertsEnabled *bool `json:"alerts_enabled,omitempty"`
}

// DefaultGroupOfflineAlerts is what the server applies to a group that
// never changed these settings.
var DefaultGroupOfflineAlerts = GroupOfflineAlerts{
	Enabled:              true,
	WarningAfterMinutes:  10,
	CriticalAfterMinutes: 30,
}

func groupOfflineAlertsPath(groupID string) string {
	return fmt.Sprintf("/api/v1/customer/groups/%s/offline-alerts", groupID)
}

func (c *Client) GetGroupOfflineAlerts(ctx context.Context, groupID string) (*GroupOfflineAlerts, error) {
	var out GroupOfflineAlerts
	if err := c.do(ctx, http.MethodGet, groupOfflineAlertsPath(groupID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) PutGroupOfflineAlerts(ctx context.Context, groupID string, p GroupOfflineAlerts) (*GroupOfflineAlerts, error) {
	body := map[string]any{
		"enabled":                p.Enabled,
		"warning_after_minutes":  p.WarningAfterMinutes,
		"critical_after_minutes": p.CriticalAfterMinutes,
	}
	var out GroupOfflineAlerts
	if err := c.do(ctx, http.MethodPut, groupOfflineAlertsPath(groupID), body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GroupHostHealth mirrors `GET/PUT /api/v1/customer/groups/{id}/host-health`:
// the swap alert (`swap_in_use`) and the failed services that never raise
// `service_failed` (exact names, or a prefix ending in `*`). Full-filesystem
// levels follow the group's disk alert tuning (rustinion_group_alert_tuning).
type GroupHostHealth struct {
	SwapAlertEnabled      bool     `json:"swap_alert_enabled"`
	SwapAlertThresholdPct int64    `json:"swap_alert_threshold_pct"`
	IgnoredFailedServices []string `json:"ignored_failed_services"`
}

// DefaultGroupHostHealth: swap alert on at 80%, no ignored services.
func DefaultGroupHostHealth() GroupHostHealth {
	return GroupHostHealth{SwapAlertEnabled: true, SwapAlertThresholdPct: 80, IgnoredFailedServices: []string{}}
}

func groupHostHealthPath(groupID string) string {
	return fmt.Sprintf("/api/v1/customer/groups/%s/host-health", groupID)
}

func (c *Client) GetGroupHostHealth(ctx context.Context, groupID string) (*GroupHostHealth, error) {
	var out GroupHostHealth
	if err := c.do(ctx, http.MethodGet, groupHostHealthPath(groupID), nil, &out); err != nil {
		return nil, err
	}
	if out.IgnoredFailedServices == nil {
		out.IgnoredFailedServices = []string{}
	}
	return &out, nil
}

func (c *Client) PutGroupHostHealth(ctx context.Context, groupID string, p GroupHostHealth) (*GroupHostHealth, error) {
	if p.IgnoredFailedServices == nil {
		p.IgnoredFailedServices = []string{}
	}
	var out GroupHostHealth
	if err := c.do(ctx, http.MethodPut, groupHostHealthPath(groupID), p, &out); err != nil {
		return nil, err
	}
	if out.IgnoredFailedServices == nil {
		out.IgnoredFailedServices = []string{}
	}
	return &out, nil
}
