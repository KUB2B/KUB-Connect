# In-App Update (Windows) — Design

**Date:** 2026-06-11
**Status:** Approved, pending implementation plan
**Scope:** Windows only

## Problem

The app already detects newer GitHub releases (`internal/updater.CheckLatest` +
`App.CheckUpdate`) and shows a dismissible banner. The banner only links out to
the release page in a browser — the user must manually download the installer
and run it. Goal: download and launch the installer from inside the app.

## Flow

Current:

> banner "Доступна vX" → link → browser → manual download → manual run

New:

> banner → **«Обновить»** button → progress bar → installer launches (UAC) → app quits

The existing browser link stays as a fallback.

## Components

### 1. `internal/updater/updater.go` — extend

Add asset parsing and a download helper:

```go
type Asset struct {
    Name string `json:"name"`
    URL  string `json:"browser_download_url"`
    Size int64  `json:"size"`
}
// Release gains: Assets []Asset `json:"assets"`

// PickInstaller returns the Windows installer asset from a release, matching the
// name produced by build/windows/installer/project.nsi OutFile
// (kub-connect-amd64-installer.exe). Reports false if none present.
func PickInstaller(rel Release) (Asset, bool)

// Download streams the asset to dst, invoking progress(done, total) as bytes
// arrive. total comes from the response Content-Length, falling back to a.Size.
func Download(ctx context.Context, a Asset, dst string, progress func(done, total int64)) error
```

- `Download` = `http.Get` over the asset's HTTPS URL + `io.TeeReader` wrapping
  the body with a byte counter that calls `progress`. Writes to
  `os.TempDir()/kub-connect-<version>-installer.exe`.
- Sanity-check: after the copy, verify the written size matches `a.Size` (cheap
  corruption guard); mismatch → error, delete the partial file.

### 2. `internal/privilege` — add elevated launcher

```go
// RunElevated launches an arbitrary executable via ShellExecuteW "runas",
// surfacing the UAC prompt. The installer carries an admin manifest, so a plain
// exec.Command (CreateProcess) would fail with ERROR_ELEVATION_REQUIRED.
func RunElevated(path string) error   // privilege_windows.go
func RunElevated(path string) error   // privilege_other.go — returns "not supported on <GOOS>"
```

Reuses the existing `procShellExecute` machinery already present in
`privilege_windows.go` (same pattern as `RelaunchElevated`). Returns
`ErrElevationDeclined` when the user dismisses UAC (SE_ERR_ACCESSDENIED).

### 3. `gui_app.go` — new bound method

```go
func (a *App) DownloadAndInstall() error
```

Steps:
1. Re-fetch latest release (`updater.CheckLatest`).
2. `PickInstaller` → installer asset; no asset → return error.
3. `updater.Download` to the temp path, with a progress callback that emits
   `wruntime.EventsEmit(ctx, "update-progress", {done, total})`.
4. `privilege.RunElevated(installerPath)` — UAC prompt + wizard.
5. `a.quit()` — the normal quit path (shutdown → Disconnect → proxy cleanup →
   exit), which also releases the lock on the running exe so NSIS can overwrite.

If the user declines UAC (`ErrElevationDeclined`), do **not** quit — return the
error so the frontend can restore the banner.

### 4. Frontend — `frontend/src/main.ts` + `index.html`

- Add an «Обновить» button to the update banner, beside the existing dismiss.
- Click → call `DownloadAndInstall()`; swap the banner content into a
  progress-bar view.
- Subscribe to `update-progress` events → set bar `width = done/total*100%`.
- On `DownloadAndInstall()` rejection (e.g. UAC declined / network) → restore the
  banner with an error note. Keep the existing browser link as a fallback.

## Race consideration

After `RunElevated` returns, the app must exit before NSIS reaches the
file-overwrite step. The NSIS wizard requires user clicks (welcome → directory →
install — several seconds), giving the app ample time to quit. Feature #1's
`taskkill /F /IM kub-connect.exe` in `.onInit` is a belt-and-suspenders backstop
if the exe is still locked. Considered safe.

## Security

- Download over **HTTPS** from GitHub (API + asset CDN), certificate validated
  against system roots — the same trust anchor already used by the version check.
- The Windows binary ships **unsigned**, so there is no Authenticode signature to
  verify. A checksum published in the same release would not defend against a
  compromised GitHub account (the attacker controls both the asset and the
  checksum); it only catches corruption, which the HTTPS transport and the
  `a.Size` check already cover. Therefore **no** checksums file is added and the
  release workflow is unchanged.
- The installer runs only from a fixed `%TEMP%` filename; UAC gives the user the
  final confirmation before any system change.

## Testing

- `PickInstaller` — table test: correct asset matched, non-Windows assets
  ignored, empty release → false. Pure, runs on Linux.
- `Download` — `httptest.Server` serves known bytes; assert dst contents match,
  `progress` was called with a non-decreasing `done` and a final `done == total`,
  and a size mismatch errors + removes the partial file.
- `RunElevated` — only the `_other` stub is unit-testable on Linux; the Windows
  path is manual QA.
- Frontend progress bar — manual QA on Windows.

## Out of scope (YAGNI)

Download cancellation; silent `/S` install; macOS/Linux in-app update; delta
updates.
