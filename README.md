# KUB Connect

Десктопный VPN-клиент компании КУБ2Б на Go + Wails. VLESS+Xray,
whitelist-маршрутизация (Telegram + кастомные IP/домены) и full-tunnel,
режимы Proxy и TUN. Windows (вкл. Windows 7) и Linux.

> 📦 **Установка для пользователей:** см. [docs/INSTALL.md](docs/INSTALL.md) · [docs/USER-GUIDE.md](docs/USER-GUIDE.md).

## Зависимости

- Go 1.21+
- Node.js + npm
- [Wails CLI](https://wails.io/docs/gettingstarted/installation): `go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0`
- **Linux:** `libgtk-3-dev libwebkit2gtk-4.1-dev` (Ubuntu 24.04: `webkit2gtk-4.1`)
- **Windows cross-compile из Linux:** `gcc-mingw-w64-x86-64`, `nsis` (для установщика)

## Сборка

```bash
# Linux
wails build -tags "wails webkit2_41"

# Windows (cross-compile из Linux) + NSIS-установщик
CC=x86_64-w64-mingw32-gcc CXX=x86_64-w64-mingw32-g++ CGO_ENABLED=1 \
  wails build -platform windows/amd64 -tags wails -nsis

# Headless CLI (без GUI, без CGO)
go build ./cmd/headless
```

> **Важно:** без `-tags wails` wails собирает stub-бинарь (1.7 МБ), который просто печатает справку и выходит.

### Windows 7 / Server 2008 R2

Официальный Go 1.21+ падает на Win7 при старте. Отдельная сборка использует
патченный toolchain [XTLS go-win7](https://github.com/XTLS/go-win7):

```bash
scripts/fetch-go-win7.sh
GOROOT="$PWD/.go-win7" PATH="$PWD/.go-win7/bin:$PATH" \
CC=x86_64-w64-mingw32-gcc CXX=x86_64-w64-mingw32-g++ CGO_ENABLED=1 \
  wails build -platform windows/amd64 -tags wails -nsis
```

Код приложения, NSIS и WebView2 идентичны обычной сборке — меняется только GOROOT.

## Dev-режим

```bash
wails dev -tags "wails webkit2_41"
```

## Headless CLI

```bash
go run ./cmd/headless -link 'vless://...'
```

## Релизы

CI: GitHub Actions на self-hosted раннере (`.github/workflows/release.yml`),
триггер — push тега `vX.Y.Z`. Артефакты:

- `kub-connect-vX.Y.Z-linux-amd64` — бинарь Linux
- `kub-connect-vX.Y.Z-windows-amd64-installer.exe` — установщик Windows 10+
- `kub-connect-vX.Y.Z-windows7-amd64-installer.exe` — установщик Windows 7 SP1+

Релизы публикуются на [GitHub KUB2B/KUB-Connect](https://github.com/KUB2B/KUB-Connect/releases);
исходники зеркалятся на GitVerse. Авто-обновление в приложении тянет последний
релиз через GitHub API и выбирает установщик по версии ОС.

## Заметки

- **geo-assets встроены** — `geoip.dat` / `geosite.dat` зашиты в бинарь, копировать не нужно. Распаковываются в кэш при старте.
- **TUN-режим** требует прав администратора/root. Proxy-режим — без привилегий.
- **Windows:** при запуске нужен [WebView2 Runtime](https://go.microsoft.com/fwlink/p/?LinkId=2124703) (обычно уже есть если установлен Edge).
- **Маршрутизация:** whitelist — по умолчанию всё direct, в VPN идут только Telegram, отмеченные пресеты сервисов (YouTube, Discord и др.) и свои домены/IP. Режим «Всё через VPN» (full tunnel) заворачивает весь трафик, LAN остаётся напрямую, опционально RU напрямую (`geoip:ru`). Список «Исключения — всегда мимо VPN» действует в обоих режимах. В TUN-режиме direct-трафик привязывается к физическому интерфейсу (sockopt.interface), поэтому доменные правила работают и в whitelist.

## Возможности

- VLESS+Xray клиент, режимы Proxy (системный SOCKS5) и TUN (полный перехват).
- Whitelist- и full-tunnel-маршрутизация, RU-direct, kill switch (TUN/Linux).
- GUI на Wails: управление серверами, пинг, выбор режима, уровень логов.
- Автозапуск при входе в систему, автоподключение при старте.
- Авто-обновление из приложения (Windows), сворачивание в трей.
- Встроенные geo-базы, отдельная сборка для Windows 7.
