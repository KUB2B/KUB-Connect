# Release Prep v0.1.0 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship vless-client v0.1.0 for Windows (proxy + TUN) and macOS (proxy) via GitHub Releases, with signed/notarized macOS `.dmg`, Windows NSIS installer, platform-aware GUI, and user-facing docs.

**Architecture:** Inject per-OS capability flags (`TUNSupported`, `KillSwitchSupported`, OS, version) into the platform-free `app.Service`; expose them in `StateDTO.Caps` so the frontend hides the TUN mode on macOS. Build/package via local scripts (Windows cross-compiled from Linux, macOS built+notarized on a Mac), published manually with `gh release create`.

**Tech Stack:** Go 1.26, Wails v2, TypeScript/Vite frontend, NSIS (Windows installer), `codesign`/`notarytool`/`stapler` (macOS), `gh` CLI.

---

## File Structure

**Created:**
- `version.go` — root `package main` var `version`, no build tag (compiles in both wails and stub builds)
- `internal/netcfg/*` — add `Supported()` per-OS (no new files; edits)
- `scripts/release.sh` — orchestrator: dispatch to per-OS build, collect artifacts
- `scripts/build-windows.sh` — Windows cross-build + NSIS
- `scripts/build-macos.sh` — macOS build + sign + dmg + notarize + staple
- `docs/INSTALL.md` — user-facing install guide (RU)
- `CHANGELOG.md` — v0.1.0 entry

**Modified:**
- `internal/netcfg/netcfg_linux.go`, `netcfg_windows.go`, `netcfg_other.go` — add `Supported()`
- `internal/app/types.go` — add `Deps.TUNSupported`, `Deps.Version`, `Deps.OS`, `CapsDTO`, `StateDTO.Caps`
- `internal/app/app.go` — compute caps + TUN→proxy fallback in `New`/`snapshot`
- `internal/app/connect.go` — add `tunSupported()` helper (mirror `killSwitchSupported`)
- `gui_app.go` — wire new Deps fields (netcfg.Supported, runtime.GOOS, version)
- `cmd/headless/main.go` — `-version` flag + `var version`
- `build/windows/info.json` — fill ProductName/Version/Company/Copyright
- `frontend/src/main.ts` — read `caps`, gate TUN option, show version
- `frontend/index.html` — version footer slot
- `frontend/wailsjs/go/models.ts`, `frontend/wailsjs/go/main/App.d.ts` — regenerated DTOs
- `.gitignore` — release artifacts
- `README.md` — link to INSTALL.md

---

## Task 1: Repo cleanup

**Files:**
- Modify: `.gitignore`
- Delete (disk only, untracked): `app.test`

- [ ] **Step 1: Delete the stray test binary**

`app.test` is a 5 MB binary committed to the working tree but already untracked (matched by `*.test` in `.gitignore`). Remove it from disk:

```bash
rm -f app.test
```

- [ ] **Step 2: Ignore release artifacts**

Add to `.gitignore` under the `# Wails` block:

```
# Release artifacts
/build/release/
*.dmg
*-installer.exe
```

- [ ] **Step 3: Verify nothing stray is tracked**

Run: `git status --porcelain && git ls-files | grep -E '\.(test|exe|dmg)$' || echo "clean"`
Expected: no `app.test`; `clean` printed (no tracked binaries).

- [ ] **Step 4: Commit**

```bash
git add .gitignore
git commit -m "chore: ignore release artifacts; drop stray app.test binary"
```

---

## Task 2: netcfg.Supported() per-OS (TDD)

Mirrors the existing `firewall.Supported()` pattern so the app layer can ask whether TUN routing is implemented on the current OS.

**Files:**
- Modify: `internal/netcfg/netcfg_linux.go`, `internal/netcfg/netcfg_windows.go`, `internal/netcfg/netcfg_other.go`
- Test: `internal/netcfg/netcfg_linux_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/netcfg/netcfg_linux_test.go`:

```go
func TestSupportedLinux(t *testing.T) {
	if !Supported() {
		t.Fatal("Supported() = false on linux, want true")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/netcfg/ -run TestSupportedLinux -v`
Expected: FAIL — `undefined: Supported`

- [ ] **Step 3: Add Supported() to each OS file**

In `internal/netcfg/netcfg_linux.go` add:

```go
// Supported reports whether TUN routing is implemented on this OS.
func Supported() bool { return true }
```

In `internal/netcfg/netcfg_windows.go` add:

```go
// Supported reports whether TUN routing is implemented on this OS.
func Supported() bool { return true }
```

In `internal/netcfg/netcfg_other.go` add:

```go
// Supported reports whether TUN routing is implemented on this OS.
func Supported() bool { return false }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/netcfg/ -run TestSupportedLinux -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/netcfg/
git commit -m "feat(netcfg): add Supported() reporting TUN routing availability per OS"
```

---

## Task 3: App capability flags + Caps DTO (TDD)

Inject `TUNSupported`, `OS`, and `Version` into `app.Service` (platform-free), expose them via `StateDTO.Caps`, and coerce a persisted `tun` mode to `proxy` when TUN is unsupported.

**Files:**
- Modify: `internal/app/types.go`, `internal/app/app.go`, `internal/app/connect.go`
- Test: `internal/app/app_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/app/app_test.go` (uses the existing test helpers; if a service constructor helper exists, follow its pattern — otherwise call `New` directly with a temp state path):

```go
func TestCapsAndTUNFallback(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")

	// Persist a state whose mode is TUN.
	st := store.Default()
	st.Settings.Mode = store.ModeTUN
	if err := store.Save(statePath, st); err != nil {
		t.Fatal(err)
	}

	svc, err := New(Deps{
		StatePath:    statePath,
		TUNSupported: func() bool { return false },
		OS:           "darwin",
		Version:      "v9.9.9",
	})
	if err != nil {
		t.Fatal(err)
	}

	got := svc.GetState()
	if got.Caps.TUNSupported {
		t.Error("Caps.TUNSupported = true, want false")
	}
	if got.Caps.OS != "darwin" {
		t.Errorf("Caps.OS = %q, want darwin", got.Caps.OS)
	}
	if got.Caps.Version != "v9.9.9" {
		t.Errorf("Caps.Version = %q, want v9.9.9", got.Caps.Version)
	}
	if got.Settings.Mode != string(store.ModeProxy) {
		t.Errorf("Settings.Mode = %q, want proxy (TUN should fall back)", got.Settings.Mode)
	}
}
```

Ensure the test file imports `path/filepath` and `github.com/zki/vless-client/internal/store`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/app/ -run TestCapsAndTUNFallback -v`
Expected: FAIL — `unknown field 'TUNSupported' in struct literal` / `got.Caps undefined`

- [ ] **Step 3: Add Deps fields and CapsDTO**

In `internal/app/types.go`, extend `Deps` (after `KillSwitchSupported`):

```go
	// TUNSupported reports whether TUN routing is implemented on this OS
	// (netcfg.Supported). Nil is treated as supported, so tests need not set it.
	TUNSupported func() bool
	// OS is the runtime GOOS, surfaced to the frontend via Caps.
	OS string
	// Version is the build version string, surfaced to the frontend via Caps.
	Version string
```

Add the `CapsDTO` type and a `Caps` field on `StateDTO`:

```go
// CapsDTO tells the frontend what this build/platform supports.
type CapsDTO struct {
	OS                  string `json:"os"`
	Version             string `json:"version"`
	TUNSupported        bool   `json:"tunSupported"`
	KillSwitchSupported bool   `json:"killSwitchSupported"`
}
```

In `StateDTO` add the field:

```go
	Caps         CapsDTO     `json:"caps"`
```

- [ ] **Step 4: Add tunSupported() helper**

In `internal/app/connect.go`, next to `killSwitchSupported`:

```go
// tunSupported reports whether TUN routing can run on this OS. A nil dep is
// treated as supported so tests need not set it.
func (s *Service) tunSupported() bool {
	return s.deps.TUNSupported == nil || s.deps.TUNSupported()
}
```

- [ ] **Step 5: Populate Caps and apply fallback**

In `internal/app/app.go`, in `New` (after loading `st`, before returning), coerce mode:

```go
	if st.Settings.Mode == store.ModeTUN && d.TUNSupported != nil && !d.TUNSupported() {
		st.Settings.Mode = store.ModeProxy
	}
