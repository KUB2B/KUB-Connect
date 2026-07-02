# UI/UX Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the 9 UI/UX fixes from `docs/superpowers/specs/2026-07-02-ui-ux-fixes-design.md` — power-button error state, collapsible settings, toggle-switch presets, toast-style errors, onboarding hint, Mux tooltip, ping-latency coloring, modal danger button restyle, and a simple/advanced settings mode.

**Architecture:** Pure frontend change — `frontend/index.html`, `frontend/src/style.css`, `frontend/src/main.ts`. No Go/backend changes, no new dependencies, no new files. Existing element IDs are preserved throughout so `main.ts`'s `$(id)` lookups keep working; markup is restructured in place (wrapped in `<details>`) rather than rewritten from scratch.

**Tech Stack:** Vanilla TypeScript + Vite + Wails (existing stack, unchanged). No new libraries.

## Global Constraints

- UI copy is Russian, matching existing tone (short, direct, no jargon left unexplained — see spec item 6).
- Reuse existing CSS custom properties (`--bg`, `--surface`, `--surface-2`, `--text`, `--muted`, `--border`, `--accent`, `--accent-dim`, `--warn`, `--error`) from `frontend/src/style.css:1-15`. Do not introduce new named colors; derive shades from `--error`'s RGB triplet `248,113,113` via `rgba()` where a tinted variant is needed.
- No frontend test framework exists (`frontend/package.json` has no jest/vitest/playwright). Verification per task is:
  1. `cd frontend && npx tsc --noEmit` — must produce no output.
  2. `cd frontend && npm run build` — must exit 0.
  3. A manual check via `wails dev -tags "wails webkit2_41"` (run from repo root) — this is a Wails desktop app; a plain browser dev server lacks the `window.go` runtime bridge that `main.ts` calls into (`GetState`, `Connect`, etc.), so manual verification must go through the real Wails runtime, not `vite dev` alone.
- Element IDs referenced by `frontend/src/main.ts` (`$("...")` calls) must not change — only their DOM ancestry/wrapping and styling change.
- DRY: the toggle-switch checkbox style introduced in Task 3 is a reusable `.toggle-switch` class, reused by Task 9 rather than duplicated.

---

### Task 1: Power-button error-state icon and color

**Files:**
- Modify: `frontend/index.html:31-36`
- Modify: `frontend/src/style.css:40-41`

**Interfaces:**
- Consumes: existing `render()` in `main.ts:130-131` which already sets `btn.className = "power " + st.conn` (values: `connected`, `connecting`, `disconnecting`, `disconnected`, `error`) — no JS changes needed, this task is HTML/CSS only.
- Produces: nothing consumed by later tasks.

- [ ] **Step 1: Add a second (alert) glyph to the power button markup**

Find in `frontend/index.html`:
```html
          <button id="power-btn" class="power disconnected" title="Подключить/отключить">
            <svg class="power-glyph" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <path d="M18.36 6.64a9 9 0 1 1-12.73 0"></path>
              <line x1="12" y1="2" x2="12" y2="12"></line>
            </svg>
          </button>
```

Replace with:
```html
          <button id="power-btn" class="power disconnected" title="Подключить/отключить">
            <svg class="power-glyph glyph-power" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <path d="M18.36 6.64a9 9 0 1 1-12.73 0"></path>
              <line x1="12" y1="2" x2="12" y2="12"></line>
            </svg>
            <svg class="power-glyph glyph-alert" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <circle cx="12" cy="12" r="10"></circle>
              <line x1="12" y1="8" x2="12" y2="12"></line>
              <line x1="12" y1="16" x2="12.01" y2="16"></line>
            </svg>
          </button>
```

- [ ] **Step 2: Split the disconnected/error CSS selector and add glyph-swap rules**

Find in `frontend/src/style.css`:
```css
.power.disconnected, .power.error { background: var(--muted); box-shadow: 0 8px 24px rgba(0,0,0,.4); }
```

Replace with:
```css
.power.disconnected { background: var(--muted); box-shadow: 0 8px 24px rgba(0,0,0,.4); }
.power.error { background: rgba(248, 113, 113, 0.28); box-shadow: 0 8px 24px rgba(248,113,113,.35); }
.glyph-alert { display: none; }
.power.error .glyph-power { display: none; }
.power.error .glyph-alert { display: block; }
```

- [ ] **Step 3: Verify typecheck and build**

Run: `cd frontend && npx tsc --noEmit`
Expected: no output, exit 0.

Run: `cd frontend && npm run build`
Expected: exit 0, `frontend/dist/` produced.

- [ ] **Step 4: Manual verification**

Run: `wails dev -tags "wails webkit2_41"` from repo root.
In the app window's devtools console (if available) or by temporarily setting `current.conn = "error"` and calling `render(current)` is not exposed — instead, trigger a real error path (e.g. pick a server with an unreachable host and Connect, or disconnect the network) to reach the `error` conn state, and confirm: button turns the reddish tint, the "!" glyph replaces the power glyph. Revert to disconnected and confirm the power glyph returns.

