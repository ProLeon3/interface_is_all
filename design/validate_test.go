package design_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"interfaceisall/design"
	"interfaceisall/internal/testproject"
)

func TestDesignRejectsInvalidConstraints(t *testing.T) {
	// 覆盖跨对象约束和路径段边界，避免仅重复检查结构体字段。
	cases := []struct {
		name   string
		change func(*design.Design)
		code   string
	}{
		{"重复ID", func(d *design.Design) { d.Interfaces[0].ID = "payment" }, "duplicate_id"},
		{"未知所属模块", func(d *design.Design) { d.Interfaces[0].ModuleID = "missing" }, "unknown_module"},
		{"未知依赖端点", func(d *design.Design) { d.ForbiddenDependencies[0].To = "missing" }, "unknown_module"},
		{"目录包含", func(d *design.Design) { d.Modules[1].Root = "payment/child" }, "overlapping_roots"},
		{"根目录独占", func(d *design.Design) { d.Modules[0].Root = "." }, "overlapping_roots"},
		{"目录相同", func(d *design.Design) { d.Modules[1].Root = "payment" }, "overlapping_roots"},
		{"路径越界", func(d *design.Design) { d.Modules[0].Root = "../payment" }, "invalid_root"},
		{"绝对路径", func(d *design.Design) { d.Modules[0].Root = "/payment" }, "invalid_root"},
		{"非规范路径", func(d *design.Design) { d.Modules[0].Root = "payment/../pay" }, "invalid_root"},
		{"反斜杠", func(d *design.Design) { d.Modules[0].Root = `payment\child` }, "invalid_root"},
		{"自引用规则", func(d *design.Design) { d.ForbiddenDependencies[0].To = "payment" }, "self_dependency"},
		{"重复方向", func(d *design.Design) {
			rule := d.ForbiddenDependencies[0]
			rule.ID = "another-rule"
			d.ForbiddenDependencies = append(d.ForbiddenDependencies, rule)
		}, "duplicate_rule"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := testproject.Design()
			tc.change(&d)
			var invalid *design.ValidationError
			if !errors.As(design.Validate(d), &invalid) {
				t.Fatal("应返回设计校验错误")
			}
			for _, issue := range invalid.Issues {
				if issue.Code == tc.code {
					return
				}
			}
			t.Fatalf("未找到 %s：%+v", tc.code, invalid)
		})
	}
	d := testproject.Design()
	d.Modules[1].Root = "payment-other"
	if err := design.Validate(d); err != nil {
		t.Fatal(err)
	}
	if d.ModuleAt("payment/api") != "payment" || d.ModuleAt("payment-other") != "order" || d.ModuleAt("payments") != "" {
		t.Fatal("目录归属必须按路径段匹配")
	}
}

func TestSchemaRejectsAmbiguousOrIncompleteJSON(t *testing.T) {
	good, _ := json.Marshal(testproject.Design())
	cases := []string{
		`null`, `{}`, string(good) + ` {}`,
		strings.Replace(string(good), `"schema_version":1`, `"schema_version":2`, 1),
		strings.Replace(string(good), `"schema_version":1`, `"schema_version":1,"schema_version":1`, 1),
		strings.Replace(string(good), `"modules":`, `"unknown":true,"modules":`, 1),
		strings.Replace(string(good), `"申请退款"`, `"   "`, 1),
		strings.Replace(string(good), `"module_id":"payment"`, `"module_id":null`, 1),
	}
	for _, input := range cases {
		if _, err := design.Parse([]byte(input)); err == nil {
			t.Fatalf("不应接受：%s", input)
		}
	}
	parsed, err := design.Parse(good)
	if err != nil || design.Fingerprint(parsed) != design.Fingerprint(testproject.Design()) {
		t.Fatalf("有效设计未能往返：%v", err)
	}
}

func TestProjectValidationAllowsFutureDirectoriesButRejectsAliases(t *testing.T) {
	root := t.TempDir()
	d := testproject.Design()
	if err := design.ValidateProject(root, d); err != nil {
		t.Fatal("尚未实现的设计必须能够保存", err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "payment")); err != nil {
		t.Fatal(err)
	}
	if err := design.ValidateProject(root, d); err == nil {
		t.Fatal("不能通过符号链接改变目录归属")
	}
}
