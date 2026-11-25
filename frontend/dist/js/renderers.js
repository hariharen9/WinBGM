// --- View Renderers ---

const CHANGEABLE = ['service', 'run', 'folder', 'task', 'shellex'];

const HELPER_EXE_RE = /(update|updater|setup|install|unins|svc|service|helper|crash|broker|daemon)/i;

function getAppIconPaths(a) {
  const paths = [];
  const lowName = (a.name || '').toLowerCase();
  // 1. Prefer matching installed app DisplayIcon / ExePath if available
  if (installedApps && installedApps.length && lowName.length >= 3) {
    const match = installedApps.find(ia => {
      const n = (ia.name || '').toLowerCase();
      return n === lowName || (n.includes(lowName) && lowName.length >= 4) || (lowName.includes(n) && n.length >= 4);
    });
    if (match) {
      if (match.displayIcon) paths.push(match.displayIcon);
      if (match.exePath) paths.push(match.exePath);
    }
  }
  // 2. Sort component executables: main GUI binaries first, updaters/services/helpers last
  const comps = (a.components || []).filter(c => c.exePath).slice().sort((x, y) => {
    const xHelper = HELPER_EXE_RE.test(base(x.exePath)) ? 1 : 0;
    const yHelper = HELPER_EXE_RE.test(base(y.exePath)) ? 1 : 0;
    if (xHelper !== yHelper) return xHelper - yHelper;
    const xSvc = x.kind === 'service' || x.kind === 'task' ? 1 : 0;
    const ySvc = y.kind === 'service' || y.kind === 'task' ? 1 : 0;
    return xSvc - ySvc;
  });
  comps.forEach(c => paths.push(c.exePath));
  if (a.path) paths.push(a.path);
  return paths;
}

function renderAppIcon(name, letter, paths, extraClass = '', extraStyle = '') {
  const list = Array.isArray(paths) ? paths : [paths];
  for (const p of list) {
    if (!p) continue;
    const uri = appIconCache[p.toLowerCase()];
    if (uri) {
      return `<img class="product-icon-img ${extraClass}" src="${uri}" alt="" style="${extraStyle}">`;
    }
  }
  return `<div class="product-icon ${extraClass}" style="${iconStyle(name)};${extraStyle}">${esc(letter || (name || 'A').slice(0, 1).toUpperCase())}</div>`;
}

function render() {
  if (!inv) return;
  $('machine-name').textContent = inv.machine || 'This PC';
  $('machine-meta').textContent = inv.os || '';
  $('status-admin').textContent = inv.elevated ? 'Administrator' : 'Standard user';
  $('nav-apps').textContent = apps().length;
  $('nav-services').textContent = comps('service').length;
  $('nav-tasks').textContent = comps('task').length;
  $('set-system').checked = showSystem;
  if ($('set-ui-scale')) $('set-ui-scale').value = uiScale;
  if ($('set-always-admin')) $('set-always-admin').checked = !!inv.alwaysAdmin;

  renderSentinelAlerts();
  renderBanners();
  renderApps();
  renderOverview();
  renderServices();
  renderTasks();
  renderSources();
  renderSnapshots();
  if (installedApps && installedApps.length) renderUninstaller();
  refreshIcons();
}

function renderBanners() {
  const w = [];
  if (!api()) w.push(['info', 'Preview with sample data — changes are only available in the desktop app.', '']);
  if (inv.elevated === false) w.push(['info', 'Running without administrator rights. Changing services and scheduled tasks needs them; Run-key and Startup-folder items for your account work without.', 'elevate']);
  (inv.warnings || []).filter(x => !/^Not running as administrator/.test(x)).forEach(x => w.push(['', x, '']));
  
  $('app-banners').innerHTML = w.map(([k, t, a]) =>
    `<div class="banner ${k}"><i data-lucide="info" style="width:14px;height:14px;flex:none;margin-top:2px"></i><span>${esc(t)}</span>${a === 'elevate' && api() ? '<button class="win-btn" style="margin-left:auto;height:24px;flex:none" onclick="doElevate()">Restart as administrator</button>' : ''}</div>`
  ).join('');
  refreshIcons();
}

function renderApps() {
  const all = apps();
  const cnt = {
    all: all.length,
    running: all.filter(a => a.running).length,
    enabled: all.filter(a => a.autoStart).length,
    network: all.filter(a => a.connections && a.connections.length > 0).length,
    review: all.filter(a => a.review).length
  };
  $('app-filters').innerHTML = [['all','All apps'],['running','Running'],['enabled','Auto-start'],['network','🌐 Network'],['review','Needs review']].map(([k, l]) =>
    `<button data-filter="${k}" class="${filter === k ? 'active' : ''}">${l} <span>${cnt[k]}</span></button>`
  ).join('');

  const nComp = all.reduce((n, a) => n + a.components.length, 0);
  const ramSummary = inv.totalRAMFormatted ? `<span class="summary-separator"></span><div class="summary-item">Background RAM: <strong>${esc(inv.totalRAMFormatted)}</strong></div>` : '';
  $('app-summary').innerHTML = `<div class="summary-item"><strong>${all.length}</strong> applications detected</div><span class="summary-separator"></span><div class="summary-item"><strong>${nComp}</strong> components</div>${ramSummary}<span class="summary-separator"></span><div class="summary-item"><span class="state-led"></span>Scanned <strong>${esc(new Date(inv.scannedAt).toLocaleTimeString([], {hour:'numeric', minute:'2-digit'}))}</strong></div>`;

  const q = query.toLowerCase();
  let list = all.filter(a =>
    (filter === 'all' || (filter === 'running' && a.running) || (filter === 'enabled' && a.autoStart) || (filter === 'network' && a.connections && a.connections.length > 0) || (filter === 'review' && a.review)) &&
    (!q || (a.name + ' ' + a.publisher + ' ' + a.components.map(c => c.name + ' ' + c.display).join(' ')).toLowerCase().includes(q))
  );

  const by = {
    name: (a, b) => a.name.localeCompare(b.name),
    running: (a, b) => Number(b.running) - Number(a.running) || a.name.localeCompare(b.name),
    ram: (a, b) => (b.ramBytes || 0) - (a.ramBytes || 0) || a.name.localeCompare(b.name),
    review: (a, b) => Number(b.review) - Number(a.review) || a.name.localeCompare(b.name)
  }[sortMode] || ((a, b) => a.name.localeCompare(b.name));
  list = list.slice().sort(by);

  if (!list.find(a => a.id === sel)) sel = list.length ? list[0].id : null;
  $('visible-count').textContent = list.length + ' items shown';
  $('list-foot').textContent = 'Showing ' + list.length + ' of ' + all.length + ' detected apps';

  $('app-list').innerHTML = list.length ? list.map(a => {
    const ramBadge = a.ramFormatted ? `<span class="badge blue" style="font-size:10px;padding:1px 6px;margin-left:6px">${esc(a.ramFormatted)}</span>` : '';
    const connCount = (a.connections || []).length;
    const netBadge = connCount > 0 ? `<span class="badge ${a.telemetryCount > 0 ? 'amber' : 'green'}" style="font-size:10px;padding:1px 6px;margin-left:4px" title="${a.telemetryCount > 0 ? a.telemetryCount + ' telemetry endpoint(s) detected' : 'Active network sockets'}">🌐 ${connCount}</span>` : '';
    const reviewBadge = a.review ? `<span class="state-text review">Review</span>` : a.running ? `<span class="state-text">Running</span>` : `<span class="state-text stopped">Not running</span>`;
    const exePaths = getAppIconPaths(a);
    return `<button class="app-row ${a.id === sel ? 'selected' : ''}" data-id="${esc(a.id)}">
      <div class="app-cell"><div class="app-name-wrap">${renderAppIcon(a.name, a.letter, exePaths)}<div style="min-width:0"><div class="app-name">${esc(a.name)}${ramBadge}${netBadge}</div><div class="app-publisher">${esc(a.publisher || 'Publisher unknown')}</div></div></div></div>
      <div class="app-cell cell-secondary">${a.components.length}</div>
      <div class="app-cell">${reviewBadge}</div>
      <div class="app-cell behavior">${esc(a.behavior)}<small>${esc(a.behaviorNote)}</small></div></button>`;
  }).join('') : '<div class="empty-state">No applications match this filter.</div>';

  renderDetail(list.find(a => a.id === sel));
  animateList('#app-list .app-row');
}

function explain(a) {
  for (const k of KNOWN) {
    if (k.m.test(a.name + ' ' + a.components.map(c => c.name + ' ' + c.display).join(' '))) return k.t;
  }
  const kinds = {};
  a.components.forEach(c => kinds[c.kind] = (kinds[c.kind] || 0) + 1);
  const parts = Object.keys(kinds).map(k => kinds[k] + ' ' + KIND[k].toLowerCase() + (kinds[k] > 1 ? 's' : ''));
  let t = a.name + ' has ' + parts.join(', ') + '. ';
  t += a.autoStart ? 'At least one component starts by itself when Windows boots or you sign in.' : 'None of its components is set to start by itself at boot or sign-in.';
  if (a.review) t += ' Something about its location or signature is worth a closer look; that does not mean it is harmful.';
  return t;
}

function renderDetailNetwork(a) {
  const conns = a && a.connections ? a.connections : [];
  if (!conns.length) return '';
  const rows = conns.slice(0, 15).map(cn => {
    const dest = cn.remoteHost ? `${esc(cn.remoteHost)} (${esc(cn.remoteAddr)})` : (cn.remotePort ? `${esc(cn.remoteAddr)}:${cn.remotePort}` : esc(cn.remoteAddr));
    const telTag = cn.isTelemetry ? `<span class="badge amber" style="font-size:9px;padding:1px 5px;margin-left:5px">Telemetry</span>` : '';
    return `<div class="detail-kv" style="align-items:center">
      <span><span class="badge" style="font-size:9px;padding:1px 5px;margin-right:4px">${esc(cn.protocol)}</span>${esc(cn.state)}</span>
      <span style="word-break:break-all;text-align:right">${dest}${cn.remotePort && cn.remoteHost ? ':' + cn.remotePort : ''}${telTag}</span>
    </div>`;
  }).join('');
  const extra = conns.length > 15 ? `<div style="font-size:10.5px;color:var(--muted);margin-top:4px">+ ${conns.length - 15} more socket(s)</div>` : '';
  return `<div class="detail-section"><div class="detail-section-title">Live network &amp; telemetry (${conns.length})</div>${rows}${extra}</div>`;
}

