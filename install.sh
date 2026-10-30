#!/bin/sh
# Установка помощника SafeBridge на macOS и Linux.
# Все переменные в ${...}: старый bash на macOS иначе склеивает имя с соседним юникодом.
#   curl -fsSL https://github.com/maxhoney920/SafeBridge/raw/downloads/install.sh | sh
set -eu

REPO="${SAFEBRIDGE_REPO:-${VLESSBRIDGE_REPO:-maxhoney920/SafeBridge}}"
BASE="${SAFEBRIDGE_BASE:-https://github.com/${REPO}/raw/downloads}"
# открытый ключ, которым CI подписывает latest.json (тот же, что встроен в помощник)
PUBKEY="MCowBQYDK2VwAyEALUl1+Ht0rVjVYOcryvMZqrWJpybkDRyiD3nALSSZYPs="

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
bust="t=$(date +%s)"

# latest.json: имя файла и SHA-256. Подпись проверяем, если openssl это умеет
# (OpenSSL 3 в Linux да, LibreSSL в macOS нет — там остаётся проверка SHA-256).
curl -fsSL -o "${tmp}/latest.json" "${BASE}/latest.json?${bust}"
if curl -fsSL -o "${tmp}/latest.sig.b64" "${BASE}/latest.json.sig?${bust}" 2>/dev/null \
  && command -v openssl >/dev/null 2>&1 && openssl pkeyutl -help 2>&1 | grep -q rawin; then
  printf -- '-----BEGIN PUBLIC KEY-----\n%s\n-----END PUBLIC KEY-----\n' "${PUBKEY}" > "${tmp}/pub.pem"
  openssl base64 -d -A -in "${tmp}/latest.sig.b64" -out "${tmp}/latest.sig"
  if ! openssl pkeyutl -verify -pubin -inkey "${tmp}/pub.pem" -rawin -in "${tmp}/latest.json" -sigfile "${tmp}/latest.sig" >/dev/null 2>&1; then
    echo "Подпись latest.json не совпала — установка отменена."
    exit 1
  fi
  echo "Подпись проверена."
fi

key="${os}-${arch}"
# из latest.json берём блок нужной платформы: имя файла и его SHA-256
block=$(tr -d '\n' < "${tmp}/latest.json" | sed -n "s/.*\"${key}\": *{\([^}]*\)}.*/\1/p")
name=$(printf '%s' "${block}" | sed -n 's/.*"name": *"\([A-Za-z0-9._-]*\)".*/\1/p')
want=$(printf '%s' "${block}" | sed -n 's/.*"sha256": *"\([0-9a-f]*\)".*/\1/p')
if [ -z "${name}" ] || [ -z "${want}" ]; then
  echo "Для ${os}/${arch} помощник не опубликован."
  exit 1
fi

echo "Скачиваю помощник для ${os}/${arch}..."
curl -fL --progress-bar -o "${tmp}/safebridge" "${BASE}/${name}?${bust}"
if command -v sha256sum >/dev/null 2>&1; then got=$(sha256sum "${tmp}/safebridge" | cut -d' ' -f1)
else got=$(shasum -a 256 "${tmp}/safebridge" | cut -d' ' -f1); fi
if [ "${got}" != "${want}" ]; then
  echo "Контрольная сумма не совпала — установка отменена. Если версия вышла только что, повторите через 5 минут."
  exit 1
fi
chmod +x "${tmp}/safebridge"
# на macOS снимаем карантин, иначе Gatekeeper не даст запустить неподписанный файл
if [ "${os}" = darwin ]; then
  xattr -d com.apple.quarantine "${tmp}/safebridge" 2>/dev/null || true
fi
"${tmp}/safebridge" install
