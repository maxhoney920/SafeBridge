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
REPO="${REPO:-maxhoney920/SafeBridge}"
UPDATE_BASE="${UPDATE_BASE:-https://raw.githubusercontent.com/$REPO/downloads/}"
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
    url="https://github.com/XTLS/Xray-core/releases/download/v$XRAY_VERSION/$name.zip"
    curl -fsSL -o "$arc.part" "$url"
    # сверяем SHA-256 с официальным файлом контрольных сумм XTLS
    want=$(curl -fsSL "$url.dgst" | sed -n 's/^SHA2-256= *//p' | tr -d '[:space:]')
    got=$(sha256sum "$arc.part" | cut -d' ' -f1)
    if [ -z "$want" ] || [ "$want" != "$got" ]; then
      echo "  ✗ контрольная сумма $name.zip не совпала (ожидалось ${want:-?}, получено $got)" >&2
      rm -f "$arc.part"; exit 1
    fi
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
    -ldflags "-s -w -X main.version=$VERSION -X main.extensionIDs=$EXTENSION_IDS -X main.bundledXray=$XRAY_VERSION -X main.updateBase=$UPDATE_BASE" -o "../$out" .)
  echo "  ✓ $out ($(du -h "$out" | cut -f1))"
  if [ "$os" = darwin ]; then
    # для кнопки «Скачать»: zip сохраняет право на запуск, двойной клик открывает установку в Терминале
    tmpd=$(mktemp -d); cp "$out" "$tmpd/vlessbridge"; chmod +x "$tmpd/vlessbridge"
    (cd "$tmpd" && zip -q "vlessbridge-macos-$arch.zip" vlessbridge)
    mv "$tmpd/vlessbridge-macos-$arch.zip" "$DIST/"; rm -rf "$tmpd"
    echo "  ✓ $DIST/vlessbridge-macos-$arch.zip"
  fi
done
rm -f host/core/xray.gz

# latest.json — по нему помощник узнаёт о новой версии и проверяет SHA-256 при обновлении
python3 - "$DIST" "$VERSION" "$XRAY_VERSION" <<'PY'
import hashlib, json, os, sys
dist, ver, xray = sys.argv[1:4]
files = {}
for name in sorted(os.listdir(dist)):
    if not name.startswith("vlessbridge-") or name.endswith((".zip", ".xpi")) or name.startswith("vlessbridge-macos"):
        continue
    key = name[len("vlessbridge-"):].removesuffix(".exe")  # windows-amd64, darwin-arm64, linux-amd64
    with open(os.path.join(dist, name), "rb") as f:
        files[key] = {"name": name, "sha256": hashlib.sha256(f.read()).hexdigest()}
json.dump({"version": ver, "xray": xray, "files": files}, open(os.path.join(dist, "latest.json"), "w"), indent=2)
print("✓", os.path.join(dist, "latest.json"), "—", ", ".join(files))
PY

# архив расширения: внутри одна папка VlessBridge с manifest.json
rm -rf "$DIST/VlessBridge" "$DIST/vlessbridge-extension-$VERSION.zip"
cp -r extension "$DIST/VlessBridge"
rm -f "$DIST/VlessBridge/manifest.firefox.json" "$DIST/VlessBridge/background.html"
(cd "$DIST" && zip -qr "vlessbridge-extension-$VERSION.zip" VlessBridge -x '*.DS_Store' && rm -rf VlessBridge)
cp "$DIST/vlessbridge-extension-$VERSION.zip" "$DIST/vlessbridge-extension.zip"  # постоянная ссылка на последнюю версию

# расширение для Firefox: тот же код, манифест V2 и фоновая страница; .xpi — это zip с manifest.json в корне
rm -rf "$DIST/ff" && cp -r extension "$DIST/ff"
# Firefox сам проверяет обновления по update_url (updates.json кладёт CI после подписи)
FF_UPDATE_URL="${FF_UPDATE_URL:-https://raw.githubusercontent.com/$REPO/downloads/updates.json}"
python3 - "$DIST/ff" "$VERSION" "$FF_UPDATE_URL" <<'PY'
import json, os, sys
d, ver, upd = sys.argv[1:4]
m = json.load(open(os.path.join(d, "manifest.firefox.json")))
m["version"] = ver
m["browser_specific_settings"]["gecko"]["update_url"] = upd
json.dump(m, open(os.path.join(d, "manifest.json"), "w"), ensure_ascii=False, indent=2)
os.remove(os.path.join(d, "manifest.firefox.json"))
PY
rm -f "$DIST/vlessbridge-firefox-$VERSION.xpi"
(cd "$DIST/ff" && zip -qr "../vlessbridge-firefox-$VERSION.xpi" . -x '*.DS_Store')
rm -rf "$DIST/ff"
cp "$DIST/vlessbridge-firefox-$VERSION.xpi" "$DIST/vlessbridge-firefox.xpi"
echo "✓ $DIST/vlessbridge-firefox-$VERSION.xpi"
cp install.sh "$DIST/install.sh"
echo "✓ $DIST/vlessbridge-extension-$VERSION.zip"
