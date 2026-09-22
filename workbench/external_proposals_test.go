package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"interfaceisall/design"
	"interfaceisall/designer"
	"interfaceisall/internal/testproject"
	"interfaceisall/store"
)

// 请求和响应通过真实 HTTP 处理器交换，固定内容只验证程序协议。
func exportRequest(t *testing.T, s *Server, kind, parent string) store.DesignRequest {
	t.Helper()
	_, hash, err := s.store.Draft()
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	current, err := s.store.Confirmed()
	if err != nil && !errors.Is(err, store.ErrUnconfirmed) {
		t.Fatal(err)
	}
	body := map[string]any{"kind": kind, "requirement": "整理模块职责", "instruction": "核对当前设计", "parent_id": parent,
		"expected_draft_hash": hash, "expected_revision": current.Revision}
	var result store.DesignRequest
	if err := json.Unmarshal(request(t, s, "POST", "/api/design-request", jsonBody(t, body), 200), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func importResult(t *testing.T, s *Server, r store.DesignRequest, result designer.Result, status int) store.Proposal {
	t.Helper()
	var p store.Proposal
	data := request(t, s, "POST", "/api/proposal-import", jsonBody(t, designer.Response{RequestID: r.RequestID, Result: result}), status)
	if status == 200 {
		if err := json.Unmarshal(data, &p); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func confirmDraft(t *testing.T, s *Server, revision string, status int) store.Snapshot {
	t.Helper()
	_, hash, err := s.store.Draft()
	if err != nil {
		t.Fatal(err)
	}
	var result store.Snapshot
	data := request(t, s, "POST", "/api/confirm", jsonBody(t, map[string]string{"draft_hash": hash, "expected_revision": revision, "actor": "审阅人"}), status)
	if status == 200 {
		json.Unmarshal(data, &result)
	}
	return result
}

func TestExternalDesignRefinementRecoveryAndConfirmation(t *testing.T) {
	s, _ := New(t.TempDir(), Options{})
	r := exportRequest(t, s, "design", "")
	if _, _, err := s.store.Draft(); !os.IsNotExist(err) {
		t.Fatal("导出写入了草稿")
	}
	result := designer.Result{Summary: "初步设计", Design: testproject.Design()}
	p := importResult(t, s, r, result, 200)
	if _, _, err := s.store.Draft(); !os.IsNotExist(err) {
		t.Fatal("导入写入了草稿")
	}
	// 并行响应不能覆盖已待审阅的提案；继续修改须显式关联父提案。
	importResult(t, s, r, result, 409)
	r = exportRequest(t, s, "design", p.ID)
	result.Design.Modules[0].Responsibility = "调整后的职责"
	updated := importResult(t, s, r, result, 200)
	request(t, s, "POST", "/api/proposals/"+p.ID+"/accept", "{}", 409)
	reopened, _ := New(s.root, Options{})
	if data := request(t, reopened, "GET", "/api/state", "", 200); !strings.Contains(string(data), updated.ID) {
		t.Fatal("新窗口无法发现外部提案")
	}
	request(t, reopened, "POST", "/api/proposals/"+updated.ID+"/accept", "{}", 200)
	request(t, reopened, "POST", "/api/proposals/"+updated.ID+"/accept", "{}", 409)
	if _, err := s.store.Confirmed(); !errors.Is(err, store.ErrUnconfirmed) {
		t.Fatal("接受提前确认")
	}
	if pending, _ := s.store.PendingDesignProposal(); pending != "" {
		t.Fatal("已接受提案仍待审阅")
	}
	baseline := confirmDraft(t, reopened, "", 200)
	if baseline.Design.Modules[0].Responsibility != result.Design.Modules[0].Responsibility {
		t.Fatal("确认内容错误")
	}
	request(t, s, "GET", "/api/handoff", "", 200)
}

func TestExternalInitialAndReanalysisKeepBaselineAndEvidence(t *testing.T) {
	root := initialProject(t)
	s, _ := New(root, Options{})
	request(t, s, "GET", "/api/state", "", 200)
	if _, err := os.Stat(filepath.Join(root, ".architecture")); !os.IsNotExist(err) {
		t.Fatal("打开项目产生写入")
	}
	r := exportRequest(t, s, "initial_analysis", "")
	if r.Request.Source == nil || len(r.Request.Source.Facts.Files) != 2 || len(r.Request.Source.Facts.Dependencies) != 1 {
		t.Fatal("扫描范围错误")
	}
	p := importResult(t, s, r, initialResult(), 200)
	refined := exportRequest(t, s, "design", p.ID)
	if refined.Request.Source.Fingerprint != r.Request.Source.Fingerprint {
		t.Fatal("调整丢失源码来源")
	}
	next := importResult(t, s, refined, initialResult(), 200)
	request(t, s, "POST", "/api/proposals/"+p.ID+"/accept", "{}", 409)
	request(t, s, "POST", "/api/proposals/"+next.ID+"/accept", "{}", 200)
	baseline := confirmDraft(t, s, "", 200)
	// 重新分析可处理只有确认快照、没有草稿的已有项目。
	if err := os.Remove(filepath.Join(root, ".architecture/draft.json")); err != nil {
		t.Fatal(err)
	}
	r = exportRequest(t, s, "reanalysis", "")
	if design.Fingerprint(r.Before) != baseline.DesignHash {
		t.Fatal("重新分析没有基准对照")
	}
	p = importResult(t, s, r, initialResult(), 200)
	if p.Baseline == nil || p.Baseline.Revision != baseline.Revision {
		t.Fatal("原基准证据丢失")
	}
	request(t, s, "POST", "/api/proposals/"+p.ID+"/accept", "{}", 200)
	current, _ := s.store.Confirmed()
	if current.Revision != baseline.Revision {
		t.Fatal("接受提前改写基准")
	}
	request(t, s, "POST", "/api/check", "{}", 200)
	confirmed := confirmDraft(t, s, baseline.Revision, 200)
	if confirmed.ParentRevision != baseline.Revision || confirmed.SourceAnalysisID != p.ID || confirmed.Revision == baseline.Revision {
		t.Fatal("相同设计没有确认新来源")
	}
}

func TestExternalResponseCannotCrossProjectsOrInventEvidence(t *testing.T) {
	root := initialProject(t)
	s, _ := New(root, Options{})
	r := exportRequest(t, s, "initial_analysis", "")
	other, _ := New(initialProject(t), Options{})
	importResult(t, other, r, initialResult(), 422)
	// 即使手动复制请求文件，其项目绑定也必须拒绝。
	testproject.Write(t, other.root, ".architecture/requests/"+r.RequestID+".json", jsonBody(t, r))
	importResult(t, other, r, initialResult(), 422)
	bad := initialResult()
	bad.Analysis.Evidence[0].Locations[0].File = "invented.go"
	importResult(t, s, r, bad, 422)
	if state, _ := s.store.InitialAnalysis(); state != nil {
		t.Fatal("无效产物留下分析状态")
	}
	// 导入不能传入替换过的源码或草稿上下文。
	body := jsonBody(t, designer.Response{RequestID: r.RequestID, Result: initialResult()})
	body = strings.TrimSuffix(body, "}") + `,"request":{}}`
	request(t, s, "POST", "/api/proposal-import", body, 422)
	r.Project = other.root
	testproject.Write(t, root, ".architecture/requests/"+r.RequestID+".json", jsonBody(t, r))
	importResult(t, s, r, initialResult(), 422)
}

func TestExternalSourceChangesRejectedAtImportAcceptAndConfirm(t *testing.T) {
	for _, stage := range []string{"import", "accept", "confirm"} {
		t.Run(stage, func(t *testing.T) {
			root := initialProject(t)
			s, _ := New(root, Options{})
			r := exportRequest(t, s, "initial_analysis", "")
			var p store.Proposal
			if stage != "import" {
				p = importResult(t, s, r, initialResult(), 200)
			}
			if stage == "confirm" {
				request(t, s, "POST", "/api/proposals/"+p.ID+"/accept", "{}", 200)
			}
			testproject.Write(t, root, "order/new.go", "package order\n// Added 改变源码快照。\nfunc Added() {}\n")
			switch stage {
			case "import":
				importResult(t, s, r, initialResult(), 409)
			case "accept":
				request(t, s, "POST", "/api/proposals/"+p.ID+"/accept", "{}", 409)
			case "confirm":
				confirmDraft(t, s, "", 409)
			}
			if _, err := s.store.Confirmed(); !errors.Is(err, store.ErrUnconfirmed) {
				t.Fatal("陈旧来源确认成功")
			}
		})
	}
}

func TestExternalDesignChangesInvalidateRequestAndAcceptance(t *testing.T) {
	for _, stage := range []string{"import", "accept"} {
		t.Run(stage, func(t *testing.T) {
			root, saved, baseline := testproject.New(t)
			s, _ := New(root, Options{})
			r := exportRequest(t, s, "design", "")
			result := designer.Result{Summary: "调整", Design: testproject.Design()}
			var p store.Proposal
			if stage == "accept" {
				p = importResult(t, s, r, result, 200)
			}
			changed := testproject.Design()
			changed.Modules[0].Responsibility = "外部新编辑"
			saved.SaveDraft(changed)
			if stage == "import" {
				importResult(t, s, r, result, 409)
			} else {
				request(t, s, "POST", "/api/proposals/"+p.ID+"/accept", "{}", 409)
			}
			current, _ := saved.Confirmed()
			if current.Revision != baseline.Revision {
				t.Fatal("基准被改写")
			}
		})
	}
}

func TestWithdrawnAcceptedAnalysisRetainsConfirmationGuard(t *testing.T) {
	s, _ := New(initialProject(t), Options{})
	p := importResult(t, s, exportRequest(t, s, "initial_analysis", ""), initialResult(), 200)
	request(t, s, "POST", "/api/proposals/"+p.ID+"/accept", "{}", 200)
	_, hash, _ := s.store.Draft()
	if err := s.store.DiscardInitialAnalysis(p.ID, hash, ""); err != nil {
		t.Fatal(err)
	}
	confirmDraft(t, s, "", 422)
	// 新分析再撤回也不能释放已有草稿的来源保护。
	p = importResult(t, s, exportRequest(t, s, "initial_analysis", ""), initialResult(), 200)
	if err := s.store.DiscardInitialAnalysis(p.ID, hash, ""); err != nil {
		t.Fatal(err)
	}
	confirmDraft(t, s, "", 422)
	p = importResult(t, s, exportRequest(t, s, "initial_analysis", ""), initialResult(), 200)
	request(t, s, "POST", "/api/proposals/"+p.ID+"/accept", "{}", 200)
	confirmDraft(t, s, "", 200)
}

func TestIncompleteSourceAndRemovedExecutionEndpoints(t *testing.T) {
	root := initialProject(t)
	s, _ := New(root, Options{})
	testproject.Write(t, root, "order/broken.go", "package order\nfunc Broken(\n")
	data := request(t, s, "POST", "/api/design-request", `{"kind":"initial_analysis","expected_draft_hash":"","expected_revision":""}`, 422)
	if !strings.Contains(string(data), `"source"`) || !strings.Contains(string(data), `"analysis_incomplete"`) {
		t.Fatal("缺少实际读取范围", string(data))
	}
	for _, endpoint := range []string{"proposals", "initial-analysis", "reanalysis"} {
		request(t, s, "POST", "/api/"+endpoint, "{}", 404)
	}
	if _, err := s.store.Confirmed(); !errors.Is(err, store.ErrUnconfirmed) {
		t.Fatal("失败产生基准")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.store.PrepareDesignRequest(ctx, store.DesignRequestOptions{Requirement: "未执行"}, ""); !errors.Is(err, context.Canceled) {
		t.Fatal("取消未生效", err)
	}
}
