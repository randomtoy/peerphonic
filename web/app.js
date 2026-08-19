const state = {
  authorization: sessionStorage.getItem("peerphonic.authorization") || "",
  activePage: sessionStorage.getItem("peerphonic.activePage") || "overview",
  refreshTimer: 0,
  session: null,
  downloadSamples: new Map(),
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
  activitySummary: document.querySelector("#activity-summary"),
  transfers: document.querySelector("#transfers"),
  transferCount: document.querySelector("#transfer-count"),
  sources: document.querySelector("#sources"),
  sourceCount: document.querySelector("#source-count"),
  scanButton: document.querySelector("#scan-now"),
  scanState: document.querySelector("#scan-state"),
  scanSummary: document.querySelector("#scan-summary"),
  scanHistory: document.querySelector("#scan-history"),
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
  prefetchSection: document.querySelector("#prefetch-section"),
  prefetchForm: document.querySelector("#prefetch-form"),
  prefetchType: document.querySelector("#prefetch-type"),
  prefetchID: document.querySelector("#prefetch-id"),
  prefetchPinned: document.querySelector("#prefetch-pinned"),
  prefetchMessage: document.querySelector("#prefetch-message"),
  unpinTrack: document.querySelector("#unpin-track"),
  auditEntries: document.querySelector("#audit-entries"),
  auditCount: document.querySelector("#audit-count"),
  usersSection: document.querySelector("#users-section"),
  artistAliasForm: document.querySelector("#artist-alias-form"),
  artistAliasSource: document.querySelector("#artist-alias-source"),
  artistAliasTarget: document.querySelector("#artist-alias-target"),
  artistAliases: document.querySelector("#artist-aliases"),
  artistAliasCount: document.querySelector("#artist-alias-count"),
  catalogMessage: document.querySelector("#catalog-message"),
  trackMetadataForm: document.querySelector("#track-metadata-form"),
  metadataTrackID: document.querySelector("#metadata-track-id"),
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
  soulseekSearch: "soulseek.search",
  soulseekAdd: "soulseek.add",
  soulseekClient: "soulseek.client-search",
  users: "users.manage",
  catalog: "catalog.manage",
};

const permissionLabels = {
  "dashboard.access": "Dashboard",
  "monitoring.view": "Monitoring",
  "sources.manage": "Sources",
  "soulseek.search": "Soulseek search",
  "soulseek.add": "Add from Soulseek",
  "soulseek.client-search": "Soulseek in music clients",
  "users.manage": "Users",
  "catalog.manage": "Catalog metadata",
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
    description: "See what is moving, what is waiting, and whether peers or seeds are available.",
  },
  search: {
    eyebrow: "REMOTE DISCOVERY",
    title: "Soulseek search",
    description: "Find tracks or review and add a complete album from a connected peer.",
  },
  users: {
    eyebrow: "ACCESS CONTROL",
    title: "Users and permissions",
    description: "Manage accounts and delegate access to individual features.",
  },
  catalog: {
    eyebrow: "CATALOG CURATION",
    title: "Catalog",
    description: "Resolve naming conflicts and correct metadata without touching source files.",
  },
  settings: {
    eyebrow: "SERVER CONFIGURATION",
    title: "Settings",
    description: "Tune network behavior without restarting Peerphonic.",
  },
};

const mebibyte = 1024 * 1024;
const activeDownloadStates = new Set(["waiting", "queued", "downloading", "transcoding"]);
const runningDownloadStates = new Set(["downloading", "transcoding"]);
const queuedDownloadStates = new Set(["waiting", "queued"]);
const attentionDownloadStates = new Set(["failed", "cancelled"]);
const audioDownloadExtensions = new Set([
  "aac", "aif", "aiff", "alac", "ape", "flac", "m4a", "m4b", "mp3", "mpc",
  "oga", "ogg", "opus", "wav", "wma", "wv",
]);

function isActiveDownload(item) {
  return activeDownloadStates.has(item.state);
}

function downloadExtension(item) {
  const match = String(item.name || "").toLowerCase().match(/\.([^.]+)$/);
  return match ? match[1] : "";
}

function isAudioDownload(item) {
  return item.provider === "soulseek" || item.provider === "transcode" ||
    audioDownloadExtensions.has(downloadExtension(item));
}

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
  const activeDownloads = downloads.filter(isActiveDownload).length;
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

