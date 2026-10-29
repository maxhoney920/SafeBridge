// Один код для Chrome и Firefox: в Firefox API живёт в `browser` и возвращает промисы,
// а кнопка расширения в Manifest V2 называется browserAction.
export const ext = globalThis.browser ?? globalThis.chrome;
export const isFirefox = typeof globalThis.browser !== 'undefined' && /Firefox\//.test(navigator.userAgent);
export const action = ext.action ?? ext.browserAction;

// storage.session есть не во всех версиях Firefox — тогда храним в памяти фоновой страницы.
const mem = {};
export const sessionStore = ext.storage.session ?? {
  async get(keys) { return Object.fromEntries([].concat(keys).filter((k) => k in mem).map((k) => [k, mem[k]])); },
  async set(obj) { Object.assign(mem, obj); },
};
