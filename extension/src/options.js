import { el, send, fmtBytes, fmtAgo, fmtDate, PRESETS, normalizeDomain, plural } from './common.js';

const app = document.getElementById('app');
let state = null;
const errors = {};
const busy = {};
let logText = null;

async function act(type, data, key = type, okText) {
  busy[key] = true;
  delete errors[key];
  render();
  try {
    const r = await send(type, data);
    if (!r?.ok) throw new Error(r?.error || 'Нет ответа от расширения');
    if (r.result?.subs) state = r.result;
    if (okText) toast(okText);
    return r.result;
  } catch (e) {
    errors[key] = e.message;
  } finally {
    busy[key] = false;
    render();
  }
}

async function reload() {
  const r = await send('state').catch(() => null);
  if (r?.ok) state = r.result;
  render();
}

chrome.runtime.onMessage.addListener((m) => {
  if (m?.type === 'changed') {
    clearTimeout(reload.t);
    reload.t = setTimeout(reload, 120);
  }
});

function toast(text) {
  const t = el('div', { class: 'toast', role: 'status', text });
  document.body.append(t);
  setTimeout(() => t.remove(), 1800);
}

const spin = () => el('span', { class: 'spinner' });

// ---------- подписки ----------

function subsSection() {
  const input = el('input', { class: 'input grow', type: 'url', placeholder: 'https://…/sub/…', id: 'newSub', 'aria-label': 'Ссылка на подписку' });
  const add = async () => {
    const res = await act('addSub', { url: input.value }, 'addSub', 'Подписка добавлена');
    if (res) input.value = '';
  };
  input.addEventListener('keydown', (e) => e.key === 'Enter' && add());

  const cards = state.subs.map((sub) => {
    const i = sub.info || {};
    const ok = (sub.servers || []).filter((s) => s.outbound).length;
    const bad = (sub.servers || []).length - ok;
    const facts = [el('span', {}, el('b', { text: String(ok) }), ` ${plural(ok, 'сервер', 'сервера', 'серверов')}`)];
    if (bad) facts.push(el('span', { text: `${bad} не поддерживается` }));
    if (i.total > 0) facts.push(el('span', {}, el('b', { text: fmtBytes((i.upload || 0) + (i.download || 0)) }), ` из ${fmtBytes(i.total)}`));
    if (i.expire > 0) facts.push(el('span', { text: `действует до ${fmtDate(i.expire)}` }));
    facts.push(el('span', { text: fmtAgo(sub.updatedAt) }));
    if (i.via === 'proxy') facts.push(el('span', { text: 'загружена через VPN' }));
    return el('div', { class: 'card sub' },
      el('div', { class: 'title', text: sub.name }),
      el('div', { class: 'url', text: sub.url }),
      el('div', { class: 'actions' },
        el('button', { class: 'btn', disabled: busy['r' + sub.id], onclick: () => act('refresh', { subId: sub.id }, 'r' + sub.id, 'Подписка обновлена') },
          busy['r' + sub.id] ? spin() : 'Обновить'),
        el('button', { class: 'btn danger', onclick: () => confirm(`Удалить подписку «${sub.name}»?`) && act('removeSub', { id: sub.id }, 'rm', 'Подписка удалена'), text: 'Удалить' })),
      el('div', { class: 'facts' }, facts),
      sub.error && el('div', { class: 'notice', text: `Последнее обновление не удалось: ${sub.error}` }),
      errors['r' + sub.id] && el('div', { class: 'notice', text: errors['r' + sub.id] }));
  });

  return el('section', { id: 'subs' },
    el('h2', { text: 'Подписки' }),
    el('p', { class: 'lead', text: `Серверы берутся из ссылки-подписки и обновляются автоматически каждые ${fmtInterval(state.intervalMin)}. Поддерживаются VLESS (Reality, TLS, WebSocket, gRPC, HTTPUpgrade), Trojan, VMess, Shadowsocks и Hysteria2.` }),
    el('div', { class: 'stack' },
      cards,
      el('div', { class: 'card stack' },
        el('div', { class: 'row' }, input,
          el('button', { class: 'btn primary', onclick: add, disabled: busy.addSub }, busy.addSub ? spin() : 'Добавить подписку')),
        errors.addSub && el('div', { class: 'notice', text: errors.addSub }))));
}

function fmtInterval(min) {
  if (min % 60 === 0) {
    const h = min / 60;
    return h === 1 ? 'час' : `${h} ${[2, 3, 4].includes(h % 10) && ![12, 13, 14].includes(h % 100) ? 'часа' : 'часов'}`;
  }
  return `${min} мин`;
}

// ---------- сайты ----------

