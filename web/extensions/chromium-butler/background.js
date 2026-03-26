const HOST_NAME = "com.butler.browser_bridge";
const SUPPORTED_PROTOCOLS = new Set(["http:", "https:"]);
const TRANSPORT_MODE_NATIVE = "native";
const TRANSPORT_MODE_REMOTE = "remote";
const ROLLOUT_MODE_NATIVE_ONLY = "native_only";
const ROLLOUT_MODE_DUAL = "dual";
const ROLLOUT_MODE_REMOTE_PREFERRED = "remote_preferred";
const FORM_STORAGE_KEY = "butler.chromiumBridge.form";
const BROWSER_INSTANCE_STORAGE_KEY = "butler.chromiumBridge.browserInstanceID";
const REMOTE_RELAY_BOOTSTRAP_ALARM = "butler.remoteRelay.bootstrap";
const pendingNativeRequests = new Map();
const remoteRelayLoops = new Map();
const remoteBindRelayLoops = new Map();

let nativePort = null;

chrome.runtime.onInstalled.addListener(() => {
  console.info("Butler Chromium Bridge installed");
  ensureRelayBootstrapAlarm();
  void ensureNativePort().catch(() => {
    // Native host is optional when transport mode is remote.
  });
  void bootstrapRemoteRelayFromStorage();
});

chrome.runtime.onStartup.addListener(() => {
  ensureRelayBootstrapAlarm();
  void ensureNativePort().catch(() => {
    // Native host is optional when transport mode is remote.
  });
  void bootstrapRemoteRelayFromStorage();
});

chrome.storage.onChanged.addListener((changes, areaName) => {
  if (areaName !== "local" || !changes?.[FORM_STORAGE_KEY]) {
    return;
  }
  void bootstrapRemoteRelayFromStorage();
});

chrome.alarms.onAlarm.addListener((alarm) => {
  if (alarm?.name !== REMOTE_RELAY_BOOTSTRAP_ALARM) {
    return;
  }
  void bootstrapRemoteRelayFromStorage();
});

chrome.runtime.onMessage.addListener((message, _sender, sendResponse) => {
  void handleMessage(message)
    .then((result) => sendResponse({ ok: true, result }))
    .catch((error) =>
      sendResponse({
        ok: false,
        error: error instanceof Error ? error.message : String(error)
      })
    );
  return true;
});

ensureRelayBootstrapAlarm();
void ensureNativePort().catch(() => {
  // Native host is optional when transport mode is remote.
});
void bootstrapRemoteRelayFromStorage();

function ensureRelayBootstrapAlarm() {
  chrome.alarms.create(REMOTE_RELAY_BOOTSTRAP_ALARM, {
    periodInMinutes: 1
  });
}

async function handleMessage(message) {
  switch (message?.type) {
    case "connect-remote":
      return connectRemote(message.payload);
    case "list-tabs":
      return listTabs();
    case "create-bind-request":
      return createBindRequest(message.payload);
    case "get-active-session":
      return getActiveSession(message.payload);
    default:
      throw new Error(`Unsupported popup message: ${String(message?.type)}`);
  }
}

async function connectRemote(payload) {
  const transport = normalizeTransport(payload?.transport);
  if (transport.mode !== TRANSPORT_MODE_REMOTE) {
    throw new Error("Connection mode must be remote to establish relay");
  }
  const browserInstanceID = await getOrCreateBrowserInstanceID();
  ensureRemoteBindRelayLoop(transport, browserInstanceID);
  await checkRemoteRelayAccess(transport, browserInstanceID);
  return {
    connected: true,
    browser_instance_id: browserInstanceID,
    relay: "bind_discovery_active"
  };
}

async function listTabs() {
  const tabs = await chrome.tabs.query({});
  return tabs
    .filter((tab) => Number.isInteger(tab.id))
    .map(normalizeTabCandidate)
    .filter(Boolean)
    .sort((left, right) => left.display_label.localeCompare(right.display_label));
}

async function createBindRequest(payload) {
  const runID = String(payload?.run_id ?? "").trim();
  const sessionKey = String(payload?.session_key ?? "").trim();
  const transport = normalizeTransport(payload?.transport);
  if (!runID) {
    throw new Error("Run ID is required");
  }
  if (!sessionKey) {
    throw new Error("Session key is required");
  }

  const tabCandidates = await listTabs();
  if (tabCandidates.length === 0) {
    throw new Error("No supported browser tabs were found");
  }

  if (transport.mode === TRANSPORT_MODE_REMOTE) {
    return createBindRequestRemote(runID, sessionKey, tabCandidates, transport);
  }

  return sendNativeRequest("bind.request", {
    run_id: runID,
    session_key: sessionKey,
    requested_via: "chromium_extension_popup",
    request_source: "chromium_extension",
    browser_hint: navigator.userAgent,
    tab_candidates: tabCandidates
  });
}

