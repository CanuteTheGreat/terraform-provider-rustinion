package client

import (
	"context"
	"fmt"
	"net/http"
)

// EscalationStatus mirrors `routes::customer_api::
// CustomerEscalationStatus` — the read side of an acknowledge, letting
// a caller check whether a rule is currently escalating, at which step,
// and whether it's already acknowledged, without direct database
// access. Returned by GetEscalationStatus; ErrNotFound (via IsNotFound)
// means the rule has never escalated at all (no policy attached, or
// never fired), not necessarily that the rule itself doesn't exist —
// see that method's own doc comment.
type EscalationStatus struct {
	RuleKind         string  `json:"rule_kind"`
	RuleID           string  `json:"rule_id"`
	CurrentStep      int64   `json:"current_step"`
	StartedAt        string  `json:"started_at"`
	LastEscalatedAt  string  `json:"last_escalated_at"`
	AcknowledgedAt   *string `json:"acknowledged_at,omitempty"`
	AcknowledgedByID *string `json:"acknowledged_by,omitempty"`
	ResolvedAt       *string `json:"resolved_at,omitempty"`
}

// GetEscalationStatus reads a rule's current escalation state. A 404
// (via IsNotFound) means either the rule doesn't belong to this tenant
// or it has never had an escalation run started — the two cases the
// server itself doesn't disambiguate at the status-code level either
// (see `routes::customer_api::get_escalation_status`'s own doc
// comment).
func (c *Client) GetEscalationStatus(ctx context.Context, ruleKind, ruleID string) (*EscalationStatus, error) {
	var out EscalationStatus
	path := fmt.Sprintf("/api/v1/customer/escalations/%s/%s", ruleKind, ruleID)
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AcknowledgeEscalation stops further escalation for rule_id's current
// firing episode. A 409 (via IsConflict) means there was nothing active
// to acknowledge (already acknowledged, already resolved, or the rule
// has never escalated); a 404 (via IsNotFound) means the rule doesn't
// belong to this tenant.
func (c *Client) AcknowledgeEscalation(ctx context.Context, ruleKind, ruleID string) error {
	path := fmt.Sprintf("/api/v1/customer/escalations/%s/%s/acknowledge", ruleKind, ruleID)
	return c.do(ctx, http.MethodPost, path, nil, nil)
}