function renderDetailNetwork(a) {
  const conns = a && a.connections ? a.connections : [];
  if (!conns.length) return '';
  const rows = conns.slice(0, 15).map(cn => {
    const dest = cn.remoteHost ? `${esc(cn.remoteHost)} (${esc(cn.remoteAddr)})` : (cn.remotePort ? `${esc(cn.remoteAddr)}:${cn.remotePort}` : esc(cn.remoteAddr));
    const telTag = cn.isTelemetry ? `<span class="badge amber" style="font-size:9px;padding:1px 5px;margin-left:5px">Telemetry</span>` : '';
    return `<div class="detail-kv" style="align-items:center">
      <span><span class="badge" style="font-size:9px;padding:1px 5px;margin-right:4px">${esc(cn.protocol)}</span>${esc(cn.state)}</span>
      <span style="word-break:break-all;text-align:right">${dest}${cn.remotePort && cn.remoteHost ? ':' + cn.remotePort : ''}${telTag}</span>
    </div>`;
  }).join('');
  const extra = conns.length > 15 ? `<div style="font-size:10.5px;color:var(--muted);margin-top:4px">+ ${conns.length - 15} more socket(s)</div>` : '';
  return `<div class="detail-section"><div class="detail-section-title">Live network &amp; telemetry (${conns.length})</div>${rows}${extra}</div>`;
}

function renderDetail(a) {
  const h = $('detail-content');
  if (!a) {
    h.innerHTML = '<div class="empty-state">Select an application to see its background components.</div>';
    return;
  }
  const ver = a.signature === 'verified';
  const ramTag = a.ramFormatted ? `<span class="small-tag good"><i data-lucide="cpu" style="width:11px;height:11px;margin-right:4px"></i>${esc(a.ramFormatted)} RAM</span>` : '';
  const detailExes = getAppIconPaths(a);
  h.innerHTML = `
  <div class="detail-head">${renderAppIcon(a.name, a.letter, detailExes, 'detail-app-icon')}<div style="min-width:0"><div class="detail-app-name">${esc(a.name)}</div><div class="detail-publisher">${esc(a.publisher || 'Publisher unknown')}</div>
   <div class="detail-tags"><span class="small-tag ${ver ? 'good' : ''}"><i data-lucide="${ver ? 'shield-check' : 'shield-alert'}" style="width:11px;height:11px;margin-right:4px"></i>${ver ? 'Verified publisher' : a.signature === 'unverified' ? 'Signature not verified' : 'Signature unknown'}</span>${ramTag}<span class="small-tag">${a.components.length} component${a.components.length === 1 ? '' : 's'}</span>${a.review ? '<span class="small-tag review">Review</span>' : ''}</div></div></div>
  <div class="detail-section"><div class="detail-section-title">Background components</div>
   ${a.components.map(c => {
     const cRam = c.ramFormatted ? `<span class="badge blue" style="font-size:10px;padding:1px 5px;margin-left:6px">${esc(c.ramFormatted)} RAM</span>` : '';
     const vtLink = c.virusTotalURL ? `<div class="detail-kv"><span>Security</span><span><a href="${esc(c.virusTotalURL)}" target="_blank" rel="noopener" style="color:var(--accent);text-decoration:underline;display:inline-flex;align-items:center;gap:4px">Check on VirusTotal <i data-lucide="external-link" style="width:11px;height:11px"></i></a></span></div>` : '';
     const shaRow = c.sha256 ? `<div class="detail-kv"><span>SHA-256</span><span style="font-family:monospace;font-size:11px">${esc(c.sha256)}</span></div>` : '';
     const isImpersonating = (c.flags || []).includes('impersonation');
     const impersonationWarning = isImpersonating ? `<div class="howto" style="color:#f87171;font-weight:600"><i data-lucide="alert-triangle" style="width:13px;height:13px;display:inline;margin-right:4px"></i>Warning: This program shares the name of a Windows system file but runs outside Windows directories!</div>` : '';
     return `<div class="component"><div class="component-head"><div class="component-symbol"><i data-lucide="${KIND_ICON[c.kind] || 'layers'}" style="width:14px;height:14px"></i></div><div class="component-copy"><div class="component-name">${esc(c.display || c.name)}${cRam}</div><div class="component-meta">${esc(KIND[c.kind] || c.kind)} · ${esc(c.trigger)}</div></div></div>
      <div class="component-bottom"><span class="component-state">${c.running ? 'Running' : 'Not running'} · ${esc(cfgText(c))}</span>${controlHtml(c)}</div>
      <details class="comp-more"><summary>Details & Security</summary>
       <div class="detail-kv"><span>Source</span><span>${esc(c.source)}</span></div>
       ${c.command ? `<div class="detail-kv"><span>Command</span><span>${esc(c.command)}</span></div>` : ''}
       <div class="detail-kv"><span>Signature</span><span>${esc(sigText(c))}</span></div>
       ${shaRow}
       ${vtLink}
       ${c.pids.length ? `<div class="detail-kv"><span>Process ID</span><span>${esc(c.pids.slice(0, 6).join(', '))}</span></div>` : ''}
       ${c.lastRun ? `<div class="detail-kv"><span>Last run</span><span>${esc(fmtTime(c.lastRun))}</span></div>` : ''}
       ${actionButtons(c)}
       ${impersonationWarning}
       ${c.flags.length ? `<div class="howto">Flags: ${esc(c.flags.map(f => ({missing:'target file missing', temp:'runs from temp/downloads', unsigned:'unsigned in user folder', impersonation:'system file name impersonation', 'winlogon-override':'non-default logon hook', 'wmi-consumer':'wmi event persistence'})[f] || f).join('; '))}.</div>` : ''}
       <div class="howto">How to change: ${esc(HOWTO[c.kind] || HOWTO.process)}</div></details></div>`;
   }).join('')}
  </div>
  <div id="detail-network-host">${renderDetailNetwork(a)}</div>
  <div id="detail-network-host">${renderDetailNetwork(a)}</div>
  <div class="detail-section"><div class="detail-section-title">About this application</div><div class="detail-explain">${esc(explain(a))}</div></div>
  <div class="detail-section"><div class="detail-section-title">Application details</div>
   <div class="detail-kv"><span>Publisher</span><span>${esc(a.publisher || 'Unknown')}</span></div>
   <div class="detail-kv"><span>Launch</span><span>${esc(a.behavior)}</span></div>
   <div class="detail-kv"><span>Location</span><span>${esc(a.path || 'Unknown')}</span></div></div>
  <div class="notice-line"><i data-lucide="shield-check" style="width:13px;height:13px;flex:none;margin-top:1px"></i><span>Changes are backed up first and can be undone from Change history. Protected components can't be changed here.</span></div>`;

  refreshIcons();
  if (window.anime) {
    anime({
      targets: '#detail-content > *',
      opacity: [0, 1],
      translateY: [5, 0],
      delay: anime.stagger(18),
      duration: 220,
      easing: 'easeOutQuad'
    });
  }
}

function stateBadge(c) {
  return `<span class="badge ${c.running ? 'green' : 'gray'}">${c.running ? 'Running' : 'Stopped'}</span>`;
}

function kvRow(a, b) {
  return `<div class="detail-kv"><span>${a}</span><span>${esc(b)}</span></div>`;
}

function expandRow(c, cols) {
  if (!expanded.has(c.id)) return '';
  const vt = c.virusTotalURL ? `<div class="detail-kv"><span>Security</span><span><a href="${esc(c.virusTotalURL)}" target="_blank" rel="noopener" style="color:var(--accent);text-decoration:underline;display:inline-flex;align-items:center;gap:4px">Check on VirusTotal <i data-lucide="external-link" style="width:11px;height:11px"></i></a></span></div>` : '';
  const sha = c.sha256 ? kvRow('SHA-256', c.sha256) : '';
  const ram = c.ramFormatted ? kvRow('RAM Usage', c.ramFormatted) : '';
  return `<tr class="expand"><td colspan="${cols}">${kvRow('Command', c.command || '—')}${kvRow('Owner app', c.appName)}${ram}${kvRow('Signature', sigText(c))}${sha}${vt}${kvRow('Source', c.source)}${c.pids.length ? kvRow('Process ID', c.pids.slice(0, 6).join(', ')) : ''}${c.locked === 'yes' ? kvRow('Locked', 'This account cannot change its settings; stopping may still work.') : ''}${kvRow('How to change', HOWTO[c.kind] || HOWTO.process)}${actionButtons(c)}</td></tr>`;
}

function renderServices() {
  const all = comps('service');
  const q = $('svc-search').value.toLowerCase();
  const t = $('svc-type').value;
  const list = all.filter(c => (!t || c.startType === t) && (!q || (c.display + ' ' + c.name + ' ' + c.publisher).toLowerCase().includes(q))).sort((a, b) => (a.display || a.name).localeCompare(b.display || b.name));

  $('svc-summary').innerHTML = `<div class="summary-item"><strong>${all.length}</strong> services shown</div><span class="summary-separator"></span><div class="summary-item"><strong>${all.filter(c => c.autoStart).length}</strong> automatic or delayed</div><span class="summary-separator"></span><div class="summary-item"><strong>${all.filter(c => c.locked === 'yes').length}</strong> locked</div>`;
  $('svc-count').textContent = list.length + ' of ' + all.length;
  $('svc-body').innerHTML = list.length ? list.map(c => {
    const ramLabel = c.ramFormatted ? `<span class="badge blue" style="font-size:9px;padding:1px 4px;margin-left:4px">${esc(c.ramFormatted)}</span>` : '';
    const vtBadge = c.virusTotalURL ? `<a href="${esc(c.virusTotalURL)}" target="_blank" rel="noopener" class="badge blue" style="font-size:9px;padding:1px 4px;margin-left:4px;text-decoration:none;display:inline-flex;align-items:center;gap:2px" title="Check service executable on VirusTotal" onclick="event.stopPropagation()"><i data-lucide="shield-check" style="width:10px;height:10px"></i>VT</a>` : '';
    return `<tr class="clickable" data-id="${esc(c.id)}" data-t="svc"><td><div class="name-cell"><div class="service-icon"><i data-lucide="cog" style="width:14px;height:14px"></i></div><div><div class="name-primary">${esc(c.display || c.name)}${ramLabel}</div><div class="name-secondary">${esc(c.name)}</div></div></div></td><td>${esc(c.publisher || '—')}${vtBadge}</td><td>${esc(c.startType)}</td><td>${stateBadge(c)}</td><td>${c.locked === 'yes' ? '<span class="badge amber">Locked</span>' : c.system ? '<span class="badge blue">Protected</span>' : '<span class="badge gray">Standard</span>'}</td></tr>${expandRow(c, 5)}`;
  }).join('') : '<tr><td colspan="5"><div class="empty-state">No services match.</div></td></tr>';

  refreshIcons();
  animateList('#svc-body tr.clickable');
}

