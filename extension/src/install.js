import { ext, isFirefox } from './compat.js';
import { MIN_HELPER, versionLess } from './common.js';
import { el, send, HELPER_FILES, DOWNLOAD_BASE, INSTALL_CMD, installSteps, helperDownload, INSTALL_DOC } from './common.js';

const app = document.getElementById('app');
const statusBox = document.getElementById('status');
let mine = null; // { os, arch } текущего компьютера
let tab = 'windows';
let host = null;
let checking = false;

async function check() {
  checking = true;
  renderStatus();
  const r = await send('retryHost').catch(() => null);
  host = r?.ok ? r.result.host : null;
  checking = false;
  renderStatus();
}

function renderStatus() {
  if (host && versionLess(host.version, MIN_HELPER)) {
    statusBox.replaceChildren(el('div', { class: 'status missing' },
      el('div', {}, el('b', { text: `Установлен старый помощник ${host.version}` }),
        el('span', { class: 'muted small', text: 'Он не поддерживает XHTTP и не обновляется сам. Установите новый по шагам ниже — он заменит старый, затем нажмите «Проверить».' })),
      el('button', { class: 'btn', onclick: check, disabled: checking }, checking ? el('span', { class: 'spinner' }) : 'Проверить')));
    return;
  }
  if (host) {
    statusBox.replaceChildren(el('div', { class: 'status ok' },
      el('div', {}, el('b', { text: 'Помощник установлен' }),
        el('span', { class: 'muted small', text: `Версия ${host.version}, ядро Xray ${host.core || '—'}. Можно закрыть эту вкладку.` }))));
    return;
  }
  statusBox.replaceChildren(el('div', { class: 'status missing' },
    el('div', {}, el('b', { text: 'Помощник пока не найден' }),
      el('span', { class: 'muted small', text: 'Установите его по шагам ниже и нажмите «Проверить».' })),
    el('button', { class: 'btn', onclick: check, disabled: checking }, checking ? el('span', { class: 'spinner' }) : 'Проверить')));
}

function copyButton(text) {
  const b = el('button', { class: 'btn', text: 'Скопировать' });
  b.addEventListener('click', async () => {
    await navigator.clipboard.writeText(text).catch(() => {});
    b.textContent = 'Скопировано';
    setTimeout(() => (b.textContent = 'Скопировать'), 1500);
  });
  return b;
}

function render() {
  const tabs = el('div', { class: 'tabs', role: 'tablist' }, HELPER_FILES.map((g) =>
    el('button', { role: 'tab', 'aria-selected': String(tab === g.os), onclick: () => { tab = g.os; render(); } },
      g.name, mine?.os === g.os && el('span', { class: 'yours', text: 'ваша' }))));

  const g = HELPER_FILES.find((x) => x.os === tab);
  const myFile = mine?.os === tab ? g.variants.find((v) => v.arch === mine.arch)?.file : null;
  const files = el('div', { class: 'files' }, g.variants.map((v) =>
    el('div', { class: 'file' + (v.file === myFile ? ' mine' : '') },
      el('div', {}, el('b', { text: v.label }), el('small', { text: v.file + (v.file === myFile ? ' — подходит для этого компьютера' : '') })),
      el('a', { class: 'btn' + (v.file === myFile ? ' primary' : ''), href: DOWNLOAD_BASE + v.file, download: v.file, text: 'Скачать' }))));

  const steps = installSteps(tab, myFile || g.variants[0].file);
  const panel = el('div', { class: 'panel', role: 'tabpanel' },
    el('div', {}, el('h2', { text: '1. Скачайте файл' }), files),
    el('div', {}, el('h2', { text: '2. Запустите его' }), el('ol', {}, steps.map((t) => el('li', { text: t })))));

  if (tab !== 'windows') {
    panel.append(el('div', {},
      el('h2', { text: 'Или одной командой' }),
      el('p', { class: 'note', text: 'Вставьте в Терминал — команда сама скачает нужный файл и установит помощник. На macOS так не нужно разрешать запуск в настройках.' }),
      el('div', { class: 'cmd-row' }, el('code', { class: 'cmd', text: INSTALL_CMD }), copyButton(INSTALL_CMD))));
  } else {
    panel.append(el('p', { class: 'note', text: 'Предупреждение SmartScreen появляется потому, что у файла нет платной цифровой подписи. Помощник ставится в вашу папку профиля и не требует прав администратора.' }));
  }

  const uninstall = {
    windows: '"%LOCALAPPDATA%\\VlessBridge\\vlessbridge.exe" uninstall',
    darwin: '~/Library/Application\\ Support/VlessBridge/vlessbridge uninstall',
    linux: '~/.local/share/vlessbridge/vlessbridge uninstall',
  }[tab];
  app.replaceChildren(tabs, panel, el('div', { class: 'foot' },
    el('span', {}, 'Удалить помощник проще всего кнопкой в настройках расширения → «Обновления» → «Удалить помощник с компьютера». Вручную: ', el('code', { text: uninstall }), tab === 'windows' ? ' в командной строке (Win+R → cmd).' : ' в Терминале.'),
    el('a', { href: INSTALL_DOC, target: '_blank', text: 'Та же инструкция на GitHub' })));
}

(async () => {
  const p = await ext.runtime.getPlatformInfo();
  const d = helperDownload(p);
  if (d) { mine = { os: d.os, arch: d.arch }; tab = d.os; }
  const hash = location.hash.slice(1);
  if (HELPER_FILES.some((g) => g.os === hash)) tab = hash;
  render();
  const r = await send('state').catch(() => null);
  host = r?.ok ? r.result.host : null;
  renderStatus();
})();
