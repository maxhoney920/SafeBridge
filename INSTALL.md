# Установка VlessBridge

Нужны две вещи: **расширение** для браузера и **помощник** — небольшая программа, через которую расширение подключается к серверам. Помощник ставится один раз, права администратора не нужны. Дальше он и ядро Xray обновляются сами.

Подходят Chrome, Edge, Brave, Яндекс Браузер, Vivaldi и Chromium на Windows, macOS и Linux.

## 1. Расширение

1. Скачайте [vlessbridge-extension.zip](https://github.com/maxhoney920/SafeBridge/raw/downloads/vlessbridge-extension.zip) и распакуйте его. Папку `VlessBridge` никуда не переносите и не удаляйте: браузер загружает расширение из неё.
2. Откройте страницу расширений: `chrome://extensions` (Edge — `edge://extensions`, Яндекс Браузер — `browser://extensions`).
3. Включите «Режим разработчика» (переключатель справа вверху).
4. Нажмите «Загрузить распакованное» и выберите папку `VlessBridge` — ту, внутри которой лежит `manifest.json`.
5. Закрепите значок расширения на панели (значок пазла → булавка).

Расширение само предложит установить помощник и покажет кнопку для вашей системы. Ниже — то же самое подробно.

## 2. Помощник

Прямые ссылки на файлы:

| Система | Процессор | Файл |
|---|---|---|
| Windows | Intel / AMD | [vlessbridge-windows-amd64.exe](https://github.com/maxhoney920/SafeBridge/raw/downloads/vlessbridge-windows-amd64.exe) |
| Windows | ARM (Snapdragon) | [vlessbridge-windows-arm64.exe](https://github.com/maxhoney920/SafeBridge/raw/downloads/vlessbridge-windows-arm64.exe) |
| macOS | Apple Silicon (M1–M4) | [vlessbridge-macos-arm64.zip](https://github.com/maxhoney920/SafeBridge/raw/downloads/vlessbridge-macos-arm64.zip) |
| macOS | Intel | [vlessbridge-macos-amd64.zip](https://github.com/maxhoney920/SafeBridge/raw/downloads/vlessbridge-macos-amd64.zip) |
| Linux | x86-64 | [vlessbridge-linux-amd64](https://github.com/maxhoney920/SafeBridge/raw/downloads/vlessbridge-linux-amd64) |
| Linux | ARM64 | [vlessbridge-linux-arm64](https://github.com/maxhoney920/SafeBridge/raw/downloads/vlessbridge-linux-arm64) |

### Windows

1. Скачайте `vlessbridge-windows-amd64.exe` и откройте его.
2. Если появится «Система Windows защитила ваш компьютер», нажмите «Подробнее», затем «Выполнить в любом случае». Предупреждение возникает потому, что у файла нет платной цифровой подписи.
3. В окне появится отчёт об установке и слово «Готово». Нажмите Enter.
4. Вернитесь в окно расширения и нажмите «Проверить».

### macOS

**Проще всего — одной командой.** Откройте Терминал (Программы → Утилиты → Терминал), вставьте и нажмите Return:

```sh
curl -fsSL https://github.com/maxhoney920/SafeBridge/raw/downloads/install.sh | sh
```

Команда сама выберет файл под ваш процессор. Разрешать запуск в настройках при этом не нужно.

**Или вручную:**

1. Скачайте `vlessbridge-macos-arm64.zip` (на Mac с Intel — `-amd64.zip`) и откройте архив двойным щелчком — рядом появится файл `vlessbridge`.
2. Откройте `vlessbridge` двойным щелчком. macOS скажет, что не может проверить разработчика, — нажмите «Готово».
3. Откройте Системные настройки → «Конфиденциальность и безопасность», внизу нажмите «Всё равно открыть» и подтвердите паролем.
4. В окне Терминала появится «Готово». Вернитесь в окно расширения и нажмите «Проверить».

### Linux

Одной командой:

```sh
curl -fsSL https://github.com/maxhoney920/SafeBridge/raw/downloads/install.sh | sh
```

Или вручную:

```sh
chmod +x vlessbridge-linux-amd64
./vlessbridge-linux-amd64
```

Вернитесь в окно расширения и нажмите «Проверить».

## Что делает установка

Помощник копирует себя в папку профиля и регистрируется во всех найденных Chromium-браузерах:

| Система | Где лежит помощник | Где регистрируется |
|---|---|---|
| Windows | `%LOCALAPPDATA%\VlessBridge` | ключи `HKCU\Software\…\NativeMessagingHosts\com.vlessbridge.host` |
| macOS | `~/Library/Application Support/VlessBridge` | `~/Library/Application Support/<браузер>/NativeMessagingHosts/` |
| Linux | `~/.local/share/vlessbridge` | `~/.config/<браузер>/NativeMessagingHosts/` |

Скачанный файл после установки можно удалить. Если браузер был открыт во время установки и расширение по-прежнему просит установить помощник — нажмите «Проверить» или перезапустите браузер.

## Обновления

Раз в сутки расширение проверяет обновления. Ядро Xray обновляется само (это можно выключить в настройках → «Обновления»). О новой версии помощника расширение сообщит, обновление — одной кнопкой.

## Удаление

1. Удалите расширение на странице `chrome://extensions`.
2. Удалите помощник:
   - Windows (Win+R → `cmd`): `"%LOCALAPPDATA%\VlessBridge\vlessbridge.exe" uninstall`
   - macOS: `~/Library/Application\ Support/VlessBridge/vlessbridge uninstall`
   - Linux: `~/.local/share/vlessbridge/vlessbridge uninstall`

## Если что-то не работает

- **«Установите помощник» не исчезает** — перезапустите браузер. Если вы пользуетесь браузером, которого нет в списке выше, напишите, какой у вас браузер.
- **«Настройками прокси управляет другое расширение»** — отключите другие VPN- и прокси-расширения: управлять прокси может только одно.
- **Сервер серый с пометкой «не поддерживается»** — Xray не может с ним работать, причина написана под названием.
- **Журнал ядра** — настройки расширения → «Обновления» → «Показать журнал ядра».
