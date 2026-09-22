package main

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"interfaceisall/design"
	"interfaceisall/designer"
	"interfaceisall/internal/testproject"
	"interfaceisall/source"
	"interfaceisall/store"
)

func TestCLIExternalProposalWorkflow(t *testing.T) {
	root := t.TempDir()
	testproject.Write(t, root, "go.mod", "module example.test/tiny\n\ngo 1.22\n")
	testproject.Write(t, root, "core/core.go", "// Package core 提供示例能力。\npackage core\nfunc Run() {}\n")
	var r store.DesignRequest
	json.Unmarshal(invoke(t, 0, "design-request", "-project", root, "-kind", "initial_analysis"), &r)
	if r.Request.Source == nil || r.RequestID == "" {
		t.Fatal("命令未返回完整上下文")
	}
	d := source.EmptyDesign()
	d.Modules = []design.Module{{ID: "core", Root: "core", Responsibility: "当前示例能力"}}
	result := designer.Response{RequestID: r.RequestID, Result: designer.Result{Summary: "梳理源码", Design: d, Analysis: &designer.Analysis{
		Evidence: []designer.Evidence{{Kind: "module", ID: "core", Status: "supported", Explanation: "包内源码", Locations: []source.Location{{File: "core/core.go", Line: 2, Column: 1, EndLine: 3}}}},
		Issues:   []designer.Finding{}, Suggestions: []designer.Finding{}, Uncertainties: []string{},
	}}}
	data, _ := json.Marshal(result)
	testproject.Write(t, root, "response.json", string(data))
	var p store.Proposal
	json.Unmarshal(invoke(t, 0, "proposal-import", "-project", root, "-file", filepath.Join(root, "response.json")), &p)
	if p.ID == "" {
		t.Fatal("没有提案 ID")
	}
	invoke(t, 2, "show", "-project", root, "-draft")
	invoke(t, 0, "proposal-show", "-project", root)
	var accepted struct {
		Hash string `json:"draft_hash"`
	}
	json.Unmarshal(invoke(t, 0, "proposal-accept", "-project", root, "-id", p.ID), &accepted)
	invoke(t, 2, "show", "-project", root)
	invoke(t, 0, "confirm", "-project", root, "-draft-hash", accepted.Hash, "-expected", "none", "-by", "用户")
	invoke(t, 0, "check", "-project", root)
	json.Unmarshal(invoke(t, 0, "design-request", "-project", root, "-kind", "reanalysis"), &r)
	result.RequestID = r.RequestID
	data, _ = json.Marshal(result)
	testproject.Write(t, root, "response.json", string(data))
	json.Unmarshal(invoke(t, 0, "proposal-import", "-project", root, "-file", filepath.Join(root, "response.json")), &p)
	invoke(t, 0, "proposal-discard", "-project", root, "-id", p.ID)
	invoke(t, 2, "proposal-show", "-project", root)
	// 已移除的执行命令不能再启动用户程序。
	invoke(t, 2, "review", "-project", root, "-reviewer", "/does-not-run")
}
