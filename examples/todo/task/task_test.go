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
	a, err := task.Add(path, "写实现说明")
	if err != nil {
		t.Fatal(err)
	}
	b, err := task.Add(path, "运行浏览器回归")
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
	if _, err := task.Add(path, " \t"); err == nil {
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
		if _, err := task.Add(path, "新任务"); err == nil {
			t.Fatal("损坏文件被接受")
		}
		after, _ := os.ReadFile(path)
		if string(after) != content {
			t.Fatal("损坏文件被覆盖")
		}
	}
}
