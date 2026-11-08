import { ext, isFirefox, isApp } from './compat.js';
import { el, send, allServers, protoLabel, fmtBytes, fmtAgo, fmtDate, domainMatches, siteDomain, plural, helperDownload, detectOldWindows, installSteps, madeBy, MIN_HELPER, versionLess, MODES, LIST_NAMES, presetFor, presetsOf, listsSummary, relatedDomains, ACCOUNT, ACCOUNT_ERRORS, expiringSubs, fmtLeft, subExpire } from './common.js';

const app = document.getElementById('app');
let state = null;
let tabHost = '';
let tabId = null;
let pageHostList = []; // адреса, которые загружала открытая страница
let actionError = '';
let pendingAction = '';
let wasActive = null;

document.getElementById('openOptions').addEventListener('click', () => ext.runtime.openOptionsPage());

async function act(type, data = {}, label = type) {
  pendingAction = label;
  actionError = '';
  render();
  try {
    const r = await send(type, data);
    if (!r?.ok) throw new Error(r?.error || (isApp ? 'Нет ответа от приложения' : 'Нет ответа от расширения'));
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

ext.runtime.onMessage.addListener((m) => {
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
  Promise.all([ext.runtime.getPlatformInfo(), detectOldWindows()]).then(([p, oldWin]) => {
    const d = helperDownload(p, oldWin);
    const row = el('div', { class: 'row' });
    if (d) row.append(el('a', { class: 'btn primary', href: d.url, download: d.file, text: `Скачать для ${d.arch.startsWith('win7') ? (d.arch === 'win7-386' ? 'Windows (32 бита)' : 'Windows 7') : d.name}` }));
    row.append(el('button', { class: 'btn', onclick: () => openPage('install.html'), text: d ? 'Другие системы' : 'Выбрать систему' }));
    box.append(row);
    if (d) box.append(el('ol', { class: 'small' }, installSteps(d.os, d.file).map((t) => el('li', { text: t }))));
    box.append(el('div', { class: 'row' },
      el('button', { class: 'btn', onclick: () => act('retryHost', {}, 'retry'), disabled: pendingAction === 'retry' },
        pendingAction === 'retry' ? el('span', { class: 'spinner' }) : 'Проверить'),
      el('button', { class: 'btn quiet', onclick: () => openPage('install.html'), text: 'Подробная инструкция' })));
    if (state.hostError && !state.hostMissing) box.append(el('div', { class: 'notice', text: state.hostError }));
  });
  return box;
}

const openUrl = (url) => ext.tabs.create({ url });

// Ошибка последнего входа в аккаунт (не привязан Google и т. п.)
function accountErrorBox() {
  const code = state.accountError;
  if (!code) return null;
  return el('div', { class: 'notice' }, ACCOUNT_ERRORS[code] || ACCOUNT_ERRORS.invalid,
    code === 'not_linked' && el('div', { class: 'notice-actions' },
      el('button', { class: 'btn quiet', onclick: () => openUrl(ACCOUNT.bot), text: 'Открыть бота' }),
      el('button', { class: 'btn quiet', onclick: () => openUrl(ACCOUNT.cabinet), text: 'Личный кабинет' })));
}

function loginButton(label = 'Войти через Google') {
  return el('button', { class: 'btn primary login', onclick: () => act('accountLogin', {}, 'login') },
    el('span', { class: 'g', 'aria-hidden': 'true', text: 'G' }), label);
}

function viewNoSubs() {
  const input = el('input', { class: 'input', type: 'url', placeholder: 'https://…/sub/…', 'aria-label': 'Ссылка на подписку' });
  const add = () => act('addSub', { url: input.value }, 'addSub');
  input.addEventListener('keydown', (e) => e.key === 'Enter' && add());
  const acc = state.account;
  let top;
  if (acc && !acc.expired) {
    const syncing = state.busy.account || pendingAction === 'accountSync';
    top = el('div', { class: 'account-box' },
      el('p', {}, 'Вы вошли в ', el('b', { text: ACCOUNT.name }), acc.email ? ` как ${acc.email}` : '', '.'),
      el('p', { class: 'muted', text: acc.error ? `Не удалось получить ключи: ${acc.error}`
        : acc.allKeys?.length ? 'Все ключи аккаунта выключены. Включите нужные в настройках → «Подписки».'
        : 'Активных ключей пока нет. Получите или продлите ключ в боте — он появится здесь сам.' }),
      el('div', { class: 'row' },
        el('button', { class: 'btn primary', onclick: () => openUrl(acc.renewUrl || ACCOUNT.bot), text: 'Открыть бота' }),
        el('button', { class: 'btn', onclick: () => openUrl(ACCOUNT.cabinet), text: 'Личный кабинет' }),
        el('button', { class: 'btn', disabled: syncing, onclick: () => act('accountSync', {}, 'accountSync') }, syncing ? el('span', { class: 'spinner' }) : 'Проверить снова'),
        el('button', { class: 'btn quiet', onclick: () => act('accountLogout', {}, 'logout'), text: 'Выйти' })));
  } else {
    top = el('div', { class: 'account-box' },
      el('p', {}, 'Клиентам ', el('b', { text: ACCOUNT.name }), ': войдите через Google — ваши ключи добавятся сами и будут обновляться, когда вы покупаете или продлеваете ключ в боте.'),
      acc?.expired && el('p', { class: 'muted', text: 'Вход устарел — войдите ещё раз.' }),
      loginButton(),
      accountErrorBox());
  }
  return el('div', { class: 'empty' },
    el('h1', { text: 'Добавьте подписку' }),
    top,
    el('div', { class: 'or', text: 'или вставьте ссылку вручную' }),
    el('p', { class: 'small muted', text: 'Ссылка на подписку из личного кабинета или бота любого VPN-сервиса. Серверы загрузятся сами и будут обновляться каждые 2 часа.' }),
    el('div', { class: 'row' }, input,
      el('button', { class: 'btn', onclick: add, disabled: pendingAction === 'addSub' },
        pendingAction === 'addSub' ? el('span', { class: 'spinner' }) : 'Добавить')),
    actionError && el('div', { class: 'notice', text: actionError }),
  );
}

// Предупреждения аккаунта в основном окне: вход устарел, подписка заканчивается.
function accountBanners() {
  const out = [];
  const acc = state.account;
  if (acc?.expired) {
    out.push(el('div', { class: 'update warn' },
      el('span', { text: `Вход в ${ACCOUNT.name} устарел — новые ключи не подтянутся.` }),
      el('button', { class: 'btn quiet', onclick: () => act('accountLogin', {}, 'login'), text: 'Войти' })));
  }
  const app = isApp && state.updates?.app;
  if (app?.available) {
    out.push(el('div', { class: 'update' },
      el('span', { text: `Вышла новая версия SafeBridge ${app.latest}.` }),
      app.canUpdate
        ? el('button', { class: 'btn quiet', disabled: pendingAction === 'appUpdate', onclick: () => act('appUpdate', {}, 'appUpdate'), text: pendingAction === 'appUpdate' ? 'Обновляю…' : 'Обновить' })
        : el('button', { class: 'btn quiet', onclick: () => openUrl(app.page), text: 'Скачать' })));
  }
  if (state.listsUpdate) {
    out.push(el('div', { class: 'update' },
      el('span', { text: `Новые списки сайтов: ${listsSummary(state.listsUpdate.changes)}.` }),
      el('button', { class: 'btn quiet', disabled: pendingAction === 'lists', onclick: () => act('listsApply', {}, 'lists'), text: 'Обновить списки' })));
  }
  const exp = expiringSubs(state)[0];
  if (exp) {
    const own = exp.sub.account;
    out.push(el('div', { class: 'update warn' },
      el('span', { text: `Подписка «${exp.sub.name}» ${fmtLeft(exp.left)}.` }),
      own && el('button', { class: 'btn quiet', onclick: () => openUrl(acc?.renewUrl || ACCOUNT.bot), text: 'Продлить' })));
  }
  return out;
}

function bridgeCard(servers) {
  const sel = servers.find((s) => s.id === (state.core.selected || state.selected)) || servers.find((s) => s.id === state.selected);
  const active = state.enabled && state.core.running;
  const problem = state.enabled && !state.core.running;
  const cls = 'bridge' + (active ? ' on' : problem ? ' problem' : '') + (active && wasActive === false ? ' just-on' : '');
  wasActive = active;
  const stateText = pendingAction === 'toggle' ? (state.enabled ? 'Отключаю…' : 'Подключаю…')
    : active ? 'Подключено' : problem ? (state.killSwitch ? 'Нет соединения — сайты заблокированы' : 'Нет соединения — сайты идут напрямую') : 'Выключено';
  const ms = sel ? state.pings[sel.id] : null;
  return el('button', {
    class: cls, role: 'switch', 'aria-checked': String(!!state.enabled),
    disabled: pendingAction === 'toggle',
    onclick: () => act('toggle', { on: !state.enabled }, 'toggle'),
  },
    shieldBadge(),
    el('span', { class: 'state', text: stateText }),
    el('span', { class: 'server', text: sel ? `${sel.name}${ms > 0 ? `   ${ms} мс` : ''}` : 'Сервер не выбран' }),
    active && pendingAction !== 'toggle' && connLine(),
    el('span', { class: 'knob', 'aria-hidden': 'true' }),
  );
}

// Приложение для Windows: системный прокси или весь трафик (TUN) — две кнопки над подключением
let tunConfirm = false;
function appModeSwitch() {
  const tun = !!state.appTun && !!ext.elevated;
  const pick = async (on) => {
    if (on === tun && !(on && !ext.elevated)) return;
    if (on && !ext.elevated) { tunConfirm = true; render(); return; }
    tunConfirm = false;
    pendingAction = 'appMode';
    render();
    try {
      await ext.storage.local.set({ appTun: on });
      await new Promise((r) => setTimeout(r, 600)); // ядро перезапускается с новым режимом
    } finally {
      pendingAction = '';
      reload();
    }
  };
  const relaunch = async () => {
    tunConfirm = false;
    pendingAction = 'appMode';
    render();
    try {
      await ext.storage.local.set({ appTun: true });
      await new Promise((r) => setTimeout(r, 400)); // настройка успевает записаться
      await ext.bridge.call('RelaunchElevated');
    } catch (e) {
      actionError = e.message;
    } finally {
      pendingAction = '';
      render();
    }
  };
  const btn = (on, text, title) => el('button', {
    class: 'app-mode-btn' + (tun === on ? ' cur' : ''), 'aria-pressed': String(tun === on), title,
    disabled: pendingAction === 'appMode', onclick: () => pick(on), text,
  });
  return el('div', { class: 'app-mode' },
    el('div', { class: 'app-mode-btns', role: 'group', 'aria-label': 'Как пускать трафик через VPN' },
      btn(false, 'Системный прокси', 'Браузеры и программы, которые берут прокси из настроек Windows. Без прав администратора.'),
      btn(true, 'Весь трафик (TUN)', 'Весь компьютер, включая игры и мессенджеры. Нужны права администратора.')),
    tunConfirm && el('div', { class: 'app-mode-ask' },
      el('span', { text: 'Для режима «Весь трафик» SafeBridge перезапустится с правами администратора — Windows спросит разрешение.' }),
      el('div', { class: 'row' },
        el('button', { class: 'btn primary', onclick: relaunch, text: 'Перезапустить' }),
        el('button', { class: 'btn quiet', onclick: () => { tunConfirm = false; render(); }, text: 'Отмена' }))),
    !tunConfirm && state.appTun && !ext.elevated && el('div', { class: 'muted small', text: 'Выбран «Весь трафик», но нет прав администратора — работает системный прокси.' }));
}

// IP, с которым сайты видят пользователя, и сколько времени подключены к серверу
function connLine() {
  const c = state.conn;
  if (!c) return null;
  const where = c.ip ? `IP ${c.ip}${c.country ? ' · ' + countryName(c.country) : ''}`
    : c.ipError ? (/неизвестная команда/.test(c.ipError) ? 'IP — после обновления помощника' : 'IP не определён') : 'IP определяется…';
  return el('span', { class: 'conn' },
    el('span', { class: 'conn-ip', text: where, title: !c.ip && c.ipError ? c.ipError : null }),
    el('span', { class: 'conn-time', 'data-since': String(c.since), title: 'Время подключения', text: fmtElapsed(Date.now() - c.since) }));
}

function countryName(code) {
  try {
    return new Intl.DisplayNames(['ru'], { type: 'region' }).of(code) || code;
  } catch {
    return code;
  }
}

function fmtElapsed(ms) {
  const t = Math.max(0, Math.floor(ms / 1000));
  const h = Math.floor(t / 3600), m = Math.floor((t % 3600) / 60), sec = t % 60;
  const p = (n) => String(n).padStart(2, '0');
  const d = Math.floor(h / 24);
  return d > 0 ? `${d} д ${p(h % 24)}:${p(m)}:${p(sec)}` : `${p(h)}:${p(m)}:${p(sec)}`;
}

setInterval(() => {
  for (const n of document.querySelectorAll('.conn-time')) n.textContent = fmtElapsed(Date.now() - Number(n.getAttribute('data-since')));
}, 500);

// Щит из логотипа SafeBridge: серый — выключено, янтарный — подключено
function shieldBadge() {
  const ns = 'http://www.w3.org/2000/svg';
  const wrap = el('span', { class: 'shield', 'aria-hidden': 'true' });
  const svg = document.createElementNS(ns, 'svg');
  svg.setAttribute('viewBox', '0 0 100 100');
  for (const [d, cls] of [
    ['M50 8 L86 20 C86 54 72 78 50 92 C28 78 14 54 14 20 Z', 's-left'],
    ['M50 8 L86 20 C86 54 72 78 50 92 Z', 's-right'],
    ['M33 44 L50 66 L70 34 L59 34 L50 50 L42 42 Z', 's-cut'],
  ]) {
    const p = document.createElementNS(ns, 'path');
    p.setAttribute('d', d);
    p.setAttribute('class', cls);
    svg.append(p);
  }
  wrap.append(svg);
  return wrap;
}

function modeSwitch() {
  return el('div', { class: 'segments', role: 'radiogroup', 'aria-label': 'Какие сайты открывать через VPN' },
    Object.entries(MODES).map(([v, m]) => el('label', {},
      el('input', { type: 'radio', name: 'mode', value: v, checked: state.mode === v, onchange: () => act('setMode', { mode: v }, 'mode') }),
      el('span', { text: m.label }))));
}

function modeHint() {
  const box = el('div', { class: 'mode-hint' }, el('span', { text: MODES[state.mode].hint }));
  if (state.mode !== 'all') {
    const n = state[state.mode].length;
    box.append(' ', el('a', { href: '#', onclick: (e) => { e.preventDefault(); openOptions('sites'); },
      text: n ? `В списке ${n} ${plural(n, 'сайт', 'сайта', 'сайтов')}` : 'Список пока пуст — добавить сайты' }));
  }
  return box;
}

// Строка про открытую вкладку: как она сейчас открывается и кнопка, чтобы это изменить.
function siteRow() {
  if (state.mode === 'all' || !tabHost) return null;
  const key = state.mode;
  const match = domainMatches(tabHost, state[key]);
  const viaVpn = key === 'include' ? !!match : !match;
  const list = LIST_NAMES[key];
  // У больших сервисов видео и картинки лежат на других доменах (у YouTube — googlevideo.com,
  // ytimg.com), поэтому добавляем и убираем весь набор сервиса, а не один домен.
  const preset = presetFor(tabHost, key, presetsOf(state));
  const addSet = preset ? preset.domains : [siteDomain(tabHost)];
  const dropSet = preset && preset.domains.includes(match) ? preset.domains : [match];
  const add = () => act('addSite', { list: key, domains: addSet }, 'site');
  const drop = () => act('removeSite', { list: key, domains: dropSet }, 'site');
  const addTitle = preset ? `Добавить ${preset.name} (${addSet.join(', ')}) в список ${list}` : `Добавить ${addSet[0]} в список ${list}`;
  const dropTitle = `Убрать ${dropSet.join(', ')} из списка ${list}`;
  let reason, btn;
  if (key === 'include') {
    reason = match ? `в списке: ${preset && dropSet.length > 1 ? preset.name : match}` : 'нет в списке';
    btn = match
      ? el('button', { class: 'btn', title: dropTitle, onclick: drop, text: 'Убрать из списка' })
      : el('button', { class: 'btn primary', title: addTitle, onclick: add, text: 'Открывать через VPN' });
  } else {
    reason = match ? `в исключениях: ${preset && dropSet.length > 1 ? preset.name : match}` : 'нет в исключениях';
    btn = match
      ? el('button', { class: 'btn', title: dropTitle, onclick: drop, text: 'Открывать через VPN' })
      : el('button', { class: 'btn', title: addTitle, onclick: add, text: 'Открывать без VPN' });
  }
  return [el('div', { class: 'site' },
    el('div', { class: 'site-text' },
      el('div', { class: 'host', text: tabHost }),
      el('div', { class: 'route small' + (viaVpn ? ' via' : '') },
        el('b', { text: viaVpn ? 'Через VPN' : 'Без VPN' }), ` — ${reason}`)),
    btn), key === 'include' && match ? relatedRow() : null];
}

// В режиме «Только список»: сайт идёт через VPN, но его видео, картинки или API
// грузятся с других доменов — и они пока идут напрямую.
function relatedRow() {
  const rel = relatedDomains(pageHostList, tabHost, state.include);
  if (!rel.length) return null;
  const shown = rel.slice(0, 5);
  const rest = rel.length - shown.length;
  return el('div', { class: 'related' },
    el('div', { class: 'small' },
      el('b', { text: `Ещё ${rel.length} ${plural(rel.length, 'сайт', 'сайта', 'сайтов')} мимо VPN. ` }),
      'Эта страница загружает с них данные — если видео или картинки не открываются, добавьте их:'),
    el('div', { class: 'related-list small', title: rel.join('\n') },
      shown.join(', ') + (rest > 0 ? ` и ещё ${rest}` : '')),
    el('div', { class: 'related-actions' },
      el('button', { class: 'btn', text: 'Добавить все', onclick: async () => {
        await act('addSite', { list: 'include', domains: rel }, 'site');
        if (tabId != null) ext.tabs.reload(tabId).catch(() => {});
      } }),
      el('span', { class: 'muted small', text: 'страница обновится сама' })));
}

// Новая версия расширения (Chrome): ведём в настройки, там архив и как заменить папку
function extUpdateBanner() {
  const e = state.updates?.extension;
  if (!e?.available) return null;
  return el('div', { class: 'update' },
    el('span', { text: `Вышла SafeBridge ${e.latest}` }),
    el('button', { class: 'btn quiet', onclick: () => { ext.tabs.create({ url: ext.runtime.getURL('options.html#helper') }); }, text: 'Обновить' }));
}

function updateBanner() {
  if (state.host && versionLess(state.host.version, MIN_HELPER)) {
    return el('div', { class: 'update warn' },
      el('span', { text: `Помощник ${state.host.version} устарел: он не умеет XHTTP и обновляться сам. Установите новый — старый заменится.` }),
      el('button', { class: 'btn quiet', onclick: () => openPage('install.html') }, 'Установить'));
  }
  const h = state.updates?.helper;
  if (!h?.available) return null;
  const busy = state.busy.updateHelper || pendingAction === 'updateHelper';
  return el('div', { class: 'update' },
    el('span', { text: busy ? 'Обновляю помощник…' : `Вышла новая версия помощника ${h.latest}` }),
    el('button', { class: 'btn quiet', disabled: busy, onclick: () => act('updateHelper', {}, 'updateHelper') },
      busy ? el('span', { class: 'spinner' }) : 'Обновить'));
}

// Заголовок группы серверов: имя подписки, трафик, срок и время обновления
function subHeader(sub) {
  const i = sub.info || {};
  const parts = [];
  let meter = null;
  if (i.total > 0) {
    const used = (i.upload || 0) + (i.download || 0);
    parts.push(`${fmtBytes(used)} из ${fmtBytes(i.total)}`);
    const pct = Math.min(100, (used / i.total) * 100);
    meter = el('div', { class: 'meter' + (pct >= 90 ? ' full' : '') }, el('i', { style: `width:${pct.toFixed(1)}%` }));
  } else if (i.upload || i.download) {
    parts.push(`${fmtBytes((i.upload || 0) + (i.download || 0))}, без лимита`);
  }
  const exp = subExpire(sub);
  const soon = expiringSubs({ subs: [sub] })[0];
  if (exp > 0) {
    const d = new Date(exp * 1000);
    const short = d.toLocaleDateString('ru-RU', { day: 'numeric', month: 'long', ...(d.getFullYear() !== new Date().getFullYear() && { year: 'numeric' }) });
    parts.push(soon ? fmtLeft(soon.left) : `до ${short.replace(/\s*г\.$/, '')}`);
  }
  parts.push(fmtAgo(sub.updatedAt).replace(/^обновлена /, ''));
  return el('div', { class: 'group' },
    el('div', { class: 'group-name', text: sub.name }),
    el('div', { class: 'group-info' + (soon ? ' soon' : ''), text: parts.join(' · ') }),
    meter,
    sub.error && el('div', { class: 'err', text: `Не удалось обновить: ${sub.error}` }));
}

function serverList(servers) {
  const box = el('div', { class: 'servers', role: 'listbox', 'aria-label': 'Серверы' });
  const current = state.core.selected || state.selected;
  for (const sub of state.subs) {
    box.append(subHeader(sub));
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

function viewMain() {
  const servers = allServers(state);
  const nodes = [];
  if (isApp && ext.caps && !ext.caps.tunOnly) nodes.push(appModeSwitch());
  nodes.push(bridgeCard(servers));
  if (state.enabled && !state.core.running && state.coreError) nodes.push(el('div', { class: 'notice', text: state.coreError }));
  if (state.proxyControl === 'controlled_by_other_extensions') {
    nodes.push(el('div', { class: 'notice', text: 'Настройками прокси управляет другое расширение. Отключите его, чтобы SafeBridge заработал.' }));
  }
  if (actionError) nodes.push(el('div', { class: 'notice', text: actionError }));
  const ub = updateBanner();
  if (ub) nodes.push(ub);
  const eb = extUpdateBanner();
  if (eb) nodes.push(eb);
  nodes.push(...accountBanners());
  nodes.push(el('div', { class: 'mode' }, modeSwitch(), modeHint()));
  nodes.push(...[siteRow()].flat().filter(Boolean));
  const busyRefresh = state.busy.refresh || pendingAction === 'refresh';
  const busyPing = state.busy.ping || pendingAction === 'ping';
  nodes.push(el('div', { class: 'section-head' },
    el('h2', { text: 'Серверы' }),
    el('button', { class: 'btn quiet', onclick: () => act('ping', {}, 'ping'), disabled: busyPing },
      busyPing ? el('span', { class: 'spinner' }) : 'Пинг'),
    el('button', { class: 'btn quiet', onclick: () => act('refresh', {}, 'refresh'), disabled: busyRefresh, title: 'Обновить подписку сейчас' },
      busyRefresh ? el('span', { class: 'spinner' }) : 'Обновить')));
  nodes.push(serverList(servers));
  return nodes;
}

function openPage(path) {
  ext.tabs.create({ url: ext.runtime.getURL(path) });
}

function openOptions(hash) {
  ext.tabs.create({ url: ext.runtime.getURL('options.html#' + hash) });
}

function render() {
  if (!state) return;
  let content;
  if (!state.host) content = viewMissingHost();
  else if (!state.subs.length) content = viewNoSubs();
  else content = viewMain();
  const focused = document.activeElement?.closest?.('#app') ? document.activeElement : null;
  const keepInput = focused?.tagName === 'INPUT' && focused.type === 'url' ? focused.value : null;
  app.replaceChildren(...[content].flat(), madeBy());
  if (keepInput != null) {
    const i = app.querySelector('input[type=url]');
    if (i) { i.value = keepInput; i.focus(); }
  }
}

(async () => {
  try {
    let [tab] = await ext.tabs.query({ active: true, currentWindow: true });
    // ?tab=<адрес> — открыть окно расширения для конкретной страницы (удобно для проверки)
    const forced = new URLSearchParams(location.search).get('tab');
    if (forced) tab = (await ext.tabs.query({})).find((t) => t.url === forced) || { url: forced };
    const u = new URL(tab?.url || '');
    if (u.protocol === 'http:' || u.protocol === 'https:') tabHost = u.hostname;
    tabId = tab?.id ?? null;
  } catch {}
  await reload();
  if (tabHost && tabId != null) {
    const r = await send('pageHosts', { tabId }).catch(() => null);
    if (r?.ok && r.result.length) { pageHostList = r.result; render(); }
  }
})();