function groupTrackDownloads(items) {
  const rank = { cached: 0, transcoding: 1, downloading: 2, waiting: 3, queued: 4, failed: 5, cancelled: 6, evicted: 7 };
  const groups = new Map();
  items.forEach((item) => {
    const key = `${item.provider || "unknown"}:${item.trackId || item.id}`;
    const group = groups.get(key) || [];
    group.push(item);
    groups.set(key, group);
  });
  return [...groups.values()].map((attempts) => {
    attempts.sort((left, right) =>
      (rank[left.state] ?? 99) - (rank[right.state] ?? 99) ||
      new Date(right.updatedAt || 0) - new Date(left.updatedAt || 0)
    );
    return {
      ...attempts[0],
      attemptCount: attempts.length,
      failedAttempts: attempts.filter((item) => item.state === "failed").length,
    };
  }).sort((left, right) =>
    (rank[left.state] ?? 99) - (rank[right.state] ?? 99) ||
    new Date(right.updatedAt || 0) - new Date(left.updatedAt || 0)
  );
}

function relativeTime(value, now = Date.now()) {
  const timestamp = new Date(value || 0).getTime();
  if (!Number.isFinite(timestamp) || timestamp <= 0) return "unknown";
  const seconds = Math.max(0, Math.round((now - timestamp) / 1000));
  if (seconds < 5) return "just now";
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}

function sampleDownload(item, now = Date.now()) {
  const previous = state.downloadSamples.get(item.id);
  const active = isActiveDownload(item);
  let rate = 0;
  let lastProgressAt = previous?.lastProgressAt || now;
  if (previous && active) {
    const elapsed = Math.max(1, now - previous.sampledAt);
    const delta = Math.max(0, (Number(item.completedBytes) || 0) - previous.bytes);
    rate = delta * 1000 / elapsed;
    if (delta > 0) lastProgressAt = now;
  }
  state.downloadSamples.set(item.id, {
    bytes: Number(item.completedBytes) || 0,
    sampledAt: now,
    lastProgressAt,
  });
  return { rate, measuring: !previous && active, stalled: active && now - lastProgressAt >= 30_000, lastProgressAt };
}

function downloadNetworkSummary(item, transfer, sample, now) {
  const attempts = item.attemptCount > 1
    ? ` · ${item.attemptCount} source attempts${item.failedAttempts ? `, ${item.failedAttempts} failed` : ""}`
    : "";
  if (item.state === "cached") return `Ready in local cache${attempts}`;
  if (item.state === "failed") return `Source failed${attempts}`;
  if (item.state === "cancelled") return `Download was cancelled${attempts}`;
  if (item.state === "evicted") return `Removed from cache; playback will fetch it again${attempts}`;
  if (sample.stalled) return `No new data since ${relativeTime(sample.lastProgressAt, now)}${attempts}`;
  if (item.provider === "transcode") {
    return item.state === "transcoding"
      ? `Creating a reusable MP3 variant${attempts}`
      : `Preparing the transcoder${attempts}`;
  }
  if (item.provider === "torrent") {
    if (!transfer) return `Connecting to torrent swarm${attempts}`;
    if ((transfer.connectedSeeders || 0) > 0) {
      return `${transfer.connectedSeeders} connected seed${transfer.connectedSeeders === 1 ? "" : "s"} · ${transfer.activePeers || 0}/${transfer.peers || 0} active peers${attempts}`;
    }
    if ((transfer.peers || 0) > 0) return `Peers connected, but no complete seed is available${attempts}`;
    return `No peers connected; waiting for the swarm${attempts}`;
  }
  return `${item.state === "waiting" || item.state === "queued" ? "Waiting for" : "Receiving from"} Soulseek peer ${item.sourceId || "unknown"}${attempts}`;
}

function downloadStateLabel(state) {
  return ({
    waiting: "Waiting",
    queued: "Queued",
    downloading: "Downloading",
    transcoding: "Transcoding",
    cached: "Cached",
    failed: "Failed",
    cancelled: "Cancelled",
    evicted: "Not cached",
  })[state] || state;
}

function downloadActions(item, canManageSources) {
  const canControl = canManageSources && item.provider === "soulseek";
  const canCancel = canControl && ["waiting", "queued", "downloading"].includes(item.state);
  const canRetry = canControl && ["failed", "cancelled", "evicted"].includes(item.state);
  if (!canCancel && !canRetry) return "";
  return `<div class="download-actions">
    ${canCancel ? `<button class="button danger" type="button" data-download-action="cancel" data-download-id="${escapeHTML(item.id)}">Cancel</button>` : ""}
    ${canRetry ? `<button class="button secondary" type="button" data-download-action="retry" data-download-id="${escapeHTML(item.id)}">Retry</button>` : ""}
  </div>`;
}

