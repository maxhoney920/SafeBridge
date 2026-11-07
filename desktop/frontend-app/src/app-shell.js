// Оболочка приложения: здесь работает «фон» (тот же background.js, что в расширении),
// а окно расширения и настройки показываются в iframe и переключаются без перезапуска ядра.
import { ext } from './compat.js';

const frame = document.getElementById('ui');

globalThis.__sbNavigate = (path) => {
  frame.src = path;
  ext.bridge.call('ShowWindow').catch(() => {});
};

frame.addEventListener('load', () => {
  document.getElementById('boot')?.remove();
  frame.hidden = false;
});

// macOS: ядро работает в службе с правами администратора. Пока её нет (или она от
// старой версии) — предлагаем установить, «фон» и окно не запускаем.
async function serviceGate() {
  if (!ext.caps.service) return;
  let st = await ext.bridge.call('ServiceStatus');
  if (st.ok) return;
  document.getElementById('boot')?.remove();
  const gate = document.getElementById('gate');
  const update = st.installed && st.version;
  gate.querySelector('h1').textContent = update ? 'Обновите службу SafeBridge' : 'Остался один шаг';
  gate.querySelector('.text').textContent = update
    ? `Служба SafeBridge установлена от версии ${st.version}, а приложение — ${st.want}. Обновите её, это займёт пару секунд.`
    : 'Чтобы весь трафик компьютера шёл через VPN, SafeBridge один раз устанавливает свою службу. macOS попросит пароль администратора.';
  const btn = gate.querySelector('button');
  const err = gate.querySelector('.err');
  btn.textContent = update ? 'Обновить службу' : 'Установить службу';
  gate.hidden = false;
  await new Promise((resolve) => {
    btn.onclick = async () => {
      btn.disabled = true;
      err.textContent = '';
      try {
        await ext.bridge.call('ServiceInstall');
        st = await ext.bridge.call('ServiceStatus');
        if (st.ok) return resolve();
        err.textContent = st.error || 'Служба не ответила. Перезапустите SafeBridge.';
      } catch (e) {
        const m = String(e?.message || e);
        err.textContent = m === 'отменено' ? '' : m;
      }
      btn.disabled = false;
    };
  });
  gate.remove();
}

(async () => {
  await serviceGate();
  await import('./background.js');
  globalThis.__sbNavigate('popup.html');
  if (ext.caps.tray) (await import('./app-tray.js')).startTray();
})();
