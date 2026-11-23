// --- View Renderers ---

const CHANGEABLE = ['service', 'run', 'folder', 'task', 'shellex'];

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
    review: all.filter(a => a.review).length
  };
  $('app-filters').innerHTML = [['all','All apps'],['running','Running'],['enabled','Auto-start'],['review','Needs review']].map(([k, l]) =>
    `<button data-filter="${k}" class="${filter === k ? 'active' : ''}">${l} <span>${cnt[k]}</span></button>`
  ).join('');

  const nComp = all.reduce((n, a) => n + a.components.length, 0);
  const ramSummary = inv.totalRAMFormatted ? `<span class="summary-separator"></span><div class="summary-item">Background RAM: <strong>${esc(inv.totalRAMFormatted)}</strong></div>` : '';
  $('app-summary').innerHTML = `<div class="summary-item"><strong>${all.length}</strong> applications detected</div><span class="summary-separator"></span><div class="summary-item"><strong>${nComp}</strong> components</div>${ramSummary}<span class="summary-separator"></span><div class="summary-item"><span class="state-led"></span>Scanned <strong>${esc(new Date(inv.scannedAt).toLocaleTimeString([], {hour:'numeric', minute:'2-digit'}))}</strong></div>`;

  const q = query.toLowerCase();
  let list = all.filter(a =>
    (filter === 'all' || (filter === 'running' && a.running) || (filter === 'enabled' && a.autoStart) || (filter === 'review' && a.review)) &&
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
    const reviewBadge = a.review ? `<span class="state-text review">Review</span>` : a.running ? `<span class="state-text">Running</span>` : `<span class="state-text stopped">Not running</span>`;
    return `<button class="app-row ${a.id === sel ? 'selected' : ''}" data-id="${esc(a.id)}">
      <div class="app-cell"><div class="app-name-wrap"><div class="product-icon" style="${iconStyle(a.name)}">${esc(a.letter)}</div><div style="min-width:0"><div class="app-name">${esc(a.name)}${ramBadge}</div><div class="app-publisher">${esc(a.publisher || 'Publisher unknown')}</div></div></div></div>
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

function renderDetail(a) {
  const h = $('detail-content');
  if (!a) {
    h.innerHTML = '<div class="empty-state">Select an application to see its background components.</div>';
    return;
  }
  const ver = a.signature === 'verified';
  const ramTag = a.ramFormatted ? `<span class="small-tag good"><i data-lucide="cpu" style="width:11px;height:11px;margin-right:4px"></i>${esc(a.ramFormatted)} RAM</span>` : '';
  h.innerHTML = `
  <div class="detail-head"><div class="product-icon detail-app-icon" style="${iconStyle(a.name)}">${esc(a.letter)}</div><div style="min-width:0"><div class="detail-app-name">${esc(a.name)}</div><div class="detail-publisher">${esc(a.publisher || 'Publisher unknown')}</div>
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
    return `<div class="overview-row"><div class="product-icon" style="${iconStyle(a.name)}">${esc(a.letter)}</div><div class="overview-row-copy"><div class="overview-row-title">${esc(a.name)}${ramBadge}</div><div class="overview-row-sub">${esc(a.publisher || 'Publisher unknown')} · ${a.components.length} component${a.components.length === 1 ? '' : 's'}</div></div><div class="overview-row-end"><span class="state-text ${a.running ? '' : 'stopped'}">${a.running ? 'Running' : 'Not running'}</span></div></div>`;
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
     <button class="win-btn" onclick="requestFocusMode()"><i data-lucide="zap" style="width:13px;height:13px;margin-right:6px"></i>Focus Mode</button>
     <button class="win-btn primary btn-scan" onclick="doScan()"><i data-lucide="refresh-cw" style="width:13px;height:13px;margin-right:6px"></i>Scan now</button>
   </div>
  </div>
  <div class="overview-summary">
   <div class="overview-stat"><div class="overview-stat-label">Applications</div><div class="overview-stat-value" id="stat-apps">${A.length}</div><div class="overview-stat-foot">${A.filter(a => a.running).length} currently active</div></div>
   <div class="overview-stat"><div class="overview-stat-label">Background RAM</div><div class="overview-stat-value" id="stat-ram">${inv.totalRAMFormatted || '0 KB'}</div><div class="overview-stat-foot">In-memory working set</div></div>
   <div class="overview-stat"><div class="overview-stat-label">Services</div><div class="overview-stat-value" id="stat-svc">${svc.length}</div><div class="overview-stat-foot">${svc.filter(c => c.autoStart).length} automatic or delayed</div></div>
   <div class="overview-stat"><div class="overview-stat-label">Needs attention</div><div class="overview-stat-value" id="stat-rev">${rev.length}</div><div class="overview-stat-foot ${rev.length ? 'attention' : ''}">${rev.length ? 'Location or signature needs review' : 'Nothing flagged'}</div></div></div>
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
        <strong>Persistence Sentinel Alert:</strong> ${esc(a.message || a.name)} (${esc(a.timestamp)})
      </div>
      <button class="win-btn" style="height:24px;font-size:10px;margin-left:8px" onclick="doScan()">Scan PC</button>
      <button class="win-btn" style="height:24px;font-size:10px;margin-left:4px" onclick="dismissSentinelAlert('${esc(a.id)}')">Dismiss</button>
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
