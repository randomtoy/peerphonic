const state = {
  authorization: sessionStorage.getItem("peerphonic.authorization") || "",
  refreshTimer: 0,
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
  magnetForm: document.querySelector("#magnet-form"),
  magnetURI: document.querySelector("#magnet-uri"),
  torrentForm: document.querySelector("#torrent-form"),
  torrentFile: document.querySelector("#torrent-file"),
  importMessage: document.querySelector("#import-message"),
  imports: document.querySelector("#imports"),
  importCount: document.querySelector("#import-count"),
  userForm: document.querySelector("#user-form"),
  newUsername: document.querySelector("#new-username"),
  newPassword: document.querySelector("#new-password"),
  newUserRole: document.querySelector("#new-user-role"),
  userMessage: document.querySelector("#user-message"),
  users: document.querySelector("#users"),
  userCount: document.querySelector("#user-count"),
  downloads: document.querySelector("#downloads"),
  downloadCount: document.querySelector("#download-count"),
  transfers: document.querySelector("#transfers"),
  sources: document.querySelector("#sources"),
  sourceCount: document.querySelector("#source-count"),
  updatedAt: document.querySelector("#updated-at"),
  addSourceSection: document.querySelector("#add-source-section"),
  downloadsSection: document.querySelector("#downloads-section"),
  usersSection: document.querySelector("#users-section"),
  transfersSection: document.querySelector("#transfers-section"),
  sourcesSection: document.querySelector("#sources-section"),
};

const permissions = {
  dashboard: "dashboard.access",
  monitoring: "monitoring.view",
  sources: "sources.manage",
  users: "users.manage",
};

