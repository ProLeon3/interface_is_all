// 接在核心旅程之后运行，专门覆盖延迟响应、临时写锁和主动重载的并发窗口。
async page => {
  const assert = (condition, message) => { if (!condition) throw new Error(message); };
  const readState = () => page.evaluate(async () => (await fetch('/api/state')).json());
  await page.setViewportSize({width:1440,height:900});
  await page.getByRole('button',{name:'架构设计',exact:true}).click();
  await page.locator('#draft-view').click();
  await page.locator('[data-node="payment"] .node-select').click();
  const original = await readState();
  const responsibility = page.getByLabel('职责与边界');
  const originalText = await responsibility.inputValue();
  const save = async text => {
    await responsibility.fill(text);
    const saved = page.waitForResponse(response => response.url().endsWith('/api/draft') && response.request().method() === 'PUT');
    await page.locator('#save-draft').click();
    assert((await saved).status() === 200,'保存没有成功');
    // 按钮退出可保存状态才表示页面应用了响应，不依赖状态文案的具体措辞。
    await page.waitForFunction(() => document.querySelector('#save-draft').disabled);
  };
  const holdNextPoll = async () => {
    let release, captured;
    const gate = new Promise(resolve => {release = resolve;});
    const ready = new Promise(resolve => {captured = resolve;});
    await page.route('**/api/state',async route => {
      const response = await route.fetch();
      captured();await gate;
      await route.fulfill({response});
    },{times:1});
    await ready;
    return async () => {
      const received = page.waitForResponse(response => response.url().endsWith('/api/state'));
      release();await received;
      // 等待响应处理后的渲染帧，不靠固定睡眠猜测界面已更新。
      await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
    };
  };

  const releaseOld = await holdNextPoll();
  try {await save('迟到的轮询响应不能覆盖这份已保存设计');}
  finally {await releaseOld();}
  assert(await responsibility.inputValue() === '迟到的轮询响应不能覆盖这份已保存设计','旧轮询把已保存内容回退了');

  // 模拟一次真实 HTTP 423 返回，再用未变更的原版本正常重试。
  await page.route('**/api/draft',route => route.fulfill({status:423,contentType:'application/json',body:JSON.stringify({error:'设计存储正被写入，请稍后重试',code:'store_locked'})}),{times:1});
  await responsibility.fill('临时写锁后仍可直接重试这份编辑');
  await page.locator('#save-draft').click();
  await page.getByText('设计存储正被写入，请稍后重试',{exact:true}).waitFor();
  assert(await page.locator('#conflict-banner').isHidden(),'临时写锁被误认为版本冲突');
  await page.locator('#save-draft:not([disabled])').waitFor();
  await save('临时写锁后仍可直接重试这份编辑');

  // 切到确认视图后，冲突备份仍必须下载未保存草稿，而不是下载旧基准。
  await responsibility.fill('需要备份的未保存草稿');
  const external = await readState();
  external.draft.modules.find(module => module.id === 'payment').responsibility = '外部更新后的职责';
  const externalStatus = await page.evaluate(async data => (await fetch('/api/draft',{method:'PUT',headers:{'Content-Type':'application/json','X-Workbench-Token':data.token},body:JSON.stringify({design:data.draft,expected_draft_hash:data.draft_hash,expected_revision:data.confirmed.revision})})).status,external);
  assert(externalStatus === 200,'外部草稿更新失败');
  await page.locator('#conflict-banner').waitFor({state:'visible',timeout:10000});
  await page.locator('#confirmed-view').click();
  const backupEvent = page.waitForEvent('download');
  await page.getByRole('button',{name:'下载我的草稿',exact:true}).click();
  const backup = await backupEvent;
  const stream = await backup.createReadStream();
  stream.setEncoding('utf8');
  let content = '';
  for await (const chunk of stream) content += chunk;
  assert(JSON.parse(content).modules.find(module => module.id === 'payment').responsibility === '需要备份的未保存草稿','冲突备份下载了错误的设计版本');
  await page.locator('#draft-view').click();

  const releaseReload = await holdNextPoll();
  try {
    await page.getByRole('button',{name:'载入最新版本',exact:true}).click();
    await page.getByRole('dialog').getByRole('button',{name:'丢弃本地编辑并载入',exact:true}).click();
    assert(await page.getByRole('dialog').isVisible(),'主动重载在轮询忙时被跳过');
  } finally {await releaseReload();}
  await page.getByRole('dialog').waitFor({state:'hidden'});
  assert(await responsibility.inputValue() === '外部更新后的职责','主动重载没有载入最新草稿');
  await save(originalText);

  // constructor 是合法稳定 ID，不能因为 JavaScript 原型继承导致节点坐标失效。
  await page.locator('#add-module').click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('模块 ID').fill('constructor');
  await dialog.getByLabel('模块目录').fill('constructor');
  await dialog.getByLabel('职责与边界').fill('合法 ID 的浏览器边界测试');
  await dialog.getByRole('button',{name:'添加到草稿',exact:true}).click();
  const left = await page.locator('[data-node="constructor"]').evaluate(node => node.style.left);
  assert(left !== '' && !left.includes('NaN'),'合法 ID 无法正确绘制');
  await page.getByRole('button',{name:'删除此模块',exact:true}).click();
  await dialog.getByRole('button',{name:'确认删除',exact:true}).click();
  assert((await readState()).confirmed.revision === original.confirmed.revision,'边界操作改变了确认基准');
  return 'VERIFIED: 延迟轮询、临时写锁重试、草稿备份、轮询期间主动重载和合法 ID 均通过';
}
