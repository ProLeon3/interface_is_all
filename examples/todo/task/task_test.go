package task_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"example.test/workbench-todo/task"
)

func TestPersistentLifecycleAndIdempotence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	a, err := task.Add(path, "写实现说明", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := task.Add(path, "运行浏览器回归", "")
	if err != nil || a.ID == b.ID {
		t.Fatal(b, err)
	}
	if _, changed, err := task.Complete(path, a.ID); err != nil || !changed {
		t.Fatal(changed, err)
	}
	// 每次调用重新读文件；把修改时间置旧可证明重复完成没有额外写入。
	old := time.Unix(1700000000, 0)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	if _, changed, err := task.Complete(path, a.ID); err != nil || changed {
		t.Fatal(changed, err)
	}
	info, _ := os.Stat(path)
	if !info.ModTime().Equal(old) {
		t.Fatal("重复完成重写了文件")
	}
	items, err := task.Pending(path)
	if err != nil || len(items) != 1 || items[0].ID != b.ID {
		t.Fatal(items, err)
	}
	before, _ := os.ReadFile(path)
	if _, err := task.Add(path, " \t", ""); err == nil {
		t.Fatal("接受空白标题")
	}
	if _, _, err := task.Complete(path, 999); err == nil {
		t.Fatal("接受未知编号")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("无效操作改变了数据")
	}
}

func TestCorruptedDocumentsAreNotOverwritten(t *testing.T) {
	for _, content := range []string{`{broken`, `{"version":1,"next_id":2,"tasks":[{"id":1,"title":"a"},{"id":1,"title":"b"}]}`, `{"version":99,"next_id":1,"tasks":[]}`} {
		path := filepath.Join(t.TempDir(), "tasks.json")
		os.WriteFile(path, []byte(content), 0600)
		if _, err := task.Add(path, "新任务", ""); err == nil {
			t.Fatal("损坏文件被接受")
		}
		after, _ := os.ReadFile(path)
		if string(after) != content {
			t.Fatal("损坏文件被覆盖")
		}
	}
}

// 截止日期可选：未完成列表按截止日期升序，无日期居后，同日按 ID。
func TestPendingOrderedByDueDate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	for _, in := range []struct{ title, due string }{
		{"无日期甲", ""},
		{"十月十日", "2026-10-10"},
		{"十月一日乙", "2026-10-01"},
		{"已完成", "2026-09-01"},
		{"十月一日丙", "2026-10-01"},
		{"无日期丁", " "},
	} {
		if _, err := task.Add(path, in.title, in.due); err != nil {
			t.Fatal(in, err)
		}
	}
	if _, _, err := task.Complete(path, 4); err != nil {
		t.Fatal(err)
	}
	items, err := task.Pending(path)
	if err != nil {
		t.Fatal(err)
	}
	var got []uint64
	for _, item := range items {
		got = append(got, item.ID)
	}
	want := []uint64{3, 5, 2, 1, 6}
	if len(got) != len(want) {
		t.Fatal(got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("顺序 %v，期望 %v", got, want)
		}
	}
	if items[0].Due != "2026-10-01" || items[4].Due != "" {
		t.Fatal("截止日期未原样保存", items)
	}
}

// 无效截止日期在写入前被拒绝，文件保持不变。
func TestInvalidDueDateRejectedWithoutWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	if _, err := task.Add(path, "基线", ""); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	for _, due := range []string{"2026-02-30", "2026/10/01", "2026-1-5", "明天"} {
		if _, err := task.Add(path, "新任务", due); err == nil {
			t.Fatalf("接受了无效日期 %q", due)
		}
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("无效日期改变了数据")
	}
}

// 没有 due 字段的旧文档照常读取；文档中的无效日期视为损坏且不被覆盖。
func TestDueFieldInDocument(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	legacy := `{"version":1,"next_id":2,"tasks":[{"id":1,"title":"旧任务","done":false}]}`
	os.WriteFile(path, []byte(legacy), 0600)
	if items, err := task.Pending(path); err != nil || len(items) != 1 || items[0].Due != "" {
		t.Fatal(items, err)
	}
	bad := `{"version":1,"next_id":2,"tasks":[{"id":1,"title":"a","due":"2026-13-01"}]}`
	os.WriteFile(path, []byte(bad), 0600)
	if _, err := task.Add(path, "新任务", ""); err == nil {
		t.Fatal("接受了含无效日期的文档")
	}
	if after, _ := os.ReadFile(path); string(after) != bad {
		t.Fatal("损坏文件被覆盖")
	}
}
