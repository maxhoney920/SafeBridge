import { HOST_NAME, DEFAULTS, load, save, allServers, buildPac, normalizeDomain } from './common.js';

// ---------- связь с помощником ----------

let port = null;
let hostInfo = null;   // ответ hello
let hostError = '';    // последняя ошибка подключения к помощнику
let connecting = null;
let seq = 0;
const pending = new Map();

let core = { running: false, port: 0, selected: '' };
let coreError = '';
let busy = { refresh: false, ping: false };

function isMissingHost(msg) {
  return /not found|не найден/i.test(msg || '');
}

function connect() {
  if (port && hostInfo) return Promise.resolve(true);
  if (connecting) return connecting;
  connecting = new Promise((resolve) => {
    try {
      port = chrome.runtime.connectNative(HOST_NAME);
    } catch (e) {
      port = null;
      hostError = e.message;
      resolve(false);
      return;
    }
    port.onMessage.addListener(onHostMessage);
    port.onDisconnect.addListener(() => {
      const msg = chrome.runtime.lastError?.message || 'Помощник завершил работу';
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

async function applyProxy() {
  const st = await load();
  const active = st.enabled && core.running && core.port > 0;
  try {
    if (active) {
      await chrome.proxy.settings.set({
        value: { mode: 'pac_script', pacScript: { data: buildPac(st, core.port) } },
        scope: 'regular',
      });
      if (st.webrtc && st.mode === 'all') {
        await chrome.privacy.network.webRTCIPHandlingPolicy.set({ value: 'disable_non_proxied_udp' });
      } else {
        await chrome.privacy.network.webRTCIPHandlingPolicy.clear({});
      }
    } else {
      await chrome.proxy.settings.clear({ scope: 'regular' });
      await chrome.privacy.network.webRTCIPHandlingPolicy.clear({});
    }
  } catch (e) {
    console.warn('proxy settings:', e);
  }
  await updateBadge(st, active);
  notify();
}

async function updateBadge(st, active) {
  const problem = st.enabled && !active;
  await chrome.action.setIcon({
    path: active
      ? { 16: '/icons/on-16.png', 32: '/icons/on-32.png' }
      : { 16: '/icons/off-16.png', 32: '/icons/off-32.png' },
  });
  await chrome.action.setBadgeText({ text: problem ? '!' : '' });
  if (problem) await chrome.action.setBadgeBackgroundColor({ color: '#C23B3B' });
  const sel = allServers(st).find((s) => s.id === core.selected);
  await chrome.action.setTitle({
    title: active ? `VlessBridge: подключено${sel ? ' — ' + sel.name : ''}` : problem ? 'VlessBridge: нет соединения' : 'VlessBridge: выключено',
  });
}

function notify() {
  chrome.runtime.sendMessage({ type: 'changed' }).catch(() => {});
}

// ---------- подписки ----------

async function refreshSubs(onlyId) {
  if (busy.refresh) return;
  busy.refresh = true;
  notify();
  try {
    if (!(await connect())) return;
    const st = await load();
    const subs = st.subs.map((s) => ({ ...s }));
    await Promise.all(subs.filter((s) => !onlyId || s.id === onlyId).map(async (sub) => {
      try {
        const r = await call('fetch', { url: sub.url, ua: sub.ua || '' }, 90000);
        sub.servers = r.servers;
        sub.info = r.info;
        sub.updatedAt = Date.now();
        sub.error = '';
        if (!sub.customName && r.info?.title) sub.name = r.info.title;
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
  const a = await chrome.alarms.get('refresh');
  if (!a || a.periodInMinutes !== period) {
    await chrome.alarms.create('refresh', { periodInMinutes: period, delayInMinutes: period });
  }
  return period;
}

chrome.alarms.onAlarm.addListener((a) => {
  if (a.name === 'refresh') refreshSubs();
});

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
    await chrome.storage.session.set({ pings: res, pingedAt: Date.now() });
  } finally {
    busy.ping = false;
    notify();
  }
}

// ---------- запуск ----------

let ready = null;

async function init() {
  await scheduleRefresh();
  const st = await load();
  await applyCore();
  const period = Math.max(15, st.intervalMin) * 60000;
  if (st.subs.some((s) => !s.updatedAt || Date.now() - s.updatedAt > period)) refreshSubs();
}

chrome.runtime.onStartup.addListener(() => {});
chrome.runtime.onInstalled.addListener(({ reason }) => {
  if (reason === 'install') chrome.runtime.openOptionsPage?.();
});
ready = init().catch((e) => console.warn('init:', e));

// ---------- сообщения от popup и настроек ----------

async function getState() {
  const st = await load();
  const { pings = {}, pingedAt = 0 } = await chrome.storage.session.get(['pings', 'pingedAt']);
  let control = 'controllable_by_this_extension';
  try {
    control = (await chrome.proxy.settings.get({})).levelOfControl;
  } catch {}
  return {
    ...st,
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
    await refreshSubs(subId);
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
    await save({ subs: st.subs.filter((s) => s.id !== id) });
    await applyCore();
    return getState();
  },

  async updateSub({ id, patch }) {
    const st = await load();
    const subs = st.subs.map((s) => (s.id === id ? { ...s, ...patch, customName: patch.name ? true : s.customName } : s));
    await save({ subs });
    return getState();
  },

  async setMode({ mode }) {
    if (!['all', 'include', 'exclude'].includes(mode)) throw new Error('Неизвестный режим');
    await save({ mode });
    await applyProxy();
    return getState();
  },

  async addSite({ list, domain }) {
    const d = normalizeDomain(domain);
    if (!d) throw new Error('Не похоже на адрес сайта');
    const st = await load();
    const key = list === 'exclude' ? 'exclude' : 'include';
    if (!st[key].includes(d)) await save({ [key]: [...st[key], d] });
    await applyProxy();
    return getState();
  },

  async removeSite({ list, domain }) {
    const st = await load();
    const key = list === 'exclude' ? 'exclude' : 'include';
    await save({ [key]: st[key].filter((x) => x !== domain) });
    await applyProxy();
    return getState();
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

  async diag() {
    if (!(await connect())) throw new Error(hostError || 'Помощник не подключён');
    return call('status');
  },
};

chrome.runtime.onMessage.addListener((msg, sender, sendResponse) => {
  const h = handlers[msg?.type];
  if (!h) return false;
  h(msg)
    .then((result) => sendResponse({ ok: true, result }))
    .catch((e) => sendResponse({ ok: false, error: e.message }));
  return true;
});
