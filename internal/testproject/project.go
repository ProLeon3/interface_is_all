// Package testproject 为存储和扫描的行为测试创建实际 Go 项目。
package testproject

import (
	"os"
	"path/filepath"
	"testing"

	"interfaceisall/design"
	"interfaceisall/store"
)

func Design() design.Design {
	return design.Design{
		SchemaVersion: 1,
		Modules: []design.Module{
			{ID: "payment", Root: "payment", Responsibility: "处理支付和退款，不维护订单"},
			{ID: "order", Root: "order", Responsibility: "管理订单生命周期"},
		},
		Interfaces:            []design.Interface{{ID: "refund", ModuleID: "payment", Name: "申请退款", Description: "对已有支付申请退款，返回处理结果"}},
		ForbiddenDependencies: []design.ForbiddenDependency{{ID: "payment-no-order", From: "payment", To: "order", Reason: "支付不依赖订单"}},
	}
}

func Write(t *testing.T, root, relative, content string) {
	t.Helper()
	name := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func New(t *testing.T) (string, *store.Store, store.Snapshot) {
	t.Helper()
	root := t.TempDir()
	Write(t, root, "go.mod", "module example.test/shop\n\ngo 1.22\n")
	Write(t, root, "payment/api/refund.go", `package api

// RequestRefund 申请退款；名称不要求与设计接口 ID 一致。
func RequestRefund(paymentID string) error { return refund(paymentID) }

// refund 的实现也必须进入语义审查上下文。
func refund(paymentID string) error { return nil }
`)
	Write(t, root, "order/order.go", "package order\n\n// ID 标识订单。\ntype ID string\n")
	s, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := s.SaveDraft(Design())
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.Confirm(hash, "", "测试用户")
	if err != nil {
		t.Fatal(err)
	}
	return root, s, snapshot
}
