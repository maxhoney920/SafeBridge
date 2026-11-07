// Значок SafeBridge в строке меню macOS (рядом с часами): состояние VPN и быстрые
// настройки — включить/выключить, сервер, режим. Меню собирается здесь из того же
// состояния, что видит окно, а показывает его приложение (Go + AppKit).
import { ext } from './compat.js';
import { MODES } from './common.js';

const send = (type, data = {}) => ext.runtime.sendMessage({ type, ...data });
const MAX_SERVERS = 40;

async function getState() {
  const r = await send('state').catch(() => null);
  return r?.ok ? r.result : null;
}

function build(st) {
  if (!st) return { visible: true, on: false, tooltip: 'SafeBridge', items: [{ title: 'SafeBridge запускается…', disabled: true }, { sep: true }, { id: 'open', title: 'Открыть SafeBridge' }, { id: 'quit', title: 'Выйти из SafeBridge' }] };
  const servers = [];
  for (const sub of st.subs || []) for (const s of sub.servers || []) if (s.outbound) servers.push(s);
  const current = st.core?.selected || st.selected;
  const sel = servers.find((s) => s.id === current);
  const active = !!st.enabled && !!st.core?.running && !st.coreError;
  const problem = !!st.enabled && !active;
  const ms = sel ? st.pings?.[sel.id] : null;
  const status = active ? `Подключено${sel ? ` · ${sel.name}` : ''}${ms > 0 ? ` · ${ms} мс` : ''}`
    : problem ? 'Нет соединения с сервером' : 'VPN выключен';

  const items = [
    { title: status, disabled: true },
    servers.length
      ? { id: 'toggle', title: st.enabled ? 'Выключить VPN' : 'Включить VPN' }
      : { title: 'Добавьте подписку в окне SafeBridge', disabled: true },
    { sep: true },
  ];
  if (servers.length) {
    const list = servers.slice(0, MAX_SERVERS).map((s) => ({
      id: 'srv:' + s.id,
      title: s.name + (st.pings?.[s.id] > 0 ? `   ${st.pings[s.id]} мс` : ''),
      checked: s.id === current,
    }));
    if (servers.length > MAX_SERVERS) list.push({ id: 'open', title: `Ещё ${servers.length - MAX_SERVERS} — в окне SafeBridge…` });
    items.push({ title: sel ? `Сервер: ${sel.name}` : 'Сервер', items: list });
  }
  items.push({
    title: `Режим: ${MODES[st.mode]?.label || MODES.all.label}`,
    items: Object.entries(MODES).map(([k, m]) => ({ id: 'mode:' + k, title: m.label, checked: (st.mode || 'all') === k })),
  });
  items.push({ id: 'ping', title: 'Проверить пинг' });
  items.push({ sep: true });
  items.push({ id: 'open', title: 'Открыть SafeBridge' });
  items.push({ id: 'settings', title: 'Настройки…' });
  items.push({ sep: true });
  items.push({ id: 'quit', title: 'Выйти из SafeBridge' });
  return { visible: st.appTray !== false, on: active, tooltip: `SafeBridge — ${status}`, items };
}

let timer = null;
let last = '';
async function refresh() {
  const st = await getState();
  const menu = st && st.appTray === false ? { visible: false } : build(st);
  const json = JSON.stringify(menu);
  if (json === last) return;
  last = json;
  ext.bridge.call('TraySet', json).catch(() => {});
}
function schedule(ms = 300) {
  clearTimeout(timer);
  timer = setTimeout(refresh, ms);
}

async function onClick(id) {
  try {
    if (id === 'toggle') {
      const st = await getState();
      await send('toggle', { on: !st?.enabled });
    } else if (id.startsWith('srv:')) {
      await send('select', { id: id.slice(4) });
    } else if (id.startsWith('mode:')) {
      await send('setMode', { mode: id.slice(5) });
    } else if (id === 'ping') {
      await send('ping');
    } else if (id === 'open') {
      await ext.bridge.call('ShowWindow');
    } else if (id === 'settings') {
      globalThis.__sbNavigate?.('options.html#settings');
    } else if (id === 'quit') {
      await ext.bridge.call('Quit');
      return;
    }
  } catch (e) {
    console.warn('tray', id, e);
  }
  schedule(100);
}

export function startTray() {
  if (!ext.caps.tray) return;
  ext.bridge.on('tray', (id) => onClick(String(id)));
  ext.storage.onChanged.addListener(() => schedule());
  ext.bridge.on('native', () => schedule(600)); // ядро запустилось, упало, пинг…
  setInterval(refresh, 30000);
  refresh();
}
