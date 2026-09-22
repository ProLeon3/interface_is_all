package check_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ProLeon3/interface_is_all/check"
	"github.com/ProLeon3/interface_is_all/internal/testproject"
	"github.com/ProLeon3/interface_is_all/store"
)

func reviewFixture(t *testing.T) (string, *store.Store, check.ReviewRequest, check.ReviewResponse) {
	t.Helper()
	root, s, baseline := testproject.New(t)
	facts, err := check.Scan(context.Background(), root, baseline.Design, check.Options{})
	if err != nil {
		t.Fatal(err)
	}
	request, err := check.NewReviewRequest(baseline, facts)
	if err != nil {
		t.Fatal(err)
	}
	var refund check.Entry
	for _, entry := range facts.Entries {
		if entry.Name == "RequestRefund" {
			refund = entry
		}
	}
	// 固定响应只验证协议约束，不代表真实模型的语义判断能力。
	response := check.ReviewResponse{
		SchemaVersion: 1, RequestID: request.RequestID, Concerns: []check.Concern{},
		Interfaces: []check.Association{{InterfaceID: "refund", EntryIDs: []string{refund.ID}, Assessment: "aligned", Reason: "公开入口将支付标识交给退款处理；语义仍待用户核查", Evidence: []check.Evidence{{File: refund.Location.File, Line: refund.Location.Line, EndLine: refund.Location.EndLine}}}},
	}
	return root, s, request, response
}

func TestReviewRemainsPendingAndIncludesOriginalDesign(t *testing.T) {
	root, s, request, response := reviewFixture(t)
	before, _ := s.Confirmed()
	report, err := check.ImportReview(context.Background(), root, before, request, response)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "pending_confirmation" || report.Interfaces[0].Status != "pending_confirmation" {
		t.Fatal("模型关联不能自动确认为合规")
	}
	if report.Interfaces[0].DesignBasis.Description != before.Design.Interfaces[0].Description || len(report.Interfaces[0].SearchedFiles) == 0 {
		t.Fatal("缺少原始设计依据或搜索范围")
	}
	after, _ := s.Confirmed()
	if after.Revision != before.Revision {
		t.Fatal("审查不应修改设计")
	}
}

func TestReviewRejectsInvalidCoverageReferencesAndEvidence(t *testing.T) {
	_, _, request, base := reviewFixture(t)
	cases := []struct {
		name   string
		change func(*check.ReviewResponse)
	}{
		{"缺少接口", func(r *check.ReviewResponse) { r.Interfaces = []check.Association{} }},
		{"重复接口", func(r *check.ReviewResponse) { r.Interfaces = append(r.Interfaces, r.Interfaces[0]) }},
		{"未知接口", func(r *check.ReviewResponse) { r.Interfaces[0].InterfaceID = "fake" }},
		{"未知入口", func(r *check.ReviewResponse) { r.Interfaces[0].EntryIDs[0] = "fake" }},
		{"跨模块关联", func(r *check.ReviewResponse) { r.Interfaces[0].EntryIDs[0] = "example.test/shop/order:type:ID" }},
		{"空证据", func(r *check.ReviewResponse) { r.Interfaces[0].Evidence = []check.Evidence{} }},
		{"伪造文件", func(r *check.ReviewResponse) { r.Interfaces[0].Evidence[0].File = "../fake.go" }},
		{"越界行号", func(r *check.ReviewResponse) { r.Interfaces[0].Evidence[0].EndLine = 99999 }},
		{"反向行号", func(r *check.ReviewResponse) { r.Interfaces[0].Evidence[0].EndLine = 1 }},
		{"缺失却关联入口", func(r *check.ReviewResponse) { r.Interfaces[0].Assessment = "not_found" }},
		{"符合却无入口", func(r *check.ReviewResponse) { r.Interfaces[0].EntryIDs = []string{} }},
		{"请求不匹配", func(r *check.ReviewResponse) {
			r.RequestID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		}},
		{"非法状态", func(r *check.ReviewResponse) { r.Interfaces[0].Assessment = "confirmed_violation" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, _ := json.Marshal(base)
			var response check.ReviewResponse
			json.Unmarshal(data, &response)
			tc.change(&response)
			if _, err := check.ValidateReview(request, response); err == nil {
				t.Fatal("应拒绝不完整或无依据的审查")
			}
		})
	}
}

func TestMissingImplementationAndAdditionalCapabilityRemainReviewFindings(t *testing.T) {
	root, _, baseline := testproject.New(t)
	testproject.Write(t, root, "payment/api/refund.go", "package api\n\n// OverrideStatus 绕过支付流程直接改写状态。\nfunc OverrideStatus() {}\n")
	facts, err := check.Scan(context.Background(), root, baseline.Design, check.Options{})
	if err != nil {
		t.Fatal(err)
	}
	request, err := check.NewReviewRequest(baseline, facts)
	if err != nil {
		t.Fatal(err)
	}
	response := check.ReviewResponse{
		SchemaVersion: 1, RequestID: request.RequestID,
		Interfaces: []check.Association{{InterfaceID: "refund", EntryIDs: []string{}, Assessment: "not_found", Reason: "扫描的支付模块未找到退款实现，需要用户核查", Evidence: []check.Evidence{}}},
		Concerns:   []check.Concern{{Kind: "additional_capability", ModuleID: "payment", EntryIDs: []string{"example.test/shop/payment/api:function:OverrideStatus"}, Reason: "此入口可能绕过设计中的支付流程", Evidence: []check.Evidence{{File: "payment/api/refund.go", Line: 4, EndLine: 4}}}},
	}
	report, err := check.ValidateReview(request, response)
	if err != nil {
		t.Fatal(err)
	}
	if report.Interfaces[0].Conclusion.Assessment != "not_found" || report.Concerns[0].Status != "pending_confirmation" {
		t.Fatalf("错误升级语义结论：%+v", report)
	}
	if len(report.Interfaces[0].SearchedFiles) != 1 || report.Concerns[0].DesignBasis.Responsibility == "" {
		t.Fatal("缺失结论和新增能力都必须附带依据")
	}
}

func TestReviewRejectsStaleSourceAndConfirmedVersion(t *testing.T) {
	root, s, request, response := reviewFixture(t)
	testproject.Write(t, root, "payment/api/refund.go", "package api\n\n// Changed 代码已改变。\nfunc Changed() {}\n")
	if _, err := check.ImportReview(context.Background(), root, request.Baseline, request, response); err == nil {
		t.Fatal("应拒绝过期源码证据")
	}
	d := testproject.Design()
	d.Interfaces[0].Description = "新的退款语义"
	hash, _ := s.SaveDraft(d)
	current, err := s.Confirm(hash, request.Baseline.Revision, "用户")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := check.ImportReview(context.Background(), root, current, request, response); err == nil {
		t.Fatal("应拒绝过期设计")
	}
}

func TestReviewRejectsChangedBuildManifest(t *testing.T) {
	root, _, request, response := reviewFixture(t)
	// Go 源码没有变化时，构建清单改变也不能沿用旧审查。
	testproject.Write(t, root, "go.mod", "module example.test/shop\n\ngo 1.22\n\n// 依赖配置已重新审阅。\n")
	if _, err := check.ImportReview(context.Background(), root, request.Baseline, request, response); err == nil {
		t.Fatal("构建清单改变后应拒绝旧响应")
	}
}
