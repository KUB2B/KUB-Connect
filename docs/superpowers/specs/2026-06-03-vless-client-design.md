# VLESS + Xray VPN-клиент — дизайн

Дата: 2026-06-03
Статус: утверждён к написанию плана

## Цель

Кросс-платформенный десктоп-клиент (Windows + macOS) для VLESS + Xray (сервер
развёрнут через 3x-ui v3.2.6). Главная задача — заложенные настройки
маршрутизации для туннелирования конкретных IP/доменов, в первую очередь
Telegram, по модели whitelist (по умолчанию всё напрямую, в VPN идёт только
выбранное).

## Ключевые решения

| Параметр | Выбор |
|---|---|
| Платформа | Windows + macOS desktop |
| Форма | Полноценный GUI |
| Стек | Go + Wails; xray-core и sing-tun подключены как библиотеки (in-process) |
| Перехват трафика | TUN (по умолчанию) ⇄ SOCKS proxy, переключаемые |
| Маршрутизация | Whitelist: всё direct, выбранное → VPN |
| Категории | Telegram (always-on), кастомные домены/IP, geosite:category-ru → forced direct |
| Вход | Парсер `vless://` share-ссылки из 3x-ui; список серверов |
| Доп. функции | Автозапуск + автоконнект, ping/тест сервера, логи + статистика трафика, kill switch |

Почему Go, а не Rust/Tauri: xray-core и sing-tun написаны на Go — подключаются
как библиотеки в тот же процесс, без отдельного подпроцесса, IPC и слежения за
дочерним процессом. Это решающий фактор для клиента, оборачивающего xray.

## Архитектура

Один Go-процесс. Wails v2: Go-бэкенд + web-фронтенд, методы бэкенда
экспортируются во фронт. xray-core и sing-tun — библиотеки, не подпроцессы.

### Модули (`internal/`)

- **`vless`** — парсер `vless://` → структура `ServerConfig`
  (host, port, uuid, security, pbk/sid/sni/fp/flow/spx, type=tcp/ws/grpc + их
  параметры).
- **`xrayconf`** — билдер: `ServerConfig` + `RoutingProfile` → xray JSON-конфиг
  (inbounds / outbounds / routing / dns). Сердце логики маршрутизации.
- **`core`** — обёртка xray-core: `core.New(cfg)` + Start/Stop in-process.
- **`tun`** — sing-tun: поднять виртуальный адаптер (wintun / utun), направить
  трафик ОС в локальный SOCKS-inbound xray. Платформозависимо.
- **`sysproxy`** — режим системного прокси (Win: реестр, macOS: `networksetup`)
  как альтернатива TUN.
- **`routing`** — хранит тумблеры/кастом-правила → отдаёт набор правил билдеру.
- **`store`** — JSON в user-config-dir: серверы, профиль маршрутизации, настройки.
- **`stats`** — xray StatsService API → скорость up/down.
- **`autostart`** — Win: реестр Run; macOS: LaunchAgent plist.
- **`killswitch`** — при разрыве блокировать туннелируемые маршруты
  (firewall / route blackhole). Платформозависимо.
- **`frontend`** — окно: статус + connect, серверы, маршрутизация, настройки,
  логи/статистика.

Каждый модуль изолирован и тестируется отдельно. Платформо-зависимая логика
(`tun`, `sysproxy`, `autostart`, `killswitch`) скрыта за интерфейсом с файлами
`_windows.go` / `_darwin.go`.

## Поток данных

```
vless:// → parser → ServerConfig → store
тумблеры/кастом (GUI) → RoutingProfile → store
Connect → xrayconf билдит JSON → core стартует xray-core (SOCKS inbound на 127.0.0.1:port)
         → tun поднимает адаптер, гонит трафик ОС в этот SOCKS
            (или sysproxy выставляет системный прокси, если выбран proxy-режим)
```

## Логика маршрутизации

Outbounds: `proxy` (vless на сервер), `direct` (freedom), `block`.

Правила (порядок сверху вниз критичен для whitelist):

1. `geosite:category-ru`, `geoip:ru` → **direct** (forced, наивысший приоритет — RU не утечёт в туннель).
2. приватные/локальные сети (`geoip:private`) → **direct**.
3. `geosite:telegram` + `geoip:telegram` → **proxy** (always-on).
4. кастомные домены/IP пользователя → **proxy**.
5. всё остальное → **direct** (дефолт whitelist).

