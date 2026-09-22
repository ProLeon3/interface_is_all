package workbench

import (
	"github.com/ProLeon3/interface_is_all/design"
	"github.com/ProLeon3/interface_is_all/designer"
	"github.com/ProLeon3/interface_is_all/internal/testproject"
	"github.com/ProLeon3/interface_is_all/source"
	"testing"
)

// 测试从真实源码且没有 .architecture 的项目开始，固定响应只验证程序流程。
func initialProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	testproject.Write(t, root, "go.mod", "module example.test/shop\n\ngo 1.22\n")
	testproject.Write(t, root, "payment/refund.go", "package payment\n\n// Refund 是示例退款入口。\nfunc Refund(id string) error { return nil }\n")
	testproject.Write(t, root, "order/cancel.go", "package order\nimport \"example.test/shop/payment\"\n// Cancel 调用当前退款实现。\nfunc Cancel(id string) error { return payment.Refund(id) }\n")
	testproject.Write(t, root, "order/cancel_test.go", "package order\n")
	return root
}

func initialResult() designer.Result {
	d := testproject.Design()
	d.ForbiddenDependencies = []design.ForbiddenDependency{}
	d.Collaborations = []design.Collaboration{{ID: "order-refund", From: "order", InterfaceID: "refund", Purpose: "取消订单时调用现有退款能力"}}
	payment := source.Location{File: "payment/refund.go", Line: 4, Column: 1, EndLine: 4}
	order := source.Location{File: "order/cancel.go", Line: 4, Column: 1, EndLine: 4}
	a := &designer.Analysis{Evidence: []designer.Evidence{
		{Kind: "module", ID: "payment", Status: "supported", Explanation: "实现退款入口", Locations: []source.Location{payment}},
		{Kind: "module", ID: "order", Status: "supported", Explanation: "实现订单取消", Locations: []source.Location{order}},
		{Kind: "interface", ID: "refund", Status: "supported", Explanation: "Refund 当前返回空错误", Locations: []source.Location{payment}},
		{Kind: "collaboration", ID: "order-refund", Status: "supported", Explanation: "Cancel 直接调用 Refund", Locations: []source.Location{order, payment}},
	}, Issues: []designer.Finding{{Description: "退款尚无实际业务逻辑", Locations: []source.Location{payment}}}, Suggestions: []designer.Finding{{Description: "后续可增加实际退款处理，未纳入现状设计", Locations: []source.Location{payment}}}, Uncertainties: []string{"无法据此证明生产环境退款行为"}}
	return designer.Result{Summary: "还原当前订单与退款代码", Design: d, Analysis: a}
}
