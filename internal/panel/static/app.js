const kinds = [
  { key: "devices", title: "Devices", crumb: "TAP/TUN interfaces", icon: "cpu", configKey: "Devices" },
  { key: "listeners", title: "Listeners", crumb: "Inbound raw/Xray endpoints", icon: "radio", configKey: "Listeners" },
  { key: "connectors", title: "Connectors", crumb: "Outbound raw/Xray endpoints", icon: "plug", configKey: "Connectors" },
  { key: "clients", title: "Clients", crumb: "Identity and limits", icon: "user", configKey: "Clients" },
  { key: "routes", title: "Routes", crumb: "Object binding", icon: "route", configKey: "Routes" },
  { key: "xrayProfiles", title: "Xray Profiles", crumb: "Embedded/external Xray transport settings", icon: "braces", configKey: "XrayProfiles" },
  { key: "vkeys", title: "vKeys", crumb: "Raw admission keys", icon: "key", configKey: "VKeys" },
  { key: "addresses", title: "Address Limits", crumb: "TUN/TAP IP and MAC guards", icon: "shield", configKey: "Addresses" },
  { key: "settings", title: "Settings", crumb: "Panel and runtime settings", icon: "settings", configKey: "Settings" },
];

const navItems = [
  { key: "dashboard", title: "Dashboard", crumb: "Runtime overview", icon: "gauge" },
  { key: "stats", title: "Stats", crumb: "Aggregated counters", icon: "activity" },
  ...kinds,
  { key: "xrayBinary", title: "Xray Binary", crumb: "External xray-core binary management", icon: "download" },
  { key: "logs", title: "Logs", crumb: "Panel operation events", icon: "list" },
  { key: "backup", title: "Backup", crumb: "Export and restore config", icon: "download" },
  { key: "diagnostics", title: "Diagnostics", crumb: "Process and runtime health", icon: "terminal" },
  { key: "templates", title: "Templates", crumb: "Raw transport pairs", icon: "copy" },
  { key: "config", title: "Config JSON", crumb: "Full object set", icon: "braces" },
  { key: "runtime", title: "Runtime", crumb: "Apply and counters", icon: "activity" },
];

let currentPage = "dashboard";
let currentConfig = null;
let currentState = null;
let editorContext = null;
let toastTimer = null;
let authSession = { authEnabled: false, authenticated: true };
let runtimeTimer = null;
let currentTemplate = null;
const basePath = detectBasePath();
let currentShare = null;
let currentXrayBinary = null;

const el = {
  shell: document.getElementById("shell"),
  loginScreen: document.getElementById("loginScreen"),
  loginForm: document.getElementById("loginForm"),
  loginUsername: document.getElementById("loginUsername"),
  loginPassword: document.getElementById("loginPassword"),
  nav: document.getElementById("nav"),
  pageTitle: document.getElementById("pageTitle"),
  pageCrumb: document.getElementById("pageCrumb"),
  workspace: document.getElementById("workspace"),
  runtimeChip: document.getElementById("runtimeChip"),
  logoutButton: document.getElementById("logoutButton"),
  drawer: document.getElementById("drawer"),
  drawerTitle: document.getElementById("drawerTitle"),
  drawerMeta: document.getElementById("drawerMeta"),
  objectForm: document.getElementById("objectForm"),
  objectEditor: document.getElementById("objectEditor"),
  saveObjectButton: document.getElementById("saveObjectButton"),
  toast: document.getElementById("toast"),
};

document.addEventListener("click", (event) => {
  const close = event.target.closest("[data-close-drawer]");
  if (close) closeDrawer();

  const nav = event.target.closest("[data-nav]");
  if (nav) {
    setPage(nav.dataset.nav);
  }

  const edit = event.target.closest("[data-edit]");
  if (edit) {
    openObjectEditor(edit.dataset.kind, edit.dataset.id);
  }

  const remove = event.target.closest("[data-delete]");
  if (remove) {
    deleteObject(remove.dataset.kind, remove.dataset.id);
  }

  const command = event.target.closest("[data-command]");
  if (command) {
    runCommand(command.dataset.command);
  }
});

el.objectForm.addEventListener("submit", (event) => event.preventDefault());
el.objectForm.addEventListener("input", syncJSONFromForm);
el.objectForm.addEventListener("change", syncJSONFromForm);
el.objectEditor.addEventListener("blur", syncFormFromJSON);
el.saveObjectButton.addEventListener("click", saveObject);
el.loginForm.addEventListener("submit", login);
el.logoutButton.addEventListener("click", logout);

init();

async function init() {
  await refreshAuthSession();
  if (authLocked()) {
    showLogin();
    return;
  }
  await startApp();
}

async function startApp() {
  showApp();
  renderNav();
  await refreshShared();
  await setPage("dashboard");
  if (!runtimeTimer) {
    runtimeTimer = window.setInterval(refreshRuntimeLight, 4000);
  }
}

function authLocked() {
  return Boolean(authSession.authEnabled && !authSession.authenticated);
}

function showLogin() {
  if (runtimeTimer) {
    window.clearInterval(runtimeTimer);
    runtimeTimer = null;
  }
  el.loginScreen.hidden = false;
  el.shell.hidden = true;
  el.logoutButton.hidden = true;
  el.loginPassword.value = "";
  window.setTimeout(() => el.loginUsername.focus(), 0);
}

function showApp() {
  el.loginScreen.hidden = true;
  el.shell.hidden = false;
  el.logoutButton.hidden = !authSession.authEnabled;
}

async function refreshAuthSession() {
  const response = await fetch(apiURL("/api/auth/session"));
  if (!response.ok) {
    authSession = { authEnabled: false, authenticated: true };
    return authSession;
  }
  authSession = await response.json();
  return authSession;
}

async function login(event) {
  event.preventDefault();
  const username = el.loginUsername.value;
  const password = el.loginPassword.value;
  try {
    const result = await api("/api/auth/login", {
      method: "POST",
      body: JSON.stringify({ username, password }),
    });
    authSession = result;
    await refreshAuthSession();
    await startApp();
    showToast("signed in");
  } catch (error) {
    showToast(error.message || String(error));
  }
}

async function logout() {
  await api("/api/auth/logout", { method: "POST" });
  authSession = { authEnabled: true, authenticated: false };
  currentConfig = null;
  currentState = null;
  showLogin();
}

function renderNav() {
  el.nav.innerHTML = navItems.map((item) => {
    const count = item.configKey && currentConfig ? arrayOf(currentConfig[item.configKey]).length : "";
    return `
      <button class="nav-button" data-nav="${item.key}">
        ${icon(item.icon)}
        <span>${escapeHTML(item.title)}</span>
        ${count === "" ? "" : `<span class="count-pill">${count}</span>`}
      </button>
    `;
  }).join("");
  markActiveNav();
}

function markActiveNav() {
  document.querySelectorAll("[data-nav]").forEach((button) => {
    button.classList.toggle("active", button.dataset.nav === currentPage);
  });
}

async function setPage(page) {
  currentPage = page;
  markActiveNav();
  const item = navItems.find((entry) => entry.key === page) || navItems[0];
  el.pageTitle.textContent = item.title;
  el.pageCrumb.textContent = item.crumb;

  try {
    await refreshShared();
    if (page === "dashboard") await renderDashboard();
    else if (page === "stats") await renderStats();
    else if (page === "logs") await renderLogs();
    else if (page === "backup") renderBackup();
    else if (page === "diagnostics") await renderDiagnostics();
    else if (page === "xrayBinary") await renderXrayBinary();
    else if (page === "templates") renderTemplates();
    else if (page === "config") renderConfig();
    else if (page === "runtime") renderRuntime();
    else renderObjects(page);
  } catch (error) {
    renderError(error);
  }
}

async function refreshShared() {
  const [configResponse, stateResponse] = await Promise.all([
    api("/api/config"),
    api("/api/runtime/state"),
  ]);
  currentConfig = configResponse.config || {};
  currentState = stateResponse.state || {};
  updateRuntimeChip();
  renderNav();
}

async function refreshRuntimeLight() {
  if (authLocked()) return;
  try {
    const stateResponse = await api("/api/runtime/state");
    currentState = stateResponse.state || {};
    updateRuntimeChip();
    if (currentPage === "runtime" || currentPage === "dashboard") {
      if (!document.hidden) await setPage(currentPage);
    }
  } catch (_) {
    el.runtimeChip.className = "runtime-chip stopped";
    el.runtimeChip.innerHTML = `<span class="dot"></span><span>offline</span>`;
  }
}

function updateRuntimeChip() {
  const running = Boolean(currentState && currentState.running);
  el.runtimeChip.className = `runtime-chip ${running ? "running" : "stopped"}`;
  const label = running ? `running g${currentState.generation || 0}` : "stopped";
  el.runtimeChip.innerHTML = `<span class="dot"></span><span>${escapeHTML(label)}</span>`;
}

