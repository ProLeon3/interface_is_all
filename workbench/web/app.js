import { $, escapeHTML as e, clone, short, date, api, setToken, setProjectPath, toast, download, modal, confirmAction, field } from './ui.js';
import { renderReports, bindReports } from './reports.js';
import { projectPicker } from './project-picker.js';
import { initAI } from './ai-design.js';
import { objectEvidence, bindSourceEvidence } from './initial-analysis.js';
import { graphDesign, renderNodes, renderEdges } from './design-graph.js';

// 本地编辑、磁盘草稿和确认快照各自保存，轮询绝不覆盖尚未保存的编辑。
const emptyDesign = () => ({schema_version:1, modules:[], interfaces:[], forbidden_dependencies:[]});
const state = {
  server:null, draft:emptyDesign(), base:emptyDesign(), dirty:false, conflict:false,
  selected:null, view:'draft', page:'design', busy:false, scanning:false, online:false, switching:false,
  layout:{nodes:{}}, layoutSaving:false, dragging:false, observations:{check:null, review:null, warnings:[]},
  selection:{kind:'module',id:''}, proposal:null, proposalOriginal:false, aiBusy:false,
  analysisProposal:null, confirmedAnalysisProposal:null, analysisFailure:null,
};
let polling = null;
let mutationVersion = 0;
let layoutVersion = 0;
let layoutTimer;
let pendingNodes = {};
let canvasTimer;
let sessionReady = false;
let initialProject = '';
let ai;
const displayed = () => state.view === 'confirmed' ? state.server?.confirmed?.design || emptyDesign() : state.proposal ? (state.proposalOriginal ? state.proposal.before : state.proposal.after) : state.draft;
const comparison = () => state.view === 'draft' && state.proposal && !state.proposalOriginal ? state.proposal.before : null;
const graph = () => graphDesign({...displayed(),collaborations:displayed().collaborations || []},comparison());
const readonly = () => state.view === 'confirmed' || !!state.proposal || state.aiBusy;
const revision = () => state.server?.confirmed?.revision || '';
const flags = () => {
  const report = state.observations.check?.report;
  return report?.design_revision === revision() ? new Set((report.deterministic.violations || []).flatMap(item => [item.rule.from, item.rule.to])) : new Set();
};

function errorNotice(error) {
  $('#notices').innerHTML = `<div class="banner error" role="alert"><strong>${e(error.message)}</strong>${error.issues?.length ? `<ul>${error.issues.map(issue => `<li><code>${e(issue.path)}</code> ${e(issue.message)}</li>`).join('')}</ul>` : ''}<button type="button" class="small" id="dismiss-error">关闭提示</button></div>`;
  $('#dismiss-error').onclick = () => { $('#notices').innerHTML = ''; };
}

function updateConnection(online) {
  state.online = online;
  document.body.classList.toggle('offline', !online);
  $('#connection-status').textContent = online ? '已连接 · 每 3 秒同步' : '连接中断 · 正在重试';
}

// 对确认弹窗也保留打开时的版本；外部更新会使后端确认失败，要求重新审阅。
function sync(force = false) {
  // 用户主动载入时等待已发出的请求，再读取一次；不能因轮询忙而跳过这次操作。
  if (polling) return force ? polling.then(() => sync(true)) : polling;
  if (state.busy || state.aiBusy || state.switching || document.hidden && !force) return Promise.resolve();
  polling = readRemote(force).finally(() => {polling = null;});
  return polling;
}

async function readRemote(force) {
  const requestVersion = mutationVersion, requestLayoutVersion = layoutVersion;
  try {
    const [remote, observations] = await Promise.all([api('state'), api('observations')]);
    // 已发出的轮询可能晚于保存请求到达，不能把旧响应重新应用到成功保存后的页面。
    if (requestVersion !== mutationVersion) return;
    setToken(remote.token);
    setProjectPath(remote.project.path);
    updateConnection(true);
    $('#sync-error').hidden = true;
    const analysisChanged = state.server && (JSON.stringify(remote.initial_analysis) !== JSON.stringify(state.server.initial_analysis) || remote.pending_proposal_id !== state.server.pending_proposal_id);
    const changed = state.server && (analysisChanged || (remote.draft_hash !== state.server.draft_hash || (remote.confirmed?.revision || '') !== revision()));
    const first = !state.server;
    if (!first && !force && changed && (state.dirty || state.proposal || $('#dialog').open || state.conflict)) {
      if (!state.conflict) toast('外部设计已更新，你的编辑已保留。请查看页面顶部的冲突提示。');
      state.conflict = true;
    } else if (first || force || changed) {
      state.server = remote;
      state.draft = clone(remote.draft || remote.confirmed?.design || emptyDesign());
      state.base = clone(state.draft);
      state.dirty = false;
      state.conflict = false;
    } else {
      state.server = remote;
    }
    const layoutChanged = JSON.stringify(state.layout) !== JSON.stringify(remote.layout);
    if (requestLayoutVersion === layoutVersion && !state.dragging && !state.layoutSaving && !layoutTimer && !Object.keys(pendingNodes).length) state.layout = remote.layout;
    const reportsChanged = JSON.stringify(state.observations) !== JSON.stringify(observations);
    state.observations = observations;
    if (analysisChanged && !state.conflict) await ai.restore();
    renderProjectIdentity(remote);
    if (first || force || changed && !state.conflict) renderDesign();
    else if (!state.dragging && (reportsChanged || layoutChanged)) renderCanvas();
    renderTop();
    if (reportsChanged || first || force || changed) refreshReports();
  } catch (error) {
    if (requestVersion !== mutationVersion) return;
    updateConnection(false);
    const message = `无法同步项目：${error.message}。页面中的编辑已保留，请检查终端中的工作台进程。`;
    if ($('#sync-error').textContent !== message) $('#sync-error').textContent = message;
    $('#sync-error').hidden = false;
    renderTop();
    if (force) throw error;
  }
}