function renderLiveDownload(item, transfer, canManageSources, now) {
  const percent = item.totalBytes ? Math.min(100, Math.round((item.completedBytes / item.totalBytes) * 100)) : 0;
  const sample = sampleDownload(item, now);
  const active = isActiveDownload(item);
  const speed = !active ? "—" : sample.measuring ? "Measuring…" : sample.rate > 0 ? `${formatBytes(sample.rate)}/s` : "0 B/s";
  const indeterminate = active && !item.totalBytes;
  const providerDetail = item.provider === "torrent"
    ? `${transfer?.activePeers || 0} active peers · ${transfer?.connectedSeeders || 0} connected seeds`
    : item.provider === "transcode" ? "Reusable MP3 variant" : `Peer ${item.sourceId || "unknown"}`;
  const tone = attentionDownloadStates.has(item.state) ? "danger" : sample.stalled ? "warning" : "accent";
  return `<article class="activity-download activity-download-${tone}">
    <div class="activity-download-heading">
      <span class="provider-mark" aria-hidden="true">${item.provider === "torrent" ? "T" : item.provider === "soulseek" ? "S" : "MP3"}</span>
      <div class="activity-download-copy">
        <h3>${escapeHTML(item.name || item.trackId)}</h3>
        <p>${escapeHTML(item.provider || "unknown provider")} · ${escapeHTML(providerDetail)}</p>
      </div>
      <span class="state-chip state-${escapeHTML(item.state)}">${escapeHTML(downloadStateLabel(item.state))}</span>
    </div>
    <div class="progress ${indeterminate ? "indeterminate" : ""}" aria-label="${indeterminate ? "In progress" : `${percent}% complete`}"><span style="width:${indeterminate ? 35 : percent}%"></span></div>
    <p class="download-status-detail ${sample.stalled ? "stalled" : ""}">${escapeHTML(downloadNetworkSummary(item, transfer, sample, now))}</p>
    <div class="activity-download-metrics">
      <span><small>Progress</small><strong>${item.totalBytes ? `${percent}%` : "Preparing"}</strong></span>
      <span><small>Speed</small><strong>${speed}</strong></span>
      <span><small>Cached</small><strong>${formatBytes(item.completedBytes)}${item.totalBytes ? ` / ${formatBytes(item.totalBytes)}` : ""}</strong></span>
      <span><small>Updated</small><strong>${relativeTime(item.updatedAt, now)}</strong></span>
    </div>
    ${item.error ? `<p class="activity-error">${escapeHTML(item.error)}</p>` : ""}
    ${downloadActions(item, canManageSources)}
  </article>`;
}

function renderDownloadHistoryItem(item, canManageSources, now) {
  const failed = attentionDownloadStates.has(item.state);
  return `<article class="activity-history-row">
    <span class="history-kind" aria-hidden="true">♪</span>
    <div class="activity-history-copy">
      <h3>${escapeHTML(item.name || item.trackId)}</h3>
      <p>${escapeHTML(item.provider || "unknown")} · ${formatBytes(item.totalBytes || item.completedBytes)} · updated ${relativeTime(item.updatedAt, now)}</p>
      ${item.error ? `<p class="activity-error">${escapeHTML(item.error)}</p>` : ""}
    </div>
    <span class="state-chip ${failed ? "state-failed" : `state-${escapeHTML(item.state)}`}">${escapeHTML(downloadStateLabel(item.state))}</span>
    ${downloadActions(item, canManageSources)}
  </article>`;
}

function renderSupportingFiles(items, transferBySource, now) {
  const groups = new Map();
  items.forEach((item) => {
    const key = `${item.provider}:${item.sourceId || "unknown"}`;
    const current = groups.get(key) || { provider: item.provider, sourceId: item.sourceId, items: [] };
    current.items.push(item);
    groups.set(key, current);
  });
  return [...groups.values()].map((group) => {
    const transfer = transferBySource.get(`${group.provider}:${group.sourceId}`);
    const bytes = group.items.reduce((total, item) => total + (Number(item.completedBytes) || 0), 0);
    const active = group.items.filter(isActiveDownload).length;
    const failed = group.items.filter((item) => attentionDownloadStates.has(item.state)).length;
    const status = failed ? `${failed} failed` : active ? `${active} active` : "Cached";
    const newest = group.items.reduce((latest, item) =>
      new Date(item.updatedAt || 0) > new Date(latest || 0) ? item.updatedAt : latest, "");
    return `<article class="support-file-row">
      <div><h3>${escapeHTML(transfer?.name || `${group.provider || "Unknown"} source`)}</h3><p>${group.items.length} artwork or metadata files · ${formatBytes(bytes)} · updated ${relativeTime(newest, now)}</p></div>
      <span class="state-chip ${failed ? "state-failed" : "state-cached"}">${escapeHTML(status)}</span>
    </article>`;
  }).join("");
}

