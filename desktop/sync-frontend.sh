#!/bin/sh
# Интерфейс приложения — тот же, что у расширения: берём файлы из ../extension и
# добавляем свои (окно-оболочка, мост к Go, значок в строке меню) из frontend-app/.
# Папка frontend/ собирается заново перед каждой сборкой и в git не хранится.
set -eu
cd "$(dirname "$0")"
rm -rf frontend
mkdir -p frontend/src
for f in ui.css popup.css options.css popup.html options.html; do cp "../extension/$f" frontend/; done
cp -R ../extension/fonts ../extension/icons frontend/
for f in background.js common.js presets.js popup.js options.js polyfills.js; do cp "../extension/src/$f" frontend/src/; done
cp -R frontend-app/. frontend/