- [ ] **Step 5: Commit**

```bash
git add frontend/index.html frontend/src/style.css
git commit -m "fix(ui): distinguish power-button error state from disconnected"
```

---

### Task 2: Collapsible settings sections via `<details>`/`<summary>`

**Files:**
- Modify: `frontend/index.html:47-116` (the `#tab-settings` section)
- Modify: `frontend/src/style.css:50` (the `h2` rule) and its surrounding area

**Interfaces:**
- Consumes: nothing new.
- Produces: five `<details>` elements with ids `section-servers`, `section-connection`, `section-routing`, `section-startup`, `section-logs` — Task 9 attaches `data-advanced` to `section-routing` and `section-logs` and toggles their visibility.

- [ ] **Step 1: Replace the `<h2>`-delimited flat layout with `<details>` sections**

Find in `frontend/index.html` (the entire `#tab-settings` section body):
```html
        <h2>Серверы</h2>
        <div class="row">
          <input id="link-input" type="text" placeholder="vless://..." />
          <button id="add-server-btn">Добавить</button>
        </div>
        <p id="link-error" class="error"></p>
        <ul id="server-list"></ul>

        <h2>Подключение</h2>
        <div class="row">
          <select id="mode-select">
            <option value="proxy">Proxy — без прав администратора</option>
            <option value="tun">TUN — нужны права администратора</option>
          </select>
        </div>
        <p id="connection-hint" class="hint"></p>

        <h2>Что через VPN</h2>
        <label class="row">
          <span class="label">Режим</span>
          <select id="routing-mode-select">
            <option value="whitelist">Только выбранное</option>
            <option value="full">Всё через VPN</option>
          </select>
        </label>
        <p id="routing-hint" class="hint"></p>
        <div id="whitelist-only">
          <label><input type="checkbox" id="tg-toggle" /> Telegram → VPN</label>
          <span class="hint">Заворачивать Telegram в туннель.</span>
          <div id="preset-list" class="preset-list"></div>
          <span class="hint">Отмеченные сервисы идут через VPN. Если сервер пропускает только Telegram — оставьте всё выключенным.</span>
          <label class="col">
            <span class="label">Свои адреса через VPN</span>
            <textarea id="proxy-list" rows="4" spellcheck="false"
              placeholder="youtube.com&#10;geosite:google&#10;1.2.3.4&#10;10.20.0.0/16"></textarea>
          </label>
          <span class="hint">По одной записи в строке: домен, geosite:категория, IP или CIDR.</span>
        </div>
        <div id="full-only">
          <label><input type="checkbox" id="ru-toggle" /> RU напрямую</label>
          <span class="hint">Российские сайты (geoip:ru) — мимо VPN. Быстрее и без блокировок зарубежных сервисов.</span>
        </div>
        <label class="col">
          <span class="label">Исключения — всегда мимо VPN</span>
          <textarea id="direct-list" rows="3" spellcheck="false"
            placeholder="sberbank.ru&#10;gosuslugi.ru&#10;77.88.8.8"></textarea>
        </label>
        <span class="hint">Домены и IP, которые никогда не заворачиваются в туннель (в обоих режимах).</span>
        <p id="routing-error" class="error"></p>
        <div id="kill-row">
          <label><input type="checkbox" id="kill-toggle" /> Kill switch</label>
          <span class="hint">Если VPN отвалится — блокировать трафик выбранных адресов, чтобы он не утёк напрямую.</span>
        </div>
        <label><input type="checkbox" id="mux-toggle" /> Mux (для Telegram — на сервере клиент без flow)</label>

        <h2>Запуск</h2>
        <label id="autostart-row"><input type="checkbox" id="autostart-toggle" /> Автозапуск при входе в систему</label>
        <label><input type="checkbox" id="autoconnect-toggle" /> Автоподключение при старте</label>

        <h2>Логи</h2>
        <label class="row">
          <span class="label">Уровень</span>
          <select id="loglevel-select">
            <option value="error">Тихо</option>
            <option value="warning">Обычный</option>
            <option value="debug">Подробно</option>
          </select>
        </label>
```

