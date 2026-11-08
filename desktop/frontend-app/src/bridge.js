// Мост к приложению на Go (Wails или режим разработки по HTTP) и объект `ext` с API,
// как у расширения. Создаётся один раз в окне-оболочке до загрузки остальных модулей
// (boot.js) — так модулям не нужен await на верхнем уровне, которого нет в старых
// WebKit (macOS High Sierra).

// macOS с Safari старше 13.1: встроенный скрипт Wails (связь окна с программой) написан
// с «?.» и не запускается — window.go не появляется. Тогда берём из него описание методов
// (window.wailsbindings) и подключаем его же копию с переведённым синтаксисом
// (src/wails-runtime.js, готовит tools/bundle при сборке).
async function ensureWailsRuntime() {
  const wailsPage = location.protocol === 'wails:' || location.hostname === 'wails.localhost';
  if (!wailsPage || (globalThis.go && globalThis.go.main && globalThis.go.main.App)) return;
  const text = await fetch('/wails/runtime.js').then((r) => r.text());
  const m = text.match(/^window\.wailsbindings='(?:[^'\\]|\\.)*';/);
  if (m) (0, eval)(m[0]);
  const copy = await fetch('src/wails-runtime.js').then((r) => r.text());
  (0, eval)(copy);
  if (!(globalThis.go && globalThis.go.main && globalThis.go.main.App)) {
    throw new Error('связь окна с программой не запустилась (встроенный скрипт Wails), Safari: ' + navigator.userAgent);
  }
}

function createBridge() {
  const wails = globalThis.go?.main?.App;
  if (wails) {
    return {
      // Wails отклоняет промис строкой, а интерфейс ждёт Error с текстом в message
      call: (method, ...args) => Promise.resolve(wails[method](...args)).catch((e) => {
        throw e instanceof Error ? e : new Error(typeof e === 'string' ? e : (e?.message || String(e)));
      }),
      on: (name, fn) => globalThis.runtime.EventsOn(name, fn),
    };
  }
  // режим разработки: go run -tags webdev .
  const listeners = {};
  const es = new EventSource('/api/events');
  es.onmessage = (e) => {
    const m = JSON.parse(e.data);
    (listeners[m.name] || []).forEach((fn) => fn(m.data));
  };
  return {
    async call(method, ...args) {
      const r = await fetch('/api/call', { method: 'POST', body: JSON.stringify({ method, args }) });
      const j = await r.json();
      if (j.error) throw new Error(j.error);
      return j.result;
    },
    on: (name, fn) => (listeners[name] = listeners[name] || []).push(fn),
  };
}

function listenerSet() {
  const set = new Set();
  return {
    set,
    addListener: (fn) => set.add(fn),
    removeListener: (fn) => set.delete(fn),
    hasListener: (fn) => set.has(fn),
  };
}

