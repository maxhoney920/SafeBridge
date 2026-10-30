import { ext, isFirefox, action, sessionStore } from './compat.js';
import { MIN_HELPER, versionLess } from './common.js';
import { HOST_NAME, DEFAULTS, load, save, allServers, buildPac, normalizeDomain, domainMatches, ACCOUNT, LATEST_URL, DOWNLOAD_BASE } from './common.js';

// ---------- связь с помощником ----------

let port = null;
let hostInfo = null;   // ответ hello
let hostError = '';    // последняя ошибка подключения к помощнику
let connecting = null;
let seq = 0;
const pending = new Map();

let core = { running: false, port: 0, selected: '' };
let coreError = '';
let busy = { refresh: false, ping: false, updates: false, updateCore: false, updateHelper: false, account: false };
let helperRestarting = false; // помощник сам перезапускается после обновления

function isMissingHost(msg) {
  return /not found|не найден|No such native application/i.test(msg || '');
}

function connect() {
  if (port && hostInfo) return Promise.resolve(true);
  if (connecting) return connecting;
  connecting = new Promise((resolve) => {
    try {
      port = ext.runtime.connectNative(HOST_NAME);
    } catch (e) {
      port = null;
      hostError = e.message;
      resolve(false);
      return;
    }
    port.onMessage.addListener(onHostMessage);
    port.onDisconnect.addListener((p) => {
      const msg = p?.error?.message || ext.runtime.lastError?.message || 'Помощник завершил работу';
      port = null;
      hostInfo = null;
      hostError = msg;
      for (const p of pending.values()) p.reject(new Error(msg));
      pending.clear();
      onCoreDown(msg);
    });
    call('hello', {}, 30000)
      .then((info) => {
        hostInfo = info;
        onHostVersion(info);
        hostError = '';
        core = info.status || core;
        resolve(true);
      })
      .catch((e) => {
        hostError = e.message;
        resolve(false);
      });
  }).finally(() => { connecting = null; });
  return connecting;
}

function call(cmd, args = {}, timeout = 60000) {
  if (!port) return Promise.reject(new Error(hostError || 'Помощник не подключён'));
  const id = ++seq;
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      pending.delete(id);
      reject(new Error('Помощник не ответил вовремя'));
    }, timeout);
    pending.set(id, {
      resolve: (v) => { clearTimeout(timer); resolve(v); },
      reject: (e) => { clearTimeout(timer); reject(e); },
    });
    port.postMessage({ id, cmd, args });
  });
}

function onHostMessage(m) {
  if (m.event === 'core_exit') {
    onCoreDown(m.data?.message || 'Ядро остановилось', m.data?.log);
    return;
  }
  const p = pending.get(m.id);
  if (!p) return;
  pending.delete(m.id);
  if (m.ok) p.resolve(m.result);
  else p.reject(new Error(m.error || 'Ошибка помощника'));
}

let restarts = [];

async function onCoreDown(msg) {
  core = { ...core, running: false };
  if (helperRestarting) {
    await applyProxy();
    return;
  }
  coreError = 'Соединение прервалось: ' + msg;
  await applyProxy();
  notify();
  // автоматический перезапуск: не больше 3 попыток за 2 минуты
  const st = await load();
  const now = Date.now();
  restarts = restarts.filter((t) => now - t < 120000);
  if (st.enabled && restarts.length < 3) {
    restarts.push(now);
    setTimeout(() => { if (!core.running) applyCore(); }, 1500 * restarts.length);
  }
}

// Серверы разбирает помощник. Если он сменился (обновление или переустановка),
// подписки перечитываются, чтобы новые возможности (например, XHTTP) сразу появились.
async function onHostVersion(info) {
  const key = `${info.version}/${info.core}`;
  const { hostKey } = await ext.storage.local.get('hostKey');
  if (hostKey === key) return;
  await ext.storage.local.set({ hostKey: key });
  const st = await load();
  if (st.subs.length) setTimeout(() => refreshSubs().catch(() => {}), 500);
}

// ---------- ядро и прокси ----------

