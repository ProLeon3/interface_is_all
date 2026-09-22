import { $, escapeHTML as e, short, date, api, toast, download, modal, field } from './ui.js';

const assessments = {aligned:'模型认为符合约定',not_found:'尚未找到对应实现',potential_deviation:'可能偏离能力约定',uncertain:'需要进一步核查'};
const concernKinds = {additional_capability:'可能新增未经设计的能力',responsibility_drift:'可能偏离模块职责'};

// 报告始终标明扫描时间和设计版本。即使版本相同，也不把历史扫描称为当前代码已通过。
export function renderReports(state, selectModule) {
  const revision = state.server?.confirmed?.revision;
  // 尚无检查基准时不展示未运行报告或旧项目内容，起步引导由页面固定区域负责。
  if (!revision) {
    $('#check-results').innerHTML = '';
    $('#issue-count').hidden = true;
    return;
  }
  const observation = state.observations.check, semantic = state.observations.review;
  const report = observation?.report, review = semantic?.report;
  const violations = report?.deterministic.violations || [];
  const pending = (review?.interfaces?.length || 0) + (review?.concerns?.length || 0);
  const currentViolations = report?.design_revision === revision ? violations.length : 0;
  const currentPending = review?.design_revision === revision ? pending : 0;
  $('#issue-count').textContent = currentViolations + currentPending;
  $('#issue-count').hidden = !currentViolations && !currentPending;
  const statusText = {passed:'本次未发现禁止依赖',violations:`发现 ${violations.length} 处禁止依赖`,incomplete:'扫描不完整'}[report?.deterministic.status] || '尚未运行';
  const oldCheck = report && report.design_revision !== revision;
  const oldReview = review && review.design_revision !== revision;
  const cards = [];
  const finding = (item, type) => {
    const index = cards.push({...item,type}) - 1;
    return `<article class="finding ${type === 'violation' ? 'violation' : ''}"><div class="finding-head"><h3>${e(item.title)}</h3><span class="tag ${type === 'violation' ? 'danger' : 'warning'}">${type === 'violation' ? '确定性违规' : '待确认'}</span></div><p>${e(item.reason)}</p><dl><dt>设计依据</dt><dd>${e(item.basis)}</dd>${item.entryIDs?.length ? `<dt>关联实现入口</dt><dd><code>${item.entryIDs.map(e).join('\n')}</code></dd>` : ''}</dl>${item.evidence?.length ? `<div class="button-row">${item.evidence.map((location, locationIndex) => `<button type="button" class="evidence-button small" data-evidence="${index}:${locationIndex}">${e(location.file)}:${location.line}${location.end_line > location.line ? `–${location.end_line}` : ''} ↗</button>`).join('')}</div>` : `<p class="empty-note">${item.searched?.length ? '本次搜索范围：' + item.searched.map(e).join('、') : '本次报告未提供代码位置，请结合检查范围核查。'}</p>`}<div class="finding-actions"><button type="button" class="text-button" data-locate="${index}" ${!state.server?.confirmed?.design.modules.some(module => module.id === item.module) ? 'disabled' : ''}>定位模块 ${e(item.module)}</button><button type="button" class="text-button" data-copy="${index}">复制给编程工具</button></div></article>`;
  };
  const violationsHTML = violations.map(item => finding({
    title:`${item.rule.from} → ${item.rule.to} · 禁止的直接依赖`,
    reason:`${item.dependency.from_package} 直接引用了 ${item.dependency.to_package}。`,
    basis:`${item.rule.id}：${item.rule.reason}`,module:item.rule.from,
    evidence:[item.dependency.location],revision:report.design_revision,
  },'violation')).join('');
  const semanticHTML = (review?.interfaces || []).map(item => finding({
    title:`${item.design_basis.name} · ${assessments[item.conclusion.assessment] || item.conclusion.assessment}`,
    reason:item.conclusion.reason,
    basis:`${item.design_basis.id}：${item.design_basis.description}${item.design_basis.semantics ? '\n' + Object.entries(item.design_basis.semantics).map(([key,value]) => `${({inputs:'输入',outputs:'输出',errors:'错误'})[key]}：${value}`).join('\n') : ''}`,
    module:item.design_basis.module_id,evidence:item.conclusion.evidence,
    entryIDs:item.conclusion.entry_ids,searched:item.searched_files,revision:review.design_revision,
  },'semantic')).join('') + (review?.concerns || []).map(item => finding({
    title:concernKinds[item.conclusion.kind] || item.conclusion.kind,
    reason:item.conclusion.reason,basis:`${item.design_basis.id}：${item.design_basis.responsibility}`,
    module:item.design_basis.id,evidence:item.conclusion.evidence,
    entryIDs:item.conclusion.entry_ids,revision:review.design_revision,
  },'semantic')).join('');
  $('#check-results').innerHTML = `${(state.observations.warnings || []).map(warning => `<div class="banner error" role="alert">无法读取报告：${e(warning)}</div>`).join('')}${state.dirty || state.server?.draft_hash && state.server?.draft_hash !== state.server?.confirmed?.design_hash ? '<div class="banner info">草稿包含未确认的变更。检查仍使用已确认设计，保存草稿不会切换检查基准。</div>' : ''}<div class="result-grid"><section class="result-summary"><h2>确定性依赖检查</h2><strong>${oldCheck ? '旧设计版本的检查' : e(statusText)}</strong><p>${observation ? `${date(observation.updated_at)} · 基准 ${short(report.design_revision)}${oldCheck ? '，请重新运行。' : ''}` : '运行后定位违反明确禁止方向的直接包引用。'}</p></section><section class="result-summary"><h2>接口能力与职责审查</h2><strong>${oldReview ? '旧设计版本的审查' : review ? `${pending} 项判断待确认` : '语义审查尚未运行'}</strong><p>${semantic ? `${date(semantic.updated_at)} · 模型判断需人工核查，包括“符合约定”。` : '导出审查请求交给外部 AI 工具，导入响应后在此核查。'}</p></section></div>${report ? `<section class="results-section"><h2>直接依赖检查</h2><span class="muted">仅检查报告中构建条件下的直接 import，不覆盖运行时调用与业务正确性。</span>${oldCheck ? '<div class="banner warning">这份报告对应旧设计，以下内容仅供回看，不能代表当前设计的检查结果。</div>' : ''}${report.deterministic.status === 'incomplete' ? '<div class="banner warning">扫描范围不完整。下面列出的违规仍有定位依据，但不能根据其余部分推断检查通过。</div>' : ''}${violationsHTML || `<div class="report-empty"><h2>${report.deterministic.status === 'incomplete' ? '尚无法得出完整检查结论' : '本次扫描未发现禁止依赖'}</h2><p>这不表示接口能力已通过审查，也不代表业务功能正确。代码修改后请再次运行。</p></div>`}${scopeHTML(report.facts)}</section>` : ''}<section class="results-section"><h2>能力与职责 · 待确认</h2><span class="muted">语义判断保留原始设计依据，由你核查；报告不会自动改变设计。</span>${oldReview ? '<div class="banner warning">这份语义报告基于旧设计，请对最新版本重新发起审查。</div>' : ''}${semanticHTML || '<div class="report-empty"><h2>等待能力审查结果</h2><p>依赖检查不会自动调用模型。可以导出自包含的审查请求，也可以由 coding agent 调用 review-request 与 review-import 命令。</p></div>'}</section>`;
  document.querySelectorAll('[data-locate]').forEach(button => {button.onclick = () => selectModule(cards[Number(button.dataset.locate)].module);});
  document.querySelectorAll('[data-copy]').forEach(button => {button.onclick = async () => {
    const item = cards[Number(button.dataset.copy)];
    const content = `请依据已确认设计核查以下架构问题，不要自动修改设计。\n设计版本：${item.revision}\n性质：${item.type === 'violation' ? '确定性违规，请修复并复查' : '语义审查待确认，请协助核查'}\n${item.title}\n设计依据：${item.basis}\n理由：${item.reason}\n代码位置：${(item.evidence || []).map(location => `${location.file}:${location.line}-${location.end_line}`).join('、') || '尚未找到'}\n`;
    try {await navigator.clipboard.writeText(content);toast('问题、依据和代码位置已复制。');}
    catch {modal('复制给编程工具', `${field('问题与依据','content',content,{area:true,readonly:true})}<div class="dialog-footer"><button type="button" data-close>关闭</button></div>`);$('textarea', $('#dialog')).select();}
  };});
  document.querySelectorAll('[data-evidence]').forEach(button => {button.onclick = () => {
    const [index, locationIndex] = button.dataset.evidence.split(':').map(Number);
    const item = cards[index], location = item.evidence[locationIndex];
    // 确定性报告直接携带源码快照；不拿另一轮扫描的源码冒充语义审查证据。
    const source = item.type === 'violation' ? report.facts.files?.find(file => file.path === location.file) : null;
    modal('代码依据', `<p><code>${e(location.file)}:${location.line}${location.end_line > location.line ? `–${location.end_line}` : ''}</code></p><div class="banner info">${source ? '以下为该次检查保存的源码快照。修改代码后，请重新检查。' : '请按此位置在项目中核查。此语义报告只提供证据位置，本页不使用其他扫描的源码代替它。'}</div>${source ? `<div class="code-view" tabindex="0" aria-label="报告源码快照">${source.content.split('\n').map((line,index) => `<div class="code-line ${index + 1 >= location.line && index + 1 <= (location.end_line || location.line) ? 'highlight' : ''}"><span class="line-number">${index + 1}</span><span>${e(line) || ' '}</span></div>`).join('')}</div>` : ''}<p>${e(item.reason)}</p><div class="dialog-footer"><button type="button" data-close>关闭</button></div>`);
    $('.code-line.highlight', $('#dialog'))?.scrollIntoView({block:'center'});
  };});
}

