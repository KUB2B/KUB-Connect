# Автообновление

Только Windows. Приложение проверяет и устанавливает обновления из GitHub
Releases (`KUB2B/KUB-Connect`), исходники дополнительно зеркалируются на
GitVerse.

## Ключевые файлы

- `internal/updater` — `CheckLatest()` (GitHub API), `IsNewer(current, tag)`,
  `PickInstaller(release, legacyWindows)` — выбирает нужный ассет релиза
  (обычный установщик или Windows 7/Server 2008 R2), `Download(...)`.
- `gui_app.go`:
  - `CheckUpdate()` — бинд для фронтенда, сравнивает текущую версию с
    последним релизом.
  - `DownloadAndInstall()` — качает установщик в свежий каталог
    `os.MkdirTemp` (`filepath.Base(rel.TagName)` защищает от path traversal
    через произвольный тег релиза), эмитит прогресс скачивания событием
    `update-progress`, запускает установщик через
    `privilege.RunElevated(dst)` (UAC — установщик требует прав), затем
    `a.quit()`, чтобы NSIS мог перезаписать запущенный exe.

## Проверка целостности

`updater.Download` считает SHA-256 скачанного файла и сверяет с полем
`digest` ассета (`sha256:<hex>`, GitHub вычисляет его при загрузке ассета в
релиз). Ассет без digest или с несовпадением отклоняется, файл удаляется —
установщик запускается с правами администратора, поэтому fail-closed.
Скачивание ограничено 15 минутами; `CheckLatest` считает ошибкой любой
HTTP-статус кроме 200 (например, 403 при rate limit).

Подписи Authenticode нет: digest защищает от битой загрузки и подмены файла,
но не от компрометации GitHub-аккаунта.

## CI / релизы

`.github/workflows/release.yml` — self-hosted раннер, триггер: push тега
`vX.Y.Z`. Артефакты: Linux-бинарь, `...-windows-amd64-installer.exe`
(NSIS), `...-windows7-amd64-installer.exe` (сборка через патченный тулчейн
XTLS go-win7, см. `scripts/fetch-go-win7.sh`). `version` (`version.go`)
прошивается через `-ldflags "-X main.version=$(VER)"`, `VER` — из
`git describe --tags`.

## Проверка версии Windows в установщике

`build/windows/installer/kub_oscheck.nsh` (`kub.checkWindows`) заменяет
`wails.checkArchitecture`. Wails проверял `${AtLeastWin10}`, а это
версия, которую процессу сообщает Windows. Режим совместимости
(например, `~ WIN7RTM` в `HKCU\...\AppCompatFlags\Layers` на
`kub-connect.exe`) наследуется установщиком, запущенным из приложения, и на
Windows 11 он видел «Windows 7» → «only supported on Windows 10». Теперь
версия читается из `CurrentMajorVersionNumber` в реестре (не подменяется,
есть только на Win10+). Win7-установщик собирается с `KUB_WIN7=1` в
окружении (`scripts/build-windows.sh`, `release.yml`) и проверку версии
пропускает. `wails_tools.nsh` не править — перегенерируется при сборке.
