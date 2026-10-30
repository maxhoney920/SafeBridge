import { ext, isFirefox } from './compat.js';
import { MIN_HELPER, versionLess } from './common.js';
import { el, send, fmtBytes, fmtAgo, fmtDate, PRESETS_VPN, PRESETS_DIRECT, normalizeDomain, plural, MODES, LIST_NAMES, DOWNLOADS_PAGE } from './common.js';

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

ext.runtime.onMessage.addListener((m) => {
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
      /^http:\/\//i.test(sub.url) && !/^http:\/\/(127\.|localhost)/i.test(sub.url) && el('div', { class: 'notice', text: 'Ссылка без https: по дороге её содержимое можно подменить и подсунуть чужие серверы. Попросите у VPN-сервиса ссылку, которая начинается с https://.' }),
      sub.error && el('div', { class: 'notice', text: `Последнее обновление не удалось: ${sub.error}` }),
      errors['r' + sub.id] && el('div', { class: 'notice', text: errors['r' + sub.id] }));
  });

  return el('section', { id: 'subs' },
    el('h2', { text: 'Подписки' }),
    el('p', { class: 'lead', text: `Серверы берутся из ссылки-подписки и обновляются автоматически каждые ${fmtInterval(state.intervalMin)}. Поддерживаются VLESS (Reality, TLS, Vision, XHTTP, WebSocket, gRPC, HTTPUpgrade, VLESS Encryption), Trojan, VMess, Shadowsocks и Hysteria2.` }),
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

function listCard(key, title, desc, examples, presetsList) {
  const value = drafts[key] ?? state[key].join('\n');
  const ta = el('textarea', { class: 'input', spellcheck: 'false', 'aria-label': title, placeholder: examples });
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
  const presets = el('div', { class: 'presets' },
    el('span', { class: 'presets-label', text: 'Быстро добавить:' }),
    presetsList.map((p) => {
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
  const active = state.mode === key;
  return el('div', { class: 'card list-card' + (active ? ' active' : '') },
    el('div', { class: 'list-head' },
      el('h3', { text: title }),
      active ? el('span', { class: 'badge', text: 'Используется сейчас' })
        : el('button', { class: 'btn quiet', onclick: () => act('setMode', { mode: key }, 'mode', `Режим «${MODES[key].label}» включён`), text: `Включить режим «${MODES[key].label}»` })),
    el('p', { text: desc }),
    ta,
    presets,
    el('div', { class: 'row', style: 'margin-top:10px' },
      el('button', { class: 'btn primary', onclick: saveList, disabled: busy[key] || !dirty }, busy[key] ? spin() : 'Сохранить'),
      dirty && el('button', { class: 'btn quiet', onclick: () => { delete drafts[key]; render(); }, text: 'Отменить' })),
    errors[key] && el('div', { class: 'notice', style: 'margin-top:10px', text: errors[key] }));
}

function sitesSection() {
  const modes = el('div', { class: 'modes' }, Object.entries(MODES).map(([k, m]) =>
    el('label', { class: 'mode-opt' + (state.mode === k ? ' cur' : '') },
      el('input', { type: 'radio', name: 'mode-o', checked: state.mode === k, onchange: () => act('setMode', { mode: k }, 'mode', `Режим «${m.label}» включён`) }),
      el('span', {}, el('b', { text: m.label }), el('span', { class: 'muted', text: m.hint })))));
  return el('section', { id: 'sites' },
    el('h2', { text: 'Сайты' }),
    el('p', { class: 'lead', text: 'Выберите, какие сайты открывать через VPN. То же самое переключается в окне расширения.' }),
    el('div', { class: 'card' }, modes),
    el('div', { class: 'lists' },
      listCard('include', `Список ${LIST_NAMES.include}`,
        'Работает в режиме «Только список». Сюда — сайты, которые без VPN не открываются или работают плохо. Все остальные сайты пойдут напрямую.',
        'youtube.com\ninstagram.com\nchatgpt.com', PRESETS_VPN),
      listCard('exclude', `Список ${LIST_NAMES.exclude}`,
        'Работает в режиме «Исключения». Сюда — сайты, которым нужен ваш обычный адрес: банки, Госуслуги, маркетплейсы и сайты, которые не пускают из-за рубежа.',
        'gosuslugi.ru\nsberbank.ru\nru', PRESETS_DIRECT)),
    el('div', { class: 'card rules' },
      el('h3', { text: 'Как записывать сайты' }),
      el('ul', {},
        el('li', {}, 'Один сайт на строку. Можно вставить адрес страницы целиком — останется только домен.'),
        el('li', {}, el('code', { text: 'youtube.com' }), ' включает все поддомены: ', el('code', { text: 'www.youtube.com' }), ', ', el('code', { text: 'm.youtube.com' }), ' и другие. Но не ', el('code', { text: 'youtube.ru' }), ' — это другой сайт.'),
        el('li', {}, el('code', { text: 'ru' }), ' (или ', el('code', { text: '.ru' }), ', ', el('code', { text: 'рф' }), ') — целиком доменная зона: все сайты, которые заканчиваются на .ru.'),
        el('li', {}, 'Большие сайты подгружают видео и картинки с других доменов. YouTube, например, берёт видео с googlevideo.com. Готовые наборы выше уже включают такие домены.'),
        el('li', {}, 'Проще всего добавлять прямо с сайта: откройте его, нажмите на значок расширения и кнопку «Открывать через VPN» или «Открывать без VPN».'))));
}

// ---------- параметры ----------

function settingsSection() {
  const interval = el('select', { class: 'input', id: 'interval' },
    [[30, '30 минут'], [60, '1 час'], [120, '2 часа'], [360, '6 часов'], [720, '12 часов'], [1440, '24 часа']]
      .map(([v, t]) => el('option', { value: v, selected: state.intervalMin === v, text: t })));
  const ping = el('input', { class: 'input', id: 'pingUrl', type: 'url', value: state.pingUrl });
  const port = el('input', { class: 'input', id: 'port', type: 'number', min: 0, max: 65535, value: state.port || '', placeholder: 'любой свободный' });
  const webrtc = el('input', { type: 'checkbox', id: 'webrtc', checked: state.webrtc });
  const kill = el('input', { type: 'checkbox', id: 'killSwitch', checked: state.killSwitch });
  const saveAll = () => act('saveSettings', { settings: {
    intervalMin: Number(interval.value), pingUrl: ping.value, port: Number(port.value) || 0, webrtc: webrtc.checked, killSwitch: kill.checked,
  } }, 'settings', 'Параметры сохранены');
  return el('section', { id: 'settings' },
    el('h2', { text: 'Параметры' }),
    el('div', { class: 'card' },
      el('div', { class: 'field' }, el('label', { for: 'interval', text: 'Обновлять подписки' }), interval,
        el('div', { class: 'hint', text: 'Обновить вручную можно кнопкой «Обновить» в окне расширения.' })),
      el('div', { class: 'field' }, el('label', { for: 'killSwitch', text: 'Если VPN пропал' }),
        el('label', { class: 'check' }, kill, el('span', { text: 'Не открывать сайты напрямую' })),
        el('div', { class: 'hint', text: 'Пока VPN включён, но соединения с сервером нет, сайты, которые должны идти через VPN, не откроются — вместо того чтобы тихо открыться с вашего настоящего адреса.' })),
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
  const u = state.updates || {};
  const items = h ? [
    ['Помощник', `${h.version} (${h.os}/${h.arch})`],
    ['Ядро', h.core ? `Xray ${h.core}` : (h.coreError || 'не найдено')],
    ['Состояние', state.core.running ? `работает, локальный порт ${state.core.port}` : (state.coreError || 'остановлено')],
  ] : [['Помощник', state.hostMissing ? 'не установлен' : (state.hostError || 'не подключён')]];
  const outdated = h && versionLess(h.version, MIN_HELPER);
  const installLink = (!h || outdated) && el('div', { class: 'stack' },
    outdated && el('div', { class: 'notice', text: `Помощник ${h.version} устарел: он работает на старом ядре, не поддерживает XHTTP и не умеет обновляться сам. Установите новый — он заменит старый, настройки сохранятся.` }),
    el('div', { class: 'row' }, el('a', { class: 'btn primary', href: ext.runtime.getURL('install.html'), target: '_blank', text: outdated ? 'Установить новый помощник' : 'Установить помощник' })));

  const helperLine = !u.checkedAt ? 'ещё не проверялось'
    : u.helper?.error ? `не удалось проверить: ${u.helper.error}`
    : u.helper?.available ? `доступна версия ${u.helper.latest}` : 'установлена последняя версия';
  const coreLine = !u.checkedAt ? 'ещё не проверялось'
    : u.core?.error ? `не удалось проверить: ${u.core.error}`
    : u.core?.available ? `доступна версия ${u.core.latest}` : 'установлена последняя версия'
    + (u.core?.lastResult ? ` (${u.core.lastResult})` : '');
  const autoCore = el('input', { type: 'checkbox', id: 'autoCore', checked: state.autoCore, onchange: (e) => act('setAutoCore', { on: e.target.checked }, 'autoCore') });

  return el('section', { id: 'helper' },
    el('h2', { text: 'Помощник и обновления' }),
    el('p', { class: 'lead', text: 'Помощник — программа на компьютере, которая держит соединение с сервером через ядро Xray. Браузер запускает её сам и останавливает при закрытии. Раз в сутки расширение проверяет обновления на GitHub.' }),
    el('div', { class: 'card stack' },
      el('dl', { class: 'kv' }, items.map(([k, v]) => [el('dt', { text: k }), el('dd', { text: v })])),
      installLink,
      h && !outdated && el('div', { class: 'updates' },
        el('div', { class: 'upd-row' },
          el('div', {}, el('b', { text: 'Помощник' }), el('div', { class: 'muted small', text: helperLine })),
          u.helper?.available && el('button', { class: 'btn primary', disabled: state.busy.updateHelper || busy.updateHelper,
            onclick: () => act('updateHelper', {}, 'updateHelper', 'Помощник обновлён') }, state.busy.updateHelper || busy.updateHelper ? spin() : `Обновить до ${u.helper.latest}`)),
        el('div', { class: 'upd-row' },
          el('div', {}, el('b', { text: 'Ядро Xray' }), el('div', { class: 'muted small', text: coreLine })),
          u.core?.available && el('button', { class: 'btn primary', disabled: state.busy.updateCore || busy.updateCore,
            onclick: () => act('updateCore', {}, 'updateCore', 'Ядро обновлено') }, state.busy.updateCore || busy.updateCore ? spin() : `Обновить до ${u.core.latest}`)),
        el('label', { class: 'check' }, autoCore, el('span', { text: 'Обновлять ядро Xray автоматически' })),
        el('div', { class: 'muted small', text: 'Перед переключением новое ядро проверяется на ваших серверах. Если оно их не примет или не запустится, останется прежняя версия.' }),
        el('div', { class: 'row' },
          el('button', { class: 'btn', disabled: state.busy.updates || busy.checkUpdates, onclick: () => act('checkUpdates', {}, 'checkUpdates') },
            state.busy.updates || busy.checkUpdates ? spin() : 'Проверить обновления'),
          u.checkedAt && el('span', { class: 'muted small', text: `проверено ${new Date(u.checkedAt).toLocaleString('ru-RU', { day: 'numeric', month: 'long', hour: '2-digit', minute: '2-digit' })}` }),
          el('a', { class: 'small', href: ext.runtime.getURL('install.html'), target: '_blank', text: 'Скачать помощник для другой системы' }))),
      ['updateHelper', 'updateCore', 'checkUpdates'].map((k) => errors[k] && el('div', { class: 'notice', text: errors[k] })),
      el('div', { class: 'row' },
        el('button', { class: 'btn', disabled: busy.diag, onclick: async () => {
          const r = await act('diag', {}, 'diag');
          if (r) logText = `Каталог данных: ${r.dataDir}\n\n${(r.log || []).join('\n') || 'Журнал ядра пуст.'}`;
          render();
        } }, busy.diag ? spin() : 'Показать журнал ядра'),
        el('button', { class: 'btn', onclick: () => act('retryHost', {}, 'retry', 'Помощник переподключён'), disabled: busy.retry }, busy.retry ? spin() : 'Переподключить')),
      errors.diag && el('div', { class: 'notice', text: errors.diag }),
      logText && el('pre', { class: 'log', text: logText })),
    removeCard(h));
}

let helperRemoved = false;

function removeCard(h) {
  if (helperRemoved && !h) {
    return el('div', { class: 'card stack danger-zone' },
      el('b', { text: 'Помощник удалён' }),
      el('div', { class: 'muted small', text: 'Программа и её регистрация в браузерах удалены с компьютера. Осталось удалить само расширение — или установить помощник заново.' }),
      el('div', { class: 'row' },
        el('button', { class: 'btn danger', onclick: () => ext.management.uninstallSelf({ showConfirmDialog: true }).catch(() => {}), text: 'Удалить и расширение' }),
        el('a', { class: 'btn', href: ext.runtime.getURL('install.html'), target: '_blank', text: 'Установить заново' })));
  }
  if (!h) return null;
  return el('div', { class: 'card stack danger-zone' },
    el('b', { text: 'Удаление' }),
    el('div', { class: 'muted small', text: 'Удалит помощник и ядро Xray с компьютера и снимет регистрацию во всех браузерах. Подписки и списки сайтов в расширении останутся.' }),
    el('div', { class: 'row' },
      el('button', { class: 'btn danger', disabled: busy.uninstallHelper, onclick: async () => {
        if (!confirm('Удалить помощник VlessBridge с компьютера? VPN в браузере перестанет работать, пока вы не установите его снова.')) return;
        const r = await act('uninstallHelper', {}, 'uninstallHelper');
        if (r) helperRemoved = true;
        render();
      } }, busy.uninstallHelper ? spin() : 'Удалить помощник с компьютера')),
    errors.uninstallHelper && el('div', { class: 'notice', text: errors.uninstallHelper }));
}

function render() {
  if (!state) return;
  const active = document.activeElement;
  const activeId = active?.id;
  const sel = active && 'selectionStart' in active ? [active.selectionStart, active.selectionEnd] : null;
  const ta = active?.tagName === 'TEXTAREA' ? active.getAttribute('aria-label') : null;
  const newSub = document.getElementById('newSub')?.value;
  const settingsDraft = ['interval', 'pingUrl', 'port', 'webrtc', 'killSwitch'].map((id) => {
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