function scopeHTML(facts) {
  const scope = facts.scope || {};
  return `<details class="scope"><summary>本次检查范围与诊断</summary><dl><dt>构建条件</dt><dd>${e(scope.goos)} / ${e(scope.goarch)} · CGO=${e(scope.cgo_enabled)} · ${e(scope.go_version)}</dd><dt>构建标签</dt><dd>${e(scope.tags || '无显式标签')}</dd><dt>扫描范围</dt><dd>${facts.packages?.length || 0} 个包 · ${facts.files?.length || 0} 个源码文件 · 不含测试代码</dd><dt>未归属包</dt><dd>${(facts.unassigned_packages || []).map(e).join('、') || '无'}</dd><dt>暂无代码的模块</dt><dd>${(facts.empty_modules || []).map(e).join('、') || '无'}</dd></dl>${facts.diagnostics?.length ? `<strong>扫描诊断</strong><ul>${facts.diagnostics.map(item => `<li><code>${e(item.code)} / ${e(item.subject)}</code><br>${e(item.message)}</li>`).join('')}</ul>` : ''}${facts.packages?.some(pkg => pkg.excluded?.length) ? `<strong>构建范围外的文件</strong><ul>${facts.packages.filter(pkg => pkg.excluded?.length).map(pkg => `<li>${e(pkg.directory)}：${pkg.excluded.map(e).join('、')}</li>`).join('')}</ul>` : ''}</details>`;
}