async function applyCore() {
  if (!(await connect())) {
    await applyProxy();
    return;
  }
  const st = await load();
  const servers = allServers(st).filter((s) => s.outbound);
  if (!servers.length) {
    await call('stop').catch(() => {});
    core = { running: false, port: 0, selected: '' };
    coreError = '';
    await applyProxy();
    return;
  }
  let selected = st.selected;
  if (!servers.some((s) => s.id === selected)) {
    selected = servers[0].id;
    await save({ selected });
  }
  try {
    core = await call('apply', {
      servers: servers.map((s) => ({ id: s.id, outbound: s.outbound })),
      selected,
      preferredPort: st.port || 0,
    }, 30000);
    coreError = '';
  } catch (e) {
    coreError = e.message;
    core = { ...core, running: false };
  }
  await applyProxy();
}

// Firefox не умеет PAC из строки так, как Chrome, зато даёт решать по каждому запросу.
let ffRule = null; // { port, mode, list } — пока прокси включён

function isLocalHost(h) {
  return !h.includes('.') || h === 'localhost' || h === '[::1]' || h.endsWith('.local') ||
    /^(127|10)\./.test(h) || /^192\.168\./.test(h) || /^172\.(1[6-9]|2\d|3[01])\./.test(h) || /^169\.254\./.test(h);
}

if (isFirefox) {
  ext.proxy.onRequest.addListener((info) => {
    let host;
    try { host = new URL(info.url).hostname.toLowerCase(); } catch { return { type: 'direct' }; }
    noteHost(info.tabId, host, info.type === 'main_frame');
    const r = ffRule;
    if (!r) return { type: 'direct' };
    if (isLocalHost(host)) return { type: 'direct' };
    const listed = !!domainMatches(host, r.list);
    const via = r.mode === 'include' ? listed : r.mode === 'exclude' ? !listed : true;
    // запасного варианта нет: если ядро недоступно, запрос не уйдёт «напрямую» мимо VPN
    return via ? { type: 'socks', host: '127.0.0.1', port: r.port, proxyDNS: true, failoverTimeout: 1 } : { type: 'direct' };
  }, { urls: ['<all_urls>'] });
  ext.proxy.onError.addListener((e) => console.warn('proxy:', e?.message || e));
}

// Адреса, к которым обращалась каждая вкладка (только Firefox: proxy.onRequest видит все
// запросы, включая неудачные). В Chrome то же берём из журнала загрузки страницы.
const tabHosts = new Map();
function noteHost(tabId, host, reset) {
  if (tabId == null || tabId < 0) return;
  let set = tabHosts.get(tabId);
  if (!set || reset) tabHosts.set(tabId, (set = new Set()));
  if (set.size < 500) set.add(host);
}
ext.tabs.onRemoved?.addListener((id) => tabHosts.delete(id));

// Журнал загрузки страницы: какие адреса она запрашивала (картинки, видео, API).
function pageResourceHosts() {
  const hosts = new Set();
  for (const e of performance.getEntriesByType('resource')) {
    try { hosts.add(new URL(e.name).hostname); } catch {}
  }
  return [...hosts];
}

async function pageHosts(tabId) {
  const hosts = new Set(tabHosts.get(tabId) || []);
  try {
    let results;
    if (ext.scripting?.executeScript) {
      const run = (allFrames) => ext.scripting.executeScript({ target: { tabId, allFrames }, func: pageResourceHosts });
      // во фреймы чужих сайтов без разрешения не попасть — тогда читаем только саму страницу
      results = (await run(true).catch(() => run(false))).map((r) => r.result);
    } else {
      results = await ext.tabs.executeScript(tabId, { allFrames: true, code: `(${pageResourceHosts})()` });
    }
    for (const list of results || []) for (const h of list || []) hosts.add(h);
  } catch {} // служебные страницы и магазин расширений читать нельзя — это нормально
  return [...hosts];
}

// Порт, на котором заведомо никто не слушает: прокси «в никуда» блокирует сайты,
// когда VPN включён, но соединения с ним нет (kill switch).
const BLOCK_PORT = 1;

