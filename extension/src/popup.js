import { el, send, allServers, protoLabel, fmtBytes, fmtAgo, fmtDate, domainMatches, normalizeDomain, DOWNLOAD_BASE, RELEASES_URL } from './common.js';

const app = document.getElementById('app');
let state = null;
let tabHost = '';
let actionError = '';
let pendingAction = '';
let wasActive = null;

document.getElementById('openOptions').addEventListener('click', () => chrome.runtime.openOptionsPage());

async function act(type, data = {}, label = type) {
  pendingAction = label;
  actionError = '';
  render();
  try {
    const r = await send(type, data);
    if (!r?.ok) throw new Error(r?.error || 'Нет ответа от расширения');
    state = r.result;
  } catch (e) {
    actionError = e.message;
  } finally {
    pendingAction = '';
    render();
  }
}

async function reload() {
  const r = await send('state').catch(() => null);
  if (r?.ok) state = r.result;
  render();
}

chrome.runtime.onMessage.addListener((m) => {
  if (m?.type === 'changed' && !pendingAction) {
    clearTimeout(reload.t);
    reload.t = setTimeout(reload, 80);
  }
});

function pingNode(ms) {
  if (ms == null) return el('span', { class: 'ping muted', text: '' });
  if (ms < 0) return el('span', { class: 'ping bad', text: 'нет ответа' });
  return el('span', { class: 'ping ' + (ms < 300 ? 'good' : ms < 800 ? 'mid' : 'bad'), text: ms + ' мс' });
}

// ---------- экраны ----------

function viewMissingHost() {
  const box = el('div', { class: 'empty' },
    el('h1', { text: 'Установите помощник' }),
    el('p', { text: 'Расширение подключается к серверам через небольшую программу на компьютере. Её нужно установить один раз, права администратора не понадобятся.' }),
  );
  const dl = el('div', { class: 'row' });
  box.append(dl);
  chrome.runtime.getPlatformInfo().then((p) => {
    const os = { win: 'windows', mac: 'darwin', linux: 'linux' }[p.os];
    const arch = p.arch === 'arm64' || p.arch === 'arm' ? 'arm64' : 'amd64';
    let steps;
    if (os === 'windows') {
      dl.append(el('a', { class: 'btn primary', href: `${DOWNLOAD_BASE}vlessbridge-windows-${arch}.exe`, target: '_blank', text: 'Скачать для Windows' }));
      steps = [
        'Запустите скачанный файл. Если Windows предупредит о неизвестном издателе, нажмите «Подробнее», затем «Выполнить в любом случае».',
        'Вернитесь сюда и нажмите «Проверить».',
      ];
    } else {
      const cmd = `curl -fsSL ${DOWNLOAD_BASE}install.sh | sh`;
      const copyBtn = el('button', { class: 'btn primary', text: 'Скопировать команду' });
      copyBtn.addEventListener('click', async () => {
        await navigator.clipboard.writeText(cmd).catch(() => {});
        copyBtn.textContent = 'Скопировано';
      });
      dl.append(copyBtn);
      box.append(el('code', { class: 'cmd', text: cmd }));
      steps = [
        os === 'darwin' ? 'Откройте Терминал (Программы → Утилиты), вставьте команду и нажмите Return.' : 'Откройте терминал, вставьте команду и нажмите Enter.',
        'Вернитесь сюда и нажмите «Проверить».',
      ];
    }
    dl.append(el('a', { class: 'btn', href: RELEASES_URL, target: '_blank', text: 'Все версии' }));
    box.append(el('ol', { class: 'small' }, steps.map((t) => el('li', { text: t }))),
      el('div', {}, el('button', { class: 'btn', onclick: () => act('retryHost', {}, 'retry'), disabled: pendingAction === 'retry' },
        pendingAction === 'retry' ? el('span', { class: 'spinner' }) : 'Проверить')));
    if (state.hostError && !state.hostMissing) box.append(el('div', { class: 'notice', text: state.hostError }));
  });
  return box;
}