async function renderDashboard() {
  const dashboard = await api("/api/dashboard");
  const state = dashboard.runtime || currentState || {};
  currentState = state;
  const stats = dashboard.stats || {};
  const totals = stats.totals || {};
  const rates = dashboard.rates || {};
  const process = dashboard.process || {};
  const fastpath = dashboard.fastpath || {};
  const counts = dashboard.objectCounts || Object.fromEntries(kinds.map((kind) => [kind.key, arrayOf(currentConfig[kind.configKey]).length]));
  const udpCount = arrayOf(state.udpPipes).length;
  const tcpCount = arrayOf(state.tcpPipes).length;
  const xrayPipeCount = arrayOf(state.xrayPipes).length;
  const running = Boolean(state.running);

  el.workspace.innerHTML = `
    <div class="grid metrics">
      ${metric("Runtime", running ? "Running" : "Stopped")}
      ${metric("RX Rate", `${formatBytes(rates.rxBytesPerSecond || 0)}/s`)}
      ${metric("TX Rate", `${formatBytes(rates.txBytesPerSecond || 0)}/s`)}
      ${metric("Traffic", `${formatBytes(totals.rxBytes || 0)} / ${formatBytes(totals.txBytes || 0)}`)}
      ${metric("Drops", (totals.dropsGuard || 0) + (totals.dropsIO || 0))}
      ${metric("Objects", Object.values(counts).reduce((a, b) => a + b, 0))}
    </div>

    <div class="split">
      <section class="panel">
        <div class="panel-head">
          <h2>Runtime</h2>
          <div class="toolbar-group">
            ${button("Apply", "play", "apply", "primary")}
            ${button("Enforce", "shield", "enforce")}
            ${button("Stop", "square", "stop", "danger")}
            ${button("Refresh", "refresh", "refresh")}
          </div>
        </div>
        <div class="panel-body">
          ${runtimeSummaryHTML(state)}
        </div>
      </section>

      <section class="panel">
        <div class="panel-head"><h2>Diagnostics</h2></div>
        <div class="panel-body">
          <dl class="kv">
            <dt>UDP Pipes</dt><dd>${udpCount}</dd>
            <dt>TCP Pipes</dt><dd>${tcpCount}</dd>
            <dt>Xray Pipes</dt><dd>${xrayPipeCount}</dd>
            <dt>Fastpath ABI</dt><dd>${escapeHTML(fastpath.abi || 0)}</dd>
            <dt>Process</dt><dd>${escapeHTML(`${process.goos || ""}/${process.goarch || ""}`)}</dd>
            <dt>Heap</dt><dd>${escapeHTML(formatBytes(process.heapAlloc || 0))}</dd>
          </dl>
        </div>
      </section>
    </div>

    <div class="split">
      <section class="panel">
        <div class="panel-head"><h2>Objects</h2></div>
        <div class="panel-body">
          <dl class="kv">
            ${kinds.map((kind) => `<dt>${escapeHTML(kind.title)}</dt><dd>${escapeHTML(counts[kind.key] || 0)}</dd>`).join("")}
          </dl>
        </div>
      </section>
      ${dashboardLogsPanelHTML(dashboard.recentLogs)}
    </div>

    ${statsBucketPanelHTML("By Endpoint", stats.byEndpoint)}
    ${statsBucketPanelHTML("By Client", stats.byClient)}
    ${pipesPanelHTML("UDP Pipes", state.udpPipes)}
    ${pipesPanelHTML("TCP Pipes", state.tcpPipes)}
    ${pipesPanelHTML("Xray Pipes", state.xrayPipes)}
    ${xrayRuntimesPanelHTML(state.xrayRuntimes)}
  `;
}

function renderObjects(kindKey) {
  const kind = kinds.find((entry) => entry.key === kindKey);
  if (!kind) {
    renderError(new Error(`unknown page ${kindKey}`));
    return;
  }
  const items = arrayOf(currentConfig[kind.configKey]);

  el.workspace.innerHTML = `
    <div class="toolbar">
      <div class="toolbar-group">
        ${button("New", "plus", `new:${kind.key}`, "primary")}
        ${button("Refresh", "refresh", "refresh")}
      </div>
      <div class="muted">${items.length} item${items.length === 1 ? "" : "s"}</div>
    </div>
    ${kind.key === "clients" && currentShare ? clientShareHTML(currentShare) : ""}
    <section class="panel">
      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>ID</th>
              <th>Name</th>
              <th>Status</th>
              <th>Binding</th>
              <th>Transport</th>
              <th class="actions">Actions</th>
            </tr>
          </thead>
          <tbody>
            ${items.map((item) => objectRow(kind.key, item)).join("") || emptyRow("No objects")}
          </tbody>
        </table>
      </div>
    </section>
  `;
}

function clientShareHTML(share) {
  const payload = share.payload || {};
  return `
    <section class="panel">
      <div class="panel-head">
        <h2>Client Share</h2>
        <div class="toolbar-group">
          ${button("Refresh", "refresh", `share:${share.clientId}`)}
        </div>
      </div>
      <div class="panel-body share-layout">
        <div>
          <dl class="kv">
            <dt>Client</dt><dd>${escapeHTML(share.clientId || "")}</dd>
            <dt>Type</dt><dd>${escapeHTML(share.type || "")}</dd>
          </dl>
          <textarea class="mini-editor" readonly spellcheck="false">${escapeHTML(share.link || "")}</textarea>
          ${arrayOf(share.warnings).map((item) => `<div class="warning-line">${escapeHTML(item)}</div>`).join("")}
        </div>
        <img class="qr-image" alt="Client QR" src="${escapeAttr(share.qrPng || "")}">
      </div>
      <pre class="panel-body">${escapeHTML(JSON.stringify(payload, null, 2))}</pre>
    </section>
  `;
}

function renderConfig() {
  el.workspace.innerHTML = `
    <div class="toolbar">
      <div class="toolbar-group">
        ${button("Save", "save", "save-config", "primary")}
        ${button("Validate Save", "check", "validate-save")}
        ${button("Validate Apply", "shield", "validate-apply")}
        ${button("Runtime Preview", "braces", "runtime-preview")}
      </div>
      <div class="toolbar-group">
        ${button("Reload", "refresh", "refresh")}
      </div>
    </div>
    <textarea class="config-editor" id="configEditor" spellcheck="false">${escapeHTML(JSON.stringify(currentConfig, null, 2))}</textarea>
    <section class="panel">
      <div class="panel-head"><h2>Output</h2></div>
      <pre class="panel-body" id="configOutput"></pre>
    </section>
  `;
}

function renderRuntime() {
  el.workspace.innerHTML = `
    <div class="toolbar">
      <div class="toolbar-group">
        ${button("Apply", "play", "apply", "primary")}
        ${button("Enforce", "shield", "enforce")}
        ${button("Stop", "square", "stop", "danger")}
        ${button("Refresh", "refresh", "refresh")}
      </div>
      <div class="muted">generation ${currentState.generation || 0}</div>
    </div>
    <section class="panel">
      <div class="panel-head"><h2>State</h2></div>
      <div class="panel-body">${runtimeSummaryHTML(currentState)}</div>
    </section>
    ${pipesPanelHTML("UDP Pipes", currentState.udpPipes)}
    ${pipesPanelHTML("TCP Pipes", currentState.tcpPipes)}
    ${pipesPanelHTML("Xray Pipes", currentState.xrayPipes)}
    ${xrayRuntimesPanelHTML(currentState.xrayRuntimes)}
  `;
}

async function renderStats() {
  const report = await api("/api/stats");
  const totals = report.totals || {};
  const clients = arrayOf(report.clients);
  el.workspace.innerHTML = `
    <div class="toolbar">
      <div class="toolbar-group">
        ${button("Refresh", "refresh", "refresh")}
      </div>
      <div class="muted">${escapeHTML(report.generatedAt || "")}</div>
    </div>
    <div class="grid metrics">
      ${metric("RX Bytes", formatBytes(totals.rxBytes || 0))}
      ${metric("TX Bytes", formatBytes(totals.txBytes || 0))}
      ${metric("Guard Drops", totals.dropsGuard || 0)}
      ${metric("IO Drops", totals.dropsIO || 0)}
    </div>
    ${statsBucketPanelHTML("By Endpoint", report.byEndpoint)}
    ${statsBucketPanelHTML("By Device", report.byDevice)}
    ${statsBucketPanelHTML("By Route", report.byRoute)}
    ${statsBucketPanelHTML("By Client", report.byClient)}
    <section class="panel">
      <div class="panel-head"><h2>Client Quota</h2></div>
      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>ID</th>
              <th>Name</th>
              <th>Status</th>
              <th>Used</th>
              <th>Cap</th>
              <th>Remaining</th>
              <th>Pipes</th>
            </tr>
          </thead>
          <tbody>
            ${clients.map(clientQuotaRowHTML).join("") || `<tr><td colspan="7" class="muted">none</td></tr>`}
          </tbody>
        </table>
      </div>
    </section>
  `;
}

async function renderLogs() {
  const response = await api("/api/logs");
  const events = arrayOf(response.events);
  el.workspace.innerHTML = `
    <div class="toolbar">
      <div class="toolbar-group">
        ${button("Refresh", "refresh", "refresh")}
        ${button("Clear", "trash", "clear-logs", "danger")}
      </div>
      <div class="muted">${events.length} event${events.length === 1 ? "" : "s"}</div>
    </div>
    <section class="panel">
      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Seq</th>
              <th>Time</th>
              <th>Level</th>
              <th>Action</th>
              <th>Message</th>
            </tr>
          </thead>
          <tbody>
            ${events.map((event) => `
              <tr>
                <td class="id-cell">${escapeHTML(event.seq || "")}</td>
                <td>${escapeHTML(event.time || "")}</td>
                <td><span class="status ${event.level === "error" ? "disabled" : "enabled"}">${escapeHTML(event.level || "")}</span></td>
                <td class="id-cell">${escapeHTML(event.action || "")}</td>
                <td>${escapeHTML(event.message || "")}</td>
              </tr>
            `).join("") || `<tr><td colspan="5" class="muted">none</td></tr>`}
          </tbody>
        </table>
      </div>
    </section>
  `;
}