function renderTop() {
  const current = state.server?.confirmed;
  const analysis = state.server?.initial_analysis;
  const pendingAnalysis = analysis && !analysis.confirmed;
  const confirmed = state.view === 'confirmed';
  const sameBaseline = !!current && current.design_hash === state.server?.draft_hash && !state.dirty && !pendingAnalysis;
  $('#conflict-banner').hidden = !state.conflict;
  $('#save-state').textContent = state.switching ? '正在切换项目…' : !state.online ? '连接中断，编辑已保留' : state.conflict ? '版本冲突，请先处理上方提示' : state.aiBusy ? '正在处理上下文与提案' : state.busy ? '正在保存…' : confirmed ? '已确认版本 · 只读' : state.proposal ? '提案待审阅 · 接受后保存为草稿' : state.dirty ? '有修改待保存' : pendingAnalysis && analysis.withdrawn ? '分析已撤回，需重新分析后确认' : sameBaseline ? '草稿与已确认基准一致' : state.server?.draft ? '草稿已保存 · 待确认' : '从代码或需求开始，也可手动创建模块';
  // 流程提示只解释真实依赖，不把可往返的工作区变成必须逐步通过的向导。
  $('#design-flow-hint').textContent = current ? sameBaseline && !state.proposal ? '在编程工具中实现后回来检查；已有代码可直接进入「代码检查」。' : '新草稿确认前，代码检查仍使用已确认基准。' : state.server?.initial_analysis_available ? '在 coding agent 中分析源码 → 在此审阅确认 → 代码检查。' : '确认设计 → 在编程工具中实现 → 回来检查。';
  // 保存、确认、交接各占一个阶段；接受提案在提案面板中保持独立。
  $('#save-draft').hidden = confirmed || !!state.proposal || !state.dirty;
  $('#review-design').hidden = confirmed || !!state.proposal || state.dirty || !state.server?.draft || sameBaseline;
  $('#save-draft').disabled = !state.server || !state.online || !state.dirty || state.busy || state.switching || state.conflict || readonly();
  // 相同设计也可确认新源码依据；未接受或撤回的分析仍须先处理。
  $('#review-design').disabled = !state.online || !state.server?.draft || state.dirty || state.busy || state.switching || state.conflict || readonly() || (current?.design_hash === state.server?.draft_hash && !pendingAnalysis) || !!(pendingAnalysis && (!analysis.accepted || analysis.withdrawn));
  $('#review-design').title = state.dirty ? '请先保存草稿，再审阅确认' : '';
  const handoffPrimary = !!current && (confirmed || !state.proposal && sameBaseline);
  const handoff = $('#export-handoff'), handoffParent = handoffPrimary ? $('#design-primary-actions') : $('#handoff-tool-slot');
  if (handoff.parentElement !== handoffParent) handoffParent.append(handoff);
  handoff.hidden = !current;
  handoff.classList.toggle('primary',handoffPrimary);
  $('#version-summary').textContent = current ? `基准 ${short(current.revision)} · ${current.confirmed_by}` : '尚无已确认设计';
  $('#confirmed-view').disabled = !current;
  $('#draft-view').classList.toggle('selected', state.view === 'draft');
  $('#confirmed-view').classList.toggle('selected', state.view === 'confirmed');
  $('#draft-view').setAttribute('aria-pressed', String(state.view === 'draft'));
  $('#confirmed-view').setAttribute('aria-pressed', String(state.view === 'confirmed'));
  $('#add-module').disabled = !state.server || state.switching || readonly();
  $('#choose-project').disabled = !sessionReady || state.busy || state.aiBusy || state.scanning || state.switching || state.layoutSaving;
  $('#draft-hash').textContent = state.view === 'confirmed' ? `已确认于 ${date(current?.confirmed_at)}` : state.server?.draft_hash ? `草稿指纹 ${short(state.server.draft_hash)}` : '';
  for (const id of ['run-check','review-request','review-import']) $('#' + id).disabled = !state.online || !current || state.scanning || state.switching;
  // 未确认时只有起步引导；已有基准时始终可检查，未确认的新草稿不阻断检查。
  $('#check-onboarding').hidden = !!current;
  for (const id of ['run-check','check-tools','check-results']) $('#' + id).hidden = !current;
  $('#run-check').textContent = state.scanning ? '正在检查…' : '运行依赖检查 ↗';
  $('#check-baseline').textContent = current ? `检查基准 ${short(current.revision)} · ${current.confirmed_by} 于 ${date(current.confirmed_at)} 确认` : '请先在架构设计中保存并确认一个版本。';
  ai?.render();
}

function renderProjectIdentity(remote) {
  $('#project-name').textContent = remote.project.name;
  $('#project-path').textContent = remote.project.path;
  $('#breadcrumb-project').textContent = remote.project.name;
  $('#connected-path').textContent = remote.project.path;
  document.title = `${remote.project.name} · Interface 架构工作台`;
}

function chooseProject() {
  if ($('#choose-project').disabled) return;
  projectPicker({
    path:state.server?.project.path || initialProject,
    unsaved:state.dirty || !!state.proposal || !!layoutTimer || Object.keys(pendingNodes).length > 0,
    onBackup:() => download(state.draft, `${state.server?.project.name || 'project'}-draft.json`),
    onSelect:async (path, isActive) => {
      if (state.busy || state.aiBusy || state.scanning || state.layoutSaving || state.switching) throw new Error('当前操作尚未结束，请稍后再切换项目。');
      state.switching = true; mutationVersion++; renderTop();
      clearTimeout(layoutTimer); layoutTimer = null;
      clearTimeout(canvasTimer);
      try {
        const remote = await api('projects/open','POST',{path});
        const observations = await api('observations','GET',undefined,remote.project.path);
        if (!isActive()) return false;
        // 新项目全部读取成功后一次替换页面；路径错误或取消不会清空原项目的编辑。
        setProjectPath(remote.project.path); setToken(remote.token);
        state.server = remote; state.draft = clone(remote.draft || remote.confirmed?.design || emptyDesign());
        state.base = clone(state.draft); state.dirty = false; state.conflict = false;
        state.analysisProposal = null; state.confirmedAnalysisProposal = null; state.analysisFailure = null;
        state.selected = null; state.selection = {kind:'module',id:''}; state.proposal = null; state.proposalOriginal = false; state.view = 'draft'; state.layout = remote.layout;
        state.observations = observations; pendingNodes = {}; layoutVersion++;
        try {sessionStorage.setItem('interface-project-path',remote.project.path);} catch { /* 禁用浏览器存储时仍允许正常切换。 */ }
        $('#notices').innerHTML = ''; $('#sync-error').hidden = true;
        $('#layout-status').textContent = '布局单独保存，不改变设计版本';
        updateConnection(true); renderProjectIdentity(remote); showPage('design'); renderDesign(); refreshReports();
        await ai.restore();
        toast(`已切换到 ${remote.project.name}`);
        return true;
      } finally {
        state.switching = false; renderTop();
        if (Object.keys(pendingNodes).length) queueLayout();
      }
    },
  });
}

