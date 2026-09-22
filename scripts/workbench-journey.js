// 用 playwright-cli run-code 执行。前置条件：当前页面关联一个尚无设计的 examples/shop 副本。
async page => {
  const assert = (condition, message) => { if (!condition) throw new Error(message); };
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  const readState = () => page.evaluate(async () => (await fetch('/api/state')).json());
  const dialog = page.getByRole('dialog');
  const submit = name => dialog.getByRole('button', {name,exact:true}).click();
  const initial = await readState();
  assert(initial.draft === null && initial.confirmed === null, '必须在没有设计的临时项目运行，避免改动已有工作');
  await page.setViewportSize({width:1440,height:900});
  await page.getByRole('heading',{name:'在这里梳理模块与接口'}).waitFor();
  await page.screenshot({path:'empty-desktop.png'});

  const addModule = async (id, responsibility) => {
    await page.locator('#add-module').click();
    await dialog.getByLabel('模块 ID').fill(id);
    await dialog.getByLabel('模块目录').fill(id);
    await dialog.getByLabel('职责与边界').fill(responsibility);
    await submit('添加到草稿');
  };
  await addModule('payment','处理支付和退款，不管理订单生命周期');
  await page.getByRole('button',{name:'＋ 添加',exact:true}).first().click();
  await dialog.getByLabel('接口 ID').fill('payment.refund');
  await dialog.getByLabel('能力名称').fill('申请退款');
  await dialog.getByLabel('能力描述').fill('对已有支付申请退款，返回受理结果');
  await dialog.getByLabel('输入含义').fill('需要退款的支付标识');
  await dialog.getByLabel('输出含义').fill('退款请求是否被受理');
  await dialog.getByLabel('错误语义').fill('支付标识为空时返回错误');
  await submit('应用到草稿');
  await addModule('order','管理订单，并通过支付模块申请退款');
  await page.getByRole('button',{name:'＋ 添加',exact:true}).first().click();
  await dialog.getByLabel('接口 ID').fill('order.cancel');
  await dialog.getByLabel('能力名称').fill('取消已支付订单');
  await dialog.getByLabel('能力描述').fill('根据订单对应的支付标识申请退款，返回处理结果');
  await submit('应用到草稿');
  await page.locator('[data-action="new-rule"]').click();
  await dialog.getByLabel('规则 ID').fill('order-no-payment');
  await dialog.getByLabel('起点模块').selectOption('order');
  await dialog.getByLabel('禁止依赖的目标').selectOption('payment');
  await dialog.getByLabel('禁止原因').fill('测试设计约束：订单不得直接引用支付');
  await submit('应用到草稿');

  // 验证关系校验的错误能够回到页面，而不是保存半份无效设计。
  await page.getByLabel('模块目录').fill('payment/sub');
  await page.locator('#save-draft').click();
  await page.locator('#notices').getByRole('alert').waitFor();
  assert((await readState()).draft === null, '无效草稿被保存');
  await page.getByLabel('模块目录').fill('order');
  await page.locator('#save-draft').click();
  await page.locator('#review-design:not([disabled])').waitFor();
  const saved = await readState();
  assert(saved.draft.modules.length === 2 && saved.draft.interfaces.length === 2, '设计草稿未完整保存');
  assert(saved.confirmed === null, '保存草稿提前确认了设计');
  await page.locator('#review-design').click();
  await dialog.getByLabel('确认人').fill('浏览器验收用户');
  await submit('确认此设计版本');
  assert((await readState()).confirmed === null, '未勾选审阅声明也确认了设计');
  await dialog.getByRole('checkbox').check();
  await page.screenshot({path:'confirm-desktop.png'});
  await submit('确认此设计版本');
  await dialog.waitFor({state:'hidden'});
  await page.waitForFunction(() => !document.querySelector('#confirmed-view').disabled);
  const baseline = await readState();
  assert(baseline.confirmed.design_hash === saved.draft_hash, '确认内容不是审阅时的草稿');

  // 鼠标真实拖动后重新加载，验证布局持久化且没有创建设计版本。
  const grip = page.getByRole('button',{name:'移动 payment 模块'});
  const box = await grip.boundingBox();
  await page.mouse.move(box.x + box.width / 2,box.y + box.height / 2);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width / 2 + 25,box.y + box.height / 2 + 70,{steps:8});
  await page.mouse.up();
  await page.waitForFunction(() => document.querySelector('#layout-status').textContent.includes('布局已保存'));
  const positioned = await readState();
  await page.reload();
  await page.locator('[data-node="payment"]').waitFor();
  assert((await readState()).layout.nodes.payment.y === positioned.layout.nodes.payment.y, '布局没有在刷新后恢复');
  assert((await readState()).confirmed.revision === baseline.confirmed.revision, '拖动布局改变了设计版本');
  await page.screenshot({path:'design-desktop.png'});

  // 报告数量会附在导航名称后，仍按可访问名称识别代码检查入口。
  await page.getByRole('button',{name:/^代码检查/}).click();
  await page.locator('#run-check').click();
  await page.locator('.finding.violation').waitFor();
  assert(await page.getByText('语义审查尚未运行',{exact:true}).isVisible(), '依赖检查冒充语义审查');
  await page.locator('.finding.violation .evidence-button').click();
  assert((await dialog.locator('.code-line.highlight').count()) > 0, '违规没有定位到源码快照');
  await submit('关闭');
  await page.screenshot({path:'checks-desktop.png'});

  // 使用固定响应验证导入链路；这些数据不能作为真实模型准确率的证明。
  await page.locator('#capability-review-tools > summary').click();
  const requestResponse = page.waitForResponse(response => response.url().endsWith('/api/review-request') && response.request().method() === 'POST');
  const downloadEvent = page.waitForEvent('download');
  await page.locator('#review-request').click();
  const reviewRequest = await (await requestResponse).json();
  const downloaded = await downloadEvent;
  assert(downloaded.suggestedFilename() === 'review-request.json', '能力审查请求未导出');
  const requestPath = await downloaded.path();
  const response = {schema_version:1,request_id:reviewRequest.request_id,concerns:[],interfaces:reviewRequest.baseline.design.interfaces.map(item => {
    const entry = reviewRequest.facts.entries.find(entry => entry.module_id === item.module_id && entry.kind === 'function');
    assert(entry, `未扫描到 ${item.module_id} 的公开入口`);
    return {interface_id:item.id,entry_ids:[entry.id],assessment:'uncertain',reason:'固定验收响应：入口存在，业务语义仍需人工核查。',evidence:[{file:entry.location.file,line:entry.location.line,end_line:entry.location.end_line}]};
  })};
  // 固定响应由测试下载为本地文件，再通过真实文件控件选择，保持原始 JSON 导入路径。
  const responseDownload = page.waitForEvent('download');
  await page.evaluate(content => {
    const url = URL.createObjectURL(new Blob([content],{type:'application/json'}));
    const link = document.createElement('a');link.href = url;link.download = 'review-response.json';link.click();
    setTimeout(() => URL.revokeObjectURL(url),1000);
  },JSON.stringify(response));
  const responsePath = await (await responseDownload).path();
  await page.locator('#review-import:not([disabled])').waitFor();
  await page.locator('#review-import').click();
  await dialog.locator('[name=request]').setInputFiles(requestPath);
  await dialog.locator('[name=response]').setInputFiles(responsePath);
  await submit('验证并导入');
  await dialog.waitFor({state:'hidden'});
  await page.getByText('2 项判断待确认',{exact:true}).waitFor();
  assert((await readState()).confirmed.revision === baseline.confirmed.revision, '语义导入改变了设计基准');

  await page.getByRole('button',{name:'架构设计',exact:true}).click();
  await page.setViewportSize({width:390,height:844});
  await page.screenshot({path:'design-mobile.png'});
  assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), '移动端发生页面级横向溢出');
  await page.getByRole('button',{name:'移动 payment 模块'}).focus();
  await page.keyboard.press('ArrowRight');
  await page.waitForFunction(() => document.querySelector('#layout-status').textContent.includes('布局已保存'));
  await page.locator('[data-node="payment"] .node-select').click();
  await page.getByLabel('职责与边界').fill('我的未保存修改：退款允许部分退款');
  await page.locator('#save-draft:not([disabled])').waitFor();
  const external = await readState();
  external.draft.modules[0].responsibility = '其他窗口刚刚保存的新职责';
  const result = await page.evaluate(async data => {
    const result = await fetch('/api/draft',{method:'PUT',headers:{'Content-Type':'application/json','X-Workbench-Token':data.token},body:JSON.stringify({design:data.draft,expected_draft_hash:data.draft_hash,expected_revision:data.confirmed.revision})});
    return result.status;
  },external);
  assert(result === 200, '无法模拟另一个工作台保存草稿');
  await page.locator('#conflict-banner').waitFor({state:'visible',timeout:10000});
  assert((await page.getByLabel('职责与边界').inputValue()).includes('我的未保存修改'), '外部同步覆盖了正在编辑的内容');
  assert(await page.locator('#save-draft').isDisabled(), '冲突后仍允许无条件覆盖');
  await page.locator('#conflict-banner').scrollIntoViewIfNeeded();
  await page.screenshot({path:'conflict-mobile.png'});
  await page.getByRole('button',{name:'载入最新版本',exact:true}).click();
  await submit('丢弃本地编辑并载入');
  await page.waitForFunction(() => document.querySelector('[name=module-responsibility]').value.includes('其他窗口刚刚保存的新职责'));
  assert((await readState()).confirmed.revision === baseline.confirmed.revision, '外部草稿更新改变了确认基准');
  await page.getByRole('button',{name:/^代码检查/}).click();
  await page.screenshot({path:'checks-mobile.png'});
  assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), '移动端报告发生横向溢出');
  assert(errors.length === 0, `浏览器错误：${errors.join('; ')}`);
  return 'VERIFIED: CUJ-01 编辑与确认；CUJ-02 冲突保护；CUJ-03 依赖与语义报告；CUJ-04 布局持久化与响应式';
}