function renderTasks() {
  const all = comps('task');
  const q = $('task-search').value.toLowerCase();
  const list = all.filter(c => !q || (c.display + ' ' + c.command + ' ' + c.publisher + ' ' + c.source).toLowerCase().includes(q)).sort((a, b) => a.name.localeCompare(b.name));

  $('task-summary').innerHTML = `<div class="summary-item"><strong>${all.length}</strong> tasks discovered</div><span class="summary-separator"></span><div class="summary-item"><strong>${all.filter(c => c.autoStart).length}</strong> start at logon or boot</div>`;
  $('task-count').textContent = list.length + ' of ' + all.length;
  $('task-body').innerHTML = list.length ? list.map(c => {
    const vtBadge = c.virusTotalURL ? `<a href="${esc(c.virusTotalURL)}" target="_blank" rel="noopener" class="badge blue" style="font-size:9.5px;padding:1px 5px;margin-left:6px;text-decoration:none;display:inline-flex;align-items:center;gap:3px" title="Check binary hash on VirusTotal" onclick="event.stopPropagation()"><i data-lucide="shield-check" style="width:10px;height:10px"></i>VT</a>` : '';
    const binaryDisplay = esc(base(c.exePath) || '—');
    return `<tr class="clickable" data-id="${esc(c.id)}" data-t="task"><td><div class="name-primary">${esc(c.display)}</div><div class="name-secondary">${esc(c.name.slice(0, c.name.length - c.display.length) || '\\')}</div></td><td>${esc(c.trigger)}</td><td>${c.review ? '<span style="color:var(--amber)">Unverified</span>' : esc(c.publisher || '—')}</td><td><span style="font-family:monospace;font-size:10.5px">${binaryDisplay}</span>${vtBadge}</td><td>${esc(fmtTime(c.lastRun))}</td><td><span class="badge ${c.review ? 'amber' : c.enabled ? 'green' : 'gray'}">${c.review ? 'Review' : c.enabled ? 'Enabled' : 'Disabled'}</span></td></tr>${expandRow(c, 6)}`;
  }).join('') : '<tr><td colspan="6"><div class="empty-state">No tasks match.</div></td></tr>';

  refreshIcons();
  animateList('#task-body tr.clickable');
}

function renderSources() {
  const badge = { scanned: ['green', 'Scanned'], partial: ['amber', 'Partial coverage'], notscanned: ['amber', 'Not scanned yet'] };
  $('source-grid').innerHTML = inv.sources.map(s => `
    <div class="source-card" onclick="openSourceInspector('${esc(s.id)}')">
      <div class="source-card-icon"><i data-lucide="${SOURCE_ICONS[s.id] || 'layers'}" style="width:16px;height:16px"></i></div>
      <div class="source-card-copy">
        <div class="source-card-title">${esc(s.title)}</div>
        <div class="source-card-desc">${esc(s.desc)}</div>
        <div class="source-card-foot">
          <span class="source-count">${s.count} found</span>
          <span class="badge ${badge[s.status][0]}">${badge[s.status][1]}</span>
          <span class="source-explore">Explore entries →</span>
        </div>
      </div>
    </div>
  `).join('');

  refreshIcons();
  animateList('#source-grid .source-card');
}

let currentSourceId = null;
let inspectorSearch = '';

function getSourceComponents(sourceId) {
  if (!inv || !inv.apps) return [];
  const list = [];
  inv.apps.forEach(a => {
    a.components.forEach(c => {
      if (c.kind === sourceId) {
        list.push(Object.assign({}, c, {appId: a.id, appName: a.name, appPublisher: a.publisher}));
      }
    });
  });
  return list;
}

function openSourceInspector(sourceId) {
  currentSourceId = sourceId;
  inspectorSearch = '';
  const searchInput = $('inspector-search');
  if (searchInput) searchInput.value = '';

  const sourceMeta = inv.sources.find(s => s.id === sourceId) || {
    id: sourceId,
    title: KIND[sourceId] || 'Startup Location',
    desc: 'Components registered in this location mechanism.'
  };

  $('source-grid-container').style.display = 'none';
  $('source-inspector').style.display = 'block';
  $('source-header-actions').style.display = 'block';
  $('source-page-title').textContent = sourceMeta.title;
  $('source-page-subtitle').textContent = sourceMeta.desc;

  renderSourceInspectorItems();
  refreshIcons();
}

function closeSourceInspector() {
  currentSourceId = null;
  $('source-grid-container').style.display = 'block';
  $('source-inspector').style.display = 'none';
  $('source-header-actions').style.display = 'none';
  $('source-page-title').textContent = 'Startup locations';
  $('source-page-subtitle').textContent = 'The Windows mechanisms scanned to discover automatic and event-triggered launches.';
  refreshIcons();
}

function renderSourceInspectorItems() {
  if (!currentSourceId) return;
  const all = getSourceComponents(currentSourceId);
  const q = inspectorSearch.toLowerCase();
  const list = all.filter(c => !q || (c.name + ' ' + (c.display || '') + ' ' + c.appName + ' ' + (c.exePath || '') + ' ' + (c.command || '')).toLowerCase().includes(q));

  const enabledCount = all.filter(c => c.enabled).length;
  const disabledCount = all.length - enabledCount;
  $('inspector-summary').innerHTML = `
    <div class="summary-item"><strong>${all.length}</strong> total entries discovered</div>
    <span class="summary-separator"></span>
    <div class="summary-item"><strong>${enabledCount}</strong> enabled / active</div>
    <span class="summary-separator"></span>
    <div class="summary-item"><strong>${disabledCount}</strong> disabled / blocked</div>
  `;
  $('inspector-count').textContent = list.length + ' of ' + all.length;

  const tbody = $('inspector-table-body');
  if (!list.length) {
    tbody.innerHTML = `<tr><td colspan="5"><div class="empty-state">No entries match the search filter.</div></td></tr>`;
    return;
  }

  tbody.innerHTML = list.map(c => {
    const isChangeable = CHANGEABLE.includes(c.kind);
    const toggleHtml = isChangeable ? `
      <input class="toggle" type="checkbox" data-cid="${esc(c.id)}" ${toggleOn(c) ? 'checked' : ''} ${c.control !== 'yes' ? 'disabled' : ''} title="${esc(c.control !== 'yes' ? (c.controlNote || 'Locked') : 'Toggle status')}" onchange="requestChange(this.dataset.cid, this.checked ? 'enable' : 'disable')">
    ` : '—';

    const revealBtn = c.exePath ? `
      <button class="win-btn" style="height:24px;font-size:10px;padding:2px 8px" onclick="event.stopPropagation();api().Reveal('${esc(c.exePath).replace(/'/g, "\\'")}')" title="Open containing folder in File Explorer"><i data-lucide="folder-search" style="width:11px;height:11px;margin-right:2px"></i>Folder</button>
    ` : '';

    const appBtn = `
      <button class="win-btn" style="height:24px;font-size:10px;padding:2px 8px" onclick="event.stopPropagation();sel='${esc(c.appId)}';navigate('applications');" title="Inspect application"><i data-lucide="layers" style="width:11px;height:11px;margin-right:2px"></i>App</button>
    `;

    const vtBtn = c.virusTotalURL ? `
      <a href="${esc(c.virusTotalURL)}" target="_blank" rel="noopener" class="win-btn" style="height:24px;font-size:10px;padding:2px 8px;text-decoration:none;color:var(--accent);display:inline-flex;align-items:center;gap:2px" title="Check SHA-256 on VirusTotal"><i data-lucide="shield-check" style="width:11px;height:11px"></i>VT</a>
    ` : '';

    const statusBadge = c.enabled ? `<span class="badge green">Enabled</span>` : `<span class="badge amber">Blocked / Off</span>`;
    const runningBadge = c.running ? `<span class="badge blue" style="font-size:9px;padding:1px 4px;margin-left:4px">Running</span>` : '';

    return `
      <tr class="clickable" data-id="${esc(c.id)}" onclick="rowToggle(event, renderSourceInspectorItems)">
        <td>
          <div class="name-cell">
            <div class="service-icon"><i data-lucide="${KIND_ICON[c.kind] || 'layers'}" style="width:14px;height:14px"></i></div>
            <div>
              <div class="name-primary">${esc(c.display || c.name)}${runningBadge}</div>
              <div class="name-secondary">${esc(c.name)}</div>
            </div>
          </div>
        </td>
        <td>
          <div class="name-primary">${esc(c.appName)}</div>
          <div class="name-secondary">${esc(c.appPublisher || '—')}</div>
        </td>
        <td style="max-width:240px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-family:monospace;font-size:11px;color:var(--text-dim)">
          ${esc(c.command || c.exePath || '—')}
        </td>
        <td>${statusBadge}</td>
        <td>
          <div style="display:flex;align-items:center;gap:6px" onclick="event.stopPropagation()">
            ${toggleHtml}
            ${revealBtn}
            ${appBtn}
            ${vtBtn}
          </div>
        </td>
      </tr>
      ${expandRow(c, 5)}
    `;
  }).join('');

  refreshIcons();
}

