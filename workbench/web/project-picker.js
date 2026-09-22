import { $, escapeHTML as e, api, modal, field } from './ui.js';

// 本地服务浏览目录，浏览器不上传整个文件夹；用户也可以直接填写路径。
export function projectPicker({path, unsaved, onSelect, onBackup}) {
  let selecting = false;
  const dialog = modal('选择项目', `<p>输入本机项目路径，或浏览目录后选择。新项目可以先设计，运行代码检查时需要 go.mod。</p>${unsaved ? '<div class="banner warning"><strong>当前项目有未保存的编辑</strong><p>可以取消后保存，也可以先下载草稿备份。</p><button type="button" class="small" id="picker-backup">下载当前草稿</button></div>' : ''}<div class="directory-path-row">${field('项目路径','project-path',path,{hint:'支持绝对路径、相对启动目录的路径和 ~'})}<button type="button" id="browse-path">浏览</button></div><div class="directory-toolbar"><div class="button-row"><button type="button" class="small" id="directory-parent" disabled>↑ 上一级</button><button type="button" class="small" id="directory-home">主目录</button></div><label class="directory-hidden"><input type="checkbox" id="show-hidden">显示隐藏目录</label></div><div id="directory-status" class="directory-status" role="status">正在读取目录…</div><div id="directory-list" class="directory-list" aria-label="子目录"></div>${unsaved ? '<label class="checkbox-label"><input type="checkbox" name="discard" required>我同意丢弃当前项目尚未保存的编辑并切换。</label>' : ''}<div class="dialog-footer"><button type="button" data-close>取消</button><button type="submit" class="primary">选择此项目</button></div>`, async (form) => {
    const owner = $('form',dialog);
    // 切换期间固定路径并阻止重复提交，同时保留取消入口。
    selecting = true; browseVersion++;
    const controls = [...owner.querySelectorAll('input,button')].filter(control => !control.hasAttribute('data-close'));
    const disabled = controls.map(control => control.disabled);
    controls.forEach(control => {control.disabled = true;});
    owner.setAttribute('aria-busy','true'); list.removeAttribute('aria-busy');
    try {
      // 用户在读取期间关闭弹窗时取消切换，不能在后台突然换掉工作区。
      const applied = await onSelect(form.get('project-path'), () => dialog.open && owner.isConnected);
      if (applied) dialog.close();
    } finally {
      selecting = false;
      controls.forEach((control,index) => {control.disabled = disabled[index];});
      owner.removeAttribute('aria-busy');
    }
  });
  const owner = $('form',dialog);
  const active = () => dialog.open && owner.isConnected;
  const input = $('[name=project-path]',dialog);
  const submit = $('button[type=submit]',dialog);
  const status = $('#directory-status',dialog);
  const list = $('#directory-list',dialog);
  const up = $('#directory-parent',dialog);
  const hidden = $('#show-hidden',dialog);
  let parent = '', browseVersion = 0;

  const browse = async path => {
    if (selecting) return;
    const version = ++browseVersion;
    status.textContent = '正在读取目录…';
    status.classList.remove('error');
    submit.disabled = true;
    list.setAttribute('aria-busy','true');
    try {
      const result = await api('directories','POST',{path,show_hidden:hidden.checked});
      if (!active() || version !== browseVersion) return;
      input.value = result.current.path;
      parent = result.parent;
      up.disabled = result.parent === result.current.path;
      status.innerHTML = `<code>${e(result.current.path)}</code><span>${result.current.has_go_mod ? 'Go 项目' : '可以在此目录开始设计'}${result.current.has_design ? ' · 已有架构设计' : ''}</span>`;
      list.innerHTML = result.directories.map(directory => `<button type="button" class="directory-row" data-directory="${e(directory.path)}" title="${e(directory.path)}"><span aria-hidden="true" class="folder-icon">▱</span><span class="directory-name">${e(directory.name)}</span>${directory.has_design ? '<span class="tag">已有设计</span>' : directory.has_go_mod ? '<span class="tag neutral">Go 项目</span>' : ''}<span class="directory-arrow" aria-hidden="true">›</span></button>`).join('') || '<p class="directory-empty">没有可见子目录，可以直接选择此项目。</p>';
    } catch (error) {
      if (!active() || version !== browseVersion) return;
      status.textContent = error.message; status.classList.add('error');
      list.innerHTML = ''; up.disabled = true;
    } finally {
      if (active() && version === browseVersion) {submit.disabled = false;list.removeAttribute('aria-busy');}
    }
  };
  $('#browse-path',dialog).onclick = () => browse(input.value);
  $('#directory-home',dialog).onclick = () => browse('~');
  up.onclick = () => browse(parent);
  hidden.onchange = () => browse(input.value);
  input.onkeydown = event => {if (event.key === 'Enter') {event.preventDefault();browse(input.value);}};
  input.oninput = () => {
    // 用户开始输入另一条路径后，较早的目录响应不能回填并覆盖输入。
    browseVersion++; submit.disabled = false; up.disabled = true;
    status.textContent = '点击「浏览」查看目录，或直接选择输入的项目路径。';
    status.classList.remove('error'); list.innerHTML = ''; list.removeAttribute('aria-busy');
  };
  list.onclick = event => {const row = event.target.closest('[data-directory]');if (row) browse(row.dataset.directory);};
  if (unsaved) $('#picker-backup',dialog).onclick = onBackup;
  browse(path);
}
