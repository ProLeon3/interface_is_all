import { $, escapeHTML as e, short, date, ago, api, toast, download, modal, field } from './ui.js';

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
    const violation = type === 'violation';
    return `<details class="finding ${violation ? 'violation' : ''}" ${violation ? 'open' : ''}><summary class="finding-head"><span class="finding-title"><strong class="finding-name">${e(item.title)}</strong>${item.verdict ? `<span class="finding-verdict" data-assessment="${e(item.assessment)}">${e(item.verdict)}</span>` : ''}<span class="finding-module">${e(item.module)}</span><span class="finding-snippet">${e(String(item.reason || '').split(/[。；\n]/)[0])}</span></span><span class="tag ${violation ? 'danger' : 'warning'}">${violation ? '确定性违规' : '待确认'}</span></summary><div class="finding-body"><p>${e(item.reason)}</p><dl><dt>设计依据</dt><dd>${e(item.basis)}</dd>${item.entryIDs?.length ? `<dt>关联实现入口</dt><dd><code>${item.entryIDs.map(e).join('\n')}</code></dd>` : ''}</dl>${item.evidence?.length ? `<div class="button-row">${item.evidence.map((location, locationIndex) => `<button type="button" class="evidence-button small" data-evidence="${index}:${locationIndex}">${e(location.file)}:${location.line}${location.end_line > location.line ? `–${location.end_line}` : ''}</button>`).join('')}</div>` : `<p class="empty-note">${item.searched?.length ? '本次搜索范围：' + item.searched.map(e).join('、') : '本次报告未提供代码位置，请结合检查范围核查。'}</p>`}<div class="finding-actions"><button type="button" class="text-button" data-locate="${index}" ${!state.server?.confirmed?.design.modules.some(module => module.id === item.module) ? 'disabled' : ''}>定位模块 ${e(item.module)}</button><button type="button" class="text-button" data-copy="${index}">复制给编程工具</button></div></div></details>`;
  };
  const violationsHTML = violations.map(item => finding({
    title:`${item.rule.from} → ${item.rule.to} · 禁止的直接依赖`,
    reason:`${item.dependency.from_package} 直接引用了 ${item.dependency.to_package}。`,
    basis:`${item.rule.id}：${item.rule.reason}`,module:item.rule.from,
    evidence:[item.dependency.location],revision:report.design_revision,
  },'violation')).join('');
  const semanticHTML = (review?.interfaces || []).map(item => finding({
    title:item.design_basis.name,verdict:assessments[item.conclusion.assessment] || item.conclusion.assessment,assessment:item.conclusion.assessment,
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
  // 状态行：每项一个语义标签 + 结论 + 时间；过期单独用「stale」语气，只在状态条说明一次，重新运行交给页首主按钮。
  const checkTone = oldCheck ? 'stale' : {passed:'ok',violations:'danger',incomplete:'warn'}[report?.deterministic.status] || 'neutral';
  const checkChip = oldCheck ? '已过期' : {passed:'通过',violations:'违规',incomplete:'不完整'}[report?.deterministic.status] || '未运行';
  const reviewTone = oldReview ? 'stale' : review ? (pending ? 'warn' : 'ok') : 'neutral';
  const reviewChip = oldReview ? '已过期' : review ? (pending ? '待确认' : '已核查') : '未运行';
  const staleBar = oldCheck || oldReview ? `<div class="stale-bar" role="status"><svg class="ui-icon" viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/></svg><span>${oldCheck && oldReview ? '检查与审查报告' : oldCheck ? '依赖检查报告' : '能力审查报告'}基于旧基准 <code>${e(short((oldCheck ? report : review).design_revision).slice(0,6))}</code>，当前基准为 <code>${e(short(revision).slice(0,6))}</code>。</span><span id="run-check-slot"></span></div>` : '';
  const cell = (title, tone, chip, headline, detail) => `<section class="status-cell"><h2>${title}</h2><div class="status-line"><span class="status-chip" data-tone="${tone}">${chip}</span>${tone === 'stale' ? `<span class="stale-headline">上次：${headline}</span>` : `<strong>${headline}</strong>`}</div><p>${detail}</p></section>`;
  const help = text => `<span class="help" tabindex="0" role="note" aria-label="${e(text)}" data-tip="${e(text)}">?</span>`;
  const empty = report?.deterministic.status === 'incomplete' ? ['','尚无法得出完整检查结论'] : oldCheck ? ['stale','上次结果：没有禁止的直接依赖'] : ['passed','没有禁止的直接依赖'];
  // 重绘前把「运行依赖检查」放回页首，避免随结果区一起被替换
  const runButton = $('#run-check');
  $('#checks-page .page-heading').append(runButton);
  $('#check-results').innerHTML = `${(state.observations.warnings || []).map(warning => `<div class="banner error" role="alert">无法读取报告：${e(warning)}</div>`).join('')}${state.dirty || state.server?.draft_hash && state.server?.draft_hash !== state.server?.confirmed?.design_hash ? '<div class="banner info">草稿有未确认的变更，检查仍使用已确认设计。</div>' : ''}${staleBar}<div class="status-strip">${cell('依赖检查',checkTone,checkChip,e(statusText),observation ? `${ago(observation.updated_at)} · 基准 <code>${e(short(report.design_revision).slice(0,6))}</code>` : '尚未运行')}${cell('能力审查',reviewTone,reviewChip,review ? `${pending} 项判断待确认` : '语义审查尚未运行',semantic ? `${ago(semantic.updated_at)} · 模型判断` : '由外部 AI 工具完成')}</div>${report ? `<section class="results-section ${oldCheck ? 'is-stale' : ''}"><div class="results-head"><h2>直接依赖</h2>${help('只检查当前构建条件下的直接 import，不覆盖运行时调用与业务正确性。')}</div>${report.deterministic.status === 'incomplete' ? '<div class="banner warning">扫描范围不完整：已列出的违规有定位依据，其余部分不能推断为通过。</div>' : ''}${violationsHTML || (oldCheck ? '' : `<div class="report-empty ${empty[0]}"><svg class="ui-icon" viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="9"/><path d="m8 12.5 2.8 2.8L16.5 9"/></svg><div><h2>${empty[1]}</h2></div></div>`)}${scopeHTML(report.facts)}</section>` : ''}<section class="results-section ${oldReview ? 'is-stale' : ''}"><div class="results-head"><h2>能力与职责</h2>${pending && !oldReview ? `<span class="results-count">${pending}</span>` : ''}${help('模型判断保留原始设计依据，需要你逐项核查；导入结果不会自动改变设计。')}</div>${semanticHTML || '<div class="report-empty"><div><h2>等待能力审查结果</h2><p>在「能力审查工具」导出请求，或由 coding agent 调用 review-request 与 review-import。</p></div></div>'}</section>`;
  if (oldCheck) $('#run-check-slot').replaceWith(runButton);
  runButton.classList.toggle('small', !!oldCheck);
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
