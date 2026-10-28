#!/usr/bin/env bash
# Сборка помощника со встроенным sing-box под все платформы + архив расширения.
#
#   ./build.sh                 # все платформы
#   ./build.sh linux/amd64     # только указанные
#
# Переменные окружения:
#   VERSION          версия помощника (по умолчанию из extension/manifest.json)
#   SINGBOX_VERSION  версия sing-box (по умолчанию 1.14.4)
#   EXTENSION_IDS    ID расширения через запятую (по умолчанию — ID из ключа в manifest.json;
#                    добавьте сюда ID из Chrome Web Store, когда опубликуете)
set -euo pipefail
cd "$(dirname "$0")"

SINGBOX_VERSION="${SINGBOX_VERSION:-1.14.4}"
VERSION="${VERSION:-$(sed -n 's/.*"version": *"\([^"]*\)".*/\1/p' extension/manifest.json | head -1)}"
EXTENSION_IDS="${EXTENSION_IDS:-mpmmjdhamocialllnfmcpkjealmekhpl}"
TARGETS=("$@")
[ ${#TARGETS[@]} -eq 0 ] && TARGETS=(windows/amd64 windows/arm64 darwin/amd64 darwin/arm64 linux/amd64 linux/arm64)

CACHE=.cache/sing-box-$SINGBOX_VERSION
DIST=dist
mkdir -p "$CACHE" "$DIST" host/core

fetch_core() { # os arch -> путь к бинарнику sing-box
  local os=$1 arch=$2 ext=tar.gz bin=sing-box
  [ "$os" = windows ] && ext=zip && bin=sing-box.exe
  local name="sing-box-$SINGBOX_VERSION-$os-$arch"
  local arc="$CACHE/$name.$ext"
  if [ ! -f "$arc" ]; then
    echo "  ↓ $name.$ext" >&2
    curl -fsSL -o "$arc.part" "https://github.com/SagerNet/sing-box/releases/download/v$SINGBOX_VERSION/$name.$ext"
    mv "$arc.part" "$arc"
  fi
  local out="$CACHE/$name/$bin"
  if [ ! -f "$out" ]; then
    if [ "$ext" = zip ]; then unzip -qo "$arc" -d "$CACHE"; else tar -xzf "$arc" -C "$CACHE"; fi
  fi
  echo "$out"
}

for t in "${TARGETS[@]}"; do
  os=${t%/*}; arch=${t#*/}
  echo "▶ $os/$arch (sing-box $SINGBOX_VERSION)"
  core=$(fetch_core "$os" "$arch")
  gzip -9 -c "$core" > host/core/sing-box.gz
  suffix=""; [ "$os" = windows ] && suffix=".exe"
  out="$DIST/vlessbridge-$os-$arch$suffix"
  (cd host && CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -tags embedcore -trimpath \
    -ldflags "-s -w -X main.version=$VERSION -X main.extensionIDs=$EXTENSION_IDS" -o "../$out" .)
  echo "  ✓ $out ($(du -h "$out" | cut -f1))"
done
rm -f host/core/sing-box.gz

# архив расширения (для «Загрузить распакованное» и для Chrome Web Store)
rm -f "$DIST/vlessbridge-extension-$VERSION.zip"
(cd extension && zip -qr "../$DIST/vlessbridge-extension-$VERSION.zip" . -x '*.DS_Store')
cp install.sh "$DIST/install.sh"
echo "✓ $DIST/vlessbridge-extension-$VERSION.zip"