function renderDesign() {
  const d = graph();
  const selectedItems = {module:d.modules,interface:d.interfaces,collaboration:d.collaborations};
  if (state.selection.kind !== 'design' && !selectedItems[state.selection.kind]?.some(item => item.id === state.selection.id)) {
    const first = d.modules[0];
    state.selection = first ? {kind:'module',id:first.id} : {kind:'design',id:''};
  }
  // 图、详情与 AI 目标只保留一个选中来源；选择整个设计时同步取消模块高亮。
  const {kind,id} = state.selection;
  state.selected = kind === 'module' ? id : kind === 'interface' ? d.interfaces.find(item => item.id === id)?.module_id : kind === 'collaboration' ? d.collaborations.find(item => item.id === id)?.from : null;
  renderIndex(); renderCanvas(); renderInspector(); renderTop();
}

function renderIndex() {
  $('#module-count').textContent = displayed().modules.length;
  $('#module-index').innerHTML = displayed().modules.map(module => `<button type="button" class="module-link ${module.id === state.selected ? 'selected' : ''}" data-module="${e(module.id)}">${e(module.id)}</button>`).join('');
}

function pointFor(module, index) {
  const saved = Object.hasOwn(state.layout.nodes,module.id) ? state.layout.nodes[module.id] : null;
  const width = $('#canvas-scroll').clientWidth;
  const columns = Math.max(1, Math.floor((width - 24) / 400));
  return saved ? {x:Math.min(5000, Math.max(24, saved.x)), y:Math.min(5000, Math.max(24, saved.y))} : {x:34 + index % columns * 400, y:34 + Math.floor(index / columns) * Math.max(300,180 + Math.max(0,...graph().modules.map(m => graph().interfaces.filter(a => a.module_id === m.id).length))*40)};
}

function renderCanvas() {
  // 重绘前记录控件类型与稳定 ID，接口和 SVG 连线不能退回模块按钮或丢失键盘焦点。
  const focused = document.activeElement;
  const focusKind = ['interface','collaboration','drag','module'].find(kind => focused?.hasAttribute(`data-${kind}`));
  const focusID = focusKind ? focused.dataset[focusKind] : null;
  const d = displayed(), union = graph();
  $('#canvas-counts').textContent = `${d.modules.length} 模块 / ${d.interfaces.length} 接口 / ${(d.collaborations || []).length} 协作`;
  $('#canvas-empty').hidden = union.modules.length > 0;
  const points = Object.fromEntries(union.modules.map((module,index) => [module.id,pointFor(module,index)]));
  renderNodes(d,comparison(),points,state.selected,state.selection,flags());
  const nodes = [...document.querySelectorAll('[data-node]')];
  const maxX = Math.max($('#canvas-scroll').clientWidth,...nodes.map(n => n.offsetLeft+n.offsetWidth+190));
  const maxY = Math.max($('#canvas-scroll').clientHeight || 423,...nodes.map(n => n.offsetTop+n.offsetHeight+45));
  $('#canvas').style.width = `${maxX}px`; $('#canvas').style.height = `${maxY}px`;
  $('#edges').setAttribute('width',maxX); $('#edges').setAttribute('height',maxY);
  drawEdges();
  if (focusID) [...$('#canvas').querySelectorAll(`[data-${focusKind}]`)].find(node => node.dataset[focusKind] === focusID)?.focus({preventScroll:true});
}

function drawEdges() {
  renderEdges(displayed(),comparison(),state.selection,$('#show-forbidden').checked);
}

// 接口归属决定协作的目标，详情和图上的文字使用同一条解释规则。
function collaborationTitle(d, link) {
  const target = d.interfaces.find(item => item.id === link.interface_id);
  return `${link.from} → ${target?.module_id || '?'} · ${target?.name || link.interface_id}`;
}

function collaborationSection(d, moduleID) {
  const links = (d.collaborations || []).filter(link => link.from === moduleID || d.interfaces.some(item => item.id === link.interface_id && item.module_id === moduleID));
  return `<section class="inspector-section"><div class="section-title"><h3>接口协作 <span class="muted">${links.length}</span></h3>${!readonly() ? '<button type="button" class="text-button" data-action="new-collaboration">＋ 添加</button>' : ''}</div>${links.map(link => `<div class="collaboration-item"><button type="button" class="text-button" data-select-collaboration="${e(link.id)}">${e(collaborationTitle(d,link))}</button><p>${e(link.purpose)}</p></div>`).join('') || '<p class="empty-note">尚未记录协作关系。未画出的关系不自动违规。</p>'}</section>`;
}

function selectObject(kind,id) {
  state.selection = {kind,id};
  const d = graph();
  state.selected = kind === 'interface' ? d.interfaces.find(item => item.id === id)?.module_id : d.collaborations.find(item => item.id === id)?.from;
  showPage('design'); renderDesign();
  if (matchMedia('(max-width:1000px)').matches) $('#inspector').scrollIntoView({behavior:'smooth',block:'start'});
}

// 只读状态给出当前可用入口，不指向已经收起的表单。
function readonlyHint() {
  if (state.view === 'confirmed') return '这是已确认的版本。点击「返回草稿编辑」后可调整设计。';
  if (state.aiBusy) return '正在生成提案，完成后可审阅或继续调整。';
  return '这是待审阅提案。点击「继续调整提案」描述修改，或接受后编辑草稿。';
}

