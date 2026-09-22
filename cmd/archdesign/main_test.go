package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/ProLeon3/interface_is_all/check"
	"github.com/ProLeon3/interface_is_all/internal/testproject"
	"github.com/ProLeon3/interface_is_all/store"
)

func invoke(t *testing.T, expectedCode int, args ...string) []byte {
	t.Helper()
	var out, stderr bytes.Buffer
	if code := run(context.Background(), args, &out, &stderr); code != expectedCode {
		t.Fatalf("%v：退出码 %d，预期 %d\nstdout=%s\nstderr=%s", args, code, expectedCode, out.String(), stderr.String())
	}
	return out.Bytes()
}

func TestCLICompleteDesignAndCheckWorkflow(t *testing.T) {
	root := t.TempDir()
	testproject.Write(t, root, "go.mod", "module example.test/shop\n\ngo 1.22\n")
	testproject.Write(t, root, "payment/api/refund.go", "package api\n\n// Refund 申请退款。\nfunc Refund() {}\n")
	testproject.Write(t, root, "order/order.go", "package order\n")
	d := testproject.Design()
	data, _ := json.Marshal(d)
	testproject.Write(t, root, "design.json", string(data))
	file := filepath.Join(root, "design.json")
	invoke(t, 2, "check", "-project", root)
	invoke(t, 0, "validate", "-project", root, "-file", file)
	saved := invoke(t, 0, "save-draft", "-project", root, "-file", file)
	var result struct {
		DraftHash string `json:"draft_hash"`
	}
	json.Unmarshal(saved, &result)
	invoke(t, 2, "confirm", "-project", root, "-draft-hash", result.DraftHash, "-by", "用户")
	confirmed := invoke(t, 0, "confirm", "-project", root, "-draft-hash", result.DraftHash, "-expected", "none", "-by", "用户")
	var baseline store.Snapshot
	json.Unmarshal(confirmed, &baseline)
	if baseline.DesignHash != result.DraftHash {
		t.Fatal("确认了错误内容")
	}
	var report check.Report
	json.Unmarshal(invoke(t, 0, "check", "-project", root), &report)
	if report.Deterministic.Status != "passed" || report.SemanticStatus != "not_run" {
		t.Fatal("错误的检查状态")
	}
	requestData := invoke(t, 0, "review-request", "-project", root)
	request, err := check.ParseReviewRequest(requestData)
	if err != nil {
		t.Fatal(err)
	}
	response := check.ReviewResponse{
		SchemaVersion: 1, RequestID: request.RequestID, Concerns: []check.Concern{},
		Interfaces: []check.Association{{InterfaceID: "refund", EntryIDs: []string{"example.test/shop/payment/api:function:Refund"}, Assessment: "uncertain", Reason: "入口存在但当前函数为空，需要人工核查", Evidence: []check.Evidence{{File: "payment/api/refund.go", Line: 4, EndLine: 4}}}},
	}
	responseData, _ := json.Marshal(response)
	testproject.Write(t, root, "request.json", string(requestData))
	testproject.Write(t, root, "response.json", string(responseData))
	accepted := invoke(t, 0, "review-import", "-project", root, "-request", filepath.Join(root, "request.json"), "-file", filepath.Join(root, "response.json"))
	var semantic check.SemanticReport
	json.Unmarshal(accepted, &semantic)
	if semantic.Status != "pending_confirmation" {
		t.Fatal("语义结果未保持待确认")
	}
	// 引入禁止的直接依赖后，命令行必须以 1 退出，并仍输出可解析报告。
	testproject.Write(t, root, "payment/api/order_link.go", "package api\nimport _ \"example.test/shop/order\"\n")
	json.Unmarshal(invoke(t, 1, "check", "-project", root), &report)
	if len(report.Deterministic.Violations) != 1 {
		t.Fatal("退出码和违规报告不一致")
	}
	invoke(t, 2, "review-import", "-project", root, "-request", filepath.Join(root, "request.json"), "-file", filepath.Join(root, "response.json"))
}
