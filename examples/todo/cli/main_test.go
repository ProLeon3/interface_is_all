package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// 从用户命令入口验证输出、持久化、错误与退出状态。
func TestCommandJourney(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	for _, tc := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"add", "真实", "待办"}, 0, "1\t真实 待办"},
		{[]string{"list"}, 0, "真实 待办"},
		{[]string{"done", "1"}, 0, "已完成"},
		{[]string{"done", "1"}, 0, "已经完成"},
		{[]string{"list"}, 0, ""},
		{[]string{"done", "999"}, 2, "不存在"},
		{[]string{"done", "bad"}, 2, "正整数"},
		{[]string{"add", " "}, 2, "不能为空"},
	} {
		var out, stderr bytes.Buffer
		code := Run(append([]string{"-file", path}, tc.args...), &out, &stderr)
		actual := out.String() + stderr.String()
		if code != tc.code || !strings.Contains(actual, tc.want) || tc.want == "" && actual != "" {
			t.Fatalf("%v: code=%d 输出=%q", tc.args, code, actual)
		}
	}
}