async function applyProxy() {
  const st = await load();
  const active = st.enabled && core.running && core.port > 0;
  const blocked = st.enabled && !active && st.killSwitch;
  const port = active ? core.port : BLOCK_PORT;
  try {
    if (isFirefox) {
      ffRule = active || blocked ? { port, mode: st.mode, list: st.mode === 'include' ? st.include : st.mode === 'exclude' ? st.exclude : [] } : null;
    } else if (active || blocked) {
      await ext.proxy.settings.set({
        value: { mode: 'pac_script', pacScript: { data: buildPac(st, port, active ? core.selected : 'blocked') } },
        scope: 'regular',
      });
    } else {
      await ext.proxy.settings.clear({ scope: 'regular' });
    }
    if ((active || blocked) && st.webrtc && st.mode === 'all') {
      await ext.privacy.network.webRTCIPHandlingPolicy.set({ value: 'disable_non_proxied_udp' });
    } else {
      await ext.privacy.network.webRTCIPHandlingPolicy.clear({});
    }
  } catch (e) {
    console.warn('proxy settings:', e);
  }
  await updateBadge(st, active);
  notify();
}

async function updateBadge(st, active) {
  const problem = st.enabled && !active;
  await action.setIcon({
    path: active
      ? { 16: '/icons/on-16.png', 32: '/icons/on-32.png' }
      : { 16: '/icons/off-16.png', 32: '/icons/off-32.png' },
  });
  await action.setBadgeText({ text: problem ? '!' : '' });
  if (problem) await action.setBadgeBackgroundColor({ color: '#C23B3B' });
  const sel = allServers(st).find((s) => s.id === core.selected);
  await action.setTitle({
    title: active ? `SafeBridge: подключено${sel ? ' — ' + sel.name : ''}` : problem ? 'SafeBridge: нет соединения' : 'SafeBridge: выключено',
  });
}

function notify() {
  ext.runtime.sendMessage({ type: 'changed' }).catch(() => {});
}

// ---------- подписки ----------

let refreshing = null;
async function refreshSubs(onlyId) {
  if (busy.refresh) {
    // полное обновление уже идёт; одну новую подписку загрузим сразу после него
    if (!onlyId) return refreshing;
    await refreshing;
    if (busy.refresh) return refreshing;
  }
  refreshing = refreshOnce(onlyId);
  return refreshing;
}

async function refreshOnce(onlyId) {
  busy.refresh = true;
  notify();
  try {
    if (!(await connect())) return;
    const st = await load();
    const subs = st.subs.map((s) => ({ ...s }));
    await Promise.all(subs.filter((s) => !onlyId || s.id === onlyId).map(async (sub) => {
      try {
        let r;
        try {
          r = await call('fetch', { url: sub.url, ua: sub.ua || '' }, 90000);
        } catch (e) {
          // у ключей из аккаунта бывает запасной домен — на случай, если основной заблокирован
          if (!sub.mirror) throw e;
          r = await call('fetch', { url: sub.mirror, ua: sub.ua || '' }, 90000);
        }
        sub.servers = r.servers;
        sub.info = r.info;
        sub.updatedAt = Date.now();
        sub.error = '';
        // у ключей из аккаунта имя — тариф («safe-internet: Месяц»), а не общее имя из подписки
        if (!sub.customName && !sub.account && r.info?.title) sub.name = r.info.title;
      } catch (e) {
        sub.error = e.message;
        sub.errorAt = Date.now();
      }
    }));
    await save({ subs });
    await applyCore();
  } finally {
    busy.refresh = false;
    notify();
  }
}

async function scheduleRefresh() {
  const st = await load();
  const period = Math.max(15, st.intervalMin || DEFAULTS.intervalMin);
  const a = await ext.alarms.get('refresh');
  if (!a || a.periodInMinutes !== period) {
    await ext.alarms.create('refresh', { periodInMinutes: period, delayInMinutes: period });
  }
  return period;
}

// ---------- аккаунт VPN-сервиса (вход через Google на сайте) ----------
// Вход: открываем {сайт}/ext/login?nonce=…, сайт ведёт на Google и в конце на
// {сайт}/ext/done#token=…&nonce=…, где скрипт account-done.js передаёт токен сюда.
// Ключи берём из {сайт}/api/ext/keys и держим подписки из аккаунта в актуальном виде.

