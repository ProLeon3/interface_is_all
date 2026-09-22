import { $, escapeHTML as e, modal, download } from './ui.js';

// 依据属于原始分析；手动修改后保留原文，不能冒充新设计已被模型或程序核实。
// 查看旧基准时必须使用它自己的来源，不能误展示正在审阅的新源码依据。
export const analysisProposal = state => state.view === 'confirmed' ? state.confirmedAnalysisProposal : state.proposal?.analysis ? state.proposal : state.analysisProposal || state.confirmedAnalysisProposal;
const analysisTitle = p => p?.expected_revision ? '重新分析依据' : '初次分析依据';
const kindLabel = {module:'模块',interface:'接口',collaboration:'协作'};
const locationsHTML = locations => locations.map(loc => `<button type="button" class="text-button small source-location" data-source-file="${e(loc.file)}" data-source-line="${loc.line}" data-source-end="${loc.end_line}">${e(loc.file)}:${loc.line}–${loc.end_line}</button>`).join('');
const evidenceHTML = item => `<div class="analysis-entry"><strong>${kindLabel[item.kind]} · ${e(item.id)}</strong> <span class="tag ${item.status === 'uncertain' ? 'warning' : 'neutral'}">${item.status === 'uncertain' ? '不确定' : '有源码依据 · 待核对'}</span><p>${e(item.explanation)}</p>${locationsHTML(item.locations)}</div>`;

export function objectEvidence(state, kind, id) {
  const item = analysisProposal(state)?.analysis.evidence.find(item => item.kind === kind && item.id === id);
  return item ? `<section class="inspector-section"><h3>${analysisTitle(analysisProposal(state))}</h3>${evidenceHTML(item)}<p class="muted">对应分析时的源码与原提案；后续编辑不改写这份依据。</p></section>` : '';
}

function coverageHTML(context) {
  const f = context.facts, scope = f.scope;
  return `<p>${f.packages.length} 个包 · ${f.files.length} 个已读文件 · ${e(scope.goos)} / ${e(scope.goarch)} · CGO ${e(scope.cgo_enabled)} · ${e(scope.go_version)} · 构建标签：${e(scope.tags || '无')}</p><p class="muted">只分析根目录的单个 Go module；不包含测试、其他构建条件、嵌套 module、vendor、隐藏目录、下划线目录和 testdata。</p><p class="analysis-fingerprint">源码指纹 <code>${e(context.fingerprint)}</code></p><details><summary>实际读取与排除的文件</summary><ul>${f.packages.map(p => `<li><strong>${e(p.import_path)}</strong><p>已读：${f.files.filter(file => file.package_path === p.import_path).map(file => e(file.path)).join('、') || '无'}</p><p>本包排除：${(p.excluded || []).map(e).join('、') || '无'}</p></li>`).join('')}</ul></details>${f.diagnostics.length ? `<h3>未完成的读取</h3><ul>${f.diagnostics.map(d => `<li>${e(d.subject)} · ${e(d.message)}</li>`).join('')}</ul>` : ''}<details><summary>观察到的包依赖（${f.dependencies.length}）</summary><p>包导入只证明依赖，不单独证明具体接口调用。</p><ul>${f.dependencies.map(d => `<li>${e(d.from_package)} → ${e(d.to_package)} ${locationsHTML([d.location])}</li>`).join('') || '<li>本次没有观察到跨包依赖。</li>'}</ul></details>`;
}

export function renderAnalysis(state) {
  // 重做失败时展示本次失败的实际范围，不能被保留的旧分析证据遮住。
  const failure = state.view === 'confirmed' ? null : state.analysisFailure;
  const p = failure ? null : analysisProposal(state), context = failure || p?.request.source;
  const panel = $('#analysis-evidence');
  panel.hidden = !context;
  if (!context) { panel.innerHTML = ''; delete panel.dataset.rendered; return; }
  const findings = (label, items) => `<h3>${label}</h3>${items.length ? items.map(f => `<div class="analysis-entry"><p>${e(f.description)}</p>${locationsHTML(f.locations)}</div>`).join('') : '<p class="muted">模型未列出条目，请结合源码自行核对。</p>'}`;
  const html = `<div class="analysis-heading"><h2>${p ? analysisTitle(p) : '分析未完成'}</h2><button type="button" id="analysis-download">下载分析记录</button></div><p>${p ? '现状设计、架构问题和改进建议分开记录。源码位置已校验，语义结论仍需审阅。' : '以下为实际读取范围。本次未生成设计，也未改变草稿或基准。'}</p>${p && state.view !== 'confirmed' && state.server?.initial_analysis?.accepted && !state.server.initial_analysis.confirmed ? '<p class="muted">草稿可以继续编辑；下方保留分析时的原始依据，确认前会复核源码是否变化。</p>' : ''}<details data-analysis-section="scope"><summary>分析范围与源码指纹</summary>${coverageHTML(context)}</details>${p ? `<details data-analysis-section="evidence"><summary>模块、接口与协作依据（${p.analysis.evidence.length}）</summary>${p.analysis.evidence.map(evidenceHTML).join('')}</details><details data-analysis-section="findings"><summary>架构问题（${p.analysis.issues.length}） · 改进建议（${p.analysis.suggestions.length}） · 不确定项（${p.analysis.uncertainties.length}）</summary>${findings('架构问题',p.analysis.issues)}${findings('改进建议 · 未自动加入设计',p.analysis.suggestions)}<h3>不确定项</h3><ul>${p.analysis.uncertainties.map(u => `<li>${e(u)}</li>`).join('') || '<li>模型未另列不确定项；不代表不存在遗漏。</li>'}</ul></details>` : ''}`;
  // 轮询及输入时不重建同一份证据，保留展开位置和键盘焦点。
  if (panel.dataset.rendered === html) return;
  const opened = [...panel.querySelectorAll('[data-analysis-section][open]')].map(node => node.dataset.analysisSection);
  panel.innerHTML = html; panel.dataset.rendered = html;
  for (const key of opened) panel.querySelector(`[data-analysis-section="${key}"]`)?.setAttribute('open','');
  $('#analysis-download').onclick = () => download(p || context, p?.expected_revision ? 'reanalysis.json' : 'initial-analysis.json');
}

// 查看的是提案保存的原始文件，不在后台读取任意本机路径。
export function bindSourceEvidence(state) {
  document.addEventListener('click', event => {
    const button = event.target.closest('[data-source-file]');
    if (!button) return;
    const failure = state.view === 'confirmed' ? null : state.analysisFailure;
    const context = button.closest('#analysis-evidence') && failure ? failure : analysisProposal(state)?.request.source || failure;
    const file = context?.facts.files.find(file => file.path === button.dataset.sourceFile);
    if (!file) return;
    const start = Number(button.dataset.sourceLine), end = Number(button.dataset.sourceEnd);
    const lines = file.content.split('\n');
    const excerpt = lines.slice(Math.max(0,start-3),Math.min(lines.length,end+2)).map((text,index) => `${Math.max(1,start-2)+index}  ${text}`).join('\n');
    modal(`${file.path}:${start}–${end}`, `<p>分析时的源码快照，供核对上下文。</p><pre class="analysis-source" tabindex="0">${e(excerpt)}</pre><div class="dialog-footer"><button type="button" data-close>关闭</button></div>`);
  });
}
