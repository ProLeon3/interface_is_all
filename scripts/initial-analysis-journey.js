// 固定模型配合真实浏览器：已有项目选择、分析、证据、编辑、确认和后续检查。
async page => {
  const assert = (ok, message) => { if (!ok) throw new Error(message); };
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  const state = () => page.evaluate(async () => (await fetch('/api/state', {headers:{'X-Workbench-Project':encodeURIComponent(document.querySelector('#project-path').textContent)}})).json());
  const dialog = page.getByRole('dialog');
  // 已有草稿后的分析入口按需展开，撤回和重做仍走原后端保护。
  const openAnalysis = async () => {
    if (!await page.locator('#design-starter').isVisible()) await page.locator('#adjust-design').click();
    if (!await page.locator('#initial-analysis-entry').isVisible()) await page.locator('#mode-code').click();
  };
  await page.setViewportSize({width:1440,height:1000});
  await page.locator('#choose-project:not([disabled])').waitFor();
  const landing = await state();
  const target = landing.project.path.replace(/\/landing$/, '/existing');
  assert(await page.locator('#initial-analysis-entry').isHidden(),'非 Go 项目显示了代码分析入口');
  await page.locator('#choose-project').click();
  await dialog.getByLabel('项目路径').fill(target);
  await dialog.getByRole('button',{name:'选择此项目',exact:true}).click();
  await dialog.waitFor({state:'hidden'});
  await page.locator('#analyze-initial:not([disabled])').waitFor();
  assert((await state()).draft === null,'选择项目创建了草稿');
  const response = page.waitForResponse(r => r.url().endsWith('/api/initial-analysis') && r.request().method() === 'POST');
  await page.locator('#analyze-initial').click();
  const generated = await response;
  assert(generated.ok(),await generated.text());
  let proposal = await generated.json();
  await page.locator('#proposal-accept:not([disabled])').waitFor();
  assert((await state()).confirmed === null && (await state()).draft === null,'分析越过草稿或确认边界');
  assert(await page.locator('.collaboration-edge').count() === 1,'现有协作没有展示');
  assert(proposal.after.forbidden_dependencies.length === 0,'分析增加了禁止规则');
  await page.locator('[data-interface="request-refund"]').click();
  assert((await page.locator('#inspector').innerText()).includes('初次分析依据'),'详情没有关联源码依据');
  await page.locator('#inspector [data-source-file]').first().click();
  assert((await dialog.innerText()).includes('paymentID'),'无法查看实际源码');
  await dialog.getByRole('button',{name:'关闭',exact:true}).click();
  await page.locator('[data-analysis-section="findings"] summary').click();
  await page.locator('[data-analysis-section="scope"] > summary').click();
  assert((await page.locator('#analysis-evidence').innerText()).includes('未自动加入设计'),'建议没有单独展示');
  if (!page.url().includes('skip-captures')) {
  await page.locator('#toast').waitFor({state:'hidden'});
  await page.evaluate(() => scrollTo(0,0));
  await page.screenshot({path:'initial-desktop.png',fullPage:true});
  await page.setViewportSize({width:390,height:844});
  await page.evaluate(() => scrollTo(0,0));
  assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),'移动端页面横向溢出');
  await page.screenshot({path:'initial-mobile.png',fullPage:true});
  await page.setViewportSize({width:1440,height:1000});
  }

  // 新标签页没有旧会话提案 ID，仍从项目恢复待审阅分析。
  const tab = await page.context().newPage();
  await tab.goto(page.url());
  await tab.locator('#choose-project:not([disabled])').waitFor();
  await tab.locator('#choose-project').click();
  await tab.getByRole('dialog').getByLabel('项目路径').fill(target);
  await tab.getByRole('dialog').getByRole('button',{name:'选择此项目',exact:true}).click();
  await tab.getByRole('dialog').waitFor({state:'hidden'});
  await tab.locator('#proposal-accept:not([disabled])').waitFor();
  assert((await tab.locator('#proposal-review').innerText()).includes('还原订单'),'新标签页没有恢复初次分析');
  tab.on('dialog',d => d.accept());
  await tab.close({runBeforeUnload:false});

  await page.locator('#proposal-accept').click();
  await page.locator('#proposal-review').waitFor({state:'hidden'});
  assert((await state()).confirmed === null,'接受结果提前确认');
  // 撤回已接受来源必须保留保护；重新分析并接受后才恢复首次确认。
  await openAnalysis();
  await page.locator('#discard-initial').click();
  await dialog.getByRole('button',{name:'放弃本次分析',exact:true}).click();
  await dialog.waitFor({state:'hidden'});
  assert(await page.locator('#review-design').isDisabled(),'撤回来源后仍能确认旧草稿');
  assert((await state()).initial_analysis.withdrawn,'撤回没有保留来源保护');
  // 模拟本次加载失败的 HTTP 响应，确认旧来源不会遮住新的范围诊断。
  const failedSource = JSON.parse(JSON.stringify(proposal.request.source));
  failedSource.facts.diagnostics = [{code:'package_load',subject:'missing-package',message:'固定回归：本次包加载失败'}];
  await page.route('**/api/initial-analysis',route => route.fulfill({status:422,contentType:'application/json',body:JSON.stringify({error:'固定回归：加载不完整',code:'analysis_incomplete',source:failedSource})}),{times:1});
  await openAnalysis();
  await page.locator('#analyze-initial:not([disabled])').click();
  await page.locator('#analysis-evidence').getByRole('heading',{name:'分析未完成',exact:true}).waitFor();
  const scope = page.locator('[data-analysis-section="scope"]');
  if (!await scope.evaluate(node => node.open)) await scope.locator(':scope > summary').click();
  assert((await page.locator('#analysis-evidence').innerText()).includes('本次包加载失败'),'失败时显示了旧范围');
  await page.unroute('**/api/initial-analysis');
  await openAnalysis();
  const restarted = page.waitForResponse(r => r.url().endsWith('/api/initial-analysis') && r.request().method() === 'POST');
  await page.locator('#analyze-initial:not([disabled])').click();
  const regenerated = await restarted;
  assert(regenerated.ok(),await regenerated.text());
  proposal = await regenerated.json();
  await page.locator('#proposal-accept:not([disabled])').click();
  await page.locator('#proposal-review').waitFor({state:'hidden'});
  await page.locator('.node-select[data-module="payment"]').click();
  await page.getByLabel('职责与边界').fill('用户核对：退款示例只检查支付标识，不连接支付系统。');
  assert(await page.locator('#ai-generate').isDisabled(),'未保存编辑未阻止模型覆盖');
  await page.locator('#save-draft').click();
  await page.locator('#review-design:not([disabled])').waitFor();
  await page.reload();
  await page.locator('#review-design:not([disabled])').waitFor();
  await page.locator('#analysis-evidence').waitFor();
  assert((await state()).draft.modules.find(m => m.id === 'payment').responsibility.startsWith('用户核对'),'刷新丢失草稿修改');
  await page.locator('#review-design').click();
  assert((await dialog.innerText()).includes('不表示当前架构合理'),'首次确认缺少分析含义说明');
  await dialog.getByLabel('确认人').fill('初次分析固定响应浏览器验收');
  await dialog.getByRole('checkbox').check();
  await dialog.getByRole('button',{name:'确认此设计版本',exact:true}).click();
  await dialog.waitFor({state:'hidden'});
  // 首次确认后仍能主动进入重新分析，默认页面则回到设计图。
  await openAnalysis();
  await page.locator('#analyze-initial').waitFor();
  assert(await page.locator('#analyze-initial').innerText() === '重新分析代码','确认后没有切换到重新分析');
  await page.locator('#close-composer').click();
  await page.getByRole('button',{name:/^代码检查/}).click();
  const checked = page.waitForResponse(r => r.url().endsWith('/api/check') && r.request().method() === 'POST');
  await page.locator('#run-check:not([disabled])').click();
  const checkResponse = await checked;
  assert(checkResponse.ok(),await checkResponse.text());
  const report = await checkResponse.json();
  const current = await state();
  assert(current.confirmed.initial_analysis_id === proposal.id,'确认版本没有保留来源');
  assert(report.design_revision === current.confirmed.revision && report.semantic_status === 'not_run','检查错误使用基准或宣称语义通过');
  assert(errors.length === 0,errors.join('; '));
  const download = page.waitForEvent('download');
  await page.evaluate(({proposal,current,report}) => {
    const link = document.createElement('a');
    link.href = URL.createObjectURL(new Blob([JSON.stringify({verification:'fixed-model-browser',proposal,current,report},null,2)],{type:'application/json'}));
    link.download = 'initial-journey.json'; link.click();
  },{proposal,current,report});
  await (await download).saveAs('initial-journey.json');
  const result = 'VERIFIED: 初次分析浏览器流程、跨窗口恢复、源码证据、草稿编辑重开、明确确认与后续检查';
  await page.evaluate(value => {window.__initialJourneyResult=value;},result);
  return result;
}
