# SafeBridge для компьютера

Приложение на [Wails](https://wails.io): окно — тот же интерфейс, что у расширения
(`../extension`), ядро — тот же код, что у помощника (`internal/host`), плюс системный
прокси, режим «Весь трафик» (TUN), служба для macOS и значок в строке меню.

## Как устроено

- `frontend-app/` — то, что есть только у приложения: окно-оболочка, мост к Go вместо
  API браузера (`compat.js`, `bridge.js`), значок в строке меню. Остальной интерфейс
  `sync-frontend.sh` берёт из `../extension` перед каждой сборкой (папка `frontend/`
  в git не хранится).
- `internal/host` — ядро Xray, подписки, пинг, TUN; `internal/svc` — служба для macOS
  (ядро от root, окно от пользователя, связь по unix-сокету); `internal/sysproxy` —
  системный прокси; `internal/tray` — значок в строке меню macOS.
- Версия приложения — в файле `VERSION`.

## Сборка

| Что | Где | Команда |
|---|---|---|
| Windows 10/11, Windows 7 (64 и 32 бита) | Linux, CI | `./build.sh` (сначала `../build.sh` — он скачивает и проверяет Xray и Go для Windows 7) |
| macOS 12+ | Mac | `./build-macos.sh` — см. README-macOS.txt |
| macOS 10.13–11 | Mac с macOS 12+ | `./build-macos-legacy.sh` — см. README-macOS-legacy.txt |

Разработка без окна: `./sync-frontend.sh && go run -tags webdev .` — интерфейс
откроется в браузере на http://127.0.0.1:28200.

## Выпуск новой версии

1. Поднимите номер в `VERSION`, отправьте в `main` — CI соберёт Windows-версии,
   добавит их в подписанный `latest.json` (поле `app`) и выложит в ветку `downloads`.
2. Соберите на Mac `build-macos.sh` и `build-macos-legacy.sh`, положите
   `dist/SafeBridge-<версия>-macos.zip` и `…-macos-legacy.zip` в ветку `downloads`
   и перезапустите сборку (Actions → build → Run workflow): CI перенесёт их в публикацию
   и впишет в `latest.json`.

Приложения проверяют новую версию раз в 6 часов и ставят её только при верной подписи
`latest.json` и совпадении SHA-256 файла.
