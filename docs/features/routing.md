# Маршрутизация (whitelist / full-tunnel)

Два режима маршрутизации, независимо от режима перехвата (Proxy/TUN):

- **Whitelist** (по умолчанию) — весь трафик direct, в VPN идут только
  Telegram, отмеченные пресеты сервисов (YouTube, Discord и т.п.) и свои
  домены/IP.
- **Full-tunnel** («Всё через VPN») — весь трафик через VPN, LAN — напрямую,
  опционально RU напрямую (`geoip:ru`).

Список «Исключения — всегда мимо VPN» действует в обоих режимах.

## Приоритет правил xray

`category-ru`/`geoip:ru` → direct (форсировано) → `geoip:private` → direct →
Telegram → proxy → кастомные (домены/IP пользователя) → proxy → catch-all →
direct (whitelist) / proxy (full-tunnel).

## Ключевые файлы

- `internal/routing` — вайтлист-логика, категории, `routing.TelegramCIDRs`.
- `internal/xrayconf` — собирает xray routing rules из состояния приложения.
- `internal/vless` — парсер `vless://` ссылок серверов.
- `data/geoip.dat`, `data/geosite.dat` — вшиты в бинарь
  (`internal/geoassets`), не нужно копировать рядом с exe.

## Важная деталь: Telegram и geoip

Канонический v2fly `geoip.dat` **не содержит** категории `geoip:telegram`.
Маршрутизация Telegram по IP использует захардкоженные CIDR
(`routing.TelegramCIDRs`, взяты с core.telegram.org), а не geoip-категорию.
Домены Telegram (`geosite:telegram`) при этом берутся из geosite как обычно.

См. также [tun-mode.md](tun-mode.md) — в TUN-режиме direct-трафик
привязывается к физическому интерфейсу (`sockopt.interface`), чтобы
доменные правила работали и при selective host-route.