function renderOverview() {
  const A = apps(), svc = comps('service'), tasks = comps('task'), rev = A.filter(a => a.review);
  const running = A.filter(a => a.running).sort((a, b) => b.components.length - a.components.length).slice(0, 4);
  const row = a => {
    const ramBadge = a.ramFormatted ? `<span class="badge blue" style="font-size:10px;padding:1px 5px;margin-left:6px">${esc(a.ramFormatted)}</span>` : '';
    const rowExes = getAppIconPaths(a);
    return `<div class="overview-row">${renderAppIcon(a.name, a.letter, rowExes)}<div class="overview-row-copy"><div class="overview-row-title">${esc(a.name)}${ramBadge}</div><div class="overview-row-sub">${esc(a.publisher || 'Publisher unknown')} · ${a.components.length} component${a.components.length === 1 ? '' : 's'}</div></div><div class="overview-row-end"><span class="state-text ${a.running ? '' : 'stopped'}">${a.running ? 'Running' : 'Not running'}</span></div></div>`;
  };
  const revRow = a => {
    const vtComp = a.components.find(c => c.virusTotalURL);
    const vtBtn = vtComp ? `<a href="${esc(vtComp.virusTotalURL)}" target="_blank" rel="noopener" class="win-btn" style="height:22px;font-size:9.5px;padding:1px 6px;text-decoration:none;color:var(--accent);margin-left:6px;display:inline-flex;align-items:center;gap:3px" title="Check file hash on VirusTotal" onclick="event.stopPropagation()"><i data-lucide="shield-check" style="width:10px;height:10px"></i>VT</a>` : '';
    return `<div class="overview-row"><div class="product-icon" style="background:rgba(245,158,11,0.15);color:var(--amber);border:1px solid rgba(245,158,11,0.35)">!</div><div class="overview-row-copy"><div class="overview-row-title">${esc(a.name)}${vtBtn}</div><div class="overview-row-sub">${esc(a.components.filter(c => c.review).map(c => c.flags.map(f => ({missing:'Target file missing', temp:'Temp/Downloads folder', unsigned:'Unsigned, user folder', impersonation:'Impersonation hazard'})[f] || f).join(', ')).join(' · '))}</div></div><span class="badge amber">Review</span></div>`;
  };
  const nonSys = A.filter(a => a.autoStart).length;

  let bootBlock = '';
  if (bootProfile) {
    if (bootProfile.available) {
      const degs = (bootProfile.degradations || []).slice(0, 4);
      const title = bootProfile.bootFormatted ? 'Total Boot Time: ' + esc(bootProfile.bootFormatted) : 'System Boot Uptime';
      bootBlock = `
      <div class="overview-block">
        <div class="overview-block-head">
          <strong>Boot Performance Profiler</strong>
          <span class="badge ${bootProfile.bootFormatted ? 'blue' : 'green'}">${esc(bootProfile.bootFormatted || 'Active')}</span>
        </div>
        <div class="overview-row">
          <div class="service-icon"><i data-lucide="gauge" style="width:14px;height:14px"></i></div>
          <div class="overview-row-copy">
            <div class="overview-row-title">${title}</div>
            <div class="overview-row-sub">${esc(bootProfile.summary)}</div>
          </div>
        </div>
        ${degs.map(d => `
          <div class="overview-row">
            <div class="product-icon" style="background:var(--accent-subtle);color:var(--accent);border:1px solid var(--accent-border)">⚡</div>
            <div class="overview-row-copy">
              <div class="overview-row-title">${esc(d.name)}</div>
              <div class="overview-row-sub">${esc(d.type)} degradation · delay caused</div>
            </div>
            <span class="badge red">${esc(d.formattedDelay)}</span>
          </div>
        `).join('')}
      </div>`;
    } else if (bootProfile.requiresAdmin) {
      bootBlock = `
      <div class="overview-block">
        <div class="overview-block-head">
          <strong>Boot Performance Profiler</strong>
          <span class="badge amber">Admin Required</span>
        </div>
        <div class="overview-row">
          <div class="service-icon"><i data-lucide="shield-alert" style="width:14px;height:14px"></i></div>
          <div class="overview-row-copy">
            <div class="overview-row-title">Measure Exact App Boot Delays</div>
            <div class="overview-row-sub">Windows Event Diagnostics tracks boot delays (IDs 100-109). Administrator elevation is required to read this log.</div>
          </div>
          <button class="win-btn" style="height:26px;flex:none;margin-left:auto" onclick="doElevate()">Restart as Admin</button>
        </div>
      </div>`;
    } else {
      bootBlock = `
      <div class="overview-block">
        <div class="overview-block-head">
          <strong>Boot Performance Profiler</strong>
          <span class="badge gray">Diagnostics</span>
        </div>
        <div class="overview-row">
          <div class="service-icon"><i data-lucide="gauge" style="width:14px;height:14px"></i></div>
          <div class="overview-row-copy">
            <div class="overview-row-title">Boot Diagnostics</div>
            <div class="overview-row-sub">${esc(bootProfile.summary || bootProfile.error || 'Diagnostic log is active.')}</div>
          </div>
        </div>
      </div>`;
    }
  }

  let powerBlock = '';
  if (powerReport && powerReport.available) {
    const blockers = (powerReport.blockers || []).slice(0, 4);
    powerBlock = `
    <div class="overview-block">
      <div class="overview-block-head">
        <strong>Power & Sleep Blockers</strong>
        <span class="badge ${powerReport.hasBlockers ? 'amber' : 'green'}">${powerReport.hasBlockers ? blockers.length + ' active' : 'None'}</span>
      </div>
      ${powerReport.hasBlockers ? blockers.map(b => `
        <div class="overview-row">
          <div class="product-icon" style="background:rgba(245,158,11,0.15);color:var(--amber);border:1px solid rgba(245,158,11,0.35)">☾</div>
          <div class="overview-row-copy">
            <div class="overview-row-title">${esc(b.exePath ? base(b.exePath) : b.caller)}</div>
            <div class="overview-row-sub">${esc(b.category)} · ${esc(b.reason)}</div>
          </div>
          <span class="badge amber">Blocking Sleep</span>
        </div>
      `).join('') : `
        <div class="overview-row">
          <div class="service-icon"><i data-lucide="moon" style="width:14px;height:14px"></i></div>
          <div class="overview-row-copy">
            <div class="overview-row-title">No Sleep Blockers</div>
            <div class="overview-row-sub">No background applications or audio streams are preventing your PC or display from sleeping.</div>
          </div>
        </div>
      `}
    </div>`;
  }

  $('overview-content').innerHTML = `
  <div class="page-header"><div><h1>Overview</h1><p class="page-subtitle">A quick view of background activity and system impact on this PC.</p></div>
   <div class="header-actions">
     <button class="win-btn" onclick="doCreateRestorePoint()" title="Create a Windows System Restore Point"><i data-lucide="shield-plus" style="width:13px;height:13px;margin-right:6px"></i>Restore Point</button>
     <button class="win-btn" onclick="requestFocusMode()"><i data-lucide="zap" style="width:13px;height:13px;margin-right:6px"></i>Focus Mode</button>
     <button class="win-btn primary btn-scan" onclick="doScan()"><i data-lucide="refresh-cw" style="width:13px;height:13px;margin-right:6px"></i>Scan now</button>
   </div>
  </div>
  <div class="overview-summary">
   <div class="overview-stat"><div class="overview-stat-label">Applications</div><div class="overview-stat-value" id="stat-apps">${A.length}</div><div class="overview-stat-foot">${A.filter(a => a.running).length} currently active</div></div>
   <div class="overview-stat"><div class="overview-stat-label">Background RAM</div><div class="overview-stat-value" id="stat-ram">${inv.totalRAMFormatted || '0 KB'}</div><div class="overview-stat-foot">In-memory working set</div></div>
   <div class="overview-stat"><div class="overview-stat-label">Services</div><div class="overview-stat-value" id="stat-svc">${svc.length}</div><div class="overview-stat-foot">${svc.filter(c => c.autoStart).length} automatic or delayed</div></div>
   <div class="overview-stat"><div class="overview-stat-label">Needs attention</div><div class="overview-stat-value" id="stat-rev">${rev.length}</div><div class="overview-stat-foot ${rev.length ? 'attention' : ''}">${rev.length ? 'Location or signature needs review' : 'Nothing flagged'}</div></div></div>
  <div class="telemetry-strip">
    <div class="telemetry-card">
      <div class="telemetry-head"><span>Live CPU Utilization</span><span id="live-cpu-procs" class="telemetry-sub">Sampling…</span></div>
      <div class="telemetry-val-row"><span class="telemetry-val" id="live-cpu-val">—%</span><span class="telemetry-sub">All logical cores</span></div>
      <svg class="sparkline-svg" id="spark-cpu" viewBox="0 0 200 36" preserveAspectRatio="none"></svg>
    </div>
    <div class="telemetry-card">
      <div class="telemetry-head"><span>Physical Memory Load</span><span id="live-ram-pct" class="telemetry-sub">—%</span></div>
      <div class="telemetry-val-row"><span class="telemetry-val" id="live-ram-val">—</span><span class="telemetry-sub" id="live-ram-total">Total RAM</span></div>
      <svg class="sparkline-svg" id="spark-ram" viewBox="0 0 200 36" preserveAspectRatio="none"></svg>
    </div>
    <div class="telemetry-card">
      <div class="telemetry-head"><span>Active Network Sockets</span><span id="live-tel-count" class="telemetry-sub">0 telemetry</span></div>
      <div class="telemetry-val-row"><span class="telemetry-val" id="live-net-val">—</span><span class="telemetry-sub">TCP/UDP connections</span></div>
      <svg class="sparkline-svg" id="spark-net" viewBox="0 0 200 36" preserveAspectRatio="none"></svg>
    </div>
  </div>
  <p class="overview-intro">Background components can run without an app window. This inventory groups related entries so you can see what owns them and how they are launched. ${nonSys} application${nonSys === 1 ? '' : 's'} start automatically.</p>
  <div class="overview-grid">
   ${bootBlock}
   ${powerBlock}
   <div class="overview-block"><div class="overview-block-head"><strong>Active applications</strong><button onclick="navigate('applications')">View all applications →</button></div>${running.length ? running.map(row).join('') : '<div class="empty-state">No running background apps found.</div>'}</div>
   <div class="overview-block"><div class="overview-block-head"><strong>Review recommended</strong><button onclick="filter='review';navigate('applications');renderApps()">Review →</button></div>${rev.length ? rev.slice(0, 4).map(revRow).join('') : '<div class="overview-row"><div class="service-icon"><i data-lucide="shield-check" style="width:14px;height:14px"></i></div><div class="overview-row-copy"><div class="overview-row-title">Nothing flagged</div><div class="overview-row-sub">No temp-folder launches, missing files, or unsigned user-folder programs.</div></div></div>'}
    <div class="overview-row"><div class="service-icon"><i data-lucide="info" style="width:14px;height:14px"></i></div><div class="overview-row-copy"><div class="overview-row-title">Changes are reversible</div><div class="overview-row-sub">Each one is backed up and can be undone from Change history.</div></div></div></div>
   <div class="overview-block"><div class="overview-block-head"><strong>Recent changes</strong><button onclick="navigate('history')">View history →</button></div>${history.length ? history.slice(0, 3).map(h => `<div class="overview-row"><div class="overview-row-copy"><div class="overview-row-title">${esc(h.title)}</div><div class="overview-row-sub">${esc(h.summary)} · ${esc(fmtTime(h.time))}</div></div><span class="badge ${h.status === 'applied' ? 'green' : h.status === 'restored' ? 'gray' : 'amber'}">${esc(h.status)}</span></div>`).join('') : '<div class="empty-state">No changes yet.</div>'}</div>
   <div class="overview-block"><div class="overview-block-head"><strong>Startup locations scanned</strong><button onclick="navigate('sources')">View locations →</button></div>${inv.sources.filter(s => s.status === 'scanned').slice(0, 6).map(s => `<div class="overview-row"><div class="overview-row-copy"><div class="overview-row-title">${esc(s.title)}</div></div><span class="source-count">${s.count}</span></div>`).join('')}</div>
  </div>`;

  refreshIcons();
  animateCounters();
  animateList('.overview-block');
}