function renderActivitySummary(downloads, transfers) {
  const running = downloads.filter((item) => runningDownloadStates.has(item.state)).length;
  const queued = downloads.filter((item) => queuedDownloadStates.has(item.state)).length;
  const attention = downloads.filter((item) => attentionDownloadStates.has(item.state)).length;
  const peers = transfers.reduce((total, item) => total + (Number(item.activePeers) || 0), 0);
  const seeds = transfers.reduce((total, item) => total + (Number(item.connectedSeeders) || 0), 0);
  const streams = transfers.reduce((total, item) => total + (Number(item.activeStreams) || 0), 0);
  const metrics = [
    ["Running", running, running ? "accent" : "", running ? "receiving audio now" : "nothing moving"],
    ["Waiting", queued, queued ? "warning" : "", queued ? "queued for a slot" : "queue is clear"],
    ["Needs attention", attention, attention ? "danger" : "", attention ? "review failed items" : "all healthy"],
    ["Connected peers", peers, "", `${seeds} seeds · ${streams} streams`],
  ];
  elements.activitySummary.innerHTML = metrics.map(([label, value, tone, detail]) => `<article class="activity-stat ${tone}">
    <span>${label}</span><strong>${value}</strong><small>${detail}</small>
  </article>`).join("");
}

function renderDownloads(items, transfers = [], canManageSources = false) {
  const grouped = groupTrackDownloads(items);
  const music = grouped.filter(isAudioDownload);
  const supporting = grouped.filter((item) => !isAudioDownload(item));
  const running = music.filter((item) => runningDownloadStates.has(item.state));
  const queued = music.filter((item) => queuedDownloadStates.has(item.state));
  const attention = music.filter((item) => attentionDownloadStates.has(item.state));
  const history = music.filter((item) => !isActiveDownload(item) && !attentionDownloadStates.has(item.state));
  const transferBySource = new Map(transfers.map((item) => [`${item.provider}:${item.id}`, item]));
  const now = Date.now();
  renderActivitySummary(music, transfers);
  elements.downloadCount.textContent = `${music.length} music · ${supporting.length} support files`;
  const sections = [];
  const addLiveSection = (kind, title, description, sectionItems) => {
    if (!sectionItems.length) return;
    sections.push(`<section class="activity-group activity-group-${kind}">
      <div class="activity-group-heading"><div><h3>${title}</h3><p>${description}</p></div><span>${sectionItems.length}</span></div>
      <div class="activity-download-list">${sectionItems.map((item) => renderLiveDownload(
        item, transferBySource.get(`${item.provider}:${item.sourceId}`), canManageSources, now
      )).join("")}</div>
    </section>`);
  };
  addLiveSection("running", "In progress", "Playback and offline downloads currently receiving data.", running);
  addLiveSection("queued", "Waiting", "Ready to start when a provider or download slot becomes available.", queued);
  addLiveSection("attention", "Needs attention", "Failed or cancelled downloads that may need another source.", attention);
  if (!running.length && !queued.length && !attention.length) {
    sections.push(`<div class="activity-calm"><span aria-hidden="true"></span><div><strong>Everything is quiet</strong><p>No music is downloading or waiting right now.</p></div></div>`);
  }
  if (history.length) {
    sections.push(`<details class="activity-history">
      <summary><span><strong>Music history</strong><small>Completed and evicted audio</small></span><span>${history.length}</span></summary>
      <div class="activity-history-list">${history.map((item) => renderDownloadHistoryItem(item, canManageSources, now)).join("")}</div>
    </details>`);
  }
  if (supporting.length) {
    sections.push(`<details class="activity-history support-history">
      <summary><span><strong>Supporting files</strong><small>Artwork and metadata kept separate from music</small></span><span>${supporting.length}</span></summary>
      <div class="support-file-list">${renderSupportingFiles(supporting, transferBySource, now)}</div>
    </details>`);
  }
  if (!grouped.length) {
    elements.downloads.replaceChildren(empty("Play a torrent or Soulseek track to start caching it in the background."));
  } else {
    elements.downloads.innerHTML = sections.join("");
  }
  const visibleIDs = new Set(grouped.map((item) => item.id));
  [...state.downloadSamples.keys()].forEach((id) => { if (!visibleIDs.has(id)) state.downloadSamples.delete(id); });
}

