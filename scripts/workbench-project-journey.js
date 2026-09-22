// 验证真实目录浏览和项目隔离；临时初始项目的同级目录需包含“中文 项目”。
async page => {
  const assert = (condition,message) => {if (!condition) throw new Error(message);};
  const errors = [];
  page.on('pageerror',error => errors.push(error.message));
  const readDefault = () => page.evaluate(async () => (await fetch('/api/state')).json());
  const readSelected = () => page.evaluate(async () => (await fetch('/api/state',{headers:{'X-Workbench-Project':encodeURIComponent(document.querySelector('#project-path').textContent)}})).json());
  await page.setViewportSize({width:1440,height:900});
  await page.getByRole('button',{name:'架构设计',exact:true}).click();
  await page.locator('#draft-view').click();
  await page.locator('#choose-project:not([disabled])').waitFor();
  const original = await readDefault();
  const parent = original.project.path.slice(0,original.project.path.lastIndexOf('/'));
  const otherPath = parent + '/中文 项目';
  const dialog = page.getByRole('dialog');
  const choose = () => page.getByRole('button',{name:'选择项目',exact:true}).click();
  const path = () => dialog.getByLabel('项目路径');
  const submit = () => dialog.getByRole('button',{name:'选择此项目',exact:true}).click();

  await page.locator('#add-module').click();
  await dialog.getByLabel('模块 ID').fill('switch-guard');
  await dialog.getByLabel('模块目录').fill('switch-guard');
  await dialog.getByLabel('职责与边界').fill('这是尚未保存的原项目编辑');
  await dialog.getByRole('button',{name:'添加到草稿',exact:true}).click();
  await choose();
  await dialog.getByRole('checkbox',{name:'我同意丢弃'}).waitFor();
  await path().fill(parent + '/不存在的目录');
  await dialog.getByRole('button',{name:'浏览',exact:true}).click();
  await dialog.locator('#directory-status.error').waitFor();
  assert((await readSelected()).project.path === original.project.path,'无效路径改变了当前项目');
  await dialog.getByRole('button',{name:'取消',exact:true}).click();
  assert(await page.locator('[data-node="switch-guard"]').count() === 1,'取消目录选择丢失了未保存模块');

  await choose();
  await dialog.locator('#directory-parent:not([disabled])').waitFor();
  await dialog.getByRole('button',{name:'↑ 上一级',exact:true}).click();
  await dialog.locator('[data-directory]').filter({hasText:'中文 项目'}).waitFor();
  await page.screenshot({path:'project-picker-desktop.png'});
  await dialog.locator('[data-directory]').filter({hasText:'中文 项目'}).click();
  await page.waitForFunction(expected => document.querySelector('[name=project-path]').value === expected,otherPath);
  await dialog.locator('button[type=submit]:not([disabled])').waitFor();
  await submit();
  assert((await readSelected()).project.path === original.project.path,'没有同意丢弃编辑就切换了项目');
  await dialog.getByRole('checkbox',{name:'我同意丢弃'}).check();
  await submit();
  await dialog.waitFor({state:'hidden'});
  assert((await readSelected()).project.path === otherPath,'没有进入所选中文路径');
  assert((await readDefault()).draft_hash === original.draft_hash,'切换把原项目的未保存编辑写入了磁盘');
  assert(await page.locator('[data-node="switch-guard"]').count() === 0,'原项目编辑出现在新项目中');

  await page.locator('#add-module').click();
  await dialog.getByLabel('模块 ID').fill('only-b');
  await dialog.getByLabel('模块目录').fill('only-b');
  await dialog.getByLabel('职责与边界').fill('仅属于第二个项目');
  await dialog.getByRole('button',{name:'添加到草稿',exact:true}).click();
  await page.locator('#save-draft').click();
  await page.locator('#review-design:not([disabled])').waitFor();
  assert((await readSelected()).draft.modules[0].id === 'only-b','草稿没有保存到所选项目');
  assert((await readDefault()).draft_hash === original.draft_hash,'新项目保存覆盖了初始项目');
  await page.locator('#review-design').click();
  await dialog.getByLabel('确认人').fill('项目选择验收用户');
  await dialog.getByRole('checkbox').check();
  await dialog.getByRole('button',{name:'确认此设计版本',exact:true}).click();
  await dialog.waitFor({state:'hidden'});
  await page.getByRole('button',{name:/^代码检查/}).click();
  await page.locator('#run-check:not([disabled])').waitFor();
  await page.locator('#run-check').click();
  await page.locator('.result-summary').getByText('本次未发现禁止依赖',{exact:true}).waitFor();
  const report = await page.evaluate(async () => (await fetch('/api/observations',{headers:{'X-Workbench-Project':encodeURIComponent(document.querySelector('#project-path').textContent)}})).json());
  assert(report.check.report.design_revision === (await readSelected()).confirmed.revision,'检查没有使用所选项目的版本');
  assert((await readDefault()).confirmed?.revision === original.confirmed?.revision,'确认新项目改变了初始项目');
  await page.reload();
  await page.waitForFunction(expected => document.querySelector('#project-path').textContent === expected,otherPath);
  assert(await page.locator('[data-node="only-b"]').count() === 1,'刷新没有恢复所选项目与设计');

  // 新标签页保持启动项目，当前标签页的选择不能替别的窗口改项目。
  const otherTab = await page.context().newPage();
  await otherTab.goto(page.url());
  await otherTab.waitForFunction(expected => document.querySelector('#project-path').textContent === expected,original.project.path);
  assert(await otherTab.locator('[data-node="only-b"]').count() === 0,'其他标签页混入了新项目设计');
  await otherTab.close();

  await page.setViewportSize({width:390,height:844});
  await choose();
  await dialog.locator('#directory-list[aria-busy]').waitFor({state:'hidden'});
  await page.screenshot({path:'project-picker-mobile.png'});
  assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),'目录选择窗口导致页面横向溢出');
  await path().fill(original.project.path);
  await submit();
  await dialog.waitFor({state:'hidden'});
  assert((await readSelected()).project.path === original.project.path,'不能用输入路径切回初始项目');
  await page.screenshot({path:'project-selector-mobile.png'});

  // 目录读取尚未结束时取消，不得稍后在后台突然切换项目。
  let release, captured;
  const gate = new Promise(resolve => {release = resolve;});
  const ready = new Promise(resolve => {captured = resolve;});
  await page.route('**/api/projects/open',async route => {
    const response = await route.fetch();captured();await gate;await route.fulfill({response});
  },{times:1});
  await choose();await path().fill(otherPath);await submit();await ready;
  assert(await path().isDisabled(),'切换期间仍能更改目标路径');
  assert(await dialog.locator('button[type=submit]').isDisabled(),'切换期间仍能重复提交');
  await dialog.getByRole('button',{name:'取消',exact:true}).click();
  release();
  await page.locator('#choose-project:not([disabled])').waitFor();
  assert((await readSelected()).project.path === original.project.path,'取消后仍切换到了新项目');
  assert(errors.length === 0,`浏览器异常：${errors.join('; ')}`);
  return 'VERIFIED: CUJ-05 路径输入、目录浏览、中文路径、取消、未保存编辑、项目隔离、刷新恢复和窄屏入口';
}