function renderBackup() {
  el.workspace.innerHTML = `
    <div class="toolbar">
      <div class="toolbar-group">
        ${button("Generate", "braces", "backup-generate", "primary")}
        ${button("Download", "download", "backup-download")}
        ${button("Restore", "upload", "backup-restore", "danger")}
      </div>
      <div class="muted">JSON backup document</div>
    </div>
    <textarea class="config-editor" id="backupEditor" spellcheck="false"></textarea>
    <section class="panel">
      <div class="panel-head"><h2>Output</h2></div>
      <pre class="panel-body" id="backupOutput"></pre>
    </section>
  `;
}

async function renderDiagnostics() {
  const report = await api("/api/diagnostics");
  const counts = report.objectCounts || {};
  const process = report.process || {};
  const fastpath = report.fastpath || {};
  const openwrt = report.openwrt || {};
  el.workspace.innerHTML = `
    <div class="toolbar">
      <div class="toolbar-group">
        ${button("Refresh", "refresh", "refresh")}
      </div>
      <div class="muted">${escapeHTML(report.generatedAt || "")}</div>
    </div>
    <div class="grid metrics">
      ${metric("Version", report.version || "dev")}
      ${metric("Fastpath ABI", fastpath.abi ?? 0)}
      ${metric("Runtime", report.runtime && report.runtime.running ? "Running" : "Stopped")}
      ${metric("OpenWrt", openwrt.currentBuildTarget || "x86-64")}
    </div>
    <div class="split">
      <section class="panel">
        <div class="panel-head"><h2>Process</h2></div>
        <div class="panel-body">
          <dl class="kv">
            <dt>Started</dt><dd>${escapeHTML(process.startedAt || "")}</dd>
            <dt>Uptime</dt><dd>${escapeHTML(String(process.uptimeSecond || 0))}s</dd>
            <dt>Go</dt><dd>${escapeHTML(process.goVersion || "")}</dd>
            <dt>Platform</dt><dd>${escapeHTML(`${process.goos || ""}/${process.goarch || ""}`)}</dd>
            <dt>Goroutines</dt><dd>${escapeHTML(process.goroutines || 0)}</dd>
            <dt>Heap</dt><dd>${escapeHTML(formatBytes(process.heapAlloc || 0))} / ${escapeHTML(formatBytes(process.heapSys || 0))}</dd>
          </dl>
        </div>
      </section>
      <section class="panel">
        <div class="panel-head"><h2>Objects</h2></div>
        <div class="panel-body">
          <dl class="kv">
            ${kinds.map((kind) => `<dt>${escapeHTML(kind.title)}</dt><dd>${escapeHTML(counts[kind.key] || 0)}</dd>`).join("")}
          </dl>
        </div>
      </section>
    </div>
    <section class="panel">
      <div class="panel-head"><h2>Report JSON</h2></div>
      <pre class="panel-body">${escapeHTML(JSON.stringify(report, null, 2))}</pre>
    </section>
  `;
}

async function renderXrayBinary() {
  const defaultPath = xrayBinaryDefaultPath();
  if (!currentXrayBinary && defaultPath) {
    try {
      const result = await api(`/api/xray/external/status?path=${encodeURIComponent(defaultPath)}`);
      currentXrayBinary = result.binary;
    } catch (_) {
      currentXrayBinary = null;
    }
  }
  el.workspace.innerHTML = `
    <div class="toolbar">
      <div class="toolbar-group">
        ${button("Status", "refresh", "xray-binary-status")}
        ${button("Upload", "upload", "xray-binary-upload", "primary")}
        ${button("Download", "download", "xray-binary-download")}
      </div>
      <div class="muted">${escapeHTML(defaultPath || "no path")}</div>
    </div>
    <section class="panel">
      <div class="panel-head"><h2>External Xray Binary</h2></div>
      <div class="panel-body">
        <div class="field-grid">
          <div class="field wide">
            <label for="xrayBinaryPath">Path</label>
            <input id="xrayBinaryPath" value="${escapeAttr((currentXrayBinary && currentXrayBinary.path) || defaultPath)}">
          </div>
          <div class="field wide">
            <label for="xrayBinaryURL">Download URL</label>
            <input id="xrayBinaryURL" placeholder="https://example.com/xray-linux-amd64">
          </div>
          <div class="field wide">
            <label for="xrayBinaryFile">Upload File</label>
            <input id="xrayBinaryFile" type="file">
          </div>
        </div>
      </div>
    </section>
    <section class="panel">
      <div class="panel-head"><h2>Status</h2></div>
      <pre class="panel-body" id="xrayBinaryOutput">${escapeHTML(JSON.stringify(currentXrayBinary || {}, null, 2))}</pre>
    </section>
  `;
}

function renderTemplates() {
  const outputA = currentTemplate ? JSON.stringify(currentTemplate.a, null, 2) : "";
  const outputB = currentTemplate ? JSON.stringify(currentTemplate.b, null, 2) : "";
  el.workspace.innerHTML = `
    <section class="panel">
      <div class="panel-head">
        <h2>Raw Pair</h2>
        <div class="toolbar-group">
          ${button("Generate", "play", "template-generate", "primary")}
          ${button("Load A", "upload", "template-load-a")}
          ${button("Load B", "upload", "template-load-b")}
          ${button("Download A", "download", "template-download-a")}
          ${button("Download B", "download", "template-download-b")}
        </div>
      </div>
      <div class="panel-body">
        <div class="field-grid">
          ${templateField("templateTransport", "Transport", "select", "udp")}
          ${templateField("templatePort", "Port", "number", "46000")}
          ${templateField("templateHostA", "Host A", "text", "")}
          ${templateField("templateHostB", "Host B", "text", "")}
          ${templateField("templateTunA", "TUN A", "text", "10.77.0.1/30")}
          ${templateField("templateTunB", "TUN B", "text", "10.77.0.2/30")}
          ${templateField("templateIfNameA", "IfName A", "text", "tapxudp0")}
          ${templateField("templateIfNameB", "IfName B", "text", "tapxudp0")}
          ${templateField("templateMTU", "MTU", "number", "1400")}
          ${templateField("templateVKey", "vKey", "text", "")}
        </div>
      </div>
    </section>
    <div class="grid template-outputs">
      <section class="panel">
        <div class="panel-head"><h2>Side A</h2></div>
        <textarea class="config-editor" id="templateOutputA" spellcheck="false">${escapeHTML(outputA)}</textarea>
      </section>
      <section class="panel">
        <div class="panel-head"><h2>Side B</h2></div>
        <textarea class="config-editor" id="templateOutputB" spellcheck="false">${escapeHTML(outputB)}</textarea>
      </section>
    </div>
  `;
}

function templateField(id, label, type, value) {
  if (type === "select") {
    return `
      <div class="field">
        <label>${escapeHTML(label)}</label>
        <select id="${escapeAttr(id)}">
          <option value="udp" ${value === "udp" ? "selected" : ""}>udp</option>
          <option value="tcp" ${value === "tcp" ? "selected" : ""}>tcp</option>
        </select>
      </div>
    `;
  }
  return `
    <div class="field">
      <label>${escapeHTML(label)}</label>
      <input id="${escapeAttr(id)}" type="${type === "number" ? "number" : "text"}" value="${escapeAttr(value)}">
    </div>
  `;
}

function metric(label, value) {
  return `
    <section class="panel metric">
      <div class="metric-value">${escapeHTML(String(value))}</div>
      <div class="metric-label">${escapeHTML(label)}</div>
    </section>
  `;
}

function objectRow(kind, item) {
  const id = item.ID || "";
  const enabled = Boolean(item.Enabled);
  const binding = bindingLabel(item);
  const transport = item.Transport || item.Type || "";
  return `
    <tr>
      <td class="id-cell">${escapeHTML(id)}</td>
      <td>${escapeHTML(item.Name || item.Email || item.Value || item.IfName || item.Remote || item.Remark || "")}</td>
      <td><span class="status ${enabled ? "enabled" : "disabled"}">${enabled ? "enabled" : "disabled"}</span></td>
      <td>${escapeHTML(binding)}</td>
      <td>${escapeHTML(transport)}</td>
      <td class="actions">
        ${kind === "clients" ? `<button class="icon-button" data-command="share:${escapeAttr(id)}" title="Share" aria-label="Share">${icon("copy")}</button>` : ""}
        ${kind === "clients" ? `<button class="icon-button" data-command="reset-traffic:${escapeAttr(id)}" title="Reset Traffic" aria-label="Reset Traffic">${icon("refresh")}</button>` : ""}
        <button class="icon-button" data-edit data-kind="${escapeAttr(kind)}" data-id="${escapeAttr(id)}" title="Edit" aria-label="Edit">${icon("edit")}</button>
        <button class="icon-button" data-delete data-kind="${escapeAttr(kind)}" data-id="${escapeAttr(id)}" title="Delete" aria-label="Delete">${icon("trash")}</button>
      </td>
    </tr>
  `;
}