export async function createExt() {
  await ensureWailsRuntime();
  const bridge = createBridge();
  const origin = location.origin;
  const appVersion = await bridge.call('Version');
  const authPort = await bridge.call('AuthPort');
  const platform = await bridge.call('Platform');
  const elevated = await bridge.call('IsElevated');
  // service — ядро в службе (macOS); tunOnly — только «Весь трафик»; tray — значок в строке меню
  const caps = await bridge.call('Caps');
  const tunActive = () => !!data.enabled && (caps.tunOnly || (!!data.appTun && elevated));

  // ----- storage.local: настройки в файле приложения, в памяти — копия -----
  let data = {};
  try { data = JSON.parse(await bridge.call('StorageLoad')) || {}; } catch {}
  const changed = listenerSet();
  let saveTimer = null;
  const persist = () => {
    clearTimeout(saveTimer);
    saveTimer = setTimeout(() => { saveTimer = null; bridge.call('StorageSave', JSON.stringify(data)).catch((e) => console.warn('save', e)); }, 150);
  };
  const pick = (store, keys) => {
    if (keys == null) return { ...store };
    if (typeof keys === 'string') keys = [keys];
    if (Array.isArray(keys)) return Object.fromEntries(keys.filter((k) => k in store).map((k) => [k, structuredClone(store[k])]));
    return Object.fromEntries(Object.entries(keys).map(([k, d]) => [k, k in store ? structuredClone(store[k]) : d]));
  };
  // при закрытии окна дописываем отложенное сохранение
  globalThis.addEventListener('pagehide', () => {
    if (saveTimer) { clearTimeout(saveTimer); saveTimer = null; bridge.call('StorageSave', JSON.stringify(data)).catch(() => {}); }
  });
  const local = {
    async get(keys) { return pick(data, keys); },
    async set(obj) {
      const diff = {};
      for (const [k, v] of Object.entries(obj)) {
        diff[k] = { oldValue: data[k], newValue: v };
        data[k] = structuredClone(v);
      }
      persist();
      changed.set.forEach((fn) => fn(diff, 'local'));
    },
    async remove(keys) {
      for (const k of [].concat(keys)) delete data[k];
      persist();
    },
  };
  const mem = {};
  const session = {
    async get(keys) { return pick(mem, keys); },
    async set(obj) { Object.assign(mem, structuredClone(obj)); },
  };

  // ----- runtime: сообщения между «фоном» и страницами -----
  const onMessage = listenerSet();
  const ownSender = { id: 'safebridge-app', url: origin + '/popup.html' };
  function sendMessage(msg, sender = ownSender) {
    return new Promise((resolve) => {
      let async = false;
      let done = false;
      const respond = (v) => { if (!done) { done = true; resolve(v); } };
      for (const fn of onMessage.set) {
        try {
          if (fn(structuredClone(msg), sender, respond) === true) async = true;
        } catch (e) { console.warn(e); }
      }
      if (!async) queueMicrotask(() => respond(undefined));
    });
  }

  // ----- ядро вместо помощника: тот же протокол {id, cmd, args} -----
  // Связь одна на всё приложение: сообщения получает только последний «порт», а если ядро
  // (служба) отключилось — порт закрывается, как у расширения при выходе помощника.
  let curPort = null;
  bridge.on('native', (s) => {
    let m;
    try { m = JSON.parse(s); } catch { return; }
    curPort?.onMessage.set.forEach((fn) => fn(m));
  });
  bridge.on('native-down', (message) => {
    const p = curPort;
    curPort = null;
    p?.onDisconnect.set.forEach((fn) => fn({ error: { message } }));
  });
  function connectNative() {
    const port = {
      onMessage: listenerSet(),
      onDisconnect: listenerSet(),
      postMessage(m) {
        if (curPort !== port) return;
        if (m?.cmd === 'apply') {
          // режим «Весь трафик»: ядру нужны списки сайтов, правила строит Xray
          m = { ...m, args: { ...m.args, tun: tunActive() ? { mode: data.mode || 'all', include: data.include || [], exclude: data.exclude || [] } : null } };
          // macOS (только TUN): постоянный порт прокси не нужен — ядро берёт любой свободный
          if (caps.tunOnly) m.args.preferredPort = 0;
        }
        bridge.call('Native', JSON.stringify(m));
      },
      disconnect() { if (curPort === port) curPort = null; },
    };
    curPort = port;
    return port;
  }

  // ----- будильники -----
  const alarms = new Map();
  const onAlarm = listenerSet();
  const fire = (name) => onAlarm.set.forEach((fn) => fn({ name }));
  const alarmsApi = {
    create(name, info = {}) {
      const old = alarms.get(name);
      if (old) { clearTimeout(old.t); clearInterval(old.i); }
      const a = { name, periodInMinutes: info.periodInMinutes, scheduledTime: Date.now() };
      const delay = info.when ? Math.max(0, info.when - Date.now()) : (info.delayInMinutes ?? info.periodInMinutes ?? 0) * 60000;
      a.scheduledTime = Date.now() + delay;
      a.t = setTimeout(() => {
        fire(name);
        if (info.periodInMinutes) {
          a.i = setInterval(() => { a.scheduledTime = Date.now() + info.periodInMinutes * 60000; fire(name); }, info.periodInMinutes * 60000);
        } else alarms.delete(name);
      }, delay);
      alarms.set(name, a);
      return Promise.resolve();
    },
    async get(name) { const a = alarms.get(name); return a ? { name, periodInMinutes: a.periodInMinutes, scheduledTime: a.scheduledTime } : undefined; },
    async clear(name) { const a = alarms.get(name); if (a) { clearTimeout(a.t); clearInterval(a.i); alarms.delete(name); } return true; },
    onAlarm,
  };

  // ----- окно: страницы расширения открываются внутри приложения -----
  const pages = { 'options.html': 'options', 'popup.html': 'popup' };
  function openInside(url) {
    const path = String(url).replace(origin + '/', '').replace(/^\//, '');
    const page = path.split(/[?#]/)[0];
    if (!(page in pages)) return false;
    globalThis.top.__sbNavigate?.(path, pages[page]);
    return true;
  }

  // Запросы к другим сайтам (сайт аккаунта, файл версий) — через приложение: окну
  // приложения браузерный движок их не разрешает (CORS), а расширению разрешает браузер.
  const nativeFetch = globalThis.fetch.bind(globalThis);
  globalThis.fetch = async (input, init = {}) => {
    const u = new URL(typeof input === 'string' ? input : input.url, location.href);
    if (u.origin === location.origin) return nativeFetch(input, init);
    const r = await bridge.call('Fetch', u.href, init.method || 'GET', JSON.stringify(init.headers || {}));
    return new Response(r.body, { status: r.status });
  };

  const noop = { addListener() {} };
  const ext = {
    isApp: true,
    elevated,
    caps,
    appAuthPort: authPort,
    bridge,
    storage: { local, session, onChanged: changed },
    runtime: {
      id: 'safebridge-app',
      lastError: undefined,
      getURL: (p) => origin + '/' + String(p).replace(/^\//, ''),
      getManifest: () => ({ version: appVersion, name: 'SafeBridge' }),
      getPlatformInfo: async () => platform,
      sendMessage: (msg) => sendMessage(msg),
      _sendAs: sendMessage,
      onMessage,
      onInstalled: noop,
      onStartup: noop,
      connectNative,
      openOptionsPage: async () => { openInside('options.html'); },
    },
    alarms: alarmsApi,
    action: { setIcon: async () => {}, setBadgeText: async () => {}, setBadgeBackgroundColor: async () => {}, setTitle: async () => {} },
    proxy: {
      settings: {
        set: ({ value }) => bridge.call('SetProxy', JSON.stringify(value)),
        clear: () => bridge.call('SetProxy', JSON.stringify({ mode: 'direct' })),
        get: async () => ({ levelOfControl: 'controlled_by_this_extension' }),
      },
      onError: noop,
      onRequest: noop,
    },
    privacy: { network: { webRTCIPHandlingPolicy: { set: async () => {}, clear: async () => {} } } },
    tabs: {
      async create({ url }) {
        if (!openInside(url)) await bridge.call('OpenURL', url);
        return { id: -1 };
      },
      query: async () => [],
      remove: async () => {},
      reload: async () => {},
      onRemoved: noop,
    },
    management: { uninstallSelf: async () => {} },
  };

  // TUN зависит от «VPN включён», режима и списков сайтов: при их изменении пересобираем конфиг ядра
  let reapplyTimer = null;
  changed.addListener((diff) => {
    const keys = Object.keys(diff);
    const tunMode = caps.tunOnly || (data.appTun && elevated);
    const tunRelevant = keys.includes('appTun') || (tunMode && keys.some((k) => ['enabled', 'mode', 'include', 'exclude'].includes(k)));
    if (!tunRelevant) return;
    clearTimeout(reapplyTimer);
    reapplyTimer = setTimeout(() => sendMessage({ type: 'appReapply' }), 200);
  });

  // итог входа через Google: сайт вернул его на локальный адрес приложения
  bridge.on('auth', (s) => {
    let params = {};
    try { params = JSON.parse(s); } catch {}
    const site = (data.accountSite && /^http:\/\/(127\.0\.0\.1|localhost)(:\d+)?$/.test(data.accountSite) ? data.accountSite : 'https://tapp.internet-safe.online');
    sendMessage({ type: 'accountDone', params }, { id: 'safebridge-app', url: site + '/ext/done' });
  });
  return ext;
}