Replace with:
```html
        <details id="section-servers" open>
          <summary>Серверы</summary>
          <div class="row">
            <input id="link-input" type="text" placeholder="vless://..." />
            <button id="add-server-btn">Добавить</button>
          </div>
          <p id="link-error" class="error"></p>
          <ul id="server-list"></ul>
        </details>

        <details id="section-connection" open>
          <summary>Подключение</summary>
          <div class="row">
            <select id="mode-select">
              <option value="proxy">Proxy — без прав администратора</option>
              <option value="tun">TUN — нужны права администратора</option>
            </select>
          </div>
          <p id="connection-hint" class="hint"></p>
        </details>

        <details id="section-routing">
          <summary>Что через VPN</summary>
          <label class="row">
            <span class="label">Режим</span>
            <select id="routing-mode-select">
              <option value="whitelist">Только выбранное</option>
              <option value="full">Всё через VPN</option>
            </select>
          </label>
          <p id="routing-hint" class="hint"></p>
          <div id="whitelist-only">
            <label><input type="checkbox" id="tg-toggle" /> Telegram → VPN</label>
            <span class="hint">Заворачивать Telegram в туннель.</span>
            <div id="preset-list" class="preset-list"></div>
            <span class="hint">Отмеченные сервисы идут через VPN. Если сервер пропускает только Telegram — оставьте всё выключенным.</span>
            <label class="col">
              <span class="label">Свои адреса через VPN</span>
              <textarea id="proxy-list" rows="4" spellcheck="false"
                placeholder="youtube.com&#10;geosite:google&#10;1.2.3.4&#10;10.20.0.0/16"></textarea>
            </label>
            <span class="hint">По одной записи в строке: домен, geosite:категория, IP или CIDR.</span>
          </div>
          <div id="full-only">
            <label><input type="checkbox" id="ru-toggle" /> RU напрямую</label>
            <span class="hint">Российские сайты (geoip:ru) — мимо VPN. Быстрее и без блокировок зарубежных сервисов.</span>
          </div>
          <label class="col">
            <span class="label">Исключения — всегда мимо VPN</span>
            <textarea id="direct-list" rows="3" spellcheck="false"
              placeholder="sberbank.ru&#10;gosuslugi.ru&#10;77.88.8.8"></textarea>
          </label>
          <span class="hint">Домены и IP, которые никогда не заворачиваются в туннель (в обоих режимах).</span>
          <p id="routing-error" class="error"></p>
          <div id="kill-row">
            <label><input type="checkbox" id="kill-toggle" /> Kill switch</label>
            <span class="hint">Если VPN отвалится — блокировать трафик выбранных адресов, чтобы он не утёк напрямую.</span>
          </div>
          <label><input type="checkbox" id="mux-toggle" /> Mux (для Telegram — на сервере клиент без flow)</label>
        </details>

        <details id="section-startup" open>
          <summary>Запуск</summary>
          <label id="autostart-row"><input type="checkbox" id="autostart-toggle" /> Автозапуск при входе в систему</label>
          <label><input type="checkbox" id="autoconnect-toggle" /> Автоподключение при старте</label>
        </details>

        <details id="section-logs">
          <summary>Логи</summary>
          <label class="row">
            <span class="label">Уровень</span>
            <select id="loglevel-select">
              <option value="error">Тихо</option>
              <option value="warning">Обычный</option>
              <option value="debug">Подробно</option>
            </select>
          </label>
        </details>
```

- [ ] **Step 2: Restyle section headings — replace the now-unused `h2` rule with a `summary` rule**

Find in `frontend/src/style.css`:
```css
h2 { font-size: 0.95rem; margin: 1.4rem 0 0.5rem; color: var(--muted); text-transform: uppercase; letter-spacing: 0.04em; }
```

Replace with:
```css
details { margin: 1.4rem 0 0; }
details > summary {
  font-size: 0.95rem; color: var(--muted); text-transform: uppercase; letter-spacing: 0.04em;
  cursor: pointer; padding: 0.3rem 0;
}
details[open] > summary { margin-bottom: 0.3rem; }
```

- [ ] **Step 3: Verify typecheck and build**

Run: `cd frontend && npx tsc --noEmit`
Expected: no output, exit 0.

Run: `cd frontend && npm run build`
Expected: exit 0.

Run: `grep -c 'id="' frontend/index.html`
Expected: same count as before this task's edit (sanity check that no `id` was dropped during the rewrite) — cross-check by running `grep -oP 'id="\K[^"]+' frontend/index.html | sort > /tmp/ids-after.txt` and manually confirming every id previously listed (`link-input`, `add-server-btn`, `link-error`, `server-list`, `mode-select`, `connection-hint`, `routing-mode-select`, `routing-hint`, `whitelist-only`, `tg-toggle`, `preset-list`, `proxy-list`, `full-only`, `ru-toggle`, `direct-list`, `routing-error`, `kill-row`, `kill-toggle`, `mux-toggle`, `autostart-row`, `autostart-toggle`, `autoconnect-toggle`, `loglevel-select`) is present.

- [ ] **Step 4: Manual verification**

Run: `wails dev -tags "wails webkit2_41"`. Open Настройки tab. Confirm "Серверы", "Подключение", "Запуск" are open by default; "Что через VPN" and "Логи" are collapsed. Click each summary to confirm expand/collapse works and existing controls (add server, toggle mode, etc.) still function.

- [ ] **Step 5: Commit**

```bash
git add frontend/index.html frontend/src/style.css
git commit -m "feat(ui): make settings sections collapsible"
```

---

### Task 3: Reusable toggle-switch style, applied to preset checkboxes

**Files:**
- Modify: `frontend/src/style.css:148` (`.preset-list label` rule)
- Modify: `frontend/src/main.ts:337-350` (`PRESETS.forEach` block in `wire()`)

**Interfaces:**
- Consumes: nothing new.
- Produces: CSS class `.toggle-switch`, reused by Task 9 on the new advanced-mode checkbox.

