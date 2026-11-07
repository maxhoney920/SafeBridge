// Обычный скрипт (не модуль), подключается первым на каждой странице приложения.
// Дополняет старые версии WebKit (macOS High Sierra, Safari 13): в новых ничего не делает.
(function () {
  if (typeof globalThis === 'undefined') window.globalThis = window;
  if (!globalThis.structuredClone) {
    globalThis.structuredClone = function (v) { return v === undefined ? undefined : JSON.parse(JSON.stringify(v)); };
  }

  // Отступы gap во flex-раскладке появились в Safari 14.1. Где их нет — расставляем
  // поля у дочерних элементов по тем же значениям gap (страницы перерисовываются,
  // поэтому следим за изменениями).
  function flexGapSupported() {
    var d = document.createElement('div');
    d.style.cssText = 'display:flex;flex-direction:column;row-gap:1px;position:absolute;visibility:hidden';
    d.appendChild(document.createElement('div'));
    d.appendChild(document.createElement('div'));
    document.body.appendChild(d);
    var ok = d.scrollHeight === 1;
    d.parentNode.removeChild(d);
    return ok;
  }
  var forced = false;
  function px(v) { var n = parseFloat(v); return isNaN(n) ? 0 : n; }
  function apply(root) {
    var all = root.querySelectorAll('*');
    for (var i = 0; i < all.length; i++) {
      var el = all[i];
      var cs = getComputedStyle(el);
      if (cs.display !== 'flex' && cs.display !== 'inline-flex') continue;
      var col = cs.flexDirection.indexOf('column') === 0;
      var g, wrapG;
      if (el.dataset.sbGap) { // запомненные значения (имитация в тестах)
        g = +el.dataset.sbGap; wrapG = +el.dataset.sbWrap;
      } else {
        g = px(col ? cs.rowGap : cs.columnGap);
        wrapG = px(col ? cs.columnGap : cs.rowGap);
        if (forced && (g || wrapG)) {
          el.dataset.sbGap = g; el.dataset.sbWrap = wrapG;
          el.style.rowGap = '0px'; el.style.columnGap = '0px';
        }
      }
      if (!g && !wrapG) continue;
      var kids = [];
      for (var c = el.firstElementChild; c; c = c.nextElementSibling) {
        var ccs = getComputedStyle(c);
        if (ccs.position !== 'absolute' && ccs.position !== 'fixed' && ccs.display !== 'none') kids.push(c);
      }
      if (!kids.length) continue;
      if (col) {
        for (var k = 0; k < kids.length; k++) kids[k].style.marginTop = k ? g + 'px' : '';
      } else if (cs.flexWrap === 'nowrap') {
        for (var k = 0; k < kids.length; k++) kids[k].style.marginLeft = k ? g + 'px' : '';
      } else {
        // строка с переносом: поле слева у всех, кроме начинающих строку; сверху — у всех,
        // кроме первой строки
        // поле слева у каждого элемента, а внутренний отступ контейнера (или его поле, если
        // отступа не хватает) меньше на столько же — так строки, и первая, и перенесённые,
        // начинаются от прежнего края, и раскладка не «прыгает»
        if (!el.dataset.sbPadL) el.dataset.sbPadL = px(cs.paddingLeft);
        var pl = +el.dataset.sbPadL;
        if (pl >= g) el.style.paddingLeft = (pl - g) + 'px';
        else { el.style.paddingLeft = '0px'; el.style.marginLeft = (pl - g) + 'px'; }
        for (var k = 0; k < kids.length; k++) { kids[k].style.marginLeft = g + 'px'; kids[k].style.marginTop = ''; }
        var top0 = kids[0].getBoundingClientRect().top;
        for (var k = 0; k < kids.length; k++) if (kids[k].getBoundingClientRect().top > top0 + 1) kids[k].style.marginTop = wrapG + 'px';
      }
    }
  }
  function start() {
    var force = false; // проверка запасного варианта в тестах
    try { force = !!window.top.__sbForceNoGap; } catch (e) {}
    if (!force && flexGapSupported()) return;
    forced = force;
    document.documentElement.classList.add('no-flexgap');
    var pending = false;
    var run = function () { pending = false; apply(document.body); };
    run();
    new MutationObserver(function () {
      if (!pending) { pending = true; requestAnimationFrame(run); }
    }).observe(document.body, { childList: true, subtree: true });
  }
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', start);
  else start();
})();