function bindingLabel(item) {
  const parts = [];
  const binding = item.Binding || {};
  for (const key of ["RouteID", "DeviceID", "ConnectorID", "ClientID", "VKeyID", "AddressID"]) {
    if (binding[key]) parts.push(`${key}:${binding[key]}`);
  }
  for (const key of ["DeviceID", "ListenerID", "ConnectorID", "ClientID", "VKeyID", "AddressID"]) {
    if (item[key]) parts.push(`${key}:${item[key]}`);
  }
  return parts.join(" ");
}

function emptyRow(message) {
  return `<tr><td colspan="6" class="muted">${escapeHTML(message)}</td></tr>`;
}

function runtimeSummaryHTML(state) {
  const safe = state || {};
  return `
    <dl class="kv">
      <dt>Status</dt><dd>${safe.running ? "running" : "stopped"}</dd>
      <dt>Generation</dt><dd>${safe.generation || 0}</dd>
      <dt>Started</dt><dd>${escapeHTML(safe.startedAt || "")}</dd>
      <dt>Stopped</dt><dd>${escapeHTML(safe.stoppedAt || "")}</dd>
      <dt>Applied</dt><dd>${escapeHTML(safe.lastAppliedAt || "")}</dd>
      <dt>Reload Mode</dt><dd>${escapeHTML(safe.lastReloadMode || "")}</dd>
      <dt>Rollback</dt><dd>${escapeHTML(safe.lastRollbackAt || "")}</dd>
      <dt>Enforced</dt><dd>${escapeHTML(safe.lastEnforcedAt || "")}</dd>
      <dt>Rollback Error</dt><dd>${escapeHTML(safe.lastRollbackError || "")}</dd>
      <dt>Last Error</dt><dd>${escapeHTML(safe.lastError || "")}</dd>
    </dl>
  `;
}

function dashboardLogsPanelHTML(events) {
  const rows = arrayOf(events).map((event) => `
    <tr>
      <td>${escapeHTML(event.time || "")}</td>
      <td><span class="status ${event.level === "error" ? "disabled" : "enabled"}">${escapeHTML(event.level || "")}</span></td>
      <td class="id-cell">${escapeHTML(event.action || "")}</td>
      <td>${escapeHTML(event.message || "")}</td>
    </tr>
  `).join("");
  return `
    <section class="panel">
      <div class="panel-head"><h2>Recent Logs</h2></div>
      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Time</th>
              <th>Level</th>
              <th>Action</th>
              <th>Message</th>
            </tr>
          </thead>
          <tbody>${rows || `<tr><td colspan="4" class="muted">none</td></tr>`}</tbody>
        </table>
      </div>
    </section>
  `;
}

function pipesPanelHTML(title, pipes) {
  const rows = arrayOf(pipes).map((pipe) => {
    const counters = pipe.counters || {};
    return `
      <tr>
        <td class="id-cell">${escapeHTML(pipe.endpointId || "")}</td>
        <td>${escapeHTML(pipe.endpointKind || "")}</td>
        <td>${escapeHTML(pipe.deviceId || "")}</td>
        <td>${escapeHTML(pipe.deviceName || "")}</td>
        <td>${escapeHTML(pipe.localAddr || "")}</td>
        <td>${escapeHTML(pipe.remoteAddr || "")}</td>
        <td>${counterText(counters)}</td>
      </tr>
    `;
  }).join("");
  return `
    <section class="panel">
      <div class="panel-head"><h2>${escapeHTML(title)}</h2></div>
      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Endpoint</th>
              <th>Kind</th>
              <th>Device</th>
              <th>Interface</th>
              <th>Local</th>
              <th>Remote</th>
              <th>Counters</th>
            </tr>
          </thead>
          <tbody>${rows || `<tr><td colspan="7" class="muted">none</td></tr>`}</tbody>
        </table>
      </div>
    </section>
  `;
}

function xrayRuntimesPanelHTML(runtimes) {
  const rows = arrayOf(runtimes).map((runtime) => {
    const endpoints = arrayOf(runtime.endpoints)
      .map((endpoint) => `${endpoint.kind || ""}/${endpoint.id || ""}`)
      .join(" ");
    return `
      <tr>
        <td>${escapeHTML(runtime.running ? "running" : "stopped")}</td>
        <td>${escapeHTML(runtime.runtime || "")}</td>
        <td>${escapeHTML(runtime.adapter || "")}</td>
        <td>${escapeHTML(runtime.pid || "")}</td>
        <td>${escapeHTML(runtime.endpointCount || 0)}</td>
        <td>${escapeHTML(endpoints)}</td>
        <td>${escapeHTML(runtime.configPath || "")}</td>
        <td>${escapeHTML(runtime.lastError || "")}</td>
      </tr>
    `;
  }).join("");
  return `
    <section class="panel">
      <div class="panel-head"><h2>Xray Runtimes</h2></div>
      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Status</th>
              <th>Runtime</th>
              <th>Adapter</th>
              <th>PID</th>
              <th>Endpoints</th>
              <th>Refs</th>
              <th>Config</th>
              <th>Error</th>
            </tr>
          </thead>
          <tbody>${rows || `<tr><td colspan="8" class="muted">none</td></tr>`}</tbody>
        </table>
      </div>
    </section>
  `;
}

function statsBucketPanelHTML(title, buckets) {
  const rows = arrayOf(buckets).map((bucket) => `
    <tr>
      <td class="id-cell">${escapeHTML(bucket.id || "")}</td>
      <td>${escapeHTML(bucket.name || "")}</td>
      <td>${escapeHTML(bucket.kind || bucket.endpoint || "")}</td>
      <td>${escapeHTML(bucket.pipes || 0)}</td>
      <td>${counterText(bucket.counters || {})}</td>
    </tr>
  `).join("");
  return `
    <section class="panel">
      <div class="panel-head"><h2>${escapeHTML(title)}</h2></div>
      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>ID</th>
              <th>Name</th>
              <th>Kind</th>
              <th>Pipes</th>
              <th>Counters</th>
            </tr>
          </thead>
          <tbody>${rows || `<tr><td colspan="5" class="muted">none</td></tr>`}</tbody>
        </table>
      </div>
    </section>
  `;
}

function clientQuotaRowHTML(client) {
  const flags = [];
  if (!client.enabled) flags.push("disabled");
  if (client.expired) flags.push("expired");
  if (client.overQuota) flags.push("over quota");
  if (flags.length === 0) flags.push("ok");
  const statusClass = client.overQuota || client.expired || !client.enabled ? "disabled" : "enabled";
  const cap = client.trafficCap ? formatBytes(client.trafficCap) : "unlimited";
  const remaining = client.trafficCap ? formatBytes(client.remainingBytes || 0) : "";
  return `
    <tr>
      <td class="id-cell">${escapeHTML(client.id || "")}</td>
      <td>${escapeHTML(client.name || client.email || "")}</td>
      <td><span class="status ${statusClass}">${escapeHTML(flags.join(", "))}</span></td>
      <td>${escapeHTML(formatBytes(client.usedBytes || 0))}</td>
      <td>${escapeHTML(cap)}</td>
      <td>${escapeHTML(remaining)}</td>
      <td>${escapeHTML(client.activePipes || 0)}</td>
    </tr>
  `;
}

function counterText(counters) {
  const rxp = counters.RXPackets ?? counters.rxPackets ?? 0;
  const txp = counters.TXPackets ?? counters.txPackets ?? 0;
  const rxb = counters.RXBytes ?? counters.rxBytes ?? 0;
  const txb = counters.TXBytes ?? counters.txBytes ?? 0;
  const guard = counters.DropsGuard ?? counters.dropsGuard ?? 0;
  const io = counters.DropsIO ?? counters.dropsIO ?? 0;
  return `rx ${rxp}/${rxb} tx ${txp}/${txb} drop ${guard}/${io}`;
}

async function openObjectEditor(kind, id) {
  let value;
  if (id) {
    const response = await api(`/api/objects/${kind}/${encodeURIComponent(id)}`);
    value = response.item;
  } else {
    value = templateFor(kind);
  }
  editorContext = { kind, id: id || value.ID || "" };
  el.drawerTitle.textContent = id ? `Edit ${id}` : `New ${kind}`;
  el.drawerMeta.textContent = `${kind} fields + JSON`;
  renderObjectForm(kind, value);
  el.objectEditor.value = JSON.stringify(value, null, 2);
  el.drawer.classList.add("open");
  el.drawer.setAttribute("aria-hidden", "false");
  const firstInput = el.objectForm.querySelector("input, select, textarea");
  (firstInput || el.objectEditor).focus();
}

function closeDrawer() {
  el.drawer.classList.remove("open");
  el.drawer.setAttribute("aria-hidden", "true");
  editorContext = null;
  el.objectForm.innerHTML = "";
}

async function saveObject() {
  if (!editorContext) return;
  const kind = editorContext.kind;
  let parsed;
  try {
    parsed = JSON.parse(el.objectEditor.value);
  } catch (error) {
    showToast(error.message);
    return;
  }
  const id = parsed.ID || editorContext.id || generatedID(kind);
  parsed.ID = id;
  await api(`/api/objects/${kind}/${encodeURIComponent(id)}`, {
    method: "PUT",
    body: JSON.stringify(parsed),
  });
  showToast("saved");
  closeDrawer();
  await setPage(kind || currentPage);
}

