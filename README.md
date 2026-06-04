# vless-client

Десктопный VPN-клиент на Go + Wails. VLESS+Xray, whitelist-маршрутизация (Telegram + кастомные IP/домены), proxy и TUN-режимы.

## Зависимости

- Go 1.21+
- Node.js + npm
- [Wails CLI](https://wails.io/docs/gettingstarted/installation): `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- **Linux:** `libgtk-3-dev libwebkit2gtk-4.1-dev` (Ubuntu 24.04: `webkit2gtk-4.1`)
- **Windows cross-compile из Linux:** `sudo apt install gcc-mingw-w64-x86-64`

## Сборка

```bash
# Linux
wails build -tags "wails webkit2_41"

# Windows (cross-compile из Linux)
wails build -platform windows/amd64 -tags wails

# Headless CLI (без GUI, без CGO)
go build ./cmd/headless
```

> **Важно:** без `-tags wails` wails собирает stub-бинарь (1.7 МБ), который просто печатает справку и выходит.

## Dev-режим

```bash
wails dev -tags "wails webkit2_41"
```

## Headless CLI

```bash
go run ./cmd/headless -link 'vless://...'
```

## Заметки

- **geo-assets встроены** — `geoip.dat` / `geosite.dat` зашиты в бинарь, копировать не нужно. Распаковываются в `~/.cache/vless-client/geo/` при старте.
- **TUN-режим** требует root (`sudo`). Proxy-режим — без привилегий.
- **Windows:** при запуске нужен [WebView2 Runtime](https://go.microsoft.com/fwlink/p/?LinkId=2124703) (обычно уже есть если установлен Edge).
- **Маршрутизация:** whitelist — по умолчанию всё direct, в VPN идут только Telegram + кастомные правила. `geoip:ru` / `geosite:category-ru` → forced direct.

## Статус фаз

| Фаза | Статус | Описание |
|------|--------|----------|
| 1 | ✓ | Core/headless: парсер vless, xrayconf, routing, store |
| 2 | ✓ | Capture: sysproxy (Win/macOS/Linux), TUN via tun2socks |
| 3 | ✓ | Wails GUI: state machine, CRUD серверов, логи |
| 4 | ~  | Embed geo-dat ✓, TUN в GUI (Linux) ✓; Windows/macOS routing, kill switch — TODO |
| 5 | —  | Autostart, ping, статистика трафика |
