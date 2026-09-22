package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"time"

	"interfaceisall/design"
	"interfaceisall/designer"
	"interfaceisall/internal/jsonfile"
)

// Proposal 是不可变的审阅证据，保存需求、修改意图和前后设计；自身没有确认效力。
type Proposal struct {
	// 可选字段保持旧提案的内容指纹兼容，标记外部交换来源。
	ExchangeRequestID string             `json:"request_id,omitempty"`
	ID                string             `json:"id"`
	CreatedAt         time.Time          `json:"created_at"`
	ParentID          string             `json:"parent_id,omitempty"`
	ExpectedHash      string             `json:"expected_draft_hash"`
	ExpectedRevision  string             `json:"expected_revision"`
	Before            design.Design      `json:"before"`
	Request           designer.Request   `json:"request"`
	Summary           string             `json:"summary"`
	After             design.Design      `json:"after"`
	Analysis          *designer.Analysis `json:"analysis,omitempty"`
	// 重新分析保留生成时的完整基准，审阅可分别对照已保存草稿与旧基准。
	Baseline *Snapshot `json:"baseline,omitempty"`
}

func proposalHash(p Proposal) string {
	p.ID = ""
	data, _ := json.Marshal(p)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// CheckVersions 读取当前草稿和确认版本；写入操作仍在同一把锁内重新核查。
func (s *Store) CheckVersions(hash, revision string) error {
	_, actual, err := s.Draft()
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	current, err := s.Confirmed()
	if err != nil && !errors.Is(err, ErrUnconfirmed) {
		return err
	}
	if actual != hash || current.Revision != revision {
		return ErrConflict
	}
	return nil
}

func (s *Store) SaveProposal(p Proposal) (Proposal, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	return s.SaveProposalContext(ctx, p)
}

// SaveProposalContext 在同一把写锁内复核源码、草稿、基准和当前分析来源。
func (s *Store) SaveProposalContext(ctx context.Context, p Proposal) (Proposal, error) {
	if err := design.ValidateProject(s.root, p.After); err != nil {
		return Proposal{}, err
	}
	unlock, err := s.lock()
	if err != nil {
		return Proposal{}, err
	}
	defer unlock()
	if err := s.CheckVersions(p.ExpectedHash, p.ExpectedRevision); err != nil {
		return Proposal{}, err
	}
	if err := ctx.Err(); err != nil {
		return Proposal{}, err
	}
	if p.ExchangeRequestID != "" {
		if err := s.checkProposalParent(p.ParentID, p.Request.Source != nil); err != nil {
			return Proposal{}, err
		}
	}
	requiresAcceptance := false
	if p.Request.Source != nil {
		if p.ExpectedRevision != "" {
			current, err := s.Confirmed()
			if err != nil {
				return Proposal{}, err
			}
			p.Baseline = &current
		}
		initial, err := s.InitialAnalysis()
		if err != nil {
			return Proposal{}, err
		}
		if initial != nil && !initial.Confirmed {
			requiresAcceptance = initial.RequiresAcceptance || initial.Accepted || initial.Accepting
			if initial.Withdrawn {
				if p.ParentID != "" {
					return Proposal{}, ErrConflict
				}
			} else if initial.Accepted || initial.Accepting || initial.ProposalID != p.ParentID {
				return Proposal{}, ErrConflict
			}
		} else if p.ParentID != "" {
			return Proposal{}, ErrConflict
		}
		if err := designer.ValidateAnalysis(*p.Request.Source, designer.Result{Summary: p.Summary, Design: p.After, Analysis: p.Analysis}); err != nil {
			return Proposal{}, err
		}
		if err := p.Request.Source.Verify(ctx, s.root); err != nil {
			return Proposal{}, err
		}
	}
	p.CreatedAt = time.Now().UTC()
	p.ID = proposalHash(p)
	name, err := s.file("proposals/" + p.ID + ".json")
	if err != nil {
		return Proposal{}, err
	}
	if err := jsonfile.Write(name, p); err != nil {
		return Proposal{}, err
	}
	if p.Request.Source != nil {
		if err := s.writeInitialAnalysis(InitialAnalysis{ProposalID: p.ID, RequiresAcceptance: requiresAcceptance}); err != nil {
			return Proposal{}, err
		}
	}
	if p.Request.Source == nil {
		if err := s.writeProposalState(proposalState{ProposalID: p.ID}); err != nil {
			return Proposal{}, err
		}
	}
	return p, nil
}

var proposalID = regexp.MustCompile(`^[a-f0-9]{64}$`)

func (s *Store) Proposal(id string) (Proposal, error) {
	if !proposalID.MatchString(id) {
		return Proposal{}, fmt.Errorf("提案 ID 无效")
	}
	name, err := s.file("proposals/" + id + ".json")
	if err != nil {
		return Proposal{}, err
	}
	data, err := os.ReadFile(name)
	if err != nil {
		return Proposal{}, err
	}
	var p Proposal
	if err := jsonfile.Decode(data, &p); err != nil {
		return Proposal{}, err
	}
	if p.ID != id || proposalHash(p) != id {
		return Proposal{}, fmt.Errorf("设计提案内容已损坏")
	}
	if err := design.Validate(p.After); err != nil {
		return Proposal{}, err
	}
	if p.Request.Source != nil {
		if p.ExpectedRevision != "" {
			if p.Baseline == nil || p.Baseline.Revision != p.ExpectedRevision {
				return Proposal{}, fmt.Errorf("重新分析缺少对应的原基准")
			}
			if err := p.Baseline.Validate(); err != nil {
				return Proposal{}, err
			}
		}
		if err := designer.ValidateAnalysis(*p.Request.Source, designer.Result{Summary: p.Summary, Design: p.After, Analysis: p.Analysis}); err != nil {
			return Proposal{}, err
		}
	}
	return p, nil
}

// AcceptProposal 只写草稿。并发窗口仍须使用生成时的版本，不能重放旧提案覆盖新内容。
func (s *Store) AcceptProposal(id string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	return s.AcceptProposalContext(ctx, id)
}

func (s *Store) AcceptProposalContext(ctx context.Context, id string) (string, error) {
	p, err := s.Proposal(id)
	if err != nil {
		return "", err
	}
	if p.Request.Source != nil {
		return s.acceptInitialAnalysis(ctx, p)
	}
	// 旧版本的普通提案没有共享审阅状态，仍可按原有版本保护接受。
	state, err := s.readProposalState()
	if err != nil {
		return "", err
	}
	if state.ProposalID == "" && p.ExchangeRequestID == "" {
		return s.SaveDraftIfUnchanged(p.After, p.ExpectedHash, p.ExpectedRevision)
	}
	return s.acceptDesignProposal(ctx, p)
}
