# Anthropic Preset + Telegram Toggle-Grid Move Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a "Claude (Anthropic)" service preset to whitelist routing, and visually move the Telegram toggle into the same preset grid it sits above today, without changing its default-on behavior or its dedicated routing mechanism.

**Architecture:** One new entry in the Go `routing.Presets` slice (domain-list-only preset, same shape as the existing 8), the matching entry in the frontend's `PRESETS` mirror array, and a small HTML/CSS-class move of the existing `#tg-toggle` checkbox into `#preset-list` — plus a required accompanying fix so the presets `change` handler's `querySelectorAll` doesn't accidentally sweep up `#tg-toggle` once it's a sibling of the preset checkboxes.

**Tech Stack:** Go (routing package), vanilla TypeScript + HTML/CSS (frontend, no framework) — same stack as the rest of the app, no new dependencies.

## Global Constraints

- `geosite:anthropic` is confirmed present in the currently-embedded `data/geosite.dat` (verified via `strings data/geosite.dat | grep -i anthropic`, matches `claude.ai`, `anthropic.com`, `claude.com`, etc.) — use this category, not a hand-maintained domain list.
- `Profile.Telegram bool` and its routing logic (`internal/routing/profile.go` lines ~141-144: baked-in `TelegramCIDRs` + `geosite:telegram`) do not change. `routing.Default()` already returns `Telegram: true` — no default-on logic needs to be added or changed.
- The `PRESETS` array in `frontend/src/main.ts` must stay in the same order and with the same keys as `routing.Presets` in `internal/routing/profile.go` (existing comment: "Keep in sync with routing.Presets").
- No frontend test framework exists; frontend verification per task is `cd frontend && npx tsc --noEmit` (no output) and `npm run build` (exit 0), followed by `git checkout -- frontend/dist/.gitkeep` to restore that tracked file (a known repo quirk: `vite build` deletes it since `dist/` is otherwise gitignored).
- Go verification is `go test ./... -count=1` (all packages `ok`) and `go build ./...` (exit 0, no output).

---

### Task 1: Add the Anthropic preset (Go backend)

**Files:**
- Modify: `internal/routing/profile.go:59` (end of the `Presets` slice)
- Test: `internal/routing/profile_test.go`

**Interfaces:**
- Produces: a new entry in `routing.Presets` with `Key: "anthropic"`, consumed by `PresetByKey("anthropic")` and by `Profile.presetDomains()` when `"anthropic"` is present in `ProxyPresets`. Later tasks (2-4) rely on this exact key string matching the frontend's `PRESETS` entry.

- [ ] **Step 1: Write a failing test for the new preset**

Open `internal/routing/profile_test.go` and find the existing test that exercises `PresetByKey` or `Presets` (search for `PresetByKey` or `"openai"` to locate it). Add this test function anywhere in the file (top-level, alongside the other `Test...` functions):

```go
func TestPresetByKey_Anthropic(t *testing.T) {
	p, ok := PresetByKey("anthropic")
	if !ok {
		t.Fatal("expected \"anthropic\" preset to exist")
	}
	if p.Title != "Claude (Anthropic)" {
		t.Errorf("Title = %q, want %q", p.Title, "Claude (Anthropic)")
	}
	if len(p.Domains) != 1 || p.Domains[0] != "geosite:anthropic" {
		t.Errorf("Domains = %v, want [\"geosite:anthropic\"]", p.Domains)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd /home/zki/projects/vless-client && go test ./internal/routing/... -run TestPresetByKey_Anthropic -v`
Expected: FAIL — `expected "anthropic" preset to exist`

- [ ] **Step 3: Add the preset**

In `internal/routing/profile.go`, find:
```go
	{Key: "openai", Title: "ChatGPT (OpenAI)", Domains: []string{"geosite:openai"}},
}
```
Replace with:
```go
	{Key: "openai", Title: "ChatGPT (OpenAI)", Domains: []string{"geosite:openai"}},
	{Key: "anthropic", Title: "Claude (Anthropic)", Domains: []string{"geosite:anthropic"}},
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd /home/zki/projects/vless-client && go test ./internal/routing/... -run TestPresetByKey_Anthropic -v`
Expected: PASS

- [ ] **Step 5: Run the full routing package test suite**

Run: `cd /home/zki/projects/vless-client && go test ./internal/routing/... -count=1 -v`
Expected: all tests PASS, output pristine (no warnings)

- [ ] **Step 6: Run the full Go build and test suite**

