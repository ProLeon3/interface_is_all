package store

import (
	"context"
	"errors"
	"os"

	"github.com/ProLeon3/interface_is_all/design"
	"github.com/ProLeon3/interface_is_all/internal/jsonfile"
)

// 普通设计提案的审阅状态独立保存，外部导入后新窗口也能发现。
type proposalState struct {
	ProposalID string `json:"proposal_id"`
	Accepting  bool   `json:"accepting,omitempty"`
	Accepted   bool   `json:"accepted,omitempty"`
	Withdrawn  bool   `json:"withdrawn,omitempty"`
}

func (s *Store) readProposalState() (proposalState, error) {
	name, err := s.file("design-proposal.json")
	if err != nil {
		return proposalState{}, err
	}
	data, err := os.ReadFile(name)
	if os.IsNotExist(err) {
		return proposalState{}, nil
	}
	if err != nil {
		return proposalState{}, err
	}
	var state proposalState
	if err := jsonfile.Decode(data, &state); err != nil {
		return state, err
	}
	if _, err := s.Proposal(state.ProposalID); err != nil {
		return state, err
	}
	return state, nil
}

func (s *Store) writeProposalState(state proposalState) error {
	name, err := s.file("design-proposal.json")
	if err != nil {
		return err
	}
	return jsonfile.Write(name, state)
}

func (s *Store) PendingDesignProposal() (string, error) {
	state, err := s.readProposalState()
	if err != nil {
		return "", err
	}
	if state.Accepted || state.Withdrawn {
		return "", nil
	}
	return state.ProposalID, nil
}

// acceptDesignProposal 先记录接受中，允许恢复写入中断；不会发布确认版本。
func (s *Store) acceptDesignProposal(ctx context.Context, p Proposal) (string, error) {
	if err := design.ValidateProject(s.root, p.After); err != nil {
		return "", err
	}
	unlock, err := s.lock()
	if err != nil {
		return "", err
	}
	defer unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	state, err := s.readProposalState()
	if err != nil {
		return "", err
	}
	if state.ProposalID != p.ID || state.Accepted || state.Withdrawn {
		return "", ErrConflict
	}
	if err := s.CheckVersions(p.ExpectedHash, p.ExpectedRevision); err != nil {
		if !state.Accepting || !errors.Is(err, ErrConflict) {
			return "", err
		}
		if err := s.CheckVersions(design.Fingerprint(p.After), p.ExpectedRevision); err != nil {
			return "", err
		}
	}
	state.Accepting = true
	if err := s.writeProposalState(state); err != nil {
		return "", err
	}
	hash, err := s.writeDraft(p.After)
	if err != nil {
		return "", err
	}
	state.Accepting, state.Accepted = false, true
	if err := s.writeProposalState(state); err != nil {
		return "", err
	}
	return hash, nil
}

// DiscardDesignProposal 仅撤回指定提案，避免旧窗口撤回其他窗口的新提案。
func (s *Store) DiscardDesignProposal(id string) error {
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()
	state, err := s.readProposalState()
	if err != nil {
		return err
	}
	if state.ProposalID != id || state.Accepted || state.Withdrawn {
		return ErrConflict
	}
	state.Withdrawn = true
	return s.writeProposalState(state)
}
