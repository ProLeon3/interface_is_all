// Package order 演示未被禁止的跨模块依赖默认允许。
package order

import "example.test/shop/payment/api"

// Cancel 按订单关联的支付标识发起退款。
func Cancel(paymentID string) error {
	return api.RequestRefund(paymentID)
}
