# Routing Settings UX (Contextual Visibility) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Settings routing controls intuitive by hiding controls that are inert in the current mode, relabeling headings/options in plain Russian, and adding inline hints.

**Architecture:** Frontend-only presentation change. The `render(st)` function in `main.ts` already drives all Settings UI from state; we extend it to compute hint text and contextual visibility. Two pure helpers (`connectionHint`, `routingHint`) return the hint strings. No backend or routing-behavior change.

**Tech Stack:** Vanilla TypeScript, Vite. No JS test runner exists (`frontend/package.json` has only dev/build/preview), so verification is `npm run build` (includes `tsc` typecheck) plus a manual QA matrix.

**Spec:** `docs/superpowers/specs/2026-06-15-routing-settings-ux-design.md`

---

## File Structure

- `frontend/src/style.css` — MODIFY: add `.hint` class; add scoped `.hidden` rules for `#full-only` and `#kill-row` (the codebase scopes `.hidden` per element — see lines 102-104).
- `frontend/index.html` — MODIFY: rename the «Режим»/«Маршрутизация» headings, reword `<option>` labels, wrap the RU toggle in `#full-only`, wrap the kill toggle + its hint in `#kill-row`, add hint placeholders (`#connection-hint`, `#routing-hint`) and static toggle hints.
- `frontend/src/main.ts` — MODIFY: add `connectionHint`/`routingHint` helpers; in `render`, set the two hint texts, toggle `#full-only` and `#kill-row` visibility, and remove the old kill-switch `disabled`/`title` lines.

Order: CSS → HTML → TS. The build only fully validates after TS ids match HTML, so the build check lands at the end of Task 3.

---

## Task 1: CSS — hint class + scoped hidden rules

**Files:**
- Modify: `frontend/src/style.css`

- [ ] **Step 1: Add the styles**

The file already defines per-element `.hidden` rules at lines 102-104
(`.update-banner.hidden`, `#autostart-row.hidden`, `#whitelist-only.hidden`) and a
`--muted` color var (line 6). Append these rules at the end of the file:

```css
.hint {
  display: block;
  font-size: 0.78rem;
  color: var(--muted);
  margin: 0.15rem 0 0.5rem 0.1rem;
}
#full-only.hidden { display: none; }
#kill-row.hidden { display: none; }
```

- [ ] **Step 2: Verify CSS is well-formed (build still succeeds)**

Run: `cd frontend && npm run build`
Expected: build succeeds (CSS is bundled by Vite; the new selectors reference ids added in Task 2 but unused selectors are harmless).

- [ ] **Step 3: Commit**

```bash
git add frontend/src/style.css
git commit -m "style(settings): hint class + hidden rules for routing UX"
```

---

## Task 2: HTML — restructure the settings markup

**Files:**
- Modify: `frontend/index.html`

- [ ] **Step 1: Replace the connection + routing markup**

The current block (index.html:52-74) is:

```html
        <h2>Режим</h2>
        <div class="row">
          <select id="mode-select">
            <option value="proxy">Proxy (системный SOCKS)</option>
            <option value="tun">TUN (нужны права администратора)</option>
          </select>
        </div>

        <h2>Маршрутизация</h2>
        <label class="row">
          <span class="label">Режим</span>
          <select id="routing-mode-select">
            <option value="whitelist">Whitelist (только выбранное)</option>
            <option value="full">Всё через VPN</option>
          </select>
        </label>
        <div id="whitelist-only">
          <label><input type="checkbox" id="tg-toggle" /> Telegram → VPN</label>
        </div>
        <label><input type="checkbox" id="ru-toggle" /> RU напрямую</label>
        <label><input type="checkbox" id="kill-toggle" /> Kill switch (TUN)</label>
        <label><input type="checkbox" id="mux-toggle" /> Mux (для Telegram — на сервере клиент без flow)</label>
```

Replace it verbatim with:

```html
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
        </div>
        <div id="full-only">
          <label><input type="checkbox" id="ru-toggle" /> RU напрямую</label>
          <span class="hint">Российские сайты (geoip:ru) — мимо VPN. Быстрее и без блокировок зарубежных сервисов.</span>
        </div>
        <div id="kill-row">
          <label><input type="checkbox" id="kill-toggle" /> Kill switch</label>
          <span class="hint">Если VPN отвалится — блокировать трафик выбранных адресов, чтобы он не утёк напрямую.</span>
        </div>
        <label><input type="checkbox" id="mux-toggle" /> Mux (для Telegram — на сервере клиент без flow)</label>
```

Note: the ids `mode-select`, `routing-mode-select`, `tg-toggle`, `ru-toggle`,
`kill-toggle`, `mux-toggle`, `whitelist-only` are preserved (consumed by `main.ts`).
New ids: `connection-hint`, `routing-hint`, `full-only`, `kill-row`.

- [ ] **Step 2: Verify build still succeeds**

Run: `cd frontend && npm run build`
Expected: build succeeds. (The TS still references the preserved ids; the new ids are wired in Task 3.)

- [ ] **Step 3: Commit**

```bash
git add frontend/index.html
git commit -m "feat(settings): restructure routing markup + plain-language labels"
```

---

## Task 3: TS — contextual visibility + hint wiring

**Files:**
- Modify: `frontend/src/main.ts`

- [ ] **Step 1: Add the two pure hint helpers**

Add these functions immediately above `function render(st: State) {` (currently line 85):

