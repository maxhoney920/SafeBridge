// SafeBridge для компьютера: тот же интерфейс и та же «фоновая» логика, что в расширении,
// а вместо API браузера — мост к приложению на Go (Wails или режим разработки по HTTP).
// Объект `ext` создаётся один раз в окне-оболочке (index.html) и общий для окна и
// страниц в iframe (окно расширения, настройки), как общий фон у расширения.

export const isApp = true;
export const isFirefox = false;
document.documentElement.classList.add('app'); // стили окна приложения (не всплывающего окна браузера)

// Страница в iframe (окно расширения или настройки) пользуется общим объектом, но свои
// обработчики убирает, когда её сменяют: иначе они копились бы при каждом переходе.
function frameView(shared) {
  const mine = [];
  const wrap = (ls) => ({ ...ls, addListener: (fn) => { mine.push([ls, fn]); ls.addListener(fn); } });
  globalThis.addEventListener('pagehide', () => { for (const [ls, fn] of mine) ls.removeListener(fn); });
  return {
    ...shared,
    runtime: { ...shared.runtime, onMessage: wrap(shared.runtime.onMessage) },
    storage: { ...shared.storage, onChanged: wrap(shared.storage.onChanged) },
  };
}

// объект создан в boot.js окна-оболочки до загрузки этого модуля
const shared = globalThis.top.__sbExt;
if (!shared) throw new Error('SafeBridge: мост к приложению не готов');
export const ext = globalThis === globalThis.top ? shared : frameView(shared);
export const action = ext.action;
export const sessionStore = ext.storage.session;
