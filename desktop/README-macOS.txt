SafeBridge для macOS — сборка на своём Mac
==========================================

Подходит для macOS 12 Monterey и новее, на Apple Silicon (M1–M4) и Intel.
Для macOS 10.13–11 (High Sierra, Mojave, Catalina, Big Sur) — отдельная сборка:
build-macos-legacy.sh, инструкция в README-macOS-legacy.txt.

Один раз подготовьте Mac:
  1. Xcode Command Line Tools. В «Терминале»:  xcode-select --install
  2. Go 1.24 или новее: https://go.dev/dl/ (файл .pkg для macOS), или  brew install go

Сборка (5–10 минут при первом запуске, дальше быстрее):
  cd SafeBridge/desktop        ← папка desktop в исходниках SafeBridge
  ./build-macos.sh

Скрипт сам скачает Xray и сверит контрольную сумму, соберёт одно приложение
сразу для Apple Silicon и Intel и положит его в dist/SafeBridge.app.
Только для своего процессора (вдвое быстрее):  ARCHS=arm64 ./build-macos.sh
(для Intel — ARCHS=amd64).

Установка: перетащите dist/SafeBridge.app в «Программы».
На Mac, где собирали, приложение откроется сразу. Если переносите на другой Mac
(архив dist/SafeBridge-1.0.0-macos.zip), при первом запуске нажмите на значок
правой кнопкой → «Открыть» → «Открыть»: у приложения нет платного сертификата Apple.

Первый запуск:
  SafeBridge предложит установить свою службу — нажмите «Установить службу» и
  введите пароль администратора. Это один раз: служба запускает ядро с правами,
  нужными, чтобы через VPN шёл весь трафик компьютера (режим TUN). При обновлении
  SafeBridge он попросит обновить службу.

Строка меню (рядом с часами):
  значок щита показывает, подключён ли VPN; в его меню — включить/выключить,
  выбрать сервер и режим, открыть окно или настройки, выйти.
  Пока значок включён, красная кнопка закрытия только прячет окно — VPN продолжает
  работать. Выйти: меню значка → «Выйти из SafeBridge» или ⌘Q.
  Отключить значок: Настройки → Параметры → «Показывать SafeBridge в строке меню»
  (поведение кнопки закрытия поменяется после перезапуска SafeBridge).

Удалить службу: Настройки → Параметры → «Удалить службу».
Вручную (если приложение уже удалено), в «Терминале»:
  sudo launchctl bootout system/online.internet-safe.safebridge.helper
  sudo rm -f /Library/LaunchDaemons/online.internet-safe.safebridge.helper.plist /Library/PrivilegedHelperTools/online.internet-safe.safebridge.helper
  sudo rm -rf "/Library/Application Support/SafeBridge"

Если что-то пошло не так — пришлите весь вывод из «Терминала».
