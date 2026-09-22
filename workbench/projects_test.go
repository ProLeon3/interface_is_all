package workbench

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ProLeon3/interface_is_all/design"
	"github.com/ProLeon3/interface_is_all/internal/testproject"
	"github.com/ProLeon3/interface_is_all/store"
)

// scopedRequest 模拟不同标签页固定自己的项目路径，同一个服务可以同时处理两个项目。
func scopedRequest(t *testing.T, server *Server, root, method, path, body string, expected int) []byte {
	t.Helper()
	req := httptest.NewRequest(method, "http://127.0.0.1:8090"+path, strings.NewReader(body))
	req.Header.Set("X-Workbench-Project", url.PathEscape(root))
	req.Header.Set("X-Workbench-Token", server.token)
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, req)
	if response.Code != expected {
		t.Fatalf("项目 %s %s %s：状态 %d，预期 %d：%s", root, method, path, response.Code, expected, response.Body.String())
	}
	return response.Body.Bytes()
}

func TestProjectSelectionDoesNotCreateOrModifyDesign(t *testing.T) {
	root := t.TempDir()
	selected := filepath.Join(root, "中文 项目")
	if err := os.Mkdir(selected, 0755); err != nil {
		t.Fatal(err)
	}
	s, _ := New(root, Options{})
	var state struct {
		Project   map[string]string `json:"project"`
		Draft     *design.Design    `json:"draft"`
		Confirmed *store.Snapshot   `json:"confirmed"`
	}
	json.Unmarshal(request(t, s, "POST", "/api/projects/open", jsonBody(t, map[string]string{"path": selected}), 200), &state)
	if state.Project["path"] != selected || state.Draft != nil || state.Confirmed != nil {
		t.Fatal("没有正确打开尚无设计的新项目", state)
	}
	if _, err := os.Stat(filepath.Join(selected, ".architecture")); !os.IsNotExist(err) {
		t.Fatal("只选择目录就创建了设计存储")
	}
	json.Unmarshal(scopedRequest(t, s, selected, "GET", "/api/state", "", 200), &state)
	if state.Project["path"] != selected {
		t.Fatal("中文路径请求头没有正确解析")
	}
	json.Unmarshal(request(t, s, "GET", "/api/state", "", 200), &state)
	if state.Project["path"] != root {
		t.Fatal("打开项目改变了其他未选择项目的标签页")
	}
	request(t, s, "POST", "/api/projects/open", `{"path":""}`, 422)
	request(t, s, "POST", "/api/projects/open", jsonBody(t, map[string]string{"path": filepath.Join(root, "不存在")}), 422)
}

func TestDirectoryBrowserListsFoldersAndProjectMarkers(t *testing.T) {
	root := t.TempDir()
	testproject.Write(t, root, "shop/go.mod", "module example.test/shop\n")
	testproject.Write(t, root, "notes/readme.txt", "只列目录，不读取这个文件")
	testproject.Write(t, root, ".hidden/readme.txt", "隐藏目录")
	if err := os.MkdirAll(filepath.Join(root, "shop/.architecture"), 0755); err != nil {
		t.Fatal(err)
	}
	s, _ := New(root, Options{})
	var result struct {
		Current     directoryEntry   `json:"current"`
		Parent      string           `json:"parent"`
		Directories []directoryEntry `json:"directories"`
	}
	json.Unmarshal(request(t, s, "POST", "/api/directories", jsonBody(t, map[string]string{"path": root}), 200), &result)
	if result.Current.Path != root || result.Parent != filepath.Dir(root) || len(result.Directories) != 2 {
		t.Fatal("目录浏览结果错误", result)
	}
	shop := result.Directories[1]
	if shop.Name != "shop" || !shop.HasDesign || !shop.HasGoMod {
		t.Fatal("没有显示项目标记", shop)
	}
	json.Unmarshal(request(t, s, "POST", "/api/directories", jsonBody(t, map[string]any{"path": root, "show_hidden": true}), 200), &result)
	if len(result.Directories) != 3 || result.Directories[0].Name != ".hidden" {
		t.Fatal("无法浏览隐藏目录", result)
	}
	request(t, s, "POST", "/api/directories", jsonBody(t, map[string]string{"path": filepath.Join(root, "shop/go.mod")}), 422)
	request(t, s, "POST", "/api/projects/open", jsonBody(t, map[string]string{"path": filepath.Join(root, "shop/go.mod")}), 422)
	// 目录列表也必须受到令牌保护，不能被任意网页跨站读取。
	unauthorized := httptest.NewRequest("POST", "http://127.0.0.1:8090/api/directories", strings.NewReader(jsonBody(t, map[string]string{"path": root})))
	response := httptest.NewRecorder()
	s.ServeHTTP(response, unauthorized)
	if response.Code != http.StatusForbidden {
		t.Fatal("未验证目录浏览的会话令牌")
	}
}

