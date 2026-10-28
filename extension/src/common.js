// Общие константы и функции для фона, popup и настроек.

export const HOST_NAME = 'com.vlessbridge.host';

// Откуда скачивать помощник (релизы на GitHub).
export const RELEASES_URL = 'https://github.com/maxhoney920/SafeBridge/releases/latest';
export const DOWNLOAD_BASE = RELEASES_URL + '/download/';

export const DEFAULTS = {
  subs: [],            // [{id, url, name, ua, servers, info, updatedAt, error}]
  selected: '',        // id выбранного сервера
  enabled: false,      // включён ли прокси в браузере
  mode: 'all',         // all | include | exclude
  include: [],         // домены для режима «только эти сайты»
  exclude: [],         // домены для режима «все, кроме этих»
  intervalMin: 120,    // автообновление подписок
  pingUrl: 'https://www.gstatic.com/generate_204',
  webrtc: true,        // защищать от утечки IP через WebRTC в режиме «все сайты»
  port: 0,             // предпочтительный локальный порт (0 — любой свободный)
};

export async function load() {
  const st = await chrome.storage.local.get(Object.keys(DEFAULTS));
  return { ...DEFAULTS, ...st };
}

export function save(patch) {
  return chrome.storage.local.set(patch);
}

export function allServers(st) {
  const out = [];
  for (const sub of st.subs) for (const s of sub.servers || []) out.push({ ...s, subId: sub.id });
  return out;
}

// Принимает домен, адрес страницы или маску вида *.example.com, возвращает домен в punycode.
export function normalizeDomain(input) {
  let s = String(input || '').trim().toLowerCase();
  if (!s) return '';
  s = s.replace(/^\*\./, '').replace(/^\./, '');
  try {
    const u = new URL(/^[a-z][a-z0-9+.-]*:\/\//.test(s) ? s : 'http://' + s);
    s = u.hostname;
  } catch {
    return '';
  }
  s = s.replace(/\.$/, '');
  if (!/^[a-z0-9.-]+$/.test(s) && !/^\[[0-9a-f:]+\]$/.test(s)) return '';
  return s.includes('.') || s.startsWith('[') ? s : '';
}

export function domainMatches(host, list) {
  host = host.toLowerCase();
  return list.find((d) => host === d || host.endsWith('.' + d)) || null;
}

// PAC-скрипт: решение «через прокси или напрямую» по домену.
// tag меняется при смене сервера: Chrome видит новую конфигурацию прокси и закрывает
// старые соединения, иначе открытые сайты ещё какое-то время шли бы через прежний сервер.
export function buildPac({ mode, include, exclude }, port, tag = '') {
  const list = mode === 'include' ? include : mode === 'exclude' ? exclude : [];
  return `// server: ${String(tag).replace(/[^\w-]/g, '')}
var P = "SOCKS5 127.0.0.1:${port}";
var M = ${JSON.stringify(mode)};
var D = ${JSON.stringify(list)};
function inList(h) {
  for (var i = 0; i < D.length; i++) {
    var d = D[i];
    if (h === d || (h.length > d.length && h.substring(h.length - d.length - 1) === "." + d)) return true;
  }
  return false;
}
function FindProxyForURL(url, host) {
  host = host.toLowerCase();
  if (isPlainHostName(host) || host === "localhost" || host === "[::1]" || /\\.local$/.test(host) ||
      /^(127|10)\\./.test(host) || /^192\\.168\\./.test(host) || /^172\\.(1[6-9]|2\\d|3[01])\\./.test(host) ||
      /^169\\.254\\./.test(host)) return "DIRECT";
  if (M === "include") return inList(host) ? P : "DIRECT";
  if (M === "exclude") return inList(host) ? "DIRECT" : P;
  return P;
}`;
}

// Готовые наборы доменов для режима «только эти сайты».
export const PRESETS = [
  { name: 'YouTube', domains: ['youtube.com', 'youtu.be', 'googlevideo.com', 'ytimg.com', 'ggpht.com', 'youtube-nocookie.com', 'youtubei.googleapis.com'] },
  { name: 'Instagram', domains: ['instagram.com', 'cdninstagram.com', 'fbcdn.net'] },
  { name: 'Facebook', domains: ['facebook.com', 'fb.com', 'fbcdn.net', 'messenger.com'] },
  { name: 'X (Twitter)', domains: ['x.com', 'twitter.com', 'twimg.com', 't.co'] },
  { name: 'Discord', domains: ['discord.com', 'discord.gg', 'discordapp.com', 'discordapp.net', 'discord.media'] },
  { name: 'ChatGPT', domains: ['chatgpt.com', 'openai.com', 'oaistatic.com', 'oaiusercontent.com'] },
  { name: 'Claude', domains: ['claude.ai', 'anthropic.com', 'claudeusercontent.com'] },
  { name: 'LinkedIn', domains: ['linkedin.com', 'licdn.com'] },
  { name: 'Telegram Web', domains: ['telegram.org', 't.me'] },
];

// ---------- форматирование ----------

export function fmtBytes(n) {
  if (!n) return '0 ГБ';
  const gb = n / 1024 ** 3;
  if (gb >= 100) return Math.round(gb) + ' ГБ';
  if (gb >= 1) return gb.toFixed(1).replace('.', ',') + ' ГБ';
  return Math.max(1, Math.round(n / 1024 ** 2)) + ' МБ';
}

export function fmtAgo(ts) {
  if (!ts) return 'ещё не обновлялась';
  const m = Math.round((Date.now() - ts) / 60000);
  if (m < 1) return 'обновлена только что';
  if (m < 60) return `обновлена ${m} мин назад`;
  const h = Math.round(m / 60);
  if (h < 24) return `обновлена ${h} ч назад`;
  return `обновлена ${new Date(ts).toLocaleDateString('ru-RU')}`;
}

export function fmtDate(sec) {
  return new Date(sec * 1000).toLocaleDateString('ru-RU', { day: 'numeric', month: 'long', year: 'numeric' });
}

export function protoLabel(s) {
  const p = { vless: 'VLESS', vmess: 'VMess', trojan: 'Trojan', shadowsocks: 'SS', hysteria2: 'Hysteria2' }[s.protocol] || s.protocol;
  const extra = [];
  if (s.security === 'reality') extra.push('Reality');
  else if (s.security === 'tls' && s.protocol !== 'trojan' && s.protocol !== 'hysteria2') extra.push('TLS');
  const tr = { xhttp: 'XHTTP', ws: 'WS', grpc: 'gRPC', httpupgrade: 'HTTPUpgrade', kcp: 'mKCP' }[s.transport];
  if (tr) extra.push(tr);
  return [p, ...extra].join(' + ');
}

export function send(type, data = {}) {
  return chrome.runtime.sendMessage({ type, ...data });
}

export function el(tag, attrs = {}, ...children) {
  const e = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (v == null || v === false) continue;
    if (k === 'class') e.className = v;
    else if (k.startsWith('on')) e.addEventListener(k.slice(2), v);
    else if (k === 'text') e.textContent = v;
    else e.setAttribute(k, v === true ? '' : v);
  }
  for (const c of children.flat(Infinity)) if (c != null && c !== false) e.append(c.nodeType ? c : String(c));
  return e;
}

export function plural(n, one, few, many) {
  const a = n % 10, b = n % 100;
  if (a === 1 && b !== 11) return one;
  if (a >= 2 && a <= 4 && (b < 12 || b > 14)) return few;
  return many;
}
