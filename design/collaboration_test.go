package design_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"interfaceisall/design"
	"interfaceisall/internal/testproject"
)

func TestCollaborationsValidateReferencesAndRemainSeparateFromBans(t *testing.T) {
	d := testproject.Design()
	d.Collaborations = []design.Collaboration{{ID: "order-refund", From: "order", InterfaceID: "refund", Purpose: "取消已付款订单时申请退款"}}
	if err := design.Validate(d); err != nil {
		t.Fatal(err)
	}
	// 协作不是 Go 直接包引用；同方向禁止规则不应被推断成设计结构错误。
	d.ForbiddenDependencies = append(d.ForbiddenDependencies, design.ForbiddenDependency{ID: "order-no-payment", From: "order", To: "payment", Reason: "通过注入接口协作"})
	if err := design.Validate(d); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, code string
		change     func(*design.Design)
	}{
		{"未知调用方", "unknown_module", func(d *design.Design) { d.Collaborations[0].From = "missing" }},
		{"未知接口", "unknown_interface", func(d *design.Design) { d.Collaborations[0].InterfaceID = "missing" }},
		{"本模块接口", "self_collaboration", func(d *design.Design) { d.Collaborations[0].From = "payment" }},
		{"全局重复ID", "duplicate_id", func(d *design.Design) { d.Collaborations[0].ID = "refund" }},
		{"重复关系", "duplicate_collaboration", func(d *design.Design) {
			c := d.Collaborations[0]
			c.ID = "another"
			d.Collaborations = append(d.Collaborations, c)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			copy := d
			copy.Collaborations = append([]design.Collaboration{}, d.Collaborations...)
			tc.change(&copy)
			err := design.Validate(copy)
			if err == nil {
				t.Fatal("无效协作被接受")
			}
			data, _ := json.Marshal(err)
			if !strings.Contains(string(data), tc.code) {
				t.Fatalf("错误缺少定位代码：%s", data)
			}
		})
	}
}

func TestLegacyFingerprintIsUnchanged(t *testing.T) {
	d := testproject.Design()
	// 固定旧结构的字段顺序，避免仅用新实现往返而漏掉历史快照指纹变化。
	legacy := struct {
		Version    int                          `json:"schema_version"`
		Modules    []design.Module              `json:"modules"`
		Interfaces []design.Interface           `json:"interfaces"`
		Rules      []design.ForbiddenDependency `json:"forbidden_dependencies"`
	}{d.SchemaVersion, d.Modules, d.Interfaces, d.ForbiddenDependencies}
	data, _ := json.Marshal(legacy)
	hash := sha256.Sum256(data)
	if design.Fingerprint(d) != hex.EncodeToString(hash[:]) {
		t.Fatal("旧设计指纹改变")
	}
	d.Collaborations = []design.Collaboration{}
	if design.Fingerprint(d) != hex.EncodeToString(hash[:]) {
		t.Fatal("空协作改变旧指纹")
	}
	if _, err := design.Parse(data); err != nil {
		t.Fatal(err)
	}
}
