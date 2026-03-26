const STORAGE_KEY = "butler.chromiumBridge.form";
const TRANSPORT_MODE_NATIVE = "native";
const TRANSPORT_MODE_REMOTE = "remote";
const ROLLOUT_MODE_NATIVE_ONLY = "native_only";
const ROLLOUT_MODE_DUAL = "dual";
const ROLLOUT_MODE_REMOTE_PREFERRED = "remote_preferred";

const rolloutModeSelect = document.querySelector("#rollout-mode");
const connectionModeSelect = document.querySelector("#connection-mode");
const remoteBaseURLInput = document.querySelector("#remote-base-url");
const remoteAPITokenInput = document.querySelector("#remote-api-token");
const refreshTabsButton = document.querySelector("#refresh-tabs");
const connectRemoteButton = document.querySelector("#connect-remote");
const tabsList = document.querySelector("#tabs-list");
const tabCount = document.querySelector("#tab-count");
const statusPanel = document.querySelector("#status-panel");

void initialize();

async function initialize() {
  await restoreFormState();
  attachEventHandlers();
  await refreshTabs();
  setStatus("Remote relay is configured. Agent-initiated single_tab.bind will trigger tab selection automatically.");
}

function attachEventHandlers() {
  rolloutModeSelect.addEventListener("change", () => {
    updateTransportFieldsState();
    void persistFormState();
  });
  connectionModeSelect.addEventListener("change", () => {
    updateTransportFieldsState();
    void persistFormState();
  });
  remoteBaseURLInput.addEventListener("input", persistFormState);
  remoteAPITokenInput.addEventListener("input", persistFormState);
  refreshTabsButton.addEventListener("click", () => void refreshTabs());
  connectRemoteButton.addEventListener("click", () => void connectRemote());
}

async function restoreFormState() {
  const stored = await chrome.storage.local.get(STORAGE_KEY);
  const payload = stored[STORAGE_KEY];
  if (!payload || typeof payload !== "object") {
    updateTransportFieldsState();
    return;
  }
  rolloutModeSelect.value = normalizeRolloutMode(payload.rollout_mode);
  connectionModeSelect.value = normalizeConnectionMode(payload.connection_mode);
  remoteBaseURLInput.value = typeof payload.remote_base_url === "string" ? payload.remote_base_url : "";
  remoteAPITokenInput.value = typeof payload.remote_api_token === "string" ? payload.remote_api_token : "";
  updateTransportFieldsState();
}

async function persistFormState() {
  await chrome.storage.local.set({
    [STORAGE_KEY]: {
      rollout_mode: normalizeRolloutMode(rolloutModeSelect.value),
      connection_mode: normalizeConnectionMode(connectionModeSelect.value),
      remote_base_url: remoteBaseURLInput.value.trim(),
      remote_api_token: remoteAPITokenInput.value.trim()
    }
  });
}

async function refreshTabs() {
  setStatus("Loading visible tab candidates...");
  try {
    const tabs = await sendMessage({ type: "list-tabs" });
    renderTabs(Array.isArray(tabs) ? tabs : []);
    setStatus(`Discovered ${Array.isArray(tabs) ? tabs.length : 0} supported tab(s).`);
  } catch (error) {
    setStatus(formatError(error));
  }
}

async function connectRemote() {
  const payload = currentPayload();
  setStatus("Connecting remote extension relay...");
  try {
    await persistFormState();
    const result = await sendMessage({
      type: "connect-remote",
      payload
    });
    setStatus(`Connected. Browser instance: ${result.browser_instance_id}`);
  } catch (error) {
    setStatus(formatError(error));
  }
}

function currentPayload() {
  const rolloutMode = normalizeRolloutMode(rolloutModeSelect.value);
  const connectionMode = normalizeConnectionMode(connectionModeSelect.value);
  return {
    transport: {
      rollout_mode: rolloutMode,
      mode: resolveTransportMode(connectionMode, rolloutMode),
      remote_base_url: remoteBaseURLInput.value.trim(),
      remote_api_token: remoteAPITokenInput.value.trim()
    }
  };
}

function renderTabs(tabs) {
  tabsList.replaceChildren();
  tabCount.textContent = `${tabs.length} supported tab(s)`;
  if (tabs.length === 0) {
    const empty = document.createElement("li");
    empty.className = "tab-card is-empty";
    empty.textContent = "No HTTP(S) tabs are currently available for approval.";
    tabsList.appendChild(empty);
    return;
  }

  tabs.forEach((tab) => {
    const item = document.createElement("li");
    item.className = "tab-card";

    const title = document.createElement("p");
    title.className = "tab-title";
    title.textContent = tab.title || tab.domain || "Untitled tab";

    const meta = document.createElement("p");
    meta.className = "tab-meta";
    meta.textContent = `${tab.domain} | ${tab.current_url}`;

    item.append(title, meta);
    tabsList.appendChild(item);
  });
}

function setStatus(message) {
  statusPanel.textContent = message;
}

function formatError(error) {
  if (error instanceof Error) {
    return error.message;
  }
  return String(error);
}

async function sendMessage(message) {
  const response = await chrome.runtime.sendMessage(message);
  if (!response?.ok) {
    throw new Error(response?.error || "Extension request failed");
  }
  return response.result;
}

function normalizeConnectionMode(value) {
  const normalized = String(value ?? "").trim().toLowerCase();
  if (normalized === TRANSPORT_MODE_NATIVE) {
    return TRANSPORT_MODE_NATIVE;
  }
  return TRANSPORT_MODE_REMOTE;
}

function normalizeRolloutMode(value) {
  const normalized = String(value ?? "").trim().toLowerCase();
  if (normalized === ROLLOUT_MODE_NATIVE_ONLY) {
    return ROLLOUT_MODE_NATIVE_ONLY;
  }
  if (normalized === ROLLOUT_MODE_DUAL) {
    return ROLLOUT_MODE_DUAL;
  }
  return ROLLOUT_MODE_REMOTE_PREFERRED;
}

function resolveTransportMode(connectionMode, rolloutMode) {
  if (rolloutMode === ROLLOUT_MODE_REMOTE_PREFERRED) {
    return TRANSPORT_MODE_REMOTE;
  }
  if (rolloutMode === ROLLOUT_MODE_NATIVE_ONLY) {
    return TRANSPORT_MODE_NATIVE;
  }
  return connectionMode;
}

function updateTransportFieldsState() {
  const rolloutMode = normalizeRolloutMode(rolloutModeSelect.value);
  const isDual = rolloutMode === ROLLOUT_MODE_DUAL;
  const effectiveMode = resolveTransportMode(normalizeConnectionMode(connectionModeSelect.value), rolloutMode);
  connectionModeSelect.disabled = !isDual;
  const isRemote = effectiveMode === TRANSPORT_MODE_REMOTE;
  remoteBaseURLInput.disabled = !isRemote;
  remoteAPITokenInput.disabled = !isRemote;
}