async function getActiveSession(payload) {
  const sessionKey = String(payload?.session_key ?? "").trim();
  const transport = normalizeTransport(payload?.transport);
  if (!sessionKey) {
    throw new Error("Session key is required");
  }

  if (transport.mode === TRANSPORT_MODE_REMOTE) {
    return getActiveSessionRemote(sessionKey, transport);
  }

  return sendNativeRequest("session.get_active", {
    session_key: sessionKey
  });
}

async function createBindRequestRemote(runID, sessionKey, tabCandidates, transport) {
  const browserInstanceID = await getOrCreateBrowserInstanceID();
  ensureRemoteBindRelayLoop(transport, browserInstanceID);
  const response = await remoteJSONRequest(transport, "/api/v2/extension/single-tab/bind-requests", {
    method: "POST",
    body: JSON.stringify({
      run_id: runID,
      session_key: sessionKey,
      requested_via: "chromium_extension_popup_remote",
      request_source: "chromium_extension_remote",
      browser_hint: navigator.userAgent,
      browser_instance_id: browserInstanceID,
      tab_candidates: tabCandidates
    })
  });
  ensureRemoteRelayLoop(transport, sessionKey, browserInstanceID);
  return response;
}

async function getActiveSessionRemote(sessionKey, transport) {
  const browserInstanceID = await getOrCreateBrowserInstanceID();
  ensureRemoteBindRelayLoop(transport, browserInstanceID);
  ensureRemoteRelayLoop(transport, sessionKey, browserInstanceID);
  const baseURL = normalizeRemoteBaseURL(transport.remote_base_url);
  const endpoint = `${baseURL}/api/v2/extension/single-tab/session?session_key=${encodeURIComponent(sessionKey)}&browser_instance_id=${encodeURIComponent(browserInstanceID)}`;
  const response = await fetch(endpoint, {
    method: "GET",
    headers: {
      Authorization: `Bearer ${transport.remote_api_token}`,
      "X-Butler-Browser-Instance": browserInstanceID
    }
  });
  return parseRemoteResponse(response);
}

function normalizeTabCandidate(tab) {
  const url = typeof tab.url === "string" ? tab.url : "";
  if (!url) {
    return null;
  }

  let parsedURL;
  try {
    parsedURL = new URL(url);
  } catch {
    return null;
  }

  if (!SUPPORTED_PROTOCOLS.has(parsedURL.protocol)) {
    return null;
  }

  const domain = parsedURL.hostname || parsedURL.host || "unknown";
  const title = String(tab.title || domain).trim() || domain;
  return {
    internal_tab_ref: String(tab.id),
    title,
    domain,
    current_url: url,
    favicon_url: typeof tab.favIconUrl === "string" ? tab.favIconUrl : "",
    display_label: `${title} - ${domain}`
  };
}

function normalizeTransport(raw) {
  const requestedMode = normalizeTransportMode(raw?.mode);
  const rolloutMode = normalizeRolloutMode(raw?.rollout_mode);
  const mode = resolveTransportMode(requestedMode, rolloutMode);
  const remoteBaseURL = String(raw?.remote_base_url ?? "").trim();
  const remoteAPIToken = String(raw?.remote_api_token ?? "").trim();

  if (mode !== TRANSPORT_MODE_REMOTE) {
    return {
      mode: TRANSPORT_MODE_NATIVE,
      rollout_mode: rolloutMode,
      remote_base_url: "",
      remote_api_token: ""
    };
  }
  if (!remoteBaseURL) {
    throw new Error("Remote Butler URL is required for remote mode");
  }
  if (!remoteAPIToken) {
    throw new Error("Remote API token is required for remote mode");
  }

  return {
    mode,
    rollout_mode: rolloutMode,
    remote_base_url: remoteBaseURL,
    remote_api_token: remoteAPIToken
  };
}

function normalizeRemoteBaseURL(rawURL) {
  const trimmed = String(rawURL ?? "").trim().replace(/\/+$/g, "");
  if (!trimmed) {
    throw new Error("Remote Butler URL is required");
  }
  let parsed;
  try {
    parsed = new URL(trimmed);
  } catch {
    throw new Error("Remote Butler URL must be a valid absolute URL");
  }
  if (!["https:", "http:"].includes(parsed.protocol)) {
    throw new Error("Remote Butler URL must use HTTP(S)");
  }
  return trimmed;
}