function viewNoSubs() {
  const input = el('input', { class: 'input', type: 'url', placeholder: 'https://…/sub/…', 'aria-label': 'Ссылка на подписку' });
  const add = () => act('addSub', { url: input.value }, 'addSub');
  input.addEventListener('keydown', (e) => e.key === 'Enter' && add());
  setTimeout(() => input.focus(), 0);
  return el('div', { class: 'empty' },
    el('h1', { text: 'Добавьте подписку' }),
    el('p', { text: 'Вставьте ссылку на подписку из личного кабинета или бота вашего VPN. Серверы загрузятся автоматически и будут обновляться каждые 2 часа.' }),
    el('div', { class: 'row' }, input,
      el('button', { class: 'btn primary', onclick: add, disabled: pendingAction === 'addSub' },
        pendingAction === 'addSub' ? el('span', { class: 'spinner' }) : 'Добавить')),
    actionError && el('div', { class: 'notice', text: actionError }),
  );
}

function bridgeCard(servers) {
  const sel = servers.find((s) => s.id === (state.core.selected || state.selected)) || servers.find((s) => s.id === state.selected);
  const active = state.enabled && state.core.running;
  const problem = state.enabled && !state.core.running;
  const cls = 'bridge' + (active ? ' on' : problem ? ' problem' : '') + (active && wasActive === false ? ' just-on' : '');
  wasActive = active;
  const stateText = pendingAction === 'toggle' ? (state.enabled ? 'Отключаю…' : 'Подключаю…')
    : active ? 'Подключено' : problem ? 'Нет соединения' : 'Выключено';
  const ms = sel ? state.pings[sel.id] : null;
  return el('button', {
    class: cls, role: 'switch', 'aria-checked': String(!!state.enabled),
    disabled: pendingAction === 'toggle',
    onclick: () => act('toggle', { on: !state.enabled }, 'toggle'),
  },
    svgArch(),
    el('span', { class: 'state', text: stateText }),
    el('span', { class: 'server', text: sel ? `${sel.name}${ms > 0 ? `   ${ms} мс` : ''}` : 'Сервер не выбран' }),
    el('span', { class: 'knob', 'aria-hidden': 'true' }),
  );
}

function svgArch() {
  const ns = 'http://www.w3.org/2000/svg';
  const svg = document.createElementNS(ns, 'svg');
  svg.setAttribute('viewBox', '0 0 150 84');
  svg.setAttribute('class', 'arch');
  svg.setAttribute('aria-hidden', 'true');
  const p = document.createElementNS(ns, 'path');
  p.setAttribute('d', 'M8 60 H142 M30 60 A45 45 0 0 1 120 60 M55 60 V36 M75 60 V22 M95 60 V36');
  p.setAttribute('fill', 'none');
  p.setAttribute('stroke', 'currentColor');
  p.setAttribute('stroke-width', '7');
  p.setAttribute('stroke-linecap', 'round');
  svg.append(p);
  return svg;
}

function modeSwitch() {
  const opts = [['all', 'Все сайты'], ['include', 'Только список'], ['exclude', 'Кроме списка']];
  return el('div', { class: 'segments', role: 'radiogroup', 'aria-label': 'Какие сайты открывать через VPN' },
    opts.map(([v, t]) => el('label', {},
      el('input', { type: 'radio', name: 'mode', value: v, checked: state.mode === v, onchange: () => act('setMode', { mode: v }, 'mode') }),
      el('span', { text: t }))));
}

function siteRow() {
  if (state.mode === 'all' || !tabHost) {
    if (state.mode === 'all') return null;
    return el('div', { class: 'site muted' }, el('span', { text: listSummary() }),
      el('button', { class: 'btn quiet', onclick: () => openOptions('sites'), text: 'Изменить список' }));
  }
  const listKey = state.mode;
  const match = domainMatches(tabHost, state[listKey]);
  const viaVpn = state.mode === 'include' ? !!match : !match;
  const domain = normalizeDomain(tabHost.replace(/^www\./, ''));
  let btn;
  if (match) {
    btn = el('button', { class: 'btn', onclick: () => act('removeSite', { list: listKey, domain: match }, 'site'),
      text: state.mode === 'include' ? 'Не через VPN' : 'Через VPN' });
  } else {
    btn = el('button', { class: 'btn', onclick: () => act('addSite', { list: listKey, domain }, 'site'),
      text: state.mode === 'include' ? 'Через VPN' : 'Напрямую' });
  }
  return el('div', { class: 'site' },
    el('div', { style: 'min-width:0' },
      el('div', { class: 'host', text: tabHost }),
      el('div', { class: 'route small' + (viaVpn ? ' via' : ''), text: viaVpn ? 'открывается через VPN' : 'открывается напрямую' })),
    btn);
}

