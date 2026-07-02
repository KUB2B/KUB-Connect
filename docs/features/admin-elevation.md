# Перезапуск от администратора (elevation)

TUN-режим требует прав администратора (Windows) / root (Linux). Proxy-режим —
без привилегий. Если пользователь включает TUN без прав, приложение
перезапускает само себя от администратора, а не просто падает с ошибкой.

## Ключевые файлы

- `internal/privilege/privilege.go` — кросс-платформенный `IsElevated()` /
  `RequireElevated(purpose)`.
- `internal/privilege/privilege_windows.go` — реальная реализация:
  `RelaunchElevated()` и `RunElevated(path)` через `ShellExecuteW` с verb
  `"runas"` (UAC). `ret <= 32` — код ошибки Windows SE_ERR_*; `ret == 5`
  (`SE_ERR_ACCESSDENIED`) = пользователь отклонил UAC →
  `privilege.ErrElevationDeclined`.
- `internal/privilege/privilege_other.go` — стабы для не-Windows (ошибка
  «не поддерживается на этой ОС»).
- `gui_app.go` `App.RelaunchElevated(connectAfter bool)` — точка входа,
  вызывается фронтендом и из автостарта.
- `internal/app/connect.go` `SetPendingConnect`/`ResumePendingConnect` —
  межпроцессная передача намерения «подключиться после рестарта».
- `internal/store/store.go` — поле `State.PendingConnect` (персистится на
  диск, не в `Settings`).

## Поток

1. Пользователь включает TUN (или автоподключение с TUN уже настроено) без
   прав администратора.
2. `App.RelaunchElevated(connectAfter)`:
   - `svc.SetPendingConnect(connectAfter)` + `Persist()` — намерение
     сохраняется на диск **до** запуска нового процесса.
   - `svc.Disconnect()` — гасит TUN-адаптер/xray **до** спавна нового
     процесса (см. «Гонка при рестарте» ниже).
   - `privilege.RelaunchElevated()` — показывает UAC, при согласии стартует
     новый процесс того же exe с теми же `os.Args[1:]`.
   - при успехе — `a.quit()` (асинхронный `wruntime.Quit`); при отказе —
     откатывает `PendingConnect` в `false` и возвращает ошибку фронтенду.
3. Новый (уже elevated) процесс на старте: `ResumePendingConnect()` — если
   флаг был выставлен, коннектится один раз вместо обычного `MaybeAutoConnect()`.

Автозапуск ОС всегда стартует неэлевированный процесс (HKCU Run), поэтому
если включены одновременно TUN + автозапуск + автоподключение — этот путь
срабатывает при каждом входе в систему (известное ограничение, UAC на
каждый логин).

## Гонка при рестарте (исправлено 2026-07-02)

`main.go` использует встроенный в Wails `SingleInstanceLock` — именованный
Windows-мьютекс. Старый процесс отпускает его только при фактическом
завершении, а `a.quit()` — асинхронный (`PostMessage` в очередь окна),
реальное закрытие происходит только после `a.shutdown()` (там же гасится
TUN/`Disconnect`) и выхода из `RunMainLoop`.

Раньше `ShellExecuteW` спавнил новый elevated-процесс **до** `Disconnect()`.
Если новый процесс успевал дойти до собственной проверки `SingleInstanceLock`
раньше, чем старый процесс освобождал мьютекс, он видел мьютекс занятым,
форвардил себя в умирающее окно и тихо делал `os.Exit(0)` — ещё до
`OnStartup`/`ResumePendingConnect`. Внешне выглядело как «приложение не
запускается» после рестарта в режиме администратора.

Фикс: `Disconnect()` (самая долгая часть teardown — остановка TUN/xray)
теперь выполняется синхронно **до** `privilege.RelaunchElevated()`, а не
после в `a.shutdown()`. Это резко сокращает окно гонки: старому процессу
после спавна нового остаётся только слить очередь Win32-сообщений и выйти —
на порядки быстрее, чем полный teardown туннеля.