Тумблеры включают/выключают соответствующие правила. Изменение профиля →
пересборка конфига + рестарт core.

### DNS

xray dns-блок. Запросы туннелируемых доменов резолвятся через proxy (remote DNS,
напр. 1.1.1.1 через сервер), остальное — системный/локальный DNS direct. Так
Telegram-домены резолвятся «за рубежом» и корректно матчатся, без DNS-утечки.

### geoip.dat / geosite.dat

Кладём в ресурсы приложения, путь передаём xray через `XRAY_LOCATION_ASSET`. В v1
файлы bundled; авто-обновление — позже.

## GUI

Вкладки/панели:

- **Главная:** кнопка Connect/Disconnect, статус, активный сервер, индикатор скорости up/down.
- **Серверы:** поле вставки `vless://` + добавить, список серверов, выбор активного, ping/тест (мс), удалить.
- **Маршрутизация:** тумблеры Telegram / category-ru-direct; список кастомных доменов/IP; выбор режима TUN ⇄ Proxy.
- **Настройки:** автозапуск, автоконнект, kill switch, язык.
- **Логи:** живой лог xray + события подключения, очистить/копировать.

### Машина состояний соединения

```
Disconnected → Connecting → Connected → Disconnecting → Disconnected
                   ↓ (fail)
                 Error → Disconnected
```

Правка профиля/сервера в состоянии Connected → пересборка конфига + мягкий
рестарт core (Disconnecting → Connecting → Connected).

### Обработка ошибок (показывается в UI понятно)

- битый `vless://` → подсветка поля + текст, что не распарсилось.
- нет прав админа для TUN → запрос elevation (UAC на Win / диалог на macOS), либо подсказка + fallback на proxy.
- адаптер/драйвер TUN не поднялся → лог + кнопка «перейти в proxy-режим».
- xray-core не стартует → stderr в лог, статус Error.
- сервер недоступен → ping/тест красным; при connect — Error с причиной.

### Kill switch

При неожиданном падении core в состоянии Connected — не снимать блокировку,
держать туннелируемые маршруты в blackhole, пока пользователь не нажмёт
Disconnect. RU/direct трафик не трогаем (модель whitelist).

## Тестирование

- `vless` parser — табличные unit-тесты: reality/tls, tcp/ws/grpc, flow=xtls-rprx-vision, edge-кейсы (без params, спецсимволы в UUID, base64-имя).
- `xrayconf` билдер — golden-тесты: профиль → ожидаемый JSON (фиксируем порядок правил). Главная защита логики whitelist.
- `routing` — табличные: набор тумблеров → ожидаемый список правил.
- `store` — round-trip save/load.
- `core` — smoke: старт с минимальным валидным конфигом, остановка без утечки горутин.
- `tun` / `sysproxy` / `autostart` / `killswitch` — за интерфейсом, в CI мок; реальная проверка вручную на Win/macOS.

TDD для чистой логики (parser, xrayconf, routing, store). Платформенные модули —
интеграционно/вручную.

## Сборка и распространение

- Wails build на каждую ОС. cgo-зависимости (xray-core/sing-tun) ограничивают
  кросс-компиляцию → CI-матрица GitHub Actions: windows + macos раннеры.
- geoip.dat/geosite.dat в `build/assets`, копируются рядом с бинарём,
  `XRAY_LOCATION_ASSET`.
- Win: `.exe`, манифест с запросом elevation для TUN (или elevation в рантайме);
  инсталлятор (NSIS) — позже.
- macOS: `.app`. В v1 unsigned + инструкция по Gatekeeper; codesign+notarize —
  позже. utun требует root.

## Права доступа

TUN-режим требует elevation. v1: при выборе TUN запрашивать повышение прав; при
отказе/недоступности — авто-fallback в proxy-режим без админки. Состояние чётко
сообщается в UI.

## Открытые риски

1. macOS: notarization и привилегированный helper-демон для utun могут
   потребоваться для гладкой работы. В v1 допускаем ручной запуск с правами.
2. cgo-зависимости sing-tun усложняют кросс-сборку → CI-матрица обязательна.

## Вне области v1 (YAGNI)

- Авто-обновление geoip/geosite файлов.
- QR-импорт и ручной ввод полей сервера (только вставка `vless://`).
- Набор отдельных тумблеров под YouTube/Instagram/Discord (есть кастомные домены/IP).
- Инсталляторы, codesign/notarize (ручная установка в v1).