Run: `cd /home/zki/projects/vless-client && go build ./... && go test ./... -count=1`
Expected: build succeeds silently; every package reports `ok`

- [ ] **Step 7: Commit**

```bash
cd /home/zki/projects/vless-client
git add internal/routing/profile.go internal/routing/profile_test.go
git commit -m "feat(routing): add Anthropic (Claude) service preset"
```

---

### Task 2: Mirror the Anthropic preset in the frontend + fix the preset-checkbox selector

**Files:**
- Modify: `frontend/src/main.ts:34-43` (the `PRESETS` array)
- Modify: `frontend/src/main.ts:407-419` (the `PRESETS.forEach` block in `wire()`)

**Interfaces:**
- Consumes: the `"anthropic"` key produced by Task 1 (Go side) — must match exactly.
- Produces: `cb.dataset.preset = "1"` marker on every preset checkbox, and the updated `querySelectorAll` selector `'#preset-list input[data-preset]:checked'`. Task 3 relies on this marker existing so that moving `#tg-toggle` into `#preset-list` in Task 3 doesn't get swept into `profile.proxyPresets`.

This task lands the required "sibling of the preset checkboxes" fix from the spec BEFORE Task 3 physically moves `#tg-toggle` into `#preset-list` — so at no point in git history does the bug exist even transiently.

- [ ] **Step 1: Add the Anthropic entry to the `PRESETS` array**

Find in `frontend/src/main.ts`:
```ts
// Keep in sync with routing.Presets (internal/routing/profile.go).
const PRESETS: { key: string; title: string }[] = [
  { key: "youtube", title: "YouTube" },
  { key: "instagram", title: "Instagram" },
  { key: "facebook", title: "Facebook" },
  { key: "twitter", title: "Twitter / X" },
  { key: "discord", title: "Discord" },
  { key: "netflix", title: "Netflix" },
  { key: "spotify", title: "Spotify" },
  { key: "openai", title: "ChatGPT (OpenAI)" },
];
```

Replace with:
```ts
// Keep in sync with routing.Presets (internal/routing/profile.go).
const PRESETS: { key: string; title: string }[] = [
  { key: "youtube", title: "YouTube" },
  { key: "instagram", title: "Instagram" },
  { key: "facebook", title: "Facebook" },
  { key: "twitter", title: "Twitter / X" },
  { key: "discord", title: "Discord" },
  { key: "netflix", title: "Netflix" },
  { key: "spotify", title: "Spotify" },
  { key: "openai", title: "ChatGPT (OpenAI)" },
  { key: "anthropic", title: "Claude (Anthropic)" },
];
```

- [ ] **Step 2: Mark preset checkboxes and narrow the change-handler selector**

Find in `frontend/src/main.ts`:
```ts
  // Preset checkboxes (built once; render() syncs the checked state).
  const presetBox = $("preset-list");
  PRESETS.forEach((p) => {
    const label = document.createElement("label");
    const cb = document.createElement("input");
    cb.type = "checkbox";
    cb.className = "toggle-switch";
    cb.value = p.key;
    cb.addEventListener("change", () => {
      current.profile.proxyPresets = Array.from(
        document.querySelectorAll<HTMLInputElement>("#preset-list input:checked"),
      ).map((c) => c.value);
      pushProfile();
    });
    label.append(cb, " " + p.title);
    presetBox.append(label);
  });
```

Replace with:
```ts
  // Preset checkboxes (built once; render() syncs the checked state).
  // `data-preset` marks these (as opposed to #tg-toggle, which also lives
  // inside #preset-list for layout but is NOT one of these — see below) so
  // the change handler's query doesn't sweep up Telegram's checkbox.
  const presetBox = $("preset-list");
  PRESETS.forEach((p) => {
    const label = document.createElement("label");
    const cb = document.createElement("input");
    cb.type = "checkbox";
    cb.className = "toggle-switch";
    cb.dataset.preset = "1";
    cb.value = p.key;
    cb.addEventListener("change", () => {
      current.profile.proxyPresets = Array.from(
        document.querySelectorAll<HTMLInputElement>('#preset-list input[data-preset]:checked'),
      ).map((c) => c.value);
      pushProfile();
    });
    label.append(cb, " " + p.title);
    presetBox.append(label);
  });
```

- [ ] **Step 3: Verify**

Run: `cd /home/zki/projects/vless-client/frontend && npx tsc --noEmit`
Expected: no output, exit 0

Run: `cd /home/zki/projects/vless-client/frontend && npm run build`
Expected: exit 0