function renderObjectInspector(d) {
  const isInterface = state.selection.kind === 'interface';
  const item = (isInterface ? d.interfaces : d.collaborations).find(item => item.id === state.selection.id);
  if (!item) return;
  const semantic = isInterface ? item.semantics || {} : {};
  $('#inspector').innerHTML = `<div class="inspector-head"><h2>${isInterface ? '接口详情' : '协作详情'}</h2><button type="button" class="text-button" data-module="${e(state.selected)}">查看模块</button></div><div class="object-inspector"><code>${e(item.id)}</code><h3>${e(isInterface ? item.name : collaborationTitle(d,item))}</h3><p>${e(isInterface ? item.description : item.purpose)}</p>${isInterface ? `<p>所属模块：<button type="button" class="text-button" data-module="${e(item.module_id)}">${e(item.module_id)}</button></p><dl>${[['inputs','输入含义'],['outputs','输出含义'],['errors','错误语义']].map(([key,label]) => `<dt>${label}</dt><dd>${e(semantic[key] || '尚未补充')}</dd>`).join('')}</dl>` : '<p class="muted">这条线表示使用具体接口，不表示直接包引用，也不构成调用方白名单。</p>'}${!readonly() ? `<button type="button" class="soft" ${isInterface ? 'data-edit-interface' : 'data-edit-collaboration'}="${e(item.id)}">编辑${isInterface ? '接口' : '协作'}</button>` : `<p class="readonly-note">${readonlyHint()}</p>`}${isInterface ? `<section class="inspector-section"><h3>使用此接口</h3>${(d.collaborations || []).filter(link => link.interface_id === item.id).map(link => `<p><button type="button" class="text-button" data-select-collaboration="${e(link.id)}">${e(link.from)} → ${e(item.name)}</button></p>`).join('') || '<p class="empty-note">暂无协作约定</p>'}</section>` : ''}${objectEvidence(state,isInterface ? 'interface' : 'collaboration',item.id)}</div>`;
}

function editCollaboration(id) {
  if (readonly()) return;
  if (state.draft.modules.length < 2 || !state.draft.interfaces.length) {toast('请先创建至少两个模块和一个接口。'); return;}
  const item = (state.draft.collaborations || []).find(link => link.id === id);
  modal(item ? '编辑接口协作' : '添加接口协作', `${field('协作 ID','id',item?.id || '',{readonly:!!item,hint:'稳定标识，例如 order-refund'})}<label class="field"><span>使用接口的模块</span><select name="from" required>${state.draft.modules.map(module => `<option value="${e(module.id)}" ${module.id === (item?.from || state.selected) ? 'selected' : ''}>${e(module.id)}</option>`).join('')}</select></label><label class="field"><span>使用的具体接口</span><select name="interface_id" required>${state.draft.interfaces.map(api => `<option value="${e(api.id)}" ${api.id === item?.interface_id ? 'selected' : ''}>${e(api.module_id)} · ${e(api.name)}</option>`).join('')}</select></label>${field('协作目的','purpose',item?.purpose,{area:true})}<div class="dialog-footer">${item ? '<button type="button" class="danger" id="remove-collaboration">删除协作</button>' : ''}<button type="button" data-close>取消</button><button type="submit" class="primary">应用到草稿</button></div>`, (form,dialog) => {
    const link = {id:form.get('id').trim(),from:form.get('from'),interface_id:form.get('interface_id'),purpose:form.get('purpose').trim()};
    checkID(link.id,item?.id);
    if (state.draft.interfaces.find(api => api.id === link.interface_id)?.module_id === link.from) throw new Error('请选择另一个模块提供的接口。');
    state.draft.collaborations ||= [];
    if (state.draft.collaborations.some(other => other !== item && other.from === link.from && other.interface_id === link.interface_id)) throw new Error('这条协作关系已经存在。');
    if (item) state.draft.collaborations.splice(state.draft.collaborations.indexOf(item),1,link); else state.draft.collaborations.push(link);
    markDirty(); renderDesign(); dialog.close();
  });
  if (item) $('#remove-collaboration').onclick = () => confirmAction('删除接口协作',`从草稿移除 ${collaborationTitle(state.draft,item)}。`,() => {state.draft.collaborations = state.draft.collaborations.filter(link => link.id !== id); markDirty(); renderDesign();});
}

function renderInspector() {
  const d = graph(), module = d.modules.find(item => item.id === state.selected);
  if (state.selection.kind === 'interface' || state.selection.kind === 'collaboration') {renderObjectInspector(d); return;}
  if (!module) { $('#inspector').innerHTML = `<div class="inspector-head"><h2>设计详情</h2></div><div class="inspector-empty"><h2>把边界说清楚</h2><p>选择画布中的模块，查看职责、对外能力和依赖限制。</p><p>设计由你确认，模块内部如何实现交给编程工具。</p></div>`; return; }
  const interfaces = d.interfaces.filter(item => item.module_id === module.id);
  const rules = d.forbidden_dependencies.filter(item => item.from === module.id || item.to === module.id);
  $('#inspector').innerHTML = `<div class="inspector-head"><h2>模块详情</h2><span class="tag ${readonly() ? 'neutral' : ''}">${state.view === 'confirmed' ? '已确认 · 只读' : readonly() ? '提案预览 · 只读' : '编辑草稿'}</span></div><div class="inspector-body">${readonly() ? `<div class="readonly-note">${readonlyHint()}</div>` : ''}<label class="field"><span>模块 ID <small>稳定标识，不随说明改变</small></span><code>${e(module.id)}</code></label>${field('模块目录','module-root',module.root,{readonly:readonly(), hint:'项目相对路径'})}${field('职责与边界','module-responsibility',module.responsibility,{area:true,readonly:readonly()})}<section class="inspector-section"><div class="section-title"><h3>对外能力 <span class="muted">${interfaces.length}</span></h3>${!readonly() ? '<button type="button" class="text-button" data-action="new-interface">＋ 添加</button>' : ''}</div>${interfaces.map(item => `<div class="capability-item"><div class="capability-title"><strong>${e(item.name)}</strong>${!readonly() ? `<button type="button" class="small text-button" data-edit-interface="${e(item.id)}" aria-label="编辑 ${e(item.name)}">编辑</button>` : ''}</div><code>${e(item.id)}</code><p>${e(item.description)}</p>${item.semantics ? `<dl>${[['inputs','输入'],['outputs','输出'],['errors','错误']].filter(([key]) => item.semantics[key]).map(([key,label]) => `<dt>${label}</dt><dd>${e(item.semantics[key])}</dd>`).join('')}</dl>` : ''}</div>`).join('') || '<p class="empty-note">还没有对外能力，添加协作所需的接口约定。</p>'}</section>${objectEvidence(state,'module',module.id)}${collaborationSection(d,module.id)}<section class="inspector-section"><div class="section-title"><h3>依赖限制 <span class="muted">${rules.length}</span></h3>${!readonly() ? '<button type="button" class="text-button" data-action="new-rule">＋ 添加</button>' : ''}</div>${rules.map(rule => `<div class="rule-item"><strong>${e(rule.from)} → ${e(rule.to)}</strong><p>禁止依赖 · ${e(rule.reason)}</p>${!readonly() ? `<button type="button" class="text-button small" data-edit-rule="${e(rule.id)}" aria-label="编辑规则 ${e(rule.id)}">编辑规则</button>` : ''}</div>`).join('') || '<p class="empty-note">没有相关禁止规则，依赖默认允许。</p>'}</section>${!readonly() ? '<button type="button" class="text-button danger small delete-module" data-action="delete-module">删除此模块</button>' : ''}</div>`;
  for (const [name, key] of [['module-root','root'],['module-responsibility','responsibility']]) {
    $(`[name=${name}]`).oninput = event => {
      module[key] = event.target.value;
      markDirty();
      clearTimeout(canvasTimer); canvasTimer = setTimeout(renderCanvas, 160);
    };
  }
}

