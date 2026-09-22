package store_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"interfaceisall/internal/testproject"
	"interfaceisall/store"
)

func TestConditionalDraftSaveRequiresBothReadVersions(t *testing.T) {
	root, first, baseline := testproject.New(t)
	second, _ := store.Open(root)
	d := testproject.Design()
	d.Modules[0].Responsibility = "浏览器草稿"
	hash, err := first.SaveDraftIfUnchanged(d, baseline.DesignHash, baseline.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := second.SaveDraftIfUnchanged(testproject.Design(), baseline.DesignHash, baseline.Revision); !errors.Is(err, store.ErrConflict) {
		t.Fatal("旧草稿覆盖了最新草稿", err)
	}
	if _, err := second.Confirm(hash, baseline.Revision, "外部用户"); err != nil {
		t.Fatal(err)
	}
	if _, err := first.SaveDraftIfUnchanged(testproject.Design(), hash, baseline.Revision); !errors.Is(err, store.ErrConflict) {
		t.Fatal("新确认版本未使旧窗口保存失效", err)
	}
}

func TestObservationOrderingAndPathBoundary(t *testing.T) {
	root, s, baseline := testproject.New(t)
	older, _ := s.SaveArtifact("review", baseline.Revision, map[string]string{"result": "older"})
	newer, _ := s.SaveArtifact("review", baseline.Revision, map[string]string{"result": "newer"})
	// 文件名是随机数；强制不同时间，验证读取依据是修改时间。
	oldTime := time.Now().Add(-time.Hour)
	if err := os.Chtimes(older, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveArtifact("review-request", baseline.Revision, map[string]string{"request": "not a review"}); err != nil {
		t.Fatal(err)
	}
	latest, err := s.LatestObservation("review")
	if err != nil || latest.Name != filepath.Base(newer) {
		t.Fatal("读取错了报告或把请求当成结果", latest, err)
	}
	if _, err := s.LatestObservation("../confirmed"); err == nil {
		t.Fatal("不应接受任意观察路径")
	}
	if err := os.Symlink(older, filepath.Join(root, ".architecture/observations/review-link.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LatestObservation("review"); err == nil {
		t.Fatal("不应通过符号链接读取报告")
	}
}

func TestSeparateWindowLayoutUpdatesPreserveOtherNodes(t *testing.T) {
	root, first, baseline := testproject.New(t)
	second, _ := store.Open(root)
	if _, err := first.UpdateLayout(map[string]store.Point{"payment": {X: 111, Y: 222}}); err != nil {
		t.Fatal(err)
	}
	// 第二个窗口只提交它移动的节点，不携带旧的 payment 坐标。
	layout, err := second.UpdateLayout(map[string]store.Point{"order": {X: 333, Y: 444}})
	if err != nil || layout.Nodes["payment"].X != 111 || layout.Nodes["order"].X != 333 {
		t.Fatal("独立节点的布局更新互相覆盖", layout, err)
	}
	current, err := first.Confirmed()
	if err != nil || current.Revision != baseline.Revision {
		t.Fatal("布局改变了设计基准", err)
	}
}
