#!/bin/bash
# Сборка SafeBridge.app для macOS. Запускать на Mac:
#   ./build-macos.sh                 — универсальное приложение (Apple Silicon + Intel)
#   ARCHS=arm64 ./build-macos.sh     — только для своей архитектуры (быстрее)
# Нужны: Go 1.24+ и Xcode Command Line Tools (xcode-select --install).
# Пишем под bash 3.2, который стоит в macOS по умолчанию.
set -euo pipefail
cd "$(dirname "$0")"
./sync-frontend.sh

VERSION="${VERSION:-$(cat VERSION)}"
XRAY_VERSION="${XRAY_VERSION:-26.9.30}"
ARCHS="${ARCHS:-arm64 amd64}"
# Минимум — macOS 12 Monterey: ядро Xray от XTLS собрано Go 1.26, а он работает
# только на macOS 12 и новее (и на Apple Silicon, и на Intel).
MIN_MACOS="12.0"
BUNDLE_ID="online.internet-safe.safebridge"

say() { printf '\033[1;33m▸ %s\033[0m\n' "$*"; }
die() { printf '\033[1;31m✗ %s\033[0m\n' "$*" >&2; exit 1; }

[ "$(uname -s)" = Darwin ] || die "Этот скрипт запускается на macOS."

# ---------- проверки ----------
command -v go >/dev/null 2>&1 || die "Не найден Go. Установите с https://go.dev/dl/ (файл .pkg) или: brew install go"
gover=$(go env GOVERSION | sed 's/^go//')
gomaj=$(echo "$gover" | cut -d. -f1); gomin=$(echo "$gover" | cut -d. -f2)
if [ "$gomaj" -lt 1 ] || { [ "$gomaj" -eq 1 ] && [ "$gomin" -lt 24 ]; }; then
  die "Нужен Go 1.24 или новее, установлен $gover. Обновите: https://go.dev/dl/"
fi
xcode-select -p >/dev/null 2>&1 || die "Нет Xcode Command Line Tools. Выполните: xcode-select --install — и запустите скрипт снова."
command -v clang >/dev/null 2>&1 || die "Не найден clang (Xcode Command Line Tools)."

# модули golang.org/x подменены в go.mod на зеркала GitHub с закреплёнными версиями
export GOFLAGS=-mod=mod GOSUMDB=off
export GOPROXY="${GOPROXY:-https://proxy.golang.org,direct}"

CACHE=".cache/xray-$XRAY_VERSION"
mkdir -p "$CACHE" dist internal/host/core
BUILD=$(mktemp -d)
trap 'rm -rf "$BUILD" internal/host/core/xray.gz' EXIT

# ---------- Xray: скачиваем и сверяем SHA-256 ----------
fetch_xray() { # $1 — имя архива в релизе XTLS
  local zip="$CACHE/$1" want got
  if [ ! -f "$zip" ]; then
    say "Скачиваю $1 (Xray $XRAY_VERSION)"
    curl -fL --progress-bar -o "$zip.part" "https://github.com/XTLS/Xray-core/releases/download/v$XRAY_VERSION/$1"
    curl -fsSL -o "$zip.dgst" "https://github.com/XTLS/Xray-core/releases/download/v$XRAY_VERSION/$1.dgst"
    mv "$zip.part" "$zip"
  fi
  want=$(sed -n 's/^SHA2-256= *\([0-9a-f]\{64\}\).*/\1/p' "$zip.dgst" | head -1)
  got=$(shasum -a 256 "$zip" | cut -d' ' -f1)
  if [ -z "$want" ] || [ "$want" != "$got" ]; then
    rm -f "$zip" "$zip.dgst"
    die "Контрольная сумма $1 не совпала — файл удалён, запустите скрипт ещё раз."
  fi
}

# ---------- сборка под каждую архитектуру ----------
bins=()
for arch in $ARCHS; do
  case "$arch" in
    arm64) xzip=Xray-macos-arm64-v8a.zip; carch=arm64 ;;
    amd64) xzip=Xray-macos-64.zip; carch=x86_64 ;;
    *) die "Неизвестная архитектура: $arch (arm64 или amd64)" ;;
  esac
  fetch_xray "$xzip"
  unzip -p "$CACHE/$xzip" xray | gzip -9c > internal/host/core/xray.gz
  [ -s internal/host/core/xray.gz ] || die "В архиве $xzip нет xray"

  say "Собираю SafeBridge для $arch"
  CGO_ENABLED=1 GOOS=darwin GOARCH=$arch \
  CC="clang -arch $carch" \
  CGO_CFLAGS="-mmacosx-version-min=$MIN_MACOS" \
  CGO_LDFLAGS="-mmacosx-version-min=$MIN_MACOS -framework UniformTypeIdentifiers" \
  MACOSX_DEPLOYMENT_TARGET=$MIN_MACOS \
    go build -tags desktop,production,embedcore -trimpath \
      -ldflags "-s -w -X main.version=$VERSION -X github.com/safebridge/desktop/internal/host.version=$VERSION -X github.com/safebridge/desktop/internal/host.bundledXray=$XRAY_VERSION" \
      -o "$BUILD/SafeBridge-$arch" .
  bins+=("$BUILD/SafeBridge-$arch")
done

# ---------- SafeBridge.app ----------
APP="dist/SafeBridge.app"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
if [ "${#bins[@]}" -gt 1 ]; then
  lipo -create -output "$APP/Contents/MacOS/SafeBridge" "${bins[@]}"
else
  cp "${bins[0]}" "$APP/Contents/MacOS/SafeBridge"
fi
chmod +x "$APP/Contents/MacOS/SafeBridge"
cp darwin/icon.icns "$APP/Contents/Resources/icon.icns"
cat > "$APP/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key><string>SafeBridge</string>
  <key>CFBundleDisplayName</key><string>SafeBridge</string>
  <key>CFBundleExecutable</key><string>SafeBridge</string>
  <key>CFBundleIdentifier</key><string>$BUNDLE_ID</string>
  <key>CFBundleIconFile</key><string>icon</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>$VERSION</string>
  <key>CFBundleVersion</key><string>$VERSION</string>
  <key>CFBundleDevelopmentRegion</key><string>ru</string>
  <key>LSMinimumSystemVersion</key><string>$MIN_MACOS</string>
  <key>LSApplicationCategoryType</key><string>public.app-category.utilities</string>
  <key>NSHighResolutionCapable</key><true/>
  <key>NSSupportsAutomaticGraphicsSwitching</key><true/>
  <key>NSHumanReadableCopyright</key><string>Сделано с любовью safe-internet ♥</string>
</dict>
</plist>
PLIST

# подпись без сертификата разработчика (ad-hoc): на Apple Silicon без подписи приложение не запустится
say "Подписываю (ad-hoc)"
codesign --force --deep --sign - "$APP"
codesign --verify --deep "$APP"
xattr -cr "$APP" 2>/dev/null || true

ZIP="dist/SafeBridge-$VERSION-macos.zip"
rm -f "$ZIP"
ditto -c -k --keepParent "$APP" "$ZIP"

printf '\n\033[1;32m✓ Готово:\033[0m %s\n' "$APP"
echo "  Архивы: $(lipo -archs "$APP/Contents/MacOS/SafeBridge")"
echo "  Для передачи на другой Mac: $ZIP"
echo "  Установка: перетащите SafeBridge.app в «Программы»."