function markDirty() {
  state.dirty = !state.server?.draft || JSON.stringify(state.draft) !== JSON.stringify(state.base);
  renderTop();
}

function selectModule(id) {
  state.selected = id; state.selection = {kind:'module',id};
  showPage('design'); renderDesign();
  if (matchMedia('(max-width:1000px)').matches) $('#inspector').scrollIntoView({behavior:'smooth',block:'start'});
}

function showPage(page) {
  state.page = page;
  for (const name of ['design','checks','connect']) $(`#${name}-page`).hidden = name !== page;
  // 页面中的跳转 CTA 也使用 data-page，但只有导航入口拥有活动态。
  document.querySelectorAll('[data-navigation][data-page]').forEach(button => { const active = button.dataset.page === page; button.classList.toggle('active',active); if (active) button.setAttribute('aria-current','page'); else button.removeAttribute('aria-current'); });
  if (page === 'design') requestAnimationFrame(renderCanvas);
  if (page === 'checks') refreshReports();
}

function checkID(id, existing = '') {
  if (!/^[a-z][a-z0-9._-]{0,127}$/.test(id)) throw new Error('ID 以小写字母开头，只能含小写字母、数字、点、下划线和短横线，最多 128 个字符。');
  if (id !== existing && [...state.draft.modules,...state.draft.interfaces,...state.draft.forbidden_dependencies,...(state.draft.collaborations || [])].some(item => item.id === id)) throw new Error(`ID 已存在：${id}`);
}

function newModule() {
  if (readonly() || !state.server) return;
  modal('新建模块', `<p>一个模块独占一个目录子树，职责说明用于约束实现边界。</p>${field('模块 ID','id','',{hint:'例如 payment'})}${field('模块目录','root','',{hint:'例如 payment 或 internal/payment'})}${field('职责与边界','responsibility','',{area:true})}<div class="dialog-footer"><button type="button" data-close>取消</button><button type="submit" class="primary">添加到草稿</button></div>`, (form, dialog) => {
    const id = form.get('id').trim(); checkID(id);
    state.draft.modules.push({id,root:form.get('root').trim(),responsibility:form.get('responsibility').trim()});
    state.selected = id; state.selection = {kind:'module',id}; markDirty(); renderDesign(); dialog.close();
    toast('模块已加入当前草稿，请保存后审阅。');
  });
}

function editInterface(id) {
  if (readonly()) return;
  const item = state.draft.interfaces.find(item => item.id === id);
  modal(item ? '编辑对外能力' : '添加对外能力', `<p>描述模块提供什么能力，不必逐字段定义 Go 参数或结构体。</p><div class="form-grid"><div>${field('接口 ID','id',item?.id || '',{readonly:!!item,hint:'稳定标识'})}</div><div>${field('能力名称','name',item?.name)}</div><div class="wide"><label class="field"><span>所属模块</span><select name="module_id" required>${state.draft.modules.map(module => `<option value="${e(module.id)}" ${module.id === (item?.module_id || state.selected) ? 'selected' : ''}>${e(module.id)}</option>`).join('')}</select></label>${field('能力描述','description',item?.description,{area:true})}</div></div>${[['inputs','输入含义'],['outputs','输出含义'],['errors','错误语义']].map(([key,label]) => field(label,key,item?.semantics?.[key],{required:false,area:true,hint:'选填'})).join('')}<div class="dialog-footer">${item ? '<button type="button" class="danger" id="remove-interface">删除能力</button>' : ''}<button type="button" data-close>取消</button><button type="submit" class="primary">应用到草稿</button></div>`, (form, dialog) => {
    const apiID = form.get('id').trim(); checkID(apiID,item?.id);
    const updated = {id:apiID,module_id:form.get('module_id'),name:form.get('name').trim(),description:form.get('description').trim()};
    if ((state.draft.collaborations || []).some(link => link.interface_id === apiID && link.from === updated.module_id)) throw new Error('迁移后存在模块使用自身接口的协作，请先调整相关协作。');
    const semantics = Object.fromEntries(['inputs','outputs','errors'].map(key => [key,form.get(key).trim()]).filter(([,value]) => value));
    if (Object.keys(semantics).length) updated.semantics = semantics;
    if (item) state.draft.interfaces.splice(state.draft.interfaces.indexOf(item),1,updated); else state.draft.interfaces.push(updated);
    markDirty(); renderDesign(); dialog.close();
  });
  if (item) $('#remove-interface').onclick = () => confirmAction('删除对外能力',`将从草稿中删除「${item.name}」及引用它的协作关系。已确认设计保持原版本，直到你再次确认。`,() => {state.draft.interfaces = state.draft.interfaces.filter(api => api.id !== id); state.draft.collaborations = (state.draft.collaborations || []).filter(link => link.interface_id !== id); markDirty(); renderDesign();});
}

