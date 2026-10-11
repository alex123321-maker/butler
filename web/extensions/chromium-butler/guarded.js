// Private, opt-in native-host lane. This module receives only an already claimed
// Broker dispatch; observation references are DATA and confer no authority.
const PREFIX = "butler.guardedTab.v1:";
const active = new Set();
const fail = () => Object.assign(new Error("Guarded tab is stale, busy or uncertain"), { code: "guarded_tab_denied" });
const hex = (v, n) => typeof v === "string" && new RegExp(`^[a-f0-9]{${n}}$`).test(v);

// Fixed private native metadata. Owner-facing tokens are created by the kernel.
export async function guardedTabMetadata(method, params) {
  const instanceKey="butler.chromiumBridge.browserInstanceID";
  const host=(await chrome.storage.local.get(instanceKey))[instanceKey];
  if(typeof host!=="string"||!host||host.length>128)throw fail();
  async function describe(tab) {
    if(active.has(tab.id)||!/^https?:\/\//.test(tab.url||""))return null;
    const u=new URL(tab.url);
    if(u.username||u.password)return null;
    const state=(await chrome.storage.local.get(PREFIX+tab.id))[PREFIX+tab.id];
    if(state&&state.phase!=="idle")return null;
    return {browser_instance_id:host,tab_ref:String(tab.id),origin:u.origin,active:tab.active===true};
  }
  if(method==="tabs.guarded.list"){
    if(!params||Object.keys(params).length)throw fail();
    const tabs=[];
    for(const tab of (await chrome.tabs.query({})).sort((a,b)=>a.id-b.id)){
      const t=await describe(tab);if(t)tabs.push(t);
      if(tabs.length>64)throw fail();
    }
    return {tabs};
  }
  if(method!=="tabs.guarded.check"||!params||Object.keys(params).sort().join(",")!=="active,browser_instance_id,origin,tab_ref"||typeof params.active!=="boolean"||params.browser_instance_id!==host)throw fail();
  const t=await describe(await chrome.tabs.get(checkedTab(params.tab_ref)));
  if(!t||t.origin!==params.origin)throw fail();
  return {current:true};
}

export async function denyLegacyGuardedTab(tabId) {
  const key = PREFIX + tabId;
  if ((await chrome.storage.local.get(key))[key]) throw fail();
}
export async function runLegacyTab(tabId, callback) {
  if (active.has(tabId)) throw fail();
  active.add(tabId);
  try { await denyLegacyGuardedTab(tabId); return await callback(); }
  finally { active.delete(tabId); }
}

export async function acknowledgeGuarded(params) {
  const tabId = checkedTab(params?.bound_tab_ref);
  const key = PREFIX + tabId;
  const current = (await chrome.storage.local.get(key))[key];
  if (!current || current.phase !== "awaiting_ack" || current.dispatch_id !== params?.dispatch_id ||
      current.resource_id !== params?.resource_id || current.fence !== params?.resource_fence) throw fail();
  // The host issues this only AFTER durable authority Finish accepted its receipt.
  // Lost ACK leaves the persistent lane blocked; it never re-executes the action.
  await chrome.storage.local.set({ [key]: { ...current, phase: "idle", dispatch_id: "" } });
  return { acknowledged: true };
}

function checkedTab(value) {
  if (typeof value !== "string" || !/^[1-9][0-9]{0,9}$/.test(value)) throw fail();
  const n = Number(value);
  if (!Number.isSafeInteger(n)) throw fail();
  return n;
}

export async function executeGuarded(params) {
  const tabId = checkedTab(params?.bound_tab_ref);
  const key = PREFIX + tabId;
  const deadline = Date.parse(params?.deadline);
  if (!hex(params?.dispatch_id, 48) || !/^browser-tab:[a-f0-9]{64}$/.test(params?.resource_id || "") ||
      !Number.isSafeInteger(params?.resource_fence) || params.resource_fence < 1 ||
      typeof params?.single_tab_session_id !== "string" || !params.single_tab_session_id ||
      !Number.isFinite(deadline) || deadline <= Date.now() || deadline > Date.now() + 31000 || active.has(tabId)) throw fail();
  const action = params?.action_type;
  if (!["status", "click"].includes(action)) throw fail();
  let args;
  try { args = JSON.parse(params.args_json || "{}"); } catch { throw fail(); }
  if (!args || Array.isArray(args) || typeof args !== "object" ||
      Object.keys(args).some((k) => !["session_id", "selector", "observation_id"].includes(k)) ||
      args.session_id !== params.single_tab_session_id ||
      (action === "click" && (typeof args.selector !== "string" || args.selector.length > 1024 ||
        !/^[a-f0-9-]{36}$/.test(args.observation_id || "")))) throw fail();

  active.add(tabId);
  try {
    const instanceKey="butler.chromiumBridge.browserInstanceID";
    const instance=(await chrome.storage.local.get(instanceKey))[instanceKey];
    if (!instance || instance !== params.browser_instance_id) throw fail();
    const current = (await chrome.storage.local.get(key))[key];
    if (current && (current.phase !== "idle" || current.resource_id !== params.resource_id ||
        params.resource_fence <= current.fence)) throw fail();
    if (!current) {
      const all = await chrome.storage.local.get(null);
      if (Object.keys(all).filter((k) => k.startsWith(PREFIX)).length >= 256) throw fail();
    }
    const tab = await chrome.tabs.get(tabId);
    if (!/^https?:\/\//.test(tab.url || "") || new URL(tab.url).origin !== params.allowed_origin) throw fail();
    const staged = { resource_id: params.resource_id, fence: params.resource_fence,
      dispatch_id: params.dispatch_id, phase: "running" };
    // Persist BEFORE scheduling code. Worker restart, timeout or lost result
    // retains a quarantine rather than treating the missing receipt as absence.
    await chrome.storage.local.set({ [key]: staged });
    if (deadline <= Date.now()) throw fail();
    const injections = await chrome.scripting.executeScript({
      target: { tabId }, world: "ISOLATED", func: guardedPageAction,
      args: [{ action, selector: args.selector || "", observation_id: args.observation_id || "",
        session_id: params.single_tab_session_id, resource_id: params.resource_id,
        fence: params.resource_fence, deadline, origin:params.allowed_origin, fresh_id: crypto.randomUUID() }]
    });
    if (injections.length !== 1 || injections[0].frameId !== 0 || !injections[0].documentId ||
        injections[0].result?.ok !== true || deadline <= Date.now()) throw fail();
    const result = injections[0].result;
    await chrome.storage.local.set({ [key]: { ...staged, phase: "awaiting_ack" } });
    return { single_tab_session_id: params.single_tab_session_id, session_status: "ACTIVE",
      result_json: JSON.stringify({ ok: true, observation_id: result.observation_id,
        observation_version: result.observation_version }),
      dispatch_id: params.dispatch_id, resource_fence: params.resource_fence };
  } catch {
    // Never include raw page/Chrome errors or reflected private document content.
    throw fail();
  } finally {
    active.delete(tabId);
  }
}

// This function is serialized by Chrome and runs synchronously in the main
// frame's isolated world. The comparison and DOM action have no await, Promise,
// RPC or timer between them. No page content or field value leaves this world.
function guardedPageAction(p) {
  const key = "__butlerGuardedObservationV1";
  const deny = () => ({ ok: false });
  const snapshot = () => {
    const html = document.documentElement?.outerHTML || "";
    if (html.length > 1048576) throw new Error("bounded");
    const fields = Array.from(document.querySelectorAll("input,textarea,select"));
    if (fields.length > 4096) throw new Error("bounded");
    const values = fields.map((e) => [e.value, e.checked, e.selectedIndex]);
    const value = JSON.stringify([location.href, html, values, scrollX, scrollY,
      document.activeElement?.tagName || "", document.activeElement?.id || ""]);
    if (value.length > 2097152) throw new Error("bounded");
    return value;
  };
  if (Date.now() >= p.deadline || !/^https?:$/.test(location.protocol) || location.origin !== p.origin) return deny();
  let state = globalThis[key];
  if (!state) {
    if (p.action !== "status") return deny();
    state = { resource: p.resource_id, session: p.session_id, version: 1, fence: 0,
      id: "", value: "", dirty: false };
    state.observer = new MutationObserver(() => { state.dirty = true; });
    state.observer.observe(document, { subtree: true, childList: true, attributes: true, characterData: true });
    for (const event of ["pointerdown", "keydown", "input", "change", "focusin", "scroll", "visibilitychange"]) {
      document.addEventListener(event, (e) => { if (e.isTrusted) state.dirty = true; }, true);
    }
    globalThis[key] = state;
  }
  if (state.resource !== p.resource_id || state.session !== p.session_id || p.fence <= state.fence) return deny();
  state.fence = p.fence;
  try {
    const value = snapshot();
    const dirty = state.dirty || state.observer.takeRecords().length > 0 || (state.id && value !== state.value);
    if (dirty) { state.version++; state.id = ""; }
    state.dirty = false;
    if (p.action === "status") {
      state.id = p.fresh_id;
      state.value = value;
      return { ok: true, observation_id: state.id, observation_version: state.version };
    }
    if (p.action !== "click" || !state.id || state.id !== p.observation_id || value !== state.value ||
        Date.now() >= p.deadline) return deny();
    const matches = document.querySelectorAll(p.selector);
    if (matches.length !== 1 || !(matches[0] instanceof HTMLElement) || matches[0].closest("iframe") ||
        matches[0].matches(":disabled") || matches[0].getClientRects().length === 0) return deny();
    const e = matches[0];
    const rect = e.getBoundingClientRect();
    if (rect.width <= 0 || rect.height <= 0 || rect.top < 0 || rect.left < 0 ||
        rect.bottom > innerHeight || rect.right > innerWidth) return deny();
    const top = document.elementFromPoint(rect.left + rect.width / 2, rect.top + rect.height / 2);
    if (!top || (top !== e && !e.contains(top))) return deny();
    if (Date.now() >= p.deadline) return deny();
    // Invalidate BEFORE invoking handlers. An exception/navigation/lost ACK
    // cannot reuse this observation. A later action needs a fresh observation.
    state.id = ""; state.value = ""; state.version++;
    e.click();
    return { ok: true, observation_id: "", observation_version: state.version };
  } catch { return deny(); }
}