const permissionLabels = {
  "dashboard.access": "Dashboard",
  "monitoring.view": "Monitoring",
  "sources.manage": "Sources",
  "users.manage": "Users",
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

function renderSummary(cache, imports, downloads, transfers, sources, users, session) {
  const allowed = new Set(session.permissions || []);
  const capacity = cache.capacityBytes || 0;
  const utilization = capacity ? Math.round((cache.sizeBytes / capacity) * 100) : 0;
  const activeStreams = transfers.reduce((total, item) => total + (item.activeStreams || 0), 0);
  const activeDownloads = downloads.filter((item) => item.state === "downloading").length;
  const activeImports = imports.filter((item) => item.state === "fetching_metadata" || item.state === "scanning").length;
  const metrics = [["Account", session.username, "accent"]];
  if (allowed.has(permissions.monitoring)) metrics.push(
    ["Cache used", formatBytes(cache.sizeBytes), ""],
    ["Utilization", `${utilization}%`, ""],
    ["Active streams", activeStreams, ""],
    ["Downloads", activeDownloads, ""],
  );
  if (allowed.has(permissions.sources)) metrics.push(
    ["Imports", activeImports, ""],
    ["Torrent sources", sources.length, ""],
  );
  if (allowed.has(permissions.users)) metrics.push(["Users", users.length, ""]);
  elements.summary.innerHTML = metrics.map(([label, value, kind]) =>
    `<article class="metric ${kind}"><span class="metric-label">${label}</span><strong class="metric-value">${value}</strong></article>`
  ).join("");
}

function renderUsers(items, session) {
  elements.userCount.textContent = `${items.length} total`;
  elements.users.replaceChildren();
  const canAssign = session.role === "admin";
  elements.newUserRole.disabled = !canAssign;
  elements.users.innerHTML = items.map((item) => {
    const assigned = new Set(item.permissions || []);
    const protectedAccount = item.role === "admin" && !canAssign;
    return `<article class="source-row user-row" data-user="${escapeHTML(item.username)}">
    <div class="source-copy">
      <h3>${escapeHTML(item.username)}</h3>
      <p class="source-meta"><span>${item.role === "admin" ? "Administrator" : "User"}</span></p>
      <div class="permission-list" aria-label="Permissions for ${escapeHTML(item.username)}">
        ${Object.entries(permissionLabels).map(([permission, label]) => `<label class="permission-chip">
          <input type="checkbox" data-permission="${permission}" ${assigned.has(permission) ? "checked" : ""} ${!canAssign || item.role === "admin" ? "disabled" : ""}>
          <span>${label}</span>
        </label>`).join("")}
      </div>
    </div>
    <div class="source-actions">
      <button class="button secondary" data-user-action="password" data-username="${escapeHTML(item.username)}" ${protectedAccount ? "disabled" : ""}>Reset password</button>
      <button class="button danger" data-user-action="delete" data-username="${escapeHTML(item.username)}" ${item.username === session.username || protectedAccount ? "disabled" : ""}>Delete</button>
    </div>
  </article>`;
  }).join("");
}

function renderImports(items) {
  elements.importCount.textContent = `${items.length} total`;
  elements.imports.replaceChildren();
  if (!items.length) {
    elements.imports.append(empty("No magnet imports yet."));
    return;
  }
  elements.imports.innerHTML = items.map((item) => {
    const labels = {
      fetching_metadata: "Fetching metadata",
      scanning: "Scanning library",
      ready: "Ready",
      failed: "Failed",
    };
    return `<article class="source-row">
      <div class="source-copy">
        <h3>${escapeHTML(item.name || item.sourceId || item.id)}</h3>
        <p class="source-meta">
          <span>${escapeHTML(labels[item.state] || item.state)}</span>
          ${item.tracks ? `<span>${item.tracks} tracks</span>` : ""}
          <span>${escapeHTML(item.provider)}</span>
        </p>
        ${item.error ? `<p class="form-error">${escapeHTML(item.error)}</p>` : ""}
      </div>
      <span class="status ${item.state === "failed" ? "failed" : ""}">${escapeHTML(labels[item.state] || item.state)}</span>
    </article>`;
  }).join("");
}

function renderDownloads(items) {
  elements.downloadCount.textContent = `${items.length} total`;
  elements.downloads.replaceChildren();
  if (!items.length) {
    elements.downloads.append(empty("Play a torrent track to start caching it in the background."));
    return;
  }
  elements.downloads.innerHTML = items.map((item) => {
    const percent = item.totalBytes ? Math.min(100, Math.round((item.completedBytes / item.totalBytes) * 100)) : 0;
    const stateLabel = item.state.charAt(0).toUpperCase() + item.state.slice(1);
    return `<article class="transfer-card">
      <div class="card-title"><h3>${escapeHTML(item.name || item.trackId)}</h3><span class="status">${escapeHTML(stateLabel)}</span></div>
      <div class="progress" aria-label="${percent}% complete"><span style="width:${percent}%"></span></div>
      <div class="facts">
        <div class="fact"><span>Complete</span><strong>${percent}%</strong></div>
        <div class="fact"><span>Cached</span><strong>${formatBytes(item.completedBytes)}</strong></div>
        <div class="fact"><span>Total</span><strong>${formatBytes(item.totalBytes)}</strong></div>
      </div>
      ${item.error ? `<p class="form-error">${escapeHTML(item.error)}</p>` : ""}
    </article>`;
  }).join("");
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
    const session = await api("/api/v1/session");
    const allowed = new Set(session.permissions || []);
    const canMonitor = allowed.has(permissions.monitoring);
    const canManageSources = allowed.has(permissions.sources);
    const canManageUsers = allowed.has(permissions.users);
    const [cache, importPayload, downloadPayload, transferPayload, sourcePayload, userPayload] = await Promise.all([
      canMonitor ? api("/api/v1/cache/status") : Promise.resolve({}),
      canManageSources ? api("/api/v1/imports") : Promise.resolve({ imports: [] }),
      canMonitor ? api("/api/v1/downloads") : Promise.resolve({ downloads: [] }),
      canMonitor ? api("/api/v1/transfers") : Promise.resolve({ transfers: [] }),
      canManageSources ? api("/api/v1/torrents") : Promise.resolve({ sources: [] }),
      canManageUsers ? api("/api/v1/users") : Promise.resolve({ users: [] }),
    ]);
    const imports = importPayload.imports || [];
    const downloads = downloadPayload.downloads || [];
    const transfers = transferPayload.transfers || [];
    const sources = sourcePayload.sources || [];
    const users = userPayload.users || [];
    elements.addSourceSection.hidden = !canManageSources;
    elements.sourcesSection.hidden = !canManageSources;
    elements.downloadsSection.hidden = !canMonitor;
    elements.transfersSection.hidden = !canMonitor;
    elements.usersSection.hidden = !canManageUsers;
    renderSummary(cache, imports, downloads, transfers, sources, users, session);
    renderImports(imports);
    renderDownloads(downloads);
    renderTransfers(transfers);
    renderSources(sources);
    renderUsers(users, session);
    elements.updatedAt.textContent = `Updated ${new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}`;
    elements.loginError.textContent = "";
    showDashboard(true);
    const active = imports.some((item) => item.state === "fetching_metadata" || item.state === "scanning") ||
      downloads.some((item) => item.state === "downloading");
    window.clearTimeout(state.refreshTimer);
    if (active) state.refreshTimer = window.setTimeout(refresh, 3000);
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
  window.clearTimeout(state.refreshTimer);
  state.authorization = "";
  sessionStorage.removeItem("peerphonic.authorization");
  elements.password.value = "";
  showDashboard(false);
});

