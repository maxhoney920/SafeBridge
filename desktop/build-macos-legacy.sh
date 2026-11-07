#!/bin/bash
# SafeBridge для старых macOS — 10.13 High Sierra, 10.14 Mojave, 10.15 Catalina, 11 Big Sur.
# Отдельная сборка (обычная — build-macos.sh, для macOS 12+). Запускать на любом Mac с
# macOS 12 или новее (собирать на самой High Sierra не нужно):
#   ./build-macos-legacy.sh                 — Intel + Apple Silicon
#   ARCHS=amd64 ./build-macos-legacy.sh     — только Intel (все Mac с High Sierra — Intel)
# Нужны: Go 1.22.6+ (им собирается Go для старых macOS), git, Xcode Command Line Tools.
#
# Как устроено:
#  • Go 1.25.14 с двумя правками от SagerNet (возвращают поддержку macOS 10.13; так же
#    собирают sing-box для High Sierra) — legacy/go1.25-macos-10.13.patch;
#  • ядро Xray 26.2.6 — последнее, которое собирается Go 1.25 (новые требуют Go 1.26,
#    а он работает только на macOS 12+). XHTTP, Reality, Vision, Hysteria2 — есть.
#  • приложение собирается тем же Go с целевой версией macOS 10.13.
# Пишем под bash 3.2, который стоит в macOS по умолчанию.
set -euo pipefail
cd "$(dirname "$0")"
./sync-frontend.sh

VERSION="${VERSION:-$(cat VERSION)}"
GO_TAG="go1.25.14"
XRAY_TAG="v26.2.6"
ARCHS="${ARCHS:-amd64 arm64}"
MIN_MACOS="10.13"
BUNDLE_ID="online.internet-safe.safebridge"
CACHE=".cache/legacy"
OUT="dist/macos-legacy"

say() { printf '\033[1;33m▸ %s\033[0m\n' "$*"; }
die() { printf '\033[1;31m✗ %s\033[0m\n' "$*" >&2; exit 1; }

[ "$(uname -s)" = Darwin ] || die "Этот скрипт запускается на macOS."
command -v git >/dev/null 2>&1 || die "Не найден git. Выполните: xcode-select --install"
xcode-select -p >/dev/null 2>&1 || die "Нет Xcode Command Line Tools. Выполните: xcode-select --install — и запустите скрипт снова."
command -v clang >/dev/null 2>&1 || die "Не найден clang (Xcode Command Line Tools)."
command -v go >/dev/null 2>&1 || die "Не найден Go. Установите с https://go.dev/dl/ (файл .pkg) или: brew install go"
export GOTOOLCHAIN=local
BOOT_GOROOT=$(go env GOROOT)

mkdir -p "$CACHE" "$OUT" internal/host/core
BUILD=$(mktemp -d)
trap 'rm -rf "$BUILD" internal/host/core/xray.gz' EXIT

# ---------- 1. Go 1.25.14 для старых macOS ----------
LGOROOT="$PWD/$CACHE/go-$GO_TAG-macos10.13"
LGO="$LGOROOT/bin/go"
if [ ! -x "$LGO" ] || [ ! -f "$LGOROOT/.safebridge-patched" ]; then
  say "Готовлю Go $GO_TAG для macOS 10.13 (один раз, 3–5 минут)"
  rm -rf "$LGOROOT"
  git clone -q --depth 1 --branch "$GO_TAG" https://github.com/golang/go.git "$LGOROOT"
  git -C "$LGOROOT" apply "$PWD/legacy/go1.25-macos-10.13.patch" || die "Правки Go не применились"
  (cd "$LGOROOT/src" && GOROOT_BOOTSTRAP="$BOOT_GOROOT" ./make.bash) || die "Не удалось собрать Go $GO_TAG"
  touch "$LGOROOT/.safebridge-patched"
fi
"$LGO" version

# ---------- 2. Xray 26.2.6 из исходников ----------
XSRC="$CACHE/xray-$XRAY_TAG"
if [ ! -d "$XSRC/.git" ]; then
  say "Скачиваю исходники Xray $XRAY_TAG"
  rm -rf "$XSRC"
  git clone -q --depth 1 --branch "$XRAY_TAG" https://github.com/XTLS/Xray-core.git "$XSRC"
fi

# ---------- 3. сборка под каждую архитектуру ----------
bins=()
for arch in $ARCHS; do
  case "$arch" in
    amd64) carch=x86_64 ;;
    arm64) carch=arm64 ;;
    *) die "Неизвестная архитектура: $arch (amd64 или arm64)" ;;
  esac

  say "Собираю ядро Xray $XRAY_TAG для $arch"
  # зависимости Xray проверяются по его go.sum (обычный GOPROXY и GOSUMDB)
  (cd "$XSRC" && env -u GOFLAGS CGO_ENABLED=0 GOOS=darwin GOARCH=$arch \
    "$LGO" build -trimpath -ldflags "-s -w -buildid=" -o "$BUILD/xray-$arch" ./main)
  gzip -9c "$BUILD/xray-$arch" > internal/host/core/xray.gz

  say "Собираю SafeBridge для $arch (macOS $MIN_MACOS+)"
  # модули golang.org/x подменены в go.mod на зеркала GitHub с закреплёнными версиями
  GOFLAGS=-mod=mod GOSUMDB=off \
  CGO_ENABLED=1 GOOS=darwin GOARCH=$arch \
  CC="clang -arch $carch" \
  CGO_CFLAGS="-mmacosx-version-min=$MIN_MACOS" \
  CGO_LDFLAGS="-mmacosx-version-min=$MIN_MACOS -weak_framework UniformTypeIdentifiers" \
  MACOSX_DEPLOYMENT_TARGET=$MIN_MACOS \
    "$LGO" build -tags desktop,production,embedcore -trimpath \
      -ldflags "-s -w -X main.version=$VERSION -X github.com/safebridge/desktop/internal/host.version=$VERSION -X github.com/safebridge/desktop/internal/host.bundledXray=${XRAY_TAG#v} -X github.com/safebridge/desktop/internal/host.variant=macos-legacy" \
      -o "$BUILD/SafeBridge-$arch" .
  bins+=("$BUILD/SafeBridge-$arch")
done

# ---------- 4. SafeBridge.app ----------
APP="$OUT/SafeBridge.app"
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

say "Подписываю (ad-hoc)"
codesign --force --deep --sign - "$APP"
codesign --verify --deep "$APP"
xattr -cr "$APP" 2>/dev/null || true

ZIP="dist/SafeBridge-$VERSION-macos-legacy.zip"
rm -f "$ZIP"
ditto -c -k --keepParent "$APP" "$ZIP"

printf '\n\033[1;32m✓ Готово:\033[0m %s\n' "$APP"
echo "  Архитектуры: $(lipo -archs "$APP/Contents/MacOS/SafeBridge")"
echo "  Для High Sierra / Mojave / Catalina / Big Sur: $ZIP"
echo "  Перенесите архив на старый Mac, распакуйте и перетащите SafeBridge.app в «Программы»."