function renderHistory() {
  const ok = history.filter(h => h.status === 'applied' && h.restorable).length;
  // Group restorable entries by batchId if available
  const batchMap = {};
  history.forEach(h => {
    if (h.batchId && h.status === 'applied' && h.restorable) {
      batchMap[h.batchId] = (batchMap[h.batchId] || 0) + 1;
    }
  });

  let batchBanner = '';
  const batchKeys = Object.keys(batchMap);
  if (batchKeys.length > 0) {
    batchBanner = batchKeys.map(bId => `
      <div class="banner info" style="margin-bottom:12px">
        <i data-lucide="zap" style="width:14px;height:14px;flex:none;margin-top:2px"></i>
        <span>Focus Mode session (${batchMap[bId]} items can be reverted together)</span>
        <button class="win-btn" style="margin-left:auto;height:24px;flex:none" onclick="doRestoreBatch('${esc(bId)}')">Restore entire session</button>
      </div>
    `).join('');
  }

  $('history-summary').innerHTML = `<div class="summary-item"><strong>${history.length}</strong> changes recorded</div><span class="summary-separator"></span><div class="summary-item"><strong>${ok}</strong> can be restored</div>`;
  const ST = { applied: ['green', 'Applied'], restored: ['gray', 'Restored'], failed: ['red', 'Failed'], rolledback: ['amber', 'Rolled back'], pending: ['amber', 'Interrupted'] };
  
  $('history-list').innerHTML = batchBanner + (history.length ? history.map(h => {
    const st = ST[h.status] || ['gray', h.status];
    const batchBadge = h.batchId ? `<span class="badge blue" style="font-size:9px;padding:1px 5px;margin-left:4px">Batch</span>` : '';
    return `<div class="history-row"><div class="history-symbol"><i data-lucide="history" style="width:13px;height:13px"></i></div><div class="history-copy"><div class="history-title">${esc(h.title)}${batchBadge} <span class="badge ${st[0]}">${st[1]}</span></div><div class="history-meta">${esc(h.appName)} · ${esc(h.summary)}${h.error ? ' · ' + esc(h.error) : ''}</div></div><div class="history-time">${esc(fmtTime(h.time))}</div>${h.status === 'applied' && h.restorable ? `<button class="win-btn" data-hid="${esc(h.id)}" onclick="doRestore(this.dataset.hid)"><i data-lucide="rotate-ccw" style="width:11px;height:11px;margin-right:4px"></i>Restore</button>` : ''}</div>`;
  }).join('') : '<div class="empty-state">No changes yet. Every change you make is saved here first, so you can undo it.</div>');

  refreshIcons();
  animateList('#history-list .history-row');
}

function toggleOn(c) {
  return c.kind === 'service' ? c.autoStart : c.enabled;
}

function controlHtml(c) {
  const badge = c.locked === 'yes' ? '<span class="badge amber">Locked</span>' : c.review ? '<span class="badge amber">Review</span>' : '';
  const vtBtn = c.virusTotalURL ? `<a href="${esc(c.virusTotalURL)}" target="_blank" rel="noopener" class="win-btn" style="height:22px;font-size:9.5px;padding:1px 6px;text-decoration:none;color:var(--accent);display:inline-flex;align-items:center;gap:2px" title="Verify executable on VirusTotal"><i data-lucide="shield-check" style="width:11px;height:11px"></i>VT</a>` : '';
  if (!CHANGEABLE.includes(c.kind)) return `<span style="display:flex;gap:6px;align-items:center">${badge}${vtBtn}</span>`;
  const dis = c.control !== 'yes';
  return `<span style="display:flex;gap:6px;align-items:center">${badge}${vtBtn}<input class="toggle" type="checkbox" data-cid="${esc(c.id)}" ${toggleOn(c) ? 'checked' : ''} ${dis ? 'disabled' : ''} title="${esc(dis ? (c.controlNote || '') : 'Starts automatically')}" aria-label="Start automatically" onchange="requestChange(this.dataset.cid, this.checked ? 'enable' : 'disable')"></span>`;
}

function actionButtons(c) {
  let out = '';
  if (CHANGEABLE.includes(c.kind) && c.control === 'yes') {
    const b = (a, t) => `<button class="win-btn" style="height:26px;font-size:10px" data-cid="${esc(c.id)}" onclick="requestChange(this.dataset.cid,'${a}')">${t}</button>`;
    if (c.kind === 'service') out += c.autoStart ? b('disable', 'Set to Manual') : b('enable', 'Set to Automatic');
    else if (c.kind === 'shellex') out += c.enabled ? b('disable', 'Disable extension') : b('enable', 'Enable extension');
    else out += toggleOn(c) ? b('disable', c.kind === 'task' ? 'Disable task' : 'Disable at sign-in') : b('enable', c.kind === 'task' ? 'Enable task' : 'Enable at sign-in');
    if (c.kind === 'service' && c.canStop) out += c.running ? b('stop', 'Stop now') : b('start', 'Start now');
  }

  // VirusTotal lookup action button directly alongside disable/enable
  if (c.virusTotalURL) {
    out += `<a href="${esc(c.virusTotalURL)}" target="_blank" rel="noopener" class="win-btn" style="height:26px;font-size:10px;text-decoration:none;color:var(--accent);display:inline-flex;align-items:center;gap:4px" title="Scan or check executable on VirusTotal"><i data-lucide="shield-check" style="width:12px;height:12px"></i>Check on VirusTotal</a>`;
  }
  if (c.exePath) {
    out += `<button class="win-btn" style="height:26px;font-size:10px;display:inline-flex;align-items:center;gap:4px" onclick="event.stopPropagation();api().Reveal('${esc(c.exePath).replace(/'/g, "\\'")}')" title="Open containing folder in File Explorer"><i data-lucide="folder-search" style="width:12px;height:12px"></i>Open Folder</button>`;
  }

  // Graceful process management actions
  if (c.running && c.pids && c.pids.length > 0) {
    const pid = c.pids[0];
    out += `
      <div style="display:flex;gap:4px;align-items:center;margin-top:6px;flex-wrap:wrap">
        <button class="win-btn" style="height:24px;font-size:10px;padding:2px 8px" onclick="doProcessAction('freeze', ${pid})" title="Freeze process threads without terminating"><i data-lucide="pause" style="width:11px;height:11px;margin-right:3px"></i>Freeze</button>
        <button class="win-btn" style="height:24px;font-size:10px;padding:2px 8px" onclick="doProcessAction('resume', ${pid})" title="Resume frozen process"><i data-lucide="play" style="width:11px;height:11px;margin-right:3px"></i>Resume</button>
        <button class="win-btn" style="height:24px;font-size:10px;padding:2px 8px" onclick="doProcessAction('close', ${pid})" title="Gracefully close application"><i data-lucide="x" style="width:11px;height:11px;margin-right:3px"></i>Close</button>
        <button class="win-btn" style="height:24px;font-size:10px;padding:2px 8px;color:#f87171" onclick="doProcessAction('kill', ${pid})" title="Forcefully terminate process"><i data-lucide="square" style="width:11px;height:11px;margin-right:3px"></i>End task</button>
      </div>
    `;
  }

  const note = c.control !== 'yes' && c.controlNote ? `<div class="howto">${esc(c.controlNote)}</div>` : '';
  const caution = c.control === 'yes' && c.caution ? `<div class="howto" style="color:#e6c27e">${esc(c.caution)}</div>` : '';
  return note + caution + (out ? `<div style="display:flex;gap:6px;margin-top:8px;flex-wrap:wrap">${out}</div>` : '');
}

function requestChange(id, action) {
  if (!api()) { toast('Changes are only available in the desktop app.'); render(); return; }
  const f = findComp(id);
  if (!f) return;
  const d = describe(f.c, action);
  if (!d) return;
  pending = { id, action };
  $('modal-title').textContent = d.title;
  $('modal-body').innerHTML = d.body;
  $('modal-confirm').textContent = d.confirm;
  openModal();
}

function requestFocusMode() {
  $('modal-title').textContent = 'Activate Focus Mode?';
  $('modal-body').innerHTML = `
    <p>Focus Mode disables <strong>third-party automatic startup programs and services</strong> to speed up boot times and free background resources.</p>
    <p style="color:var(--accent)"><i data-lucide="shield-check" style="width:13px;height:13px;display:inline;margin-right:4px"></i>Windows system components and antivirus software are strictly protected and will not be touched.</p>
    <p>Every single item is backed up before being modified. You can undo this anytime from <strong>Change history</strong>.</p>
  `;
  $('modal-confirm').textContent = 'Activate Focus Mode';
  $('modal-confirm').onclick = async () => {
    closeModal();
    await doFocusMode();
  };
  openModal();
  refreshIcons();
}

function renderSentinelAlerts() {
  const host = $('sentinel-banner-host');
  if (!host) return;
  if (!sentinelAlerts || sentinelAlerts.length === 0) {
    host.innerHTML = '';
    return;
  }
  host.innerHTML = sentinelAlerts.map(a => `
    <div class="banner" style="margin-bottom:8px;display:flex;align-items:center;padding:10px 14px">
      <i data-lucide="shield-alert" style="width:16px;height:16px;flex:none;color:var(--amber);margin-right:8px"></i>
      <div style="flex:1">
        <strong>Watchdog Alert:</strong> ${esc(a.message || a.name)} (${esc(a.timestamp)})
      </div>
      <button class="win-btn primary" style="height:24px;font-size:10px;margin-left:8px" onclick="disableSentinelEntry('${esc(a.id)}')">Disable Safely</button>
      <button class="win-btn" style="height:24px;font-size:10px;margin-left:4px" onclick="dismissSentinelEntry('${esc(a.id)}')">Keep</button>
    </div>
  `).join('');
  refreshIcons();
}