async function remoteJSONRequest(transport, path, init) {
  const baseURL = normalizeRemoteBaseURL(transport.remote_base_url);
  const endpoint = `${baseURL}${path}`;
  const response = await fetch(endpoint, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${transport.remote_api_token}`,
      ...(init?.headers || {})
    }
  });
  return parseRemoteResponse(response);
}

async function parseRemoteResponse(response) {
  let payload = null;
  try {
    payload = await response.json();
  } catch {
    payload = null;
  }
  if (!response.ok) {
    const message = typeof payload?.error === "string"
      ? payload.error
      : `Remote Butler API returned status ${response.status}`;
    throw new Error(message);
  }
  return payload;
}

async function checkRemoteRelayAccess(transport, browserInstanceID) {
  const baseURL = normalizeRemoteBaseURL(transport.remote_base_url);
  const endpoint = `${baseURL}/api/v2/extension/single-tab/bind-requests/next?browser_instance_id=${encodeURIComponent(browserInstanceID)}&timeout_ms=1000`;
  const response = await fetch(endpoint, {
    method: "GET",
    headers: {
      Authorization: `Bearer ${transport.remote_api_token}`,
      "X-Butler-Browser-Instance": String(browserInstanceID ?? "").trim()
    }
  });
  if (response.status === 204 || response.status === 200) {
    return;
  }
  await parseRemoteResponse(response);
}

function ensureRemoteBindRelayLoop(transport, browserInstanceID) {
  const normalizedBrowserInstanceID = String(browserInstanceID ?? "").trim();
  if (!normalizedBrowserInstanceID) {
    return;
  }
  const baseURL = normalizeRemoteBaseURL(transport.remote_base_url);
  const loopKey = `${baseURL}|bind|${normalizedBrowserInstanceID}`;
  const existing = remoteBindRelayLoops.get(loopKey);
  if (existing && !existing.stopped) {
    existing.transport = transport;
    existing.browserInstanceID = normalizedBrowserInstanceID;
    return;
  }
  if (existing?.stopped) {
    remoteBindRelayLoops.delete(loopKey);
  }

  const state = {
    stopped: false,
    transport,
    browserInstanceID: normalizedBrowserInstanceID
  };
  remoteBindRelayLoops.set(loopKey, state);
  void runRemoteBindRelayLoop(loopKey, state);
}

async function runRemoteBindRelayLoop(loopKey, state) {
  while (!state.stopped) {
    let dispatchID = "";
    try {
      const pending = await pollRemoteBindDispatch(state.transport, state.browserInstanceID);
      if (!pending) {
        continue;
      }
      const dispatch = pending?.dispatch;
      if (!dispatch || typeof dispatch !== "object") {
        continue;
      }
      dispatchID = String(dispatch.dispatch_id ?? "").trim();
      if (!dispatchID) {
        continue;
      }

      const sessionKey = String(dispatch.session_key ?? "").trim();
      if (sessionKey) {
        ensureRemoteRelayLoop(state.transport, sessionKey, state.browserInstanceID);
      }

      const tabCandidates = await listTabs();
      await postRemoteBindDispatchResult(state.transport, dispatchID, state.browserInstanceID, {
        ok: true,
        browser_hint: navigator.userAgent,
        tab_candidates: tabCandidates
      });
    } catch (error) {
      if (dispatchID) {
        const coded = normalizeRuntimeError(error);
        try {
          await postRemoteBindDispatchResult(state.transport, dispatchID, state.browserInstanceID, {
            ok: false,
            error: coded
          });
        } catch (postError) {
          console.warn("Failed to post remote bind dispatch error result", postError);
        }
      } else {
        await sleep(1500);
      }
    }
  }
  if (remoteBindRelayLoops.get(loopKey) === state) {
    remoteBindRelayLoops.delete(loopKey);
  }
}

async function pollRemoteBindDispatch(transport, browserInstanceID) {
  const baseURL = normalizeRemoteBaseURL(transport.remote_base_url);
  const endpoint = `${baseURL}/api/v2/extension/single-tab/bind-requests/next?browser_instance_id=${encodeURIComponent(browserInstanceID)}&timeout_ms=25000`;
  const response = await fetch(endpoint, {
    method: "GET",
    headers: {
      Authorization: `Bearer ${transport.remote_api_token}`,
      "X-Butler-Browser-Instance": String(browserInstanceID ?? "").trim()
    }
  });
  if (response.status === 204) {
    return null;
  }
  return parseRemoteResponse(response);
}

async function postRemoteBindDispatchResult(transport, dispatchID, browserInstanceID, payload) {
  const safeDispatchID = encodeURIComponent(String(dispatchID ?? "").trim());
  if (!safeDispatchID) {
    throw new Error("dispatch id is required");
  }
  const safeBrowserInstanceID = encodeURIComponent(String(browserInstanceID ?? "").trim());
  return remoteJSONRequest(transport, `/api/v2/extension/single-tab/bind-requests/${safeDispatchID}/result?browser_instance_id=${safeBrowserInstanceID}`, {
    method: "POST",
    headers: {
      "X-Butler-Browser-Instance": String(browserInstanceID ?? "").trim()
    },
    body: JSON.stringify(payload)
  });
}

function ensureRemoteRelayLoop(transport, sessionKey, browserInstanceID) {
  const normalizedSessionKey = String(sessionKey ?? "").trim();
  if (!normalizedSessionKey) {
    return;
  }
  const normalizedBrowserInstanceID = String(browserInstanceID ?? "").trim();
  if (!normalizedBrowserInstanceID) {
    return;
  }
  const baseURL = normalizeRemoteBaseURL(transport.remote_base_url);
  const loopKey = `${baseURL}|${normalizedSessionKey}|${normalizedBrowserInstanceID}`;
  const existing = remoteRelayLoops.get(loopKey);
  if (existing && !existing.stopped) {
    existing.transport = transport;
    existing.sessionKey = normalizedSessionKey;
    existing.browserInstanceID = normalizedBrowserInstanceID;
    return;
  }
  if (existing?.stopped) {
    remoteRelayLoops.delete(loopKey);
  }

  const state = {
    stopped: false,
    transport,
    sessionKey: normalizedSessionKey,
    browserInstanceID: normalizedBrowserInstanceID
  };
  remoteRelayLoops.set(loopKey, state);
  void runRemoteRelayLoop(loopKey, state);
}

async function runRemoteRelayLoop(loopKey, state) {
  while (!state.stopped) {
    let dispatchID = "";
    try {
      const pending = await pollRemoteDispatch(state.transport, state.sessionKey, state.browserInstanceID);
      if (!pending) {
        continue;
      }
      const dispatch = pending?.dispatch;
      if (!dispatch || typeof dispatch !== "object") {
        continue;
      }
      dispatchID = String(dispatch.dispatch_id ?? "").trim();
      if (!dispatchID) {
        continue;
      }

      const actionResult = await executeAction({
        single_tab_session_id: dispatch.single_tab_session_id,
        bound_tab_ref: dispatch.bound_tab_ref,
        action_type: dispatch.action_type,
        args_json: dispatch.args_json
      });
      await postRemoteDispatchResult(state.transport, dispatchID, state.browserInstanceID, {
        ok: true,
        result: actionResult
      });
    } catch (error) {
      if (dispatchID) {
        const coded = normalizeRuntimeError(error);
        try {
          await postRemoteDispatchResult(state.transport, dispatchID, state.browserInstanceID, {
            ok: false,
            error: coded
          });
        } catch (postError) {
          console.warn("Failed to post remote dispatch error result", postError);
        }
      } else {
        await sleep(1500);
      }
    }
  }
  if (remoteRelayLoops.get(loopKey) === state) {
    remoteRelayLoops.delete(loopKey);
  }
}

async function pollRemoteDispatch(transport, sessionKey, browserInstanceID) {
  const baseURL = normalizeRemoteBaseURL(transport.remote_base_url);
  const endpoint = `${baseURL}/api/v2/extension/single-tab/actions/next?session_key=${encodeURIComponent(sessionKey)}&browser_instance_id=${encodeURIComponent(browserInstanceID)}&timeout_ms=25000`;
  const response = await fetch(endpoint, {
    method: "GET",
    headers: {
      Authorization: `Bearer ${transport.remote_api_token}`,
      "X-Butler-Browser-Instance": String(browserInstanceID ?? "").trim()
    }
  });
  if (response.status === 204) {
    return null;
  }
  return parseRemoteResponse(response);
}

async function postRemoteDispatchResult(transport, dispatchID, browserInstanceID, payload) {
  const safeDispatchID = encodeURIComponent(String(dispatchID ?? "").trim());
  if (!safeDispatchID) {
    throw new Error("dispatch id is required");
  }
  const safeBrowserInstanceID = encodeURIComponent(String(browserInstanceID ?? "").trim());
  return remoteJSONRequest(transport, `/api/v2/extension/single-tab/actions/${safeDispatchID}/result?browser_instance_id=${safeBrowserInstanceID}`, {
    method: "POST",
    headers: {
      "X-Butler-Browser-Instance": String(browserInstanceID ?? "").trim()
    },
    body: JSON.stringify(payload)
  });
}

function normalizeRuntimeError(error) {
  const fallback = { code: "runtime_error", message: String(error ?? "runtime error") };
  if (!(error instanceof Error)) {
    return fallback;
  }
  const code = String(error.code ?? "").trim();
  return {
    code: code || "runtime_error",
    message: String(error.message || "runtime error")
  };
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function bootstrapRemoteRelayFromStorage() {
  try {
    const stored = await chrome.storage.local.get(FORM_STORAGE_KEY);
    const payload = stored?.[FORM_STORAGE_KEY];
    if (!payload || typeof payload !== "object") {
      stopAllRemoteRelayLoops();
      return;
    }
    const mode = normalizeTransportMode(payload.connection_mode);
    const rolloutMode = normalizeRolloutMode(payload.rollout_mode);
    const effectiveMode = resolveTransportMode(mode, rolloutMode);
    if (effectiveMode !== TRANSPORT_MODE_REMOTE) {
      stopAllRemoteRelayLoops();
      return;
    }
    const transport = normalizeTransport({
      mode,
      rollout_mode: rolloutMode,
      remote_base_url: payload.remote_base_url,
      remote_api_token: payload.remote_api_token
    });
    const browserInstanceID = await getOrCreateBrowserInstanceID();
    const baseURL = normalizeRemoteBaseURL(transport.remote_base_url);
    const bindLoopKey = `${baseURL}|bind|${browserInstanceID}`;
    stopRemoteBindLoopsExcept(bindLoopKey);
    ensureRemoteBindRelayLoop(transport, browserInstanceID);

    const sessionKey = String(payload.session_key ?? "").trim();
    if (sessionKey) {
      const actionLoopKey = `${baseURL}|${sessionKey}|${browserInstanceID}`;
      stopRemoteActionLoopsExcept(actionLoopKey);
      ensureRemoteRelayLoop(transport, sessionKey, browserInstanceID);
    } else {
      stopRemoteActionLoopsExcept("");
    }
  } catch (error) {
    stopAllRemoteRelayLoops();
    console.warn("Failed to bootstrap remote relay from storage", error);
  }
}

function stopAllRemoteRelayLoops() {
  stopRemoteBindLoopsExcept("");
  stopRemoteActionLoopsExcept("");
}

function stopRemoteBindLoopsExcept(allowedKey) {
  for (const [loopKey, state] of remoteBindRelayLoops.entries()) {
    if (loopKey === allowedKey) {
      continue;
    }
    state.stopped = true;
    remoteBindRelayLoops.delete(loopKey);
  }
}

function stopRemoteActionLoopsExcept(allowedKey) {
  for (const [loopKey, state] of remoteRelayLoops.entries()) {
    if (loopKey === allowedKey) {
      continue;
    }
    state.stopped = true;
    remoteRelayLoops.delete(loopKey);
  }
}

async function getOrCreateBrowserInstanceID() {
  const stored = await chrome.storage.local.get(BROWSER_INSTANCE_STORAGE_KEY);
  const existing = String(stored?.[BROWSER_INSTANCE_STORAGE_KEY] ?? "").trim();
  if (existing) {
    return existing;
  }
  const generated = typeof crypto?.randomUUID === "function"
    ? `browser-${crypto.randomUUID()}`
    : `browser-${Date.now()}-${Math.random().toString(16).slice(2)}`;
  await chrome.storage.local.set({
    [BROWSER_INSTANCE_STORAGE_KEY]: generated
  });
  return generated;
}

function normalizeTransportMode(value) {
  const normalized = String(value ?? "").trim().toLowerCase();
  if (normalized === TRANSPORT_MODE_REMOTE) {
    return TRANSPORT_MODE_REMOTE;
  }
  return TRANSPORT_MODE_NATIVE;
}

function normalizeRolloutMode(value) {
  const normalized = String(value ?? "").trim().toLowerCase();
  if (normalized === ROLLOUT_MODE_NATIVE_ONLY) {
    return ROLLOUT_MODE_NATIVE_ONLY;
  }
  if (normalized === ROLLOUT_MODE_REMOTE_PREFERRED) {
    return ROLLOUT_MODE_REMOTE_PREFERRED;
  }
  return ROLLOUT_MODE_DUAL;
}

function resolveTransportMode(mode, rolloutMode) {
  if (rolloutMode === ROLLOUT_MODE_REMOTE_PREFERRED) {
    return TRANSPORT_MODE_REMOTE;
  }
  if (rolloutMode === ROLLOUT_MODE_NATIVE_ONLY) {
    return TRANSPORT_MODE_NATIVE;
  }
  return normalizeTransportMode(mode);
}

async function ensureNativePort() {
  if (nativePort) {
    return nativePort;
  }

  try {
    nativePort = chrome.runtime.connectNative(HOST_NAME);
    nativePort.onMessage.addListener((message) => {
      void handleNativeMessage(message);
    });
    nativePort.onDisconnect.addListener(() => {
      const errorMessage = chrome.runtime.lastError?.message || "Native host disconnected";
      for (const pending of pendingNativeRequests.values()) {
        pending.reject(new Error(errorMessage));
      }
      pendingNativeRequests.clear();
      nativePort = null;
    });
    return nativePort;
  } catch (error) {
    nativePort = null;
    throw new Error(error instanceof Error ? error.message : String(error));
  }
}

async function handleNativeMessage(message) {
  if (message && typeof message.method === "string") {
    await handleHostRequest(message);
    return;
  }

  const pending = pendingNativeRequests.get(message?.id);
  if (!pending) {
    console.warn("Received native message without pending request", message);
    return;
  }
  pendingNativeRequests.delete(message.id);
  if (message.ok !== true) {
    pending.reject(new Error(message?.error?.message || "Native host request failed"));
    return;
  }
  pending.resolve(message.result);
}

async function handleHostRequest(request) {
  switch (request.method) {
    case "action.dispatch":
      try {
        const result = await executeAction(request.params);
        postNativeMessage({
          id: request.id,
          ok: true,
          result
        });
      } catch (error) {
        postNativeMessage({
          id: request.id,
          ok: false,
          error: {
            code: error.code || "runtime_error",
            message: error.message || String(error)
          }
        });
      }
      return;
    default:
      postNativeMessage({
        id: request.id,
        ok: false,
        error: {
          code: "unsupported_method",
          message: `Unsupported host request: ${String(request.method)}`
        }
      });
  }
}

async function sendNativeRequest(method, params) {
  const port = await ensureNativePort();
  const id = crypto.randomUUID();
  const promise = new Promise((resolve, reject) => {
    pendingNativeRequests.set(id, { resolve, reject });
  });
  port.postMessage({ id, method, params });
  return promise;
}

function postNativeMessage(message) {
  if (!nativePort) {
    throw Object.assign(new Error("Native host is not connected"), { code: "host_unavailable" });
  }
  nativePort.postMessage(message);
}

async function executeAction(params) {
  const tabId = parseTabId(params?.bound_tab_ref);
  const tab = await getBoundTab(tabId);
  const actionType = String(params?.action_type ?? "").trim();
  const args = parseArgsJSON(params?.args_json);

  switch (actionType) {
    case "reload":
      return performReload(tab.id, params?.single_tab_session_id);
    case "go_back":
      return performGoBack(tab.id, params?.single_tab_session_id);
    case "go_forward":
      return performGoForward(tab.id, params?.single_tab_session_id);
    case "navigate":
      return performNavigate(tab.id, args, params?.single_tab_session_id);
    case "click":
      return performClick(tab.id, args, params?.single_tab_session_id);
    case "fill":
      return performFill(tab.id, args, params?.single_tab_session_id);
    case "type":
      return performType(tab.id, args, params?.single_tab_session_id);
    case "press_keys":
      return performPressKeys(tab.id, args, params?.single_tab_session_id);
    case "scroll":
      return performScroll(tab.id, args, params?.single_tab_session_id);
    case "wait_for":
      return performWaitFor(tab.id, args, params?.single_tab_session_id);
    case "extract_text":
      return performExtractText(tab.id, args, params?.single_tab_session_id);
    case "capture_visible":
      return performCaptureVisible(tab, params?.single_tab_session_id);
    case "status":
      return buildActionResult(params?.single_tab_session_id, tab, { ok: true });
    default:
      throw codedError("action_not_allowed", `Unsupported action type: ${actionType}`);
  }
}

function parseTabId(boundTabRef) {
  const numeric = Number.parseInt(String(boundTabRef ?? "").trim(), 10);
  if (!Number.isInteger(numeric)) {
    throw codedError("invalid_request", "bound_tab_ref must be a numeric tab id");
  }
  return numeric;
}

function parseArgsJSON(argsJSON) {
  if (!argsJSON) {
    return {};
  }
  try {
    return JSON.parse(argsJSON);
  } catch {
    throw codedError("invalid_request", "args_json must be valid JSON");
  }
}

async function getBoundTab(tabId) {
  try {
    return await chrome.tabs.get(tabId);
  } catch {
    throw codedError("tab_closed", "The bound browser tab is no longer available");
  }
}

async function performNavigate(tabId, args, sessionID) {
  const targetURL = String(args?.url ?? "").trim();
  if (!targetURL) {
    throw codedError("invalid_request", "url is required");
  }
  await chrome.tabs.update(tabId, { url: targetURL });
  const tab = await waitForTabLoad(tabId, 12000);
  return buildActionResult(sessionID, tab, {
    ok: true,
    current_url: tab.url || targetURL,
    title: tab.title || ""
  });
}

async function performReload(tabId, sessionID) {
  await chrome.tabs.reload(tabId);
  const tab = await waitForTabLoad(tabId, 12000);
  return buildActionResult(sessionID, tab, {
    ok: true
  });
}

async function performGoBack(tabId, sessionID) {
  await chrome.scripting.executeScript({
    target: { tabId },
    func: () => {
      history.back();
    }
  }).catch((error) => {
    throw codedError("runtime_error", String(error?.message || error));
  });
  const tab = await waitForTabLoad(tabId, 12000);
  return buildActionResult(sessionID, tab, {
    ok: true
  });
}

async function performGoForward(tabId, sessionID) {
  await chrome.scripting.executeScript({
    target: { tabId },
    func: () => {
      history.forward();
    }
  }).catch((error) => {
    throw codedError("runtime_error", String(error?.message || error));
  });
  const tab = await waitForTabLoad(tabId, 12000);
  return buildActionResult(sessionID, tab, {
    ok: true
  });
}

async function performClick(tabId, args, sessionID) {
  const selector = String(args?.selector ?? "").trim();
  if (!selector) {
    throw codedError("invalid_request", "selector is required");
  }

  await chrome.scripting.executeScript({
    target: { tabId },
    func: (targetSelector) => {
      const element = document.querySelector(targetSelector);
      if (!element) {
        throw new Error("selector_not_found");
      }
      element.click();
    },
    args: [selector]
  }).catch((error) => {
    if (String(error?.message || "").includes("selector_not_found")) {
      throw codedError("selector_not_found", `Selector not found: ${selector}`);
    }
    throw codedError("runtime_error", String(error?.message || error));
  });

  const tab = await getBoundTab(tabId);
  return buildActionResult(sessionID, tab, { ok: true });
}

async function performFill(tabId, args, sessionID) {
  const selector = String(args?.selector ?? "").trim();
  const value = String(args?.value ?? "");
  if (!selector) {
    throw codedError("invalid_request", "selector is required");
  }

  await chrome.scripting.executeScript({
    target: { tabId },
    func: (targetSelector, nextValue) => {
      const element = document.querySelector(targetSelector);
      if (!(element instanceof HTMLInputElement || element instanceof HTMLTextAreaElement)) {
        throw new Error("selector_not_found");
      }
      element.focus();
      element.value = nextValue;
      element.dispatchEvent(new Event("input", { bubbles: true }));
      element.dispatchEvent(new Event("change", { bubbles: true }));
    },
    args: [selector, value]
  }).catch((error) => {
    if (String(error?.message || "").includes("selector_not_found")) {
      throw codedError("selector_not_found", `Fill target not found: ${selector}`);
    }
    throw codedError("runtime_error", String(error?.message || error));
  });

  const tab = await getBoundTab(tabId);
  return buildActionResult(sessionID, tab, { ok: true });
}

async function performType(tabId, args, sessionID) {
  const selector = String(args?.selector ?? "").trim();
  const text = String(args?.text ?? "");
  if (!selector) {
    throw codedError("invalid_request", "selector is required");
  }

  await chrome.scripting.executeScript({
    target: { tabId },
    func: async (targetSelector, nextText) => {
      const element = document.querySelector(targetSelector);
      if (!(element instanceof HTMLInputElement || element instanceof HTMLTextAreaElement)) {
        throw new Error("selector_not_found");
      }
      element.focus();
      for (const character of nextText) {
        element.value += character;
        element.dispatchEvent(new Event("input", { bubbles: true }));
        await new Promise((resolve) => setTimeout(resolve, 15));
      }
      element.dispatchEvent(new Event("change", { bubbles: true }));
    },
    args: [selector, text]
  }).catch((error) => {
    if (String(error?.message || "").includes("selector_not_found")) {
      throw codedError("selector_not_found", `Type target not found: ${selector}`);
    }
    throw codedError("runtime_error", String(error?.message || error));
  });

  const tab = await getBoundTab(tabId);
  return buildActionResult(sessionID, tab, { ok: true });
}

async function performPressKeys(tabId, args, sessionID) {
  const keys = normalizeKeys(args?.keys);
  if (keys.length === 0) {
    throw codedError("invalid_request", "keys must include at least one key");
  }

  await chrome.scripting.executeScript({
    target: { tabId },
    func: (nextKeys) => {
      const active = document.activeElement instanceof HTMLElement ? document.activeElement : document.body;
      for (const key of nextKeys) {
        active.dispatchEvent(new KeyboardEvent("keydown", { key, bubbles: true }));
        if (key.length === 1 && (active instanceof HTMLInputElement || active instanceof HTMLTextAreaElement)) {
          active.value += key;
          active.dispatchEvent(new Event("input", { bubbles: true }));
        }
        if (key === "Enter" && active instanceof HTMLInputElement && active.form) {
          active.form.requestSubmit();
        }
        active.dispatchEvent(new KeyboardEvent("keyup", { key, bubbles: true }));
      }
    },
    args: [keys]
  }).catch((error) => {
    throw codedError("runtime_error", String(error?.message || error));
  });

  const tab = await getBoundTab(tabId);
  return buildActionResult(sessionID, tab, { ok: true, keys });
}

async function performScroll(tabId, args, sessionID) {
  const deltaX = Number.isFinite(args?.x) ? args.x : 0;
  const deltaY = Number.isFinite(args?.y) ? args.y : 600;
  const behavior = String(args?.behavior ?? "smooth");

  await chrome.scripting.executeScript({
    target: { tabId },
    func: (x, y, nextBehavior) => {
      window.scrollBy({
        left: x,
        top: y,
        behavior: nextBehavior === "instant" ? "auto" : nextBehavior
      });
    },
    args: [deltaX, deltaY, behavior]
  }).catch((error) => {
    throw codedError("runtime_error", String(error?.message || error));
  });

  const tab = await getBoundTab(tabId);
  return buildActionResult(sessionID, tab, {
    ok: true,
    x: deltaX,
    y: deltaY
  });
}

async function performWaitFor(tabId, args, sessionID) {
  const selector = String(args?.selector ?? "").trim();
  const timeoutMs = Number.isFinite(args?.timeout_ms) ? args.timeout_ms : 5000;
  if (!selector) {
    throw codedError("invalid_request", "selector is required");
  }

  const [{ result }] = await chrome.scripting.executeScript({
    target: { tabId },
    func: async (targetSelector, nextTimeoutMs) => {
      const startedAt = Date.now();
      while (Date.now() - startedAt < nextTimeoutMs) {
        const element = document.querySelector(targetSelector);
        if (element) {
          const style = window.getComputedStyle(element);
          if (style.display !== "none" && style.visibility !== "hidden") {
            return true;
          }
        }
        await new Promise((resolve) => setTimeout(resolve, 100));
      }
      return false;
    },
    args: [selector, timeoutMs]
  }).catch((error) => {
    throw codedError("runtime_error", String(error?.message || error));
  });

  const tab = await getBoundTab(tabId);
  return buildActionResult(sessionID, tab, {
    matched: Boolean(result)
  });
}

async function performExtractText(tabId, args, sessionID) {
  const selector = String(args?.selector ?? "").trim();
  const [{ result }] = await chrome.scripting.executeScript({
    target: { tabId },
    func: (targetSelector) => {
      if (!targetSelector) {
        return document.body?.innerText || "";
      }
      const element = document.querySelector(targetSelector);
      if (!element) {
        throw new Error("selector_not_found");
      }
      return element.innerText || element.textContent || "";
    },
    args: [selector]
  }).catch((error) => {
    if (String(error?.message || "").includes("selector_not_found")) {
      throw codedError("selector_not_found", `Text target not found: ${selector}`);
    }
    throw codedError("runtime_error", String(error?.message || error));
  });

  const tab = await getBoundTab(tabId);
  return buildActionResult(sessionID, tab, {
    text: typeof result === "string" ? result : ""
  });
}

async function performCaptureVisible(tab, sessionID) {
  const imageDataURL = await chrome.tabs.captureVisibleTab(tab.windowId, { format: "png" })
    .catch((error) => {
      throw codedError("runtime_error", String(error?.message || error));
    });
  return buildActionResult(sessionID, tab, {
    image_ref: imageDataURL
  });
}

async function waitForTabLoad(tabId, timeoutMs) {
  const existing = await getBoundTab(tabId);
  if (existing.status === "complete") {
    return existing;
  }

  return new Promise((resolve, reject) => {
    const timeout = setTimeout(() => {
      chrome.tabs.onUpdated.removeListener(listener);
      reject(codedError("runtime_error", "Timed out waiting for tab load"));
    }, timeoutMs);

    function listener(updatedTabId, changeInfo, updatedTab) {
      if (updatedTabId !== tabId) {
        return;
      }
      if (changeInfo.status === "complete") {
        clearTimeout(timeout);
        chrome.tabs.onUpdated.removeListener(listener);
        resolve(updatedTab);
      }
    }

    chrome.tabs.onUpdated.addListener(listener);
  });
}

function buildActionResult(sessionID, tab, result) {
  const currentURL = typeof tab?.url === "string" ? tab.url : "";
  const currentTitle = typeof tab?.title === "string" ? tab.title : "";
  return {
    single_tab_session_id: String(sessionID ?? ""),
    session_status: "ACTIVE",
    current_url: currentURL,
    current_title: currentTitle,
    result_json: JSON.stringify({
      ...result,
      current_url: currentURL,
      title: currentTitle
    })
  };
}

function codedError(code, message) {
  return Object.assign(new Error(message), { code });
}

function normalizeKeys(keysValue) {
  if (Array.isArray(keysValue)) {
    return keysValue
      .map((item) => String(item ?? "").trim())
      .filter(Boolean);
  }
  const single = String(keysValue ?? "").trim();
  return single ? [single] : [];
}
