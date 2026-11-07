// Напоминание продлить подписку системным уведомлением: окно приложения часто свёрнуто
// или спрятано в строку меню, и полосу «закончится через N дней» в нём не видно.
// Для каждой подписки — не чаще раза в сутки, пока её не продлят (срок не сдвинется).
import { ext } from './compat.js';
import { load, save, expiringSubs, fmtLeft, subExpire, ACCOUNT } from './common.js';

const DAY = 86400e3;

async function check() {
  const st = await load();
  const due = expiringSubs(st);
  if (!due.length) return;
  const sent = { ...(st.expireNotified || {}) };
  let changed = false;
  for (const { sub, left } of due) {
    const key = `${sub.id}:${subExpire(sub)}`;
    if (Date.now() - (sent[key] || 0) < DAY) continue;
    const link = sub.account && st.account ? (st.account.renewUrl || ACCOUNT.bot) : '';
    const text = `Подписка «${sub.name}» ${fmtLeft(left)}.` + (link ? ' Нажмите, чтобы продлить.' : '');
    try {
      await ext.bridge.call('Notify', 'SafeBridge', text, link);
      sent[key] = Date.now();
      changed = true;
    } catch (e) {
      console.warn('notify', e);
    }
  }
  if (changed) {
    // старые отметки (подписка продлена или удалена) больше не нужны
    for (const k of Object.keys(sent)) if (Date.now() - sent[k] > 30 * DAY) delete sent[k];
    await save({ expireNotified: sent });
  }
}

export function startNotify() {
  setTimeout(() => check().catch(() => {}), 20000); // после запуска — когда подписки обновятся
  setInterval(() => check().catch(() => {}), 60 * 60000);
}
