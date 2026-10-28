// Общие константы и функции для фона, popup и настроек.

export const HOST_NAME = 'com.vlessbridge.host';

// Откуда скачивать помощник: ветка downloads репозитория на GitHub (её обновляет CI).
export const REPO_URL = 'https://github.com/maxhoney920/SafeBridge';
export const DOWNLOADS_PAGE = REPO_URL + '/tree/downloads';
export const DOWNLOAD_BASE = REPO_URL + '/raw/downloads/';

// Файл помощника для платформы браузера (chrome.runtime.getPlatformInfo).
export function helperDownload(p) {
  const arch = p.arch === 'arm64' || p.arch === 'arm' ? 'arm64' : 'amd64';
  if (p.os === 'win') return { os: 'windows', name: 'Windows', file: `vlessbridge-windows-${arch}.exe` };
  if (p.os === 'mac') return { os: 'darwin', name: 'macOS', file: `vlessbridge-macos-${arch}.zip` };
  if (p.os === 'linux') return { os: 'linux', name: 'Linux', file: `vlessbridge-linux-${arch}` };
  return null;
}

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
  autoCore: true,      // обновлять ядро Xray автоматически
  updates: null,       // результат последней проверки обновлений
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

// Принимает домен, адрес страницы, маску *.example.com или доменную зону (.ru, рф),
// возвращает домен в punycode. Пустая строка — не похоже на адрес сайта.
export function normalizeDomain(input) {
  let s = String(input || '').trim().toLowerCase();
  if (!s) return '';
  s = s.replace(/^\*\./, '').replace(/^\./, '');
  if (/^[a-z]{2,24}$/.test(s) || /^xn--[a-z0-9-]+$/.test(s)) return s; // зона: ru, com, xn--p1ai
  if (/^[^\x00-\x7f]+$/.test(s)) {
    try { return new URL('http://a.' + s).hostname.slice(2); } catch { return ''; } // зона «рф»
  }
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

// Домен сайта без поддоменов: music.youtube.com → youtube.com, www.bbc.co.uk → bbc.co.uk.
const SECOND_LEVEL = new Set(['co', 'com', 'net', 'org', 'gov', 'edu', 'ac', 'or', 'ne', 'go', 'msk', 'spb']);
export function siteDomain(host) {
  const parts = host.toLowerCase().replace(/\.$/, '').split('.');
  if (parts.length <= 2 || /^\d+$/.test(parts[parts.length - 1])) return parts.join('.');
  const n = parts.length;
  const take = SECOND_LEVEL.has(parts[n - 2]) && parts[n - 1].length === 2 ? 3 : 2;
  return parts.slice(-take).join('.');
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

// Готовые наборы для списка «Через VPN».
export const PRESETS_VPN = [
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

// Готовые наборы для списка «Без VPN».
export const PRESETS_DIRECT = [
  { name: 'Все сайты .ru, .рф, .su', domains: ['ru', 'xn--p1ai', 'su'] },
  { name: 'Госуслуги и налоги', domains: ['gosuslugi.ru', 'nalog.gov.ru', 'mos.ru', 'esia.gosuslugi.ru'] },
  { name: 'Банки', domains: ['sberbank.ru', 'tbank.ru', 'tinkoff.ru', 'vtb.ru', 'alfabank.ru', 'gazprombank.ru', 'raiffeisen.ru'] },
  { name: 'Маркетплейсы', domains: ['ozon.ru', 'wildberries.ru', 'wb.ru', 'avito.ru', 'market.yandex.ru'] },
  { name: 'Яндекс', domains: ['yandex.ru', 'ya.ru', 'yandex.net', 'yastatic.net', 'yandex.com'] },
  { name: 'VK и Mail.ru', domains: ['vk.com', 'vk.ru', 'userapi.com', 'vkuser.net', 'mail.ru', 'ok.ru'] },
];

export const PRESETS = PRESETS_VPN;

// Режимы и списки — одни и те же слова в окне расширения и в настройках.
export const MODES = {
  all: { label: 'Все сайты', hint: 'Все сайты открываются через VPN.' },
  include: { label: 'Только список', hint: 'Через VPN открываются только сайты из списка «Через VPN». Остальные — напрямую, как без расширения.' },
  exclude: { label: 'Исключения', hint: 'Через VPN открываются все сайты, кроме исключений из списка «Без VPN».' },
};
export const LIST_NAMES = { include: '«Через VPN»', exclude: '«Без VPN»' };

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
