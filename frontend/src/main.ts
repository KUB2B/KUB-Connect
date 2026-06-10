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
  Ping,
} from "../wailsjs/go/main/App";
import { EventsOn, BrowserOpenURL } from "../wailsjs/runtime";

type Profile = {
  telegram: boolean;
  forceRUDirect: boolean;
  customProxyDomains: string[];
  customProxyIPs: string[];
};
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

const STATUS: Record<string, string> = {
  connected: "Подключено",
  connecting: "Подключение…",
  disconnecting: "Отключение…",
  disconnected: "Отключено",
  error: "Ошибка",
};

let current: State;

// Ephemeral ping results, keyed by `${host}:${port}` so they survive re-renders
// and index shifts when a server is removed.
const pingResults: Record<string, string> = {};

function setTab(name: string) {
  document.querySelectorAll<HTMLElement>(".tab").forEach((b) => {
    b.classList.toggle("active", b.dataset.tab === name);
  });
  document.querySelectorAll<HTMLElement>(".tab-panel").forEach((p) => {
    p.classList.toggle("hidden", p.id !== "tab-" + name);
  });
}

function render(st: State) {
  // Power button: color from conn state.
  const btn = $("power-btn");
  btn.className = "power " + st.conn;
  $("status-text").textContent = STATUS[st.conn] ?? st.conn;
  $("error-line").textContent = st.lastError || "";

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
  st.servers.forEach((s, i) => {
    const li = document.createElement("li");
    li.className = i === st.activeServer ? "active" : "";
    li.innerHTML = `<span>${s.name} (${s.host}:${s.port})</span>`;
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
    ping.onclick = () => {
      result.textContent = "…";
      Ping(i)
        .then((r) => {
          const text = r.ok ? `${r.latencyMs} мс` : (r.error || "ошибка");
          pingResults[key] = text;
          result.textContent = text;
        })
        .catch(() => {
          pingResults[key] = "ошибка";
          result.textContent = "ошибка";
        });
    };
    li.append(pick, del, ping, result);
    list.append(li);
  });

  (<HTMLInputElement>$("tg-toggle")).checked = st.profile.telegram;
  (<HTMLInputElement>$("ru-toggle")).checked = st.profile.forceRUDirect;
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

function appendLog(line: string) {
  const view = $("log-view");
  view.textContent += line + "\n";
  view.scrollTop = view.scrollHeight;
}

function pushSettings() {
  UpdateSettings(current.settings).catch((e) => ($("error-line").textContent = String(e)));
}

function wire() {
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
      Disconnect().catch((e) => ($("error-line").textContent = String(e)));
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
      Connect().catch((e) => ($("error-line").textContent = String(e)));
    }
    // connecting/disconnecting: ignore.
  });

  $("server-select").addEventListener("change", () => {
    const v = parseInt((<HTMLSelectElement>$("server-select")).value, 10);
    if (v >= 0) SetActiveServer(v);
  });

  $("add-server-btn").addEventListener("click", () => {
    const input = <HTMLInputElement>$("link-input");
    AddServer(input.value)
      .then(() => {
        input.value = "";
        $("link-error").textContent = "";
      })
      .catch((e) => ($("link-error").textContent = String(e)));
  });

  $("tg-toggle").addEventListener("change", () => {
    current.profile.telegram = (<HTMLInputElement>$("tg-toggle")).checked;
    UpdateProfile(current.profile);
  });
  $("ru-toggle").addEventListener("change", () => {
    current.profile.forceRUDirect = (<HTMLInputElement>$("ru-toggle")).checked;
    UpdateProfile(current.profile);
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
  $("loglevel-select").addEventListener("change", () => {
    current.settings.logLevel = (<HTMLSelectElement>$("loglevel-select")).value;
    pushSettings();
  });
  $("clear-logs-btn").addEventListener("click", () => {
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
      $("error-line").textContent = String(e);
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
}

function checkUpdate() {
  CheckUpdate().then((info) => {
    if (!info.available) return;
    const banner = $("update-banner");
    $("update-version").textContent = info.version;
    const link = <HTMLAnchorElement>$("update-link");
    link.href = "#";
    link.onclick = (e) => { e.preventDefault(); BrowserOpenURL(info.url); };
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
