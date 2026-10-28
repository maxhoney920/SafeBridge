#!/usr/bin/env bash
# Сборка помощника со встроенным Xray-core под все платформы + архив расширения.
#
#   ./build.sh                 # все платформы
#   ./build.sh linux/amd64     # только указанные
#
# Переменные окружения:
#   VERSION        версия помощника (по умолчанию из extension/manifest.json)
#   XRAY_VERSION   версия Xray-core (по умолчанию 26.9.30)
#   EXTENSION_IDS  ID расширения через запятую (по умолчанию — ID из ключа в manifest.json;
#                  добавьте сюда ID из Chrome Web Store, когда опубликуете)
set -euo pipefail
cd "$(dirname "$0")"

XRAY_VERSION="${XRAY_VERSION:-26.9.30}"
VERSION="${VERSION:-$(sed -n 's/.*"version": *"\([^"]*\)".*/\1/p' extension/manifest.json | head -1)}"
EXTENSION_IDS="${EXTENSION_IDS:-mpmmjdhamocialllnfmcpkjealmekhpl}"
TARGETS=("$@")
[ ${#TARGETS[@]} -eq 0 ] && TARGETS=(windows/amd64 windows/arm64 darwin/amd64 darwin/arm64 linux/amd64 linux/arm64)

CACHE=.cache/xray-$XRAY_VERSION
DIST=dist
mkdir -p "$CACHE" "$DIST" host/core

fetch_core() { # goos goarch -> путь к бинарнику xray
  local os=$1 arch=$2 xos xarch bin=xray
  case "$os" in windows) xos=windows; bin=xray.exe ;; darwin) xos=macos ;; linux) xos=linux ;; esac
  case "$arch" in amd64) xarch=64 ;; arm64) xarch=arm64-v8a ;; esac
  local name="Xray-$xos-$xarch"
  local arc="$CACHE/$name.zip"
  if [ ! -f "$arc" ]; then
    echo "  ↓ $name.zip" >&2
    curl -fsSL -o "$arc.part" "https://github.com/XTLS/Xray-core/releases/download/v$XRAY_VERSION/$name.zip"
    mv "$arc.part" "$arc"
  fi
  local out="$CACHE/$name/$bin"
  [ -f "$out" ] || unzip -qo "$arc" "$bin" -d "$CACHE/$name"
  echo "$out"
}

for t in "${TARGETS[@]}"; do
  os=${t%/*}; arch=${t#*/}
  echo "▶ $os/$arch (Xray $XRAY_VERSION)"
  core=$(fetch_core "$os" "$arch")
  gzip -9 -c "$core" > host/core/xray.gz
  suffix=""; [ "$os" = windows ] && suffix=".exe"
  out="$DIST/vlessbridge-$os-$arch$suffix"
  (cd host && CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -tags embedcore -trimpath \
    -ldflags "-s -w -X main.version=$VERSION -X main.extensionIDs=$EXTENSION_IDS" -o "../$out" .)
  echo "  ✓ $out ($(du -h "$out" | cut -f1))"
done
rm -f host/core/xray.gz

# архив расширения: внутри одна папка VlessBridge с manifest.json
rm -rf "$DIST/VlessBridge" "$DIST/vlessbridge-extension-$VERSION.zip"
cp -r extension "$DIST/VlessBridge"
(cd "$DIST" && zip -qr "vlessbridge-extension-$VERSION.zip" VlessBridge -x '*.DS_Store' && rm -rf VlessBridge)
cp install.sh "$DIST/install.sh"
echo "✓ $DIST/vlessbridge-extension-$VERSION.zip"