function listSummary() {
  const n = state[state.mode]?.length || 0;
  const w = n % 10 === 1 && n % 100 !== 11 ? 'сайт' : [2, 3, 4].includes(n % 10) && ![12, 13, 14].includes(n % 100) ? 'сайта' : 'сайтов';
  return n ? `В списке ${n} ${w}` : 'Список сайтов пуст';
}

function serverList(servers) {
  const box = el('div', { class: 'servers', role: 'listbox', 'aria-label': 'Серверы' });
  const multi = state.subs.length > 1;
  const current = state.core.selected || state.selected;
  for (const sub of state.subs) {
    if (multi) box.append(el('div', { class: 'group', text: sub.name }));
    for (const s of sub.servers || []) {
      const ok = !!s.outbound;
      box.append(el('button', {
        class: 'srv' + (s.id === current ? ' sel' : ''),
        role: 'option', 'aria-selected': String(s.id === current),
        disabled: !ok, title: ok ? `${s.address}:${s.port}` : s.unsupported,
        onclick: () => s.id !== current && act('select', { id: s.id }, 'select'),
      },
        el('span', { class: 'name', text: s.name }),
        el('span', { class: 'proto', text: ok ? protoLabel(s) : (s.unsupported || 'не поддерживается') }),
        pingNode(state.pings[s.id])));
    }
  }
  return box;
}

function footer() {
  const box = el('div', { class: 'foot' });
  for (const sub of state.subs) {
    const i = sub.info || {};
    const parts = [];
    if (i.total > 0) {
      const used = (i.upload || 0) + (i.download || 0);
      box.append(el('div', { text: `${state.subs.length > 1 ? sub.name + ': ' : ''}${fmtBytes(used)} из ${fmtBytes(i.total)}` }),
        el('div', { class: 'meter' }, el('i', { style: `width:${Math.min(100, (used / i.total) * 100).toFixed(1)}%` })));
    } else if (state.subs.length > 1) parts.push(sub.name);
    if (i.expire > 0) parts.push(`до ${fmtDate(i.expire)}`);
    parts.push(fmtAgo(sub.updatedAt));
    box.append(el('div', { text: parts.join(', ') }));
    if (sub.error) box.append(el('div', { class: 'err', text: `Не удалось обновить: ${sub.error}` }));
  }
  return box;
}

function viewMain() {
  const servers = allServers(state);
  const nodes = [bridgeCard(servers)];
  if (state.enabled && !state.core.running && state.coreError) nodes.push(el('div', { class: 'notice', text: state.coreError }));
  if (state.proxyControl === 'controlled_by_other_extensions') {
    nodes.push(el('div', { class: 'notice', text: 'Настройками прокси управляет другое расширение. Отключите его, чтобы VlessBridge заработал.' }));
  }
  if (actionError) nodes.push(el('div', { class: 'notice', text: actionError }));
  nodes.push(modeSwitch());
  const sr = siteRow();
  if (sr) nodes.push(sr);
  const busyRefresh = state.busy.refresh || pendingAction === 'refresh';
  const busyPing = state.busy.ping || pendingAction === 'ping';
  nodes.push(el('div', { class: 'section-head' },
    el('h2', { text: 'Серверы' }),
    el('button', { class: 'btn quiet', onclick: () => act('ping', {}, 'ping'), disabled: busyPing },
      busyPing ? el('span', { class: 'spinner' }) : 'Пинг'),
    el('button', { class: 'btn quiet', onclick: () => act('refresh', {}, 'refresh'), disabled: busyRefresh, title: 'Обновить подписку сейчас' },
      busyRefresh ? el('span', { class: 'spinner' }) : 'Обновить')));
  nodes.push(serverList(servers));
  nodes.push(footer());
  return nodes;
}

function openOptions(hash) {
  chrome.tabs.create({ url: chrome.runtime.getURL('options.html#' + hash) });
}

function render() {
  if (!state) return;
  let content;
  if (!state.host) content = viewMissingHost();
  else if (!state.subs.length) content = viewNoSubs();
  else content = viewMain();
  const focused = document.activeElement?.closest?.('#app') ? document.activeElement : null;
  const keepInput = focused?.tagName === 'INPUT' && focused.type === 'url' ? focused.value : null;
  app.replaceChildren(...[content].flat());
  if (keepInput != null) {
    const i = app.querySelector('input[type=url]');
    if (i) { i.value = keepInput; i.focus(); }
  }
}

(async () => {
  try {
    const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
    const u = new URL(tab?.url || '');
    if (u.protocol === 'http:' || u.protocol === 'https:') tabHost = u.hostname;
  } catch {}
  await reload();
})();