function editRule(id) {
  if (readonly()) return;
  if (state.draft.modules.length < 2) { toast('至少需要两个模块才能添加跨模块依赖限制。'); return; }
  const item = state.draft.forbidden_dependencies.find(item => item.id === id);
  const select = (name,label,value) => `<label class="field"><span>${label}</span><select name="${name}" required>${state.draft.modules.map(module => `<option value="${e(module.id)}" ${module.id === value ? 'selected' : ''}>${e(module.id)}</option>`).join('')}</select></label>`;
  modal(item ? '编辑依赖限制' : '添加依赖限制', `<p>只禁止指定方向的直接包引用，其他方向默认允许。</p>${field('规则 ID','id',item?.id || '',{readonly:!!item,hint:'例如 payment-no-order'})}<div class="form-grid"><div>${select('from','起点模块',item?.from || state.selected)}</div><div>${select('to','禁止依赖的目标',item?.to || state.draft.modules.find(module => module.id !== state.selected)?.id)}</div></div>${field('禁止原因','reason',item?.reason,{area:true})}<div class="dialog-footer">${item ? '<button type="button" class="danger" id="remove-rule">删除限制</button>' : ''}<button type="button" data-close>取消</button><button type="submit" class="primary">应用到草稿</button></div>`, (form, dialog) => {
    const ruleID = form.get('id').trim(); checkID(ruleID,item?.id);
    const updated = {id:ruleID,from:form.get('from'),to:form.get('to'),reason:form.get('reason').trim()};
    if (updated.from === updated.to) throw new Error('请选择两个不同的模块。');
    if (state.draft.forbidden_dependencies.some(rule => rule !== item && rule.from === updated.from && rule.to === updated.to)) throw new Error('这个依赖方向已经有禁止规则。');
    if (item) state.draft.forbidden_dependencies.splice(state.draft.forbidden_dependencies.indexOf(item),1,updated); else state.draft.forbidden_dependencies.push(updated);
    markDirty(); renderDesign(); dialog.close();
  });
  if (item) $('#remove-rule').onclick = () => confirmAction('删除依赖限制',`从草稿移除 ${item.from} → ${item.to} 的禁止规则。`,() => {state.draft.forbidden_dependencies = state.draft.forbidden_dependencies.filter(rule => rule.id !== id); markDirty(); renderDesign();});
}

async function saveDraft() {
  if (!state.dirty || state.busy || state.conflict) return;
  state.busy = true; renderTop(); $('#notices').innerHTML = '';
  mutationVersion++;
  // 保存期间仍可输入；只把请求发送时的内容记为已保存，后续键入继续保持脏状态。
  const submitted = clone(state.draft);
  try {
    const result = await api('draft','PUT',{design:submitted,expected_draft_hash:state.server.draft_hash,expected_revision:revision()});
    state.base = submitted; state.server.draft = clone(submitted); state.server.draft_hash = result.draft_hash;
    markDirty(); toast('草稿已保存。审阅确认后才会成为检查基准。');
  } catch (error) { if (error.code === 'version_conflict') state.conflict = true; errorNotice(error); }
  finally { state.busy = false; renderTop(); }
}

// 按稳定 ID 比较内容，审阅时既展示完整草稿，也列出会删除或改写的原约定。
function differences(before, after) {
  const changes = [];
  for (const [key,label] of [['modules','模块'],['interfaces','能力'],['collaborations','协作'],['forbidden_dependencies','限制']]) {
    const old = new Map((before?.[key] || []).map(item => [item.id,item]));
    const next = new Map((after[key] || []).map(item => [item.id,item]));
    for (const [id,item] of next) {
      if (!old.has(id)) changes.push(`新增${label} ${id}`);
      else if (JSON.stringify(old.get(id)) !== JSON.stringify(item)) changes.push(`修改${label} ${id}：${JSON.stringify(old.get(id))} → ${JSON.stringify(item)}`);
    }
    for (const [id,item] of old) if (!next.has(id)) changes.push(`删除${label} ${id}：${JSON.stringify(item)}`);
  }
  return changes;
}

