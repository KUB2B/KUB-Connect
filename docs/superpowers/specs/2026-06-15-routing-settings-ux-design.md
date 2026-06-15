# Routing Settings UX — Contextual Visibility + Explanations

**Date:** 2026-06-15
**Status:** Approved, pending implementation plan
**Scope:** Frontend only (`frontend/index.html`, `frontend/src/main.ts`, `frontend/src/style.css`). No backend/behavior change.

## Problem

The Settings → routing controls confuse users because several are inert in the
current mode, yet always visible with no explanation:

- **«RU напрямую»** (`forceRUDirect`) is a **no-op in whitelist mode**. Whitelist
  rules end in a catch-all `direct` (`internal/routing/profile.go:89`), so RU
  traffic already goes direct; the `geoip:ru → direct` rule
  (`profile.go:68-72`) changes nothing observable. It only matters in full-tunnel
  mode (`fullRules`, `profile.go:96-107`), where the catch-all is `proxy` and the
  RU rule is the exception that keeps Russian sites off the VPN. But the checkbox
  sits outside the `whitelist-only` block (`index.html:72`), so it shows in both
  modes — creating the illusion it applies to whitelist.
- **«Kill switch»** is effective only in **TUN + whitelist + Linux**
  (`internal/tunnel/tunnel.go:116` gates on `KillSwitch && !Full`;
  `internal/firewall/firewall_other.go:16` makes `Supported()` false off Linux).
  On Windows it renders as a greyed, dead checkbox (`main.ts:172`); in proxy or
  full-tunnel mode it is silently ignored. Its label never explains what it does.

Root cause: two orthogonal dropdowns (connection mode proxy/tun + routing mode
whitelist/full) plus mode-specific toggles form a matrix where, in any given
state, some controls do nothing. Users see no effect and lose trust.

## Approach

Keep the existing two-axis model and backend untouched. Make the UI **contextual**:
show each control only in the state where it has a real effect, relabel headings
in plain language, and add a short inline hint under each control. Inert controls
are **hidden** (not greyed), so what remains visible always does something.

## Contextual visibility rules

State inputs already available in the frontend render path (`main.ts` `render`):
`current.profile.full` (routing), `current.settings.mode` (`"proxy"`/`"tun"`),
`st.caps.killSwitchSupported`.

| Control (element id) | Visible when |
|---|---|
| Подключение (`mode-select`) | always |
| Что через VPN (`routing-mode-select`) | always |
| Telegram → VPN (`tg-toggle`, in `#whitelist-only`) | `routing == whitelist` (i.e. `!full`) — unchanged |
| RU напрямую (`ru-toggle`) | `routing == full` |
| Kill switch (`kill-toggle`) | `mode == "tun"` AND `killSwitchSupported` AND `routing == whitelist` (`!full`) |
| Mux (`mux-toggle`) | always |

Implementation mirrors the existing `#whitelist-only` show/hide pattern
(`main.ts:158`, `index.html:69`):
- Wrap `ru-toggle`'s `<label>` in a new `#full-only` container, toggled
  `hidden` by `!full` (inverse of `#whitelist-only`).
- Compute kill-switch visibility in `render` and toggle the `hidden` class on
  the `kill-toggle`'s `<label>`. Replace the current "disable + tooltip" handling
  (`main.ts:170-173`) with hide/show; the `disabled`/`title` lines are removed
  because the control is hidden whenever it would have been disabled.

Hidden toggles keep their stored values (`forceRUDirect`, `killSwitch` remain in
the profile/settings and persist). They reappear with the stored value when the
user returns to a state where they apply. Backend gating is unchanged, so a
stored-but-hidden value remains a harmless no-op exactly as today.

## Labels and inline hints

Headings and option labels are reworded; a muted hint line is added under the
relevant control. Exact Russian strings:

**Heading «Режим» → «Подключение»**, options:
- `proxy` → label «Proxy — без прав администратора»
  hint: «Трафик идёт через системный SOCKS. Telegram его игнорирует — для него выберите TUN.»
- `tun` → label «TUN — нужны права администратора»
  hint: «Перехватывает трафик сам, работает со всеми приложениями.»

The connection hint shows the line matching the selected option (computed in
`render`, placed in a `<p class="hint">` under `mode-select`).

**Heading «Маршрутизация» → «Что через VPN»**, options:
- `whitelist` → label «Только выбранное»
  hint: «В VPN идёт только Telegram. Остальное — напрямую.»
- `full` → label «Всё через VPN»
  hint: «Весь трафик через туннель. Российские сайты можно оставить напрямую.»

The routing hint shows the line matching the selected option, in a `<p class="hint">`
under `routing-mode-select`.

**Per-toggle hints** (static `<span class="hint">` next to/under each label):
- Telegram → VPN: «Заворачивать Telegram в туннель.»
- RU напрямую: «Российские сайты (geoip:ru) — мимо VPN. Быстрее и без блокировок зарубежных сервисов.»
- Kill switch: «Если VPN отвалится — блокировать трафик выбранных адресов, чтобы он не утёк напрямую.»
- Mux: keep existing inline label «Mux (для Telegram — на сервере клиент без flow)»; no extra hint.

## Styling

Add a `.hint` CSS class: small, muted (`var(--muted)`), block, slight top margin,
indented under its control. One new rule in `style.css`; no layout restructure.

## Components / files

- `frontend/index.html` — rename headings; reword `<option>` labels; add
  `#full-only` wrapper around `ru-toggle`; add `<p class="hint">` placeholders for
  the connection and routing hints (filled by `render`); add static `<span class="hint">`
  under the Telegram/RU/Kill toggles.
- `frontend/src/main.ts` — in `render`: set connection + routing hint text from the
  selected option; toggle `#full-only` `hidden` on `!full`; compute and apply
  kill-switch label visibility; remove the kill-switch `disabled`/`title` lines.
- `frontend/src/style.css` — add `.hint`.

## Out of scope (flagged, not built)

- **Custom proxy list editor.** `customProxyDomains`/`customProxyIPs` exist in the
  DTO (`main.ts:25-26`) but have no UI, so whitelist mode is effectively
  «Telegram only». Adding an editor is a separate feature.
- **Kill switch for full-tunnel mode.** Protecting all traffic would need backend
  work (`tunnel.go:116` currently gates on `!Full`); not addressed here.

## Testing

This is presentation logic with no backend change.
- Manual QA matrix (the value of the redesign is the contextual behavior):
  - whitelist + proxy: Telegram toggle + Mux visible; RU напрямую and Kill switch hidden.
  - whitelist + tun (Linux): Kill switch also visible; RU напрямую hidden.
  - whitelist + tun (Windows): Kill switch hidden (unsupported); RU напрямую hidden.
  - full + proxy/tun: RU напрямую visible; Telegram toggle and Kill switch hidden.
  - Switching routing whitelist↔full preserves a previously-set RU напрямую value.
- `npm run build` (tsc + vite) must pass.
- Optional: if a small pure helper is extracted (e.g. `connectionHint(mode)` /
  `routingHint(full)` returning the string), add a unit test for it; otherwise the
  change is DOM wiring verified by manual QA.
