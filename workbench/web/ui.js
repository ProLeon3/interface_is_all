// 动态项目内容只作为文本插入，不能让设计说明或源码变成可执行 HTML。
export const escapeHTML = (value = '') => String(value ?? '').replace(/[&<>"']/g, char => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[char]));
export const $ = (selector, root = document) => root.querySelector(selector);
export const clone = value => structuredClone(value);
export const short = value => value ? value.slice(0, 9) : '尚未确认';
export const date = value => value ? new Date(value).toLocaleString('zh-CN', {hour12: false}) : '—';

let token = '';
let projectPath = '';
export function setToken(value) { token = value; }
export function setProjectPath(value) { projectPath = value; }

// 服务返回可定位的校验错误；调用方区分版本冲突与普通输入错误。
export async function api(path, method = 'GET', body, project = projectPath) {
  // 中文路径以 URI 编码放入请求头，并在请求发出时固定项目，避免切换时写错目录。
  const headers = project ? {'X-Workbench-Project':encodeURIComponent(project)} : {};
  if (method !== 'GET') Object.assign(headers,{'Content-Type':'application/json', 'X-Workbench-Token':token});
  const response = await fetch(`/api/${path}`, {
    method, cache: 'no-store',
    headers,
    body: body === undefined ? undefined : typeof body === 'string' ? body : JSON.stringify(body),
  });
  const result = await response.json();
  if (!response.ok) {
    const error = new Error(result.error || `请求失败（${response.status}）`);
    error.status = response.status;
    error.code = result.code;
    error.issues = result.issues || [];
    error.source = result.source;
    throw error;
  }
  return result;
}

let toastTimer;
export function toast(message) {
  $('#toast').textContent = message;
  $('#toast').hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { $('#toast').hidden = true; }, 4500);
}

export function download(value, name) {
  const blob = new Blob([JSON.stringify(value, null, 2) + '\n'], {type:'application/json'});
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = name;
  link.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

export function modal(title, content, onSubmit) {
  const dialog = $('#dialog');
  if (dialog.open) dialog.close();
  dialog.innerHTML = `<div class="dialog-head"><h2 id="dialog-title">${escapeHTML(title)}</h2><button type="button" class="dialog-close" aria-label="关闭对话框">×</button></div><form class="dialog-content">${content}<p class="dialog-error" role="alert" hidden></p></form>`;
  $('.dialog-close', dialog).onclick = () => dialog.close();
  dialog.querySelectorAll('[data-close]').forEach(button => { button.onclick = () => dialog.close(); });
  $('form', dialog).onsubmit = async event => {
    event.preventDefault();
    const submit = $('button[type=submit]', dialog);
    if (submit?.disabled) return;
    const original = submit?.textContent;
    if (submit) { submit.disabled = true; submit.textContent = '正在处理…'; }
    const errorNode = $('.dialog-error', dialog);
    errorNode.hidden = true;
    try { await onSubmit?.(new FormData(event.currentTarget), dialog); }
    catch (error) { errorNode.textContent = error.message; errorNode.hidden = false; }
    finally { if (submit) { submit.disabled = false; submit.textContent = original; } }
  };
  dialog.showModal();
  return dialog;
}

export function confirmAction(title, description, action, label = '确认删除') {
  modal(title, `<p>${escapeHTML(description)}</p><div class="dialog-footer"><button type="button" data-close>取消</button><button type="submit" class="danger">${escapeHTML(label)}</button></div>`, async (_, dialog) => { await action(); dialog.close(); });
}

export function field(label, name, value = '', options = {}) {
  const {required = true, area = false, hint = '', readonly = false} = options;
  const attributes = `name="${name}" ${required ? 'required' : ''} ${readonly ? 'readonly' : ''}`;
  return `<label class="field"><span>${escapeHTML(label)}${hint ? `<small>${escapeHTML(hint)}</small>` : ''}</span>${area ? `<textarea ${attributes}>${escapeHTML(value)}</textarea>` : `<input ${attributes} value="${escapeHTML(value)}" autocomplete="off">`}</label>`;
}