```

In `snapshot()`, set the `Caps` field on the returned `StateDTO`:

```go
		Caps: CapsDTO{
			OS:                  s.deps.OS,
			Version:             s.deps.Version,
			TUNSupported:        s.tunSupported(),
			KillSwitchSupported: s.killSwitchSupported(),
		},
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/app/ -run TestCapsAndTUNFallback -v`
Expected: PASS

- [ ] **Step 7: Run full app + netcfg suites**

Run: `go test ./internal/app/ ./internal/netcfg/`
Expected: `ok` for both.

- [ ] **Step 8: Commit**

```bash
git add internal/app/
git commit -m "feat(app): expose platform Caps DTO; fall back TUN->proxy when unsupported"
```

---

## Task 4: Version variable + headless --version

**Files:**
- Create: `version.go`
- Modify: `cmd/headless/main.go`

- [ ] **Step 1: Add root version var**

Create `version.go` (root `package main`, NO build tag so it compiles in both the wails and stub builds):

```go
package main

// version is the build version, overridden at release time via
//	-ldflags "-X main.version=v0.1.0"
var version = "dev"
```

- [ ] **Step 2: Add headless version var + flag**

In `cmd/headless/main.go`, add a package-level var near the top of the file (after imports):

```go
// version is overridden at release time via -ldflags "-X main.version=v0.1.0".
var version = "dev"
```

In `main()`, add the flag alongside the others and handle it right after `flag.Parse()`:

```go
	showVersion := flag.Bool("version", false, "print version and exit")
```

```go
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}
```

- [ ] **Step 3: Verify both builds compile and version threads through**

Run:
```bash
go build ./cmd/headless && go build -ldflags "-X main.version=v0.1.0-test" -o /tmp/hl ./cmd/headless && /tmp/hl -version
```
Expected: prints `v0.1.0-test`.

Also verify the stub build still compiles:
Run: `go build -o /tmp/stub . && echo ok`
Expected: `ok`

- [ ] **Step 4: Commit**

```bash
git add version.go cmd/headless/main.go
git commit -m "feat: add build version var and headless --version flag"
```

---

## Task 5: Wire version + capabilities into the GUI app

**Files:**
- Modify: `gui_app.go`

- [ ] **Step 1: Pass new Deps from startup**

In `gui_app.go`, add the `netcfg` and `runtime` imports:

```go
	"runtime"

	"github.com/zki/vless-client/internal/netcfg"
```

In `startup`, extend the `app.New(app.Deps{...})` literal with:

```go
		KillSwitchSupported: firewall.Supported,
		TUNSupported:        netcfg.Supported,
		OS:                  runtime.GOOS,
		Version:             version,
```

(Keep the existing `KillSwitchSupported` line; add the three new fields.)

- [ ] **Step 2: Verify the GUI build compiles**

Run: `wails build -tags "wails webkit2_41" -ldflags "-X main.version=v0.1.0-test"`
Expected: build succeeds, binary at `build/bin/vless-client`.

> If a Wails toolchain is unavailable in the execution environment, fall back to a compile check of the wails-tagged package:
> Run: `go build -tags "wails webkit2_41" -o /tmp/gui . && echo ok` — Expected: `ok`.

- [ ] **Step 3: Commit**

```bash
git add gui_app.go
git commit -m "feat(gui): inject TUN/killswitch caps, OS, and version into the service"
```

---

## Task 6: Regenerate Wails bindings (Caps DTO)

The `StateDTO.Caps` field must appear in the generated TypeScript models the frontend imports.

**Files:**
- Modify: `frontend/wailsjs/go/models.ts`

- [ ] **Step 1: Regenerate bindings**

Run: `wails generate module`
Expected: `frontend/wailsjs/go/models.ts` now contains a `CapsDTO` class and a `caps` field on the state class.

> If `wails generate module` is unavailable, hand-edit `frontend/wailsjs/go/models.ts`: add a `CapsDTO` class with fields `os: string`, `version: string`, `tunSupported: boolean`, `killSwitchSupported: boolean`, and add `caps: CapsDTO` to the state DTO class (matching the JSON tags `os`/`version`/`tunSupported`/`killSwitchSupported`/`caps`).

- [ ] **Step 2: Verify the field exists**

Run: `grep -n "tunSupported\|CapsDTO\|caps" frontend/wailsjs/go/models.ts`
Expected: matches for `CapsDTO`, `tunSupported`, and `caps`.

- [ ] **Step 3: Commit**

```bash
git add frontend/wailsjs/
git commit -m "chore(wailsjs): regenerate models for Caps DTO"
```

---

## Task 7: Frontend — gate TUN on macOS + show version

The frontend has no test harness, so this task is verified by build + manual inspection rather than TDD.

**Files:**
- Modify: `frontend/src/main.ts`, `frontend/index.html`

- [ ] **Step 1: Add Caps to the State type**

In `frontend/src/main.ts`, add a `Caps` type and a field on `State`:

```ts
type Caps = {
  os: string;
  version: string;
  tunSupported: boolean;
  killSwitchSupported: boolean;
};
```

In the `State` type, add:

```ts
  caps: Caps;
