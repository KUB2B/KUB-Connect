# Автозапуск и автоподключение

Две независимые настройки в `store.Settings`:

- **Автозапуск при входе в систему** (`AutoStart`) — приложение стартует
  вместе с ОС.
- **Автоподключение при старте** (`AutoConnect`) — при запуске приложение
  сразу подключается к активному серверу.

## Ключевые файлы

- `internal/autostart` — платформенный менеджер (`Supported`/`Enable`/
  `Disable`/`New()`):
  - Windows: `HKCU\...\Run` значение `"KUB Connect"` = путь к exe в кавычках
    (реестр, без прав администратора).
  - macOS: LaunchAgent `~/Library/LaunchAgents/pro.qb2b.kub-connect.plist` +
    `launchctl load/unload`.
  - остальные ОС: no-op, `Supported() == false`.
- `internal/app/autostart.go` — `applyAutostart`, `ReconcileAutostart` (на
  старте асинхронно обновляет путь к exe в записи автозапуска — на случай,
  если бинарь переехал).
- `internal/app/autoconnect.go` — `WantsElevatedAutoConnect()`: если
  `AutoConnect` включён, режим TUN и процесс не elevated — нужен рестарт от
  администратора вместо прямого `Connect()` (см.
  [admin-elevation.md](admin-elevation.md)).
- `gui_app.go` `startup()` — порядок на старте: сначала
  `ResumePendingConnect()` (намерение от предыдущего elevation-рестарта), и
  только если его не было — `WantsElevatedAutoConnect()` /
  `MaybeAutoConnect()`.

## Известное ограничение

Автозапуск ОС всегда стартует **неэлевированный** процесс. Если одновременно
включены TUN + автозапуск + автоподключение, при каждом входе в систему
приложение стартует, сразу понимает, что нужна элевация, и показывает UAC —
т.е. UAC-запрос на каждый логин. Более дружественное решение (не
реализовано): запуск через Task Scheduler с наивысшими правами или
helper-service (Amnezia-style), без запроса UAC на каждый запуск.