```ts
function connectionHint(mode: string): string {
  return mode === "tun"
    ? "Перехватывает трафик сам, работает со всеми приложениями."
    : "Трафик идёт через системный SOCKS. Telegram его игнорирует — для него выберите TUN.";
}

function routingHint(full: boolean): string {
  return full
    ? "Весь трафик через туннель. Российские сайты можно оставить напрямую."
    : "В VPN идёт только Telegram. Остальное — напрямую.";
}
```

- [ ] **Step 2: Update the render block**

In `render`, replace the current lines 154-174:

```ts
  (<HTMLInputElement>$("tg-toggle")).checked = st.profile.telegram;
  (<HTMLInputElement>$("ru-toggle")).checked = st.profile.forceRUDirect;
  const routingSel = <HTMLSelectElement>$("routing-mode-select");
  routingSel.value = st.profile.full ? "full" : "whitelist";
  $("whitelist-only").classList.toggle("hidden", st.profile.full);
  (<HTMLSelectElement>$("mode-select")).value = st.settings.mode;
  const modeSel = <HTMLSelectElement>$("mode-select");
  const tunOpt = modeSel.querySelector<HTMLOptionElement>('option[value="tun"]');
  if (tunOpt) {
    tunOpt.disabled = !st.caps.tunSupported;
    tunOpt.hidden = !st.caps.tunSupported;
  }
  if (!st.caps.tunSupported && modeSel.value === "tun") {
    modeSel.value = "proxy";
  }
  $("app-version").textContent = st.caps.version;
  const killToggle = <HTMLInputElement>$("kill-toggle");
  killToggle.checked = st.settings.killSwitch;
  killToggle.disabled = !st.caps.killSwitchSupported;
  killToggle.title = st.caps.killSwitchSupported ? "" : "Kill switch не поддерживается на этой ОС";
  (<HTMLInputElement>$("mux-toggle")).checked = st.settings.mux;
```

with:

```ts
  (<HTMLInputElement>$("tg-toggle")).checked = st.profile.telegram;
  (<HTMLInputElement>$("ru-toggle")).checked = st.profile.forceRUDirect;
  const routingSel = <HTMLSelectElement>$("routing-mode-select");
  routingSel.value = st.profile.full ? "full" : "whitelist";
  $("whitelist-only").classList.toggle("hidden", st.profile.full);
  $("full-only").classList.toggle("hidden", !st.profile.full);
  $("routing-hint").textContent = routingHint(st.profile.full);

  (<HTMLSelectElement>$("mode-select")).value = st.settings.mode;
  const modeSel = <HTMLSelectElement>$("mode-select");
  const tunOpt = modeSel.querySelector<HTMLOptionElement>('option[value="tun"]');
  if (tunOpt) {
    tunOpt.disabled = !st.caps.tunSupported;
    tunOpt.hidden = !st.caps.tunSupported;
  }
  if (!st.caps.tunSupported && modeSel.value === "tun") {
    modeSel.value = "proxy";
  }
  $("connection-hint").textContent = connectionHint(modeSel.value);
  $("app-version").textContent = st.caps.version;

  const killToggle = <HTMLInputElement>$("kill-toggle");
  killToggle.checked = st.settings.killSwitch;
  // Kill switch only has an effect in TUN + whitelist on a supporting OS
  // (tunnel.go gates on KillSwitch && !Full; firewall is Linux-only). Hide it
  // everywhere else so no inert control is shown.
  const killVisible =
    modeSel.value === "tun" && st.caps.killSwitchSupported && !st.profile.full;
  $("kill-row").classList.toggle("hidden", !killVisible);

  (<HTMLInputElement>$("mux-toggle")).checked = st.settings.mux;
```

- [ ] **Step 3: Build (typecheck + bundle)**

Run: `cd frontend && npm run build`
Expected: `tsc` passes (no unused-var or type errors; `routingSel` is still used to set `.value`), Vite build succeeds.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/main.ts
git commit -m "feat(settings): contextual routing controls + inline hints"
```

---

## Final verification

- [ ] **Build**

Run: `cd frontend && npm run build`
Expected: success.

- [ ] **Manual QA matrix** (run the app; toggle the two dropdowns and observe visibility):

| State | Visible | Hidden |
|---|---|---|
| whitelist + proxy | Telegram → VPN, Mux | RU напрямую, Kill switch |
| whitelist + tun (Linux, kill supported) | Telegram → VPN, Kill switch, Mux | RU напрямую |
| whitelist + tun (Windows, kill unsupported) | Telegram → VPN, Mux | RU напрямую, Kill switch |
| full + proxy | RU напрямую, Mux | Telegram → VPN, Kill switch |
| full + tun | RU напрямую, Mux | Telegram → VPN, Kill switch |

Also verify:
- Connection hint text changes between Proxy and TUN selections.
- Routing hint text changes between «Только выбранное» and «Всё через VPN».
- Set «RU напрямую» in full mode, switch to whitelist (RU control hides) and back to full — the checkbox is still checked (value preserved).

---

## Notes for the implementer

- No JS test runner is configured; do not add one. `npm run build` runs `tsc`
  (typecheck) then Vite, which is the automated gate. The behavioral value is in
  the manual QA matrix above.
- The change is presentation-only. `forceRUDirect` and `killSwitch` remain in the
  profile/settings and persist when hidden; backend gating
  (`internal/tunnel/tunnel.go`, `internal/routing/profile.go`) is unchanged, so a
  stored-but-hidden value stays a harmless no-op as it is today.
- Out of scope (do not build): a custom-list editor for
  `customProxyDomains`/`customProxyIPs` (no UI today), and a full-tunnel kill
  switch (backend-gated to whitelist only).