```

- [ ] **Step 2: Gate the TUN option in render()**

In `render(st: State)`, after the `mode-select` value is set (currently `frontend/src/main.ts:106`), add:

```ts
  const modeSel = <HTMLSelectElement>$("mode-select");
  const tunOpt = modeSel.querySelector<HTMLOptionElement>('option[value="tun"]');
  if (tunOpt) {
    tunOpt.disabled = !st.caps.tunSupported;
    tunOpt.hidden = !st.caps.tunSupported;
  }
  if (!st.caps.tunSupported && modeSel.value === "tun") {
    modeSel.value = "proxy";
  }
```

- [ ] **Step 3: Render the version footer**

In `frontend/index.html`, add a version slot at the end of the main container (before the closing body content area; place it after the last tab panel):

```html
<div id="app-version" class="version"></div>
```

In `frontend/src/main.ts` `render()`, set its text:

```ts
  $("app-version").textContent = st.caps.version;
```

- [ ] **Step 4: Verify frontend builds**

Run: `cd frontend && npm install && npm run build`
Expected: build succeeds, no TypeScript errors (`caps` resolves against the regenerated models).

- [ ] **Step 5: Manual check (when GUI runs)**

On macOS the `mode-select` shows only "Proxy" (TUN hidden); the version string appears in the footer. Note this as a QA item for Task 11.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/main.ts frontend/index.html
git commit -m "feat(ui): hide TUN mode where unsupported; show build version"
```

---

## Task 8: Windows version metadata

**Files:**
- Modify: `build/windows/info.json`

- [ ] **Step 1: Fill the version manifest**

Replace `build/windows/info.json` template placeholders are driven by Wails from `wails.json` + flags; set concrete values so the `.exe` carries real metadata. Update `wails.json` `info` block (Wails reads product metadata there) — add an `info` section:

```json
  "info": {
    "companyName": "qb2b",
    "productName": "VLESS Client",
    "productVersion": "0.1.0",
    "copyright": "Copyright © 2026 qb2b",
    "comments": "VLESS+Xray desktop client"
  }
```

Leave `build/windows/info.json` as the template that references `{{.Info.*}}` — Wails fills it from `wails.json`. Confirm the placeholders in `info.json` match the keys above.

- [ ] **Step 2: Verify the Windows build embeds metadata**

Run: `wails build -platform windows/amd64 -tags wails -ldflags "-X main.version=v0.1.0" -nsis`
Expected: produces `build/bin/vless-client-amd64-installer.exe`; the `vless-client.exe` properties show ProductName "VLESS Client", version 0.1.0.

> If cross-build toolchain is unavailable here, defer this verification to the release run (Task 9) and just commit the metadata.

- [ ] **Step 3: Commit**

```bash
git add wails.json build/windows/info.json
git commit -m "chore(build): set Windows product metadata (name, version, company)"
```

---

## Task 9: Build & packaging scripts

**Files:**
- Create: `scripts/build-windows.sh`, `scripts/build-macos.sh`, `scripts/release.sh`

- [ ] **Step 1: Windows build script**

Create `scripts/build-windows.sh`:

```bash
#!/usr/bin/env bash
# Cross-compile the Windows GUI from Linux and produce an NSIS installer.
# Usage: VER=v0.1.0 scripts/build-windows.sh
set -euo pipefail

VER="${VER:-$(git describe --tags --always)}"
OUT="build/release"
mkdir -p "$OUT"

wails build -platform windows/amd64 -tags wails \
  -ldflags "-X main.version=${VER}" -nsis

cp build/bin/vless-client-amd64-installer.exe \
   "${OUT}/vless-client-${VER}-windows-amd64-installer.exe"

echo "Windows artifact: ${OUT}/vless-client-${VER}-windows-amd64-installer.exe (UNSIGNED)"
```

- [ ] **Step 2: macOS build script**

Create `scripts/build-macos.sh`:

```bash
#!/usr/bin/env bash
# Build, sign, package (.dmg), notarize, and staple the macOS GUI.
# Run ON A MAC. Required env:
#   VER                     release version, e.g. v0.1.0 (default: git describe)
#   APPLE_SIGN_IDENTITY     "Developer ID Application: NAME (TEAMID)"
#   APPLE_ID                Apple account email
#   APPLE_TEAM_ID           10-char team id
#   APPLE_APP_PASSWORD      app-specific password for notarytool
set -euo pipefail

VER="${VER:-$(git describe --tags --always)}"
: "${APPLE_SIGN_IDENTITY:?set APPLE_SIGN_IDENTITY}"
: "${APPLE_ID:?set APPLE_ID}"
: "${APPLE_TEAM_ID:?set APPLE_TEAM_ID}"
: "${APPLE_APP_PASSWORD:?set APPLE_APP_PASSWORD}"

OUT="build/release"
APP="build/bin/vless-client.app"
DMG="${OUT}/vless-client-${VER}-macos-universal.dmg"
mkdir -p "$OUT"

wails build -platform darwin/universal -tags wails \
  -ldflags "-X main.version=${VER}"

codesign --deep --force --options runtime --timestamp \
  --sign "${APPLE_SIGN_IDENTITY}" "${APP}"

# Build the DMG (uses hdiutil; a staging dir keeps the layout minimal).
STAGE="$(mktemp -d)"
cp -R "${APP}" "${STAGE}/"
ln -s /Applications "${STAGE}/Applications"
hdiutil create -volname "VLESS Client" -srcfolder "${STAGE}" \
  -ov -format UDZO "${DMG}"
rm -rf "${STAGE}"

codesign --force --timestamp --sign "${APPLE_SIGN_IDENTITY}" "${DMG}"

xcrun notarytool submit "${DMG}" \
  --apple-id "${APPLE_ID}" --team-id "${APPLE_TEAM_ID}" \
  --password "${APPLE_APP_PASSWORD}" --wait

xcrun stapler staple "${DMG}"

echo "macOS artifact: ${DMG} (signed + notarized)"
```

- [ ] **Step 3: Release orchestrator**

Create `scripts/release.sh`:

```bash
#!/usr/bin/env bash
# Orchestrate a release build. On Linux: builds Windows. On macOS: builds macOS.
# Publish step is manual (see PUBLISH note printed at the end).
# Usage: VER=v0.1.0 scripts/release.sh
set -euo pipefail

VER="${VER:-$(git describe --tags --always)}"
export VER

case "$(uname -s)" in
  Linux)  scripts/build-windows.sh ;;
  Darwin) scripts/build-macos.sh ;;
  *) echo "unsupported build host: $(uname -s)" >&2; exit 1 ;;
esac

echo
echo "PUBLISH (run after both Windows and macOS artifacts are in build/release/):"
echo "  gh release create ${VER} build/release/* \\"
echo "    --title \"VLESS Client ${VER}\" --notes-file CHANGELOG.md"
```

- [ ] **Step 4: Make scripts executable + smoke-check syntax**

Run:
```bash
chmod +x scripts/build-windows.sh scripts/build-macos.sh scripts/release.sh
bash -n scripts/build-windows.sh && bash -n scripts/build-macos.sh && bash -n scripts/release.sh && echo "syntax ok"
```
Expected: `syntax ok`

- [ ] **Step 5: Commit**

```bash
git add scripts/
git commit -m "build: add per-OS release build scripts + orchestrator"
```

---

## Task 10: User documentation + CHANGELOG

**Files:**
- Create: `docs/INSTALL.md`, `CHANGELOG.md`
- Modify: `README.md`

- [ ] **Step 1: Write the install guide**

Create `docs/INSTALL.md`:

```markdown
# Установка VLESS Client

## Windows

1. Скачайте `vless-client-vX.Y.Z-windows-amd64-installer.exe` со страницы
   [Releases](https://github.com/zki/vless-client/releases).
2. При запуске Windows SmartScreen покажет «Система Windows защитила ваш
   компьютер» — приложение не подписано. Нажмите **Подробнее → Всё равно
   выполнить**.
3. Пройдите установку, запустите «VLESS Client».
4. На вкладке серверов вставьте `vless://…` ссылку и нажмите добавить.
5. Выберите режим:
   - **Proxy** — системный SOCKS, без прав администратора.
   - **TUN** — полный перехват, требует прав администратора.
6. Нажмите кнопку питания для подключения.

## macOS

1. Скачайте `vless-client-vX.Y.Z-macos-universal.dmg` со страницы Releases.
2. Откройте `.dmg`, перетащите «VLESS Client» в папку Applications.
3. Запустите приложение (оно подписано и нотаризовано — предупреждений не будет).
4. Добавьте `vless://…` ссылку, выберите режим **Proxy** и подключитесь.