// Другой адрес сайта — только локальный, для проверки: токен аккаунта не должен уйти
// на чужой сайт, даже если кто-то подменит настройки расширения.
const accountSite = (st) => {
  const alt = st.accountSite || '';
  return (/^http:\/\/(127\.0\.0\.1|localhost)(:\d+)?$/.test(alt) ? alt : ACCOUNT.site).replace(/\/+$/, '');
};

function randomNonce() {
  const b = crypto.getRandomValues(new Uint8Array(24));
  return btoa(String.fromCharCode(...b)).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

async function accountFetch(st, path, init = {}) {
  const r = await fetch(accountSite(st) + path, {
    ...init,
    cache: 'no-store',
    credentials: 'omit',
    headers: { Authorization: 'Bearer ' + st.account.token, Accept: 'application/json' },
  });
  if (r.status === 401) {
    const e = new Error('Вход устарел — войдите снова');
    e.expired = true;
    throw e;
  }
  if (!r.ok) {
    let detail = '';
    try { detail = (await r.json()).detail || ''; } catch {}
    throw new Error(detail || `Сайт ответил ${r.status}`);
  }
  return r.json();
}

const isoToSec = (iso) => (iso ? Math.floor(Date.parse(iso) / 1000) || 0 : 0);

// Ключ аккаунта по-разному: id из бота, а если его нет — ссылка
const keyRef = (k) => (k.id != null ? String(k.id) : k.url);

// Приводит подписки из аккаунта к списку ключей account.allKeys без выключенных
// пользователем (account.off). Возвращает id добавленных подписок.
function reconcileAccountSubs(st) {
  const off = new Set(st.account?.off || []);
  const wanted = (st.account?.allKeys || []).filter((k) => !off.has(keyRef(k)));
  const urls = new Set(wanted.map((k) => k.url));
  // ключи, которых больше нет (истекли, удалены в боте или выключены), убираем
  const subs = st.subs.filter((s) => !s.account || urls.has(s.url));
  const added = [];
  for (const k of wanted) {
    const fields = { account: true, keyId: k.id, mirror: k.mirror || '', expireAt: k.expireAt || 0, plan: k.plan || '' };
    const cur = subs.find((s) => s.url === k.url);
    if (cur) {
      Object.assign(cur, fields);
      if (!cur.customName && k.plan) cur.name = `${ACCOUNT.name}: ${k.plan}`;
    } else {
      const id = 'sub' + Date.now().toString(36) + Math.random().toString(36).slice(2, 6);
      subs.push({ id, url: k.url, name: `${ACCOUNT.name}${k.plan ? ': ' + k.plan : ''}`, customName: false, servers: [], ...fields });
      added.push(id);
    }
  }
  const changed = added.length > 0 || subs.length !== st.subs.length;
  return { subs, added, changed };
}

// Сверяет подписки из аккаунта с ключами на сайте. true — состав подписок изменился.
async function syncAccount() {
  let st = await load();
  if (!st.account?.token) return false;
  busy.account = true;
  notify();
  try {
    const data = await accountFetch(st, '/api/ext/keys');
    st = await load(); // за время запроса могли что-то поменять
    const allKeys = (data.keys || []).filter((k) => /^https?:\/\//i.test(k.subscription_url || '')).map((k) => ({
      id: k.id, url: k.subscription_url, mirror: k.mirror_url || '', plan: k.plan_label || '', expireAt: isoToSec(k.expire_at),
    }));
    // выключенные ключи, которых больше нет на сайте, забываем
    const refs = new Set(allKeys.map(keyRef));
    const off = (st.account.off || []).filter((r) => refs.has(r));
    const account = { ...st.account, name: data.account?.name || st.account.name || '', email: data.account?.email || st.account.email || '',
      renewUrl: data.renew_url || ACCOUNT.bot, syncedAt: Date.now(), allKeys, off, keys: allKeys.length - off.length, error: '', expired: false };
    const { subs, changed } = reconcileAccountSubs({ ...st, account });
    await save({ subs, account });
    return changed;
  } catch (e) {
    st = await load();
    if (st.account) await save({ account: { ...st.account, error: e.message, expired: !!e.expired, errorAt: Date.now() } });
    return false;
  } finally {
    busy.account = false;
    notify();
  }
}

