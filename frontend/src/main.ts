import "./style.css";
import {
  GetState,
  AddServer,
  RemoveServer,
  SetActiveServer,
  UpdateProfile,
  UpdateSettings,
  Connect,
  Disconnect,
  Logs,
  HideToTray,
  QuitApp,
  RelaunchElevated,
  CheckUpdate,
  DownloadAndInstall,
  Ping,
} from "../wailsjs/go/main/App";
import { EventsOn, BrowserOpenURL } from "../wailsjs/runtime";

type Profile = {
  full: boolean;
  telegram: boolean;
  forceRUDirect: boolean;
  // Go nil slices arrive as null at runtime; readers use `?? []`.
  customProxyDomains: string[];
  customProxyIPs: string[];
  proxyPresets: string[];
  customDirectDomains: string[];
  customDirectIPs: string[];
};

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

// Mirrors backend validation: bare IPv4/IPv6 or CIDR goes to the IP list,
// everything else is treated as a domain rule (geosite: prefixes contain
// non-hex letters, so they never match the IPv6 branch).
const looksLikeIP = (s: string) =>
  /^\d{1,3}(\.\d{1,3}){3}(\/\d{1,2})?$/.test(s) ||
  (s.includes(":") && /^[0-9a-fA-F:]+(\/\d{1,3})?$/.test(s));
type Settings = {
  mode: string;
  autoConnect: boolean;
  autoStart: boolean;
  killSwitch: boolean;
  mux: boolean;
  logLevel: string;
};
type Server = { name: string; host: string; port: number; security: string; network: string };
type Caps = {
  os: string;
  version: string;
  tunSupported: boolean;
  killSwitchSupported: boolean;
  elevated: boolean;
  autostartSupported: boolean;
};
type State = {
  servers: Server[];
  activeServer: number;
  profile: Profile;
  settings: Settings;
  conn: string;
  lastError: string;
  caps: Caps;
};

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

const STATUS: Record<string, string> = {
  connected: "Подключено",
  connecting: "Подключение…",
  disconnecting: "Отключение…",
  disconnected: "Отключено",
  error: "Ошибка",
};

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

// Ephemeral ping results, keyed by `${host}:${port}` so they survive re-renders
// and index shifts when a server is removed.
const pingResults: Record<string, string> = {};

// Live ping-result <span> per server key, rebuilt on every render so an
// in-flight Ping resolves into the currently-mounted element instead of a
// detached one when a `state` event re-renders the list mid-request.
let pingEls: Record<string, HTMLElement> = {};

function setTab(name: string) {
  document.querySelectorAll<HTMLElement>(".tab").forEach((b) => {
    b.classList.toggle("active", b.dataset.tab === name);
  });
  document.querySelectorAll<HTMLElement>(".tab-panel").forEach((p) => {
    p.classList.toggle("hidden", p.id !== "tab-" + name);
  });
}

function connectionHint(mode: string): string {
  return mode === "tun"
    ? "Перехватывает трафик сам, работает со всеми приложениями."
    : "Трафик идёт через системный SOCKS. Telegram его игнорирует — для него выберите TUN.";
}

function routingHint(full: boolean): string {
  return full
    ? "Весь трафик через туннель. Российские сайты можно оставить напрямую."
    : "В VPN — Telegram, отмеченные сервисы и свои адреса. Остальное напрямую.";
}

// setListArea fills a textarea from the split domain/IP lists, unless the user
// is typing in it (a `state` re-render mid-edit must not clobber the draft).
function setListArea(id: string, ...lists: (string[] | null | undefined)[]) {
  const el = <HTMLTextAreaElement>$(id);
  if (document.activeElement === el) return;
  el.value = lists.flatMap((l) => l ?? []).join("\n");
}

