// Запуск окна-оболочки: сначала мост к приложению, потом остальные модули.
import { createExt } from './bridge.js';

globalThis.__sbExtPromise = createExt().then((e) => {
  globalThis.__sbExt = e;
  return e;
});
globalThis.__sbExtPromise
  .then(() => import('./app-shell.js'))
  .catch((e) => {
    const boot = document.getElementById('boot');
    if (boot) boot.textContent = 'Не удалось запустить SafeBridge: ' + (e && e.message ? e.message : e);
    console.error(e);
  });
