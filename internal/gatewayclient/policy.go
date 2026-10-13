package gatewayclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// PolicyRevisionMeta is metadata for policy list (no YAML body).
type PolicyRevisionMeta struct {
	Rev       int       `json:"rev"`
	UpdatedAt time.Time `json:"updated_at"`
	Bytes     int       `json:"bytes"`
	Status    string    `json:"status"`
}

// Proposal is a policy.local chunk stored on the gateway.
type Proposal struct {
	ID               string    `json:"id"`
	Sandbox          string    `json:"sandbox"`
	Status           string    `json:"status"`
	IntentSummary    string    `json:"intent_summary,omitempty"`
	RuleName         string    `json:"rule_name,omitempty"`
	RuleYAML         string    `json:"rule_yaml,omitempty"`
	Hosts            []string  `json:"hosts,omitempty"`
	RejectionReason  string    `json:"rejection_reason,omitempty"`
	ValidationResult string    `json:"validation_result,omitempty"`
	SecurityFlagged  bool      `json:"security_flagged,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	DecidedAt        time.Time `json:"decided_at"`
}

// ListProposals GET /v1/sandboxes/{name}/proposals.
func (c *Client) ListProposals(ctx context.Context, sandbox, status string) ([]Proposal, error) {
	path := "/v1/sandboxes/" + url.PathEscape(sandbox) + "/proposals"
	if status != "" {
		path += "?status=" + url.QueryEscape(status)
	}
	var out struct {
		Proposals []Proposal `json:"proposals"`
	}
	if err := c.get(ctx, path, &out); err != nil {
		return nil, err
	}
	return out.Proposals, nil
}

// GetProposal GET /v1/sandboxes/{name}/proposals/{id}.
func (c *Client) GetProposal(ctx context.Context, sandbox, id string) (Proposal, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+"/v1/sandboxes/"+url.PathEscape(sandbox)+"/proposals/"+url.PathEscape(id), nil)
	if err != nil {
		return Proposal{}, err
	}
	c.auth(req)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return Proposal{}, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		err = fmt.Errorf("gateway read response body: %w", err)
		return Proposal{}, err
	}
	if res.StatusCode >= 300 {
		return Proposal{}, fmt.Errorf("get proposal: %s: %s", res.Status, bytes.TrimSpace(body))
	}
	var p Proposal
	if err := json.Unmarshal(body, &p); err != nil {
		return Proposal{}, err
	}
	return p, nil
}

// ApproveProposal POST /v1/sandboxes/{name}/proposals/{id}/approve — merges rule into base.
func (c *Client) ApproveProposal(ctx context.Context, sandbox, id string) (Proposal, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Base+"/v1/sandboxes/"+url.PathEscape(sandbox)+"/proposals/"+url.PathEscape(id)+"/approve", nil)
	if err != nil {
		return Proposal{}, err
	}
	c.auth(req)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return Proposal{}, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		err = fmt.Errorf("gateway read response body: %w", err)
		return Proposal{}, err
	}
	if res.StatusCode >= 300 {
		return Proposal{}, fmt.Errorf("approve proposal: %s: %s", res.Status, bytes.TrimSpace(body))
	}
	var p Proposal
	if err := json.Unmarshal(body, &p); err != nil {
		return Proposal{}, err
	}
	return p, nil
}

// RejectProposal POST /v1/sandboxes/{name}/proposals/{id}/reject.
func (c *Client) RejectProposal(ctx context.Context, sandbox, id, reason string) (Proposal, error) {
	b, err := json.Marshal(map[string]string{"reason": reason})
	if err != nil {
		err = fmt.Errorf("gateway encode request: %w", err)
		return Proposal{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Base+"/v1/sandboxes/"+url.PathEscape(sandbox)+"/proposals/"+url.PathEscape(id)+"/reject", bytes.NewReader(b))
	if err != nil {
		return Proposal{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	c.auth(req)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return Proposal{}, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		err = fmt.Errorf("gateway read response body: %w", err)
		return Proposal{}, err
	}
	if res.StatusCode >= 300 {
		return Proposal{}, fmt.Errorf("reject proposal: %s: %s", res.Status, bytes.TrimSpace(body))
	}
	var p Proposal
	if err := json.Unmarshal(body, &p); err != nil {
		return Proposal{}, err
	}
	return p, nil
}