function fieldGroupsFor(kind) {
  const common = [
    { path: "ID", label: "ID" },
    { path: "Enabled", label: "Enabled", type: "checkbox" },
    { path: "Name", label: "Name" },
  ];
  const remark = { path: "Remark", label: "Remark", type: "textarea", wide: true };
  const binding = [
    { path: "Binding.RouteID", label: "Binding Route" },
    { path: "Binding.DeviceID", label: "Binding Device" },
    { path: "Binding.ConnectorID", label: "Binding Connector" },
    { path: "Binding.ClientID", label: "Binding Client" },
    { path: "Binding.VKeyID", label: "Binding vKey" },
    { path: "Binding.AddressID", label: "Binding Address" },
  ];
  const rawUDP = [
    { path: "RawUDP.PeerMode", label: "Peer Mode", type: "select", options: ["", "any", "fixed", "learn"] },
    { path: "RawUDP.FixedPeer", label: "Fixed Peer" },
    { path: "RawUDP.BindInterface", label: "Bind Interface" },
    { path: "RawUDP.BindAddress", label: "Bind Address" },
    { path: "RawUDP.ReceiveBuffer", label: "Receive Buffer", type: "number" },
    { path: "RawUDP.SendBuffer", label: "Send Buffer", type: "number" },
    { path: "RawUDP.ReuseAddr", label: "SO_REUSEADDR", type: "checkbox" },
    { path: "RawUDP.ReusePort", label: "SO_REUSEPORT", type: "checkbox" },
    { path: "RawUDP.KeepAliveSecond", label: "Keepalive Seconds", type: "number" },
    { path: "RawUDP.Workers", label: "Workers", type: "number" },
    { path: "RawUDP.QueueSize", label: "Queue Size", type: "number" },
    { path: "RawUDP.DTLS.Enabled", label: "DTLS Enabled", type: "checkbox" },
    { path: "RawUDP.DTLS.CertFile", label: "DTLS Cert File" },
    { path: "RawUDP.DTLS.KeyFile", label: "DTLS Key File" },
    { path: "RawUDP.DTLS.CAFile", label: "DTLS CA File" },
    { path: "RawUDP.DTLS.ServerName", label: "DTLS Server Name" },
    { path: "RawUDP.DTLS.ALPN", label: "DTLS ALPN", type: "array" },
    { path: "RawUDP.DTLS.MinVersion", label: "DTLS Min Version", type: "select", options: ["", "1.0", "1.1", "1.2", "1.3"] },
    { path: "RawUDP.DTLS.MaxVersion", label: "DTLS Max Version", type: "select", options: ["", "1.0", "1.1", "1.2", "1.3"] },
    { path: "RawUDP.DTLS.AllowInsecure", label: "DTLS Allow Insecure", type: "checkbox" },
    { path: "RawUDP.DTLS.MTU", label: "DTLS MTU", type: "number" },
    { path: "RawUDP.DTLS.ReplayWindow", label: "DTLS Replay Window", type: "number" },
  ];
  const rawTCP = [
    { path: "RawTCP.LengthMode", label: "Length Mode", type: "select", options: ["", "uint16", "uint32"] },
    { path: "RawTCP.BindInterface", label: "Bind Interface" },
    { path: "RawTCP.BindAddress", label: "Bind Address" },
    { path: "RawTCP.ReceiveBuffer", label: "Receive Buffer", type: "number" },
    { path: "RawTCP.SendBuffer", label: "Send Buffer", type: "number" },
    { path: "RawTCP.NoDelay", label: "TCP_NODELAY", type: "checkbox" },
    { path: "RawTCP.KeepAliveSecond", label: "Keepalive Seconds", type: "number" },
    { path: "RawTCP.FastOpen", label: "TCP Fast Open", type: "checkbox" },
    { path: "RawTCP.ConnectTimeout", label: "Connect Timeout", type: "number" },
    { path: "RawTCP.ReconnectSecond", label: "Reconnect Seconds", type: "number" },
    { path: "RawTCP.Workers", label: "Workers", type: "number" },
    { path: "RawTCP.ReadBuffer", label: "Read Buffer", type: "number" },
    { path: "RawTCP.WriteBuffer", label: "Write Buffer", type: "number" },
    { path: "RawTCP.TLS.Enabled", label: "TLS Enabled", type: "checkbox" },
    { path: "RawTCP.TLS.CertFile", label: "TLS Cert File" },
    { path: "RawTCP.TLS.KeyFile", label: "TLS Key File" },
    { path: "RawTCP.TLS.CAFile", label: "TLS CA File" },
    { path: "RawTCP.TLS.ServerName", label: "TLS Server Name" },
    { path: "RawTCP.TLS.ALPN", label: "TLS ALPN", type: "array" },
    { path: "RawTCP.TLS.MinVersion", label: "TLS Min Version", type: "select", options: ["", "1.0", "1.1", "1.2", "1.3"] },
    { path: "RawTCP.TLS.MaxVersion", label: "TLS Max Version", type: "select", options: ["", "1.0", "1.1", "1.2", "1.3"] },
    { path: "RawTCP.TLS.AllowInsecure", label: "TLS Allow Insecure", type: "checkbox" },
  ];

  if (kind === "devices") {
    return [
      { title: "Device", fields: [
        ...common,
        { path: "Type", label: "Type", type: "select", options: ["tun", "tap"] },
        { path: "IfName", label: "Interface Name" },
        { path: "MTU", label: "MTU", type: "number" },
        { path: "MSSClamp", label: "MSS Clamp", type: "number" },
        { path: "IPv4CIDR", label: "IPv4 CIDR" },
        { path: "IPv6CIDR", label: "IPv6 CIDR" },
        remark,
      ] },
      { title: "Bridge", fields: [
        { path: "Bridge.Enabled", label: "Enabled", type: "checkbox" },
        { path: "Bridge.Name", label: "Bridge Name" },
        { path: "Bridge.IfName", label: "Member Interface" },
        { path: "Bridge.MTU", label: "Bridge MTU", type: "number" },
      ] },
      { title: "Routes", fields: [
        { path: "Routes", label: "Static Routes", type: "json", wide: true },
      ] },
      { title: "DNS", fields: [
        { path: "DNS", label: "DNS Config", type: "json", wide: true },
      ] },
    ];
  }
  if (kind === "listeners") {
    return [
      { title: "Listener", fields: [
        ...common,
        { path: "BindHost", label: "Bind Host" },
        { path: "BindPort", label: "Bind Port", type: "number" },
        { path: "Transport", label: "Transport", type: "select", options: ["udp", "tcp", "xray"] },
        { path: "XrayProfileID", label: "Xray Profile" },
        remark,
      ] },
      { title: "Binding", fields: binding },
      { title: "Raw UDP", fields: rawUDP },
      { title: "Raw TCP", fields: rawTCP },
    ];
  }
  if (kind === "connectors") {
    return [
      { title: "Connector", fields: [
        ...common,
        { path: "Remote", label: "Remote" },
        { path: "Port", label: "Port", type: "number" },
        { path: "Transport", label: "Transport", type: "select", options: ["udp", "tcp", "xray"] },
        { path: "XrayProfileID", label: "Xray Profile" },
        remark,
      ] },
      { title: "Binding", fields: binding },
      { title: "Raw UDP", fields: rawUDP },
      { title: "Raw TCP", fields: rawTCP },
    ];
  }
  if (kind === "clients") {
    return [
      { title: "Client", fields: [
        ...common,
        { path: "Email", label: "Email" },
        { path: "ListenerID", label: "Listener" },
        { path: "CredentialType", label: "Credential Type", type: "select", options: ["", "uuid", "password", "vkey"] },
        { path: "CredentialValue", label: "Credential Value" },
        { path: "AddressID", label: "Address Limit" },
        { path: "ExpiresAt", label: "Expires At", type: "number" },
        { path: "TrafficCap", label: "Traffic Cap", type: "number" },
        { path: "TrafficResetAt", label: "Traffic Reset At", type: "number" },
        { path: "TrafficRXOffset", label: "Traffic RX Offset", type: "number" },
        { path: "TrafficTXOffset", label: "Traffic TX Offset", type: "number" },
        remark,
      ] },
      { title: "Binding", fields: binding },
    ];
  }
  if (kind === "routes") {
    return [
      { title: "Route", fields: [
        { path: "ID", label: "ID" },
        { path: "Enabled", label: "Enabled", type: "checkbox" },
        { path: "VKeyID", label: "vKey" },
        { path: "ListenerID", label: "Listener" },
        { path: "DeviceID", label: "Device" },
        { path: "ConnectorID", label: "Connector" },
        { path: "ClientID", label: "Client" },
        { path: "AddressID", label: "Address Limit" },
      ] },
    ];
  }
  if (kind === "vkeys") {
    return [
      { title: "vKey", fields: [
        ...common,
        { path: "Value", label: "Value", type: "textarea", wide: true },
        remark,
      ] },
    ];
  }
  if (kind === "xrayProfiles") {
    return [
      { title: "Xray Profile", fields: [
        ...common,
        { path: "Runtime", label: "Runtime", type: "select", options: ["", "embedded", "external"] },
        { path: "InboundProtocol", label: "Inbound Protocol" },
        { path: "OutboundProtocol", label: "Outbound Protocol" },
        { path: "Network", label: "Network" },
        { path: "Security", label: "Security" },
        remark,
      ] },
      { title: "Endpoint JSON", fields: [
        { path: "InboundSettingsJSON", label: "inbound settings", type: "textarea", wide: true },
        { path: "OutboundSettingsJSON", label: "outbound settings", type: "textarea", wide: true },
      ] },
      { title: "Advanced JSON", fields: [
        { path: "StreamSettingsJSON", label: "streamSettings", type: "textarea", wide: true },
        { path: "SniffingJSON", label: "sniffing", type: "textarea", wide: true },
        { path: "MuxJSON", label: "mux", type: "textarea", wide: true },
        { path: "SockoptJSON", label: "sockopt", type: "textarea", wide: true },
        { path: "FallbacksJSON", label: "fallbacks", type: "textarea", wide: true },
        { path: "RoutingJSON", label: "routing", type: "textarea", wide: true },
        { path: "DNSJSON", label: "dns", type: "textarea", wide: true },
        { path: "PolicyJSON", label: "policy", type: "textarea", wide: true },
        { path: "AdvancedJSON", label: "advanced JSON", type: "textarea", wide: true },
      ] },
    ];
  }
  if (kind === "addresses") {
    return [
      { title: "Address Limit", fields: [
        ...common,
        { path: "DeviceID", label: "Device" },
        { path: "ClientID", label: "Client" },
        { path: "MACs", label: "MACs", type: "array", wide: true },
        { path: "IPv4CIDRs", label: "IPv4 CIDRs", type: "array", wide: true },
        { path: "IPv6CIDRs", label: "IPv6 CIDRs", type: "array", wide: true },
        { path: "IPv4Gateway", label: "IPv4 Gateway" },
        { path: "IPv6Gateway", label: "IPv6 Gateway" },
        { path: "DNS", label: "DNS", type: "array", wide: true },
        { path: "Routes", label: "Pushed Routes", type: "array", wide: true },
        { path: "AllowDefaultRoute", label: "Allow Default Route", type: "checkbox" },
        remark,
      ] },
    ];
  }
  if (kind === "settings") {
    return [
      { title: "Panel", fields: [
        ...common,
        { path: "PanelListen", label: "Panel Listen" },
        { path: "PanelHTTPS", label: "Panel HTTPS", type: "checkbox" },
        { path: "PanelCertFile", label: "Panel Cert File" },
        { path: "PanelKeyFile", label: "Panel Key File" },
        { path: "PanelAuthEnabled", label: "Panel Auth", type: "checkbox" },
        { path: "AdminUsername", label: "Admin Username" },
        { path: "AdminPasswordHash", label: "Admin Password Hash", type: "textarea", wide: true },
        { path: "SessionTTLSecond", label: "Session TTL", type: "number" },
        remark,
      ] },
      { title: "Runtime", fields: [
        { path: "ExternalXrayPath", label: "External Xray Path" },
        { path: "LogLevel", label: "Log Level", type: "select", options: ["", "debug", "info", "warn", "error"] },
        { path: "StatsIntervalSecond", label: "Stats Interval", type: "number" },
        { path: "BackupDir", label: "Backup Dir" },
        { path: "DataDir", label: "Data Dir" },
        { path: "OpenWrtBuildTarget", label: "OpenWrt Build Target", type: "select", options: ["", "x86-64"] },
        { path: "AdvancedJSON", label: "advanced JSON", type: "textarea", wide: true },
      ] },
    ];
  }
  return [{ title: "Object", fields: [{ path: "ID", label: "ID" }, { path: "Enabled", label: "Enabled", type: "checkbox" }] }];
}