- [ ] **Step 1: Add the `.toggle-switch` class and preset-label flex layout**

Find in `frontend/src/style.css`:
```css
.preset-list label { font-size: 0.9rem; }
```

Replace with:
```css
.preset-list label { font-size: 0.9rem; display: flex; align-items: center; gap: 0.5rem; cursor: pointer; }
.toggle-switch {
  appearance: none; -webkit-appearance: none;
  width: 34px; height: 20px; border-radius: 10px;
  background: var(--surface-2); border: 1px solid var(--border);
  position: relative; cursor: pointer; flex: none;
  transition: background 0.2s;
}
.toggle-switch::before {
  content: ""; position: absolute; top: 1px; left: 1px;
  width: 16px; height: 16px; border-radius: 50%;
  background: var(--muted); transition: transform 0.2s, background 0.2s;
}
.toggle-switch:checked { background: var(--accent-dim); border-color: var(--accent-dim); }
.toggle-switch:checked::before { transform: translateX(14px); background: #fff; }
```

- [ ] **Step 2: Apply the class to preset checkboxes in `main.ts`**

Find in `frontend/src/main.ts`:
```ts
  PRESETS.forEach((p) => {
    const label = document.createElement("label");
    const cb = document.createElement("input");
    cb.type = "checkbox";
    cb.value = p.key;
```

Replace with:
```ts
  PRESETS.forEach((p) => {
    const label = document.createElement("label");
    const cb = document.createElement("input");
    cb.type = "checkbox";
    cb.className = "toggle-switch";
    cb.value = p.key;
```

- [ ] **Step 3: Verify typecheck and build**

Run: `cd frontend && npx tsc --noEmit`
Expected: no output, exit 0.

Run: `cd frontend && npm run build`
Expected: exit 0.

- [ ] **Step 4: Manual verification**

Run: `wails dev -tags "wails webkit2_41"`. Open Настройки → "Что через VPN" (expand it). Confirm preset checkboxes render as pill toggle-switches, click to toggle, confirm the underlying profile update still fires (check Логи tab or network — an existing behavior, unchanged).

- [ ] **Step 5: Commit**

```bash
git add frontend/src/style.css frontend/src/main.ts
git commit -m "feat(ui): style preset checkboxes as toggle switches"
```

---

### Task 4: Toast-style error display

**Files:**
- Modify: `frontend/src/style.css:69` (`.error` rule)
- Modify: `frontend/src/main.ts` (add `setError` helper, replace all direct `.textContent =` error assignments, add click-to-dismiss listener)

**Interfaces:**
- Consumes: existing `.error` elements (`#error-line`, `#link-error`, `#routing-error`).
- Produces: `function setError(el: HTMLElement, text: string): void` — module-level function in `main.ts`, used by all error-setting call sites (this task replaces every one of them; no call sites are left using raw `.textContent =` for these three elements).

- [ ] **Step 1: Add the toast CSS**

Find in `frontend/src/style.css`:
```css
.error { color: var(--error); min-height: 1em; font-size: 0.85rem; }
```

Replace with:
```css
.error {
  color: var(--error); min-height: 1em; font-size: 0.85rem;
  opacity: 0; transition: opacity 0.15s ease;
}
.error.show {
  opacity: 1;
  background: rgba(248, 113, 113, .08);
  border: 1px solid rgba(248, 113, 113, .3);
  border-radius: 6px;
  padding: 0.35rem 0.5rem;
  cursor: pointer;
}
```

- [ ] **Step 2: Add the `setError` helper to `main.ts`**

Find in `frontend/src/main.ts`:
```ts
const $ = (id: string) => document.getElementById(id)!;
```

Replace with:
```ts
const $ = (id: string) => document.getElementById(id)!;

// Error toast: shows text with the `.show` class (visible, boxed), auto-hides
// after 5s or on click. Text stays in the DOM (for screen readers / debugging)
// even while hidden; only the `.show` class controls visibility.
const errorTimers = new WeakMap<HTMLElement, number>();
function setError(el: HTMLElement, text: string) {
  const existing = errorTimers.get(el);
  if (existing) window.clearTimeout(existing);
  el.textContent = text;
  el.classList.toggle("show", !!text);
  if (text) {
    const timer = window.setTimeout(() => el.classList.remove("show"), 5000);
    errorTimers.set(el, timer);
  }
}
```

- [ ] **Step 3: Add click-to-dismiss for any visible toast**

Find in `frontend/src/main.ts` (inside `wire()`, right after the opening brace):
```ts
function wire() {
  // Tabs
```

Replace with:
```ts
function wire() {
  // Error toasts: click anywhere on a shown `.error` to dismiss early.
  document.addEventListener("click", (e) => {
    const target = e.target as HTMLElement;
    if (target.classList.contains("error") && target.classList.contains("show")) {
      target.classList.remove("show");
    }
  });

  // Tabs
```

- [ ] **Step 4: Replace every direct error-text assignment with `setError`**