function renderTransfers(items) {
  elements.transferCount.textContent = `${items.length} source${items.length === 1 ? "" : "s"}`;
  elements.transfers.replaceChildren();
  if (!items.length) {
    elements.transfers.append(empty("No torrent source is connected yet."));
    return;
  }
  const peers = items.reduce((total, item) => total + (Number(item.activePeers) || 0), 0);
  const seeds = items.reduce((total, item) => total + (Number(item.connectedSeeders) || 0), 0);
  const streams = items.reduce((total, item) => total + (Number(item.activeStreams) || 0), 0);
  const uploaded = items.reduce((total, item) => total + (Number(item.uploadedBytes) || 0), 0);
  const sourceRows = items.map((item) => {
    const label = item.activeStreams ? "Streaming" : item.activePeers ? "Connected" : item.seeding ? "Sharing" : "Idle";
    return `<article class="network-source-row">
      <div class="network-source-copy"><span class="network-dot ${item.activePeers || item.activeStreams ? "online" : ""}" aria-hidden="true"></span><div><h3>${escapeHTML(item.name || item.id)}</h3><p>${formatBytes(item.completedBytes)} cached locally · ${formatBytes(item.totalBytes)} available</p></div></div>
      <div class="network-source-facts"><span><small>Peers</small><strong>${item.activePeers || 0} / ${item.peers || 0}</strong></span><span><small>Seeds</small><strong>${item.connectedSeeders || 0}</strong></span><span><small>Uploaded</small><strong>${formatBytes(item.uploadedBytes)}</strong></span></div>
      <span class="state-chip state-${item.activePeers || item.activeStreams ? "cached" : "idle"}">${label}</span>
    </article>`;
  }).join("");
  const list = items.length > 6
    ? `<details class="network-source-drawer"><summary><span>Source details</span><span>${items.length}</span></summary><div class="network-source-list">${sourceRows}</div></details>`
    : `<div class="network-source-list">${sourceRows}</div>`;
  elements.transfers.innerHTML = `<div class="network-stats">
    <span><small>Active peers</small><strong>${peers}</strong></span>
    <span><small>Connected seeds</small><strong>${seeds}</strong></span>
    <span><small>Active streams</small><strong>${streams}</strong></span>
    <span><small>Uploaded</small><strong>${formatBytes(uploaded)}</strong></span>
  </div>${list}`;
}

