#!/usr/bin/env bash
# Сборка SafeBridge для компьютера (Windows). Для macOS — build-macos.sh на самом Mac
# (Wails использует Cocoa/WebKit системы, нужен CGO).
#   ./build.sh                          — все варианты
#   ./build.sh windows7/386             — только нужные
# Windows 7: обычный Go с версии 1.21 её не поддерживает — собираем пропатченным Go
# от XTLS (им же собраны их сборки Xray-win7), ядро — Xray-win7.
set -euo pipefail
cd "$(dirname "$0")"
VERSION="${VERSION:-$(cat VERSION)}"
XRAY_VERSION="${XRAY_VERSION:-26.9.30}"
XRAY_CACHE="${XRAY_CACHE:-../.cache/xray-$XRAY_VERSION}"
WINRES="${WINRES:-$(command -v go-winres || true)}"
GO_WIN7_VERSION="${GO_WIN7_VERSION:-1.26.7}"
GO_WIN7_SHA256="${GO_WIN7_SHA256:-3ec8549eab338720c1a891f37c6d2467e7205997554bf649527d083f9973b82d}"
TARGETS=("$@")
[ ${#TARGETS[@]} -eq 0 ] && TARGETS=(windows/amd64 windows/arm64 windows7/amd64 windows7/386)
export GOPROXY="${GOPROXY:-direct}" GOSUMDB=off GOFLAGS=-mod=mod

go_win7() {
  local dir="${GO_WIN7_DIR:-../.cache/go-win7-$GO_WIN7_VERSION}"
  if [ ! -x "$dir/bin/go" ]; then
    echo "  ↓ go-win7 $GO_WIN7_VERSION" >&2
    mkdir -p "$(dirname "$dir")"
    curl -fsSL --retry 5 --retry-delay 5 -o "$dir.zip.part" "https://github.com/XTLS/go-win7/releases/download/patched-$GO_WIN7_VERSION/go-for-win7-linux-amd64.zip"
    [ "$(sha256sum "$dir.zip.part" | cut -d' ' -f1)" = "$GO_WIN7_SHA256" ] || { echo "контрольная сумма go-win7 не совпала" >&2; rm -f "$dir.zip.part"; exit 1; }
    rm -rf "$dir" && mkdir -p "$dir" && unzip -q "$dir.zip.part" -d "$dir" && rm -f "$dir.zip.part"
  fi
  echo "$(cd "$dir" && pwd)/bin/go"
}

./sync-frontend.sh
mkdir -p dist internal/host/core
trap 'rm -f internal/host/core/xray.gz internal/host/core/wintun.dll rsrc_windows_*.syso' EXIT
for t in "${TARGETS[@]}"; do
  os=${t%/*}; arch=${t#*/}
  case "$os/$arch" in
    windows/amd64) xname=Xray-windows-64; label=windows-x64 ;;
    windows/arm64) xname=Xray-windows-arm64-v8a; label=windows-arm64 ;;
    windows7/amd64) xname=Xray-win7-64; label=windows7-x64 ;;
    windows7/386) xname=Xray-win7-32; label=windows7-x86 ;;
    *) echo "неизвестная цель $t"; exit 1 ;;
  esac
  gobin=go; variant=""
  if [ "$os" = windows7 ]; then gobin=$(go_win7); variant=win7; fi
  xdir=$XRAY_CACHE/$xname
  [ -f "$xdir/xray.exe" ] || unzip -qo "$xdir.zip" xray.exe -d "$xdir" 2>/dev/null || { echo "нет $xdir.zip (сначала запустите ../build.sh — он скачает и проверит Xray)"; exit 1; }
  gzip -9c "$xdir/xray.exe" > internal/host/core/xray.gz
  unzip -p "$xdir.zip" wintun.dll > internal/host/core/wintun.dll
  rm -f rsrc_windows_*.syso
  if [ -n "$WINRES" ]; then
    "$WINRES" make --in winres/winres.json --arch "$arch" --out rsrc --product-version "$VERSION.0" --file-version "$VERSION.0"
  fi
  out="dist/SafeBridge-$VERSION-$label.exe"
  CGO_ENABLED=0 GOOS=windows GOARCH=$arch GOTOOLCHAIN=local "$gobin" build -tags desktop,production,embedcore -trimpath \
    -ldflags "-s -w -H windowsgui -X main.version=$VERSION -X github.com/safebridge/desktop/internal/host.version=$VERSION -X github.com/safebridge/desktop/internal/host.bundledXray=$XRAY_VERSION -X github.com/safebridge/desktop/internal/host.variant=$variant" \
    -o "$out" .
  echo "✓ $out ($(du -h "$out" | cut -f1))"
done