const drafts = {};

function listCard(key, title, desc) {
  const value = drafts[key] ?? state[key].join('\n');
  const ta = el('textarea', { class: 'input', spellcheck: 'false', 'aria-label': title, placeholder: 'youtube.com\ninstagram.com' });
  ta.value = value;
  ta.addEventListener('input', () => { drafts[key] = ta.value; });
  const saveList = async () => {
    const domains = ta.value.split(/[\s,]+/).filter(Boolean);
    const bad = domains.filter((d) => !normalizeDomain(d));
    if (bad.length) {
      errors[key] = `Не похоже на адреса сайтов: ${bad.slice(0, 5).join(', ')}`;
      render();
      return;
    }
    const r = await act('setSites', { list: key, domains }, key, 'Список сохранён');
    if (r) delete drafts[key];
    render();
  };
  const current = new Set((drafts[key] ?? state[key].join('\n')).split(/[\s,]+/).map(normalizeDomain).filter(Boolean));
  const presets = key === 'include' && el('div', { class: 'presets' },
    PRESETS.map((p) => {
      const added = p.domains.every((d) => current.has(d));
      return el('button', {
        class: 'chip' + (added ? ' added' : ''), title: p.domains.join(', '),
        onclick: () => {
          const set = new Set((drafts[key] ?? state[key].join('\n')).split(/[\s,]+/).filter(Boolean));
          p.domains.forEach((d) => (added ? set.delete(d) : set.add(d)));
          drafts[key] = [...set].join('\n');
          render();
        },
      }, (added ? '− ' : '+ ') + p.name);
    }));
  const dirty = drafts[key] != null && drafts[key] !== state[key].join('\n');
  return el('div', { class: 'card list-card' + (state.mode === key ? ' active' : '') },
    el('h3', { text: title }),
    el('p', { text: desc }),
    ta,
    presets,
    el('div', { class: 'row', style: 'margin-top:10px' },
      el('button', { class: 'btn primary', onclick: saveList, disabled: busy[key] || !dirty }, busy[key] ? spin() : 'Сохранить'),
      dirty && el('button', { class: 'btn quiet', onclick: () => { delete drafts[key]; render(); }, text: 'Отменить' }),
      state.mode !== key && el('button', { class: 'btn quiet', onclick: () => act('setMode', { mode: key }, 'mode', 'Режим включён'), text: 'Включить этот режим' })),
    errors[key] && el('div', { class: 'notice', style: 'margin-top:10px', text: errors[key] }));
}

function sitesSection() {
  const modeText = { all: 'Сейчас через VPN открываются все сайты.', include: 'Сейчас через VPN открываются только сайты из первого списка.', exclude: 'Сейчас через VPN открываются все сайты, кроме второго списка.' }[state.mode];
  return el('section', { id: 'sites' },
    el('h2', { text: 'Сайты' }),
    el('p', { class: 'lead', text: `${modeText} Режим переключается в окне расширения: «Все сайты», «Только список» или «Кроме списка». Каждый домен в списке включает и все его поддомены. Сайт можно добавить прямо из окна расширения, находясь на нём.` }),
    el('div', { class: 'lists' },
      listCard('include', 'Только эти сайты через VPN', 'Для режима «Только список». Остальные сайты открываются напрямую.'),
      listCard('exclude', 'Эти сайты напрямую', 'Для режима «Кроме списка». Например, банки и госуслуги, которые не открываются из-за рубежа.')));
}

// ---------- параметры ----------

