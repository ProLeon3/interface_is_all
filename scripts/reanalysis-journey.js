// 接续首次分析的隔离项目，用固定模型与真实浏览器验证已有基准的更新流程。
async page => {
  const assert = (ok, message) => { if (!ok) throw new Error(message); };
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  const state = () => page.evaluate(async () => (await fetch('/api/state', {headers:{'X-Workbench-Project':encodeURIComponent(document.querySelector('#project-path').textContent)}})).json());
  const dialog = page.getByRole('dialog');
  const baseline = (await state()).confirmed;
  const target = (await state()).project.path;
  assert(!!baseline,'回归前置项目没有确认基准');
  await page.getByRole('button',{name:'架构设计',exact:true}).click();
  await page.locator('#draft-view').click();
  // 重新分析是主动展开的任务，不能靠强制点击隐藏按钮绕过界面。
  const openAnalysis = async () => {
    if (!await page.locator('#design-starter').isVisible()) await page.locator('#adjust-design').click();
    if (!await page.locator('#initial-analysis-entry').isVisible()) await page.locator('#mode-code').click();
  };
  const generate = async () => {
    await openAnalysis();
    const response = page.waitForResponse(r => r.url().endsWith('/api/reanalysis') && r.request().method() === 'POST');
    await page.locator('#analyze-initial').click();
    const generated = await response;
    assert(generated.ok(),await generated.text());
    await page.locator('#proposal-accept:not([disabled])').waitFor();
    return generated.json();
  };
  const accept = async () => {
    await page.locator('#proposal-accept:not([disabled])').click();
    await page.locator('#proposal-review').waitFor({state:'hidden'});
  };
  const discard = async () => {
    // 待审阅时使用提案工具；接受后的来源管理仍从代码分析入口进入。
    if (await page.locator('#proposal-review').isVisible()) {
      await page.locator('#proposal-more > summary').click();
      await page.locator('#proposal-discard').click();
    } else {
      await openAnalysis();
      await page.locator('#discard-initial').click();
    }
    await dialog.getByRole('button',{name:'放弃本次分析',exact:true}).click();
    await dialog.waitFor({state:'hidden'});
  };
  const confirm = async () => {
    await page.locator('#review-design:not([disabled])').click();
    assert((await dialog.innerText()).includes('确认前将复核源码'),'重新确认缺少源码复核说明');
    await dialog.getByLabel('确认人').fill('重新分析固定响应浏览器验收');
    await dialog.getByRole('checkbox').check();
    await dialog.getByRole('button',{name:'确认此设计版本',exact:true}).click();
    await dialog.waitFor({state:'hidden'});
    await page.locator('#confirmed-view:not([disabled])').waitFor();
    return (await state()).confirmed;
  };
  let proposal = await generate();
  assert(proposal.baseline.revision === baseline.revision,'提案没有保存生成时的基准');
  assert((await state()).confirmed.revision === baseline.revision,'生成提前替换了基准');
  const baselineDiff = page.locator('.proposal-baseline-diff');
  await baselineDiff.locator('summary').click();
  assert((await baselineDiff.innerText()).includes('旧禁止规则'),'差异中没有说明设计约束的处理');

  // 旧基准视图不能混用本次重新分析的证据。
  await page.locator('#confirmed-view').click();
  await page.locator('#analysis-evidence').getByRole('heading',{name:'初次分析依据',exact:true}).waitFor();
  assert(await page.locator('#proposal-review').isHidden(),'基准视图混入待审阅提案');
  await page.locator('#draft-view').click();
  await page.locator('#analysis-evidence').getByRole('heading',{name:'重新分析依据',exact:true}).waitFor();

  // 新窗口从磁盘恢复提案，不依赖发起窗口的缓存。
  const tab = await page.context().newPage();
  await tab.goto(page.url());
  await tab.locator('#choose-project:not([disabled])').click();
  await tab.getByRole('dialog').getByLabel('项目路径').fill(target);
  await tab.getByRole('dialog').getByRole('button',{name:'选择此项目',exact:true}).click();
  await tab.locator('#proposal-accept:not([disabled])').waitFor();
  assert((await tab.locator('.proposal-baseline-diff').textContent()).includes(baseline.revision),'新窗口丢失基准对照');
  await tab.close({runBeforeUnload:false});

  if (!page.url().includes('skip-captures')) {
    await page.locator('#toast').waitFor({state:'hidden'});
    await page.setViewportSize({width:1440,height:1000});
    await page.evaluate(() => scrollTo(0,0));
    assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),'桌面页面横向溢出');
    await page.screenshot({path:'reanalysis-desktop.png',fullPage:true});
    await page.setViewportSize({width:390,height:844});
    await page.evaluate(() => scrollTo(0,0));
    assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),'移动端页面横向溢出');
    await page.screenshot({path:'reanalysis-mobile.png',fullPage:true});
    await page.setViewportSize({width:1440,height:1000});
  }
  // 待审阅时仍可运行旧基准检查。
  await page.getByRole('button',{name:/^代码检查/}).click();
  const oldCheck = page.waitForResponse(r => r.url().endsWith('/api/check') && r.request().method() === 'POST');
  await page.locator('#run-check:not([disabled])').click();
  assert((await (await oldCheck).json()).design_revision === baseline.revision,'审阅期间检查用了新提案');
  await page.getByRole('button',{name:'架构设计',exact:true}).click();
  await accept();
  assert((await state()).confirmed.revision === baseline.revision,'接受提前替换了基准');
  await discard();
  assert(await page.locator('#review-design').isDisabled(),'撤回已接受分析后仍能确认');
  await generate();
  await discard();
  assert((await state()).initial_analysis.withdrawn,'撤回替代提案丢失原草稿保护');
  await page.reload();
  await page.locator('#adjust-design').waitFor();
  assert(await page.locator('#proposal-review').isHidden(),'刷新恢复了已撤回提案');
  assert(await page.locator('#review-design').isDisabled(),'刷新绕过来源保护');
  proposal = await generate();
  await accept();
  const current = await confirm();
  assert(current.parent_revision === baseline.revision && current.source_analysis_id === proposal.id && current.initial_analysis_id === baseline.initial_analysis_id,'新确认丢失历史链或来源');

  // 相同设计也要允许确认一次新的源码依据，而不是禁用确认按钮。
  const same = await generate();
  await accept();
  const sameState = await state();
  assert(sameState.draft_hash === current.design_hash,'固定模型没有产生预期的相同设计');
  const latest = await confirm();
  assert(latest.revision !== current.revision && latest.design_hash === current.design_hash && latest.source_analysis_id === same.id,'相同设计没有建立新证据版本');
  await page.getByRole('button',{name:/^代码检查/}).click();
  const checked = page.waitForResponse(r => r.url().endsWith('/api/check') && r.request().method() === 'POST');
  await page.locator('#run-check:not([disabled])').click();
  const report = await (await checked).json();
  assert(report.design_revision === latest.revision && report.semantic_status === 'not_run','后续检查没有使用新基准');
  assert(errors.length === 0,errors.join('; '));
  const {token, ...safeState} = await state();
  const download = page.waitForEvent('download');
  await page.evaluate(data => {
    const link = document.createElement('a');
    link.href = URL.createObjectURL(new Blob([JSON.stringify(data,null,2)],{type:'application/json'}));
    link.download = 'reanalysis-journey.json'; link.click();
  },{verification:'fixed-model-browser',baseline,proposal,current,latest,state:safeState,report});
  await (await download).saveAs('reanalysis-journey.json');
  const result = 'VERIFIED: 已有基准重新分析、基准对照、旧证据隔离、跨窗口恢复、撤回保护、相同设计确认及新基准检查';
  await page.evaluate(value => {window.__reanalysisJourneyResult=value;},result);
  return result;
}
