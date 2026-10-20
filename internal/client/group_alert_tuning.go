package client

import (
	"context"
	"fmt"
	"net/http"
)

// AlertTuning mirrors the server's alert tuning response
// (`GET/PUT /api/v1/customer/groups/{id}/alert-tuning`): the tuning keys
// set on the group (`settings`, absent key = built-in default) and the
// resolved values. Keys: {cpu,memory,disk}_alert_{enabled,threshold_pct,
// critical_pct,duration_minutes,hysteresis_pct}, oom_alert_scope,
// reason_delivery, role_preset.
type AlertTuning struct {
	GroupID   string         `json:"group_id"`
	Settings  map[string]any `json:"settings"`
	Effective map[string]any `json:"effective"`
}

// AlertTuningKeys lists every key the server owns, so a full replace can
// clear the ones a config leaves out (null = back to the default).
var AlertTuningKeys = func() []string {
	keys := []string{}
	for _, m := range []string{"cpu", "memory", "disk"} {
		for _, f := range []string{"enabled", "threshold_pct", "critical_pct", "duration_minutes", "hysteresis_pct"} {
			keys = append(keys, m+"_alert_"+f)
		}
	}
	return append(keys, "oom_alert_scope", "reason_delivery", "role_preset")
}()

func groupAlertTuningPath(groupID string) string {
	return fmt.Sprintf("/api/v1/customer/groups/%s/alert-tuning", groupID)
}

func (c *Client) GetGroupAlertTuning(ctx context.Context, groupID string) (*AlertTuning, error) {
	var out AlertTuning
	if err := c.do(ctx, http.MethodGet, groupAlertTuningPath(groupID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ReplaceGroupAlertTuning sets exactly `settings`: every other tuning key
// is cleared.
func (c *Client) ReplaceGroupAlertTuning(ctx context.Context, groupID string, settings map[string]any) (*AlertTuning, error) {
	body := map[string]any{}
	for _, k := range AlertTuningKeys {
		body[k] = nil
	}
	for k, v := range settings {
		body[k] = v
	}
	var out AlertTuning
	if err := c.do(ctx, http.MethodPut, groupAlertTuningPath(groupID), body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
