// Package api 演示公开能力位于子包的合法组织方式。
package api

import "errors"

// RequestRefund 演示退款能力的代码入口，名称与设计 ID 无需一致。
// 此示例只完成参数检查，用于演示检查流程，不连接真实支付系统。
func RequestRefund(paymentID string) error {
	if paymentID == "" {
		return errors.New("支付标识不能为空")
	}
	return nil
}
