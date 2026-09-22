package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"interfaceisall/design"
	"interfaceisall/designer"
	"interfaceisall/internal/jsonfile"
	"interfaceisall/source"
)

// DesignRequestOptions 只描述意图；源码、原设计及版本由程序从项目读取。
type DesignRequestOptions struct {
	Kind             string             `json:"kind"`
	Requirement      string             `json:"requirement"`
	Instruction      string             `json:"instruction"`
	Selection        designer.Selection `json:"selection"`
	ParentID         string             `json:"parent_id"`
	ExpectedHash     *string            `json:"expected_draft_hash,omitempty"`
	ExpectedRevision *string            `json:"expected_revision,omitempty"`
}

// DesignRequest 是持久化的交换凭据；项目路径和内容指纹防止跨项目或篡改输入。
type DesignRequest struct {
	SchemaVersion    int              `json:"schema_version"`
	RequestID        string           `json:"request_id"`
	Project          string           `json:"project"`
	Kind             string           `json:"kind"`
	ParentID         string           `json:"parent_id,omitempty"`
	ExpectedHash     string           `json:"expected_draft_hash"`
	ExpectedRevision string           `json:"expected_revision"`
	Before           design.Design    `json:"before"`
	Request          designer.Request `json:"request"`
	ResponseSchema   json.RawMessage  `json:"response_schema"`
}

// SourceContextError 保留失败时实际读取的范围，避免把不完整扫描当成可用请求。
type SourceContextError struct {
	Source source.Context
	Err    error
}

func (e *SourceContextError) Error() string { return e.Err.Error() }
func (e *SourceContextError) Unwrap() error { return e.Err }

