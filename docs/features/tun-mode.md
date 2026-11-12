# TUN-режим и kill switch

TUN — режим полного перехвата трафика через виртуальный сетевой адаптер (в
отличие от Proxy — системного SOCKS5, который не все приложения уважают,
например Telegram Desktop на Windows). Требует прав администратора/root
(см. [admin-elevation.md](admin-elevation.md)).

## Ключевые файлы

- `internal/tunnel/tunnel.go` — оркестратор: поднимает xray-core, при TUN ещё
  `internal/tun` (движок) + `internal/netcfg` (маршруты ОС) + опционально
  `internal/firewall` (kill switch).
- `internal/tun` — обёртка над `github.com/xjasonlyu/tun2socks/v2` (embedded,
  не субпроцесс). `SetLogWriter`/`SetLogLevel` — логи движка идут в
  `tun.log`, т.к. у GUI-процесса нет полезного stderr.
- `internal/netcfg` — назначает адаптеру IP и маршруты через ОС-специфичные
  команды (`netsh` на Windows, `ip`/`iproute2` на Linux). Только
  вайтлистнутые CIDR заворачиваются в туннель — не default-route (см. ниже).
- `internal/wintundll` — на Windows tun2socks ищет `wintun.dll` только рядом
  с exe / в System32 (не PATH/cwd). Установщик кладёт DLL в `$INSTDIR` рядом
  с exe (запись только администратору), и `Ensure()` грузит её оттуда. Вшитая
  копия (`//go:embed`) распаковывается в пользовательский кэш только когда DLL
  рядом с exe нет (dev/портативный запуск); содержимое файла в кэше сверяется
  побайтно, а не по размеру — DLL исполняется в elevated-процессе, а кэш
  доступен на запись любому процессу пользователя.
- `internal/firewall` — kill switch, **только Linux** (nftables): дропает
  вайтлистнутый трафик, если он не идёт через TUN-интерфейс (fail-closed на
  случай падения tun2socks).
- `internal/xrayconf` — loop-guard: блокирует исходящие в TUN-подсеть
  (`routing.TUNReservedCIDR`), чтобы xray не редиалил сам себя через
  on-link-маршрут адаптера.

## Модель маршрутизации в TUN

Выбран **selective host-route**, не default-route: в TUN-интерфейс попадают
только вайтлистнутые IP/CIDR (Telegram, кастомные), остальной трафик идёт
через физический интерфейс как обычно. Плюс: не нужен fwmark/policy routing,
нет петли по умолчанию. Минус (известное ограничение): доменные правила
(geosite) сами по себе не хост-роутятся — direct-трафик в TUN-режиме
привязывается к физическому интерфейсу через `sockopt.interface`, чтобы
доменные правила xray всё равно работали.

## Известные грабли (уже исправлены, но важны при регрессиях)

- **Петля на TUN-подсети**: если TUN IP/маска слишком широкие (было `/15`),
  on-link-маршрут адаптера сам попадает под catch-all direct → xray редиалит
  через тот же адаптер → флуд UDP-потоков → рост RAM/CPU, Telegram
  «умирает». Фикс: узкий префикс адаптера (`/30`) + явный loop-guard в
  `xrayconf`, блокирующий `routing.TUNReservedCIDR`.
- **Sniffing без `routeOnly`**: `destOverride` без `routeOnly=true` подменяет
  реальный dest сниффленным SNI — ломает MTProto (Telegram шлёт
  fake-TLS/обфусцированный handshake, sniffing путает его с реальным SNI).
  Обязательно `sniffing.routeOnly=true`.
- **Dial storm**: без `Mux` каждое TCP/UDP-соединение приложения — отдельный
  Reality-хендшейк к серверу; при параллельных подключениях (Telegram, Happy
  Eyeballs v4/v6) сервер захлёбывается ретраями. `Settings.Mux` (по
  умолчанию выключен — требует, чтобы клиент на сервере не использовал
  `flow: xtls-rprx-vision`, несовместим с mux) резко снижает число реальных
  соединений к серверу.
- **Консоль netsh мелькает**: подпроцессы `netsh` запускать с
  `SysProcAttr{HideWindow, CreationFlags: CREATE_NO_WINDOW}` на Windows.
- **PowerShell при включении TUN**: поиск физического default route выполняется
  напрямую через Windows IP Helper API (`GetIpForwardTable2`), поэтому
  PowerShell не запускается. `netsh` пока используется только для изменения
  адреса и маршрутов и работает без консольного окна.
