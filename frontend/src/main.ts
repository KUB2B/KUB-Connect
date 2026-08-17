import "./style.css";
import {
  AddServer,
  CheckUpdate,
  Connect,
  Disconnect,
  DownloadAndInstall,
  GetState,
  HideToTray,
  Logs,
  Ping,
  Presets,
  QuitApp,
  RelaunchElevated,
  RemoveServer,
  SetActiveServer,
  UpdateProfile,
  UpdateSettings,
} from "../wailsjs/go/main/App";
import { BrowserOpenURL, EventsOn } from "../wailsjs/runtime";

type Profile = {
  full: boolean;
  telegram: boolean;
  forceRUDirect: boolean;
  customProxyDomains: string[];
  customProxyIPs: string[];
  proxyPresets: string[];
  customDirectDomains: string[];
  customDirectIPs: string[];
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
type Preset = { key: string; title: string };

const $ = <T extends HTMLElement = HTMLElement>(id: string) => document.getElementById(id) as T;
const cloneSettings = (s: Settings): Settings => ({ ...s });
const cloneProfile = (p: Profile): Profile => ({
  ...p,
  customProxyDomains: [...(p.customProxyDomains ?? [])],
  customProxyIPs: [...(p.customProxyIPs ?? [])],
  proxyPresets: [...(p.proxyPresets ?? [])],
  customDirectDomains: [...(p.customDirectDomains ?? [])],
  customDirectIPs: [...(p.customDirectIPs ?? [])],
});

function normalizeState(raw: State): State {
  raw.servers ??= [];
  raw.profile.customProxyDomains ??= [];
  raw.profile.customProxyIPs ??= [];
  raw.profile.proxyPresets ??= [];
  raw.profile.customDirectDomains ??= [];
  raw.profile.customDirectIPs ??= [];
  return raw;
}

function errorText(error: unknown): string {
  const text = error instanceof Error ? error.message : String(error ?? "Неизвестная ошибка");
  return text.replace(/^(?:Error:\s*)+/i, "");
}

const errorTimers = new WeakMap<HTMLElement, number>();
function setError(el: HTMLElement, text: string, timeout = 6500) {
  const previous = errorTimers.get(el);
  if (previous) window.clearTimeout(previous);
  if (el.textContent === text && el.classList.contains("show")) return;
  el.textContent = text;
  el.classList.toggle("show", Boolean(text));
  if (text && timeout > 0) {
    const timer = window.setTimeout(() => el.classList.remove("show"), timeout);
    errorTimers.set(el, timer);
  }
}

let transientGlobalError = "";
let transientGlobalErrorTimer = 0;
let dismissedBackendError = "";
function showGlobalError(text: string, timeout = 6500) {
  window.clearTimeout(transientGlobalErrorTimer);
  transientGlobalError = text;
  setError($("error-line"), text, 0);
  if (timeout > 0) {
    transientGlobalErrorTimer = window.setTimeout(() => {
      if (transientGlobalError !== text) return;
      transientGlobalError = "";
      if (!current?.lastError) setError($("error-line"), "");
    }, timeout);
  }
}

const STATUS: Record<string, string> = {
  connected: "Подключено",
  connecting: "Подключение…",
  disconnecting: "Отключение…",
  disconnected: "Не подключено",
  error: "Ошибка подключения",
};

const STATUS_DESCRIPTION: Record<string, string> = {
  connected: "Соединение активно, правила маршрутизации применены",
  connecting: "Запускаем защищённый туннель",
  disconnecting: "Безопасно завершаем текущую сессию",
  disconnected: "Нажмите кнопку, чтобы включить защиту",
  error: "Проверьте сообщение ниже и повторите подключение",
};

let current: State | null = null;
let backendState: State | null = null;
let stateEventVersion = 0;
let connectionActionPending = false;

function setTab(name: string, focus = false) {
  const tabs = Array.from(document.querySelectorAll<HTMLButtonElement>(".tab"));
  tabs.forEach((button) => {
    const active = button.dataset.tab === name;
    button.classList.toggle("active", active);
    button.setAttribute("aria-selected", String(active));
    button.tabIndex = active ? 0 : -1;
    if (active && focus) button.focus();
  });
  document.querySelectorAll<HTMLElement>(".tab-panel").forEach((panel) => {
    const active = panel.id === `tab-${name}`;
    panel.classList.toggle("hidden", !active);
    panel.hidden = !active;
  });
}

function connectionHint(mode: string): string {
  return mode === "tun"
    ? "Защищает трафик всех приложений. Нужны права администратора."
    : "Использует системный прокси и не требует прав администратора.";
}

function routingHint(full: boolean): string {
  return full
    ? "Весь трафик идёт через VPN, кроме заданных исключений."
    : "Через VPN идут только выбранные сервисы и адреса.";
}

function setListArea(id: string, ...lists: (string[] | null | undefined)[]) {
  const el = $<HTMLTextAreaElement>(id);
  if (document.activeElement === el) return;
  el.value = lists.flatMap((list) => list ?? []).join("\n");
}

let onboardingShown = false;
function maybeShowOnboarding(state: State) {
  if (state.servers.length !== 0 || onboardingShown) return;
  onboardingShown = true;
  setTab("settings");
  $("section-servers").setAttribute("open", "");
  const input = $<HTMLInputElement>("link-input");
  input.focus();
  input.classList.add("onboard-hint");
  input.addEventListener("input", () => input.classList.remove("onboard-hint"), { once: true });
}

const pingResults = new Map<string, { text: string; className: string }>();
const pingElements = new Map<string, HTMLElement>();

function serverSignature(server: Server): string {
  return `${server.name}\u0000${server.host}\u0000${server.port}\u0000${server.security}\u0000${server.network}`;
}

function serverKey(servers: Server[], index: number): string {
  const signature = serverSignature(servers[index]);
  const occurrence = servers.slice(0, index).filter((server) => serverSignature(server) === signature).length;
  return `${signature}\u0000${occurrence}`;
}

function serverInitial(server: Server): string {
  return (server.name || server.host || "S").trim().charAt(0).toUpperCase();
}

function renderServerList(state: State) {
  const list = $("server-list");
  list.replaceChildren();
  pingElements.clear();
  if (state.servers.length === 0) {
    const empty = document.createElement("li");
    empty.className = "server-empty";
    empty.textContent = "Серверов пока нет — вставьте VLESS-ссылку выше";
    list.append(empty);
    return;
  }

  state.servers.forEach((server, index) => {
    const key = serverKey(state.servers, index);
    const item = document.createElement("li");
    item.className = `server-item${index === state.activeServer ? " active" : ""}`;

    const info = document.createElement("div");
    info.className = "server-info";
    const avatar = document.createElement("span");
    avatar.className = "server-avatar";
    avatar.textContent = serverInitial(server);
    const copy = document.createElement("span");
    copy.className = "server-copy";
    const title = document.createElement("strong");
    title.textContent = server.name || "Без названия";
    const address = document.createElement("small");
    address.textContent = `${server.host}:${server.port} · ${server.security.toUpperCase()} / ${server.network.toUpperCase()}`;
    copy.append(title, address);
    info.append(avatar, copy);

    const result = document.createElement("span");
    const cachedPing = pingResults.get(key);
    result.className = `ping-result ${cachedPing?.className ?? ""}`;
    result.textContent = cachedPing?.text ?? "";
    result.setAttribute("aria-live", "polite");
    pingElements.set(key, result);

    const actions = document.createElement("div");
    actions.className = "server-actions";
    let select: HTMLElement;
    if (index === state.activeServer) {
      select = document.createElement("span");
      select.className = "server-active-label";
      select.textContent = "Выбран";
    } else {
      const selectButton = document.createElement("button");
      selectButton.type = "button";
      selectButton.className = "button server-action";
      selectButton.textContent = "Выбрать";
      selectButton.addEventListener("click", () => void withBusy(selectButton, async () => {
        try {
          await SetActiveServer(index);
        } catch (error) {
          showGlobalError(`Не удалось выбрать сервер: ${errorText(error)}`);
        }
      }));
      select = selectButton;
    }

    const ping = document.createElement("button");
    ping.type = "button";
    ping.className = "button server-action";
    ping.textContent = "Пинг";
    ping.addEventListener("click", () => void withBusy(ping, async () => {
      pingResults.set(key, { text: "…", className: "" });
      result.textContent = "…";
      try {
        const response = await Ping(index);
        const text = response.ok ? `${response.latencyMs} мс` : response.error || "ошибка";
        const className = response.ok
          ? response.latencyMs < 150 ? "ping-fast" : response.latencyMs <= 400 ? "ping-mid" : "ping-slow"
          : "ping-slow";
        pingResults.set(key, { text, className });
        const mounted = pingElements.get(key);
        if (mounted) {
          mounted.textContent = text;
          mounted.className = `ping-result ${className}`;
        }
      } catch (error) {
        pingResults.set(key, { text: "ошибка", className: "ping-slow" });
        const mounted = pingElements.get(key);
        if (mounted) {
          mounted.textContent = "ошибка";
          mounted.className = "ping-result ping-slow";
        }
      }
    }));

    const remove = document.createElement("button");
    remove.type = "button";
    remove.className = "button server-action";
    remove.textContent = "Удалить";
    let removeArmed = false;
    let removeTimer = 0;
    remove.addEventListener("click", () => {
      if (!removeArmed) {
        removeArmed = true;
        remove.textContent = "Точно?";
        remove.classList.add("danger-confirm");
        removeTimer = window.setTimeout(() => {
          removeArmed = false;
          remove.textContent = "Удалить";
          remove.classList.remove("danger-confirm");
        }, 3500);
        return;
      }
      window.clearTimeout(removeTimer);
      void withBusy(remove, async () => {
        try {
          await RemoveServer(index);
          pingResults.delete(key);
        } catch (error) {
          removeArmed = false;
          remove.textContent = "Удалить";
          remove.classList.remove("danger-confirm");
          showGlobalError(`Не удалось удалить сервер: ${errorText(error)}`);
        }
      });
    });

    actions.append(select, ping, remove);
    item.append(info, result, actions);
    list.append(item);
  });
}

function render(state: State) {
  const power = $<HTMLButtonElement>("power-btn");
  power.className = `power ${state.conn}`;
  power.disabled = connectionActionPending || state.conn === "connecting" || state.conn === "disconnecting";
  power.setAttribute("aria-label", state.conn === "connected" ? "Отключиться" : "Подключиться");
  $("status-text").textContent = STATUS[state.conn] ?? state.conn;
  $("status-description").textContent = state.servers.length === 0
    ? "Добавьте VLESS-сервер, чтобы начать"
    : STATUS_DESCRIPTION[state.conn] ?? "Состояние соединения изменилось";
  const badge = $("status-badge");
  badge.className = `status-badge ${state.conn}`;
  $("status-badge-text").textContent = STATUS[state.conn] ?? state.conn;
  if (!state.lastError) dismissedBackendError = "";
  if (state.lastError && state.lastError !== dismissedBackendError) setError($("error-line"), state.lastError, state.conn === "error" ? 0 : 6500);
  else if (transientGlobalError) setError($("error-line"), transientGlobalError, 0);
  else setError($("error-line"), "");

  const serverSelect = $<HTMLSelectElement>("server-select");
  serverSelect.replaceChildren();
  if (state.servers.length === 0) {
    serverSelect.append(new Option("Сначала добавьте сервер", "-1"));
    serverSelect.disabled = true;
  } else {
    state.servers.forEach((server, index) => {
      serverSelect.append(new Option(`${server.name || server.host} — ${server.host}:${server.port}`, String(index)));
    });
    serverSelect.value = String(state.activeServer);
    serverSelect.disabled = false;
  }

  renderServerList(state);
  $<HTMLInputElement>("tg-toggle").checked = state.profile.telegram;
  $<HTMLInputElement>("ru-toggle").checked = state.profile.forceRUDirect;
  document.querySelectorAll<HTMLInputElement>("#preset-list input[data-preset]").forEach((checkbox) => {
    checkbox.checked = state.profile.proxyPresets.includes(checkbox.value);
  });
  setListArea("proxy-list", state.profile.customProxyDomains, state.profile.customProxyIPs);
  setListArea("direct-list", state.profile.customDirectDomains, state.profile.customDirectIPs);

  const routingSelect = $<HTMLSelectElement>("routing-mode-select");
  routingSelect.value = state.profile.full ? "full" : "whitelist";
  $("whitelist-only").classList.toggle("hidden", state.profile.full);
  $("full-only").classList.toggle("hidden", !state.profile.full);
  $("routing-hint").textContent = routingHint(state.profile.full);

  const modeSelect = $<HTMLSelectElement>("mode-select");
  modeSelect.value = state.settings.mode;
  const tunOption = modeSelect.querySelector<HTMLOptionElement>('option[value="tun"]');
  if (tunOption) {
    tunOption.disabled = !state.caps.tunSupported;
    tunOption.hidden = !state.caps.tunSupported;
  }
  $("connection-hint").textContent = connectionHint(modeSelect.value);
  $<HTMLInputElement>("kill-toggle").checked = state.settings.killSwitch;
  $("kill-row").classList.toggle("hidden", modeSelect.value !== "tun" || !state.caps.killSwitchSupported || state.profile.full);
  $<HTMLInputElement>("mux-toggle").checked = state.settings.mux;
  $<HTMLInputElement>("autostart-toggle").checked = state.settings.autoStart;
  $<HTMLInputElement>("autoconnect-toggle").checked = state.settings.autoConnect;
  $("autostart-row").classList.toggle("hidden", !state.caps.autostartSupported);
  $<HTMLSelectElement>("loglevel-select").value = state.settings.logLevel || "warning";

  $("home-mode").textContent = modeSelect.value === "tun" ? "TUN · вся система" : "Proxy · SOCKS5";
  $("home-routing").textContent = state.profile.full ? "Весь трафик" : "Только выбранное";
  const platform = ({ windows: "Windows", linux: "Linux", darwin: "macOS" } as Record<string, string>)[state.caps.os] ?? state.caps.os;
  $("home-platform").textContent = `${platform}${state.caps.elevated ? " · Admin" : ""}`;
  $("app-version").textContent = state.caps.version ? `Версия ${state.caps.version}` : "";
  document.body.classList.remove("is-loading");
  maybeShowOnboarding(state);
}

async function refresh() {
  const versionBefore = stateEventVersion;
  try {
    const state = normalizeState(await GetState() as unknown as State);
    if (stateEventVersion === versionBefore) applyBackendState(state);
  } catch (error) {
    document.body.classList.remove("is-loading");
    showGlobalError(`Не удалось загрузить состояние: ${errorText(error)}`, 0);
  }
}

let settingsQueue: Promise<void> = Promise.resolve();
let profileQueue: Promise<void> = Promise.resolve();
let pendingSettings = 0;
let pendingProfiles = 0;
let settingsRevision = 0;
let profileRevision = 0;
let desiredSettings: Settings | null = null;
let desiredProfile: Profile | null = null;
let saveStatusTimer = 0;

function updateSaveStatus() {
  const status = $("settings-save-status");
  window.clearTimeout(saveStatusTimer);
  if (pendingSettings + pendingProfiles > 0) {
    status.textContent = "Сохраняем изменения…";
    status.className = "save-status saving";
    return;
  }
  status.textContent = current?.conn === "connected"
    ? "Сохранено · применится при следующем подключении"
    : "Сохранено";
  status.className = "save-status saved";
  saveStatusTimer = window.setTimeout(() => {
    status.textContent = "";
    status.className = "save-status";
  }, 1800);
}

function commitSettings(next: Settings): Promise<boolean> {
  if (!current) return Promise.resolve(false);
  const revision = ++settingsRevision;
  desiredSettings = cloneSettings(next);
  current.settings = cloneSettings(next);
  render(current);
  pendingSettings++;
  updateSaveStatus();

  let result = false;
  const task = settingsQueue.then(async () => {
    try {
      await UpdateSettings(next);
      if (backendState) backendState.settings = cloneSettings(next);
      result = true;
    } catch (error) {
      showGlobalError(`Не удалось сохранить настройки: ${errorText(error)}`);
    }
  });
  settingsQueue = task.then(() => undefined);
  return task.then(() => {
    pendingSettings--;
    if (revision === settingsRevision) {
      desiredSettings = null;
      if (backendState) current!.settings = cloneSettings(backendState.settings);
      if (current) render(current);
    }
    updateSaveStatus();
    return result;
  });
}

function commitProfile(next: Profile): Promise<boolean> {
  if (!current) return Promise.resolve(false);
  const revision = ++profileRevision;
  desiredProfile = cloneProfile(next);
  current.profile = cloneProfile(next);
  render(current);
  pendingProfiles++;
  updateSaveStatus();

  let result = false;
  const task = profileQueue.then(async () => {
    try {
      await UpdateProfile(next);
      if (backendState) backendState.profile = cloneProfile(next);
      result = true;
      setError($("routing-error"), "");
    } catch (error) {
      setError($("routing-error"), errorText(error), 0);
    }
  });
  profileQueue = task.then(() => undefined);
  return task.then(() => {
    pendingProfiles--;
    if (revision === profileRevision) {
      desiredProfile = null;
      if (backendState) current!.profile = cloneProfile(backendState.profile);
      if (current) render(current);
    }
    updateSaveStatus();
    return result;
  });
}

function applyBackendState(state: State) {
  backendState = normalizeState(state);
  current = normalizeState({
    ...state,
    servers: [...state.servers],
    settings: desiredSettings ? cloneSettings(desiredSettings) : cloneSettings(state.settings),
    profile: desiredProfile ? cloneProfile(desiredProfile) : cloneProfile(state.profile),
  });
  render(current);
}

async function withBusy(button: HTMLButtonElement, action: () => Promise<void>) {
  if (button.disabled) return;
  button.disabled = true;
  button.classList.add("is-busy");
  try {
    await action();
  } finally {
    if (button.isConnected && button.dataset.keepDisabled !== "true") {
      button.disabled = false;
      button.classList.remove("is-busy");
    }
  }
}

// The initial Logs() snapshot and live EventsOn stream overlap. Buffer live
// lines until the snapshot arrives and remove the common suffix/prefix so the
// same line is not rendered twice.
const MAX_LOG_LINES = 2000;
const logLines: string[] = [];
const bufferedLiveLogs: string[] = [];
let logsReady = false;
let logRenderPending = false;

function trimLogs() {
  if (logLines.length > MAX_LOG_LINES) logLines.splice(0, logLines.length - MAX_LOG_LINES);
}

function renderLogs(forceBottom = false) {
  const view = $("log-view");
  const nearBottom = view.scrollHeight - view.scrollTop - view.clientHeight < 40;
  view.textContent = logLines.length ? `${logLines.join("\n")}\n` : "Здесь появятся события подключения";
  view.classList.toggle("log-empty", logLines.length === 0);
  if (forceBottom || nearBottom) view.scrollTop = view.scrollHeight;
}

function scheduleLogRender() {
  if (logRenderPending) return;
  logRenderPending = true;
  window.requestAnimationFrame(() => {
    logRenderPending = false;
    renderLogs();
  });
}

function receiveLog(line: string) {
  if (!logsReady) {
    bufferedLiveLogs.push(line);
    return;
  }
  logLines.push(line);
  trimLogs();
  scheduleLogRender();
}

async function loadInitialLogs() {
  try {
    const snapshot = await Logs() as string[] ?? [];
    let overlap = Math.min(snapshot.length, bufferedLiveLogs.length);
    while (overlap > 0) {
      const suffix = snapshot.slice(snapshot.length - overlap);
      const prefix = bufferedLiveLogs.slice(0, overlap);
      if (suffix.every((line, index) => line === prefix[index])) break;
      overlap--;
    }
    logLines.push(...snapshot, ...bufferedLiveLogs.slice(overlap));
    trimLogs();
  } catch (error) {
    showGlobalError(`Не удалось загрузить логи: ${errorText(error)}`);
  } finally {
    bufferedLiveLogs.length = 0;
    logsReady = true;
    renderLogs(true);
  }
}

function buildPresetControls(presets: Preset[]) {
  const box = $("preset-list");
  box.querySelectorAll("[data-preset-label]").forEach((node) => node.remove());
  presets.forEach((preset) => {
    const label = document.createElement("label");
    label.className = "preset-chip";
    label.dataset.presetLabel = "1";
    const checkbox = document.createElement("input");
    checkbox.type = "checkbox";
    checkbox.dataset.preset = "1";
    checkbox.value = preset.key;
    const text = document.createElement("span");
    text.textContent = preset.title;
    checkbox.addEventListener("change", () => {
      if (!current) return;
      const next = cloneProfile(current.profile);
      next.proxyPresets = Array.from(document.querySelectorAll<HTMLInputElement>('#preset-list input[data-preset]:checked')).map((item) => item.value);
      void commitProfile(next);
    });
    label.append(checkbox, text);
    box.append(label);
  });
  if (current) render(current);
}

async function loadPresets() {
  try {
    buildPresetControls(await Presets() as unknown as Preset[] ?? []);
  } catch (error) {
    setError($("routing-error"), `Не удалось загрузить пресеты: ${errorText(error)}`, 0);
  }
}

let activeModal: HTMLElement | null = null;
let focusBeforeModal: HTMLElement | null = null;
let elevateForConnect = false;

function openModal(id: string, focusId: string) {
  if (activeModal && activeModal.id !== id) activeModal.classList.add("hidden");
  focusBeforeModal = document.activeElement as HTMLElement | null;
  activeModal = $(id);
  activeModal.classList.remove("hidden");
  window.setTimeout(() => $<HTMLButtonElement>(focusId).focus(), 0);
}

function hideModal(id: string, restoreFocus = true) {
  $(id).classList.add("hidden");
  if (activeModal?.id === id) activeModal = null;
  if (restoreFocus) focusBeforeModal?.focus();
  focusBeforeModal = null;
}

function openElevation(forConnect: boolean) {
  elevateForConnect = forConnect;
  $<HTMLButtonElement>("elevate-cancel").textContent = forConnect ? "Отмена" : "Остаться в Proxy";
  openModal("elevate-modal", "elevate-restart");
}

async function closeElevation(revert: boolean) {
  if (revert && current) {
    const next = cloneSettings(current.settings);
    next.mode = "proxy";
    await commitSettings(next);
  }
  hideModal("elevate-modal");
  elevateForConnect = false;
}

// Mirrors backend validation closely enough to split a mixed textarea into
// the domain and IP fields. The backend remains the source of truth.
const looksLikeIP = (value: string) =>
  /^\d{1,3}(\.\d{1,3}){3}(\/\d{1,2})?$/.test(value) ||
  (value.includes(":") && /^[0-9a-fA-F:]+(\/\d{1,3})?$/.test(value));

function wire() {
  const advancedToggle = $<HTMLInputElement>("advanced-toggle");
  const advanced = localStorage.getItem("ui.advanced") === "1";
  advancedToggle.checked = advanced;
  $("tab-settings").classList.toggle("simple-mode", !advanced);
  advancedToggle.addEventListener("change", () => {
    localStorage.setItem("ui.advanced", advancedToggle.checked ? "1" : "0");
    $("tab-settings").classList.toggle("simple-mode", !advancedToggle.checked);
  });

  document.addEventListener("click", (event) => {
    const target = event.target as HTMLElement;
    if (target.classList.contains("error-toast") && target.classList.contains("show")) {
      transientGlobalError = "";
      dismissedBackendError = current?.lastError ?? "";
      window.clearTimeout(transientGlobalErrorTimer);
      target.classList.remove("show");
    }
  });

  const tabs = Array.from(document.querySelectorAll<HTMLButtonElement>(".tab"));
  tabs.forEach((tab, index) => {
    tab.addEventListener("click", () => setTab(tab.dataset.tab!));
    tab.addEventListener("keydown", (event) => {
      if (event.key !== "ArrowLeft" && event.key !== "ArrowRight" && event.key !== "Home" && event.key !== "End") return;
      event.preventDefault();
      const nextIndex = event.key === "Home" ? 0
        : event.key === "End" ? tabs.length - 1
        : (index + (event.key === "ArrowRight" ? 1 : -1) + tabs.length) % tabs.length;
      setTab(tabs[nextIndex].dataset.tab!, true);
    });
  });

  $<HTMLButtonElement>("power-btn").addEventListener("click", async () => {
    if (!current) return;
    if (current.servers.length === 0) {
      setTab("settings");
      $("section-servers").setAttribute("open", "");
      $<HTMLInputElement>("link-input").focus();
      setError($("link-error"), "Сначала добавьте VLESS-сервер", 0);
      return;
    }
    if (connectionActionPending) return;
    if (current.conn === "connected") {
      connectionActionPending = true;
      render(current);
      try {
        await Disconnect();
      } catch (error) {
        showGlobalError(errorText(error));
      } finally {
        connectionActionPending = false;
        if (current) render(current);
      }
    } else if (current.conn === "disconnected" || current.conn === "error") {
      if (current.settings.mode === "tun" && current.caps.tunSupported && !current.caps.elevated) {
        openElevation(true);
      } else {
        connectionActionPending = true;
        render(current);
        try {
          await Connect();
        } catch (error) {
          showGlobalError(errorText(error));
        } finally {
          connectionActionPending = false;
          if (current) render(current);
        }
      }
    }
  });

  $<HTMLSelectElement>("server-select").addEventListener("change", async (event) => {
    const select = event.currentTarget as HTMLSelectElement;
    const index = Number.parseInt(select.value, 10);
    if (!current || index < 0) return;
    select.disabled = true;
    try {
      await SetActiveServer(index);
    } catch (error) {
      select.value = String(current.activeServer);
      showGlobalError(`Не удалось выбрать сервер: ${errorText(error)}`);
    } finally {
      select.disabled = false;
    }
  });

  $<HTMLFormElement>("add-server-form").addEventListener("submit", (event) => {
    event.preventDefault();
    const input = $<HTMLInputElement>("link-input");
    const button = $<HTMLButtonElement>("add-server-btn");
    const link = input.value.trim();
    if (!link) {
      setError($("link-error"), "Вставьте ссылку vless://", 0);
      input.focus();
      return;
    }
    void withBusy(button, async () => {
      try {
        await AddServer(link);
        input.value = "";
        input.classList.remove("onboard-hint");
        setError($("link-error"), "");
      } catch (error) {
        setError($("link-error"), errorText(error), 0);
      }
    });
  });

  $<HTMLInputElement>("tg-toggle").addEventListener("change", (event) => {
    if (!current) return;
    const next = cloneProfile(current.profile);
    next.telegram = (event.currentTarget as HTMLInputElement).checked;
    void commitProfile(next);
  });
  $<HTMLInputElement>("ru-toggle").addEventListener("change", (event) => {
    if (!current) return;
    const next = cloneProfile(current.profile);
    next.forceRUDirect = (event.currentTarget as HTMLInputElement).checked;
    void commitProfile(next);
  });

  const wireList = (id: string, apply: (profile: Profile, domains: string[], ips: string[]) => void) => {
    $<HTMLTextAreaElement>(id).addEventListener("change", (event) => {
      if (!current) return;
      const lines = (event.currentTarget as HTMLTextAreaElement).value.split("\n").map((line) => line.trim()).filter(Boolean);
      const next = cloneProfile(current.profile);
      apply(next, lines.filter((line) => !looksLikeIP(line)), lines.filter(looksLikeIP));
      void commitProfile(next);
    });
  };
  wireList("proxy-list", (profile, domains, ips) => {
    profile.customProxyDomains = domains;
    profile.customProxyIPs = ips;
  });
  wireList("direct-list", (profile, domains, ips) => {
    profile.customDirectDomains = domains;
    profile.customDirectIPs = ips;
  });

  $<HTMLSelectElement>("routing-mode-select").addEventListener("change", (event) => {
    if (!current) return;
    const next = cloneProfile(current.profile);
    next.full = (event.currentTarget as HTMLSelectElement).value === "full";
    void commitProfile(next);
  });

  $<HTMLSelectElement>("mode-select").addEventListener("change", async (event) => {
    if (!current) return;
    const select = event.currentTarget as HTMLSelectElement;
    const next = cloneSettings(current.settings);
    next.mode = select.value;
    select.disabled = true;
    const saved = await commitSettings(next);
    select.disabled = false;
    if (saved && next.mode === "tun" && current.caps.tunSupported && !current.caps.elevated) openElevation(false);
  });

  const wireSettingCheckbox = (id: string, key: keyof Pick<Settings, "killSwitch" | "mux" | "autoStart" | "autoConnect">) => {
    $<HTMLInputElement>(id).addEventListener("change", (event) => {
      if (!current) return;
      const next = cloneSettings(current.settings);
      next[key] = (event.currentTarget as HTMLInputElement).checked;
      void commitSettings(next);
    });
  };
  wireSettingCheckbox("kill-toggle", "killSwitch");
  wireSettingCheckbox("mux-toggle", "mux");
  wireSettingCheckbox("autostart-toggle", "autoStart");
  wireSettingCheckbox("autoconnect-toggle", "autoConnect");
  $<HTMLSelectElement>("loglevel-select").addEventListener("change", (event) => {
    if (!current) return;
    const next = cloneSettings(current.settings);
    next.logLevel = (event.currentTarget as HTMLSelectElement).value;
    void commitSettings(next);
  });

  $<HTMLButtonElement>("clear-logs-btn").addEventListener("click", () => {
    logLines.length = 0;
    renderLogs();
  });

  $<HTMLButtonElement>("modal-hide").addEventListener("click", () => {
    hideModal("close-modal", false);
    void HideToTray();
  });
  $<HTMLButtonElement>("modal-quit").addEventListener("click", () => void QuitApp());
  $<HTMLButtonElement>("modal-cancel").addEventListener("click", () => hideModal("close-modal"));
  EventsOn("close-requested", () => {
    if (!activeModal) openModal("close-modal", "modal-hide");
  });

  $<HTMLButtonElement>("elevate-cancel").addEventListener("click", () => void closeElevation(!elevateForConnect));
  $<HTMLButtonElement>("elevate-restart").addEventListener("click", () => {
    const restart = $<HTMLButtonElement>("elevate-restart");
    const cancel = $<HTMLButtonElement>("elevate-cancel");
    void withBusy(restart, async () => {
      cancel.disabled = true;
      try {
        await RelaunchElevated(elevateForConnect);
        restart.textContent = "Перезапуск…";
        restart.dataset.keepDisabled = "true";
      } catch (error) {
        cancel.disabled = false;
        await closeElevation(!elevateForConnect);
        showGlobalError(errorText(error));
      }
    });
  });

  document.querySelectorAll<HTMLElement>(".modal-overlay").forEach((overlay) => {
    overlay.addEventListener("mousedown", (event) => {
      if (event.target !== overlay) return;
      if (overlay.id === "close-modal") hideModal("close-modal");
      else void closeElevation(!elevateForConnect);
    });
  });
  document.addEventListener("keydown", (event) => {
    if (!activeModal) return;
    if (event.key === "Escape") {
      event.preventDefault();
      if (activeModal.id === "elevate-modal") void closeElevation(!elevateForConnect);
      else hideModal(activeModal.id);
      return;
    }
    if (event.key !== "Tab") return;
    const focusable = Array.from(activeModal.querySelectorAll<HTMLElement>('button:not(:disabled), [href], input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [tabindex]:not([tabindex="-1"])'));
    if (focusable.length === 0) return;
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  });

  EventsOn("state", (state: State) => {
    stateEventVersion++;
    applyBackendState(normalizeState(state));
  });
  EventsOn("log", receiveLog);
  EventsOn("update-progress", (progress: { done: number; total: number }) => {
    const percent = progress.total > 0 ? Math.max(0, Math.min(100, Math.round(progress.done / progress.total * 100))) : 0;
    $("update-percent").textContent = `${percent}%`;
    $("update-bar-fill").style.width = `${percent}%`;
    const bar = $("update-progress-wrap").querySelector<HTMLElement>("[role=progressbar]");
    bar?.setAttribute("aria-valuenow", String(percent));
  });
}

async function checkUpdate() {
  try {
    const info = await CheckUpdate();
    if (!info.available) return;
    $("update-version").textContent = info.version;
    $<HTMLButtonElement>("update-link").onclick = () => BrowserOpenURL(info.url);
    $<HTMLButtonElement>("update-dismiss").onclick = () => $("update-banner").classList.add("hidden");
    $<HTMLButtonElement>("update-btn").onclick = () => {
      const button = $<HTMLButtonElement>("update-btn");
      void withBusy(button, async () => {
        $("update-text").classList.add("hidden");
        $("update-progress-wrap").classList.remove("hidden");
        try {
          await DownloadAndInstall();
        } catch (error) {
          $("update-progress-wrap").classList.add("hidden");
          $("update-text").classList.remove("hidden");
          showGlobalError(`Обновление не удалось: ${errorText(error)}`);
        }
      });
    };
    $("update-banner").classList.remove("hidden");
  } catch {
    // Update checks are deliberately silent: connectivity should not affect VPN UX.
  }
}

document.addEventListener("DOMContentLoaded", () => {
  wire();
  void Promise.allSettled([refresh(), loadInitialLogs(), loadPresets(), checkUpdate()]);
});
