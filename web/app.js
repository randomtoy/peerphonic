const state = {
  authorization: sessionStorage.getItem("peerphonic.authorization") || "",
};

const elements = {
  loginPanel: document.querySelector("#login-panel"),
  dashboard: document.querySelector("#dashboard"),
  loginForm: document.querySelector("#login-form"),
  loginError: document.querySelector("#login-error"),
  username: document.querySelector("#username"),
  password: document.querySelector("#password"),
  connection: document.querySelector("#connection-state"),
  refresh: document.querySelector("#refresh"),
  logout: document.querySelector("#logout"),
  summary: document.querySelector("#summary"),
  transfers: document.querySelector("#transfers"),
  sources: document.querySelector("#sources"),
  sourceCount: document.querySelector("#source-count"),
  updatedAt: document.querySelector("#updated-at"),
};

function api(path, options = {}) {
  return fetch(path, {
    ...options,
    headers: { Authorization: state.authorization, ...(options.headers || {}) },
  }).then(async (response) => {
    if (!response.ok) {
      const message = (await response.text()).trim() || `${response.status} ${response.statusText}`;
      const error = new Error(message);
      error.status = response.status;
      throw error;
    }
    return response.status === 204 ? null : response.json();
  });
}

function formatBytes(value = 0) {
  const units = ["B", "KB", "MB", "GB", "TB"];
  let size = Number(value) || 0;
  let unit = 0;
  while (Math.abs(size) >= 1024 && unit < units.length - 1) {
    size /= 1024;
    unit += 1;
  }
  return `${size.toFixed(unit === 0 || size >= 10 ? 0 : 1)} ${units[unit]}`;
}

function escapeHTML(value) {
  const node = document.createElement("span");
  node.textContent = value == null ? "" : String(value);
  return node.innerHTML;
}

function empty(message) {
  const fragment = document.querySelector("#empty-template").content.cloneNode(true);
  fragment.querySelector("p").textContent = message;
  return fragment;
}

function showDashboard(visible) {
  elements.loginPanel.hidden = visible;
  elements.dashboard.hidden = !visible;
  elements.logout.hidden = !visible;
  elements.connection.textContent = visible ? "Connected" : "Not connected";
  elements.connection.classList.toggle("online", visible);
}

function renderSummary(cache, transfers, sources) {
  const capacity = cache.capacityBytes || 0;
  const utilization = capacity ? Math.round((cache.sizeBytes / capacity) * 100) : 0;
  const activeStreams = transfers.reduce((total, item) => total + (item.activeStreams || 0), 0);
  const metrics = [
    ["Cache used", formatBytes(cache.sizeBytes), "accent"],
    ["Utilization", `${utilization}%`, ""],
    ["Active streams", activeStreams, ""],
    ["Torrent sources", sources.length, ""],
  ];
  elements.summary.innerHTML = metrics.map(([label, value, kind]) =>
    `<article class="metric ${kind}"><span class="metric-label">${label}</span><strong class="metric-value">${value}</strong></article>`
  ).join("");
}

function renderTransfers(items) {
  elements.transfers.replaceChildren();
  if (!items.length) {
    elements.transfers.append(empty("No torrent has been opened since the server started."));
    return;
  }
  elements.transfers.innerHTML = items.map((item) => {
    const percent = item.totalBytes ? Math.min(100, Math.round((item.completedBytes / item.totalBytes) * 100)) : 0;
    return `<article class="transfer-card">
      <div class="card-title"><h3>${escapeHTML(item.name || item.id)}</h3><span class="status">${item.seeding ? "Seeding" : "Attached"}</span></div>
      <div class="progress" aria-label="${percent}% complete"><span style="width:${percent}%"></span></div>
      <div class="facts">
        <div class="fact"><span>Complete</span><strong>${percent}%</strong></div>
        <div class="fact"><span>Peers</span><strong>${item.activePeers || 0} / ${item.peers || 0}</strong></div>
        <div class="fact"><span>Uploaded</span><strong>${formatBytes(item.uploadedBytes)}</strong></div>
      </div>
    </article>`;
  }).join("");
}

