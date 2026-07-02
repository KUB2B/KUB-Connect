# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Язык

Всегда отвечай пользователю на русском языке, независимо от языка вопроса или языка кода/комментариев.

## Проект

**KUB Connect** — десктопный VPN-клиент на Go + Wails (v2). VLESS+Xray (xray-core
встроен как библиотека, без субпроцесса). Две модели маршрутизации: whitelist
(по умолчанию весь трафик direct, в VPN только Telegram + отмеченные пресеты +
свои домены/IP) и full-tunnel («всё через VPN»). Два режима перехвата: Proxy
(системный SOCKS5) и TUN (полный перехват через tun2socks, требует
администратора/root). Платформы: Windows (в т.ч. отдельная сборка для
Windows 7/Server 2008 R2), Linux. Модуль `github.com/zki/vless-client`.

Frontend — vanilla TypeScript + Vite (без фреймворка), UI на русском (без i18n).

## Команды

Все цели — в `Makefile` (`make help` их печатает).

```bash
# Тесты (весь Go-код)
go test ./...
# Один пакет / один тест
go test ./internal/app/... -run TestName

# Dev-режим GUI (live reload, Linux)
make dev            # = wails dev -tags "wails webkit2_41"

# Сборка GUI под хостовую ОС (Linux)
make gui            # = wails build -tags "wails webkit2_41"

# Headless CLI (без GUI, без CGO)
make headless
go run ./cmd/headless -link 'vless://...'

# Кросс-сборка Windows (нужны mingw + nsis)
make windows
# Windows 7 (патченный тулчейн XTLS go-win7, см. scripts/fetch-go-win7.sh)
make windows7

# Проверка типов фронтенда (аналог линта; запускается и внутри `frontend:build`)
cd frontend && npm run build   # tsc && vite build
cd frontend && npm run dev     # vite dev server
```

**Важно:** без `-tags wails` собирается stub-бинарь (`main_stub.go`, ~1.7 МБ) —
просто печатает справку и выходит. Реальный GUI требует тег `wails`.

## Архитектура

Слои сверху вниз:

- **`main.go`** (`//go:build wails`) — точка входа Wails: биндинг `App`,
  `SingleInstanceLock` (именованный мьютекс ОС — вторая копия форвардит себя в
  уже запущенное окно и выходит), `OnStartup`/`OnShutdown`/`OnBeforeClose`.
  `main_stub.go` — сборка без тега `wails`.
- **`gui_app.go`** — объект `App`, привязанный к фронтенду (все методы,
  которые дергает JS). Тонкий адаптер над `internal/app.Service`: разбирает
  DTO, эмитит события Wails, решает Windows-специфику (elevation, автообновление,
  трей через `tray.go`). Здесь же живёт логика перезапуска от администратора.
- **`internal/app`** — сервисный слой, ядро состояния приложения (не знает про
  Wails/GUI). `Service` хранит `store.State` под мьютексом, оркестрирует
  подключение/отключение (`connect.go`), CRUD серверов (`servers.go`),
  настройки (`settings.go`), автоподключение (`autoconnect.go`), автозапуск ОС
  (`autostart.go`), пинг (`ping.go`), логи через `internal/logbus`.
- **`internal/tunnel`** — оркестратор захвата трафика: поднимает embedded
  xray-core, при режиме TUN ещё и `internal/tun` (tun2socks) +
  `internal/netcfg` (маршруты ОС) + `internal/firewall` (kill switch, только
  Linux), при режиме Proxy — `internal/sysproxy` (системный SOCKS5).
- **`internal/xrayconf`** — строит конфиг xray-core (inbound/outbound/routing
  rules) из состояния приложения; здесь же loop-guard против петли на
  TUN-подсети и правило `sniffing.routeOnly`.
- **`internal/routing`** — правила whitelist/full-tunnel (geoip/geosite
  категории, захардкоженные CIDR Telegram — в каноническом geoip.dat нет
  категории `geoip:telegram`).
- **`internal/vless`** — парсер `vless://` ссылок.
- **`internal/store`** — персистентное состояние (`~/.../state.json`):
  серверы, настройки, флаг `PendingConnect` (см. ниже про elevation).
- **`internal/privilege`** — детект прав (`IsElevated`) и self-elevation
  restart (`RelaunchElevated`/`RunElevated` через `ShellExecuteW` verb
  `"runas"` на Windows; no-op-стабы на остальных ОС).
- **Платформенные пакеты с `_windows.go`/`_other.go`/`_linux.go`-суффиксами**:
  `netcfg`, `firewall`, `sysproxy`, `tun`, `wintundll`, `autostart` — паттерн
  «общий интерфейс + build-tag реализации», проверяй все варианты при правках.
- **`internal/geoassets`** — geoip.dat/geosite.dat вшиты в бинарь
  (`//go:embed`) и распаковываются в кэш при старте.
- **`internal/updater`** — проверка/скачивание релиза с GitHub, выбор
  установщика под версию ОС (обычный / Windows 7).
- **`frontend/`** — vanilla TS + Vite, один файл `src/main.ts`, общается с Go
  через сгенерированные биндинги `frontend/wailsjs/` (регенерируются
  `wails build`/`wails dev`, изменения в них не редактировать руками).
- **`cmd/headless`** — CLI без GUI поверх того же `internal/app`/`internal/core`.

Кросс-платформенность держится на **build tags**, а не на рантайм-ветвлении:
для правки платформенной логики почти всегда нужно поправить несколько файлов
(`_windows.go` + `_other.go`/стаб), иначе сборка под другую ОС не пройдёт или
получит no-op.

## Документация по фичам

- [Перезапуск от администратора (elevation)](docs/features/admin-elevation.md) — самоповышение прав, гонка при рестарте, PendingConnect
- [TUN-режим и kill switch](docs/features/tun-mode.md) — захват трафика, петля на TUN-подсети, kill switch
- [Маршрутизация (whitelist / full-tunnel)](docs/features/routing.md) — правила, Telegram CIDR, geoip/geosite
- [Автозапуск и автоподключение](docs/features/autostart.md) — автостарт ОС, автоподключение при старте
- [Автообновление](docs/features/updater.md) — проверка/скачивание релиза, установка

См. также `README.md`, `docs/INSTALL.md`, `docs/USER-GUIDE.md`, `CHANGELOG.md`.
