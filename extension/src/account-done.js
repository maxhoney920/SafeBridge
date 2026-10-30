// Страница {сайт}/ext/done — последний шаг входа в аккаунт VPN-сервиса.
// Сайт кладёт результат в часть адреса после #, отсюда передаём его расширению.
// Запускается в самом начале загрузки страницы (document_start): забираем токен и сразу
// стираем его из адреса, раньше скриптов страницы и до того, как адрес попадёт в историю.
// Обычный скрипт (не модуль): так он работает как скрипт страницы в любом браузере.
(() => {
  const api = globalThis.browser ?? globalThis.chrome;
  const params = Object.fromEntries(new URLSearchParams(location.hash.slice(1)));
  if (!params.nonce && !params.token && !params.error) return;
  // код ошибки (не привязан Google и т. п.) не секрет — он остаётся, его показывает сама страница
  if (params.token) history.replaceState(null, '', location.pathname);

  const ui = (t, x) => {
    const apply = () => {
      const title = document.getElementById('vb-title');
      const text = document.getElementById('vb-text');
      if (title) title.textContent = t;
      if (text && x != null) text.textContent = x;
    };
    if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', apply, { once: true });
    else apply();
  };
  if (params.token) ui('Входим…', 'Загружаем ваши ключи в SafeBridge.');

  api.runtime.sendMessage({ type: 'accountDone', params }).then((r) => {
    const res = r?.result;
    if (!r?.ok) return ui('Не получилось войти', r?.error || 'Попробуйте ещё раз из окна SafeBridge.');
    if (res?.message) return ui('Не получилось войти', res.message);
    if (res?.ok) {
      ui('Готово!', res.keys
        ? `Ключи добавлены в SafeBridge: ${res.keys}. Нажмите на значок расширения и включите VPN. Вкладка закроется сама.`
        : 'Вы вошли, но активных ключей пока нет. Получите ключ в боте — он появится в SafeBridge сам.');
    }
  }).catch(() => ui('Не получилось войти', 'Расширение SafeBridge не ответило. Обновите его и попробуйте ещё раз.'));
})();