Find in `frontend/src/main.ts` (in `render()`):
```ts
  $("error-line").textContent = st.lastError || "";
```
Replace with:
```ts
  setError($("error-line"), st.lastError || "");
```

Find:
```ts
function pushSettings() {
  UpdateSettings(current.settings).catch((e) => ($("error-line").textContent = String(e)));
}
```
Replace with:
```ts
function pushSettings() {
  UpdateSettings(current.settings).catch((e) => setError($("error-line"), String(e)));
}
```

Find:
```ts
    if (c === "connected") {
      Disconnect().catch((e) => ($("error-line").textContent = String(e)));
    } else if (c === "disconnected" || c === "error") {
```
Replace with:
```ts
    if (c === "connected") {
      Disconnect().catch((e) => setError($("error-line"), String(e)));
    } else if (c === "disconnected" || c === "error") {
```

Find:
```ts
      Connect().catch((e) => ($("error-line").textContent = String(e)));
```
Replace with:
```ts
      Connect().catch((e) => setError($("error-line"), String(e)));
```

Find:
```ts
    AddServer(input.value)
      .then(() => {
        input.value = "";
        $("link-error").textContent = "";
      })
      .catch((e) => ($("link-error").textContent = String(e)))
```
Replace with:
```ts
    AddServer(input.value)
      .then(() => {
        input.value = "";
        setError($("link-error"), "");
      })
      .catch((e) => setError($("link-error"), String(e)))
```

Find:
```ts
  const pushProfile = () => {
    UpdateProfile(current.profile)
      .then(() => ($("routing-error").textContent = ""))
      .catch((e) => ($("routing-error").textContent = String(e)));
  };
```
Replace with:
```ts
  const pushProfile = () => {
    UpdateProfile(current.profile)
      .then(() => setError($("routing-error"), ""))
      .catch((e) => setError($("routing-error"), String(e)));
  };
```

Find:
```ts
    UpdateProfile(current.profile).catch((e) => {
      current.profile.full = prev;
      (<HTMLSelectElement>$("routing-mode-select")).value = prev ? "full" : "whitelist";
      $("whitelist-only").classList.toggle("hidden", prev);
      $("error-line").textContent = String(e);
    });
```
Replace with:
```ts
    UpdateProfile(current.profile).catch((e) => {
      current.profile.full = prev;
      (<HTMLSelectElement>$("routing-mode-select")).value = prev ? "full" : "whitelist";
      $("whitelist-only").classList.toggle("hidden", prev);
      setError($("error-line"), String(e));
    });
```

Find:
```ts
    UpdateSettings(current.settings).catch((e) => {
      current.settings.autoStart = prev;
      cb.checked = prev;
      $("error-line").textContent = String(e);
    });
  });
  $("autoconnect-toggle").addEventListener("change", () => {
    const cb = <HTMLInputElement>$("autoconnect-toggle");
    const prev = current.settings.autoConnect;
    current.settings.autoConnect = cb.checked;
    UpdateSettings(current.settings).catch((e) => {
      current.settings.autoConnect = prev;
      cb.checked = prev;
      $("error-line").textContent = String(e);
    });
  });
```
Replace with:
```ts
    UpdateSettings(current.settings).catch((e) => {
      current.settings.autoStart = prev;
      cb.checked = prev;
      setError($("error-line"), String(e));
    });
  });
  $("autoconnect-toggle").addEventListener("change", () => {
    const cb = <HTMLInputElement>$("autoconnect-toggle");
    const prev = current.settings.autoConnect;
    current.settings.autoConnect = cb.checked;
    UpdateSettings(current.settings).catch((e) => {
      current.settings.autoConnect = prev;
      cb.checked = prev;
      setError($("error-line"), String(e));
    });
  });
```

Find:
```ts
  $("elevate-restart").addEventListener("click", () => {
    RelaunchElevated(elevateForConnect).catch((e) => {
      closeElevate(!elevateForConnect);
      $("error-line").textContent = String(e);
    });
  });
```
Replace with:
```ts
  $("elevate-restart").addEventListener("click", () => {
    RelaunchElevated(elevateForConnect).catch((e) => {
      closeElevate(!elevateForConnect);
      setError($("error-line"), String(e));
    });
  });
```

Find:
```ts
      DownloadAndInstall().catch((err) => {
        // UAC declined / network / no asset — restore the banner with a note.
        $("update-progress-wrap").classList.add("hidden");
        $("update-text").classList.remove("hidden");
        $("error-line").textContent = "Обновление не удалось: " + String(err);
      });
```
Replace with:
```ts
      DownloadAndInstall().catch((err) => {
        // UAC declined / network / no asset — restore the banner with a note.
        $("update-progress-wrap").classList.add("hidden");
        $("update-text").classList.remove("hidden");
        setError($("error-line"), "Обновление не удалось: " + String(err));
      });
```

- [ ] **Step 5: Confirm no direct assignments remain**

Run: `grep -n '"error-line"\|"link-error"\|"routing-error"' frontend/src/main.ts | grep '\.textContent ='`
Expected: no output (every remaining reference goes through `setError`).