export function bindReports(state, {renderTop,refresh,errorNotice}) {
  const scan = async request => {
    if (state.scanning) return;
    state.scanning = true;renderTop();
    try {
      const result = await api(request ? 'review-request' : 'check','POST',{});
      if (request) {download(result,'review-request.json');toast('审查请求已导出，其中包含设计和本次源码快照。');}
      else toast(result.deterministic.status === 'incomplete' ? '扫描不完整，请查看诊断。' : '依赖检查完成，结果已保存。');
    } catch (error) {errorNotice(error);}
    finally {state.scanning = false;renderTop();await refresh();}
  };
  $('#run-check').onclick = () => scan(false);
  $('#review-request').onclick = () => scan(true);
  $('#review-import').onclick = () => {
    modal('导入能力审查响应', '<p>选择交给外部 AI 工具的原始请求，以及它返回的响应。导入会重新扫描，验证设计与代码是否仍与请求一致。</p><label class="field"><span>原始审查请求</span><input type="file" name="request" accept=".json,application/json" required></label><label class="field"><span>审查响应</span><input type="file" name="response" accept=".json,application/json" required></label><div class="banner info">导入后所有语义判断仍为「待确认」，不会修改设计或代码。</div><div class="dialog-footer"><button type="button" data-close>取消</button><button type="submit" class="primary">验证并导入</button></div>', async (form, dialog) => {
      if (state.scanning) throw new Error('已有检查正在运行，请等待结束。');
      const request = form.get('request'), response = form.get('response');
      if (request.size + response.size > 32 * 1024 * 1024 - 64) throw new Error('两个文件总大小不能超过 32 MiB。');
      state.scanning = true;renderTop();
      try {
        // 保留文件的原始 JSON，后端仍可检测重复键和未知字段。
        await api('review-import','POST',`{"request":${await request.text()},"response":${await response.text()}}`);
        dialog.close();toast('审查结果已保存，请逐项核查判断与依据。');
      } finally {state.scanning = false;renderTop();await refresh();}
    });
  };
}
