// Страница {сайт}/ext/done — последний шаг входа в аккаунт VPN-сервиса.
// Сайт кладёт результат в часть адреса после #, отсюда передаём его расширению.
// Обычный скрипт (не модуль): так он работает как скрипт страницы в любом браузере.
(() => {
  const api = globalThis.browser ?? globalThis.chrome;
  const params = Object.fromEntries(new URLSearchParams(location.hash.slice(1)));
  if (!params.nonce && !params.token && !params.error) return;
  // убираем токен из адреса и истории браузера
  history.replaceState(null, '', location.pathname);

  const title = document.getElementById('vb-title');
  const text = document.getElementById('vb-text');
  const show = (t, x) => {
    if (title) title.textContent = t;
    if (text && x != null) text.textContent = x;
  };
  if (params.token) show('Входим…', 'Загружаем ваши ключи в SafeBridge.');

  api.runtime.sendMessage({ type: 'accountDone', params }).then((r) => {
    const res = r?.result;
    if (!r?.ok) return show('Не получилось войти', r?.error || 'Попробуйте ещё раз из окна SafeBridge.');
    if (res?.message) return show('Не получилось войти', res.message);
    if (res?.ok) {
      show('Готово!', res.keys
        ? `Ключи добавлены в SafeBridge: ${res.keys}. Нажмите на значок расширения и включите VPN. Вкладка закроется сама.`
        : 'Вы вошли, но активных ключей пока нет. Получите ключ в боте — он появится в SafeBridge сам.');
    }
    // ошибки (не привязан Google и т. п.) страница сайта уже показала сама
  }).catch(() => show('Не получилось войти', 'Расширение SafeBridge не ответило. Обновите его и попробуйте ещё раз.'));
})();