function renderObjectForm(kind, value) {
  const groups = fieldGroupsFor(kind);
  el.objectForm.innerHTML = groups.map((group) => `
    <section class="editor-section">
      <h3>${escapeHTML(group.title)}</h3>
      <div class="field-grid">
        ${group.fields.map((field) => fieldHTML(field, value)).join("")}
      </div>
    </section>
  `).join("");
}

function fieldHTML(field, value) {
  const type = field.type || "text";
  const current = getPath(value, field.path);
  const cls = `field ${field.wide || type === "array" || type === "json" ? "wide" : ""} ${type === "checkbox" ? "checkbox" : ""}`.trim();
  const data = `data-path="${escapeAttr(field.path)}" data-type="${escapeAttr(type)}"`;
  if (type === "checkbox") {
    return `
      <div class="${cls}">
        <label>${escapeHTML(field.label)}</label>
        <span class="checkbox-row">
          <input type="checkbox" ${data} ${current ? "checked" : ""}>
          <span>${escapeHTML(field.label)}</span>
        </span>
      </div>
    `;
  }
  if (type === "select") {
    const selected = current == null ? "" : String(current);
    return `
      <div class="${cls}">
        <label>${escapeHTML(field.label)}</label>
        <select ${data}>
          ${(field.options || []).map((option) => `<option value="${escapeAttr(option)}" ${String(option) === selected ? "selected" : ""}>${escapeHTML(option || "default")}</option>`).join("")}
        </select>
      </div>
    `;
  }
  if (type === "textarea" || type === "array" || type === "json") {
    let text = current == null ? "" : String(current);
    if (type === "array") text = arrayOf(current).join("\n");
    if (type === "json") text = JSON.stringify(current == null ? [] : current, null, 2);
    return `
      <div class="${cls}">
        <label>${escapeHTML(field.label)}</label>
        <textarea ${data} spellcheck="false">${escapeHTML(text)}</textarea>
      </div>
    `;
  }
  const inputType = type === "number" ? "number" : "text";
  const inputValue = current == null ? "" : String(current);
  return `
    <div class="${cls}">
      <label>${escapeHTML(field.label)}</label>
      <input type="${inputType}" ${data} value="${escapeAttr(inputValue)}">
    </div>
  `;
}

function syncJSONFromForm() {
  if (!editorContext) return;
  try {
    const value = objectFromForm();
    el.objectEditor.value = JSON.stringify(value, null, 2);
  } catch (error) {
    showToast(error.message);
  }
}

function syncFormFromJSON() {
  if (!editorContext) return;
  try {
    const value = JSON.parse(el.objectEditor.value);
    renderObjectForm(editorContext.kind, value);
  } catch (_) {
    return;
  }
}

function objectFromForm() {
  let value = {};
  if (el.objectEditor.value.trim()) {
    value = JSON.parse(el.objectEditor.value);
  }
  for (const input of el.objectForm.querySelectorAll("[data-path]")) {
    const type = input.dataset.type || "text";
    let next;
    if (type === "checkbox") {
      next = input.checked;
    } else if (type === "number") {
      next = input.value === "" ? 0 : Number(input.value);
      if (Number.isNaN(next)) throw new Error(`${input.dataset.path} must be a number`);
    } else if (type === "array") {
      next = input.value.split(/\r?\n/).map((line) => line.trim()).filter(Boolean);
    } else if (type === "json") {
      next = input.value.trim() ? JSON.parse(input.value) : [];
    } else {
      next = input.value;
    }
    setPath(value, input.dataset.path, next);
  }
  normalizeObject(value);
  return value;
}

function normalizeObject(value) {
  if (value.Binding && Object.values(value.Binding).every((item) => item === "" || item == null)) {
    value.Binding = {};
  }
  if (value.Bridge) {
    const bridgeEmpty = !value.Bridge.Enabled && !value.Bridge.Name && !value.Bridge.IfName && !value.Bridge.MTU;
    if (bridgeEmpty) value.Bridge = null;
  }
  if (value.DNS) {
    const dnsEmpty = !value.DNS.Enabled && arrayOf(value.DNS.Nameservers).length === 0 && arrayOf(value.DNS.SearchDomains).length === 0 && arrayOf(value.DNS.Options).length === 0 && !value.DNS.OutputPath;
    if (dnsEmpty) value.DNS = null;
  }
}

function getPath(value, path) {
  return path.split(".").reduce((current, part) => {
    if (current == null || typeof current !== "object") return undefined;
    return current[part];
  }, value);
}

function setPath(value, path, next) {
  const parts = path.split(".");
  let current = value;
  for (let i = 0; i < parts.length - 1; i += 1) {
    const part = parts[i];
    if (current[part] == null || typeof current[part] !== "object") {
      current[part] = {};
    }
    current = current[part];
  }
  current[parts[parts.length - 1]] = next;
}

async function deleteObject(kind, id) {
  if (!id || !window.confirm(`Delete ${id}?`)) return;
  await api(`/api/objects/${kind}/${encodeURIComponent(id)}`, { method: "DELETE" });
  showToast("deleted");
  await setPage(kind);
}

