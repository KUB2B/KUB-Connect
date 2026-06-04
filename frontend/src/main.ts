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
} from "../wailsjs/go/main/App";
import { EventsOn } from "../wailsjs/runtime";

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
type State = {
  servers: Server[];
  activeServer: number;
  profile: Profile;
  settings: Settings;
  conn: string;
  lastError: string;
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
    li.append(pick, del);
    list.append(li);
  });

  (<HTMLInputElement>$("tg-toggle")).checked = st.profile.telegram;
  (<HTMLInputElement>$("ru-toggle")).checked = st.profile.forceRUDirect;
  (<HTMLSelectElement>$("mode-select")).value = st.settings.mode;
  (<HTMLInputElement>$("kill-toggle")).checked = st.settings.killSwitch;
  (<HTMLInputElement>$("mux-toggle")).checked = st.settings.mux;
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

  // Power button: toggle based on current state.
  $("power-btn").addEventListener("click", () => {
    const c = current?.conn;
    if (c === "connected") {
      Disconnect().catch((e) => ($("error-line").textContent = String(e)));
    } else if (c === "disconnected" || c === "error") {
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
    current.settings.mode = (<HTMLSelectElement>$("mode-select")).value;
    pushSettings();
  });
  $("kill-toggle").addEventListener("change", () => {
    current.settings.killSwitch = (<HTMLInputElement>$("kill-toggle")).checked;
    pushSettings();
  });
  $("mux-toggle").addEventListener("change", () => {
    current.settings.mux = (<HTMLInputElement>$("mux-toggle")).checked;
    pushSettings();
  });
  $("loglevel-select").addEventListener("change", () => {
    current.settings.logLevel = (<HTMLSelectElement>$("loglevel-select")).value;
    pushSettings();
  });
  $("clear-logs-btn").addEventListener("click", () => {
    $("log-view").textContent = "";
  });

  EventsOn("state", (st: State) => {
    current = st;
    render(st);
  });
  EventsOn("log", (line: string) => appendLog(line));
}

document.addEventListener("DOMContentLoaded", () => {
  wire();
  refresh();
  Logs().then((lines) => (lines as string[]).forEach(appendLog));
});