// Включить или выключить один ключ аккаунта
async function setAccountKey(ref, on) {
  const st = await load();
  if (!st.account) throw new Error('Вы не вошли в аккаунт');
  const off = new Set(st.account.off || []);
  if (on) off.delete(ref); else off.add(ref);
  const account = { ...st.account, off: [...off] };
  account.keys = (account.allKeys || []).filter((k) => !off.has(keyRef(k))).length;
  const { subs, added } = reconcileAccountSubs({ ...st, account });
  await save({ subs, account });
  if (added.length) for (const id of added) await refreshSubs(id);
  else await applyCore();
}

// Плановое обновление: сначала ключи из аккаунта, затем сами подписки.
async function refreshAll() {
  await syncAccount();
  await refreshSubs();
}

ext.alarms.onAlarm.addListener((a) => {
  if (a.name === 'refresh') refreshAll();
  if (a.name === 'updates') checkUpdates(true).catch(() => {});
});

// ---------- обновления помощника и ядра ----------

const UPDATE_PERIOD_MIN = 24 * 60;

// Новая версия самого расширения. Только Chrome и другие Chromium: Firefox обновляет
// подписанное дополнение сам. Версию берём из того же latest.json, что и помощник.
async function checkExtension() {
  if (isFirefox) return null;
  const current = ext.runtime.getManifest().version;
  try {
    const r = await fetch(`${LATEST_URL}?t=${Date.now()}`, { cache: 'no-store', credentials: 'omit' });
    if (!r.ok) throw new Error(`GitHub ответил ${r.status}`);
    const li = await r.json();
    const latest = li.extension?.version || '';
    const name = li.extension?.name || 'safebridge-extension.zip';
    if (!/^[\w.-]+\.zip$/.test(name)) throw new Error('Неожиданное имя файла');
    return { current, latest, available: !!latest && versionLess(current, latest), url: DOWNLOAD_BASE + name, checkedAt: Date.now() };
  } catch (e) {
    return { current, error: e.message, checkedAt: Date.now() };
  }
}

async function checkUpdates(auto = false) {
  if (busy.updates) return (await load()).updates;
  busy.updates = true;
  notify();
  try {
    const extension = await checkExtension();
    if (extension) await save({ updates: { ...((await load()).updates || {}), extension } });
    if (hostInfo && versionLess(hostInfo.version, MIN_HELPER)) return (await load()).updates; // старый помощник не умеет
    if (!(await connect())) throw new Error(hostError || 'Помощник не подключён');
    const info = await call('checkUpdates', {}, 90000);
    const updates = { ...info, extension: extension || undefined, checkedAt: Date.now() };
    await save({ updates });
    const st = await load();
    if (auto && st.autoCore && info.core?.available) await updateCore().catch(() => {});
    return (await load()).updates;
  } finally {
    busy.updates = false;
    notify();
  }
}

async function updateCore() {
  if (busy.updateCore) return;
  busy.updateCore = true;
  notify();
  try {
    if (!(await connect())) throw new Error(hostError || 'Помощник не подключён');
    const res = await call('updateCore', {}, 11 * 60000);
    const st = await load();
    const updates = st.updates ? { ...st.updates } : { core: {}, helper: {} };
    updates.core = { ...updates.core, current: res.version, available: false, lastResult: res.updated ? `обновлено с ${res.previous} до ${res.version}` : '' };
    await save({ updates });
    if (hostInfo) hostInfo = { ...hostInfo, core: res.version };
    core = (await call('status')).status;
    return res;
  } finally {
    busy.updateCore = false;
    notify();
  }
}

async function updateHelper() {
  if (busy.updateHelper) return;
  busy.updateHelper = true;
  helperRestarting = true;
  notify();
  try {
    if (!(await connect())) throw new Error(hostError || 'Помощник не подключён');
    const res = await call('updateHelper', {}, 11 * 60000);
    if (res.updated) {
      // помощник завершится сам; ждём и подключаемся к новой версии
      for (let i = 0; i < 20 && port; i++) await new Promise((r) => setTimeout(r, 250));
      await new Promise((r) => setTimeout(r, 300));
      await connect();
      await applyCore();
      const st = await load();
      if (st.updates?.helper) {
        await save({ updates: { ...st.updates, helper: { ...st.updates.helper, current: hostInfo?.version, available: false } } });
      }
    }
    return res;
  } finally {
    helperRestarting = false;
    busy.updateHelper = false;
    notify();
  }
}

