package designer

import (
	"encoding/json"
	"strings"
	"testing"

	"interfaceisall/design"
	"interfaceisall/source"
)

// 两个真实形状的源码片段用于检验出处约束；不把固定语义解释当成真实模型证明。
func analysisSample() (source.Context, Result) {
	facts := source.Facts{Files: []source.SourceFile{
		{Path: "caller/call.go", PackagePath: "example/caller", Content: "package caller\nimport \"example/service\"\nfunc Call() { service.Run() }\nfunc Unrelated() {}\n"},
		{Path: "service/run.go", PackagePath: "example/service", Content: "package service\nfunc Run() {}\nfunc Unrelated() {}\n"},
	}, Packages: []source.Package{{ImportPath: "example/caller", Directory: "caller", Files: []string{"call.go"}}, {ImportPath: "example/service", Directory: "service", Files: []string{"run.go"}}}}
	code := source.Context{Facts: facts, Fingerprint: source.Fingerprint(facts)}
	caller := source.Location{File: "caller/call.go", Line: 3, Column: 1, EndLine: 3}
	service := source.Location{File: "service/run.go", Line: 2, Column: 1, EndLine: 2}
	d := design.Design{SchemaVersion: 1, Modules: []design.Module{{ID: "caller", Root: "caller", Responsibility: "发起调用"}, {ID: "service", Root: "service", Responsibility: "提供运行入口"}}, Interfaces: []design.Interface{{ID: "run", ModuleID: "service", Name: "运行", Description: "当前执行空逻辑", Semantics: &design.Semantics{Inputs: "无", Outputs: "无", Errors: "无"}}}, Collaborations: []design.Collaboration{{ID: "call-run", From: "caller", InterfaceID: "run", Purpose: "调用运行能力"}}, ForbiddenDependencies: []design.ForbiddenDependency{}}
	a := &Analysis{Evidence: []Evidence{
		{Kind: "module", ID: "caller", Status: "supported", Explanation: "调用代码", Locations: []source.Location{caller}},
		{Kind: "module", ID: "service", Status: "supported", Explanation: "服务代码", Locations: []source.Location{service}},
		{Kind: "interface", ID: "run", Status: "supported", Explanation: "运行实现", Locations: []source.Location{service}},
		{Kind: "collaboration", ID: "call-run", Status: "supported", Explanation: "直接调用", Locations: []source.Location{caller, service}},
	}, Issues: []Finding{}, Suggestions: []Finding{}, Uncertainties: []string{}}
	return code, Result{Summary: "源码现状", Design: d, Analysis: a}
}

func TestAnalysisValidatesEveryObjectAndEvidenceBoundary(t *testing.T) {
	code, result := analysisSample()
	if err := ValidateAnalysis(code, result); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*Result){
		"missing_evidence": func(r *Result) { r.Analysis.Evidence = r.Analysis.Evidence[:3] },
		"duplicate":        func(r *Result) { r.Analysis.Evidence[1] = r.Analysis.Evidence[0] },
		"fake_directory":   func(r *Result) { r.Design.Modules[0].Root = "invented" },
		"missing_package": func(r *Result) {
			r.Design.Modules = r.Design.Modules[1:]
			r.Design.Collaborations = nil
			r.Analysis.Evidence = r.Analysis.Evidence[1:3]
		},
		"forbidden": func(r *Result) {
			r.Design.ForbiddenDependencies = []design.ForbiddenDependency{{ID: "ban", From: "caller", To: "service", Reason: "模型建议"}}
		},
		"wrong_owner":   func(r *Result) { r.Analysis.Evidence[0].Locations = r.Analysis.Evidence[1].Locations },
		"fake_file":     func(r *Result) { r.Analysis.Evidence[0].Locations[0].File = "../../outside.go" },
		"line_overflow": func(r *Result) { r.Analysis.Evidence[0].Locations[0].EndLine = 999 },
		"import_only": func(r *Result) {
			r.Analysis.Evidence[3].Locations[0].Line = 2
			r.Analysis.Evidence[3].Locations[0].EndLine = 2
		},
		"no_target": func(r *Result) { r.Analysis.Evidence[3].Locations = r.Analysis.Evidence[3].Locations[:1] },
		"unrelated_caller": func(r *Result) {
			r.Analysis.Evidence[3].Locations[0].Line = 4
			r.Analysis.Evidence[3].Locations[0].EndLine = 4
		},
		"unrelated_target": func(r *Result) {
			r.Analysis.Evidence[3].Locations[1].Line = 3
			r.Analysis.Evidence[3].Locations[1].EndLine = 3
		},
		"invented_issue":   func(r *Result) { r.Analysis.Issues = []Finding{{Description: "无来源结论"}} },
		"missing_analysis": func(r *Result) { r.Analysis = nil },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			code, result := analysisSample()
			mutate(&result)
			if err := ValidateAnalysis(code, result); err == nil {
				t.Fatal("错误依据被接受")
			}
		})
	}
	// 仅有导入时必须标不确定；观察依赖依然保留，不能自动成为禁止规则。
	result.Analysis.Evidence[3].Locations[0].Line = 2
	result.Analysis.Evidence[3].Locations[0].EndLine = 2
	result.Analysis.Evidence[3].Status = "uncertain"
	if err := ValidateAnalysis(code, result); err != nil {
		t.Fatal(err)
	}
}

// 外部响应沿用同一份 Schema 和证据补全约束，完全不需要模型服务。
func TestExternalAnalysisResponseSchemaAndEvidence(t *testing.T) {
	code, result := analysisSample()
	result.Analysis.Evidence[3].Locations = result.Analysis.Evidence[3].Locations[:1]
	data, _ := json.Marshal(Response{RequestID: strings.Repeat("a", 64), Result: result})
	got, err := ParseResponse(data, Request{Source: &code})
	if err != nil || len(got.Analysis.Evidence[3].Locations) != 2 {
		t.Fatal(got, err)
	}
	if _, err := ParseResponse(data, Request{}); err == nil {
		t.Fatal("普通设计响应混入了来源分析")
	}
}

func TestCompletingEvidenceDoesNotRepairUnrelatedUsage(t *testing.T) {
	code, result := analysisSample()
	result.Analysis.Evidence[3].Locations = result.Analysis.Evidence[3].Locations[:1]
	result.Analysis.Evidence[3].Locations[0].Line = 4
	result.Analysis.Evidence[3].Locations[0].EndLine = 4
	completeCollaborationEvidence(&result)
	if err := ValidateAnalysis(code, result); err == nil {
		t.Fatal("补目标依据绕过了调用方真实使用校验")
	}
	// 已有目标位置引用了无关声明时不得被补全逻辑修补成有效结论。
	code, result = analysisSample()
	result.Analysis.Evidence[3].Locations[1].Line = 3
	result.Analysis.Evidence[3].Locations[1].EndLine = 3
	completeCollaborationEvidence(&result)
	if err := ValidateAnalysis(code, result); err == nil {
		t.Fatal("补目标依据掩盖了已有的错误目标位置")
	}
}
