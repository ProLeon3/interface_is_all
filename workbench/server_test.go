package workbench

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ProLeon3/interface_is_all/check"
	"github.com/ProLeon3/interface_is_all/design"
	"github.com/ProLeon3/interface_is_all/internal/testproject"
	"github.com/ProLeon3/interface_is_all/store"
)

// 通过实际 HTTP 处理器覆盖浏览器行为契约，测试不依赖真实端口或模型。
func request(t *testing.T, server *Server, method, path, body string, status int) []byte {
	t.Helper()
	req := httptest.NewRequest(method, "http://127.0.0.1:8090"+path, strings.NewReader(body))
	if method != http.MethodGet {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Workbench-Token", server.token)
	}
	out := httptest.NewRecorder()
	server.ServeHTTP(out, req)
	if out.Code != status {
		t.Fatalf("%s %s：状态 %d，预期 %d：%s", method, path, out.Code, status, out.Body.String())
	}
	return out.Body.Bytes()
}

func jsonBody(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestWorkbenchEmptyProjectAndConfirmationJourney(t *testing.T) {
	root := t.TempDir()
	s, err := New(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	var initial struct {
		Draft     *design.Design  `json:"draft"`
		Confirmed *store.Snapshot `json:"confirmed"`
		Token     string          `json:"token"`
	}
	json.Unmarshal(request(t, s, "GET", "/api/state", "", 200), &initial)
	if initial.Draft != nil || initial.Confirmed != nil || initial.Token == "" {
		t.Fatal("新项目不应自动生成设计或确认版本")
	}
	if _, err := os.Stat(filepath.Join(root, ".architecture")); !os.IsNotExist(err) {
		t.Fatal("只读页面创建了项目文件")
	}
	for _, asset := range []string{"/", "/app.js", "/styles.css", "/reports.js", "/ui.js"} {
		request(t, s, "GET", asset, "", 200)
	}
	d := testproject.Design()
	request(t, s, "POST", "/api/validate", jsonBody(t, d), 200)
	request(t, s, "PUT", "/api/draft", jsonBody(t, map[string]any{"design": d}), 400)
	request(t, s, "PUT", "/api/draft", jsonBody(t, map[string]any{"design": d, "expected_draft_hash": "", "expected_revision": ""}), 200)
	_, hash, _ := s.store.Draft()
	request(t, s, "POST", "/api/check", `{}`, 409)
	request(t, s, "POST", "/api/confirm", jsonBody(t, map[string]any{"draft_hash": hash, "actor": "用户"}), 400)
	request(t, s, "POST", "/api/confirm", jsonBody(t, map[string]any{"draft_hash": hash, "expected_revision": "", "actor": "用户"}), 200)
	first, err := s.store.Confirmed()
	if err != nil || first.DesignHash != hash || first.ConfirmedBy != "用户" {
		t.Fatal("浏览器确认未持久化正确快照", err)
	}
	d.Modules[0].Responsibility = "新的职责草稿"
	request(t, s, "PUT", "/api/draft", jsonBody(t, map[string]any{"design": d, "expected_draft_hash": hash, "expected_revision": first.Revision}), 200)
	request(t, s, "PATCH", "/api/layout", `{"nodes":{"payment":{"x":333,"y":222}}}`, 200)
	request(t, s, "POST", "/api/confirm", jsonBody(t, map[string]any{"draft_hash": hash, "expected_revision": first.Revision, "actor": "用户"}), 409)
	current, _ := s.store.Confirmed()
	if current.Revision != first.Revision {
		t.Fatal("草稿、布局或陈旧确认改变了检查基准")
	}
	// 重新创建服务代表进程重启，应能读到文件中的草稿、布局和独立基准。
	reopened, _ := New(root, Options{})
	var state struct {
		Draft     design.Design  `json:"draft"`
		Layout    store.Layout   `json:"layout"`
		Confirmed store.Snapshot `json:"confirmed"`
	}
	json.Unmarshal(request(t, reopened, "GET", "/api/state", "", 200), &state)
	if state.Draft.Modules[0].Responsibility != d.Modules[0].Responsibility || state.Layout.Nodes["payment"].X != 333 || state.Confirmed.Revision != first.Revision {
		t.Fatal("重新打开工作台丢失了保存结果")
	}
}

func TestWorkbenchConflictNeverOverwritesExternalChanges(t *testing.T) {
	root, external, first := testproject.New(t)
	s, _ := New(root, Options{})
	d := testproject.Design()
	d.Modules[0].Responsibility = "外部工具的新职责"
	newHash, _ := external.SaveDraft(d)
	request(t, s, "PUT", "/api/draft", jsonBody(t, map[string]any{"design": testproject.Design(), "expected_draft_hash": first.DesignHash, "expected_revision": first.Revision}), 409)
	_, actualHash, _ := external.Draft()
	if actualHash != newHash {
		t.Fatal("工作台覆盖了外部工具的新草稿")
	}
	request(t, s, "POST", "/api/confirm", jsonBody(t, map[string]any{"draft_hash": first.DesignHash, "expected_revision": first.Revision, "actor": "用户"}), 409)
	if _, err := external.Confirm(newHash, first.Revision, "其他窗口"); err != nil {
		t.Fatal(err)
	}
	request(t, s, "PUT", "/api/draft", jsonBody(t, map[string]any{"design": testproject.Design(), "expected_draft_hash": newHash, "expected_revision": first.Revision}), 409)
}

func TestWorkbenchTemporaryLockCanBeRetriedWithoutReloading(t *testing.T) {
	root, external, baseline := testproject.New(t)
	s, _ := New(root, Options{})
	d := testproject.Design()
	d.Modules[0].Responsibility = "等待保存的职责"
	body := jsonBody(t, map[string]any{"design": d, "expected_draft_hash": baseline.DesignHash, "expected_revision": baseline.Revision})
	lock := filepath.Join(root, ".architecture/write.lock")
	if err := os.WriteFile(lock, []byte("pid=测试进程\n"), 0600); err != nil {
		t.Fatal(err)
	}
	result := request(t, s, "PUT", "/api/draft", body, http.StatusLocked)
	if !strings.Contains(string(result), `"code":"store_locked"`) {
		t.Fatal("临时写锁被误报为需要丢弃编辑的版本冲突")
	}
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	request(t, s, "PUT", "/api/draft", body, 200)
	actual, _, _ := external.Draft()
	if actual.Modules[0].Responsibility != d.Modules[0].Responsibility {
		t.Fatal("锁释放后无法用原版本重试保存")
	}
}

func TestWorkbenchReportsExposeViolationsAndPendingReview(t *testing.T) {
	root, external, baseline := testproject.New(t)
	s, _ := New(root, Options{})
	testproject.Write(t, root, "payment/api/order_link.go", "package api\nimport _ \"example.test/shop/order\"\n")
	var report check.Report
	json.Unmarshal(request(t, s, "POST", "/api/check", `{}`, 200), &report)
	if report.Deterministic.Status != "violations" || len(report.Deterministic.Violations) != 1 || report.SemanticStatus != "not_run" {
		t.Fatal("确定性检查未展示违规或混淆了语义状态")
	}
	var reviewRequest check.ReviewRequest
	json.Unmarshal(request(t, s, "POST", "/api/review-request", `{}`, 200), &reviewRequest)
	response := check.ReviewResponse{SchemaVersion: 1, RequestID: reviewRequest.RequestID, Concerns: []check.Concern{}, Interfaces: []check.Association{{
		InterfaceID: "refund", EntryIDs: []string{"example.test/shop/payment/api:function:RequestRefund"}, Assessment: "uncertain", Reason: "入口存在，需要核查真实退款语义", Evidence: []check.Evidence{{File: "payment/api/refund.go", Line: 4, EndLine: 4}},
	}}}
	var semantic check.SemanticReport
	json.Unmarshal(request(t, s, "POST", "/api/review-import", jsonBody(t, map[string]any{"request": reviewRequest, "response": response}), 200), &semantic)
	if semantic.Status != "pending_confirmation" || semantic.Interfaces[0].Status != "pending_confirmation" {
		t.Fatal("导入误将模型判断标为确认结果")
	}
	var observations struct {
		Check  *store.Observation `json:"check"`
		Review *store.Observation `json:"review"`
	}
	json.Unmarshal(request(t, s, "GET", "/api/observations", "", 200), &observations)
	if observations.Check == nil || observations.Review == nil {
		t.Fatal("没有读取共享观察目录中的报告")
	}
	// 保存其他版本后，旧报告仍保留原版本供界面识别，不能冒充新版本结果。
	d := testproject.Design()
	d.Modules[0].Responsibility = "新版职责"
	hash, _ := external.SaveDraft(d)
	external.Confirm(hash, baseline.Revision, "用户")
	var historical check.SemanticReport
	json.Unmarshal(observations.Review.Report, &historical)
	if historical.DesignRevision != baseline.Revision {
		t.Fatal("历史报告的基准被改写")
	}
	request(t, s, "POST", "/api/review-import", jsonBody(t, map[string]any{"request": reviewRequest, "response": response}), 422)
}

func TestWorkbenchRejectsCrossOriginRequestsAndInvalidImport(t *testing.T) {
	s, _ := New(t.TempDir(), Options{})
	for _, test := range []struct{ host, origin, token, site string }{
		{"attacker.example:8090", "", s.token, ""},
		{"127.0.0.1:8090", "https://attacker.example", s.token, ""},
		{"127.0.0.1:8090", "", "", ""},
		{"127.0.0.1:8090", "", s.token, "cross-site"},
	} {
		req := httptest.NewRequest("POST", "http://127.0.0.1:8090/api/confirm", strings.NewReader(`{}`))
		req.Host = test.host
		req.Header.Set("Origin", test.origin)
		req.Header.Set("X-Workbench-Token", test.token)
		req.Header.Set("Sec-Fetch-Site", test.site)
		req.Header.Set("Content-Type", "application/json")
		out := httptest.NewRecorder()
		s.ServeHTTP(out, req)
		if out.Code != 403 {
			t.Fatalf("未拒绝跨站或缺少令牌的写入：%+v %d", test, out.Code)
		}
	}
	request(t, s, "POST", "/api/validate", `{"schema_version":1,"schema_version":1,"modules":[],"interfaces":[],"forbidden_dependencies":[]}`, 422)
	d := testproject.Design()
	d.Modules[1].Root = "payment/sub"
	data := request(t, s, "POST", "/api/validate", jsonBody(t, d), 422)
	if !strings.Contains(string(data), "overlapping_roots") || !strings.Contains(string(data), "/modules/1/root") {
		t.Fatal("设计关系错误缺少可定位的字段路径")
	}
	if err := Listen(context.Background(), t.TempDir(), "0.0.0.0:8090", Options{}, io.Discard); err == nil {
		t.Fatal("不应绑定非回环地址")
	}
}

func TestWorkbenchReadsExternalArtifactsAndReportsCorruption(t *testing.T) {
	root, external, baseline := testproject.New(t)
	s, _ := New(root, Options{})
	report, err := check.Run(context.Background(), root, baseline, check.Options{})
	if err != nil {
		t.Fatal(err)
	}
	path, err := external.SaveArtifact("check", baseline.Revision, report)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(request(t, s, "GET", "/api/observations", "", 200)), "passed") {
		t.Fatal("未读取外部进程保存的检查结果")
	}
	if err := os.WriteFile(path, []byte(`{"broken":`), 0600); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Check    *store.Observation `json:"check"`
		Warnings []string           `json:"warnings"`
	}
	json.Unmarshal(request(t, s, "GET", "/api/observations", "", 200), &result)
	if result.Check != nil || len(result.Warnings) != 1 {
		t.Fatal("损坏报告不能继续显示为通过")
	}
	// JSON 合法但缺少扫描事实时也不能接受一个孤立的 passed 标志。
	if err := os.WriteFile(path, []byte(`{"schema_version":1,"design_revision":"old","deterministic":{"status":"passed"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(request(t, s, "GET", "/api/observations", "", 200), &result)
	if result.Check != nil || len(result.Warnings) != 1 {
		t.Fatal("不完整报告被误当成有效检查结果")
	}
}