function settingsSection() {
  const interval = el('select', { class: 'input', id: 'interval' },
    [[30, '30 минут'], [60, '1 час'], [120, '2 часа'], [360, '6 часов'], [720, '12 часов'], [1440, '24 часа']]
      .map(([v, t]) => el('option', { value: v, selected: state.intervalMin === v, text: t })));
  const ping = el('input', { class: 'input', id: 'pingUrl', type: 'url', value: state.pingUrl });
  const port = el('input', { class: 'input', id: 'port', type: 'number', min: 0, max: 65535, value: state.port || '', placeholder: 'любой свободный' });
  const webrtc = el('input', { type: 'checkbox', id: 'webrtc', checked: state.webrtc });
  const saveAll = () => act('saveSettings', { settings: {
    intervalMin: Number(interval.value), pingUrl: ping.value, port: Number(port.value) || 0, webrtc: webrtc.checked,
  } }, 'settings', 'Параметры сохранены');
  return el('section', { id: 'settings' },
    el('h2', { text: 'Параметры' }),
    el('div', { class: 'card' },
      el('div', { class: 'field' }, el('label', { for: 'interval', text: 'Обновлять подписки' }), interval,
        el('div', { class: 'hint', text: 'Обновить вручную можно кнопкой «Обновить» в окне расширения.' })),
      el('div', { class: 'field' }, el('label', { for: 'webrtc', text: 'Защита от утечки IP' }),
        el('label', { class: 'check' }, webrtc, el('span', { text: 'Пускать WebRTC только через VPN' })),
        el('div', { class: 'hint', text: 'Работает в режиме «Все сайты». Без неё сайты могут увидеть ваш настоящий IP через звонки и WebRTC.' })),
      el('div', { class: 'field' }, el('label', { for: 'pingUrl', text: 'Адрес для проверки пинга' }), ping,
        el('div', { class: 'hint', text: 'Только https. Пинг — время загрузки этого адреса через сервер.' })),
      el('div', { class: 'field' }, el('label', { for: 'port', text: 'Локальный порт' }), port,
        el('div', { class: 'hint', text: 'Порт прокси на 127.0.0.1. Оставьте пустым, если не нужен постоянный.' })),
      el('div', { class: 'row', style: 'margin-top:8px' },
        el('button', { class: 'btn primary', onclick: saveAll, disabled: busy.settings }, busy.settings ? spin() : 'Сохранить')),
      errors.settings && el('div', { class: 'notice', style: 'margin-top:10px', text: errors.settings })));
}

// ---------- помощник ----------

function helperSection() {
  const h = state.host;
  const items = h ? [
    ['Помощник', `${h.version} (${h.os}/${h.arch})`],
    ['Ядро', h.core ? `sing-box ${h.core}` : (h.coreError || 'не найдено')],
    ['Состояние', state.core.running ? `работает, локальный порт ${state.core.port}` : (state.coreError || 'остановлено')],
  ] : [['Помощник', state.hostError || 'не подключён']];
  return el('section', { id: 'helper' },
    el('h2', { text: 'Помощник' }),
    el('p', { class: 'lead', text: 'Программа на компьютере, которая держит соединение с сервером. Браузер запускает её сам и останавливает при закрытии.' }),
    el('div', { class: 'card' },
      el('dl', { class: 'kv' }, items.map(([k, v]) => [el('dt', { text: k }), el('dd', { text: v })])),
      el('div', { class: 'row', style: 'margin-top:14px' },
        el('button', { class: 'btn', disabled: busy.diag, onclick: async () => {
          const r = await act('diag', {}, 'diag');
          if (r) logText = `Каталог данных: ${r.dataDir}\n\n${(r.log || []).join('\n') || 'Журнал ядра пуст.'}`;
          render();
        } }, busy.diag ? spin() : 'Показать журнал ядра'),
        el('button', { class: 'btn', onclick: () => act('retryHost', {}, 'retry', 'Помощник перезапущен'), disabled: busy.retry }, busy.retry ? spin() : 'Переподключить')),
      errors.diag && el('div', { class: 'notice', style: 'margin-top:10px', text: errors.diag }),
      logText && el('pre', { class: 'log', text: logText })));
}

function render() {
  if (!state) return;
  const active = document.activeElement;
  const activeId = active?.id;
  const sel = active && 'selectionStart' in active ? [active.selectionStart, active.selectionEnd] : null;
  const ta = active?.tagName === 'TEXTAREA' ? active.getAttribute('aria-label') : null;
  const newSub = document.getElementById('newSub')?.value;
  const settingsDraft = ['interval', 'pingUrl', 'port', 'webrtc'].map((id) => {
    const e = document.getElementById(id);
    return e && [id, e.type === 'checkbox' ? e.checked : e.value];
  }).filter(Boolean);

  app.replaceChildren(subsSection(), sitesSection(), settingsSection(), helperSection());

  if (newSub) document.getElementById('newSub').value = newSub;
  if (!busy.settings) for (const [id, v] of settingsDraft) {
    const e = document.getElementById(id);
    if (e.type === 'checkbox') e.checked = v; else e.value = v;
  }
  const again = ta ? app.querySelector(`textarea[aria-label="${ta}"]`) : activeId ? document.getElementById(activeId) : null;
  if (again) {
    again.focus();
    if (sel && 'setSelectionRange' in again) try { again.setSelectionRange(...sel); } catch {}
  }
  highlightNav();
}

function highlightNav() {
  const h = location.hash || '#subs';
  document.querySelectorAll('.side a').forEach((a) => a.classList.toggle('cur', a.getAttribute('href') === h));
}
window.addEventListener('hashchange', highlightNav);

(async () => {
  await reload();
  if (location.hash) document.querySelector(location.hash)?.scrollIntoView();
})();