func designRequestHash(r DesignRequest) string {
	r.RequestID = ""
	data, _ := json.Marshal(r)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// PrepareDesignRequest 仅采集并保存输入，不生成提案或改变草稿。
func (s *Store) PrepareDesignRequest(ctx context.Context, input DesignRequestOptions, tags string) (DesignRequest, error) {
	if input.Kind == "" {
		input.Kind = "design"
	}
	if input.Kind != "design" && input.Kind != "initial_analysis" && input.Kind != "reanalysis" {
		return DesignRequest{}, fmt.Errorf("请求 kind 必须为 design、initial_analysis 或 reanalysis")
	}
	before, hash, err := s.Draft()
	if err != nil && !os.IsNotExist(err) {
		return DesignRequest{}, err
	}
	hasDraft := err == nil
	current, err := s.Confirmed()
	if err != nil && !errors.Is(err, ErrUnconfirmed) {
		return DesignRequest{}, err
	}
	if !hasDraft {
		before = source.EmptyDesign()
		if current.Revision != "" {
			before = current.Design
		}
	}
	if input.ExpectedHash != nil && *input.ExpectedHash != hash || input.ExpectedRevision != nil && *input.ExpectedRevision != current.Revision {
		return DesignRequest{}, ErrConflict
	}
	if input.Kind == "initial_analysis" && current.Revision != "" || input.Kind == "reanalysis" && current.Revision == "" {
		return DesignRequest{}, ErrConflict
	}
	if input.Selection.Kind == "" {
		input.Selection = designer.Selection{Kind: "design"}
	}
	r := DesignRequest{SchemaVersion: 1, Project: s.root, Kind: input.Kind, ParentID: input.ParentID, ExpectedHash: hash, ExpectedRevision: current.Revision, Before: before,
		Request: designer.Request{Requirement: input.Requirement, Instruction: input.Instruction, Selection: input.Selection, Design: before}}
	if input.ParentID != "" {
		if input.Kind != "design" {
			return DesignRequest{}, fmt.Errorf("继续调整提案须使用 design 请求")
		}
		parent, err := s.Proposal(input.ParentID)
		if err != nil {
			return DesignRequest{}, err
		}
		if parent.ExpectedHash != hash || parent.ExpectedRevision != current.Revision {
			return DesignRequest{}, ErrConflict
		}
		r.Before, r.Request.Design, r.Request.Source = parent.Before, parent.After, parent.Request.Source
		if r.Request.Requirement == "" {
			r.Request.Requirement = parent.Request.Requirement
		}
	}
	if input.Kind != "design" {
		code, err := source.Collect(ctx, s.root, source.Options{Tags: tags})
		if err != nil {
			return DesignRequest{}, err
		}
		if err := code.Validate(); err != nil {
			return DesignRequest{}, &SourceContextError{Source: code, Err: err}
		}
		r.Request.Source = &code
		if r.Request.Requirement == "" {
			r.Request.Requirement = "忠实还原当前源码，单列问题、改进建议和不确定项"
		}
	}
	if strings.TrimSpace(r.Request.Requirement+r.Request.Instruction) == "" || len(r.Request.Requirement)+len(r.Request.Instruction) > 32000 {
		return DesignRequest{}, fmt.Errorf("请提供需求或修改意图，合计不超过 32000 字节")
	}
	if err := r.Request.Selection.Validate(r.Request.Design); err != nil {
		return DesignRequest{}, err
	}
	if err := design.ValidateProject(s.root, r.Request.Design); err != nil {
		return DesignRequest{}, err
	}
	if input.ParentID != "" && r.Request.Source != nil {
		if err := r.Request.Source.Verify(ctx, s.root); err != nil {
			return DesignRequest{}, err
		}
	}
	r.ResponseSchema = designer.ResponseSchema(r.Request.Source != nil)
	r.RequestID = designRequestHash(r)
	unlock, err := s.lock()
	if err != nil {
		return DesignRequest{}, err
	}
	defer unlock()
	if err := ctx.Err(); err != nil {
		return DesignRequest{}, err
	}
	if err := s.CheckVersions(hash, current.Revision); err != nil {
		return DesignRequest{}, err
	}
	if err := s.checkProposalParent(r.ParentID, r.Request.Source != nil); err != nil {
		return DesignRequest{}, err
	}
	name, err := s.file("requests/" + r.RequestID + ".json")
	if err != nil {
		return DesignRequest{}, err
	}
	if err := jsonfile.Write(name, r); err != nil {
		return DesignRequest{}, err
	}
	return r, nil
}

// DesignRequest 只加载本项目签发的请求；导入方不能用自造上下文替换它。
func (s *Store) DesignRequest(id string) (DesignRequest, error) {
	if !proposalID.MatchString(id) {
		return DesignRequest{}, fmt.Errorf("请求 ID 无效")
	}
	name, err := s.file("requests/" + id + ".json")
	if err != nil {
		return DesignRequest{}, err
	}
	data, err := os.ReadFile(name)
	if err != nil {
		return DesignRequest{}, fmt.Errorf("无法读取本项目的设计请求：%w", err)
	}
	var r DesignRequest
	if err := jsonfile.Decode(data, &r); err != nil {
		return DesignRequest{}, err
	}
	if r.SchemaVersion != 1 || r.Project != s.root || r.RequestID != id || designRequestHash(r) != id {
		return DesignRequest{}, fmt.Errorf("设计请求已损坏或属于其他项目")
	}
	return r, nil
}

// ImportProposal 读取已保存输入，复核响应并沿用提案存储的锁和来源保护。
func (s *Store) ImportProposal(ctx context.Context, data []byte) (Proposal, error) {
	var envelope designer.Response
	if err := jsonfile.Decode(data, &envelope); err != nil {
		return Proposal{}, err
	}
	r, err := s.DesignRequest(envelope.RequestID)
	if err != nil {
		return Proposal{}, err
	}
	response, err := designer.ParseResponse(data, r.Request)
	if err != nil {
		return Proposal{}, err
	}
	return s.SaveProposalContext(ctx, Proposal{ExchangeRequestID: r.RequestID, ParentID: r.ParentID, ExpectedHash: r.ExpectedHash, ExpectedRevision: r.ExpectedRevision,
		Before: r.Before, Request: r.Request, Summary: response.Summary, After: response.Design, Analysis: response.Analysis})
}

// checkProposalParent 在写锁内阻止并行请求覆盖当前待审阅提案。
func (s *Store) checkProposalParent(parent string, sourceBased bool) error {
	initial, err := s.InitialAnalysis()
	if err != nil {
		return err
	}
	if initial != nil && !initial.Confirmed && !initial.Withdrawn {
		if !sourceBased || initial.Accepted || initial.Accepting || initial.ProposalID != parent {
			return ErrConflict
		}
		return nil
	}
	if sourceBased && parent != "" {
		return ErrConflict
	}
	pending, err := s.PendingDesignProposal()
	if err != nil {
		return err
	}
	if pending != parent {
		return ErrConflict
	}
	return nil
}
