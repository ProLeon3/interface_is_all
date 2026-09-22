package workbench

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/ProLeon3/interface_is_all/store"
)

// designRequest 只导出已绑定版本的上下文，不启动模型或 agent。
func (s *Server) designRequest(w http.ResponseWriter, r *http.Request) {
	var input store.DesignRequestOptions
	if !decode(w, r, &input) {
		return
	}
	if input.ExpectedHash == nil || input.ExpectedRevision == nil {
		writeError(w, 400, "导出请求必须提供当前草稿和基准版本", nil)
		return
	}
	ctx, release, ok := s.startScan(w, r)
	if !ok {
		return
	}
	defer release()
	result, err := s.store.PrepareDesignRequest(ctx, input, s.options.Tags)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, result)
}

// importProposal 与命令行使用相同的校验及保存路径。
func (s *Server) importProposal(w http.ResponseWriter, r *http.Request) {
	var data json.RawMessage
	if !decode(w, r, &data) {
		return
	}
	ctx, release, ok := s.startScan(w, r)
	if !ok {
		return
	}
	defer release()
	p, err := s.store.ImportProposal(ctx, data)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, p)
}

func (s *Server) discardProposal(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DiscardDesignProposal(r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, map[string]bool{"discarded": true})
}

func (s *Server) proposal(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.Proposal(r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, p)
}

func (s *Server) acceptProposal(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), s.options.Timeout)
	defer cancel()
	hash, err := s.store.AcceptProposalContext(ctx, r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, map[string]string{"draft_hash": hash})
}

// 交接从磁盘读取已确认快照，不接受浏览器提交的草稿或待审阅提案。
func (s *Server) handoff(w http.ResponseWriter, r *http.Request) {
	current, err := s.store.Confirmed()
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, map[string]any{"schema_version": 1, "status": "confirmed", "baseline": current,
		"instructions": "请按 baseline.design 实现功能，保留模块职责、接口含义和协作约定。设计需要调整时先请用户确认；不要为迁就代码改写基准。完成后运行 archdesign check，并结合代码审阅核对具体接口协作。未画出的关系不自动违规，Go 包导入检查不能证明运行时接口调用符合设计。业务测试与验收由开发流程完成。"})
}
