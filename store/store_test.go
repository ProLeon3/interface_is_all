package store_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"interfaceisall/internal/testproject"
	"interfaceisall/store"
)

func TestConfirmationRequiresReviewedDraftAndCurrentBaseline(t *testing.T) {
	root, s, first := testproject.New(t)
	updated := testproject.Design()
	updated.Interfaces[0].Description = "支持部分退款，返回受理结果"
	hash, err := s.SaveDraft(updated)
	if err != nil {
		t.Fatal(err)
	}
	// 保存和读取草稿不能改变后续检查所使用的已确认版本。
	current, err := s.Confirmed()
	if err != nil || current.Revision != first.Revision {
		t.Fatalf("草稿影响了基准：%v", err)
	}
	if _, err := s.Confirm(first.DesignHash, first.Revision, "用户"); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("旧草稿应冲突：%v", err)
	}
	if _, err := s.Confirm(hash, "", "用户"); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("旧基准应冲突：%v", err)
	}
	if _, err := s.Confirm(hash, first.Revision, "  "); err == nil {
		t.Fatal("缺少确认记录")
	}
	second, err := s.Confirm(hash, first.Revision, "用户")
	if err != nil {
		t.Fatal(err)
	}
	if second.ParentRevision != first.Revision || second.DesignHash != hash {
		t.Fatal("版本链错误")
	}
	reopened, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	current, err = reopened.Confirmed()
	if err != nil || current.Revision != second.Revision {
		t.Fatalf("重启后读取失败：%v", err)
	}
	archived, err := os.ReadFile(filepath.Join(root, ".architecture/versions", first.Revision+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var previous store.Snapshot
	if err := json.Unmarshal(archived, &previous); err != nil {
		t.Fatal(err)
	}
	if err := previous.Validate(); err != nil || previous.DesignHash != first.DesignHash {
		t.Fatalf("历史版本被修改：%v", err)
	}
}

func TestLayoutArtifactsAndInvalidDraftCannotChangeConfirmedDesign(t *testing.T) {
	root, s, first := testproject.New(t)
	before, _ := os.ReadFile(filepath.Join(root, ".architecture/confirmed.json"))
	if err := s.SaveLayout(store.Layout{Nodes: map[string]store.Point{"payment": {X: 10, Y: 20}}}); err != nil {
		t.Fatal(err)
	}
	if layout, err := s.Layout(); err != nil || layout.Nodes["payment"].X != 10 {
		t.Fatalf("布局未保存：%v", err)
	}
	if _, err := s.SaveArtifact("review", first.Revision, map[string]string{"suggestion": "修改设计"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveArtifact("../confirmed", first.Revision, nil); err == nil {
		t.Fatal("观察路径应受限")
	}
	invalid := testproject.Design()
	invalid.Interfaces[0].ModuleID = "missing"
	if _, err := s.SaveDraft(invalid); err == nil {
		t.Fatal("不应保存无效设计")
	}
	_, hash, err := s.Draft()
	if err != nil || hash != first.DesignHash {
		t.Fatalf("失败保存损坏草稿：%v", err)
	}
	after, _ := os.ReadFile(filepath.Join(root, ".architecture/confirmed.json"))
	if string(before) != string(after) {
		t.Fatal("观察或布局写入改变了设计")
	}
}

// blockingArtifact 将写入暂停在可控位置，用公开存储接口验证跨操作的锁约束。
type blockingArtifact struct {
	started chan struct{}
	release chan struct{}
}

func (a blockingArtifact) MarshalJSON() ([]byte, error) {
	close(a.started)
	<-a.release
	return []byte(`{"checked":true}`), nil
}

func TestArtifactPublicationAndConfirmationShareOneLock(t *testing.T) {
	_, s, first := testproject.New(t)
	d := testproject.Design()
	d.Interfaces[0].Description = "更新的设计描述"
	hash, err := s.SaveDraft(d)
	if err != nil {
		t.Fatal(err)
	}
	value := blockingArtifact{make(chan struct{}), make(chan struct{})}
	result := make(chan error, 1)
	go func() {
		_, err := s.SaveArtifact("check", first.Revision, value)
		result <- err
	}()
	<-value.started
	_, confirmErr := s.Confirm(hash, first.Revision, "用户")
	close(value.release)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if !errors.Is(confirmErr, store.ErrLocked) {
		t.Fatalf("报告发布期间不应切换基准：%v", confirmErr)
	}
	if _, err := s.Confirm(hash, first.Revision, "用户"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveArtifact("check", first.Revision, map[string]bool{"checked": true}); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("旧基准的报告应被拒绝：%v", err)
	}
}

func TestConcurrentConfirmPublishesOnlyOneVersion(t *testing.T) {
	root, s, first := testproject.New(t)
	d := testproject.Design()
	d.Interfaces[0].Description = "新的退款描述"
	hash, err := s.SaveDraft(d)
	if err != nil {
		t.Fatal(err)
	}
	// 两个独立 Store 模拟工作台和外部工具同时确认。
	other, _ := store.Open(root)
	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for _, candidate := range []*store.Store{s, other} {
		workers.Add(1)
		go func(candidate *store.Store) {
			defer workers.Done()
			<-start
			_, err := candidate.Confirm(hash, first.Revision, "用户")
			results <- err
		}(candidate)
	}
	close(start)
	workers.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if !errors.Is(err, store.ErrLocked) && !errors.Is(err, store.ErrConflict) {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatalf("成功确认次数应为 1，实际 %d", success)
	}
	if _, err := s.Confirmed(); err != nil {
		t.Fatal("并发后基准损坏", err)
	}
}

func TestStoreRejectsTamperedSnapshotsAndSymlinkWrites(t *testing.T) {
	root, s, _ := testproject.New(t)
	name := filepath.Join(root, ".architecture/confirmed.json")
	data, _ := os.ReadFile(name)
	var snapshot store.Snapshot
	json.Unmarshal(data, &snapshot)
	snapshot.Design.Modules[0].Responsibility = "被静默修改的职责"
	corrupt, _ := json.Marshal(snapshot)
	if err := os.WriteFile(name, corrupt, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Confirmed(); err == nil {
		t.Fatal("应拒绝被改写的基准")
	}

	otherRoot := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(otherRoot, ".architecture")); err != nil {
		t.Fatal(err)
	}
	other, _ := store.Open(otherRoot)
	if _, err := other.SaveDraft(testproject.Design()); err == nil {
		t.Fatal("不能通过符号链接写出项目")
	}
	if names, _ := os.ReadDir(outside); len(names) != 0 {
		t.Fatal("外部目录被写入")
	}
}
