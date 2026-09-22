// 页面内固定替身扮演 coding agent，配合真实浏览器验证导出请求、导入提案到确认交接的完整生命周期，不证明模型质量。
async page => {
  const assert = (ok,message) => {if (!ok) throw new Error(message);};
  const errors = [];
  page.on('pageerror',e => errors.push(e.message));
  const state = () => page.evaluate(async () => (await fetch('/api/state')).json());
  // 用户先选择调整任务，再填写意图；验收也沿用真实展开入口。
  const openAdjustment = async () => {
    if (!await page.locator('#design-starter').isVisible()) await page.locator('#adjust-design').click();
    if (!await page.locator('#ai-composer').isVisible()) await page.locator('#mode-requirement').click();
  };
  // 导出请求 → 固定替身生成响应 → 经「设计工具」的真实文件控件导入，复现与 coding agent 的文件交换。
  const exchange = async button => {
    const exported = page.waitForResponse(r => r.url().endsWith('/api/design-request') && r.request().method() === 'POST');
    const requestDownload = page.waitForEvent('download');
    await page.locator(button).click();
    const result = await exported;
    assert(result.ok(),`导出请求失败：${await result.text()}`);
    const request = await result.json();
    assert((await requestDownload).suggestedFilename().endsWith('request.json'),'请求文件未下载');
    const responseDownload = page.waitForEvent('download');
    await page.evaluate(request => {
      const url = URL.createObjectURL(new Blob([JSON.stringify(window.__fixedAgent(request))],{type:'application/json'}));
      const link = document.createElement('a'); link.href = url; link.download = 'agent-response.json'; link.click();
      setTimeout(() => URL.revokeObjectURL(url),1000);
    },request);
    const responsePath = await (await responseDownload).path();
    // 「导入 agent 提案」按钮的禁用状态决定能否导入；playwright-cli 会把原生文件选择框当作模态状态拦截，
    // 因此直接向它打开的隐藏文件控件设置文件，仍经过同一个 change 处理器和服务端校验。
    assert(!await page.locator('#proposal-import').isDisabled(),'导入 agent 提案入口不可用');
    const imported = page.waitForResponse(r => r.url().endsWith('/api/proposal-import') && r.request().method() === 'POST');
    await page.locator('#proposal-file').setInputFiles(responsePath);
    const importResult = await imported;
    assert(importResult.ok(),`导入提案失败：${await importResult.text()}`);
    await page.locator('#proposal-accept:not([disabled])').waitFor();
    return importResult.json();
  };
  const generate = () => exchange('#ai-generate');
  await page.setViewportSize({width:1440,height:1000});
  await page.locator('#ai-generate:not([disabled])').waitFor();
  await page.locator('#ai-requirement').fill('订单取消：已付款订单先申请退款，未付款订单直接取消。');
  const initial = await generate();
  assert(initial.request_id && (await state()).draft === null,'导入提案提前保存了草稿或缺少请求绑定');
  assert(await page.locator('.collaboration-edge').count() === 2,'协作线未展示');
  assert(await page.locator('.forbidden-edge').count() === 0,'禁止规则混进默认协作图');
  await page.locator('#show-forbidden').check();
  assert(await page.locator('.forbidden-edge').count() === 1,'无法单独查看禁止规则');
  await page.locator('#show-forbidden').uncheck();
  await page.locator('[data-interface="request-refund"]').click();
  assert(await page.evaluate(() => document.activeElement.dataset.interface === 'request-refund'),'接口选择后焦点丢失');
  assert((await page.locator('#inspector').innerText()).includes('支付标识和退款金额'),'接口详情缺失');
  await page.locator('#proposal-accept').click();
  await page.locator('#proposal-review').waitFor({state:'hidden'});
  const saved = await state();
  assert(saved.draft.collaborations.length === 2 && saved.confirmed === null,'接受提案改变了确认边界');

  await page.locator('.node-select[data-module="payment"]').click();
  await openAdjustment();
  await page.locator('#ai-instruction').fill('把退款拆成独立模块，支付只负责收款。');
  const split = await generate();
  assert(split.request.selection.kind === 'module' && split.request.selection.id === 'payment','未携带真实选择对象');
  assert(await page.locator('[data-node="refund"].change-added').count() === 1,'新增模块没有突出显示');
  assert(await page.locator('[data-node="refund"] [data-interface="request-refund"].change-modified').count() === 1,'接口迁移没有突出显示');
  assert(await page.locator('[data-collaboration="order-refund"].change-modified').count() === 1,'接口迁移未改变协作终点');
  assert((await state()).draft_hash === saved.draft_hash,'预览覆盖了已保存草稿');
  await page.locator('#proposal-before').click();
  assert(await page.evaluate(() => document.activeElement.id === 'proposal-before'),'前后对照丢失键盘焦点');
  assert(await page.locator('[data-node="refund"]').count() === 0,'修改前视图显示了新增模块');
  await page.locator('#proposal-after').click();
  await page.locator('[data-interface="request-refund"]').click();
  await openAdjustment();
  await page.locator('#ai-instruction').fill('细化退款错误语义，说明重复请求行为。');
  const refined = await generate();
  assert(refined.parent_id === split.id && refined.before.modules.length === 2,'继续调整丢失原草稿对照');
  await openAdjustment();
  await page.locator('#ai-whole').click();
  await page.locator('#ai-instruction').fill('本轮不需要收款，删除支付模块及它的接口和协作。');
  const removal = await generate();
  assert(removal.after.modules.length === 2,'删除提案不完整');
  assert(await page.locator('[data-node="payment"].change-removed').count() === 1,'删除模块未保留轮廓');
  assert(await page.locator('[data-collaboration="order-charge"].change-removed').count() === 1,'删除协作未保留轮廓');
  await page.locator('[data-collaboration="order-refund"]').focus();
  await page.keyboard.press('Enter');
  assert(await page.evaluate(() => document.activeElement.dataset.collaboration === 'order-refund'),'协作线选择后焦点丢失');
  assert((await page.locator('#inspector').innerText()).includes('协作详情'),'键盘不能选择协作线');
  await page.locator('#toast').waitFor({state:'hidden'});
  await page.evaluate(() => scrollTo(0,0));
  await page.screenshot({path:'ai-desktop.png',fullPage:true});
  await page.setViewportSize({width:390,height:844});
  await page.locator('#toast').waitFor({state:'hidden'});
  await page.evaluate(() => scrollTo(0,0));
  assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),'手机页面存在横向溢出');
  await page.screenshot({path:'ai-mobile.png',fullPage:true});
  await page.setViewportSize({width:1440,height:1000});
  await page.locator('#proposal-accept').click();
  await page.locator('#review-design:not([disabled])').waitFor();
  await page.locator('#review-design').click();
  const dialog = page.getByRole('dialog');
  assert((await dialog.innerText()).includes('order → refund · 申请退款'),'最终确认未包含接口协作');
  await dialog.getByLabel('确认人').fill('固定响应浏览器回归');
  await dialog.getByRole('checkbox').check();
  await dialog.getByRole('button',{name:'确认此设计版本',exact:true}).click();
  await dialog.waitFor({state:'hidden'});
  await page.locator('#export-handoff:not([disabled])').waitFor();
  const download = page.waitForEvent('download');
  await page.locator('#export-handoff').click();
  await (await download).saveAs('confirmed-handoff.json');
  const confirmed = await state();
  assert(confirmed.confirmed.design.interfaces.find(a => a.id === 'request-refund').module_id === 'refund','确认丢失接口归属');

  // 未保存编辑会阻止导出与导入；外部更新后不能接受旧提案。
  await page.locator('.node-select[data-module="order"]').click();
  await page.getByLabel('职责与边界').fill('尚未保存的本地职责');
  assert(await page.locator('#ai-generate').isDisabled() && await page.locator('#proposal-import').isDisabled(),'未保存编辑未受保护');
  await page.locator('#save-draft').click();
  await openAdjustment();
  await page.locator('#ai-generate:not([disabled])').waitFor();
  await page.locator('#ai-instruction').fill('细化错误语义');
  await generate();
  const remote = await state();
  remote.draft.modules[0].responsibility = '另一个窗口的新职责';
  await page.evaluate(async remote => {
    const r = await fetch('/api/draft',{method:'PUT',headers:{'Content-Type':'application/json','X-Workbench-Token':remote.token},body:JSON.stringify({design:remote.draft,expected_draft_hash:remote.draft_hash,expected_revision:remote.confirmed.revision})});
    if (!r.ok) throw new Error(await r.text());
  },remote);
  // 直接点击或轮询先发现冲突，两条路径均必须阻止覆盖。
  if (!(await page.locator('#proposal-accept').isDisabled())) await page.locator('#proposal-accept').click();
  await page.locator('#conflict-banner').waitFor();
  assert((await state()).draft.modules[0].responsibility === '另一个窗口的新职责','旧提案覆盖外部更新');
  assert((await state()).confirmed.revision === confirmed.confirmed.revision,'草稿更新改变了确认快照');
  assert(errors.length === 0,`浏览器错误：${errors.join('; ')}`);
  const result = `VERIFIED: AI 固定响应回归；导出请求、导入提案、选中修改、继续调整、图形差异、刷新恢复、确认交接、并发保护。初始提案 ${initial.id}`;
  await page.evaluate(message => {window.__aiJourneyResult = message;},result);
  return result;
}