function renderAudit(items) {
  elements.auditCount.textContent = `${items.length} recent`;
  elements.auditEntries.replaceChildren();
  if (!items.length) {
    elements.auditEntries.append(empty("No administrative changes recorded yet."));
    return;
  }
  elements.auditEntries.innerHTML = items.map((item) => `<article class="source-row">
    <div class="source-copy">
      <h3>${escapeHTML(item.method)} ${escapeHTML(item.path)}</h3>
      <p class="source-meta"><span>${escapeHTML(item.actor)}</span><span>${escapeHTML(item.remoteAddress || "unknown address")}</span><span>${escapeHTML(new Date(item.occurredAt).toLocaleString())}</span></p>
    </div>
    <span class="status ${item.status >= 400 ? "failed" : ""}">${item.status}</span>
  </article>`).join("");
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
  const history = status.history || [];
  elements.scanHistory.replaceChildren();
  if (!history.length) {
    elements.scanHistory.append(empty("No maintenance runs have completed since the server started."));
  } else {
    elements.scanHistory.innerHTML = history.map((run) => {
      const finished = new Date(run.finishedAt).toLocaleString([], { dateStyle: "medium", timeStyle: "short" });
      const warnings = run.warnings || [];
      return `<article class="source-card">
        <div class="source-main"><span class="source-name">${escapeHTML(run.trigger || "scan")} · ${escapeHTML(finished)}</span><span class="source-meta">${run.tracks || 0} tracks · ${warnings.length} warning${warnings.length === 1 ? "" : "s"}${run.error ? ` · ${escapeHTML(run.error)}` : ""}</span>${warnings.length ? `<small>${warnings.slice(0, 3).map(escapeHTML).join(" · ")}</small>` : ""}</div>
      </article>`;
    }).join("");
  }
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

function renderCatalog(payload) {
  const artists = payload.artists || [];
  const aliases = payload.aliases || [];
  const options = artists.map((artist) => `<option value="${escapeHTML(artist.id)}">${escapeHTML(artist.name)} · ${artist.albumCount || 0} albums</option>`).join("");
  elements.artistAliasSource.innerHTML = options;
  elements.artistAliasTarget.innerHTML = options;
  if (artists.length > 1) elements.artistAliasTarget.selectedIndex = 1;
  elements.artistAliasCount.textContent = `${aliases.length} alias${aliases.length === 1 ? "" : "es"}`;
  elements.artistAliases.replaceChildren();
  if (!aliases.length) {
    elements.artistAliases.append(empty("No manual artist aliases. Exact case and spacing variants are merged automatically."));
    return;
  }
  elements.artistAliases.innerHTML = aliases.map((alias) => `<article class="source-card">
    <div class="source-main"><span class="source-name">${escapeHTML(alias.aliasName)}</span><span class="source-meta">Shown as ${escapeHTML(alias.targetName)}</span></div>
    <div class="source-actions"><button class="button danger" type="button" data-delete-artist-alias="${escapeHTML(alias.aliasId)}">Remove alias</button></div>
  </article>`).join("");
}

function formatDuration(seconds = 0) {
  const total = Math.max(0, Math.round(Number(seconds) || 0));
  const minutes = Math.floor(total / 60);
  return `${minutes}:${String(total % 60).padStart(2, "0")}`;
}

function renderSoulseekResults(items, collections = [], canAdd = false) {
  const grouped = collections.length ? collections : items.map((item) => ({
    id: item.id,
    name: item.album || "Unknown Album",
    artist: item.artist || "Unknown Artist",
    path: item.path.split("/").slice(0, -1).join("/"),
    peer: item.peer,
    matchedTracks: 1,
    matchedSize: item.size,
    formats: item.suffix ? [item.suffix.toUpperCase()] : [],
    uploadSpeedBytesPerSecond: item.uploadSpeedBytesPerSecond,
    queueLength: item.queueLength,
    freeUploadSlot: item.freeUploadSlot,
    requiresApproval: item.requiresApproval,
    results: [item],
  }));
  const results = [...grouped].sort((left, right) =>
    Number(right.freeUploadSlot) - Number(left.freeUploadSlot) ||
    Number(left.requiresApproval) - Number(right.requiresApproval) ||
    (left.queueLength || 0) - (right.queueLength || 0) ||
    (right.uploadSpeedBytesPerSecond || 0) - (left.uploadSpeedBytesPerSecond || 0)
  );
  elements.soulseekSearchCount.textContent = `${results.length} album${results.length === 1 ? "" : "s"} · ${items.length} match${items.length === 1 ? "" : "es"}`;
  elements.soulseekSearchResults.replaceChildren();
  if (!results.length) {
    elements.soulseekSearchResults.append(empty("No audio results arrived. Try a broader query or search again."));
    return;
  }
  elements.soulseekSearchResults.innerHTML = results.map((group) => {
    const availability = group.requiresApproval ? "Locked" : group.freeUploadSlot ? "Free slot" : "Queued";
    const facts = [
      `${group.matchedTracks} matched track${group.matchedTracks === 1 ? "" : "s"}`,
      formatBytes(group.matchedSize),
      (group.formats || []).join(" / ") || "Audio",
    ];
    const matchRows = (group.results || []).map((item) => `<li>
      <div><span>${escapeHTML(item.title || item.path)}</span><small>${escapeHTML((item.suffix || "audio").toUpperCase())} · ${formatBytes(item.size)} · ${item.durationSeconds ? formatDuration(item.durationSeconds) : "Unknown length"}${item.bitRate ? ` · ${item.bitRate} kbps` : ""}</small></div>
      <button class="button secondary" type="button" data-add-soulseek="track" data-source-id="${escapeHTML(item.id)}" ${item.requiresApproval || !canAdd ? "disabled" : ""}>${item.requiresApproval ? "Locked" : canAdd ? "Add track" : "No add access"}</button>
    </li>`).join("");
    const trackList = group.matchedTracks > 1
      ? `<details class="search-matches"><summary>Show ${group.matchedTracks} matched tracks</summary><ol class="search-match-list">${matchRows}</ol></details>`
      : `<ol class="search-match-list">${matchRows}</ol>`;
    return `<article class="source-row search-result">
      <div class="source-copy">
        <div class="search-result-title"><h3>${escapeHTML(group.artist || "Unknown Artist")} — ${escapeHTML(group.name || "Unknown Album")}</h3><span class="status ${group.requiresApproval ? "failed" : ""}">${availability}</span></div>
        <p class="search-path">${escapeHTML(group.path)}</p>
        <p class="source-meta">${facts.map((fact) => `<span>${escapeHTML(fact)}</span>`).join("")}</p>
      </div>
      <dl class="search-peer-facts">
        <div><dt>Peer</dt><dd>${escapeHTML(group.peer || "Unknown")}</dd></div>
        <div><dt>Upload</dt><dd>${formatBytes(group.uploadSpeedBytesPerSecond)}/s</dd></div>
        <div><dt>Queue</dt><dd>${Number(group.queueLength) || 0}</dd></div>
      </dl>
      <div class="search-actions">
        <button class="button primary search-add" type="button" data-preview-soulseek-album data-source-id="${escapeHTML(group.id)}" ${group.requiresApproval ? "disabled" : ""}>${canAdd ? "Review & add whole album" : "View full album"}</button>
      </div>
      ${trackList}
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
      <button class="button primary" type="button" data-import-soulseek-album data-source-id="${escapeHTML(sourceId)}" ${state.session?.permissions?.includes(permissions.soulseekAdd) ? "" : "disabled"}>${state.session?.permissions?.includes(permissions.soulseekAdd) ? "Import album" : "No import access"}</button>
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
    state.session = session;
    const allowed = new Set(session.permissions || []);
    const canMonitor = allowed.has(permissions.monitoring);
    const canManageSources = allowed.has(permissions.sources);
    const canSearchSoulseek = allowed.has(permissions.soulseekSearch);
    const canManageUsers = allowed.has(permissions.users);
    const canManageCatalog = allowed.has(permissions.catalog);
    const [cache, importPayload, downloadPayload, transferPayload, sourcePayload, userPayload, scanStatus, transferSettings, soulseekStatus, catalogPayload, auditPayload] = await Promise.all([
      canMonitor ? api("/api/v1/cache/status") : Promise.resolve({}),
      canManageSources ? api("/api/v1/imports") : Promise.resolve({ imports: [] }),
      canMonitor ? api("/api/v1/downloads") : Promise.resolve({ downloads: [] }),
      canMonitor ? api("/api/v1/transfers") : Promise.resolve({ transfers: [] }),
      canManageSources ? api("/api/v1/torrents") : Promise.resolve({ sources: [] }),
      canManageUsers ? api("/api/v1/users") : Promise.resolve({ users: [] }),
      canManageSources ? api("/api/v1/library/scan") : Promise.resolve({}),
      canManageSources ? api("/api/v1/settings/transfers") : Promise.resolve({}),
      canSearchSoulseek ? api("/api/v1/providers/soulseek/status") : Promise.resolve({}),
      canManageCatalog ? api("/api/v1/catalog/artists") : Promise.resolve({ artists: [], aliases: [] }),
      canMonitor ? api("/api/v1/audit?limit=50") : Promise.resolve({ entries: [] }),
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
    elements.prefetchSection.hidden = !canManageSources;
    elements.usersSection.hidden = !canManageUsers;
    configureNavigation(session);
    renderSummary(cache, imports, downloads, transfers, sources, users, session);
    renderImports(imports);
    renderDownloads(downloads, transfers, canManageSources);
    renderTransfers(transfers);
    if (canMonitor) renderAudit(auditPayload.entries || []);
    renderSources(sources);
    renderUsers(users, session);
    if (canManageCatalog) renderCatalog(catalogPayload);
    if (canManageSources) renderLibraryScan(scanStatus);
    if (canManageSources) renderTransferSettings(transferSettings);
    if (canSearchSoulseek) renderSoulseekStatus(soulseekStatus);
    elements.updatedAt.textContent = `Updated ${new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}`;
    elements.loginError.textContent = "";
    showDashboard(true);
    const active = imports.some((item) => item.state === "fetching_metadata" || item.state === "scanning") ||
      downloads.some(isActiveDownload) || scanStatus.scanning;
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
    renderSoulseekResults(results, payload.collections || [], Boolean(state.session?.permissions?.includes(permissions.soulseekAdd)));
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

elements.artistAliasForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  const aliasID = elements.artistAliasSource.value;
  const targetID = elements.artistAliasTarget.value;
  elements.catalogMessage.className = "form-message";
  if (!aliasID || !targetID || aliasID === targetID) {
    elements.catalogMessage.className = "form-message error";
    elements.catalogMessage.textContent = "Choose two different artists.";
    return;
  }
  const button = elements.artistAliasForm.querySelector("button[type=submit]");
  button.disabled = true;
  try {
    await api(`/api/v1/catalog/artist-aliases/${encodeURIComponent(aliasID)}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ targetId: targetID }),
    });
    elements.catalogMessage.textContent = "Artist alias saved. Music clients will see one artist after synchronization.";
    await refresh();
  } catch (error) {
    elements.catalogMessage.className = "form-message error";
    elements.catalogMessage.textContent = error.message;
  } finally {
    button.disabled = false;
  }
});

