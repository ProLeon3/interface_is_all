import { $, api, clone, escapeHTML as e, download, toast, confirmAction } from './ui.js';
import { changes, changeLabel } from './design-graph.js';
import { renderAnalysis } from './initial-analysis.js';

// 提案与草稿分开保存。刷新可恢复待审阅提案，接受之后才写入磁盘草稿。
export function initAI(state, {renderDesign, renderTop, errorNotice, invalidate, sync, revision}) {
  const key = () => `interface-ai:${state.server?.project.path}`;
  let reviewHTML = '';
  let wasCompact = false;
  // 展开状态属于当前项目的页面交互，轮询和表单输入不能重置它。
  let composerOpen = false;
  let mode = '';
  let renderedProject = '';
  let renderedProposal = '';
  const remember = () => {
    try { sessionStorage.setItem(key(),JSON.stringify({proposal:state.proposal?.id || '',requirement:$('#ai-requirement').value,instruction:$('#ai-instruction').value})); } catch { /* 存储禁用时仍保留当前页面内容。 */ }
  };
  function render() {
    const p = state.proposal, confirmed = state.view === 'confirmed';
    const compact = !!p || state.draft.modules.length > 0 || confirmed && !!state.server?.confirmed?.design.modules.length;
    const canAnalyze = !!(state.server?.initial_analysis_available || state.server?.reanalysis_available);
    const project = state.server?.project.path || '';
    if (project !== renderedProject) {
      renderedProject = project; composerOpen = false; mode = ''; renderedProposal = '';
      $('#ai-requirement-context').open = !compact;
    }
    if ((p?.id || '') !== renderedProposal) {
      renderedProposal = p?.id || ''; composerOpen = false;
      if (p) mode = 'requirement';
    }
    if (!mode || mode === 'code' && !canAnalyze) mode = !compact && canAnalyze ? 'code' : 'requirement';
    const open = !confirmed && (!compact || composerOpen);
    $('#design-starter').hidden = !open;
    $('#design-starter').classList.toggle('initial',!compact);
    $('#starter-title').textContent = compact ? p ? '继续调整提案' : '调整设计' : '开始设计';
    $('#close-composer').hidden = !compact;
    $('#adjust-design').hidden = !compact;
    $('#adjust-design').textContent = confirmed ? '返回草稿编辑' : open ? '收起调整' : p ? '继续调整提案' : '调整设计';
    $('#adjust-design').setAttribute('aria-expanded',String(open));
    $('#adjust-design').disabled = state.switching || state.aiBusy;
    $('#mode-code').hidden = !canAnalyze;
    $('#mode-code').disabled = !!p || state.aiBusy;
    $('#mode-code').textContent = state.server?.confirmed ? '重新分析代码' : '分析已有代码';
    $('#mode-requirement').textContent = compact ? '描述修改' : '从需求设计';
    $('#mode-requirement').disabled = state.aiBusy;
    $('#design-modes').hidden = !canAnalyze;
    for (const [id,value] of [['mode-code','code'],['mode-requirement','requirement']]) {
      $('#' + id).classList.toggle('selected',mode === value);
      $('#' + id).setAttribute('aria-pressed',String(mode === value));
    }
    $('#ai-composer').classList.toggle('compact',compact);
    if (compact !== wasCompact) $('#ai-requirement-context').open = !compact;
    wasCompact = compact;
    $('#ai-composer').hidden = !open || mode !== 'requirement';
    $('#ai-instruction-field').hidden = !compact;
    $('#ai-target').hidden = !compact;
    $('#ai-title').textContent = compact ? '描述修改' : '描述需求';
    $('#proposal-review').hidden = confirmed || !p;
    const selection = state.selection;
    $('#ai-selection').textContent = selection.kind === 'design' ? '整个设计' : `${({module:'模块',interface:'接口',collaboration:'协作'})[selection.kind]} · ${selection.id}`;
    $('#ai-status').textContent = state.aiBusy ? '正在处理上下文与提案…' : '';
    $('#ai-status').hidden = !$('#ai-status').textContent;
    $('#ai-hint').textContent = state.dirty ? '先保存当前草稿，再导出供 coding agent 使用的上下文。' : state.conflict ? '设计已被外部更新，请先处理版本冲突。' : p?.request.source ? '导出的请求包含本次分析源码和所选对象。交给 coding agent 调整后导入结果。' : '将导出的请求交给 coding agent，导入结果后在图上审阅。接受只保存草稿，确认仍是单独一步。';
    // 导出、导入和接受都只执行本地校验；外部 agent 自行完成推理。
    const writeDisabled = !state.online || state.aiBusy || state.busy || state.switching || state.dirty || state.conflict;
    const disabled = writeDisabled;
    $('#proposal-import').disabled = writeDisabled;
    // 已确认的来源保留为历史依据；只有未完成的本次分析占用审阅入口。
    const initial = state.server?.initial_analysis?.confirmed ? null : state.server?.initial_analysis;
    const reanalysis = !!state.server?.confirmed;
    $('#initial-analysis-entry').hidden = !open || mode !== 'code' || !canAnalyze;
    $('#initial-analysis-title').textContent = reanalysis ? '核对最新代码与设计基准' : '从代码还原模块与接口';
    $('#initial-analysis-hint').textContent = reanalysis ? '导出最新源码及设计上下文，交给 coding agent 分析，再导入提案与旧基准对照。' : '导出当前构建范围的源码，交给 coding agent 还原模块与接口，再导入结果审阅。';
    $('#analyze-initial').hidden = !!initial && !initial.withdrawn;
    $('#analyze-initial').disabled = disabled || !!p || state.scanning;
    $('#analyze-initial').textContent = state.aiBusy ? '正在导出…' : reanalysis ? '导出重新分析请求' : '导出源码分析请求';
    $('#analyze-initial').classList.toggle('primary',!compact && !state.dirty);
    $('#discard-initial').hidden = !initial || initial.withdrawn;
    $('#discard-initial').disabled = state.aiBusy || state.busy || !state.online;
    $('#initial-analysis-state').textContent = state.aiBusy ? '正在读取和核对源码，完成后下载请求…' : state.analysisFailure ? '本次请求未完成，草稿保留。实际读取范围在画布下方。' : initial?.withdrawn ? '已撤回分析，草稿保留。请重新分析并接受新提案后再确认。' : initial ? initial.accepted ? '分析已接受为草稿。可继续编辑，再明确确认整个版本。' : '已有分析提案待审阅；接受前不会替换现有草稿。' : p ? '先处理当前待审阅提案，再开始代码分析。' : state.dirty ? '先保存当前编辑，再分析代码；原草稿将在接受提案时才被替换。' : state.conflict ? '请先处理版本冲突，再分析最新代码。' : reanalysis ? '原草稿、基准及历史依据保留。旧禁止规则会在差异中列出，请审阅后决定是否保留。' : '接受分析结果只保存草稿，确认版本仍需单独审阅。';
    renderAnalysis(state);
    $('#ai-generate').disabled = disabled;
    $('#ai-generate').hidden = false;
    $('#ai-generate').classList.toggle('primary',!compact && !state.dirty);
    $('#ai-generate').textContent = state.aiBusy ? '正在导出…' : p ? '导出提案调整请求' : '导出设计请求';
    $('#ai-requirement').disabled = state.aiBusy;
    $('#ai-instruction').disabled = state.aiBusy;
    $('#ai-whole').disabled = state.aiBusy;
    $('#export-handoff').disabled = !state.online || !state.server?.confirmed || state.switching;
    if (!p) { $('#proposal-review').innerHTML = ''; reviewHTML = ''; return; }
    const diff = changes(p.before,p.after);
    const baselineDiff = p.baseline ? changes(p.baseline.design,p.after) : null;
    const entries = items => items.map(item => `<div class="diff-entry change-${item.status}"><strong>${changeLabel(item.status)}${item.label} · ${e(item.id)}</strong>${item.before ? `<p><span>原约定</span> ${e(JSON.stringify(item.before))}</p>` : ''}${item.after ? `<p><span>新约定</span> ${e(JSON.stringify(item.after))}</p>` : ''}</div>`).join('');
    const html = `<div class="proposal-heading"><div><h2>审阅 AI 提案</h2><p>${e(p.summary)}</p></div><span class="tag warning">${diff.length} 处变更 · 尚未接受</span></div>
      <div class="proposal-tools"><div class="segmented" aria-label="提案对照"><button type="button" id="proposal-after" aria-pressed="${!state.proposalOriginal}" class="${!state.proposalOriginal ? 'selected' : ''}">修改后与变更</button><button type="button" id="proposal-before" aria-pressed="${!!state.proposalOriginal}" class="${state.proposalOriginal ? 'selected' : ''}">修改前</button></div><div class="button-row"><details id="proposal-more" class="action-menu"><summary>提案工具</summary><div class="action-menu-panel"><button type="button" id="proposal-download">下载提案</button><button type="button" id="proposal-discard" ${state.aiBusy || state.busy ? 'disabled' : ''}>放弃提案</button></div></details><button type="button" id="proposal-accept" class="primary" ${writeDisabled ? 'disabled' : ''}>接受并保存草稿</button></div></div>
      <details class="proposal-diff"><summary>${baselineDiff ? '相对已保存草稿的变更' : '逐项查看变更'}（${diff.length}）</summary>${entries(diff) || (p.request.source ? '<p>设计内容没有变化；接受后仍可确认本次源码依据。</p>' : '<p>设计内容没有变化，可以继续说明修改意图。</p>')}</details>${baselineDiff ? `<details class="proposal-baseline-diff"><summary>相对已确认基准的变更（${baselineDiff.length}）</summary><p>对照生成时的基准版本 <code>${e(p.expected_revision)}</code>。旧禁止规则属于设计约束，本次现状提案不自动保留；可在接受后编辑草稿恢复需要的规则。</p>${entries(baselineDiff) || '<p>设计内容与旧基准相同，本次仍保存新的源码依据。</p>'}</details>` : ''}<p class="graph-legend"><span class="change-added">新增</span><span class="change-modified">修改 / 迁移</span><span class="change-removed">删除</span>图上保留被删除对象的轮廓；详细差异见上方。</p>`;
    if (html !== reviewHTML) {
      // 对照按钮切换视图后仍保留焦点与展开的差异，允许连续键盘审阅。
      const focusedID = $('#proposal-review').contains(document.activeElement) ? document.activeElement.id : '';
      const detailsOpen = $('.proposal-diff')?.open || false;
      const baselineOpen = $('.proposal-baseline-diff')?.open || false;
      const moreOpen = $('#proposal-more')?.open || false;
      $('#proposal-review').innerHTML = html; reviewHTML = html;
      $('.proposal-diff').open = detailsOpen;
      if ($('.proposal-baseline-diff')) $('.proposal-baseline-diff').open = baselineOpen;
      $('#proposal-more').open = moreOpen;
      $('#proposal-before').onclick = () => {state.proposalOriginal = true; renderDesign();};
      $('#proposal-after').onclick = () => {state.proposalOriginal = false; renderDesign();};
      $('#proposal-download').onclick = () => download(p,'design-proposal.json');
      $('#proposal-discard').onclick = async () => {
        if (p.request.source) { askDiscard(); return; }
        try {
          await api(`proposals/${p.id}/discard`,'POST',{});
          state.server.pending_proposal_id = ''; state.proposal = null; state.proposalOriginal = false; remember(); renderDesign(); toast('已放弃提案，原草稿保留。');
        } catch (error) { errorNotice(error); }
      };
      $('#proposal-accept').onclick = accept;
      if (focusedID) document.getElementById(focusedID)?.focus({preventScroll:true});
    }
  }
  // 切换提案时继承尚未确认草稿的来源保护，与服务器持久记录保持一致。
  function analysisRecord(p) {
    const previous = state.server.initial_analysis;
    const guarded = !previous?.confirmed && (previous?.requires_acceptance || previous?.accepted || previous?.accepting);
    return {proposal_id:p.id,accepted:false,...(guarded ? {requires_acceptance:true} : {})};
  }
  async function generate() {
    if ($('#ai-generate').disabled) return;
    const requirement = $('#ai-requirement').value.trim(), instruction = $('#ai-instruction').value.trim();
    if (!requirement && !instruction || (state.draft.modules.length || state.proposal) && !instruction) { toast('请填写需求；调整已有设计时请写下本次修改意图。'); return; }
    state.aiBusy = true; invalidate(); renderDesign(); remember();
    try {
      const request = await api('design-request','POST',{kind:'design',requirement,instruction,selection:state.selection,parent_id:state.proposal?.id || '',expected_draft_hash:state.server.draft_hash,expected_revision:revision()});
      download(request,'design-request.json');
      toast('请求已导出。交给 coding agent 处理，再导入提案响应。');
    } catch (error) { if (error.code === 'version_conflict') state.conflict = true; errorNotice(error); }
    finally { state.aiBusy = false; renderDesign(); }
  }
  async function accept() {
    if ($('#proposal-accept').disabled) return;
    const p = state.proposal;
    state.busy = true; invalidate(); renderTop();
    try {
      const result = await api(`proposals/${p.id}/accept`,'POST',{});
      state.draft = clone(p.after); state.base = clone(p.after); state.server.draft = clone(p.after); state.server.draft_hash = result.draft_hash;
      if (p.request.source) {
        state.analysisProposal = p;
        state.server.initial_analysis = {...state.server.initial_analysis,proposal_id:p.id,accepted:true};
        delete state.server.initial_analysis.accepting;
      }
      state.server.pending_proposal_id = '';
      state.dirty = false; state.proposal = null; state.proposalOriginal = false; remember();
      toast('提案已接受并保存为草稿。请审阅并确认整个设计版本。');
    } catch (error) { if (error.code === 'version_conflict') state.conflict = true; errorNotice(error); }
    finally { state.busy = false; renderDesign(); }
  }
  async function restore() {
    const project = state.server?.project.path;
    let saved;
    try {saved = JSON.parse(sessionStorage.getItem(key()) || '{}');} catch {saved = {};}
    $('#ai-requirement').value = saved.requirement || '';
    $('#ai-instruction').value = saved.instruction || '';
    const initial = state.server?.initial_analysis;
    const confirmedID = state.server?.confirmed?.source_analysis_id || state.server?.confirmed?.initial_analysis_id;
    // 来源保存在项目中；新窗口也能恢复，不能仅依赖旧标签页的 sessionStorage。
    if (!saved.proposal && !state.server?.pending_proposal_id && !initial && !confirmedID) {state.proposal = null; state.analysisProposal = null; state.confirmedAnalysisProposal = null; render(); return;}
    state.aiBusy = true; renderTop();
    try {
      if (initial) {
        const source = await api(`proposals/${initial.proposal_id}`,'GET',undefined,project);
        if (state.server?.project.path !== project) return;
        state.analysisProposal = source;
      } else { state.analysisProposal = null; }
      state.confirmedAnalysisProposal = confirmedID ? confirmedID === state.analysisProposal?.id ? state.analysisProposal : await api(`proposals/${confirmedID}`,'GET',undefined,project) : null;
      if (state.server?.project.path !== project) return;
      const id = initial && !initial.accepted && !initial.withdrawn && !initial.confirmed ? initial.proposal_id : state.server.pending_proposal_id || saved.proposal;
      // 已撤回或已确认的分析不能由旧标签页缓存重新变成待审阅提案。
      state.proposal = null;
      if (id && id !== confirmedID && !((initial?.accepted || initial?.withdrawn || initial?.confirmed) && id === initial.proposal_id)) {
        const p = id === state.analysisProposal?.id ? state.analysisProposal : await api(`proposals/${id}`,'GET',undefined,project);
        if (state.server?.project.path !== project) return;
        // 已接受或撤回的外部提案不能由标签页缓存重新恢复。
        if (p.request_id && !p.request.source && state.server.pending_proposal_id !== p.id) { remember(); return; }
        state.proposal = p;
        state.conflict ||= p.expected_draft_hash !== state.server.draft_hash || p.expected_revision !== revision();
      }
    } catch (error) {errorNotice(error);}
    finally {state.aiBusy = false; renderDesign();}
  }
  async function analyze() {
    if ($('#analyze-initial').disabled) return;
    state.aiBusy = true; state.analysisFailure = null; invalidate(); renderDesign();
    try {
      const reanalysis = !!revision();
      const request = await api('design-request','POST',{kind:reanalysis ? 'reanalysis' : 'initial_analysis',expected_draft_hash:state.server.draft_hash,expected_revision:revision()});
      download(request,reanalysis ? 'reanalysis-request.json' : 'initial-analysis-request.json');
      toast('源码请求已导出。交给 coding agent 分析，再导入结果审阅。');
    } catch (error) {
      if (error.source) state.analysisFailure = error.source;
      if (error.code === 'version_conflict') state.conflict = true;
      errorNotice(error);
    } finally {state.aiBusy = false; renderDesign();}
  }
  // 只导入本项目已导出请求的响应；源码和版本由服务端重新验证。
  $('#proposal-import').onclick = () => $('#proposal-file').click();
  $('#proposal-file').onchange = async event => {
    const file = event.target.files[0]; event.target.value = '';
    if (!file || $('#proposal-import').disabled) return;
    state.aiBusy = true; invalidate(); renderDesign();
    try {
      const p = await api('proposal-import','POST',JSON.parse(await file.text()));
      state.proposal = p; state.proposalOriginal = false;
      if (p.request.source) {
        state.analysisProposal = p; state.server.initial_analysis = analysisRecord(p);
      } else { state.server.pending_proposal_id = p.id; }
      remember(); toast('提案已导入，请在图上审阅变更。');
    } catch (error) { if (error.code === 'version_conflict') state.conflict = true; errorNotice(error); }
    finally { state.aiBusy = false; renderDesign(); }
  };
  function askDiscard() {
    confirmAction('放弃本次源码分析','原草稿、基准和历史提案仍保留。若草稿含尚未确认的分析结果，需要重新分析并接受新提案后才能确认新版本。',async () => {
      state.busy = true; invalidate(); renderTop();
      try {
        // 撤回只改来源状态。冲突时读取最新版本，不覆盖任一窗口的草稿或本地编辑。
        const remote = await api('state');
        const result = await api(remote.confirmed ? 'reanalysis/discard' : 'initial-analysis/discard','POST',{proposal_id:state.server.initial_analysis.proposal_id,expected_draft_hash:remote.draft_hash,expected_revision:remote.confirmed?.revision || ''});
        state.server.initial_analysis = result.initial_analysis;
        if (!result.initial_analysis) state.analysisProposal = null;
        state.analysisFailure = null;
        if (state.proposal?.request.source) state.proposal = null;
        state.proposalOriginal = false; remember();
        toast('已放弃分析，保存的草稿仍保留。');
      } catch (error) {if (error.code === 'version_conflict') state.conflict = true; throw error;}
      finally {state.busy = false; renderDesign();}
    },'放弃本次分析');
  }
  $('#analyze-initial').onclick = analyze;
  $('#discard-initial').onclick = askDiscard;
  $('#ai-generate').onclick = generate;
  $('#ai-whole').onclick = () => {state.selection = {kind:'design',id:''}; state.selected = null; renderDesign();};
  $('#adjust-design').onclick = () => {
    if (state.view === 'confirmed') {state.view = 'draft'; composerOpen = true; mode = 'requirement'; renderDesign();}
    else {composerOpen = !composerOpen; render();}
  };
  $('#close-composer').onclick = () => {composerOpen = false; render(); $('#adjust-design').focus();};
  for (const [id,value] of [['mode-code','code'],['mode-requirement','requirement']]) {
    $('#' + id).onclick = () => {mode = value; render();};
  }
  $('#ai-requirement').oninput = remember;
  $('#ai-instruction').oninput = remember;
  $('#export-handoff').onclick = async () => {
    try { download(await api('handoff'),`${state.server.project.name}-confirmed-handoff.json`); }
    catch (error) { errorNotice(error); }
  };
  return {render,restore,remember,clear:() => {state.proposal = null; state.proposalOriginal = false; remember();}};
}
