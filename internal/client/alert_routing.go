package client

import (
	"context"
	"net/http"
)

// NtfyPriorities is the ntfy `Priority` header per finding severity, plus
// one for scheduled digests: min, low, default, high or urgent. min/low
// arrive silently (no pop-up, no sound); high/urgent buzz the phone.
type NtfyPriorities struct {
	Critical string `json:"critical"`
	Warning  string `json:"warning"`
	Info     string `json:"info"`
	Digest   string `json:"digest"`
}

// AlertRouting mirrors `GET/PUT /api/v1/customer/alert-routing`: which
// finding severities each tenant-level destination gets, the ntfy
// priorities, and when digests go out. The webhook URL itself is set on
// the tenant page only (an ntfy topic URL is a secret).
type AlertRouting struct {
	NotifyEmail          bool           `json:"notify_email"`
	NotifyAllAdmins      bool           `json:"notify_all_admins"`
	ExtraRecipients      []string       `json:"extra_recipients"`
	EmailSeverities      []string       `json:"email_severities"`
	WebhookSeverities    []string       `json:"webhook_severities"`
	PagerdutySeverities  []string       `json:"pagerduty_severities"`
	NtfyPriorities       NtfyPriorities `json:"ntfy_priorities"`
	AlertDigestCadence   string         `json:"alert_digest_cadence"`
	SummaryDigestCadence string         `json:"summary_digest_cadence"`
	DigestHour           int64          `json:"digest_hour"`
	DigestTimezone       string         `json:"digest_timezone"`
}

// DefaultAlertRouting: email every admin with every severity, push
// destinations (webhook, PagerDuty) critical only, loud criticals and
// silent digests on ntfy, digests at 09:00 UTC.
func DefaultAlertRouting() AlertRouting {
	return AlertRouting{
		NotifyEmail:          true,
		NotifyAllAdmins:      true,
		ExtraRecipients:      []string{},
		EmailSeverities:      []string{"critical", "warning", "info"},
		WebhookSeverities:    []string{"critical"},
		PagerdutySeverities:  []string{"critical"},
		NtfyPriorities:       NtfyPriorities{Critical: "urgent", Warning: "low", Info: "min", Digest: "min"},
		AlertDigestCadence:   "every_sweep",
		SummaryDigestCadence: "weekly",
		DigestHour:           9,
		DigestTimezone:       "UTC",
	}
}

const alertRoutingPath = "/api/v1/customer/alert-routing"

func (a *AlertRouting) fillNils() {
	if a.ExtraRecipients == nil {
		a.ExtraRecipients = []string{}
	}
	if a.EmailSeverities == nil {
		a.EmailSeverities = []string{}
	}
	if a.WebhookSeverities == nil {
		a.WebhookSeverities = []string{}
	}
	if a.PagerdutySeverities == nil {
		a.PagerdutySeverities = []string{}
	}
}

func (c *Client) GetAlertRouting(ctx context.Context) (*AlertRouting, error) {
	var out AlertRouting
	if err := c.do(ctx, http.MethodGet, alertRoutingPath, nil, &out); err != nil {
		return nil, err
	}
	out.fillNils()
	return &out, nil
}

func (c *Client) PutAlertRouting(ctx context.Context, a AlertRouting) (*AlertRouting, error) {
	a.fillNils()
	var out AlertRouting
	if err := c.do(ctx, http.MethodPut, alertRoutingPath, a, &out); err != nil {
		return nil, err
	}
	out.fillNils()
	return &out, nil
}