elements.prefetchForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  const button = elements.prefetchForm.querySelector("button[type=submit]");
  button.disabled = true;
  elements.prefetchMessage.className = "form-message";
  elements.prefetchMessage.textContent = "Adding music to the background cache queue…";
  try {
    const result = await api(`/api/v1/cache/${elements.prefetchType.value}/${encodeURIComponent(elements.prefetchID.value.trim())}`, {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ pinned: elements.prefetchPinned.checked }),
    });
    const count = result?.tracks;
    elements.prefetchMessage.textContent = count ? `Queued ${count} tracks.` : "Track queued for caching.";
    elements.prefetchForm.reset();
    await refresh();
  } catch (error) {
    elements.prefetchMessage.className = "form-message error";
    elements.prefetchMessage.textContent = error.message;
  } finally {
    button.disabled = false;
  }
});

elements.unpinTrack.addEventListener("click", async () => {
  const id = elements.prefetchID.value.trim();
  if (!id || elements.prefetchType.value !== "tracks") {
    elements.prefetchMessage.className = "form-message error";
    elements.prefetchMessage.textContent = "Enter a track ID and select Track.";
    return;
  }
  elements.unpinTrack.disabled = true;
  try {
    await api(`/api/v1/cache/tracks/${encodeURIComponent(id)}/pin`, {
      method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ pinned: false }),
    });
    elements.prefetchMessage.className = "form-message";
    elements.prefetchMessage.textContent = "Track is no longer protected from eviction.";
  } catch (error) {
    elements.prefetchMessage.className = "form-message error";
    elements.prefetchMessage.textContent = error.message;
  } finally {
    elements.unpinTrack.disabled = false;
  }
});

