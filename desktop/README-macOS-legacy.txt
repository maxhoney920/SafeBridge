SafeBridge для старых macOS — отдельная сборка
===============================================

Для macOS 10.13 High Sierra, 10.14 Mojave, 10.15 Catalina и 11 Big Sur
(Intel и Apple Silicon). Для macOS 12 и новее используйте обычную сборку —
build-macos.sh и README-macOS.txt.

Чем отличается от обычной:
  • ядро Xray 26.2.6 — последнее, которое работает на этих системах (новые Xray
    собираются Go 1.26, а он работает только с macOS 12). XHTTP, Reality, Vision,
    WebSocket, gRPC, Trojan, VMess, Shadowsocks, Hysteria2 — поддерживаются;
  • ядро обновляется только вместе с приложением (кнопки обновления ядра нет);
  • окно рассчитано и на старый движок Safari 13 — может выглядеть чуть иначе;
  • если сменить сеть (например, с Wi-Fi на кабель) при включённом VPN —
    выключите и включите VPN, чтобы SafeBridge подхватил новую сеть.

Собирать нужно на Mac с macOS 12 или новее (на самой High Sierra не нужно):
  1. Xcode Command Line Tools:  xcode-select --install
  2. Go 1.22.6 или новее: https://go.dev/dl/ (файл .pkg), или  brew install go
  3. В папке desktop исходников SafeBridge:
       ./build-macos-legacy.sh
     Для Mac с High Sierra хватит Intel-версии (вдвое быстрее):
       ARCHS=amd64 ./build-macos-legacy.sh

Первый запуск скрипта — 10–15 минут: он скачает исходники Go 1.25.14, применит
правки для macOS 10.13 (legacy/go1.25-macos-10.13.patch, те же, которыми
собирают sing-box для High Sierra) и соберёт Go; потом соберёт Xray 26.2.6 из
исходников и само приложение. Повторные сборки — быстрее.

Результат: dist/SafeBridge-1.0.0-macos-legacy.zip
Перенесите его на старый Mac, распакуйте, перетащите SafeBridge.app в «Программы».
При первом запуске: правой кнопкой по значку → «Открыть» → «Открыть»
(у приложения нет платного сертификата Apple).

Дальше всё как в обычной версии: SafeBridge попросит один раз установить службу
(пароль администратора), значок в строке меню включается в настройках.

Если что-то не работает — пришлите:
  • весь вывод скрипта сборки (если ошибка при сборке);
  • файл ~/Library/Application Support/SafeBridge/app.log
    и /Library/Application Support/SafeBridge/service/core.log (если ошибка в работе).
