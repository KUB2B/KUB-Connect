# In-App Update (Windows) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the user download and launch the Windows installer from inside the app — replacing the browser-link-only update banner.

**Architecture:** Extend `internal/updater` to parse release assets and stream-download the installer with progress callbacks. Add `privilege.RunElevated` to launch the admin-manifested installer via `ShellExecuteW "runas"`. A new `App.DownloadAndInstall` bound method downloads to `%TEMP%`, emits `update-progress` events, launches the installer, then quits. The frontend banner gains an «Обновить» button and a progress bar.

**Tech Stack:** Go (stdlib `net/http`, `io`, `os`; `golang.org/x/sys/windows`), Wails v2, vanilla TypeScript.

**Spec:** `docs/superpowers/specs/2026-06-11-in-app-update-design.md`

---

## File Structure

- `internal/updater/updater.go` — MODIFY: add `Asset` type, `Assets` field on `Release`, `PickInstaller`, `Download`, `progressWriter`.
- `internal/updater/updater_test.go` — MODIFY: add tests for `PickInstaller` and `Download`.
- `internal/privilege/privilege_windows.go` — MODIFY: add `RunElevated`.
- `internal/privilege/privilege_other.go` — MODIFY: add `RunElevated` stub.
- `internal/privilege/privilege_other_test.go` — CREATE: stub returns error off Windows.
- `gui_app.go` — MODIFY: add `DownloadAndInstall` bound method.
- `frontend/wailsjs/go/main/App.js` / `App.d.ts` — MODIFY: add `DownloadAndInstall` binding (regenerated identically on next `wails build`).
- `frontend/index.html` — MODIFY: add «Обновить» button + progress-bar markup to the update banner.
- `frontend/src/main.ts` — MODIFY: wire the button, listen for `update-progress`.
- `frontend/src/style.css` — MODIFY: progress-bar styles.

---

## Task 1: Parse release assets + PickInstaller

**Files:**
- Modify: `internal/updater/updater.go`
- Test: `internal/updater/updater_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/updater/updater_test.go`:

```go
func TestPickInstaller(t *testing.T) {
	rel := Release{Assets: []Asset{
		{Name: "KUB-Connect.dmg", URL: "https://x/dmg", Size: 10},
		{Name: "kub-connect-amd64-installer.exe", URL: "https://x/exe", Size: 20},
	}}
	a, ok := PickInstaller(rel)
	if !ok {
		t.Fatal("PickInstaller: expected ok=true")
	}
	if a.Name != "kub-connect-amd64-installer.exe" || a.URL != "https://x/exe" {
		t.Errorf("PickInstaller picked wrong asset: %+v", a)
	}

	if _, ok := PickInstaller(Release{Assets: []Asset{{Name: "notes.txt"}}}); ok {
		t.Error("PickInstaller: expected ok=false when no installer asset")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/updater/ -run TestPickInstaller -v`
Expected: FAIL — `undefined: Asset` / `undefined: PickInstaller`.

- [ ] **Step 3: Add the Asset type, Assets field, and PickInstaller**

In `internal/updater/updater.go`, add `"strings"` to imports if not present (it is), then add the `Asset` type and extend `Release`:

```go
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

type Release struct {
	TagName string  `json:"tag_name"`
	HTMLURL string  `json:"html_url"`
	Assets  []Asset `json:"assets"`
}
```

(Replace the existing `Release` struct — keep `TagName` and `HTMLURL`, add `Assets`.)

Add the picker:

```go
// PickInstaller returns the Windows installer asset from a release. It matches
// the name produced by build/windows/installer/project.nsi OutFile, which ends
// in "-installer.exe" (e.g. kub-connect-amd64-installer.exe). Reports false if
// the release carries no such asset.
func PickInstaller(rel Release) (Asset, bool) {
	for _, a := range rel.Assets {
		if strings.HasSuffix(strings.ToLower(a.Name), "-installer.exe") {
			return a, true
		}
	}
	return Asset{}, false
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/updater/ -run TestPickInstaller -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/updater/updater.go internal/updater/updater_test.go
git commit -m "feat(updater): parse release assets + PickInstaller"
```

---

## Task 2: Stream-download with progress

**Files:**
- Modify: `internal/updater/updater.go`
- Test: `internal/updater/updater_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/updater/updater_test.go` (and add imports `"context"`, `"net/http"`, `"net/http/httptest"`, `"os"`, `"path/filepath"`, `"testing"` — `testing` already present):