elements.artistAliases.addEventListener("click", async (event) => {
  const button = event.target.closest("[data-delete-artist-alias]");
  if (!button) return;
  button.disabled = true;
  try {
    await api(`/api/v1/catalog/artist-aliases/${encodeURIComponent(button.dataset.deleteArtistAlias)}`, { method: "DELETE" });
    elements.catalogMessage.textContent = "Artist alias removed.";
    await refresh();
  } catch (error) {
    button.disabled = false;
    elements.catalogMessage.className = "form-message error";
    elements.catalogMessage.textContent = error.message;
  }
});

elements.trackMetadataForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  const payload = {};
  for (const [key, selector] of Object.entries({
    title: "#metadata-title", artist: "#metadata-artist", album: "#metadata-album",
    albumArtist: "#metadata-album-artist", genre: "#metadata-genre",
  })) {
    const value = elements.trackMetadataForm.querySelector(selector).value.trim();
    if (value) payload[key] = value;
  }
  for (const [key, selector] of Object.entries({
    year: "#metadata-year", trackNumber: "#metadata-track-number", discNumber: "#metadata-disc-number",
  })) {
    const value = elements.trackMetadataForm.querySelector(selector).value;
    if (value !== "") payload[key] = Number(value);
  }
  if (!Object.keys(payload).length) {
    elements.catalogMessage.className = "form-message error";
    elements.catalogMessage.textContent = "Enter at least one metadata value.";
    return;
  }
  const button = elements.trackMetadataForm.querySelector("button[type=submit]");
  button.disabled = true;
  try {
    await api(`/api/v1/catalog/tracks/${encodeURIComponent(elements.metadataTrackID.value.trim())}`, {
      method: "PATCH", headers: { "Content-Type": "application/json" }, body: JSON.stringify(payload),
    });
    elements.catalogMessage.className = "form-message";
    elements.catalogMessage.textContent = "Track metadata updated.";
    elements.trackMetadataForm.reset();
    await refresh();
  } catch (error) {
    elements.catalogMessage.className = "form-message error";
    elements.catalogMessage.textContent = error.message;
  } finally {
    button.disabled = false;
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