elements.magnetForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  const button = elements.magnetForm.querySelector("button[type=submit]");
  button.disabled = true;
  elements.importMessage.className = "form-message";
  elements.importMessage.textContent = "Adding magnet…";
  try {
    await api("/api/v1/torrents/magnet", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ magnet: elements.magnetURI.value }),
    });
    elements.magnetForm.reset();
    elements.importMessage.textContent = "Magnet accepted. Metadata retrieval is running in the background.";
    await refresh();
  } catch (error) {
    elements.importMessage.className = "form-message error";
    elements.importMessage.textContent = error.message;
  } finally {
    button.disabled = false;
  }
});

elements.torrentForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  const file = elements.torrentFile.files[0];
  if (!file) return;
  const button = elements.torrentForm.querySelector("button[type=submit]");
  button.disabled = true;
  elements.importMessage.className = "form-message";
  elements.importMessage.textContent = `Uploading ${file.name}…`;
  try {
    const result = await api("/api/v1/torrents", {
      method: "POST",
      headers: { "Content-Type": "application/x-bittorrent" },
      body: file,
    });
    elements.torrentForm.reset();
    elements.importMessage.textContent = `Imported ${result.name || file.name} (${result.tracks || 0} tracks).`;
    await refresh();
  } catch (error) {
    elements.importMessage.className = "form-message error";
    elements.importMessage.textContent = error.message;
  } finally {
    button.disabled = false;
  }
});

elements.userForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  const button = elements.userForm.querySelector("button[type=submit]");
  button.disabled = true;
  elements.userMessage.className = "form-message";
  elements.userMessage.textContent = "Creating user…";
  try {
    const created = await api("/api/v1/users", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        username: elements.newUsername.value,
        password: elements.newPassword.value,
        role: elements.newUserRole.value,
        permissions: [],
      }),
    });
    elements.userForm.reset();
    elements.userMessage.textContent = `Created ${created.username}.`;
    await refresh();
  } catch (error) {
    elements.userMessage.className = "form-message error";
    elements.userMessage.textContent = error.message;
  } finally {
    button.disabled = false;
  }
});

elements.users.addEventListener("click", async (event) => {
  const button = event.target.closest("button[data-user-action]");
  if (!button || button.disabled) return;
  const { userAction, username } = button.dataset;
  let options;
  if (userAction === "password") {
    const password = window.prompt(`New password for ${username} (8–72 bytes):`);
    if (password == null) return;
    options = {
      path: `/api/v1/users/${encodeURIComponent(username)}/password`,
      request: { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ password }) },
    };
  } else {
    if (!window.confirm(`Delete user “${username}”? Their personal library data will be kept.`)) return;
    options = { path: `/api/v1/users/${encodeURIComponent(username)}`, request: { method: "DELETE" } };
  }
  button.disabled = true;
  elements.userMessage.className = "form-message";
  try {
    await api(options.path, options.request);
    elements.userMessage.textContent = userAction === "password" ? `Password updated for ${username}.` : `Deleted ${username}.`;
    await refresh();
  } catch (error) {
    elements.userMessage.className = "form-message error";
    elements.userMessage.textContent = error.message;
    button.disabled = false;
  }
});

elements.users.addEventListener("change", async (event) => {
  const checkbox = event.target.closest("input[data-permission]");
  if (!checkbox) return;
  const row = checkbox.closest("article[data-user]");
  const username = row.dataset.user;
  const assigned = [...row.querySelectorAll("input[data-permission]:checked")].map((item) => item.dataset.permission);
  row.querySelectorAll("input[data-permission]").forEach((item) => { item.disabled = true; });
  elements.userMessage.className = "form-message";
  elements.userMessage.textContent = `Updating permissions for ${username}…`;
  try {
    await api(`/api/v1/users/${encodeURIComponent(username)}/permissions`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ permissions: assigned }),
    });
    elements.userMessage.textContent = `Permissions updated for ${username}.`;
    await refresh();
  } catch (error) {
    elements.userMessage.className = "form-message error";
    elements.userMessage.textContent = error.message;
    await refresh();
  }
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