```go
func TestDownload(t *testing.T) {
	body := []byte("hello-installer-bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "out.exe")
	a := Asset{Name: "x-installer.exe", URL: srv.URL, Size: int64(len(body))}

	var lastDone, lastTotal int64
	var calls int
	err := Download(context.Background(), a, dst, func(done, total int64) {
		if done < lastDone {
			t.Errorf("progress done decreased: %d after %d", done, lastDone)
		}
		lastDone, lastTotal = done, total
		calls++
	})
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != string(body) {
		t.Errorf("downloaded content = %q, want %q", got, body)
	}
	if calls == 0 {
		t.Error("progress callback never fired")
	}
	if lastDone != int64(len(body)) || lastTotal != int64(len(body)) {
		t.Errorf("final progress = %d/%d, want %d/%d", lastDone, lastTotal, len(body), len(body))
	}
}

func TestDownloadSizeMismatchRemovesFile(t *testing.T) {
	body := []byte("short")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "out.exe")
	a := Asset{Name: "x-installer.exe", URL: srv.URL, Size: int64(len(body)) + 1} // wrong size

	if err := Download(context.Background(), a, dst, nil); err == nil {
		t.Fatal("Download: expected size-mismatch error, got nil")
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Error("Download: partial file should be removed on size mismatch")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/updater/ -run TestDownload -v`
Expected: FAIL — `undefined: Download`.

- [ ] **Step 3: Implement Download + progressWriter**

In `internal/updater/updater.go` add imports `"context"`, `"fmt"`, `"io"`, `"os"` (keep existing `"encoding/json"`, `"net/http"`, `"strings"`, `"time"`, and the semver import). Add:

```go
// progressWriter counts bytes written and reports cumulative progress.
type progressWriter struct {
	done, total int64
	cb          func(done, total int64)
}

func (w *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	w.done += int64(n)
	if w.cb != nil {
		w.cb(w.done, w.total)
	}
	return n, nil
}

// Download streams the asset to dst over HTTPS, calling progress(done, total)
// as bytes arrive (progress may be nil). total is the response Content-Length,
// falling back to a.Size when the server omits it. On any error, or when the
// written size disagrees with a known a.Size, the partial file is removed.
func Download(ctx context.Context, a Asset, dst string, progress func(done, total int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", a.Name, resp.StatusCode)
	}

	total := resp.ContentLength
	if total <= 0 {
		total = a.Size
	}

	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	pw := &progressWriter{total: total, cb: progress}
	written, copyErr := io.Copy(f, io.TeeReader(resp.Body, pw))
	closeErr := f.Close()
	if copyErr != nil {
		os.Remove(dst)
		return copyErr
	}
	if closeErr != nil {
		os.Remove(dst)
		return closeErr
	}
	if a.Size > 0 && written != a.Size {
		os.Remove(dst)
		return fmt.Errorf("download %s: size mismatch got %d want %d", a.Name, written, a.Size)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/updater/ -run TestDownload -v`
Expected: PASS (both `TestDownload` and `TestDownloadSizeMismatchRemovesFile`).

- [ ] **Step 5: Run the full updater package test**

Run: `go test ./internal/updater/ -v`
Expected: PASS (`TestIsNewer`, `TestPickInstaller`, `TestDownload`, `TestDownloadSizeMismatchRemovesFile`).

- [ ] **Step 6: Commit**

```bash
git add internal/updater/updater.go internal/updater/updater_test.go
git commit -m "feat(updater): stream-download installer with progress"
```

---

## Task 3: privilege.RunElevated

**Files:**
- Modify: `internal/privilege/privilege_windows.go`
- Modify: `internal/privilege/privilege_other.go`
- Test: `internal/privilege/privilege_other_test.go` (create)

- [ ] **Step 1: Write the failing test (off-Windows stub)**

Create `internal/privilege/privilege_other_test.go`:

```go
//go:build !windows

package privilege

import "testing"

func TestRunElevatedUnsupported(t *testing.T) {
	if err := RunElevated("/tmp/whatever.exe"); err == nil {
		t.Error("RunElevated: expected error off Windows, got nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/privilege/ -run TestRunElevatedUnsupported -v`
Expected: FAIL — `undefined: RunElevated`.

- [ ] **Step 3: Add the stub (off-Windows)**

In `internal/privilege/privilege_other.go`, append:

```go
// RunElevated is unsupported off Windows; the in-app updater only ships a
// Windows installer.
func RunElevated(path string) error {
	return errors.New("elevated launch is not supported on this OS")
}
```