async function runCommand(command) {
  if (command.startsWith("new:")) {
    await openObjectEditor(command.slice(4), "");
    return;
  }
  if (command === "refresh") {
    await setPage(currentPage);
    return;
  }
  if (command.startsWith("share:")) {
    const id = command.slice(6);
    const result = await api(`/api/share/clients/${encodeURIComponent(id)}`);
    currentShare = result.share;
    currentPage = "clients";
    markActiveNav();
    el.pageTitle.textContent = "Clients";
    el.pageCrumb.textContent = "Identity and limits";
    renderObjects("clients");
    showToast("share generated");
    return;
  }
  if (command.startsWith("reset-traffic:")) {
    const id = command.slice("reset-traffic:".length);
    if (!window.confirm(`Reset traffic for ${id}?`)) return;
    await api(`/api/clients/${encodeURIComponent(id)}/traffic/reset`, { method: "POST" });
    showToast("traffic reset");
    await setPage(currentPage);
    return;
  }
  if (command === "xray-binary-status") {
    const path = xrayBinaryPathValue();
    const result = await api(`/api/xray/external/status?path=${encodeURIComponent(path)}`);
    currentXrayBinary = result.binary;
    await renderXrayBinary();
    showToast("status refreshed");
    return;
  }
  if (command === "xray-binary-upload") {
    const path = xrayBinaryPathValue();
    const fileInput = document.getElementById("xrayBinaryFile");
    const file = fileInput && fileInput.files ? fileInput.files[0] : null;
    if (!file) throw new Error("upload file is required");
    const form = new FormData();
    form.append("file", file);
    const result = await api(`/api/xray/external/upload?path=${encodeURIComponent(path)}`, { method: "POST", body: form });
    currentXrayBinary = result.binary;
    await renderXrayBinary();
    showToast("binary uploaded");
    return;
  }
  if (command === "xray-binary-download") {
    const path = xrayBinaryPathValue();
    const urlInput = document.getElementById("xrayBinaryURL");
    const url = urlInput ? urlInput.value.trim() : "";
    if (!url) throw new Error("download URL is required");
    const result = await api("/api/xray/external/download", { method: "POST", body: JSON.stringify({ path, url }) });
    currentXrayBinary = result.binary;
    await renderXrayBinary();
    showToast("binary downloaded");
    return;
  }
  if (command === "template-generate") {
    currentTemplate = await fetchRawPairTemplate();
    renderTemplates();
    showToast("template generated");
    return;
  }
  if (command === "template-load-a" || command === "template-load-b") {
    const side = command.endsWith("-a") ? "a" : "b";
    const cfg = templateConfig(side);
    currentConfig = cfg;
    currentPage = "config";
    markActiveNav();
    el.pageTitle.textContent = "Config JSON";
    el.pageCrumb.textContent = "Full object set";
    renderConfig();
    showToast(`loaded side ${side.toUpperCase()}`);
    return;
  }
  if (command === "template-download-a" || command === "template-download-b") {
    const side = command.endsWith("-a") ? "a" : "b";
    downloadJSON(`tapx-raw-${side}-${timestampForFile()}.json`, templateConfig(side));
    showToast(`downloaded side ${side.toUpperCase()}`);
    return;
  }
  if (command === "clear-logs") {
    await api("/api/logs", { method: "DELETE" });
    showToast("logs cleared");
    await setPage("logs");
    return;
  }
  if (command === "apply") {
    await api("/api/runtime/apply", { method: "POST" });
    showToast("runtime applied");
    await setPage(currentPage);
    return;
  }
  if (command === "stop") {
    await api("/api/runtime/stop", { method: "POST" });
    showToast("runtime stopped");
    await setPage(currentPage);
    return;
  }
  if (command === "enforce") {
    const result = await api("/api/runtime/enforce", { method: "POST" });
    const closed = arrayOf(result.events).reduce((sum, event) => sum + (event.closedPipes || 0), 0);
    showToast(closed ? `closed ${closed} pipe${closed === 1 ? "" : "s"}` : "no limits triggered");
    await setPage(currentPage);
    return;
  }
  if (command === "save-config") {
    const cfg = configEditorValue();
    await api("/api/config", { method: "PUT", body: JSON.stringify(cfg) });
    showToast("config saved");
    await setPage("config");
    return;
  }
  if (command === "validate-save" || command === "validate-apply") {
    const cfg = configEditorValue();
    const mode = command === "validate-save" ? "save" : "apply";
    const result = await api(`/api/config/validate?mode=${mode}`, { method: "POST", body: JSON.stringify(cfg) });
    writeConfigOutput(result);
    showToast("valid");
    return;
  }
  if (command === "runtime-preview") {
    const cfg = configEditorValue();
    const result = await api("/api/runtime", { method: "POST", body: JSON.stringify(cfg) });
    writeConfigOutput(result.runtime);
    return;
  }
  if (command === "backup-generate") {
    const backup = await api("/api/backup");
    writeBackupEditor(backup);
    writeBackupOutput({ ok: true, generated: backup.exportedAt });
    showToast("backup generated");
    return;
  }
  if (command === "backup-download") {
    const backup = await api("/api/backup");
    downloadJSON(`tapx-backup-${timestampForFile()}.json`, backup);
    writeBackupEditor(backup);
    writeBackupOutput({ ok: true, downloaded: backup.exportedAt });
    showToast("backup downloaded");
    return;
  }
  if (command === "backup-restore") {
    const editor = document.getElementById("backupEditor");
    if (!editor || !editor.value.trim()) throw new Error("backup JSON is empty");
    const backup = JSON.parse(editor.value);
    const result = await api("/api/backup/restore", { method: "POST", body: JSON.stringify(backup) });
    writeBackupOutput(result);
    showToast("backup restored");
    await refreshShared();
  }
}

async function fetchRawPairTemplate() {
  const params = new URLSearchParams();
  for (const [key, id] of Object.entries({
    transport: "templateTransport",
    hostA: "templateHostA",
    hostB: "templateHostB",
    port: "templatePort",
    tunA: "templateTunA",
    tunB: "templateTunB",
    ifNameA: "templateIfNameA",
    ifNameB: "templateIfNameB",
    mtu: "templateMTU",
    vkey: "templateVKey",
  })) {
    const input = document.getElementById(id);
    if (input && input.value !== "") params.set(key, input.value);
  }
  const result = await api(`/api/templates/raw-pair?${params.toString()}`);
  return result.template;
}

function templateConfig(side) {
  const editor = document.getElementById(side === "a" ? "templateOutputA" : "templateOutputB");
  if (editor && editor.value.trim()) return JSON.parse(editor.value);
  if (!currentTemplate) throw new Error("template is empty");
  return side === "a" ? currentTemplate.a : currentTemplate.b;
}

function configEditorValue() {
  const editor = document.getElementById("configEditor");
  if (!editor) throw new Error("config editor is not open");
  return JSON.parse(editor.value);
}

function writeConfigOutput(value) {
  const out = document.getElementById("configOutput");
  if (out) out.textContent = JSON.stringify(value, null, 2);
}

function writeBackupEditor(value) {
  const editor = document.getElementById("backupEditor");
  if (editor) editor.value = JSON.stringify(value, null, 2);
}

function writeBackupOutput(value) {
  const out = document.getElementById("backupOutput");
  if (out) out.textContent = JSON.stringify(value, null, 2);
}

function xrayBinaryPathValue() {
  const input = document.getElementById("xrayBinaryPath");
  const path = input ? input.value.trim() : xrayBinaryDefaultPath();
  if (!path) throw new Error("external xray path is required");
  return path;
}

function xrayBinaryDefaultPath() {
  const settings = arrayOf(currentConfig && currentConfig.Settings).find((item) => item && item.Enabled && item.ExternalXrayPath);
  return settings ? String(settings.ExternalXrayPath || "") : "";
}