async function scheduleUpdates() {
  const a = await ext.alarms.get('updates');
  if (!a) await ext.alarms.create('updates', { periodInMinutes: UPDATE_PERIOD_MIN, delayInMinutes: 5 });
  const st = await load();
  if (!st.updates?.checkedAt || Date.now() - st.updates.checkedAt > UPDATE_PERIOD_MIN * 60000) {
    setTimeout(() => checkUpdates(true).catch(() => {}), 20000);
  }
}

// ---------- пинг ----------

async function pingAll() {
  if (busy.ping) return;
  busy.ping = true;
  notify();
  try {
    if (!core.running) await applyCore();
    const st = await load();
    const ids = allServers(st).filter((s) => s.outbound).map((s) => s.id);
    const res = await call('ping', { ids, url: st.pingUrl, timeoutMs: 5000 }, 120000);
    await sessionStore.set({ pings: res, pingedAt: Date.now() });
  } finally {
    busy.ping = false;
    notify();
  }
}

// ---------- запуск ----------

let ready = null;

async function init() {
  await scheduleRefresh();
  await scheduleUpdates();
  const st = await load();
  await applyCore();
  const period = Math.max(15, st.intervalMin) * 60000;
  if (st.subs.some((s) => !s.updatedAt || Date.now() - s.updatedAt > period)) refreshSubs();
}

ext.runtime.onStartup.addListener(() => {});
ext.runtime.onInstalled.addListener(({ reason }) => {
  if (reason === 'install') ext.runtime.openOptionsPage?.();
});
ready = init().catch((e) => console.warn('init:', e));

// ---------- сообщения от popup и настроек ----------

async function getState() {
  const st = await load();
  const { pings = {}, pingedAt = 0 } = await sessionStore.get(['pings', 'pingedAt']);
  let control = 'controllable_by_this_extension';
  if (!isFirefox) {
    try {
      control = (await ext.proxy.settings.get({})).levelOfControl;
    } catch {}
  }
  return {
    ...st,
    // токен аккаунта нужен только фону — окнам расширения его не отдаём
    account: st.account ? { ...st.account, token: undefined } : null,
    host: hostInfo,
    hostError,
    hostMissing: !hostInfo && isMissingHost(hostError),
    core,
    coreError,
    busy,
    pings,
    pingedAt,
    proxyControl: control,
  };
}

