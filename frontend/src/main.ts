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

function render(st: State) {
  const pill = $("status-pill");
  pill.textContent = st.conn;
  pill.className = "pill " + st.conn;

  $("active-server").textContent =
    st.activeServer >= 0 && st.servers[st.activeServer]
      ? st.servers[st.activeServer].name
      : "—";
  $("error-line").textContent = st.lastError || "";

  const list = $("server-list");
  list.innerHTML = "";
  st.servers.forEach((s, i) => {
    const li = document.createElement("li");
    li.className = i === st.activeServer ? "active" : "";
    li.innerHTML = `<span>${s.name} (${s.host}:${s.port})</span>`;
    const sel = document.createElement("button");
    sel.textContent = "Select";
    sel.onclick = () => SetActiveServer(i);
    const del = document.createElement("button");
    del.textContent = "Delete";
    del.onclick = () => RemoveServer(i);
    li.append(sel, del);
    list.append(li);
  });

  (<HTMLInputElement>$("tg-toggle")).checked = st.profile.telegram;
  (<HTMLInputElement>$("ru-toggle")).checked = st.profile.forceRUDirect;
  (<HTMLSelectElement>$("mode-select")).value = st.settings.mode;
  (<HTMLInputElement>$("kill-toggle")).checked = st.settings.killSwitch;
  (<HTMLInputElement>$("mux-toggle")).checked = st.settings.mux;
}

let current: State;

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

function wire() {
  $("connect-btn").addEventListener("click", () => {
    Connect().catch((e) => ($("error-line").textContent = String(e)));
  });
  $("disconnect-btn").addEventListener("click", () => {
    Disconnect().catch((e) => ($("error-line").textContent = String(e)));
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
    UpdateSettings(current.settings).catch((e) => ($("error-line").textContent = String(e)));
  });
  $("kill-toggle").addEventListener("change", () => {
    current.settings.killSwitch = (<HTMLInputElement>$("kill-toggle")).checked;
    UpdateSettings(current.settings).catch((e) => ($("error-line").textContent = String(e)));
  });
  $("mux-toggle").addEventListener("change", () => {
    current.settings.mux = (<HTMLInputElement>$("mux-toggle")).checked;
    UpdateSettings(current.settings).catch((e) => ($("error-line").textContent = String(e)));
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