(`"errors"` is already imported in that file.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/privilege/ -run TestRunElevatedUnsupported -v`
Expected: PASS.

- [ ] **Step 5: Add the Windows implementation**

In `internal/privilege/privilege_windows.go`, append:

```go
// RunElevated launches an arbitrary executable via the UAC "runas" verb. The
// installer carries an admin manifest, so a plain exec.Command (CreateProcess)
// would fail with ERROR_ELEVATION_REQUIRED; ShellExecuteW surfaces the prompt.
// Returns ErrElevationDeclined if the user dismisses UAC.
func RunElevated(path string) error {
	verb, _ := syscall.UTF16PtrFromString("runas")
	file, _ := syscall.UTF16PtrFromString(path)
	dir, _ := syscall.UTF16PtrFromString(filepath.Dir(path))

	const swShowNormal = 1
	ret, _, _ := procShellExecute.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		0,
		uintptr(unsafe.Pointer(dir)),
		uintptr(swShowNormal),
	)
	if ret <= 32 {
		if ret == 5 { // SE_ERR_ACCESSDENIED — UAC dismissed
			return ErrElevationDeclined
		}
		return fmt.Errorf("ShellExecuteW failed (code %d)", ret)
	}
	return nil
}
```

(All referenced symbols — `procShellExecute`, `syscall`, `filepath`, `unsafe`, `fmt`, `ErrElevationDeclined` — are already imported/defined in the privilege package.)

- [ ] **Step 6: Verify both OS build graphs compile**

Run: `go build ./internal/privilege/ && GOOS=windows CGO_ENABLED=0 go build ./internal/privilege/`
Expected: both succeed, no output.

- [ ] **Step 7: Commit**

```bash
git add internal/privilege/privilege_windows.go internal/privilege/privilege_other.go internal/privilege/privilege_other_test.go
git commit -m "feat(privilege): RunElevated to launch installer via UAC"
```

---

## Task 4: App.DownloadAndInstall bound method

**Files:**
- Modify: `gui_app.go`
- Modify: `frontend/wailsjs/go/main/App.js`
- Modify: `frontend/wailsjs/go/main/App.d.ts`

- [ ] **Step 1: Add the bound method**

In `gui_app.go`, add `"context"` is already imported; add `"fmt"`, `"os"` (already imported), `"path/filepath"` (already imported). Add `"github.com/zki/vless-client/internal/privilege"` (already imported). Add the method below `CheckUpdate`:

```go
// DownloadAndInstall fetches the latest Windows installer, streaming download
// progress to the frontend via "update-progress" events, launches it (UAC),
// then quits this instance so NSIS can overwrite the running exe. Returns an
// error (without quitting) if no installer asset exists, the download fails, or
// the user declines UAC, so the frontend can restore the banner.
func (a *App) DownloadAndInstall() error {
	rel, err := updater.CheckLatest()
	if err != nil {
		return fmt.Errorf("проверка обновления: %w", err)
	}
	asset, ok := updater.PickInstaller(rel)
	if !ok {
		return fmt.Errorf("установщик не найден в релизе %s", rel.TagName)
	}

	dst := filepath.Join(os.TempDir(), fmt.Sprintf("kub-connect-%s-installer.exe", rel.TagName))
	err = updater.Download(a.ctx, asset, dst, func(done, total int64) {
		wruntime.EventsEmit(a.ctx, "update-progress", map[string]int64{"done": done, "total": total})
	})
	if err != nil {
		return fmt.Errorf("скачивание: %w", err)
	}

	if err := privilege.RunElevated(dst); err != nil {
		return fmt.Errorf("запуск установщика: %w", err)
	}
	a.quit()
	return nil
}
```

- [ ] **Step 2: Build the wails target to verify it compiles**

Run: `go build -tags wails ./...` 2>&1 | head
Expected: no errors referencing `DownloadAndInstall` (CGO/webkit link errors unrelated to this change are acceptable on a headless host; a clean `go vet -tags wails .` is the fallback check).

Fallback if the full wails build needs the GUI toolchain:
Run: `go vet -tags wails .`
Expected: no errors about `DownloadAndInstall`.

- [ ] **Step 3: Add the frontend binding (App.js)**

In `frontend/wailsjs/go/main/App.js`, add (next to `CheckUpdate`):

```js
export function DownloadAndInstall() {
  return window['go']['main']['App']['DownloadAndInstall']();
}
```

- [ ] **Step 4: Add the binding type (App.d.ts)**

In `frontend/wailsjs/go/main/App.d.ts`, add (next to `CheckUpdate`):

```ts
export function DownloadAndInstall():Promise<void>;
```

- [ ] **Step 5: Commit**

```bash
git add gui_app.go frontend/wailsjs/go/main/App.js frontend/wailsjs/go/main/App.d.ts
git commit -m "feat(gui): DownloadAndInstall bound method with progress events"
```

---

## Task 5: Frontend — update button + progress bar

**Files:**
- Modify: `frontend/index.html`
- Modify: `frontend/src/main.ts`
- Modify: `frontend/src/style.css`

- [ ] **Step 1: Update the banner markup**

In `frontend/index.html`, replace the update-banner block (currently lines ~9-13) with:

```html
    <div id="update-banner" class="update-banner hidden">
      <span id="update-text">
        Доступна новая версия <strong id="update-version"></strong> —
        <a id="update-link" href="#" target="_blank">в браузере</a>
        <button id="update-btn" class="update-btn">Обновить</button>
      </span>
      <span id="update-progress-wrap" class="hidden">
        Скачивание… <span id="update-percent">0%</span>
        <span class="update-bar"><span id="update-bar-fill" class="update-bar-fill"></span></span>
      </span>
      <button id="update-dismiss" class="update-dismiss" title="Закрыть">✕</button>
    </div>