function render(st: State) {
  maybeShowOnboarding(st);

  // Power button: color from conn state.
  const btn = $("power-btn");
  btn.className = "power " + st.conn;
  $("status-text").textContent = STATUS[st.conn] ?? st.conn;
  setError($("error-line"), st.lastError || "");

  // Active-server selector on Главная.
  const sel = <HTMLSelectElement>$("server-select");
  sel.innerHTML = "";
  if (st.servers.length === 0) {
    const opt = document.createElement("option");
    opt.textContent = "Нет серверов — добавьте в Настройках";
    opt.value = "-1";
    sel.append(opt);
    sel.disabled = true;
  } else {
    sel.disabled = false;
    st.servers.forEach((s, i) => {
      const opt = document.createElement("option");
      opt.value = String(i);
      opt.textContent = `${s.name} (${s.host}:${s.port})`;
      sel.append(opt);
    });
    sel.value = String(st.activeServer);
  }

  // Server list on Настройки.
  const list = $("server-list");
  list.innerHTML = "";
  pingEls = {};
  st.servers.forEach((s, i) => {
    const li = document.createElement("li");
    li.className = i === st.activeServer ? "active" : "";
    // textContent, not innerHTML: server name/host come from a user-pasted
    // vless:// link and may contain markup or `&`/`<` characters.
    const label = document.createElement("span");
    label.textContent = `${s.name} (${s.host}:${s.port})`;
    const pick = document.createElement("button");
    pick.textContent = "Выбрать";
    pick.onclick = () => SetActiveServer(i);
    const del = document.createElement("button");
    del.textContent = "Удалить";
    del.onclick = () => RemoveServer(i);
    const key = `${s.host}:${s.port}`;
    const ping = document.createElement("button");
    ping.textContent = "Пинг";
    const result = document.createElement("span");
    result.className = "ping-result";
    result.textContent = pingResults[key] ?? "";
    pingEls[key] = result;
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
    li.append(label, pick, del, ping, result);
    list.append(li);
  });

  (<HTMLInputElement>$("tg-toggle")).checked = st.profile.telegram;
  (<HTMLInputElement>$("ru-toggle")).checked = st.profile.forceRUDirect;
  const selectedPresets = st.profile.proxyPresets ?? [];
  document.querySelectorAll<HTMLInputElement>("#preset-list input").forEach((cb) => {
    cb.checked = selectedPresets.includes(cb.value);
  });
  setListArea("proxy-list", st.profile.customProxyDomains, st.profile.customProxyIPs);
  setListArea("direct-list", st.profile.customDirectDomains, st.profile.customDirectIPs);
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
  (<HTMLInputElement>$("autostart-toggle")).checked = st.settings.autoStart;
  (<HTMLInputElement>$("autoconnect-toggle")).checked = st.settings.autoConnect;
  $("autostart-row").classList.toggle("hidden", !st.caps.autostartSupported);
  (<HTMLSelectElement>$("loglevel-select")).value = st.settings.logLevel || "warning";
}

function refresh() {
  GetState().then((st) => {
    current = st as State;
    render(current);
  });
}

// Cap the in-memory log so a long-running session doesn't grow the DOM text
// node without bound.
const MAX_LOG_LINES = 2000;
const logLines: string[] = [];

function appendLog(line: string) {
  logLines.push(line);
  if (logLines.length > MAX_LOG_LINES) {
    logLines.splice(0, logLines.length - MAX_LOG_LINES);
  }
  const view = $("log-view");
  view.textContent = logLines.join("\n") + "\n";
  view.scrollTop = view.scrollHeight;
}

function pushSettings() {
  UpdateSettings(current.settings).catch((e) => setError($("error-line"), String(e)));
}