- [ ] **Step 6: Verify typecheck and build**

Run: `cd frontend && npx tsc --noEmit`
Expected: no output, exit 0.

Run: `cd frontend && npm run build`
Expected: exit 0.

- [ ] **Step 7: Manual verification**

Run: `wails dev -tags "wails webkit2_41"`. On Настройки, paste an invalid string (e.g. `not-a-link`) into the server-link input and click Добавить. Confirm the error appears as a boxed/bordered toast (not plain text), and that it fades out after 5 seconds, or immediately on click.

- [ ] **Step 8: Commit**

```bash
git add frontend/src/style.css frontend/src/main.ts
git commit -m "feat(ui): show errors as auto-dismissing toasts"
```

---

### Task 5: Onboarding hint when no servers are configured

**Files:**
- Modify: `frontend/src/style.css` (add `.onboard-hint`)
- Modify: `frontend/src/main.ts` (add onboarding check, call from `render()`)

**Interfaces:**
- Consumes: `State.servers` (existing field, `main.ts:69`), `setTab` (existing function, `main.ts:99-106`).
- Produces: nothing consumed by later tasks.

- [ ] **Step 1: Add the highlight CSS**

Find in `frontend/src/style.css`:
```css
input[type="text"] { flex: 1; }
```

Replace with:
```css
input[type="text"] { flex: 1; }
input.onboard-hint { outline: 2px solid var(--accent); outline-offset: 1px; }
```

- [ ] **Step 2: Add the onboarding function and a module-level flag**

Find in `frontend/src/main.ts`:
```ts
let current: State;
```

Replace with:
```ts
let current: State;

// Shown once per app session the first time we see an empty server list —
// steers a fresh install straight to where a link needs to be pasted.
let onboardingShown = false;
function maybeShowOnboarding(st: State) {
  if (st.servers.length !== 0 || onboardingShown) return;
  onboardingShown = true;
  setTab("settings");
  const input = <HTMLInputElement>$("link-input");
  input.focus();
  input.classList.add("onboard-hint");
  input.addEventListener("input", () => input.classList.remove("onboard-hint"), { once: true });
}
```

- [ ] **Step 3: Call it from `render()`**

Find in `frontend/src/main.ts`:
```ts
function render(st: State) {
  // Power button: color from conn state.
```

Replace with:
```ts
function render(st: State) {
  maybeShowOnboarding(st);

  // Power button: color from conn state.
```

- [ ] **Step 4: Verify typecheck and build**

Run: `cd frontend && npx tsc --noEmit`
Expected: no output, exit 0.

Run: `cd frontend && npm run build`
Expected: exit 0.

- [ ] **Step 5: Manual verification**

Run: `wails dev -tags "wails webkit2_41"` with a config that has zero servers (remove all servers via the UI first, or point `--` at a fresh config path if the app supports one). Confirm: app opens on/switches to Настройки, `link-input` is focused and has a visible accent outline, and the outline disappears as soon as you start typing. Confirm re-adding then removing a server later in the same session does NOT re-trigger the auto-switch (once-per-session flag).

- [ ] **Step 6: Commit**

```bash
git add frontend/src/style.css frontend/src/main.ts
git commit -m "feat(ui): auto-open settings with a highlighted input when no servers exist"
```

---

### Task 6: Mux tooltip

**Files:**
- Modify: `frontend/index.html` (the Mux `<label>`, now inside `#section-routing` from Task 2)

**Interfaces:** none.

- [ ] **Step 1: Add a `title` attribute with a plain-language explanation**

Find in `frontend/index.html`:
```html
          <label><input type="checkbox" id="mux-toggle" /> Mux (для Telegram — на сервере клиент без flow)</label>
```

Replace with:
```html
          <label title="Ускоряет соединение для Telegram — включайте, если сервер настроен без flow-контроля"><input type="checkbox" id="mux-toggle" /> Mux (для Telegram — на сервере клиент без flow)</label>
```

- [ ] **Step 2: Verify typecheck and build**

Run: `cd frontend && npx tsc --noEmit`
Expected: no output, exit 0.

Run: `cd frontend && npm run build`
Expected: exit 0.

- [ ] **Step 3: Manual verification**

Run: `wails dev -tags "wails webkit2_41"`. Open Настройки → "Что через VPN", hover the Mux label, confirm the native tooltip shows the explanation text.

- [ ] **Step 4: Commit**

```bash
git add frontend/index.html
git commit -m "docs(ui): add explanatory tooltip to Mux toggle"
```

---

### Task 7: Ping-latency color coding

**Files:**
- Modify: `frontend/src/style.css:67` (`.ping-result` rule)
- Modify: `frontend/src/main.ts:159-195` (server-list rendering + ping click handler)

**Interfaces:**
- Consumes: `Ping(index): Promise<app.PingResultDTO>` (existing, `frontend/wailsjs/go/main/App.d.ts:22`), fields `ok: boolean`, `latencyMs: number`, `error: string`.
- Produces: nothing consumed by later tasks.