function reviewDesign() {
  if ($('#review-design').disabled) return;
  const reviewed = clone(state.draft), hash = state.server.draft_hash, expected = revision();
  const changes = differences(state.server.confirmed?.design,reviewed);
  modal('审阅并确认设计', `<p>确认后，这份设计会替代当前检查基准。外部工具将以它作为实现约束。</p>${state.server?.initial_analysis?.accepted && !state.server.initial_analysis.confirmed ? '<p>本草稿源于代码分析。确认只表示你选择此设计为约束，不表示当前架构合理、业务正确或模型没有遗漏。确认前将复核源码是否变化。</p>' : ''}<div class="banner info"><strong>${reviewed.modules.length} 个模块 · ${reviewed.interfaces.length} 项能力 · ${(reviewed.collaborations || []).length} 条协作 · ${reviewed.forbidden_dependencies.length} 条禁止规则</strong><p>草稿指纹 <code>${short(hash)}</code> · 当前基准 <code>${short(expected)}</code></p></div><details><summary>相对已确认版本的变更（${changes.length}）</summary><ul class="diff-list">${changes.map(text => `<li>${e(text)}</li>`).join('') || '<li>设计内容未改变</li>'}</ul></details><div class="review-document" tabindex="0" aria-label="待确认的完整设计">${reviewed.modules.map(module => `<section class="review-module"><h3>${e(module.id)} <span class="muted">${e(module.root)}</span></h3><p>${e(module.responsibility)}</p><ul>${reviewed.interfaces.filter(item => item.module_id === module.id).map(item => `<li><strong>${e(item.name)}</strong> <code>${e(item.id)}</code><p>${e(item.description)}</p>${item.semantics ? Object.entries(item.semantics).map(([key,value]) => `<p>${({inputs:'输入',outputs:'输出',errors:'错误'})[key]}：${e(value)}</p>`).join('') : ''}</li>`).join('')}</ul></section>`).join('') || '<p>当前设计没有模块。</p>'}${reviewed.collaborations?.length ? `<section class="review-module"><h3>接口协作</h3>${reviewed.collaborations.map(link => `<p>${e(collaborationTitle(reviewed,link))}：${e(link.purpose)}</p>`).join('')}</section>` : ''}${reviewed.forbidden_dependencies.length ? `<section class="review-module"><h3>禁止依赖的方向</h3>${reviewed.forbidden_dependencies.map(rule => `<p><code>${e(rule.id)}</code><br>${e(rule.from)} → ${e(rule.to)}：${e(rule.reason)}</p>`).join('')}</section>` : '<p class="muted">没有禁止依赖规则，所有方向默认允许。</p>'}</div>${field('确认人','actor',state.server.confirmed?.confirmed_by || '')}<label class="checkbox-label"><input type="checkbox" name="reviewed" required>我已审阅以上完整设计，同意将其作为实现与检查基准。</label><div class="dialog-footer"><button type="button" data-close>继续编辑</button><button type="submit" class="primary">确认此设计版本</button></div>`, async (form, dialog) => {
    state.busy = true; mutationVersion++; renderTop();
    try {
      await api('confirm','POST',{draft_hash:hash,expected_revision:expected,actor:form.get('actor').trim()});
      dialog.close();
    } catch (error) { if (error.code === 'version_conflict') {state.conflict = true; renderTop();} throw error; }
    finally {state.busy = false;renderTop();}
    await sync(true); toast('设计已确认，外部工具和后续检查将使用这个版本。');
  });
}

function queueLayout() {
  layoutVersion++;
  clearTimeout(layoutTimer);
  $('#layout-status').textContent = '布局等待保存…';
  layoutTimer = setTimeout(saveLayout,350);
}

async function saveLayout() {
  layoutTimer = null;
  if (state.layoutSaving) {layoutTimer = setTimeout(saveLayout,350);return;}
  state.layoutSaving = true;
  renderTop();
  const version = layoutVersion;
  const submitted = pendingNodes;
  pendingNodes = {};
  try {
    const merged = await api('layout','PATCH',{nodes:submitted});
    state.layout = {nodes:{...merged.nodes,...pendingNodes}};
    if (!state.dragging) renderCanvas();
    if (version === layoutVersion) $('#layout-status').textContent = '布局已保存 · 不改变设计版本';
  }
  catch (error) { pendingNodes = {...submitted,...pendingNodes}; $('#layout-status').textContent = '布局保存失败，重新移动节点可重试'; errorNotice(error); }
  finally {state.layoutSaving = false;renderTop();}
}

// 只在手柄上接管拖动；节点按钮仍支持键盘选择，方向键提供无鼠标布局操作。
$('#nodes').addEventListener('pointerdown', event => {
  const handle = event.target.closest('[data-drag]');
  if (!handle || event.button !== 0) return;
  event.preventDefault();
  const module = graph().modules.find(item => item.id === handle.dataset.drag);
  const point = pointFor(module,graph().modules.indexOf(module));
  const start = {x:event.clientX,y:event.clientY};
  const node = handle.closest('.module-node');
  state.dragging = true; node.classList.add('dragging'); handle.setPointerCapture(event.pointerId);
  let moved = false;
  const move = next => {
    moved = true;
    const position = {x:Math.max(24, Math.min(5000,point.x + next.clientX-start.x)),y:Math.max(24, Math.min(5000,point.y + next.clientY-start.y))};
    state.layout.nodes[module.id] = position;
    pendingNodes[module.id] = position;
    node.style.left = `${position.x}px`; node.style.top = `${position.y}px`; drawEdges();
  };
  const finish = () => {
    handle.removeEventListener('pointermove',move); handle.removeEventListener('pointerup',finish); handle.removeEventListener('pointercancel',finish);
    state.dragging = false; node.classList.remove('dragging');
    if (moved) {renderCanvas();queueLayout();}
  };
  handle.addEventListener('pointermove',move); handle.addEventListener('pointerup',finish); handle.addEventListener('pointercancel',finish);
});
$('#nodes').addEventListener('keydown', event => {
  const handle = event.target.closest('[data-drag]');
  const delta = {ArrowLeft:[-20,0],ArrowRight:[20,0],ArrowUp:[0,-20],ArrowDown:[0,20]}[event.key];
  if (!handle || !delta) return;
  event.preventDefault();
  const module = graph().modules.find(item => item.id === handle.dataset.drag), point = pointFor(module,graph().modules.indexOf(module));
  state.layout.nodes[module.id] = {x:Math.max(24,point.x+delta[0]),y:Math.max(24,point.y+delta[1])};
  pendingNodes[module.id] = state.layout.nodes[module.id];
  renderCanvas(); queueLayout();
  [...document.querySelectorAll('[data-drag]')].find(button => button.dataset.drag === module.id)?.focus();
});

function refreshReports() {
  renderReports(state, moduleID => {state.view = 'confirmed'; selectModule(moduleID);});
}
bindReports(state, {renderTop,refresh:async () => {await sync();refreshReports();},errorNotice});

// 轻量工具使用原生 details；外部点击收起，Escape 将焦点还给入口。
document.addEventListener('click',event => {
  for (const menu of document.querySelectorAll('.action-menu[open]')) {
    const action = menu.contains(event.target) && event.target.closest('.action-menu-panel button');
    if (!menu.contains(event.target) || action) {
      menu.open = false;
      if (action && !$('#dialog').open) menu.querySelector('summary').focus({preventScroll:true});
    }
  }
});
document.addEventListener('keydown',event => {
  if (event.key !== 'Escape' || $('#dialog').open) return;
  for (const menu of document.querySelectorAll('.action-menu[open]')) {
    event.preventDefault(); menu.open = false;
    if (menu.contains(document.activeElement)) menu.querySelector('summary').focus({preventScroll:true});
  }
});

