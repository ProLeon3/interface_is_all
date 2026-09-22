#!/usr/bin/env python3
"""仅供程序回归的固定模型替身，不代表真实 AI 生成效果。"""
import copy
import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


def initial_analysis(context):
    """只用于 examples/shop 的固定回归，行号从本次实际文件中定位。"""
    files = {item['path']: item['content'] for item in context['source']['facts']['files']}
    def location(path, needle):
        line = next(i + 1 for i, text in enumerate(files[path].splitlines()) if needle in text)
        return {'file': path, 'line': line, 'column': 1, 'end_line': line}
    caller = location('order/cancel.go', 'return api.RequestRefund')
    provider = location('payment/api/refund.go', 'if paymentID ==')
    design = {
        'schema_version': 1,
        'modules': [
            {'id': 'order', 'root': 'order', 'responsibility': '按订单支付标识发起退款'},
            {'id': 'payment', 'root': 'payment', 'responsibility': '提供示例退款入口，当前仅检查支付标识'},
        ],
        'interfaces': [{'id': 'request-refund', 'module_id': 'payment', 'name': '申请退款', 'description': '校验支付标识，空值返回错误；当前没有真实支付调用', 'semantics': {'inputs': '支付标识', 'outputs': '错误或空错误', 'errors': '支付标识不能为空'}}],
        'collaborations': [{'id': 'order-refund', 'from': 'order', 'interface_id': 'request-refund', 'purpose': 'Cancel 直接调用 RequestRefund'}],
        'forbidden_dependencies': [],
    }
    evidence = [
        {'kind': kind, 'id': ident, 'status': 'supported', 'explanation': explanation, 'locations': locations}
        for kind, ident, explanation, locations in [
            ('module', 'order', '取消入口委托退款', [caller]),
            ('module', 'payment', '现有逻辑只有支付标识校验', [provider]),
            ('interface', 'request-refund', '示例能力由函数提供，非 Go interface', [provider]),
            ('collaboration', 'order-refund', '引用调用语句和目标实现，不能仅凭包导入判断', [caller, provider]),
        ]
    ]
    return {'summary': '固定回归样本：还原订单直接调用支付退款入口的现状。', 'design': design,
            'analysis': {'evidence': evidence, 'issues': [{'description': '退款入口尚未执行真实退款', 'locations': [provider]}],
                         'suggestions': [{'description': '后续可补充退款服务；本次不虚构已有实现', 'locations': [provider]}],
                         'uncertainties': ['真实支付系统与运行时业务行为不在样例范围。']}}


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_POST(self):
        request = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
        context = json.loads(request['messages'][-1]['content'])
        d = copy.deepcopy(context['design'])
        instruction = context['instruction']
        if not d['modules']:
            d = {
                'schema_version': 1,
                'modules': [
                    {'id': 'order', 'root': 'order', 'responsibility': '管理订单取消，已付款订单通过退款能力协作'},
                    {'id': 'payment', 'root': 'payment', 'responsibility': '处理收款和退款申请'},
                ],
                'interfaces': [
                    {'id': 'cancel-order', 'module_id': 'order', 'name': '取消订单', 'description': '对已付款订单申请退款后取消订单'},
                    {'id': 'request-refund', 'module_id': 'payment', 'name': '申请退款', 'description': '根据支付记录受理退款', 'semantics': {'inputs': '支付标识和退款金额', 'outputs': '退款申请编号', 'errors': '支付不存在或金额超出可退额度'}},
                    {'id': 'charge', 'module_id': 'payment', 'name': '收款', 'description': '收取订单款项'},
                ],
                'collaborations': [
                    {'id': 'order-refund', 'from': 'order', 'interface_id': 'request-refund', 'purpose': '取消已付款订单时先申请退款'},
                    {'id': 'order-charge', 'from': 'order', 'interface_id': 'charge', 'purpose': '订单收款'},
                ],
                'forbidden_dependencies': [{'id': 'payment-no-order', 'from': 'payment', 'to': 'order', 'reason': '支付不管理订单生命周期'}],
            }
        elif '独立' in instruction:
            d['modules'].append({'id': 'refund', 'root': 'refund', 'responsibility': '受理退款申请并验证可退款额度'})
            for module in d['modules']:
                if module['id'] == 'payment':
                    module['responsibility'] = '只负责收款，不管理退款生命周期'
            for api in d['interfaces']:
                if api['id'] == 'request-refund':
                    api['module_id'] = 'refund'
        elif '删除支付' in instruction:
            d['modules'] = [m for m in d['modules'] if m['id'] != 'payment']
            d['interfaces'] = [a for a in d['interfaces'] if a['module_id'] != 'payment']
            d['collaborations'] = [c for c in d['collaborations'] if c['interface_id'] != 'charge']
            d['forbidden_dependencies'] = []
        else:
            for api in d['interfaces']:
                if api['id'] == 'request-refund':
                    api.setdefault('semantics', {})['errors'] = '退款金额越界时拒绝；重复请求返回原退款申请编号'
        result = {'summary': '固定回归样本：联动调整模块职责、接口归属与协作约定。', 'design': d}
        if context.get('source'):
            result = initial_analysis(context)
        data = json.dumps({'choices': [{'finish_reason': 'stop', 'message': {'content': json.dumps(result, ensure_ascii=False)}}]}).encode()
        self.send_response(200)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(data)))
        self.end_headers()
        self.wfile.write(data)


if __name__ == '__main__':
    server = ThreadingHTTPServer(('127.0.0.1', 0), Handler)
    print(f'http://127.0.0.1:{server.server_port}/v1', flush=True)
    server.serve_forever()
