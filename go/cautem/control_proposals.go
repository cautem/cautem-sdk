package cautem

import (
	"context"
	"fmt"
	controlv1 "github.com/cautem/cautem-proto/gen/go/cautem/control/v1"
	"time"
)

func (c *Client) ListProposals(ctx context.Context, sandbox, status string) ([]Proposal, error) {
	conn, err := c.controlConn()
	if err != nil {
		return nil, err
	}
	client := controlv1.NewPolicyServiceClient(conn)
	after := ""
	var result []Proposal
	for {
		response, callErr := client.ListPolicyProposals(ctx, &controlv1.ListPolicyProposalsRequest{
			Workspace: c.workspace(), SandboxName: sandbox, Status: status, PageSize: 100, AfterId: after,
		})
		if callErr != nil {
			return nil, fmt.Errorf("list control proposals: %w", callErr)
		}
		for _, item := range response.GetProposals() {
			result = append(result, proposalFromSummary(item))
		}
		after = response.GetNextAfterId()
		if after == "" {
			return result, nil
		}
	}
}

// GetProposal returns one visible policy proposal through the Control API.
func (c *Client) GetProposal(ctx context.Context, sandbox, id string) (Proposal, error) {
	conn, err := c.controlConn()
	if err != nil {
		return Proposal{}, err
	}
	response, err := controlv1.NewPolicyServiceClient(conn).GetPolicyProposal(ctx, &controlv1.GetPolicyProposalRequest{
		Workspace: c.workspace(), Id: id,
	})
	if err != nil {
		return Proposal{}, fmt.Errorf("get control proposal %q: %w", id, err)
	}
	proposal := proposalFromSummary(response.GetProposal())
	if sandbox != "" && proposal.Sandbox != sandbox {
		return Proposal{}, fmt.Errorf("proposal %q not found in sandbox %q", id, sandbox)
	}
	return proposal, nil
}

// ApproveProposal approves a visible policy proposal through the Control API.
func (c *Client) ApproveProposal(ctx context.Context, sandbox, id string) (Proposal, error) {
	conn, err := c.controlConn()
	if err != nil {
		return Proposal{}, err
	}
	response, err := controlv1.NewPolicyServiceClient(conn).ApprovePolicyProposal(ctx, &controlv1.ApprovePolicyProposalRequest{
		Workspace: c.workspace(), Id: id,
	})
	if err != nil {
		return Proposal{}, fmt.Errorf("approve control proposal %q: %w", id, err)
	}
	proposal := proposalFromSummary(response.GetProposal())
	if sandbox != "" && proposal.Sandbox != sandbox {
		return Proposal{}, fmt.Errorf("proposal %q not found in sandbox %q", id, sandbox)
	}
	return proposal, nil
}

// RejectProposal rejects a visible policy proposal through the Control API.
func (c *Client) RejectProposal(ctx context.Context, sandbox, id, reason string) (Proposal, error) {
	conn, err := c.controlConn()
	if err != nil {
		return Proposal{}, err
	}
	response, err := controlv1.NewPolicyServiceClient(conn).RejectPolicyProposal(ctx, &controlv1.RejectPolicyProposalRequest{
		Workspace: c.workspace(), Id: id, Reason: reason,
	})
	if err != nil {
		return Proposal{}, fmt.Errorf("reject control proposal %q: %w", id, err)
	}
	proposal := proposalFromSummary(response.GetProposal())
	if sandbox != "" && proposal.Sandbox != sandbox {
		return Proposal{}, fmt.Errorf("proposal %q not found in sandbox %q", id, sandbox)
	}
	return proposal, nil
}

func proposalFromSummary(proposal *controlv1.PolicyProposalSummary) Proposal {
	if proposal == nil {
		return Proposal{}
	}
	return Proposal{
		ID: proposal.GetId(), Sandbox: proposal.GetSandboxName(), Status: proposal.GetStatus(),
		IntentSummary: proposal.GetIntentSummary(), RuleName: proposal.GetRuleName(),
		RuleYAML: proposal.GetRuleYaml(), Hosts: append([]string(nil), proposal.GetHosts()...),
		RejectionReason: proposal.GetRejectionReason(), ValidationResult: proposal.GetValidationResult(),
		SecurityFlagged: proposal.GetSecurityFlagged(),
		CreatedAt:       time.UnixMilli(proposal.GetCreatedAtUnixMs()),
		DecidedAt:       time.UnixMilli(proposal.GetDecidedAtUnixMs()),
	}
}