document.addEventListener('click', event => {
  const button = event.target.closest('button');
  if (!button || button.disabled) return;
  if (button.dataset.page) return showPage(button.dataset.page);
  if (button.dataset.module) return selectModule(button.dataset.module);
  if (button.dataset.interface) return selectObject('interface',button.dataset.interface);
  if (button.dataset.selectCollaboration) return selectObject('collaboration',button.dataset.selectCollaboration);
  if (button.dataset.editCollaboration) return editCollaboration(button.dataset.editCollaboration);
  if (button.dataset.editInterface) return editInterface(button.dataset.editInterface);
  if (button.dataset.editRule) return editRule(button.dataset.editRule);
  switch (button.dataset.action) {
    case 'choose-project': chooseProject();break;
    case 'new-module': newModule();break;
    case 'new-interface': editInterface();break;
    case 'new-rule': editRule();break;
    case 'new-collaboration': editCollaboration();break;
    case 'export': download(displayed(), `${state.server?.project.name || 'project'}-${state.view === 'confirmed' ? 'confirmed' : state.proposal ? 'proposal' : 'draft'}.json`);break;
    case 'export-draft': download(state.draft, `${state.server?.project.name || 'project'}-draft.json`);break;
    case 'import': if (state.proposal || state.aiBusy) {toast('请先接受或放弃提案，再导入设计。'); break;} if (readonly()) {state.view = 'draft';renderDesign();} $('#design-file').click();break;
    case 'delete-module': {
      if (readonly()) return;
      const id = state.selected, count = state.draft.interfaces.filter(item => item.module_id === id).length, rules = state.draft.forbidden_dependencies.filter(item => item.from === id || item.to === id).length;
      confirmAction('删除模块及其约定', `从草稿删除 ${id}，同时删除它的 ${count} 项能力和 ${rules} 条相关规则及相关协作关系。保存草稿不会改变已确认设计。`,() => {
        const removedInterfaces = new Set(state.draft.interfaces.filter(item => item.module_id === id).map(item => item.id));
        state.draft.collaborations = (state.draft.collaborations || []).filter(link => link.from !== id && !removedInterfaces.has(link.interface_id));
        state.draft.modules = state.draft.modules.filter(item => item.id !== id);
        state.draft.interfaces = state.draft.interfaces.filter(item => item.module_id !== id);
        state.draft.forbidden_dependencies = state.draft.forbidden_dependencies.filter(item => item.from !== id && item.to !== id);
        markDirty();renderDesign();
      });break;
    }
    case 'reload-latest': confirmAction('载入最新版本','当前未保存的编辑将被丢弃。需要保留时，请先取消并下载我的草稿。',async () => { ai.clear(); await sync(true); await ai.restore(); },'丢弃本地编辑并载入');break;
  }
});
$('#add-module').onclick = newModule;
$('#choose-project').onclick = chooseProject;
$('#show-forbidden').onchange = drawEdges;
// SVG 连线与详情列表都能选中协作，键盘用户不必依赖命中细线。
$('#edges').addEventListener('click',event => {const edge = event.target.closest('[data-collaboration]'); if (edge) selectObject('collaboration',edge.dataset.collaboration);});
$('#edges').addEventListener('keydown',event => {const edge = event.target.closest('[data-collaboration]'); if (edge && ['Enter',' '].includes(event.key)) {event.preventDefault(); selectObject('collaboration',edge.dataset.collaboration);}});
$('#save-draft').onclick = saveDraft;
$('#review-design').onclick = reviewDesign;
$('#draft-view').onclick = () => {state.view = 'draft';renderDesign();};
$('#confirmed-view').onclick = () => {state.view = 'confirmed';renderDesign();};
$('#arrange').onclick = () => {
  for (const module of displayed().modules) delete state.layout.nodes[module.id];
  for (const [index,module] of displayed().modules.entries()) state.layout.nodes[module.id] = pointFor(module,index);
  for (const module of displayed().modules) pendingNodes[module.id] = state.layout.nodes[module.id];
  renderCanvas();queueLayout();
};
$('#design-file').onchange = async event => {
  const file = event.target.files[0]; event.target.value = '';
  if (!file || state.proposal || state.aiBusy) return;
  const requestVersion = mutationVersion, project = state.server?.project.path;
  try {
    if (file.size > 32 * 1024 * 1024) throw new Error('设计文件不能超过 32 MiB。');
    const imported = await api('validate','POST',await file.text(),project);
    if (requestVersion !== mutationVersion) return;
    const apply = () => {state.view = 'draft';state.draft = imported;markDirty();renderDesign();toast('设计已导入当前页面，请保存草稿。');};
    if (state.dirty) confirmAction('替换当前草稿','导入的设计将替换页面中未保存的编辑，已确认设计不受影响。',apply,'替换当前编辑');else apply();
  } catch (error) {errorNotice(error);}
};
window.addEventListener('beforeunload', event => {
  if (state.dirty || state.proposal || state.aiBusy || state.layoutSaving || layoutTimer || Object.keys(pendingNodes).length) {event.preventDefault();event.returnValue = '';}
});
let resizeTimer;
window.addEventListener('resize', () => {clearTimeout(resizeTimer);resizeTimer = setTimeout(() => {if (!state.dragging) renderCanvas();},150);});
document.addEventListener('visibilitychange', () => {if (!document.hidden) sync();});
async function connect() {
  try {
    const session = await api('session','GET',undefined,'');
    setToken(session.token); sessionReady = true; initialProject = session.project.path;
    let selected = initialProject;
    try {selected = sessionStorage.getItem('interface-project-path') || selected;} catch { /* 无存储权限时使用启动目录。 */ }
    setProjectPath(selected); renderTop(); await sync(); if (state.server) await ai.restore();
  } catch (error) {updateConnection(false);errorNotice(error);renderTop();}
}
bindSourceEvidence(state);
// reviewDesign 交给提案面板，供「接受并审阅确认」在写入草稿后直接打开审阅窗口。
ai = initAI(state,{renderDesign,renderTop,errorNotice,invalidate:() => {mutationVersion++;},sync,revision,reviewDesign});
await connect();
setInterval(() => sessionReady ? sync() : connect(),3000);