function downloadJSON(filename, value) {
  const blob = new Blob([JSON.stringify(value, null, 2)], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(url);
}

function timestampForFile() {
  return new Date().toISOString().replaceAll(":", "").replaceAll(".", "-");
}

function formatBytes(value) {
  const bytes = Number(value) || 0;
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KiB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MiB`;
}

function templateFor(kind) {
  const id = generatedID(kind);
  const templates = {
    devices: { ID: id, Enabled: true, Name: "", Type: "tun", IfName: "tapx%d", MTU: 1500, MSSClamp: 0, IPv4CIDR: "", IPv6CIDR: "", Bridge: null, Routes: [], DNS: null, Remark: "" },
    listeners: { ID: id, Enabled: true, Name: "", BindHost: "127.0.0.1", BindPort: 40000, Transport: "udp", XrayProfileID: "", RawUDP: { PeerMode: "learn", FixedPeer: "", BindInterface: "", BindAddress: "", ReceiveBuffer: 0, SendBuffer: 0, ReuseAddr: true, ReusePort: false, KeepAliveSecond: 0, Workers: 0, QueueSize: 0, DTLS: { Enabled: false, CertFile: "", KeyFile: "", CAFile: "", ServerName: "", ALPN: [], MinVersion: "", MaxVersion: "", AllowInsecure: false, MTU: 0, ReplayWindow: 0 } }, RawTCP: { LengthMode: "uint16", BindInterface: "", BindAddress: "", ReceiveBuffer: 0, SendBuffer: 0, NoDelay: true, KeepAliveSecond: 30, FastOpen: false, ConnectTimeout: 3, ReconnectSecond: 0, Workers: 0, ReadBuffer: 0, WriteBuffer: 0, TLS: { Enabled: false, CertFile: "", KeyFile: "", CAFile: "", ServerName: "", ALPN: [], MinVersion: "", MaxVersion: "", AllowInsecure: false } }, Binding: {}, Remark: "" },
    connectors: { ID: id, Enabled: true, Name: "", Remote: "127.0.0.1", Port: 40000, Transport: "udp", XrayProfileID: "", RawUDP: { PeerMode: "fixed", FixedPeer: "", BindInterface: "", BindAddress: "", ReceiveBuffer: 0, SendBuffer: 0, ReuseAddr: true, ReusePort: false, KeepAliveSecond: 0, Workers: 0, QueueSize: 0, DTLS: { Enabled: false, CertFile: "", KeyFile: "", CAFile: "", ServerName: "", ALPN: [], MinVersion: "", MaxVersion: "", AllowInsecure: false, MTU: 0, ReplayWindow: 0 } }, RawTCP: { LengthMode: "uint16", BindInterface: "", BindAddress: "", ReceiveBuffer: 0, SendBuffer: 0, NoDelay: true, KeepAliveSecond: 30, FastOpen: false, ConnectTimeout: 3, ReconnectSecond: 0, Workers: 0, ReadBuffer: 0, WriteBuffer: 0, TLS: { Enabled: false, CertFile: "", KeyFile: "", CAFile: "", ServerName: "", ALPN: [], MinVersion: "", MaxVersion: "", AllowInsecure: false } }, Binding: {}, Remark: "" },
    clients: { ID: id, Enabled: true, Name: "", Email: "", ListenerID: "", CredentialType: "", CredentialValue: "", Binding: {}, AddressID: "", ExpiresAt: 0, TrafficCap: 0, TrafficResetAt: 0, TrafficRXOffset: 0, TrafficTXOffset: 0, Remark: "" },
    routes: { ID: id, Enabled: true, VKeyID: "", ListenerID: "", DeviceID: "", ConnectorID: "", ClientID: "", AddressID: "" },
    xrayProfiles: { ID: id, Enabled: true, Name: "", Runtime: "embedded", InboundProtocol: "", InboundSettingsJSON: "{}", OutboundProtocol: "", OutboundSettingsJSON: "{}", Network: "", Security: "", StreamSettingsJSON: "{}", SniffingJSON: "", MuxJSON: "", SockoptJSON: "", FallbacksJSON: "", RoutingJSON: "", DNSJSON: "", PolicyJSON: "", AdvancedJSON: "", Remark: "" },
    vkeys: { ID: id, Enabled: true, Name: "", Value: "", Remark: "" },
    addresses: { ID: id, Enabled: true, Name: "", DeviceID: "", ClientID: "", MACs: [], IPv4CIDRs: [], IPv6CIDRs: [], IPv4Gateway: "", IPv6Gateway: "", DNS: [], Routes: [], AllowDefaultRoute: false, Remark: "" },
    settings: { ID: id, Enabled: true, Name: "Default", PanelListen: "127.0.0.1:8080", PanelHTTPS: false, PanelCertFile: "", PanelKeyFile: "", PanelAuthEnabled: false, AdminUsername: "admin", AdminPasswordHash: "", SessionTTLSecond: 86400, ExternalXrayPath: "", LogLevel: "info", StatsIntervalSecond: 5, BackupDir: "", DataDir: "", OpenWrtBuildTarget: "x86-64", AdvancedJSON: "", Remark: "" },
  };
  return templates[kind] || { ID: id, Enabled: true };
}

function generatedID(kind) {
  return `${kind.slice(0, -1) || kind}-${Date.now().toString(36)}`;
}

async function api(path, options = {}) {
  const init = { ...options, headers: { ...(options.headers || {}) } };
  if (init.body !== undefined && !(init.body instanceof FormData)) {
    init.headers["content-type"] = "application/json";
  }
  const response = await fetch(apiURL(path), init);
  const text = await response.text();
  let data = {};
  if (text.trim()) {
    try {
      data = JSON.parse(text);
    } catch (_) {
      data = { error: text };
    }
  }
  if (!response.ok) {
    if (response.status === 401 && path !== "/api/auth/session" && path !== "/api/auth/login") {
      authSession = { authEnabled: true, authenticated: false };
      showLogin();
    }
    const message = data.error || `${response.status} ${response.statusText}`;
    const details = data.problems ? `\n${JSON.stringify(data.problems, null, 2)}` : "";
    throw new Error(message + details);
  }
  return data;
}

function detectBasePath() {
  const path = window.location.pathname || "/";
  const trimmed = path.endsWith("/") ? path.slice(0, -1) : path.replace(/\/[^/]*$/, "");
  if (!trimmed || trimmed === "/") return "";
  return trimmed;
}

function apiURL(path) {
  if (!path.startsWith("/")) return `${basePath}/${path}`;
  return `${basePath}${path}`;
}

function renderError(error) {
  el.workspace.innerHTML = `<div class="error-box">${escapeHTML(error.message || String(error))}</div>`;
  showToast(error.message || String(error));
}

function showToast(message) {
  window.clearTimeout(toastTimer);
  el.toast.textContent = message;
  el.toast.classList.add("show");
  toastTimer = window.setTimeout(() => el.toast.classList.remove("show"), 2400);
}

function button(label, iconName, command, variant = "") {
  return `<button class="button ${variant}" data-command="${escapeAttr(command)}">${icon(iconName)}<span>${escapeHTML(label)}</span></button>`;
}

function icon(name) {
  const paths = {
    activity: '<path d="M22 12h-4l-3 8-6-16-3 8H2"/>',
    braces: '<path d="M8 3H7a3 3 0 0 0-3 3v2a2 2 0 0 1-2 2 2 2 0 0 1 2 2v2a3 3 0 0 0 3 3h1M16 21h1a3 3 0 0 0 3-3v-2a2 2 0 0 1 2-2 2 2 0 0 1-2-2v-2a3 3 0 0 0-3-3h-1"/>',
    check: '<path d="m20 6-11 11-5-5"/>',
    copy: '<rect x="9" y="9" width="13" height="13" rx="2"/><rect x="2" y="2" width="13" height="13" rx="2"/>',
    cpu: '<rect x="7" y="7" width="10" height="10" rx="2"/><path d="M9 1v3M15 1v3M9 20v3M15 20v3M20 9h3M20 14h3M1 9h3M1 14h3"/>',
    edit: '<path d="M12 20h9"/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z"/>',
    gauge: '<path d="M12 14l4-4"/><path d="M3.3 18a10 10 0 1 1 17.4 0"/><path d="M7 18h10"/>',
    key: '<path d="M21 2l-2 2M15 8l-4 4"/><circle cx="7" cy="15" r="4"/><path d="M10 12l9-9 2 2-9 9"/>',
    download: '<path d="M12 3v12"/><path d="m7 10 5 5 5-5"/><path d="M5 21h14"/>',
    list: '<path d="M8 6h13M8 12h13M8 18h13"/><path d="M3 6h.01M3 12h.01M3 18h.01"/>',
    play: '<path d="M5 3v18l15-9Z"/>',
    plug: '<path d="M12 22v-5"/><path d="M9 8V2M15 8V2M6 8h12v4a6 6 0 0 1-12 0Z"/>',
    plus: '<path d="M12 5v14M5 12h14"/>',
    radio: '<path d="M4.9 19.1a10 10 0 0 1 0-14.2M7.8 16.2a6 6 0 0 1 0-8.5M16.2 7.8a6 6 0 0 1 0 8.5M19.1 4.9a10 10 0 0 1 0 14.2"/><circle cx="12" cy="12" r="2"/>',
    refresh: '<path d="M21 12a9 9 0 0 0-15-6.7L3 8M3 3v5h5M3 12a9 9 0 0 0 15 6.7l3-2.7M21 21v-5h-5"/>',
    route: '<circle cx="6" cy="19" r="3"/><circle cx="18" cy="5" r="3"/><path d="M6 16V8a3 3 0 0 1 3-3h6"/>',
    save: '<path d="M19 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11l5 5v11a2 2 0 0 1-2 2Z"/><path d="M17 21v-8H7v8M7 3v5h8"/>',
    shield: '<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10Z"/>',
    settings: '<path d="M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6Z"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.9-.3 1.7 1.7 0 0 0-1 1.6V22a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1-1.6 1.7 1.7 0 0 0-1.9.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.9 1.7 1.7 0 0 0-1.6-1H2a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.6-1 1.7 1.7 0 0 0-.3-1.9l-.1-.1A2 2 0 1 1 6.1 3l.1.1a1.7 1.7 0 0 0 1.9.3H8.2a1.7 1.7 0 0 0 1-1.6V2a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.6 1.7 1.7 0 0 0 1.9-.3l.1-.1A2 2 0 1 1 19 6.1l-.1.1a1.7 1.7 0 0 0-.3 1.9v.1a1.7 1.7 0 0 0 1.6 1H22a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.6 1Z"/>',
    square: '<rect x="6" y="6" width="12" height="12" rx="1"/>',
    terminal: '<path d="m4 17 6-6-6-6"/><path d="M12 19h8"/>',
    trash: '<path d="M3 6h18M8 6V4h8v2M6 6l1 15h10l1-15M10 11v6M14 11v6"/>',
    upload: '<path d="M12 21V9"/><path d="m17 14-5-5-5 5"/><path d="M5 3h14"/>',
    user: '<path d="M20 21a8 8 0 0 0-16 0"/><circle cx="12" cy="7" r="4"/>',
  };
  return `<svg viewBox="0 0 24 24" aria-hidden="true">${paths[name] || paths.activity}</svg>`;
}

function arrayOf(value) {
  return Array.isArray(value) ? value : [];
}

function escapeHTML(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

function escapeAttr(value) {
  return escapeHTML(value);
}
