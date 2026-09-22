package check_test

import (
	"context"
	"strings"
	"testing"

	"interfaceisall/check"
	"interfaceisall/internal/testproject"
)

func TestMainPackageScanDoesNotRequireWorkingVCSMetadata(t *testing.T) {
	root, _, baseline := testproject.New(t)
	// 导出副本可能保留不完整的 .git 目录，go list 不应尝试为 main 包打版本标记。
	testproject.Write(t, root, ".git/HEAD", "unavailable vcs metadata\n")
	testproject.Write(t, root, "cmd/main.go", "package main\nfunc main() {}\n")
	report, err := check.Run(context.Background(), root, baseline, check.Options{})
	if err != nil || report.Deterministic.Status != "passed" {
		t.Fatalf("静态扫描不应依赖 VCS 状态：%v %+v", err, report.Facts.Diagnostics)
	}
}

func TestDirectDependenciesAreDirectionalAndLocateSubpackageImports(t *testing.T) {
	root, _, baseline := testproject.New(t)
	// 被禁止的是支付到订单的直接导入；子包同样属于支付模块。
	testproject.Write(t, root, "payment/api/order_link.go", "package api\n\nimport alias \"example.test/shop/order\"\n\nvar _ alias.ID\n")
	report, err := check.Run(context.Background(), root, baseline, check.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Deterministic.Status != "violations" || len(report.Deterministic.Violations) != 1 {
		t.Fatalf("未定位禁止依赖：%+v", report)
	}
	violation := report.Deterministic.Violations[0]
	if violation.Dependency.Location.File != "payment/api/order_link.go" || violation.Dependency.Location.Line != 3 || violation.Rule.ID != "payment-no-order" {
		t.Fatalf("证据不准确：%+v", violation)
	}
	if report.SemanticStatus != "not_run" {
		t.Fatal("确定性检查不能冒充能力审查")
	}
}

func TestAllowedNewDependenciesAndRefactoringDoNotCreateViolations(t *testing.T) {
	root, _, baseline := testproject.New(t)
	testproject.Write(t, root, "order/order.go", "package order\n\nimport \"example.test/shop/payment/api\"\n\n// Cancel 通过支付能力处理退款。\nfunc Cancel() error { return api.RequestRefund(\"1\") }\n")
	report, err := check.Run(context.Background(), root, baseline, check.Options{})
	if err != nil || report.Deterministic.Status != "passed" || len(report.Facts.Dependencies) != 1 {
		t.Fatalf("默认允许的方向被误报：%+v %v", report, err)
	}
	// 名称及参数重组只改变代码事实，不触发确定性接口违规。
	testproject.Write(t, root, "payment/api/refund.go", "package api\n\ntype Request struct{ PaymentID string }\n\n// Apply 申请退款。\nfunc Apply(request Request) error { return nil }\n")
	testproject.Write(t, root, "order/order.go", "package order\n\nimport \"example.test/shop/payment/api\"\n\n// Cancel 仍通过支付能力退款。\nfunc Cancel() error { return api.Apply(api.Request{PaymentID: \"1\"}) }\n")
	report, err = check.Run(context.Background(), root, baseline, check.Options{})
	if err != nil || report.Deterministic.Status != "passed" {
		t.Fatalf("正常重构被误报：%+v %v", report, err)
	}
}

func TestBuildTagsAndTestFilesHaveExplicitScope(t *testing.T) {
	root, _, baseline := testproject.New(t)
	testproject.Write(t, root, "payment/api/special.go", "//go:build special\n\npackage api\n\nimport _ \"example.test/shop/order\"\n")
	testproject.Write(t, root, "payment/api/refund_test.go", "package api\n\nimport _ \"example.test/shop/order\"\n")
	defaultReport, err := check.Run(context.Background(), root, baseline, check.Options{})
	if err != nil || defaultReport.Deterministic.Status != "passed" {
		t.Fatalf("默认范围错误：%v %+v", err, defaultReport)
	}
	tagged, err := check.Run(context.Background(), root, baseline, check.Options{Tags: "special"})
	if err != nil || len(tagged.Deterministic.Violations) != 1 || tagged.Facts.Scope.Tags != "special" {
		t.Fatalf("构建标签未生效：%v %+v", err, tagged)
	}
	for _, file := range tagged.Facts.Files {
		if strings.HasSuffix(file.Path, "_test.go") {
			t.Fatal("不应包含测试源码")
		}
	}
}

func TestCodeEvidenceIncludesPrivateHelpersGenericsAndExportedVariables(t *testing.T) {
	root, _, baseline := testproject.New(t)
	testproject.Write(t, root, "payment/api/extra.go", `package api

type worker[T any] struct{}
// Refund 可通过工厂返回的未导出类型调用。
func (w *worker[T]) Refund(value T) error { return nil }
// New 返回处理器。
func New() *worker[string] { return &worker[string]{} }
// Bypass 是函数变量，也可能构成未经设计的能力。
var Bypass = func() {}
`)
	facts, err := check.Scan(context.Background(), root, baseline.Design, check.Options{})
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, entry := range facts.Entries {
		found[entry.Kind+":"+entry.Receiver+":"+entry.Name] = true
	}
	if !found["method:worker:Refund"] || !found["var::Bypass"] || !found["function::RequestRefund"] {
		t.Fatalf("公开候选不完整：%+v", facts.Entries)
	}
	for _, file := range facts.Files {
		if strings.Contains(file.Content, "func refund(") {
			return
		}
	}
	t.Fatal("缺少私有辅助实现上下文")
}

func TestLoadFailuresAndNestedModulesCannotPassSilently(t *testing.T) {
	for _, scenario := range []string{"missing_import", "syntax", "nested"} {
		t.Run(scenario, func(t *testing.T) {
			root, _, baseline := testproject.New(t)
			switch scenario {
			case "missing_import":
				testproject.Write(t, root, "payment/api/broken.go", "package api\nimport _ \"example.test/shop/missing\"\n")
			case "syntax":
				testproject.Write(t, root, "payment/api/broken.go", "package api\nfunc Broken( {\n")
			case "nested":
				testproject.Write(t, root, "nested/go.mod", "module example.test/nested\n\ngo 1.22\n")
			}
			report, err := check.Run(context.Background(), root, baseline, check.Options{})
			if err != nil {
				t.Fatal(err)
			}
			if report.Deterministic.Status != "incomplete" || len(report.Facts.Diagnostics) == 0 {
				t.Fatalf("漏报不完整范围：%+v", report)
			}
			if _, err := check.NewReviewRequest(baseline, report.Facts); err == nil {
				t.Fatal("不完整扫描不能当完整审查输入")
			}
		})
	}
}

func TestLineDirectivesDoNotRewriteEvidenceLocations(t *testing.T) {
	root, _, baseline := testproject.New(t)
	// 生成代码常使用 //line；报告仍必须指向本次实际扫描的文件。
	testproject.Write(t, root, "payment/api/generated.go", "package api\n\n//line invented.go:900\nimport _ \"example.test/shop/order\"\n\n//line invented.go:1200\nfunc Generated() {}\n")
	report, err := check.Run(context.Background(), root, baseline, check.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Deterministic.Violations) != 1 {
		t.Fatal("应发现直接禁止依赖")
	}
	position := report.Deterministic.Violations[0].Dependency.Location
	if position.File != "payment/api/generated.go" || position.Line != 4 {
		t.Fatalf("导入位置被重定向：%+v", position)
	}
	for _, entry := range report.Facts.Entries {
		if entry.Name == "Generated" {
			if entry.Location.File != "payment/api/generated.go" || entry.Location.Line != 7 {
				t.Fatalf("入口位置被重定向：%+v", entry)
			}
			return
		}
	}
	t.Fatal("缺少公开入口")
}
