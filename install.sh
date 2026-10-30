#!/bin/sh
# Установка помощника SafeBridge на macOS и Linux.
# Все переменные в ${...}: старый bash на macOS иначе склеивает имя с соседним юникодом.
#   curl -fsSL https://github.com/maxhoney920/SafeBridge/raw/downloads/install.sh | sh
set -eu

REPO="${VLESSBRIDGE_REPO:-maxhoney920/SafeBridge}"
BASE="https://github.com/${REPO}/raw/downloads"

case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) echo "Эта система не поддерживается: $(uname -s)"; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) echo "Эта архитектура не поддерживается: $(uname -m)"; exit 1 ;;
esac

tmp=$(mktemp -d)
trap 'rm -rf "${tmp}"' EXIT
echo "Скачиваю помощник для ${os}/${arch}..."
curl -fL --progress-bar -o "${tmp}/vlessbridge" "${BASE}/safebridge-${os}-${arch}"
chmod +x "${tmp}/vlessbridge"
# на macOS снимаем карантин, иначе Gatekeeper не даст запустить неподписанный файл
if [ "${os}" = darwin ]; then
  xattr -d com.apple.quarantine "${tmp}/vlessbridge" 2>/dev/null || true
fi
"${tmp}/vlessbridge" install
