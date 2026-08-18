const state = {
  authorization: sessionStorage.getItem("peerphonic.authorization") || "",
  activePage: sessionStorage.getItem("peerphonic.activePage") || "overview",
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
  scanButton: document.querySelector("#scan-now"),
  scanState: document.querySelector("#scan-state"),
  scanSummary: document.querySelector("#scan-summary"),
  transferSettingsForm: document.querySelector("#transfer-settings-form"),
  downloadLimit: document.querySelector("#download-limit"),
  uploadLimit: document.querySelector("#upload-limit"),
  transferSettingsSummary: document.querySelector("#transfer-settings-summary"),
  transferSettingsMessage: document.querySelector("#transfer-settings-message"),
  soulseekState: document.querySelector("#soulseek-state"),
  soulseekIndicator: document.querySelector("#soulseek-indicator"),
  soulseekMessage: document.querySelector("#soulseek-message"),
  soulseekConfigured: document.querySelector("#soulseek-configured"),
  soulseekReachable: document.querySelector("#soulseek-reachable"),
  soulseekAuthenticated: document.querySelector("#soulseek-authenticated"),
  soulseekSearchForm: document.querySelector("#soulseek-search-form"),
  soulseekQuery: document.querySelector("#soulseek-query"),
  soulseekLimit: document.querySelector("#soulseek-limit"),
  soulseekSearchMessage: document.querySelector("#soulseek-search-message"),
  soulseekSearchResults: document.querySelector("#soulseek-search-results"),
  soulseekSearchCount: document.querySelector("#search-result-count"),
  updatedAt: document.querySelector("#updated-at"),
  addSourceSection: document.querySelector("#add-source-section"),
  downloadsSection: document.querySelector("#downloads-section"),
  usersSection: document.querySelector("#users-section"),
  transfersSection: document.querySelector("#transfers-section"),
  sourcesSection: document.querySelector("#sources-section"),
  navigation: document.querySelector(".sidebar-nav"),
  navItems: [...document.querySelectorAll("[data-page]")],
  pagePanels: [...document.querySelectorAll("[data-page-panel]")],
  pageEyebrow: document.querySelector("#page-eyebrow"),
  pageTitle: document.querySelector("#page-title"),
  pageDescription: document.querySelector("#page-description"),
  accountAvatar: document.querySelector("#account-avatar"),
  accountName: document.querySelector("#account-name"),
  accountRole: document.querySelector("#account-role"),
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

const pages = {
  overview: {
    eyebrow: "CONTROL ROOM",
    title: "Server overview",
    description: "A quick look at Peerphonic right now.",
  },
  sources: {
    eyebrow: "MUSIC LIBRARY",
    title: "Sources",
    description: "Connect, import and maintain the music available to your clients.",
  },
  activity: {
    eyebrow: "LIVE STATUS",
    title: "Activity",
    description: "Follow on-demand downloads, cache progress and peer transfers.",
  },
  search: {
    eyebrow: "REMOTE DISCOVERY",
    title: "Soulseek search",
    description: "Find tracks across connected peers before adding them to your library.",
  },
  users: {
    eyebrow: "ACCESS CONTROL",
    title: "Users and permissions",
    description: "Manage accounts and delegate access to individual features.",
  },
  settings: {
    eyebrow: "SERVER CONFIGURATION",
    title: "Settings",
    description: "Tune network behavior without restarting Peerphonic.",
  },
};

const mebibyte = 1024 * 1024;

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

function activatePage(requestedPage) {
  const availablePages = elements.navItems.filter((item) => !item.hidden).map((item) => item.dataset.page);
  const page = availablePages.includes(requestedPage) ? requestedPage : "overview";
  const metadata = pages[page];
  state.activePage = page;
  sessionStorage.setItem("peerphonic.activePage", page);
  elements.navItems.forEach((item) => {
    if (item.dataset.page === page) item.setAttribute("aria-current", "page");
    else item.removeAttribute("aria-current");
  });
  elements.pagePanels.forEach((panel) => { panel.hidden = panel.dataset.pagePanel !== page; });
  elements.pageEyebrow.textContent = metadata.eyebrow;
  elements.pageTitle.textContent = metadata.title;
  elements.pageDescription.textContent = metadata.description;
}

function configureNavigation(session) {
  const allowed = new Set(session.permissions || []);
  elements.navItems.forEach((item) => {
    const required = item.dataset.requiredPermission;
    item.hidden = Boolean(required && !allowed.has(required));
  });
  elements.accountName.textContent = session.username;
  elements.accountRole.textContent = session.role === "admin" ? "Administrator" : "User";
  elements.accountAvatar.textContent = (session.username || "?").charAt(0).toUpperCase();
  activatePage(state.activePage);
}

function renderSummary(cache, imports, downloads, transfers, sources, users, session) {
  const allowed = new Set(session.permissions || []);
  const capacity = cache.capacityBytes || 0;
  const utilization = capacity ? Math.round((cache.sizeBytes / capacity) * 100) : 0;
  const activeStreams = transfers.reduce((total, item) => total + (item.activeStreams || 0), 0);
  const activeDownloads = downloads.filter((item) => item.state === "queued" || item.state === "downloading").length;
  const activeImports = imports.filter((item) => item.state === "fetching_metadata" || item.state === "scanning").length;
  const metrics = [];
  if (allowed.has(permissions.monitoring)) metrics.push(
    ["Media cache", formatBytes(cache.sizeBytes), "accent", `${utilization}% of capacity`],
    ["Active streams", activeStreams, "", "playing right now"],
    ["Downloads", activeDownloads, "", "currently running"],
  );
  if (allowed.has(permissions.sources)) metrics.push(
    ["Sources", sources.length, "", "torrent libraries"],
    ["Imports", activeImports, "", "currently processing"],
  );
  if (allowed.has(permissions.users)) metrics.push(["Users", users.length, "", "configured accounts"]);
  if (!metrics.length) metrics.push(["Account", session.username, "accent", "dashboard access"]);
  elements.summary.innerHTML = metrics.map(([label, value, kind, detail]) =>
    `<article class="metric ${kind}"><span class="metric-label">${label}</span><strong class="metric-value">${value}</strong><small class="metric-detail">${detail}</small></article>`
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

function renderDownloads(items, canManageSources = false) {
  elements.downloadCount.textContent = `${items.length} total`;
  elements.downloads.replaceChildren();
  if (!items.length) {
    elements.downloads.append(empty("Play a torrent or Soulseek track to start caching it in the background."));
    return;
  }
  elements.downloads.innerHTML = items.map((item) => {
    const percent = item.totalBytes ? Math.min(100, Math.round((item.completedBytes / item.totalBytes) * 100)) : 0;
    const stateLabels = {
      queued: "Queued",
      downloading: "Downloading",
      cached: "Cached",
      failed: "Failed",
      cancelled: "Cancelled",
      evicted: "Evicted",
    };
    const stateLabel = stateLabels[item.state] || item.state;
    const canControl = canManageSources && item.provider === "soulseek";
    const canCancel = canControl && (item.state === "queued" || item.state === "downloading");
    const canRetry = canControl && ["failed", "cancelled", "evicted"].includes(item.state);
    return `<article class="transfer-card">
      <div class="card-title"><div><h3>${escapeHTML(item.name || item.trackId)}</h3><p class="download-provider">${escapeHTML(item.provider || "unknown provider")}</p></div><span class="status ${item.state === "failed" || item.state === "cancelled" ? "failed" : ""}">${escapeHTML(stateLabel)}</span></div>
      <div class="progress" aria-label="${percent}% complete"><span style="width:${percent}%"></span></div>
      <div class="facts">
        <div class="fact"><span>Complete</span><strong>${percent}%</strong></div>
        <div class="fact"><span>Cached</span><strong>${formatBytes(item.completedBytes)}</strong></div>
        <div class="fact"><span>Total</span><strong>${formatBytes(item.totalBytes)}</strong></div>
      </div>
      ${item.error ? `<p class="form-error">${escapeHTML(item.error)}</p>` : ""}
      ${canCancel || canRetry ? `<div class="download-actions">
        ${canCancel ? `<button class="button danger" type="button" data-download-action="cancel" data-download-id="${escapeHTML(item.id)}">Cancel</button>` : ""}
        ${canRetry ? `<button class="button secondary" type="button" data-download-action="retry" data-download-id="${escapeHTML(item.id)}">Retry</button>` : ""}
      </div>` : ""}
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

function renderLibraryScan(status) {
  const failed = Boolean(status.lastError);
  elements.scanButton.disabled = Boolean(status.scanning);
  elements.scanButton.textContent = status.scanning ? "Scanning…" : "Scan now";
  elements.scanState.className = `pill${status.scanning ? " active" : failed ? " failed" : ""}`;
  elements.scanState.textContent = status.scanning ? "Running" : failed ? "Failed" : "Idle";
  if (status.scanning) {
    elements.scanSummary.textContent = "Checking local files and connected sources in the background.";
    return;
  }
  if (failed) {
    elements.scanSummary.textContent = `Last scan failed: ${status.lastError}`;
    return;
  }
  if (status.lastFinishedAt) {
    const finished = new Date(status.lastFinishedAt).toLocaleString([], {
      dateStyle: "medium", timeStyle: "short",
    });
    elements.scanSummary.textContent = `Last completed ${finished} · ${status.tracks || 0} tracks indexed.`;
    return;
  }
  elements.scanSummary.textContent = "No library scan has completed since the server started.";
}

function renderTransferSettings(settings) {
  const download = settings.downloadLimitBytesPerSecond || 0;
  const upload = settings.uploadLimitBytesPerSecond || 0;
  elements.downloadLimit.value = download ? String(Math.round((download / mebibyte) * 1000) / 1000) : "0";
  elements.uploadLimit.value = upload ? String(Math.round((upload / mebibyte) * 1000) / 1000) : "0";
  const describe = (value) => value ? `${formatBytes(value)}/s` : "Unlimited";
  elements.transferSettingsSummary.textContent = `Download ${describe(download)} · Upload ${describe(upload)}`;
}

function renderSoulseekStatus(status) {
  const connected = Boolean(status.configured && status.reachable && status.authenticated);
  const failed = Boolean(status.configured && !connected);
  elements.soulseekState.className = `pill${connected ? " active" : failed ? " failed" : ""}`;
  elements.soulseekState.textContent = connected ? "Connected" : failed ? "Needs attention" : "Not configured";
  elements.soulseekIndicator.className = `provider-indicator${connected ? " online" : failed ? " failed" : ""}`;
  elements.soulseekMessage.textContent = status.message || "slskd status is unavailable.";
  elements.soulseekConfigured.textContent = status.configured ? "Yes" : "No";
  elements.soulseekReachable.textContent = status.reachable ? "Yes" : "No";
  elements.soulseekAuthenticated.textContent = status.authenticated ? "Granted" : "No";
}

function formatDuration(seconds = 0) {
  const total = Math.max(0, Math.round(Number(seconds) || 0));
  const minutes = Math.floor(total / 60);
  return `${minutes}:${String(total % 60).padStart(2, "0")}`;
}

function renderSoulseekResults(items) {
  const results = [...items].sort((left, right) =>
    Number(right.freeUploadSlot) - Number(left.freeUploadSlot) ||
    Number(left.requiresApproval) - Number(right.requiresApproval) ||
    (left.queueLength || 0) - (right.queueLength || 0) ||
    (right.uploadSpeedBytesPerSecond || 0) - (left.uploadSpeedBytesPerSecond || 0)
  );
  elements.soulseekSearchCount.textContent = `${results.length} result${results.length === 1 ? "" : "s"}`;
  elements.soulseekSearchResults.replaceChildren();
  if (!results.length) {
    elements.soulseekSearchResults.append(empty("No audio results arrived. Try a broader query or search again."));
    return;
  }
  elements.soulseekSearchResults.innerHTML = results.map((item) => {
    const availability = item.requiresApproval ? "Locked" : item.freeUploadSlot ? "Free slot" : "Queued";
    const facts = [
      item.suffix ? item.suffix.toUpperCase() : "Audio",
      formatBytes(item.size),
      item.durationSeconds ? formatDuration(item.durationSeconds) : "Unknown length",
      item.bitRate ? `${item.bitRate} kbps` : "Unknown bitrate",
    ];
    return `<article class="source-row search-result">
      <div class="source-copy">
        <div class="search-result-title"><h3>${escapeHTML(item.title || item.path)}</h3><span class="status ${item.requiresApproval ? "failed" : ""}">${availability}</span></div>
        <p class="search-path">${escapeHTML(item.path)}</p>
        <p class="source-meta">${facts.map((fact) => `<span>${escapeHTML(fact)}</span>`).join("")}</p>
      </div>
      <dl class="search-peer-facts">
        <div><dt>Peer</dt><dd>${escapeHTML(item.peer || "Unknown")}</dd></div>
        <div><dt>Upload</dt><dd>${formatBytes(item.uploadSpeedBytesPerSecond)}/s</dd></div>
        <div><dt>Queue</dt><dd>${Number(item.queueLength) || 0}</dd></div>
      </dl>
      <div class="search-actions">
        <button class="button secondary search-add" type="button" data-add-soulseek="track" data-source-id="${escapeHTML(item.id)}" ${item.requiresApproval ? "disabled" : ""}>${item.requiresApproval ? "Locked" : "Add track"}</button>
        <button class="button secondary search-add" type="button" data-preview-soulseek-album data-source-id="${escapeHTML(item.id)}" ${item.requiresApproval ? "disabled" : ""}>Preview album</button>
      </div>
    </article>`;
  }).join("");
}

function renderSoulseekAlbumPreview(article, album, sourceId) {
  const tracks = album.tracks || [];
  const totalSize = tracks.reduce((sum, track) => sum + (Number(track.size) || 0), 0);
  const totalDuration = tracks.reduce((sum, track) => sum + (Number(track.durationSeconds) || 0), 0);
  let preview = article.querySelector(".album-preview");
  if (!preview) {
    preview = document.createElement("section");
    preview.className = "album-preview";
    article.append(preview);
  }
  preview.innerHTML = `
    <div class="album-preview-heading">
      <div>
        <span class="card-kicker">Remote album</span>
        <h4>${escapeHTML(album.artist || "Unknown Artist")} — ${escapeHTML(album.name || "Unknown Album")}</h4>
        <p>${tracks.length} track${tracks.length === 1 ? "" : "s"} · ${formatBytes(totalSize)} · ${formatDuration(totalDuration)} · ${album.hasArtwork ? "Cover found" : "No cover found"}</p>
      </div>
      <button class="button primary" type="button" data-import-soulseek-album data-source-id="${escapeHTML(sourceId)}">Import album</button>
    </div>
    <ol class="album-track-list">
      ${tracks.map((track) => `<li>
        <span>${escapeHTML(track.title)}</span>
        <small>${escapeHTML((track.suffix || "audio").toUpperCase())} · ${formatBytes(track.size)} · ${track.durationSeconds ? formatDuration(track.durationSeconds) : "Unknown length"}</small>
      </li>`).join("")}
    </ol>`;
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
    const [cache, importPayload, downloadPayload, transferPayload, sourcePayload, userPayload, scanStatus, transferSettings, soulseekStatus] = await Promise.all([
      canMonitor ? api("/api/v1/cache/status") : Promise.resolve({}),
      canManageSources ? api("/api/v1/imports") : Promise.resolve({ imports: [] }),
      canMonitor ? api("/api/v1/downloads") : Promise.resolve({ downloads: [] }),
      canMonitor ? api("/api/v1/transfers") : Promise.resolve({ transfers: [] }),
      canManageSources ? api("/api/v1/torrents") : Promise.resolve({ sources: [] }),
      canManageUsers ? api("/api/v1/users") : Promise.resolve({ users: [] }),
      canManageSources ? api("/api/v1/library/scan") : Promise.resolve({}),
      canManageSources ? api("/api/v1/settings/transfers") : Promise.resolve({}),
      canManageSources ? api("/api/v1/providers/soulseek/status") : Promise.resolve({}),
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
    configureNavigation(session);
    renderSummary(cache, imports, downloads, transfers, sources, users, session);
    renderImports(imports);
    renderDownloads(downloads, canManageSources);
    renderTransfers(transfers);
    renderSources(sources);
    renderUsers(users, session);
    if (canManageSources) renderLibraryScan(scanStatus);
    if (canManageSources) renderTransferSettings(transferSettings);
    if (canManageSources) renderSoulseekStatus(soulseekStatus);
    elements.updatedAt.textContent = `Updated ${new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}`;
    elements.loginError.textContent = "";
    showDashboard(true);
    const active = imports.some((item) => item.state === "fetching_metadata" || item.state === "scanning") ||
      downloads.some((item) => item.state === "queued" || item.state === "downloading") || scanStatus.scanning;
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
elements.scanButton.addEventListener("click", async () => {
  elements.scanButton.disabled = true;
  elements.scanButton.textContent = "Starting…";
  try {
    const status = await api("/api/v1/library/scan", { method: "POST" });
    renderLibraryScan(status);
    window.clearTimeout(state.refreshTimer);
    state.refreshTimer = window.setTimeout(refresh, 1000);
  } catch (error) {
    elements.scanButton.disabled = false;
    elements.scanButton.textContent = "Scan now";
    elements.scanState.className = "pill failed";
    elements.scanState.textContent = "Failed";
    elements.scanSummary.textContent = error.message;
  }
});
elements.transferSettingsForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  const button = elements.transferSettingsForm.querySelector("button[type=submit]");
  button.disabled = true;
  elements.transferSettingsMessage.className = "form-message";
  elements.transferSettingsMessage.textContent = "Saving transfer limits…";
  try {
    const settings = await api("/api/v1/settings/transfers", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        downloadLimitBytesPerSecond: Math.round(Number(elements.downloadLimit.value) * mebibyte),
        uploadLimitBytesPerSecond: Math.round(Number(elements.uploadLimit.value) * mebibyte),
      }),
    });
    renderTransferSettings(settings);
    elements.transferSettingsMessage.textContent = "Transfer limits saved and applied.";
    await refresh();
  } catch (error) {
    elements.transferSettingsMessage.className = "form-message error";
    elements.transferSettingsMessage.textContent = error.message;
  } finally {
    button.disabled = false;
  }
});
elements.soulseekSearchForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  const button = elements.soulseekSearchForm.querySelector("button[type=submit]");
  button.disabled = true;
  button.textContent = "Searching…";
  elements.soulseekSearchMessage.className = "form-message";
  elements.soulseekSearchMessage.textContent = "Waiting for Soulseek peers. This usually takes 5–8 seconds…";
  elements.soulseekSearchCount.textContent = "Searching";
  try {
    const payload = await api("/api/v1/providers/soulseek/search", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        query: elements.soulseekQuery.value,
        limit: Number(elements.soulseekLimit.value),
      }),
    });
    const results = payload.results || [];
    renderSoulseekResults(results);
    elements.soulseekSearchMessage.textContent = results.length
      ? `Found ${results.length} audio result${results.length === 1 ? "" : "s"} for “${payload.query}”.`
      : `No audio results found for “${payload.query}”.`;
  } catch (error) {
    elements.soulseekSearchMessage.className = "form-message error";
    elements.soulseekSearchMessage.textContent = error.message;
    elements.soulseekSearchCount.textContent = "Failed";
  } finally {
    button.disabled = false;
    button.textContent = "Search";
  }
});
elements.soulseekSearchResults.addEventListener("click", async (event) => {
  const button = event.target.closest("[data-add-soulseek], [data-preview-soulseek-album], [data-import-soulseek-album]");
  if (!button || button.disabled) return;
  const original = button.textContent;
  button.disabled = true;
  const preview = button.hasAttribute("data-preview-soulseek-album");
  const album = button.hasAttribute("data-import-soulseek-album");
  button.textContent = preview ? "Loading…" : "Adding…";
  elements.soulseekSearchMessage.className = "form-message";
  try {
    if (preview) {
      const result = await api(`/api/v1/providers/soulseek/albums/${encodeURIComponent(button.dataset.sourceId)}`);
      renderSoulseekAlbumPreview(button.closest(".search-result"), result, button.dataset.sourceId);
      button.textContent = "Preview loaded";
      elements.soulseekSearchMessage.textContent = `Found ${result.tracks.length} tracks in “${result.name}”. Review them before importing.`;
      return;
    }
    const result = await api(`/api/v1/providers/soulseek/${album ? "albums" : "tracks"}/${encodeURIComponent(button.dataset.sourceId)}`, { method: "POST" });
    button.textContent = "Added";
    elements.soulseekSearchMessage.textContent = album
      ? `Added ${result.tracks} tracks from “${result.name}”. Audio stays remote until you press play.`
      : `Added “${result.title}” to ${result.album || "the library"}. Play it from any OpenSubsonic client to start downloading.`;
  } catch (error) {
    button.disabled = false;
    button.textContent = original;
    elements.soulseekSearchMessage.className = "form-message error";
    elements.soulseekSearchMessage.textContent = error.message;
  }
});
elements.downloads.addEventListener("click", async (event) => {
  const button = event.target.closest("[data-download-action]");
  if (!button || button.disabled) return;
  const action = button.dataset.downloadAction;
  const id = encodeURIComponent(button.dataset.downloadId);
  button.disabled = true;
  button.textContent = action === "cancel" ? "Cancelling…" : "Retrying…";
  try {
    await api(action === "cancel" ? `/api/v1/downloads/${id}` : `/api/v1/downloads/${id}/retry`, {
      method: action === "cancel" ? "DELETE" : "POST",
    });
    await refresh();
  } catch (error) {
    button.disabled = false;
    button.textContent = action === "cancel" ? "Cancel" : "Retry";
    elements.loginError.textContent = error.message;
  }
});
elements.navigation.addEventListener("click", (event) => {
  const item = event.target.closest("[data-page]");
  if (!item || item.hidden) return;
  activatePage(item.dataset.page);
});
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
