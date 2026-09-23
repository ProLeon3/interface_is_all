import { $, escapeHTML as e } from './ui.js';

// 用稳定 ID 计算真实内容差异，不采用模型自报的变更清单。
export const groups = [['modules','模块'],['interfaces','接口'],['collaborations','协作'],['forbidden_dependencies','禁止规则']];
const equal = (a,b) => JSON.stringify(a) === JSON.stringify(b);
export function changes(before, after) {
  return groups.flatMap(([key,label]) => {
    const old = new Map((before?.[key] || []).map(item => [item.id,item]));
    const next = new Map((after?.[key] || []).map(item => [item.id,item]));
    return [...new Set([...old.keys(),...next.keys()])].flatMap(id => {
      const a = old.get(id), b = next.get(id);
      if (a && b && equal(a,b)) return [];
      return [{key,label,id,before:a,after:b,status:!a ? 'added' : !b ? 'removed' : 'modified'}];
    });
  });
}

export const changeLabel = status => ({added:'新增',removed:'删除',modified:'修改'})[status] || '';

// 逐字段对照：只列出真正变化的字段，审阅者不必在整段 JSON 里找差异；新增或删除的对象列出全部字段。
const fieldLabels = {root:'目录',responsibility:'职责',module_id:'所属模块',name:'名称',description:'能力描述','semantics.inputs':'输入','semantics.outputs':'输出','semantics.errors':'错误',from:'调用模块',interface_id:'使用接口',purpose:'协作目的',to:'禁止依赖',reason:'原因'};
const flatten = item => Object.fromEntries(Object.entries(item || {}).flatMap(([key,value]) => key === 'id' ? [] : key === 'semantics' ? Object.entries(value || {}).map(([sub,text]) => [`semantics.${sub}`,text]) : [[key,value]]));
export function fieldChanges(before, after) {
  const a = flatten(before), b = flatten(after);
  return [...new Set([...Object.keys(a),...Object.keys(b)])].filter(key => a[key] !== b[key]).map(key => ({field:key,label:fieldLabels[key] || key,before:a[key],after:b[key]}));
}
export function fieldChangesHTML(item) {
  return fieldChanges(item.before,item.after).map(({label,before,after}) => `<p><span>${e(label)}</span> ${item.status === 'modified' ? `${before === undefined ? '' : `<del>${e(before)}</del> `}${after === undefined ? '<em>已删去</em>' : `<ins>${e(after)}</ins>`}` : e(item.status === 'added' ? after : before)}</p>`).join('');
}
export function graphDesign(d, before) {
  if (!before) return d;
  // 删除对象作为轮廓保留在原位置，迁出的接口也在原模块留下可见标记。
  return Object.fromEntries(Object.entries(d).map(([key,value]) => [key,groups.some(([name]) => name === key)
    ? [...value,...(before[key] || []).filter(item => !value.some(next => next.id === item.id))] : value]));
}

export function renderNodes(d, before, points, selected, selection, flagged) {
  const union = graphDesign({...d,collaborations:d.collaborations || []}, before);
  const diff = changes(before || d,d);
  const status = (key,id) => diff.find(item => item.key === key && item.id === id)?.status || '';
  $('#nodes').innerHTML = union.modules.map(module => {
    const point = points[module.id], kind = status('modules',module.id);
    const apis = union.interfaces.filter(item => item.module_id === module.id);
    const moved = (before?.interfaces || []).filter(item => item.module_id === module.id && d.interfaces.some(next => next.id === item.id && next.module_id !== module.id));
    return `<article class="module-node ${selected === module.id ? 'selected' : ''} ${flagged.has(module.id) ? 'flagged' : ''} ${kind ? `change-${kind}` : ''}" data-node="${e(module.id)}" style="left:${point.x}px;top:${point.y}px">
      <div class="node-top"><span class="node-number">${changeLabel(kind) || '模块'}${flagged.has(module.id) ? ' · 违规涉及' : ''}</span><button type="button" class="drag-handle" data-drag="${e(module.id)}" aria-label="移动 ${e(module.id)} 模块" title="拖动或用方向键移动">⠿</button></div>
      <button type="button" class="node-select" data-module="${e(module.id)}" aria-pressed="${selected === module.id}"><strong>${e(module.id)}</strong><code>./${e(module.root)}</code><p>${e(module.responsibility)}</p></button>
      <div class="node-interfaces">${apis.map(item => `<button type="button" class="interface-port ${selection.kind === 'interface' && selection.id === item.id ? 'selected' : ''} ${status('interfaces',item.id) ? `change-${status('interfaces',item.id)}` : ''}" data-interface="${e(item.id)}" aria-label="查看接口 ${e(item.name)}"><span>${e(item.name)}</span><small>${changeLabel(status('interfaces',item.id)) || '接口'}</small></button>`).join('') || '<span class="empty-note">尚未约定对外能力</span>'}${moved.map(item => `<span class="moved-interface">迁出：${e(item.name)}</span>`).join('')}</div></article>`;
  }).join('');
}