```

- [ ] **Step 2: Add the import**

In `frontend/src/main.ts`, add `DownloadAndInstall` to the `App` import block (after `CheckUpdate,`):

```ts
  CheckUpdate,
  DownloadAndInstall,
```

- [ ] **Step 3: Wire the button + progress listener**

In `frontend/src/main.ts`, replace the `checkUpdate` function with:

```ts
function checkUpdate() {
  CheckUpdate().then((info) => {
    if (!info.available) return;
    const banner = $("update-banner");
    $("update-version").textContent = info.version;
    const link = <HTMLAnchorElement>$("update-link");
    link.href = "#";
    link.onclick = (e) => { e.preventDefault(); BrowserOpenURL(info.url); };

    $("update-btn").onclick = () => {
      $("update-text").classList.add("hidden");
      $("update-progress-wrap").classList.remove("hidden");
      DownloadAndInstall().catch((err) => {
        // UAC declined / network / no asset — restore the banner with a note.
        $("update-progress-wrap").classList.add("hidden");
        $("update-text").classList.remove("hidden");
        $("error-line").textContent = "Обновление не удалось: " + String(err);
      });
    };

    banner.classList.remove("hidden");
    $("update-dismiss").onclick = () => banner.classList.add("hidden");
  }).catch(() => {/* network error — silently ignore */});
}
```

Then add an `update-progress` listener inside `wire()`, next to the other `EventsOn` calls (after the `"log"` listener near line 382):

```ts
  EventsOn("update-progress", (p: { done: number; total: number }) => {
    const pct = p.total > 0 ? Math.round((p.done / p.total) * 100) : 0;
    $("update-percent").textContent = pct + "%";
    (<HTMLElement>$("update-bar-fill")).style.width = pct + "%";
  });
```

- [ ] **Step 4: Add progress-bar styles**

In `frontend/src/style.css`, after the `.update-dismiss` rule, add:

```css
.update-btn {
  margin-left: 8px;
  background: var(--accent);
  color: #000;
  border: none;
  border-radius: 4px;
  padding: 2px 10px;
  cursor: pointer;
}
.update-bar {
  display: inline-block;
  width: 160px;
  height: 8px;
  margin-left: 8px;
  background: rgba(255, 255, 255, 0.15);
  border-radius: 4px;
  vertical-align: middle;
  overflow: hidden;
}
.update-bar-fill {
  display: block;
  height: 100%;
  width: 0%;
  background: var(--accent);
  transition: width 0.2s ease;
}
```

- [ ] **Step 5: Build the frontend to verify it compiles**

Run: `cd frontend && npm run build`
Expected: build succeeds, no TypeScript errors. (`$` is the existing `getElementById` helper; `EventsOn`/`BrowserOpenURL` already imported.)

- [ ] **Step 6: Commit**

```bash
git add frontend/index.html frontend/src/main.ts frontend/src/style.css
git commit -m "feat(frontend): update button + download progress bar"
```

---

## Final verification

- [ ] **Run the full Go test suite**

Run: `go test ./...`
Expected: all packages PASS.

- [ ] **Cross-compile the unit-testable packages for Windows**

Run: `GOOS=windows CGO_ENABLED=0 go build ./internal/updater/ ./internal/privilege/`
Expected: succeeds, no output.

- [ ] **Manual QA (Windows host, requires a real newer release):**
  - Banner appears when a newer release exists.
  - «Обновить» → progress bar advances 0→100%.
  - UAC prompt appears; accepting launches the NSIS wizard (which detects the existing version per feature #1 and offers upgrade).
  - App quits; after install, the new version runs.
  - Declining UAC restores the banner with the error note (app stays running).

---

## Notes for the implementer

- `wailsjs/go/*` bindings regenerate on every `wails build`; the hand-added `DownloadAndInstall` entries in Task 4 match what wails generates, so they survive regeneration as a no-op diff.
- The Windows installer asset is named `kub-connect-amd64-installer.exe` (from `build/windows/installer/project.nsi` `OutFile`). `PickInstaller` matches the `-installer.exe` suffix, so a future ARM64 asset would also need disambiguation — out of scope now (amd64-only release).
- Do not add a checksums file or touch the release workflow — the spec deliberately relies on HTTPS + the `a.Size` sanity check (unsigned binary; same-release checksum adds no real security).