function renderSnapshots() {
  const list = snapshots || [];
  const sumEl = $('snapshot-summary');
  if (sumEl) {
    sumEl.innerHTML = `
      <div class="summary-item"><strong>${list.length}</strong> baseline snapshot${list.length === 1 ? '' : 's'} saved</div>
      <span class="summary-separator"></span>
      <div class="summary-item">Live inventory: <strong>${apps().length}</strong> applications</div>
    `;
  }

  const el = $('snapshot-list');
  if (!el) return;
  if (!list.length) {
    el.innerHTML = '<div class="empty-state">No baseline snapshots saved yet. Click "Create Baseline" to save current PC state.</div>';
    return;
  }

  el.innerHTML = list.map(s => `
    <div class="overview-row" style="padding:12px 16px;border-bottom:1px solid var(--border)">
      <div class="product-icon" style="background:var(--accent-subtle);color:var(--accent);border:1px solid var(--accent-border)">📷</div>
      <div class="overview-row-copy">
        <div class="overview-row-title">${esc(s.name)}</div>
        <div class="overview-row-sub">Created ${esc(fmtTime(s.createdAt))} · ${s.appCount} apps · ${s.itemCount} background items</div>
      </div>
      <div style="display:flex;gap:6px;margin-left:auto">
        <button class="win-btn primary" style="height:26px;font-size:11px" onclick="doDiff('${esc(s.id)}')"><i data-lucide="git-compare" style="width:12px;height:12px;margin-right:4px"></i>Compare with Live</button>
        <button class="win-btn" style="height:26px;font-size:11px;color:#f87171" onclick="doDeleteSnapshot('${esc(s.id)}')"><i data-lucide="trash-2" style="width:12px;height:12px"></i></button>
      </div>
    </div>
  `).join('');

  refreshIcons();
}

function renderDiffResults(diff) {
  const container = $('diff-container');
  const content = $('diff-content');
  if (!container || !content) return;

  $('diff-heading').textContent = `Diff vs. ${diff.baseName || 'Baseline'}`;
  let html = `
    <div style="background:rgba(255,255,255,0.03);border:1px solid var(--border);border-radius:6px;padding:12px 16px;margin-bottom:16px;font-size:12px;color:var(--text);display:flex;align-items:center;gap:8px">
      <i data-lucide="info" style="width:15px;height:15px;color:#60a5fa;flex:none"></i>
      <span>${esc(diff.summary)}</span>
    </div>
  `;

  if (diff.added && diff.added.length > 0) {
    html += `<div style="font-weight:600;color:#4ade80;margin:16px 0 8px 0;display:flex;align-items:center;gap:6px"><i data-lucide="plus-circle" style="width:14px;height:14px"></i> New Items Dropped / Added (${diff.added.length})</div>`;
    html += diff.added.map(c => {
      const vtBtn = c.virusTotalURL ? `<a href="${esc(c.virusTotalURL)}" target="_blank" rel="noopener" class="win-btn" style="height:24px;font-size:10px;padding:2px 8px;text-decoration:none;color:var(--accent);display:inline-flex;align-items:center;gap:3px;margin-left:auto" title="Check binary hash on VirusTotal"><i data-lucide="shield-check" style="width:11px;height:11px"></i>VirusTotal</a>` : '';
      return `
      <div class="overview-row" style="background:rgba(34,197,94,0.07);border:1px solid rgba(34,197,94,0.25);margin-bottom:8px;border-radius:6px;padding:12px 16px;display:flex;align-items:center;justify-content:space-between">
        <div class="overview-row-copy">
          <div class="overview-row-title" style="font-weight:600;font-size:12px">${esc(c.display || c.name)} <span class="badge green" style="font-size:9.5px;padding:1px 5px;margin-left:6px">${esc(c.kind)}</span></div>
          <div class="overview-row-sub" style="margin-top:4px">${esc(c.command || c.exePath || c.source)}</div>
        </div>
        ${vtBtn}
      </div>`;
    }).join('');
  }

  if (diff.removed && diff.removed.length > 0) {
    html += `<div style="font-weight:600;color:#f87171;margin:16px 0 8px 0;display:flex;align-items:center;gap:6px"><i data-lucide="minus-circle" style="width:14px;height:14px"></i> Removed Items (${diff.removed.length})</div>`;
    html += diff.removed.map(c => `
      <div class="overview-row" style="background:rgba(239,68,68,0.07);border:1px solid rgba(239,68,68,0.25);margin-bottom:8px;border-radius:6px;padding:12px 16px">
        <div class="overview-row-copy">
          <div class="overview-row-title" style="font-weight:600;font-size:12px">${esc(c.display || c.name)} <span class="badge red" style="font-size:9.5px;padding:1px 5px;margin-left:6px">${esc(c.kind)}</span></div>
          <div class="overview-row-sub" style="margin-top:4px">${esc(c.command || c.exePath || c.source)}</div>
        </div>
      </div>
    `).join('');
  }

  if (diff.modified && diff.modified.length > 0) {
    html += `<div style="font-weight:600;color:#fbbf24;margin:16px 0 8px 0;display:flex;align-items:center;gap:6px"><i data-lucide="alert-circle" style="width:14px;height:14px"></i> Modified State (${diff.modified.length})</div>`;
    html += diff.modified.map(m => {
      const vtBtn = m.component.virusTotalURL ? `<a href="${esc(m.component.virusTotalURL)}" target="_blank" rel="noopener" class="win-btn" style="height:24px;font-size:10px;padding:2px 8px;text-decoration:none;color:var(--accent);display:inline-flex;align-items:center;gap:3px;margin-left:auto" title="Check binary hash on VirusTotal"><i data-lucide="shield-check" style="width:11px;height:11px"></i>VirusTotal</a>` : '';
      return `
      <div class="overview-row" style="background:rgba(251,191,36,0.07);border:1px solid rgba(251,191,36,0.25);margin-bottom:8px;border-radius:6px;padding:12px 16px;display:flex;align-items:center;justify-content:space-between">
        <div class="overview-row-copy">
          <div class="overview-row-title" style="font-weight:600;font-size:12px">${esc(m.component.display || m.component.name)} <span class="badge blue" style="font-size:9.5px;padding:1px 5px;margin-left:6px">${esc(m.component.kind)}</span></div>
          <div class="overview-row-sub" style="margin-top:4px">
            <span style="color:#f87171">Previous: ${esc(m.beforeState)}</span> &nbsp;→&nbsp; <span style="color:#4ade80;font-weight:600">Current: ${esc(m.afterState)}</span>
          </div>
        </div>
        ${vtBtn}
      </div>`;
    }).join('');
  }

  if (!diff.added?.length && !diff.removed?.length && !diff.modified?.length) {
    html += '<div class="empty-state" style="padding:16px 0">No differences detected! Live system matches this baseline exactly.</div>';
  }

  content.innerHTML = html;
  container.style.display = 'block';
  refreshIcons();
}

// --- Installed Apps & Deep Uninstaller View ---