- [ ] **Step 1: Replace the flat `.ping-result` style with color variants**

Find in `frontend/src/style.css`:
```css
.ping-result { flex: none; margin-left: 0.5rem; opacity: 0.8; font-size: 0.85em; color: var(--muted); font-weight: normal; }
```

Replace with:
```css
.ping-result { flex: none; margin-left: 0.5rem; font-size: 0.85em; color: var(--muted); font-weight: normal; }
.ping-result.ping-fast { color: var(--accent); }
.ping-result.ping-mid { color: var(--warn); }
.ping-result.ping-slow { color: var(--error); }
```

- [ ] **Step 2: Track a per-server ping color class alongside the existing ping text**

Find in `frontend/src/main.ts`:
```ts
// Ephemeral ping results, keyed by `${host}:${port}` so they survive re-renders
// and index shifts when a server is removed.
const pingResults: Record<string, string> = {};
```

Replace with:
```ts
// Ephemeral ping results, keyed by `${host}:${port}` so they survive re-renders
// and index shifts when a server is removed.
const pingResults: Record<string, string> = {};
// Color class (ping-fast/ping-mid/ping-slow) for the same key, re-applied on
// every render so a re-render doesn't wipe the latency color.
const pingClasses: Record<string, string> = {};
```

- [ ] **Step 3: Apply the stored class when the result span is (re)created**

Find in `frontend/src/main.ts`:
```ts
    const result = document.createElement("span");
    result.className = "ping-result";
    result.textContent = pingResults[key] ?? "";
```

Replace with:
```ts
    const result = document.createElement("span");
    result.className = "ping-result " + (pingClasses[key] ?? "");
    result.textContent = pingResults[key] ?? "";
```

- [ ] **Step 4: Classify latency into fast/mid/slow when a ping completes**

Find in `frontend/src/main.ts`:
```ts
    ping.onclick = () => {
      pingResults[key] = "…";
      result.textContent = "…";
      const apply = (text: string) => {
        pingResults[key] = text;
        // Resolve into the currently-mounted span, which may differ from
        // `result` if a re-render happened while the Ping was in flight.
        const el = pingEls[key];
        if (el) el.textContent = text;
      };
      Ping(i)
        .then((r) => apply(r.ok ? `${r.latencyMs} мс` : r.error || "ошибка"))
        .catch(() => apply("ошибка"));
    };
```

Replace with:
```ts
    ping.onclick = () => {
      pingResults[key] = "…";
      pingClasses[key] = "";
      result.className = "ping-result";
      result.textContent = "…";
      const apply = (text: string, cls: string) => {
        pingResults[key] = text;
        pingClasses[key] = cls;
        // Resolve into the currently-mounted span, which may differ from
        // `result` if a re-render happened while the Ping was in flight.
        const el = pingEls[key];
        if (el) {
          el.textContent = text;
          el.className = "ping-result " + cls;
        }
      };
      Ping(i)
        .then((r) => {
          if (!r.ok) {
            apply(r.error || "ошибка", "ping-slow");
            return;
          }
          const cls = r.latencyMs < 150 ? "ping-fast" : r.latencyMs <= 400 ? "ping-mid" : "ping-slow";
          apply(`${r.latencyMs} мс`, cls);
        })
        .catch(() => apply("ошибка", "ping-slow"));
    };
```

- [ ] **Step 5: Verify typecheck and build**

Run: `cd frontend && npx tsc --noEmit`
Expected: no output, exit 0.

Run: `cd frontend && npm run build`
Expected: exit 0.

- [ ] **Step 6: Manual verification**

Run: `wails dev -tags "wails webkit2_41"`. Open Настройки → Серверы, click Пинг on a server. Confirm the latency text is colored green (<150ms), yellow (150-400ms), or red (>400ms or error) accordingly. Trigger a re-render (e.g. toggle a setting) and confirm the color persists.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/style.css frontend/src/main.ts
git commit -m "feat(ui): color-code ping latency by speed"
```

---

### Task 8: Modal danger-button restyle

**Files:**
- Modify: `frontend/src/style.css:94` (`.modal-btn.danger` rule)

**Interfaces:** none.

- [ ] **Step 1: Restyle the danger button as a de-emphasized, separated action**

Find in `frontend/src/style.css`:
```css
.modal-btn.danger { color: var(--error); }
```

Replace with:
```css
.modal-btn.danger {
  color: var(--error);
  background: none;
  border: none;
  border-top: 1px solid var(--border);
  border-radius: 0;
  padding-top: 0.7rem;
}
.modal-btn.danger:hover { background: rgba(248, 113, 113, .08); }
```

- [ ] **Step 2: Verify typecheck and build**

Run: `cd frontend && npx tsc --noEmit`
Expected: no output, exit 0.

Run: `cd frontend && npm run build`
Expected: exit 0.

- [ ] **Step 3: Manual verification**

Run: `wails dev -tags "wails webkit2_41"`. Trigger the close-confirmation modal (click the window's OS close button). Confirm "Выйти" now reads as a de-emphasized link-like row separated by a top border, while "Свернуть в трей" remains the clear full-width primary action.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/style.css
git commit -m "fix(ui): de-emphasize destructive Quit action in close modal"
```

