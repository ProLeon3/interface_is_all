package storage_test

import (
	"os"
	"path/filepath"
	"testing"

	"example.test/workbench-todo/storage"
)

// 无效替换文档和已损坏的原文档都必须在写入前被拒绝。
func TestInvalidDocumentsNeverReplaceOriginal(t *testing.T) {
	for _, tc := range []struct{ original, next string }{
		{`{"valid":true}`, `{broken`},
		{`{broken`, `{"valid":true}`},
	} {
		path := filepath.Join(t.TempDir(), "document.json")
		if err := os.WriteFile(path, []byte(tc.original), 0600); err != nil {
			t.Fatal(err)
		}
		if err := storage.WriteJSON(path, []byte(tc.next)); err == nil {
			t.Fatal("接受了损坏文档")
		}
		actual, err := os.ReadFile(path)
		if err != nil || string(actual) != tc.original {
			t.Fatal("原文件被覆盖", err)
		}
	}
}
