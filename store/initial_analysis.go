package store

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/ProLeon3/interface_is_all/design"
	"github.com/ProLeon3/interface_is_all/internal/jsonfile"
)

// InitialAnalysis 兼容原首次接入记录，也保存后续源码分析的当前来源。
// 提案本体不可变；接受状态不等于设计确认状态，旧文件名继续保留。
type InitialAnalysis struct {
	ProposalID string `json:"proposal_id"`
	Accepted   bool   `json:"accepted"`
	// 已接受的来源撤回后仍保留标记，旧草稿必须经新分析审阅，不能绕过源码复核。
	Withdrawn bool `json:"withdrawn,omitempty"`
	// 草稿与状态是两个文件；先记录接受中，崩溃后仍能恢复或安全撤回来源。
	Accepting bool `json:"accepting,omitempty"`
	// 新提案继承旧草稿的来源保护，撤回新提案也不能清除尚未确认的旧来源。
	RequiresAcceptance bool `json:"requires_acceptance,omitempty"`
	// 由当前确认快照推导，不依赖确认后再写一个完成标记，避免中断造成歧义。
	Confirmed bool `json:"confirmed,omitempty"`
}

func (s *Store) InitialAnalysis() (*InitialAnalysis, error) {
	name, err := s.file("initial-analysis.json")
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(name)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var initial InitialAnalysis
	if err := jsonfile.Decode(data, &initial); err != nil {
		return nil, err
	}
	p, err := s.Proposal(initial.ProposalID)
	if err != nil {
		return nil, fmt.Errorf("源码分析记录损坏：%w", err)
	}
	if p.Request.Source == nil {
		return nil, fmt.Errorf("分析记录缺少源码来源")
	}
	if initial.Withdrawn && !initial.Accepted && !initial.RequiresAcceptance {
		return nil, fmt.Errorf("源码分析撤回状态无效")
	}
	if initial.Accepting && initial.Accepted {
		return nil, fmt.Errorf("源码分析接受状态无效")
	}
	current, err := s.Confirmed()
	if err != nil && !errors.Is(err, ErrUnconfirmed) {
		return nil, err
	}
	initial.Confirmed = current.SourceAnalysisID == p.ID || current.InitialAnalysisID == p.ID
	return &initial, nil
}

func (s *Store) writeInitialAnalysis(initial InitialAnalysis) error {
	name, err := s.file("initial-analysis.json")
	if err != nil {
		return err
	}
	return jsonfile.Write(name, initial)
}

func (s *Store) acceptInitialAnalysis(ctx context.Context, p Proposal) (string, error) {
	if err := design.ValidateProject(s.root, p.After); err != nil {
		return "", err
	}
	unlock, err := s.lock()
	if err != nil {
		return "", err
	}
	defer unlock()
	initial, err := s.InitialAnalysis()
	if err != nil {
		return "", err
	}
	if initial == nil || initial.ProposalID != p.ID || initial.Accepted || initial.Withdrawn || initial.Confirmed {
		return "", ErrConflict
	}
	if err := s.CheckVersions(p.ExpectedHash, p.ExpectedRevision); err != nil {
		// 仅恢复本次接受已写入的相同草稿，其他窗口的新编辑仍会冲突。
		if !initial.Accepting || !errors.Is(err, ErrConflict) {
			return "", err
		}
		if err := s.CheckVersions(design.Fingerprint(p.After), p.ExpectedRevision); err != nil {
			return "", err
		}
	}
	if err := p.Request.Source.Verify(ctx, s.root); err != nil {
		return "", err
	}
	initial.Accepting = true
	if err := s.writeInitialAnalysis(*initial); err != nil {
		return "", err
	}
	hash, err := s.writeDraft(p.After)
	if err != nil {
		return "", err
	}
	// 中间失败保留 Accepting；重试能补全，撤回也不能删除已经写入草稿的来源。
	initial.Accepted = true
	initial.Accepting = false
	if err := s.writeInitialAnalysis(*initial); err != nil {
		return "", err
	}
	return hash, nil
}

// DiscardInitialAnalysis 保留草稿和不可变提案；已接受来源只标记撤回，直到新分析替换。
func (s *Store) DiscardInitialAnalysis(id, hash, revision string) error {
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()
	if err := s.CheckVersions(hash, revision); err != nil {
		return err
	}
	initial, err := s.InitialAnalysis()
	if err != nil {
		return err
	}
	if initial == nil || initial.ProposalID != id || initial.Confirmed {
		return ErrConflict
	}
	if initial.Accepted || initial.Accepting || initial.RequiresAcceptance {
		initial.Accepted = initial.Accepted || initial.Accepting
		initial.Accepting = false
		initial.Withdrawn = true
		return s.writeInitialAnalysis(*initial)
	}
	name, err := s.file("initial-analysis.json")
	if err != nil {
		return err
	}
	return os.Remove(name)
}
