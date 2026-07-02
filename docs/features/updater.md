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
  - `DownloadAndInstall()` — качает установщик во временный каталог
    (`filepath.Base(rel.TagName)` защищает от path traversal через
    произвольный тег релиза), эмитит прогресс скачивания событием
    `update-progress`, запускает установщик через
    `privilege.RunElevated(dst)` (UAC — установщик требует прав), затем
    `a.quit()`, чтобы NSIS мог перезаписать запущенный exe.

## CI / релизы

`.github/workflows/release.yml` — self-hosted раннер, триггер: push тега
`vX.Y.Z`. Артефакты: Linux-бинарь, `...-windows-amd64-installer.exe`
(NSIS), `...-windows7-amd64-installer.exe` (сборка через патченный тулчейн
XTLS go-win7, см. `scripts/fetch-go-win7.sh`). `version` (`version.go`)
прошивается через `-ldflags "-X main.version=$(VER)"`, `VER` — из
`git describe --tags`.