---

### Task 9: Simple/Advanced settings mode

**Files:**
- Modify: `frontend/index.html` (add mode toggle control, add `data-advanced` to `#section-routing` and `#section-logs`)
- Modify: `frontend/src/style.css` (add `.simple-mode` visibility rule + toggle row layout)
- Modify: `frontend/src/main.ts` (persist/read `localStorage`, apply the class)

**Interfaces:**
- Consumes: `.toggle-switch` CSS class from Task 3; `#section-routing`/`#section-logs` ids from Task 2.
- Produces: nothing consumed by later tasks (final task).

- [ ] **Step 1: Add `data-advanced` markers and the mode-toggle control**

Find in `frontend/index.html`:
```html
      <section id="tab-settings" class="tab-panel hidden">
        <details id="section-servers" open>
```

Replace with:
```html
      <section id="tab-settings" class="tab-panel hidden">
        <div class="row mode-toggle-row">
          <label class="mode-switch">
            <input type="checkbox" id="advanced-toggle" class="toggle-switch" />
            <span>Расширенный режим</span>
          </label>
        </div>
        <details id="section-servers" open>
```

Find:
```html
        <details id="section-routing">
```
Replace with:
```html
        <details id="section-routing" data-advanced>
```

Find:
```html
        <details id="section-logs">
```
Replace with:
```html
        <details id="section-logs" data-advanced>
```

- [ ] **Step 2: Add CSS for the hide-when-simple rule and the toggle row layout**

Find in `frontend/src/style.css`:
```css
#full-only.hidden { display: none; }
```

Replace with:
```css
#full-only.hidden { display: none; }
#tab-settings.simple-mode details[data-advanced] { display: none; }
.mode-toggle-row { justify-content: flex-end; margin: 0 0 0.5rem; }
.mode-switch { display: flex; align-items: center; gap: 0.5rem; font-size: 0.85rem; color: var(--muted); cursor: pointer; margin: 0; }
```

- [ ] **Step 3: Wire up persistence and the visibility class in `main.ts`**

Find in `frontend/src/main.ts`:
```ts
function wire() {
  // Error toasts: click anywhere on a shown `.error` to dismiss early.
```

Replace with:
```ts
const ADVANCED_MODE_KEY = "ui.advanced";

function applyAdvancedMode(advanced: boolean) {
  $("tab-settings").classList.toggle("simple-mode", !advanced);
}

function wire() {
  // Simple/advanced settings mode — a UI-only preference, not sent to the backend.
  const advancedToggle = <HTMLInputElement>$("advanced-toggle");
  const storedAdvanced = localStorage.getItem(ADVANCED_MODE_KEY) === "1";
  advancedToggle.checked = storedAdvanced;
  applyAdvancedMode(storedAdvanced);
  advancedToggle.addEventListener("change", () => {
    localStorage.setItem(ADVANCED_MODE_KEY, advancedToggle.checked ? "1" : "0");
    applyAdvancedMode(advancedToggle.checked);
  });

  // Error toasts: click anywhere on a shown `.error` to dismiss early.
```

- [ ] **Step 4: Verify typecheck and build**

Run: `cd frontend && npx tsc --noEmit`
Expected: no output, exit 0.

Run: `cd frontend && npm run build`
Expected: exit 0.

- [ ] **Step 5: Manual verification**

Run: `wails dev -tags "wails webkit2_41"`. On first run (empty `localStorage`), confirm Настройки shows only Серверы/Подключение/Запуск ("Что через VPN" and "Логи" hidden) and the mode toggle is off. Turn on "Расширенный режим", confirm the two sections appear (even though collapsed, per Task 2 defaults). Reload the app (or restart `wails dev`) and confirm the advanced state persisted via `localStorage`.

- [ ] **Step 6: Commit**

```bash
git add frontend/index.html frontend/src/style.css frontend/src/main.ts
git commit -m "feat(ui): add simple/advanced settings mode toggle"
```

---

## Self-Review Notes

- **Spec coverage:** all 9 spec items map 1:1 to Task 1–9. The two "open questions" in the spec (error-icon markup approach; onboarding not annoying users who deliberately empty their server list) are resolved: markup-based glyph swap (Task 1, no XSS surface, CSS-driven) and a once-per-session flag (Task 5).
- **Type consistency:** `setError(el: HTMLElement, text: string)` (Task 4) is used identically at every call site. `applyAdvancedMode(advanced: boolean)` (Task 9) and `maybeShowOnboarding(st: State)` (Task 5) each have one producer and are each called from exactly one place — no signature drift across tasks.
- **DRY:** `.toggle-switch` is defined once (Task 3) and reused as-is (Task 9), not redefined.
- **No placeholders:** every step shows the literal before/after code; no "add appropriate styling" or "similar to Task N" steps.