function renderSources(items) {
  elements.sourceCount.textContent = `${items.length} total`;
  elements.sources.replaceChildren();
  if (!items.length) {
    elements.sources.append(empty("Upload a .torrent through the API to add a source."));
    return;
  }
  elements.sources.innerHTML = items.map((item) => `<article class="source-row">
    <div class="source-copy">
      <h3>${escapeHTML(item.name || item.id)}</h3>
      <p class="source-meta">
        <span>${item.tracks || 0} tracks</span>
        <span>${item.attached ? "Attached" : "Idle"}</span>
        ${item.paused ? '<span class="tag">Paused</span>' : ""}
        ${item.pinned ? '<span class="tag pinned">Pinned</span>' : ""}
      </p>
    </div>
    <div class="source-actions">
      <button class="button secondary" data-action="${item.paused ? "resume" : "pause"}" data-id="${escapeHTML(item.id)}">${item.paused ? "Resume" : "Pause"}</button>
      <button class="button secondary" data-action="${item.pinned ? "unpin" : "pin"}" data-id="${escapeHTML(item.id)}">${item.pinned ? "Unpin" : "Pin cache"}</button>
      <button class="button danger" data-action="remove" data-id="${escapeHTML(item.id)}" data-name="${escapeHTML(item.name)}">Remove</button>
    </div>
  </article>`).join("");
}

async function refresh() {
  if (!state.authorization) return;
  elements.refresh.disabled = true;
  try {
    const [cache, transferPayload, sourcePayload] = await Promise.all([
      api("/api/v1/cache/status"),
      api("/api/v1/transfers"),
      api("/api/v1/torrents"),
    ]);
    const transfers = transferPayload.transfers || [];
    const sources = sourcePayload.sources || [];
    renderSummary(cache, transfers, sources);
    renderTransfers(transfers);
    renderSources(sources);
    elements.updatedAt.textContent = `Updated ${new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}`;
    elements.loginError.textContent = "";
    showDashboard(true);
  } catch (error) {
    if (error.status === 401) {
      state.authorization = "";
      sessionStorage.removeItem("peerphonic.authorization");
      showDashboard(false);
      elements.loginError.textContent = "The username or password is incorrect.";
    } else {
      elements.connection.textContent = "Connection error";
      elements.connection.classList.remove("online");
      elements.loginError.textContent = error.message;
    }
  } finally {
    elements.refresh.disabled = false;
  }
}

elements.loginForm.addEventListener("submit", (event) => {
  event.preventDefault();
  const credentials = new TextEncoder().encode(`${elements.username.value}:${elements.password.value}`);
  let binary = "";
  credentials.forEach((byte) => { binary += String.fromCharCode(byte); });
  state.authorization = `Basic ${btoa(binary)}`;
  sessionStorage.setItem("peerphonic.authorization", state.authorization);
  refresh();
});

elements.refresh.addEventListener("click", refresh);
elements.logout.addEventListener("click", () => {
  state.authorization = "";
  sessionStorage.removeItem("peerphonic.authorization");
  elements.password.value = "";
  showDashboard(false);
});

elements.sources.addEventListener("click", async (event) => {
  const button = event.target.closest("button[data-action]");
  if (!button) return;
  const { action, id, name } = button.dataset;
  let path = `/api/v1/torrents/${encodeURIComponent(id)}/${action}`;
  let method = "POST";
  if (action === "remove") {
    if (!window.confirm(`Remove “${name || id}” from the library? Cached audio will be kept.`)) return;
    path = `/api/v1/torrents/${encodeURIComponent(id)}`;
    method = "DELETE";
  }
  button.disabled = true;
  try {
    await api(path, { method });
    await refresh();
  } catch (error) {
    window.alert(error.message);
    button.disabled = false;
  }
});

if (state.authorization) refresh();
else showDashboard(false);