function wire() {
  // Error toasts: click anywhere on a shown `.error` to dismiss early.
  document.addEventListener("click", (e) => {
    const target = e.target as HTMLElement;
    if (target.classList.contains("error") && target.classList.contains("show")) {
      target.classList.remove("show");
    }
  });

  // Tabs
  document.querySelectorAll<HTMLElement>(".tab").forEach((b) => {
    b.addEventListener("click", () => setTab(b.dataset.tab!));
  });

  // Distinguishes how the elevate modal was opened: from the power button
  // (connect intent — auto-connect after restart, cancel leaves TUN as-is) vs
  // from the mode dropdown (cancel reverts to proxy).
  let elevateForConnect = false;

  // Power button: toggle based on current state.
  $("power-btn").addEventListener("click", () => {
    const c = current?.conn;
    if (c === "connected") {
      Disconnect().catch((e) => setError($("error-line"), String(e)));
    } else if (c === "disconnected" || c === "error") {
      // TUN needs admin. If unelevated, offer restart-with-admin instead of a
      // doomed Connect that the backend would reject.
      if (
        current.settings.mode === "tun" &&
        current.caps.tunSupported &&
        !current.caps.elevated
      ) {
        elevateForConnect = true;
        $("elevate-modal").classList.remove("hidden");
        return;
      }
      Connect().catch((e) => setError($("error-line"), String(e)));
    }
    // connecting/disconnecting: ignore.
  });

  $("server-select").addEventListener("change", () => {
    const v = parseInt((<HTMLSelectElement>$("server-select")).value, 10);
    if (v >= 0) SetActiveServer(v);
  });

  $("add-server-btn").addEventListener("click", () => {
    const input = <HTMLInputElement>$("link-input");
    const btn = <HTMLButtonElement>$("add-server-btn");
    if (btn.disabled) return;
    btn.disabled = true;
    AddServer(input.value)
      .then(() => {
        input.value = "";
        setError($("link-error"), "");
      })
      .catch((e) => setError($("link-error"), String(e)))
      .finally(() => {
        btn.disabled = false;
      });
  });

  const pushProfile = () => {
    UpdateProfile(current.profile)
      .then(() => setError($("routing-error"), ""))
      .catch((e) => setError($("routing-error"), String(e)));
  };

  $("tg-toggle").addEventListener("change", () => {
    current.profile.telegram = (<HTMLInputElement>$("tg-toggle")).checked;
    pushProfile();
  });
  $("ru-toggle").addEventListener("change", () => {
    current.profile.forceRUDirect = (<HTMLInputElement>$("ru-toggle")).checked;
    pushProfile();
  });

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

  // Address lists: one entry per line; IPs/CIDRs and domains are split into the
  // profile's separate lists (backend validates each).
  const wireListArea = (id: string, apply: (domains: string[], ips: string[]) => void) => {
    $(id).addEventListener("change", () => {
      const lines = (<HTMLTextAreaElement>$(id)).value
        .split("\n")
        .map((s) => s.trim())
        .filter(Boolean);
      apply(lines.filter((l) => !looksLikeIP(l)), lines.filter(looksLikeIP));
      pushProfile();
    });
  };
  wireListArea("proxy-list", (domains, ips) => {
    current.profile.customProxyDomains = domains;
    current.profile.customProxyIPs = ips;
  });
  wireListArea("direct-list", (domains, ips) => {
    current.profile.customDirectDomains = domains;
    current.profile.customDirectIPs = ips;
  });
  $("routing-mode-select").addEventListener("change", () => {
    const full = (<HTMLSelectElement>$("routing-mode-select")).value === "full";
    const prev = current.profile.full;
    current.profile.full = full;
    $("whitelist-only").classList.toggle("hidden", full);
    UpdateProfile(current.profile).catch((e) => {
      current.profile.full = prev;
      (<HTMLSelectElement>$("routing-mode-select")).value = prev ? "full" : "whitelist";
      $("whitelist-only").classList.toggle("hidden", prev);
      setError($("error-line"), String(e));
    });
  });
  $("mode-select").addEventListener("change", () => {
    const val = (<HTMLSelectElement>$("mode-select")).value;
    current.settings.mode = val;
    pushSettings();
    // TUN needs admin. If we are not elevated, offer a restart-with-admin.
    if (val === "tun" && current.caps.tunSupported && !current.caps.elevated) {
      elevateForConnect = false;
      $("elevate-modal").classList.remove("hidden");
    }
  });
  $("kill-toggle").addEventListener("change", () => {
    current.settings.killSwitch = (<HTMLInputElement>$("kill-toggle")).checked;
    pushSettings();
  });
  $("mux-toggle").addEventListener("change", () => {
    current.settings.mux = (<HTMLInputElement>$("mux-toggle")).checked;
    pushSettings();
  });
  $("autostart-toggle").addEventListener("change", () => {
    const cb = <HTMLInputElement>$("autostart-toggle");
    const prev = current.settings.autoStart;
    current.settings.autoStart = cb.checked;
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
  $("loglevel-select").addEventListener("change", () => {
    current.settings.logLevel = (<HTMLSelectElement>$("loglevel-select")).value;
    pushSettings();
  });
  $("clear-logs-btn").addEventListener("click", () => {
    logLines.length = 0;
    $("log-view").textContent = "";
  });

  // Close-choice modal.
  const closeModal = $("close-modal");
  $("modal-hide").addEventListener("click", () => {
    closeModal.classList.add("hidden");
    HideToTray();
  });
  $("modal-quit").addEventListener("click", () => {
    QuitApp();
  });
  $("modal-cancel").addEventListener("click", () => {
    closeModal.classList.add("hidden");
  });
  EventsOn("close-requested", () => {
    closeModal.classList.remove("hidden");
  });
  // Escape dismisses the close-choice modal (same as Отмена). Yields to the
  // elevate modal when both happen to be open (it handles its own Escape).
  document.addEventListener("keydown", (e) => {
    const elevateOpen = !$("elevate-modal").classList.contains("hidden");
    if (e.key === "Escape" && !closeModal.classList.contains("hidden") && !elevateOpen) {
      closeModal.classList.add("hidden");
    }
  });

  // Elevate (restart-with-admin) modal.
  const elevateModal = $("elevate-modal");
  // Close the modal. When opened from the mode dropdown (revert=true) we fall
  // back to proxy; when opened from the power button we leave the mode as TUN.
  const closeElevate = (revert: boolean) => {
    elevateModal.classList.add("hidden");
    if (revert) {
      const sel = <HTMLSelectElement>$("mode-select");
      sel.value = "proxy";
      current.settings.mode = "proxy";
      pushSettings();
    }
    elevateForConnect = false;
  };
  $("elevate-restart").addEventListener("click", () => {
    RelaunchElevated(elevateForConnect).catch((e) => {
      closeElevate(!elevateForConnect);
      setError($("error-line"), String(e));
    });
  });
  $("elevate-cancel").addEventListener("click", () => closeElevate(!elevateForConnect));
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && !elevateModal.classList.contains("hidden")) {
      closeElevate(!elevateForConnect);
    }
  });

  EventsOn("state", (st: State) => {
    current = st;
    render(st);
  });
  EventsOn("log", (line: string) => appendLog(line));
  EventsOn("update-progress", (p: { done: number; total: number }) => {
    const pct = p.total > 0 ? Math.round((p.done / p.total) * 100) : 0;
    $("update-percent").textContent = pct + "%";
    (<HTMLElement>$("update-bar-fill")).style.width = pct + "%";
  });
}

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
        setError($("error-line"), "Обновление не удалось: " + String(err));
      });
    };

    banner.classList.remove("hidden");
    $("update-dismiss").onclick = () => banner.classList.add("hidden");
  }).catch(() => {/* network error — silently ignore */});
}

document.addEventListener("DOMContentLoaded", () => {
  wire();
  refresh();
  Logs().then((lines) => (lines as string[]).forEach(appendLog));
  checkUpdate();
});