func TestSelectedProjectWritesAndChecksStayInTheirOwnDirectory(t *testing.T) {
	firstRoot, firstStore, first := testproject.New(t)
	secondRoot, secondStore, second := testproject.New(t)
	s, _ := New(firstRoot, Options{})
	request(t, s, "POST", "/api/projects/open", jsonBody(t, map[string]string{"path": secondRoot}), 200)
	d := testproject.Design()
	d.Modules[0].Responsibility = "只属于第二个项目的新草稿"
	scopedRequest(t, s, secondRoot, "PUT", "/api/draft", jsonBody(t, map[string]any{"design": d, "expected_draft_hash": second.DesignHash, "expected_revision": second.Revision}), 200)
	firstDraft, _, _ := firstStore.Draft()
	secondDraft, secondHash, _ := secondStore.Draft()
	if firstDraft.Modules[0].Responsibility == d.Modules[0].Responsibility || secondDraft.Modules[0].Responsibility != d.Modules[0].Responsibility {
		t.Fatal("跨项目保存覆盖了错误目录")
	}
	scopedRequest(t, s, secondRoot, "POST", "/api/confirm", jsonBody(t, map[string]any{"draft_hash": secondHash, "expected_revision": second.Revision, "actor": "第二项目用户"}), 200)
	scopedRequest(t, s, secondRoot, "PATCH", "/api/layout", `{"nodes":{"payment":{"x":99,"y":88}}}`, 200)
	scopedRequest(t, s, secondRoot, "POST", "/api/check", `{}`, 200)
	if latest, _ := firstStore.Confirmed(); latest.Revision != first.Revision {
		t.Fatal("确认改变了第一个项目的版本")
	}
	if _, err := firstStore.Layout(); !os.IsNotExist(err) {
		t.Fatal("布局保存到了第一个项目")
	}
	if report, _ := firstStore.LatestObservation("check"); report != nil {
		t.Fatal("检查报告保存到了第一个项目")
	}
	if report, err := secondStore.LatestObservation("check"); err != nil || report == nil {
		t.Fatal("第二个项目没有检查结果", err)
	}
	// 另一个旧标签页仍可使用它原先读取的第一个项目版本保存。
	d.Modules[0].Responsibility = "第一个项目自己的修改"
	scopedRequest(t, s, firstRoot, "PUT", "/api/draft", jsonBody(t, map[string]any{"design": d, "expected_draft_hash": first.DesignHash, "expected_revision": first.Revision}), 200)
}

func TestSessionAllowsEscapingAnInvalidInitialDesign(t *testing.T) {
	root := t.TempDir()
	testproject.Write(t, root, ".architecture/draft.json", "损坏的草稿")
	s, _ := New(root, Options{})
	request(t, s, "GET", "/api/state", "", 422)
	var session struct {
		Token string `json:"token"`
	}
	json.Unmarshal(request(t, s, "GET", "/api/session", "", 200), &session)
	if session.Token == "" {
		t.Fatal("初始草稿损坏后无法选择其他项目")
	}
	request(t, s, "POST", "/api/projects/open", jsonBody(t, map[string]string{"path": t.TempDir()}), 200)
}