const handlers = {
  async state() {
    await ready;
    if (connecting) await connecting;
    if (!hostInfo) await connect().then((ok) => ok && !core.running && applyCore());
    return getState();
  },

  async retryHost() {
    hostError = '';
    // переподключаемся заново: после переустановки браузер запустит уже новый файл помощника
    if (port) {
      const old = port;
      port = null;
      hostInfo = null;
      for (const p of pending.values()) p.reject(new Error('Переподключение'));
      pending.clear();
      try { old.disconnect(); } catch {}
      await new Promise((r) => setTimeout(r, 600));
    }
    await connect();
    await applyCore();
    return getState();
  },

  async toggle({ on }) {
    await save({ enabled: !!on });
    if (on && !core.running) await applyCore();
    else await applyProxy();
    return getState();
  },

  async select({ id }) {
    await save({ selected: id });
    if (core.running) {
      try {
        core = await call('select', { id });
        coreError = '';
      } catch (e) {
        await applyCore();
      }
      await applyProxy();
    } else {
      await applyCore();
    }
    return getState();
  },

  async refresh({ subId }) {
    if (subId) await refreshSubs(subId);
    else await refreshAll();
    return getState();
  },

  // ----- аккаунт -----
  async accountLogin() {
    const st = await load();
    const nonce = randomNonce();
    await sessionStore.set({ loginNonce: nonce, loginAt: Date.now() });
    await save({ account: st.account ? { ...st.account, error: '' } : null, accountError: '' });
    await ext.tabs.create({ url: `${accountSite(st)}/ext/login?nonce=${encodeURIComponent(nonce)}` });
    return getState();
  },

  // Пришло со страницы {сайт}/ext/done (скрипт account-done.js)
  async accountDone({ params }, sender) {
    const st = await load();
    const site = accountSite(st);
    if (!sender?.url || !sender.url.startsWith(site + '/ext/done')) throw new Error('Чужая страница');
    const { loginNonce, loginAt = 0 } = await sessionStore.get(['loginNonce', 'loginAt']);
    // вход начат именно этим расширением и не позже 15 минут назад
    if (!loginNonce || params.nonce !== loginNonce || Date.now() - loginAt > 15 * 60 * 1000) {
      return { ok: false, message: 'Вход устарел. Нажмите «Войти» в окне SafeBridge ещё раз.' };
    }
    await sessionStore.set({ loginNonce: '' });
    if (params.error || !params.token) {
      await save({ accountError: params.error || 'invalid' });
      return { ok: false };
    }
    await save({ account: { token: params.token, email: params.email || '', at: Date.now() }, accountError: '' });
    await syncAccount();
    refreshSubs(); // серверы догрузятся в фоне — страницу входа не держим
    const after = await load();
    if (sender.tab?.id != null) setTimeout(() => ext.tabs.remove(sender.tab.id).catch(() => {}), 2500);
    return { ok: true, keys: after.account?.keys || 0 };
  },

  async accountSync() {
    if (await syncAccount()) await refreshSubs();
    return getState();
  },

  async accountSetKey({ ref, on }) {
    await setAccountKey(String(ref), !!on);
    return getState();
  },

  async accountLogout() {
    const st = await load();
    if (st.account?.token) {
      await accountFetch(st, '/api/ext/logout', { method: 'POST' }).catch(() => {}); // отзываем токен на сайте
    }
    const after = await load();
    await save({ account: null, accountError: '', subs: after.subs.filter((s) => !s.account) });
    await applyCore();
    return getState();
  },

  async ping() {
    await pingAll();
    return getState();
  },

  async addSub({ url, name }) {
    url = String(url || '').trim();
    if (!/^https?:\/\/\S+$/i.test(url)) throw new Error('Ссылка подписки должна начинаться с http:// или https://');
    const st = await load();
    if (st.subs.some((s) => s.url === url)) throw new Error('Эта подписка уже добавлена');
    const sub = { id: 'sub' + Date.now().toString(36), url, name: name || new URL(url).hostname, customName: !!name, servers: [] };
    await save({ subs: [...st.subs, sub] });
    await refreshSubs(sub.id);
    const after = (await load()).subs.find((s) => s.id === sub.id);
    if (after?.error && !after.servers?.length) {
      // не сохраняем подписку, которая ни разу не загрузилась
      await save({ subs: (await load()).subs.filter((s) => s.id !== sub.id) });
      throw new Error(after.error);
    }
    return getState();
  },

  async removeSub({ id }) {
    const st = await load();
    const sub = st.subs.find((s) => s.id === id);
    if (sub?.account && st.account) {
      // ключ из аккаунта не удаляем, а выключаем — иначе он вернулся бы при следующей сверке
      const k = (st.account.allKeys || []).find((x) => x.url === sub.url);
      await setAccountKey(k ? keyRef(k) : sub.url, false);
      return getState();
    }
    await save({ subs: st.subs.filter((s) => s.id !== id) });
    await applyCore();
    return getState();
  },

  async updateSub({ id, patch }) {
    const st = await load();
    // менять можно только имя: адрес, серверы и признак «из аккаунта» правит сам фон
    const name = String(patch?.name ?? '').trim().slice(0, 100);
    if (!name) throw new Error('Пустое имя');
    const subs = st.subs.map((s) => (s.id === id ? { ...s, name, customName: true } : s));
    await save({ subs });
    return getState();
  },

  async setMode({ mode }) {
    if (!['all', 'include', 'exclude'].includes(mode)) throw new Error('Неизвестный режим');
    await save({ mode });
    await applyProxy();
    return getState();
  },

  // domain — один сайт, domains — сразу несколько (сайт и всё, что он загружает)
  async addSite({ list, domain, domains }) {
    const add = [...new Set([domain, ...(domains || [])].filter(Boolean).map(normalizeDomain).filter(Boolean))];
    if (!add.length) throw new Error('Не похоже на адрес сайта');
    const st = await load();
    const key = list === 'exclude' ? 'exclude' : 'include';
    const fresh = add.filter((d) => !st[key].includes(d));
    if (fresh.length) await save({ [key]: [...st[key], ...fresh] });
    await applyProxy();
    return getState();
  },

  async removeSite({ list, domain, domains }) {
    const st = await load();
    const key = list === 'exclude' ? 'exclude' : 'include';
    const drop = new Set([domain, ...(domains || [])].filter(Boolean));
    await save({ [key]: st[key].filter((x) => !drop.has(x)) });
    await applyProxy();
    return getState();
  },

  async pageHosts({ tabId }) {
    return tabId == null ? [] : pageHosts(tabId);
  },

  async setSites({ list, domains }) {
    const key = list === 'exclude' ? 'exclude' : 'include';
    const clean = [...new Set((domains || []).map(normalizeDomain).filter(Boolean))];
    await save({ [key]: clean });
    await applyProxy();
    return getState();
  },

  async saveSettings({ settings }) {
    const patch = {};
    if (settings.intervalMin != null) patch.intervalMin = Math.min(24 * 60, Math.max(15, Number(settings.intervalMin) || 120));
    if (settings.pingUrl != null) {
      const u = String(settings.pingUrl).trim();
      if (!/^https:\/\//.test(u)) throw new Error('Адрес для проверки пинга должен начинаться с https://');
      patch.pingUrl = u;
    }
    if (settings.webrtc != null) patch.webrtc = !!settings.webrtc;
    if (settings.killSwitch != null) patch.killSwitch = !!settings.killSwitch;
    if (settings.port != null) {
      const p = Number(settings.port) || 0;
      if (p && (p < 1024 || p > 65535)) throw new Error('Порт должен быть от 1024 до 65535 или 0');
      patch.port = p;
    }
    const before = await load();
    await save(patch);
    await scheduleRefresh();
    if (patch.port != null && patch.port !== before.port) await applyCore();
    else await applyProxy();
    return getState();
  },

  async checkUpdates() {
    await checkUpdates(false);
    return getState();
  },

  async updateCore() {
    await updateCore();
    return getState();
  },

  async updateHelper() {
    await updateHelper();
    return getState();
  },

  async uninstallHelper() {
    if (!(await connect())) throw new Error(hostError || 'Помощник не подключён');
    helperRestarting = true; // не перезапускать ядро, когда помощник завершится
    try {
      await call('uninstall', {}, 30000);
      for (let i = 0; i < 20 && port; i++) await new Promise((r) => setTimeout(r, 150));
    } finally {
      port = null;
      hostInfo = null;
      hostError = 'not found';
      core = { running: false, port: 0, selected: '' };
      coreError = '';
      helperRestarting = false;
    }
    await save({ enabled: false, updates: null });
    await ext.storage.local.remove('hostKey');
    await applyProxy();
    return getState();
  },

  async setAutoCore({ on }) {
    await save({ autoCore: !!on });
    return getState();
  },

  async diag() {
    if (!(await connect())) throw new Error(hostError || 'Помощник не подключён');
    return call('status');
  },
};

// Сообщения принимаем только от самого расширения. Его страницы (окно, настройки) могут
// всё, а скрипт на странице сайта аккаунта (account-done.js) — только сообщить итог входа.
const fromContentScript = new Set(['accountDone']);
const extOrigin = ext.runtime.getURL('');
ext.runtime.onMessage.addListener((msg, sender, sendResponse) => {
  const h = handlers[msg?.type];
  if (!h) return false;
  const ownPage = sender?.id === ext.runtime.id && typeof sender.url === 'string' && sender.url.startsWith(extOrigin);
  if (sender?.id !== ext.runtime.id || (!ownPage && !fromContentScript.has(msg.type))) {
    sendResponse({ ok: false, error: 'Нет доступа' });
    return false;
  }
  h(msg, sender)
    .then((result) => sendResponse({ ok: true, result }))
    .catch((e) => sendResponse({ ok: false, error: e.message }));
  return true;
});