Run: `cd /home/zki/projects/vless-client && git checkout -- frontend/dist/.gitkeep`

- [ ] **Step 4: Commit**

```bash
cd /home/zki/projects/vless-client
git add frontend/src/main.ts
git commit -m "feat(ui): add Claude (Anthropic) preset, isolate preset selector from tg-toggle"
```

---

### Task 3: Move the Telegram checkbox into the preset grid

**Files:**
- Modify: `frontend/index.html` (the `#whitelist-only` block)

**Interfaces:**
- Consumes: `data-preset` marker semantics from Task 2 (this task relies on `#tg-toggle` NOT having `data-preset`, so it must not be added here).
- Produces: nothing consumed by later tasks (final task).

- [ ] **Step 1: Move the markup**

Find in `frontend/index.html`:
```html
          <div id="whitelist-only">
            <label><input type="checkbox" id="tg-toggle" /> Telegram → VPN</label>
            <span class="hint">Заворачивать Telegram в туннель.</span>
            <div id="preset-list" class="preset-list"></div>
            <span class="hint">Отмеченные сервисы идут через VPN. Если сервер пропускает только Telegram — оставьте всё выключенным.</span>
```

Replace with:
```html
          <div id="whitelist-only">
            <div id="preset-list" class="preset-list">
              <label><input type="checkbox" id="tg-toggle" class="toggle-switch" /> Telegram → VPN</label>
            </div>
            <span class="hint">Отмеченные сервисы идут через VPN. Telegram включён по умолчанию — если сервер пропускает только его, оставьте остальное выключенным.</span>
```

(Everything below this point in the file — the "Свои адреса через VPN" textarea and its hint — is unchanged; only the block shown above moves/changes.)

- [ ] **Step 2: Verify `#tg-toggle`'s own change handler and default-checked state are untouched**

Run: `grep -n '"tg-toggle"' /home/zki/projects/vless-client/frontend/src/main.ts`
Expected output includes these two (unchanged) lines — confirm both are present, this task does not touch `main.ts`:
```
  (<HTMLInputElement>$("tg-toggle")).checked = st.profile.telegram;
```
```
  $("tg-toggle").addEventListener("change", () => {
```

- [ ] **Step 3: Verify the id-integrity and dataset markers**

Run: `grep -n 'id="tg-toggle"' /home/zki/projects/vless-client/frontend/index.html`
Expected: exactly one match, and it does NOT include `data-preset` (open the line and confirm — it should read `<input type="checkbox" id="tg-toggle" class="toggle-switch" />`, nothing else).

- [ ] **Step 4: Verify build**

Run: `cd /home/zki/projects/vless-client/frontend && npx tsc --noEmit`
Expected: no output, exit 0

Run: `cd /home/zki/projects/vless-client/frontend && npm run build`
Expected: exit 0

Run: `cd /home/zki/projects/vless-client && git checkout -- frontend/dist/.gitkeep`

- [ ] **Step 5: Manual verification**

Run: `wails dev -tags "wails webkit2_41"` from the repo root (if a display is available in your environment — skip this step and note it as a concern in your report if not). Open Настройки → "Что через VPN" (expand the collapsed section). Confirm: Telegram appears as the first toggle-switch in the same grid as YouTube/Instagram/etc., is checked by default (on a profile with default settings), and "Claude (Anthropic)" appears as a toggle in the grid too. Toggle a couple of the OTHER presets (e.g. YouTube) on and off, then check that Telegram's own state is unaffected. If you can inspect network/logs, confirm no stray `"on"` value ever appears in the profile sent to `UpdateProfile`.

- [ ] **Step 6: Commit**

```bash
cd /home/zki/projects/vless-client
git add frontend/index.html
git commit -m "feat(ui): move Telegram toggle into the preset grid"
```

---

## Self-Review Notes

- **Spec coverage:** Section 1 (Anthropic preset, Go+frontend) → Tasks 1-2. Section 2 (Telegram visual move) → Task 3. Section 3 (mandatory selector fix) → Task 2, landed *before* Task 3's move so the bug never exists in git history, exactly as the spec's ordering implies is safest.
- **Type consistency:** the `"anthropic"` key string is identical in Task 1's Go literal, Task 2's TS literal, and nowhere else redefined. `data-preset` marker name and selector string (`input[data-preset]`) are introduced once in Task 2 and not touched again.
- **No placeholders:** every step shows literal before/after code or an exact command with expected output.
- **Scope:** single small subsystem (whitelist routing presets UI), no decomposition needed.