// 线的终点直接取接口按钮的位置；协作线可鼠标或键盘选择，禁止规则始终单独显示。
export function renderEdges(d, before, selection, showForbidden) {
  const union = graphDesign({...d,collaborations:d.collaborations || []},before), diff = changes(before || d,d);
  const canvasRect = $('#canvas').getBoundingClientRect();
  const rect = node => { const b = node?.getBoundingClientRect(); return b ? {x:b.left-canvasRect.left,y:b.top-canvasRect.top,w:b.width,h:b.height} : null; };
  const nodes = new Map([...document.querySelectorAll('[data-node]')].map(node => [node.dataset.node,rect(node)]));
  const ports = new Map([...document.querySelectorAll('[data-interface]')].map(node => [node.dataset.interface,rect(node)]));
  const links = union.collaborations.map(link => {
    const api = union.interfaces.find(item => item.id === link.interface_id);
    const kind = diff.find(item => item.key === 'collaborations' && item.id === link.id)?.status;
    const oldAPI = before?.interfaces.find(item => item.id === link.interface_id);
    return {...link,to:api?.module_id,target:ports.get(link.interface_id),label:api?.name || link.interface_id,kind:kind || (oldAPI && api?.module_id !== oldAPI.module_id ? 'modified' : '')};
  });
  if (showForbidden) links.push(...union.forbidden_dependencies.map(rule => ({...rule,forbidden:true,label:'禁止直接依赖',kind:diff.find(item => item.key === 'forbidden_dependencies' && item.id === rule.id)?.status})));
  $('#edges').innerHTML = `<defs>${[['use','#476c52'],['ban','#946740'],['removed','#a34d40'],['added','#2d7050'],['modified','#91601c']].map(([id,color]) => `<marker id="arrow-${id}" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto"><path d="M1 1L9 5L1 9" fill="none" stroke="${color}" stroke-width="1.5"/></marker>`).join('')}</defs>` + links.map((link,index) => {
    const from = nodes.get(link.from), owner = nodes.get(link.to), to = link.target || owner;
    if (!from || !to || !owner) return '';
    const right = to.x >= from.x + from.w;
    const separated = right || to.x + to.w <= from.x;
    const x1 = separated ? (right ? from.x+from.w : from.x) : from.x+from.w;
    const y1 = from.y+65;
    const x2 = separated ? (right ? owner.x : owner.x+owner.w) : owner.x+owner.w;
    const y2 = to.y+to.h/2;
    const lane = separated ? (x1+x2)/2 + (index%3-1)*8 : Math.max(from.x+from.w,owner.x+owner.w)+38+(index%5)*24;
    const path = `M${x1} ${y1}H${lane}V${y2}H${x2}`;
    const color = link.kind || (link.forbidden ? 'ban' : 'use');
    const title = link.forbidden ? `${link.from} 禁止依赖 ${link.to}：${link.reason}` : `${link.from} → ${link.to} · ${link.label}：${link.purpose}`;
    return `<g class="graph-edge ${link.forbidden ? 'forbidden-edge' : 'collaboration-edge'} ${link.kind ? `change-${link.kind}` : ''} ${selection.id === link.id ? 'selected' : ''}" ${!link.forbidden ? `data-collaboration="${e(link.id)}" tabindex="0" role="button" aria-label="${e(title)}"` : ''}><title>${e(title)}</title><path class="edge-hit" d="${path}"/><path class="edge-line" d="${path}" marker-end="url(#arrow-${color})"/>${!link.forbidden ? `<text x="${lane+7}" y="${(y1+y2)/2}" class="edge-label">${e(link.label.length > 14 ? link.label.slice(0,13)+'…' : link.label)}${link.kind ? ` · ${changeLabel(link.kind)}` : ''}</text>` : ''}</g>`;
  }).join('');
}
