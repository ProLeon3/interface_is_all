// 浏览器回归中扮演 coding agent 的固定响应替身：读取工作台导出的请求，返回固定提案。
// 它注册在浏览器上下文里，刷新和新标签页都可用；不访问网络或模型，不代表真实 agent 效果。
async page => {
  await page.context().addInitScript(() => {
    const clone = value => JSON.parse(JSON.stringify(value));
    // 只用于 examples/shop 副本的源码分析样本，行号从本次实际读取的文件中定位。
    const analysis = context => {
      const files = Object.fromEntries(context.source.facts.files.map(file => [file.path, file.content]));
      const location = (path, needle) => {
        const line = (files[path] || '').split('\n').findIndex(text => text.includes(needle)) + 1;
        if (!line) throw new Error(`固定替身找不到 ${path} 中的 ${needle}`);
        return {file: path, line, column: 1, end_line: line};
      };
      const caller = location('order/cancel.go', 'return api.RequestRefund');
      const provider = location('payment/api/refund.go', 'if paymentID ==');
      const design = {
        schema_version: 1,
        modules: [
          {id: 'order', root: 'order', responsibility: '按订单支付标识发起退款'},
          {id: 'payment', root: 'payment', responsibility: '提供示例退款入口，当前仅检查支付标识'},
        ],
        interfaces: [{id: 'request-refund', module_id: 'payment', name: '申请退款', description: '校验支付标识，空值返回错误；当前没有真实支付调用', semantics: {inputs: '支付标识', outputs: '错误或空错误', errors: '支付标识不能为空'}}],
        collaborations: [{id: 'order-refund', from: 'order', interface_id: 'request-refund', purpose: 'Cancel 直接调用 RequestRefund'}],
        forbidden_dependencies: [],
      };
      const evidence = [
        ['module', 'order', '取消入口委托退款', [caller]],
        ['module', 'payment', '现有逻辑只有支付标识校验', [provider]],
        ['interface', 'request-refund', '示例能力由函数提供，非 Go interface', [provider]],
        ['collaboration', 'order-refund', '引用调用语句和目标实现，不能仅凭包导入判断', [caller, provider]],
      ].map(([kind, id, explanation, locations]) => ({kind, id, status: 'supported', explanation, locations}));
      return {summary: '固定回归样本：还原订单直接调用支付退款入口的现状。', design,
        analysis: {evidence, issues: [{description: '退款入口尚未执行真实退款', locations: [provider]}],
          suggestions: [{description: '后续可补充退款服务；本次不虚构已有实现', locations: [provider]}],
          uncertainties: ['真实支付系统与运行时业务行为不在样例范围。']}};
    };
    // 需求设计样本按修改意图联动调整，覆盖新建、拆分迁移、语义细化和删除四种变更。
    const design = context => {
      let d = clone(context.design);
      const instruction = context.instruction || '';
      if (!d.modules.length) {
        d = {
          schema_version: 1,
          modules: [
            {id: 'order', root: 'order', responsibility: '管理订单取消，已付款订单通过退款能力协作'},
            {id: 'payment', root: 'payment', responsibility: '处理收款和退款申请'},
          ],
          interfaces: [
            {id: 'cancel-order', module_id: 'order', name: '取消订单', description: '对已付款订单申请退款后取消订单'},
            {id: 'request-refund', module_id: 'payment', name: '申请退款', description: '根据支付记录受理退款', semantics: {inputs: '支付标识和退款金额', outputs: '退款申请编号', errors: '支付不存在或金额超出可退额度'}},
            {id: 'charge', module_id: 'payment', name: '收款', description: '收取订单款项'},
          ],
          collaborations: [
            {id: 'order-refund', from: 'order', interface_id: 'request-refund', purpose: '取消已付款订单时先申请退款'},
            {id: 'order-charge', from: 'order', interface_id: 'charge', purpose: '订单收款'},
          ],
          forbidden_dependencies: [{id: 'payment-no-order', from: 'payment', to: 'order', reason: '支付不管理订单生命周期'}],
        };
      } else if (instruction.includes('独立')) {
        d.modules.push({id: 'refund', root: 'refund', responsibility: '受理退款申请并验证可退款额度'});
        for (const module of d.modules) if (module.id === 'payment') module.responsibility = '只负责收款，不管理退款生命周期';
        for (const api of d.interfaces) if (api.id === 'request-refund') api.module_id = 'refund';
      } else if (instruction.includes('删除支付')) {
        d.modules = d.modules.filter(m => m.id !== 'payment');
        d.interfaces = d.interfaces.filter(a => a.module_id !== 'payment');
        d.collaborations = (d.collaborations || []).filter(c => c.interface_id !== 'charge');
        d.forbidden_dependencies = [];
      } else {
        for (const api of d.interfaces) if (api.id === 'request-refund') (api.semantics ||= {}).errors = '退款金额越界时拒绝；重复请求返回原退款申请编号';
      }
      return {summary: '固定回归样本：联动调整模块职责、接口归属与协作约定。', design: d};
    };
    // 输入是 design-request 导出的完整请求；输出遵循其中 response_schema 的顶层字段。
    window.__fixedAgent = request => {
      const context = request.request;
      const result = context.source ? analysis(context) : design(context);
      return {request_id: request.request_id, ...result};
    };
  });
  await page.reload();
  return 'fixed agent ready';
}