function renderUninstaller() {
  const sumEl = $('uninstaller-summary');
  const countMeta = $('uninstaller-count-meta');
  const tbody = $('uninstaller-body');

  if (!installedApps || !installedApps.length) {
    if (tbody) tbody.innerHTML = '<tr><td colspan="6"><div class="empty-state">No installed programs detected.</div></td></tr>';
    updateBatchUninstallBar();
    return;
  }

  // Summary Metrics
  const totalApps = installedApps.length;
  const totalBytes = installedApps.reduce((acc, a) => acc + (a.sizeBytes || 0), 0);
  const heavyCount = installedApps.filter(a => a.hasHeavyProfile).length;
  const userCount = installedApps.filter(a => a.isUserInstall).length;

  let totalSizeStr = (uninstallerSummary && uninstallerSummary.totalSizeFormatted) || '0 B';
  if (!uninstallerSummary || !uninstallerSummary.totalSizeFormatted) {
    const kb = 1024, mb = kb * 1024, gb = mb * 1024;
    if (totalBytes >= gb) totalSizeStr = (totalBytes / gb).toFixed(1) + ' GB';
    else if (totalBytes >= mb) totalSizeStr = (totalBytes / mb).toFixed(1) + ' MB';
    else totalSizeStr = (totalBytes / kb).toFixed(1) + ' KB';
  }

  if (sumEl) {
    sumEl.innerHTML = `
      <div class="summary-item"><strong>${totalApps}</strong> programs installed</div>
      <span class="summary-separator"></span>
      <div class="summary-item">Total footprint: <strong>${esc(totalSizeStr)}</strong></div>
      <span class="summary-separator"></span>
      <div class="summary-item"><span class="badge amber" style="font-size:10px;padding:1px 6px">🔥 ${heavyCount} heavy dependencies</span></div>
      <span class="summary-separator"></span>
      <div class="summary-item">User space: <strong>${userCount}</strong></div>
    `;
  }

  // Filter items
  const q = (uninstallerSearch || '').toLowerCase();
  let list = installedApps.filter(a => {
    if (uninstallerFilter === 'heavy' && !a.hasHeavyProfile) return false;
    if (uninstallerFilter === 'large' && (a.sizeBytes || 0) < 1024 * 1024 * 1024) return false;
    if (uninstallerFilter === 'store' && a.source !== 'StoreApp') return false;
    if (uninstallerFilter === 'pkg' && !['Steam', 'Scoop', 'Chocolatey'].includes(a.source)) return false;
    if (uninstallerFilter === 'orphan' && a.source !== 'Orphaned') return false;
    if (uninstallerFilter === 'user' && !a.isUserInstall) return false;
    if (q) {
      const hay = (a.name + ' ' + a.publisher + ' ' + (a.installLocation || '')).toLowerCase();
      if (!hay.includes(q)) return false;
    }
    return true;
  });

  // Sort items
  if (uninstallerSort === 'size') {
    list.sort((a, b) => (b.sizeBytes || 0) - (a.sizeBytes || 0));
  } else if (uninstallerSort === 'name') {
    list.sort((a, b) => a.name.localeCompare(b.name));
  } else if (uninstallerSort === 'date') {
    list.sort((a, b) => (b.installDate || '').localeCompare(a.installDate || ''));
  }

  if (countMeta) {
    countMeta.textContent = `Showing ${list.length} of ${totalApps} programs`;
  }

  if (!tbody) return;

  if (list.length === 0) {
    tbody.innerHTML = '<tr><td colspan="6"><div class="empty-state">No installed programs match this filter or search.</div></td></tr>';
    updateBatchUninstallBar();
    refreshIcons();
    return;
  }

  tbody.innerHTML = list.map(a => {
    const isExpanded = expanded.has('uninst_' + a.id);
    const isChecked = typeof selectedUninstallIds !== 'undefined' && selectedUninstallIds.has(a.id);
    const heavyLabel = (a.heavyDisplaySize && a.heavyDisplaySize !== '—') ? `${esc(a.heavyDisplaySize)} models/data` : 'External Data / Cache';
    const heavyBadge = a.hasHeavyProfile
      ? `<span class="uninstaller-badge heavy" title="External models / cache detected"><i data-lucide="flame" style="width:11px;height:11px"></i>${heavyLabel}</span>`
      : '';
    const userBadge = a.isUserInstall ? `<span class="uninstaller-badge user">User</span>` : '';
    const sysBadge = a.isSystemComponent ? `<span class="uninstaller-badge sys">System</span>` : '';
    const instBadge = (a.installerType && a.installerType !== 'Standard' && a.installerType !== 'Unknown')
      ? `<span class="badge" style="font-size:9.5px;padding:1px 6px;margin-left:5px">${esc(a.installerType)}</span>`
      : '';
    const letter = (a.name || 'A').slice(0, 2).toUpperCase();

    // VirusTotal button if executable exists
    const vtBtn = a.exePath
      ? `<button class="win-btn subtle" style="height:26px;padding:0 6px" onclick="event.stopPropagation();openAppVirusTotal('${esc(a.id)}')" title="Check ${esc(base(a.exePath))} on VirusTotal"><i data-lucide="shield-check" style="width:13px;height:13px;color:var(--accent)"></i></button>`
      : '';

    // Reveal button if path exists
    const loc = a.installLocation || a.exePath;
    const revealBtn = loc
      ? `<button class="win-btn subtle" style="height:26px;padding:0 6px" onclick="event.stopPropagation();openAppLocation('${esc(a.id)}')" title="Open folder in File Explorer"><i data-lucide="folder-open" style="width:13px;height:13px"></i></button>`
      : '';

    let expandedRow = '';
    if (isExpanded) {
      const heavyDetails = a.hasHeavyProfile
        ? `<div style="margin-top:8px;padding:8px 12px;border-radius:6px;background:rgba(245,158,11,0.08);border:1px solid rgba(245,158,11,0.25);color:var(--text)">
            <strong style="color:#f59e0b">🔥 Smart Heavy Dependency Profile:</strong> External data/cache detected (e.g. model weights, SDKs, or media cache). Standard uninstallation leaves these behind; click <strong>Clean Leftovers Only</strong> or <strong>Deep Uninstall</strong> to audit exact sizes and clean safely.
           </div>`
        : '';

      expandedRow = `
        <tr class="detail-row">
          <td colspan="6" style="padding:12px 18px;background:var(--surface-hover)">
            <div style="display:flex;flex-direction:column;gap:6px;font-size:11.5px">
              <div><strong style="color:var(--text-dim)">Installation Directory:</strong> <code>${esc(a.installLocation || 'Not specified in registry')}</code></div>
              ${a.exePath ? `<div><strong style="color:var(--text-dim)">Main Executable:</strong> <code>${esc(a.exePath)}</code></div>` : ''}
              <div><strong style="color:var(--text-dim)">Uninstall Command:</strong> <code>${esc(a.uninstallString || 'None')}</code></div>
              ${a.quietUninstall ? `<div><strong style="color:var(--text-dim)">Silent Uninstall Command:</strong> <code>${esc(a.quietUninstall)}</code></div>` : ''}
              <div><strong style="color:var(--text-dim)">Registry Location:</strong> <code>${esc(a.registryPath)}</code></div>
              ${heavyDetails}
              <div style="margin-top:10px;display:flex;align-items:center;gap:8px">
                <button class="win-btn primary" onclick="event.stopPropagation();promptUninstall('${esc(a.id)}')"><i data-lucide="trash-2"></i>Deep Uninstall</button>
                <button class="win-btn" onclick="event.stopPropagation();scanAndCleanOnly('${esc(a.id)}')"><i data-lucide="sparkles"></i>Clean Leftovers Only</button>
                ${loc ? `<button class="win-btn" onclick="event.stopPropagation();openAppLocation('${esc(a.id)}')"><i data-lucide="folder-search"></i>Open Install Directory</button>` : ''}
              </div>
            </div>
          </td>
        </tr>
      `;
    }

    return `
      <tr class="clickable" data-id="uninst_${esc(a.id)}" onclick="rowToggle(event, renderUninstaller)">
        <td style="text-align:center" onclick="event.stopPropagation()">
          <input type="checkbox" class="uninst-check" ${isChecked ? 'checked' : ''} onchange="toggleUninstallSelect('${esc(a.id)}', this.checked)" aria-label="Select ${esc(a.name)}" />
        </td>
        <td>
          <div style="display:flex;align-items:center;gap:10px">
            ${renderAppIcon(a.name, letter, [a.displayIcon, a.exePath], '', 'width:26px;height:26px;font-size:10px;flex:none')}
            <div style="min-width:0">
              <div style="font-weight:600;font-size:12px;color:var(--text);display:flex;align-items:center;flex-wrap:wrap">
                ${esc(a.name)}
                ${instBadge}
                ${heavyBadge}
                ${userBadge}
                ${sysBadge}
              </div>
              ${a.version ? `<div style="font-size:10.5px;color:var(--text-dim)">v${esc(a.version)}</div>` : ''}
            </div>
          </div>
        </td>
        <td><span style="font-size:11.5px;color:var(--text-2)">${esc(a.publisher || '—')}</span></td>
        <td><span style="font-size:11px;color:var(--text-dim)">${esc(a.installDate || '—')}</span></td>
        <td style="text-align:right">
          <strong style="font-size:12px;color:var(--accent)">${esc(a.displaySize)}</strong>
        </td>
        <td style="text-align:right">
          <div style="display:inline-flex;align-items:center;gap:6px" onclick="event.stopPropagation()">
            ${vtBtn}
            ${revealBtn}
            <button class="win-btn" style="height:26px;padding:0 8px;font-size:11px" onclick="scanAndCleanOnly('${esc(a.id)}')" title="Audit and clean leftover caches & dependencies without uninstalling"><i data-lucide="sparkles" style="width:12px;height:12px;color:#10b981"></i>Clean</button>
            <button class="win-btn primary" style="height:26px;padding:0 9px;font-size:11px" onclick="promptUninstall('${esc(a.id)}')"><i data-lucide="trash-2" style="width:12px;height:12px"></i>Uninstall</button>
          </div>
        </td>
      </tr>
      ${expandedRow}
    `;
  }).join('');

  if ($('uninstaller-select-all')) {
    $('uninstaller-select-all').checked = list.length > 0 && list.every(a => selectedUninstallIds.has(a.id));
  }
  updateBatchUninstallBar();
  refreshIcons();
}

// --- Live Overview SVG Sparkline Telemetry ---

function buildSparklineSVG(values, maxCap, color) {
  if (!values || values.length === 0) return '';
  const w = 200, h = 36;
  const pts = values.length === 1 ? [values[0], values[0]] : values;
  const maxVal = maxCap > 0 ? maxCap : Math.max(10, ...pts);
  const stepX = w / (pts.length - 1);

  const coords = pts.map((v, i) => {
    const x = (i * stepX).toFixed(1);
    const ratio = Math.max(0, Math.min(1, v / maxVal));
    const y = (h - 3 - ratio * (h - 6)).toFixed(1);
    return `${x},${y}`;
  });

  const linePoints = coords.join(' ');
  const areaPoints = `0,${h} ${linePoints} ${w},${h}`;

  return `
    <polygon points="${areaPoints}" fill="${color}" fill-opacity="0.16" />
    <polyline points="${linePoints}" fill="none" stroke="${color}" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" />
  `;
}

function updateOverviewTelemetryUI() {
  if (!liveTelemetry) return;
  const cpuVal = $('live-cpu-val');
  const cpuProcs = $('live-cpu-procs');
  const ramVal = $('live-ram-val');
  const ramPct = $('live-ram-pct');
  const ramTotal = $('live-ram-total');
  const netVal = $('live-net-val');
  const telCount = $('live-tel-count');

  if (cpuVal) cpuVal.textContent = Math.round(liveTelemetry.cpuPercent || 0) + '%';
  if (cpuProcs) cpuProcs.textContent = (liveTelemetry.processCount || 0) + ' processes';
  if (ramVal) ramVal.textContent = liveTelemetry.usedPhysFormatted || '—';
  if (ramPct) ramPct.textContent = (liveTelemetry.memoryLoadPercent || 0) + '% used';
  if (ramTotal) ramTotal.textContent = 'of ' + (liveTelemetry.totalPhysFormatted || '—');
  if (netVal) netVal.textContent = liveTelemetry.totalSockets || 0;
  if (telCount) {
    const t = liveTelemetry.telemetrySockets || 0;
    telCount.textContent = t + ' telemetry';
    telCount.style.color = t > 0 ? 'var(--amber)' : '';
  }

  const sCpu = $('spark-cpu');
  const sRam = $('spark-ram');
  const sNet = $('spark-net');
  if (sCpu) sCpu.innerHTML = buildSparklineSVG(telemetryHistory.cpu, 100, 'var(--accent)');
  if (sRam) sRam.innerHTML = buildSparklineSVG(telemetryHistory.ram, 100, '#10b981');
  if (sNet) sNet.innerHTML = buildSparklineSVG(telemetryHistory.net, 0, '#f59e0b');
}

// --- Live Processes & Efficiency Mode Renderer ---

