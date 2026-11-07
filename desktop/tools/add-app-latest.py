#!/usr/bin/env python3
"""Добавляет приложение для компьютера в latest.json (поле app) — до подписи файла.

  python3 desktop/tools/add-app-latest.py dist 1.0.0

Берёт из dist файлы SafeBridge-<версия>-<платформа>.(exe|zip), считает SHA-256 и кладёт
рядом копии с постоянными именами (SafeBridge-windows-x64.exe…) для ссылок в README.
Приложение по этому полю узнаёт о новой версии и проверяет файл перед установкой.
"""
import hashlib, json, os, shutil, sys

dist, ver = sys.argv[1], sys.argv[2]
platforms = {  # ключ в latest.json → суффикс файла
    "windows-amd64": "windows-x64.exe",
    "windows-arm64": "windows-arm64.exe",
    "windows7-amd64": "windows7-x64.exe",
    "windows7-386": "windows7-x86.exe",
    "macos": "macos.zip",
    "macos-legacy": "macos-legacy.zip",
}
files = {}
for key, suffix in platforms.items():
    name = f"SafeBridge-{ver}-{suffix}"
    path = os.path.join(dist, name)
    if not os.path.exists(path):
        continue
    with open(path, "rb") as f:
        files[key] = {"name": name, "sha256": hashlib.sha256(f.read()).hexdigest()}
    shutil.copyfile(path, os.path.join(dist, f"SafeBridge-{suffix}"))
p = os.path.join(dist, "latest.json")
li = json.load(open(p))
li["app"] = {"version": ver, "files": files}
json.dump(li, open(p, "w"), indent=2)
print("✓ приложение", ver, "в latest.json:", ", ".join(files) or "нет файлов")