> На macOS доступен только режим **Proxy** — TUN в этой версии не реализован.

## Как работает маршрутизация

Клиент использует **whitelist**: по умолчанию весь трафик идёт напрямую
(direct), а в VPN заворачиваются только Telegram и ваши кастомные IP/домены.
Правила `geoip:ru` / `geosite:category-ru` всегда идут напрямую.

## Известные ограничения (v0.1.0)

- **Kill switch** не реализован ни на одной ОС.
- **macOS** — только режим Proxy (TUN не поддерживается).
- **Windows** — бинарь не подписан, SmartScreen покажет предупреждение.
- Нет авто-обновления — следите за страницей Releases.
```

- [ ] **Step 2: Write the changelog**

Create `CHANGELOG.md`:

```markdown
# Changelog

## v0.1.0 — 2026-06-08

Первый публичный релиз.

### Возможности
- VLESS+Xray клиент с whitelist-маршрутизацией (Telegram + кастомные IP/домены).
- **Windows:** режимы Proxy и TUN.
- **macOS:** режим Proxy.
- Встроенные geo-базы (geoip/geosite) — отдельная установка не нужна.
- GUI на Wails: управление серверами, выбор режима, уровень логов, логи.

### Известные ограничения
- Kill switch не реализован.
- macOS TUN не поддерживается.
- Windows-бинарь не подписан (SmartScreen warning).
```

- [ ] **Step 3: Link the guide from README**

In `README.md`, add near the top (after the one-line description):

```markdown
> 📦 **Установка для пользователей:** см. [docs/INSTALL.md](docs/INSTALL.md).
```

- [ ] **Step 4: Verify docs render / no broken intent**

Run: `ls docs/INSTALL.md CHANGELOG.md && grep -q "INSTALL.md" README.md && echo ok`
Expected: `ok`

- [ ] **Step 5: Commit**

```bash
git add docs/INSTALL.md CHANGELOG.md README.md
git commit -m "docs: add user install guide and v0.1.0 changelog"
```

---

## Task 11: QA + release (manual)

This task is manual verification on real machines plus the publish step. No code.

- [ ] **Step 1: Tag the release**

```bash
git tag v0.1.0
```

- [ ] **Step 2: Build Windows (on Linux host)**

Run: `VER=v0.1.0 scripts/release.sh`
Expected: `build/release/vless-client-v0.1.0-windows-amd64-installer.exe` exists.

- [ ] **Step 3: Build macOS (on the Mac, with Apple env set)**

Run (with `APPLE_*` env vars exported): `VER=v0.1.0 scripts/release.sh`
Expected: `build/release/vless-client-v0.1.0-macos-universal.dmg` exists, notarized.

- [ ] **Step 4: Manual QA — Windows**

Install via the installer. Verify: app launches; add a real `vless://` server; **Proxy** mode connects (status → Подключено), traffic to Telegram routes through VPN, other traffic direct; **TUN** mode connects with admin rights; version shows in footer.

- [ ] **Step 5: Manual QA — macOS**

Open `.dmg`, drag to Applications, launch — **no Gatekeeper bypass needed**. Verify: TUN option is hidden in the mode dropdown; Proxy connects; version shows in footer.

- [ ] **Step 6: Publish**

```bash
git push origin main --tags
gh release create v0.1.0 build/release/* \
  --title "VLESS Client v0.1.0" --notes-file CHANGELOG.md
```
Expected: GitHub Release v0.1.0 with both artifacts attached.

---

## Self-Review notes

- **Spec coverage:** Scope/platform matrix → Tasks 7,8,10,11. Version+metadata → Tasks 4,5,8. GUI gating → Tasks 2,3,6,7. Build/packaging → Tasks 8,9,11. Docs → Task 10. Repo cleanup → Task 1. All spec components mapped.
- **Correction vs spec:** `app.test` is untracked (matched by `*.test`), so Task 1 deletes it from disk only — no `git rm`.
- **Type consistency:** `Supported()` (netcfg), `TUNSupported`/`OS`/`Version` (Deps), `CapsDTO{os,version,tunSupported,killSwitchSupported}`, `StateDTO.Caps`, frontend `Caps`/`caps` — names consistent across Tasks 2,3,6,7.
- **TDD scope:** Go capability code (Tasks 2,3) is TDD. Frontend (Task 7) has no harness → build + manual verification. Scripts/docs/metadata → non-code, verified by syntax/build checks.