function renderLiveProcesses() {
  const sumEl = $('processes-summary');
  const countMeta = $('proc-count-meta');
  const tbody = $('proc-body');
  if (!tbody) return;

  const totalGroups = liveProcessGroups.length;
  const totalPIDs = liveProcessGroups.reduce((n, g) => n + g.processCount, 0);
  const ecoCount = liveProcessGroups.filter(g => g.ecoMode).length;
  const cpuStr = liveTelemetry ? Math.round(liveTelemetry.cpuPercent || 0) + '%' : '—';
  const ramStr = liveTelemetry ? `${liveTelemetry.usedPhysFormatted || '—'} / ${liveTelemetry.totalPhysFormatted || '—'}` : '—';

  if (sumEl) {
    sumEl.innerHTML = `
      <div class="summary-item"><strong>${totalGroups}</strong> process groups (${totalPIDs} PIDs)</div>
      <span class="summary-separator"></span>
      <div class="summary-item">System CPU: <strong>${esc(cpuStr)}</strong></div>
      <span class="summary-separator"></span>
      <div class="summary-item">Physical RAM: <strong>${esc(ramStr)}</strong></div>
      <span class="summary-separator"></span>
      <div class="summary-item"><span class="eco-pill">🍃 ${ecoCount} in Efficiency Mode</span></div>
    `;
  }

  const q = (procSearch || '').toLowerCase();
  let list = liveProcessGroups.filter(g => {
    if (procFilter === 'windowed' && !g.hasWindow) return false;
    if (procFilter === 'eco' && !g.ecoMode) return false;
    if (procFilter === 'network' && g.connCount === 0) return false;
    if (procFilter === 'frozen' && !g.suspended) return false;
    if (q) {
      const hay = (g.name + ' ' + g.exePath + ' ' + g.pids.join(' ')).toLowerCase();
      if (!hay.includes(q)) return false;
    }
    return true;
  });

  if (procSort === 'cpu') {
    list.sort((a, b) => (b.cpuPercent || 0) - (a.cpuPercent || 0) || b.ramBytes - a.ramBytes);
  } else if (procSort === 'io') {
    list.sort((a, b) => (b.ioBytesPerSec || 0) - (a.ioBytesPerSec || 0) || b.ramBytes - a.ramBytes);
  } else if (procSort === 'name') {
    list.sort((a, b) => a.name.localeCompare(b.name));
  } else {
    list.sort((a, b) => (b.ramBytes || 0) - (a.ramBytes || 0));
  }

  if (countMeta) countMeta.textContent = `Showing ${list.length} of ${totalGroups} groups · Click a row to inspect PIDs`;

  if (!list.length) {
    tbody.innerHTML = '<tr><td colspan="6"><div class="empty-state">No running processes match this filter.</div></td></tr>';
    refreshIcons();
    return;
  }

  tbody.innerHTML = list.map(g => {
    const rowId = 'proc_' + g.id;
    const isExp = expanded.has(rowId);
    const letter = (g.name || 'P').slice(0, 2).toUpperCase();
    const countTag = g.processCount > 1 ? `<span class="badge" style="font-size:10px;padding:1px 6px;margin-left:6px">${g.processCount}</span>` : '';
    const ecoTag = g.ecoMode ? `<span class="eco-pill" style="margin-left:6px">🍃 Eco</span>` : '';
    const frozenTag = g.suspended ? `<span class="badge blue" style="font-size:10px;padding:1px 6px;margin-left:6px">❄️ Frozen</span>` : '';
    const netTag = g.connCount > 0 ? `<span class="badge ${g.telemetryCount > 0 ? 'amber' : 'green'}" style="font-size:10px;padding:1px 6px;margin-left:6px">🌐 ${g.connCount}</span>` : '';
    const protTag = g.protected ? `<span class="badge gray" style="font-size:10px;padding:1px 6px;margin-left:6px">Protected</span>` : '';

    const cpuDisp = (g.cpuPercent || 0).toFixed(1) + '%';
    const cpuColor = g.cpuPercent > 25 ? 'var(--amber)' : 'var(--text)';

    const controlsHtml = g.protected
      ? `<span style="font-size:11px;color:var(--muted)">System protected</span>`
      : `<div style="display:inline-flex;align-items:center;gap:5px" onclick="event.stopPropagation()">
           <button class="win-btn" style="height:25px;padding:0 7px;font-size:10.5px" onclick="toggleGroupEcoMode('${esc(g.id).replace(/'/g, "\\'")}', ${!g.ecoMode})" title="Toggle Windows 11 Efficiency Mode (EcoQoS)">${g.ecoMode ? 'Normal' : '🍃 Eco'}</button>
           <button class="win-btn" style="height:25px;padding:0 7px;font-size:10.5px" onclick="freezeProcessGroup('${esc(g.id).replace(/'/g, "\\'")}', ${!g.suspended})" title="${g.suspended ? 'Resume process threads' : 'Freeze process without killing'}">${g.suspended ? 'Resume' : 'Freeze'}</button>
           ${g.exePath ? `<button class="win-btn subtle" style="height:25px;padding:0 6px" onclick="openProcVT('${esc(g.exePath).replace(/\\/g, '\\\\').replace(/'/g, "\\'")}')" title="Check on VirusTotal"><i data-lucide="shield-check" style="width:13px;height:13px;color:var(--accent)"></i></button>` : ''}
           ${g.exePath ? `<button class="win-btn subtle" style="height:25px;padding:0 6px" onclick="openProcLocation('${esc(g.exePath).replace(/\\/g, '\\\\').replace(/'/g, "\\'")}')" title="Open file location"><i data-lucide="folder-open" style="width:13px;height:13px"></i></button>` : ''}
           <button class="win-btn" style="height:25px;padding:0 7px;font-size:10.5px;color:#f87171" onclick="killProcessGroup('${esc(g.id).replace(/'/g, "\\'")}')" title="End all processes in this group">End</button>
         </div>`;

    let childRows = '';
    if (isExp && g.children && g.children.length) {
      childRows = `<tr class="detail-row"><td colspan="6" style="padding:10px 18px;background:var(--surface-hover)">
        <div style="font-size:11px;color:var(--text-dim);margin-bottom:8px">${g.exePath ? `<strong>Path:</strong> <code>${esc(g.exePath)}</code>` : 'System / protected binary'}</div>
        <div style="display:flex;flex-direction:column;gap:6px">
          ${g.children.slice(0, 20).map(ch => `
            <div style="display:flex;align-items:center;gap:10px;padding:6px 10px;border-radius:5px;background:var(--surface-card);border:1px solid var(--border);font-size:11px">
              <span style="min-width:70px;font-family:monospace">PID ${ch.pid}</span>
              <span style="min-width:55px">${(ch.cpuPercent || 0).toFixed(1)}% CPU</span>
              <span style="min-width:75px;font-weight:600">${esc(ch.ramFormatted)}</span>
              <span style="min-width:70px;color:var(--text-dim)">${esc(ch.ioFormatted)}</span>
              <span style="flex:1">${ch.ecoMode ? '<span class="eco-pill">🍃 Eco</span>' : ''}</span>
              ${ch.protected ? '<span style="color:var(--muted)">Protected</span>' : `
                <select class="select-control" style="height:24px;font-size:10.5px;padding:0 6px" onchange="changePidPriority(${ch.pid}, this.value)">
                  ${['Idle', 'Below Normal', 'Normal', 'Above Normal', 'High'].map(p => `<option value="${p}" ${ch.priority === p ? 'selected' : ''}>${p}</option>`).join('')}
                </select>
                <button class="win-btn" style="height:24px;font-size:10px;padding:0 7px" onclick="togglePidEcoMode(${ch.pid}, ${!ch.ecoMode})">${ch.ecoMode ? 'Normal' : '🍃 Eco'}</button>
              `}
            </div>
          `).join('')}
        </div>
      </td></tr>`;
    }

    return `
      <tr class="clickable" data-id="${esc(rowId)}" onclick="rowToggle(event, renderLiveProcesses)">
        <td>
          <div style="display:flex;align-items:center;gap:10px">
            ${renderAppIcon(g.name, letter, g.exePath, '', 'width:26px;height:26px;font-size:10px;flex:none')}
            <div style="min-width:0">
              <div style="font-weight:600;font-size:12px;color:var(--text);display:flex;align-items:center;flex-wrap:wrap">
                ${esc(g.name)}${countTag}${ecoTag}${frozenTag}${netTag}${protTag}
              </div>
              <div style="font-size:10.5px;color:var(--text-dim)">${g.hasWindow ? 'Foreground App' : 'Background Process'}</div>
            </div>
          </div>
        </td>
        <td style="text-align:right;font-weight:600;color:${cpuColor}">${cpuDisp}</td>
        <td style="text-align:right;font-weight:600;color:var(--accent)">${esc(g.ramFormatted)}</td>
        <td style="text-align:right;font-size:11px;color:var(--text-2)">${esc(g.ioFormatted)}</td>
        <td><span style="font-size:11px;color:var(--text-2)">${esc(g.priority)}</span></td>
        <td style="text-align:right">${controlsHtml}</td>
      </tr>
      ${childRows}
    `;
  }).join('');

  refreshIcons();
}

// --- File Lock Finder Renderer ---

function renderLockResults(target) {
  const meta = $('lock-count-meta');
  const tbody = $('lock-body');
  if (!tbody) return;

  if (meta) {
    meta.textContent = lockResults.length
      ? `${lockResults.length} process(es) locking "${base(target)}"`
      : `No open locks detected on "${base(target)}"`;
  }

  if (!lockResults.length) {
    tbody.innerHTML = `<tr><td colspan="5"><div class="empty-state">No running applications are currently holding a Restart Manager lock on this path.</div></td></tr>`;
    refreshIcons();
    return;
  }

  tbody.innerHTML = lockResults.map(lp => `
    <tr>
      <td><strong>${esc(lp.appName || base(lp.exePath) || 'Unknown')}</strong> ${lp.protected ? '<span class="badge gray" style="font-size:10px;margin-left:6px">Protected</span>' : ''}</td>
      <td><code>${lp.pid}</code></td>
      <td>${esc(lp.serviceName || '—')}</td>
      <td><code style="font-size:11px">${esc(lp.exePath || '—')}</code></td>
      <td style="text-align:right">
        ${lp.protected ? '<span style="font-size:11px;color:var(--muted)">System protected</span>' : `
          <div style="display:inline-flex;gap:6px">
            <button class="win-btn" style="height:26px;font-size:11px" onclick="unlockAction(${lp.pid}, 'close')">Close App</button>
            <button class="win-btn" style="height:26px;font-size:11px;color:#f87171" onclick="unlockAction(${lp.pid}, 'kill')">End Task</button>
          </div>
        `}
      </td>
    </tr>
  `).join('');

  refreshIcons();
}

