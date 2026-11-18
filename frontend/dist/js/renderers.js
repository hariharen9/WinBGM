// --- View Renderers ---

const CHANGEABLE = ['service', 'run', 'folder', 'task'];

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

  renderBanners();
  renderApps();
  renderOverview();
  renderServices();
  renderTasks();
  renderSources();
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
  $('app-summary').innerHTML = `<div class="summary-item"><strong>${all.length}</strong> applications detected</div><span class="summary-separator"></span><div class="summary-item"><strong>${nComp}</strong> background components</div><span class="summary-separator"></span><div class="summary-item"><span class="state-led"></span>Scanned <strong>${esc(new Date(inv.scannedAt).toLocaleTimeString([], {hour:'numeric', minute:'2-digit'}))}</strong></div>`;

  const q = query.toLowerCase();
  let list = all.filter(a =>
    (filter === 'all' || (filter === 'running' && a.running) || (filter === 'enabled' && a.autoStart) || (filter === 'review' && a.review)) &&
    (!q || (a.name + ' ' + a.publisher + ' ' + a.components.map(c => c.name + ' ' + c.display).join(' ')).toLowerCase().includes(q))
  );

  const by = {
    name: (a, b) => a.name.localeCompare(b.name),
    running: (a, b) => Number(b.running) - Number(a.running) || a.name.localeCompare(b.name),
    review: (a, b) => Number(b.review) - Number(a.review) || a.name.localeCompare(b.name)
  }[sortMode];
  list = list.slice().sort(by);

  if (!list.find(a => a.id === sel)) sel = list.length ? list[0].id : null;
  $('visible-count').textContent = list.length + ' items shown';
  $('list-foot').textContent = 'Showing ' + list.length + ' of ' + all.length + ' detected apps';

  $('app-list').innerHTML = list.length ? list.map(a => `<button class="app-row ${a.id === sel ? 'selected' : ''}" data-id="${esc(a.id)}">
    <div class="app-cell"><div class="app-name-wrap"><div class="product-icon" style="${iconStyle(a.name)}">${esc(a.letter)}</div><div style="min-width:0"><div class="app-name">${esc(a.name)}</div><div class="app-publisher">${esc(a.publisher || 'Publisher unknown')}</div></div></div></div>
    <div class="app-cell cell-secondary">${a.components.length}</div>
    <div class="app-cell"><span class="state-text ${a.review ? 'review' : a.running ? '' : 'stopped'}">${a.review ? 'Review' : a.running ? 'Running' : 'Not running'}</span></div>
    <div class="app-cell behavior">${esc(a.behavior)}<small>${esc(a.behaviorNote)}</small></div></button>`).join('') : '<div class="empty-state">No applications match this filter.</div>';

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
  h.innerHTML = `
  <div class="detail-head"><div class="product-icon detail-app-icon" style="${iconStyle(a.name)}">${esc(a.letter)}</div><div style="min-width:0"><div class="detail-app-name">${esc(a.name)}</div><div class="detail-publisher">${esc(a.publisher || 'Publisher unknown')}</div>
   <div class="detail-tags"><span class="small-tag ${ver ? 'good' : ''}"><i data-lucide="${ver ? 'shield-check' : 'shield-alert'}" style="width:11px;height:11px;margin-right:4px"></i>${ver ? 'Verified publisher' : a.signature === 'unverified' ? 'Signature not verified' : 'Signature unknown'}</span><span class="small-tag">${a.components.length} component${a.components.length === 1 ? '' : 's'}</span>${a.review ? '<span class="small-tag review">Review</span>' : ''}</div></div></div>
  <div class="detail-section"><div class="detail-section-title">Background components</div>
   ${a.components.map(c => `<div class="component"><div class="component-head"><div class="component-symbol"><i data-lucide="${KIND_ICON[c.kind] || 'layers'}" style="width:14px;height:14px"></i></div><div class="component-copy"><div class="component-name">${esc(c.display || c.name)}</div><div class="component-meta">${esc(KIND[c.kind])} · ${esc(c.trigger)}</div></div></div>
    <div class="component-bottom"><span class="component-state">${c.running ? 'Running' : 'Not running'} · ${esc(cfgText(c))}</span>${controlHtml(c)}</div>
    <details class="comp-more"><summary>Details</summary>
     <div class="detail-kv"><span>Source</span><span>${esc(c.source)}</span></div>
     ${c.command ? `<div class="detail-kv"><span>Command</span><span>${esc(c.command)}</span></div>` : ''}
     <div class="detail-kv"><span>Signature</span><span>${esc(sigText(c))}</span></div>
     ${c.pids.length ? `<div class="detail-kv"><span>Process ID</span><span>${esc(c.pids.slice(0, 6).join(', '))}</span></div>` : ''}
     ${c.lastRun ? `<div class="detail-kv"><span>Last run</span><span>${esc(fmtTime(c.lastRun))}</span></div>` : ''}
     ${actionButtons(c)}
     ${c.flags.length ? `<div class="howto">Flags: ${esc(c.flags.map(f => ({missing:'target file missing', temp:'runs from a temp/downloads folder', unsigned:'unsigned file in a user folder'})[f] || f).join('; '))}.</div>` : ''}
     <div class="howto">How to change: ${esc(HOWTO[c.kind])}</div></details></div>`).join('')}
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
  return `<tr class="expand"><td colspan="${cols}">${kvRow('Command', c.command || '—')}${kvRow('Owner app', c.appName)}${kvRow('Signature', sigText(c))}${kvRow('Source', c.source)}${c.pids.length ? kvRow('Process ID', c.pids.slice(0, 6).join(', ')) : ''}${c.locked === 'yes' ? kvRow('Locked', 'This account cannot change its settings; stopping may still work.') : ''}${kvRow('How to change', HOWTO[c.kind])}${actionButtons(c)}</td></tr>`;
}

function renderServices() {
  const all = comps('service');
  const q = $('svc-search').value.toLowerCase();
  const t = $('svc-type').value;
  const list = all.filter(c => (!t || c.startType === t) && (!q || (c.display + ' ' + c.name + ' ' + c.publisher).toLowerCase().includes(q))).sort((a, b) => (a.display || a.name).localeCompare(b.display || b.name));

  $('svc-summary').innerHTML = `<div class="summary-item"><strong>${all.length}</strong> services shown</div><span class="summary-separator"></span><div class="summary-item"><strong>${all.filter(c => c.autoStart).length}</strong> automatic or delayed</div><span class="summary-separator"></span><div class="summary-item"><strong>${all.filter(c => c.locked === 'yes').length}</strong> locked</div>`;
  $('svc-count').textContent = list.length + ' of ' + all.length;
  $('svc-body').innerHTML = list.length ? list.map(c => `<tr class="clickable" data-id="${esc(c.id)}" data-t="svc"><td><div class="name-cell"><div class="service-icon"><i data-lucide="cog" style="width:14px;height:14px"></i></div><div><div class="name-primary">${esc(c.display || c.name)}</div><div class="name-secondary">${esc(c.name)}</div></div></div></td><td>${esc(c.publisher || '—')}</td><td>${esc(c.startType)}</td><td>${stateBadge(c)}</td><td>${c.locked === 'yes' ? '<span class="badge amber">Locked</span>' : c.system ? '<span class="badge blue">Protected</span>' : '<span class="badge gray">Standard</span>'}</td></tr>${expandRow(c, 5)}`).join('') : '<tr><td colspan="5"><div class="empty-state">No services match.</div></td></tr>';

  refreshIcons();
  animateList('#svc-body tr.clickable');
}

function renderTasks() {
  const all = comps('task');
  const q = $('task-search').value.toLowerCase();
  const list = all.filter(c => !q || (c.display + ' ' + c.command + ' ' + c.publisher + ' ' + c.source).toLowerCase().includes(q)).sort((a, b) => a.name.localeCompare(b.name));

  $('task-summary').innerHTML = `<div class="summary-item"><strong>${all.length}</strong> tasks discovered</div><span class="summary-separator"></span><div class="summary-item"><strong>${all.filter(c => c.autoStart).length}</strong> start at logon or boot</div>`;
  $('task-count').textContent = list.length + ' of ' + all.length;
  $('task-body').innerHTML = list.length ? list.map(c => `<tr class="clickable" data-id="${esc(c.id)}" data-t="task"><td><div class="name-primary">${esc(c.display)}</div><div class="name-secondary">${esc(c.name.slice(0, c.name.length - c.display.length) || '\\')}</div></td><td>${esc(c.trigger)}</td><td>${c.review ? '<span style="color:var(--amber)">Unverified</span>' : esc(c.publisher || '—')}</td><td>${esc(base(c.exePath) || '—')}</td><td>${esc(fmtTime(c.lastRun))}</td><td><span class="badge ${c.review ? 'amber' : c.enabled ? 'green' : 'gray'}">${c.review ? 'Review' : c.enabled ? 'Enabled' : 'Disabled'}</span></td></tr>${expandRow(c, 6)}`).join('') : '<tr><td colspan="6"><div class="empty-state">No tasks match.</div></td></tr>';

  refreshIcons();
  animateList('#task-body tr.clickable');
}

function renderSources() {
  const badge = { scanned: ['green', 'Scanned'], partial: ['amber', 'Partial coverage'], notscanned: ['amber', 'Not scanned yet'] };
  $('source-grid').innerHTML = inv.sources.map(s => `<div class="source-card"><div class="source-card-icon"><i data-lucide="${SOURCE_ICONS[s.id] || 'layers'}" style="width:16px;height:16px"></i></div><div class="source-card-copy"><div class="source-card-title">${esc(s.title)}</div><div class="source-card-desc">${esc(s.desc)}</div><div class="source-card-foot"><span class="source-count">${s.status === 'notscanned' ? 'Advanced' : s.count + ' found'}</span><span class="badge ${badge[s.status][0]}">${badge[s.status][1]}</span></div></div></div>`).join('');

  refreshIcons();
  animateList('#source-grid .source-card');
}

function renderOverview() {
  const A = apps(), svc = comps('service'), tasks = comps('task'), rev = A.filter(a => a.review);
  const running = A.filter(a => a.running).sort((a, b) => b.components.length - a.components.length).slice(0, 4);
  const row = a => `<div class="overview-row"><div class="product-icon" style="${iconStyle(a.name)}">${esc(a.letter)}</div><div class="overview-row-copy"><div class="overview-row-title">${esc(a.name)}</div><div class="overview-row-sub">${esc(a.publisher || 'Publisher unknown')} · ${a.components.length} component${a.components.length === 1 ? '' : 's'}</div></div><div class="overview-row-end"><span class="state-text ${a.running ? '' : 'stopped'}">${a.running ? 'Running' : 'Not running'}</span></div></div>`;
  const revRow = a => `<div class="overview-row"><div class="product-icon" style="background:#39332a;color:#e4c27e;border-color:#5b5030">!</div><div class="overview-row-copy"><div class="overview-row-title">${esc(a.name)}</div><div class="overview-row-sub">${esc(a.components.filter(c => c.review).map(c => c.flags.map(f => ({missing:'Target file missing', temp:'Temp/Downloads folder', unsigned:'Unsigned, user folder'})[f] || f).join(', ')).join(' · '))}</div></div><span class="badge amber">Review</span></div>`;
  const nonSys = A.filter(a => a.autoStart).length;

  $('overview-content').innerHTML = `
  <div class="page-header"><div><h1>Overview</h1><p class="page-subtitle">A quick view of background activity detected on this PC.</p></div><div class="header-actions"><button class="win-btn primary btn-scan" onclick="doScan()"><i data-lucide="refresh-cw" style="width:13px;height:13px;margin-right:6px"></i>Scan now</button></div></div>
  <div class="overview-summary">
   <div class="overview-stat"><div class="overview-stat-label">Applications</div><div class="overview-stat-value" id="stat-apps">${A.length}</div><div class="overview-stat-foot">${A.filter(a => a.running).length} currently active</div></div>
   <div class="overview-stat"><div class="overview-stat-label">Services</div><div class="overview-stat-value" id="stat-svc">${svc.length}</div><div class="overview-stat-foot">${svc.filter(c => c.autoStart).length} automatic or delayed</div></div>
   <div class="overview-stat"><div class="overview-stat-label">Scheduled tasks</div><div class="overview-stat-value" id="stat-tasks">${tasks.length}</div><div class="overview-stat-foot">${tasks.filter(c => c.autoStart).length} at logon or startup</div></div>
   <div class="overview-stat"><div class="overview-stat-label">Needs attention</div><div class="overview-stat-value" id="stat-rev">${rev.length}</div><div class="overview-stat-foot ${rev.length ? 'attention' : ''}">${rev.length ? 'Location or signature needs review' : 'Nothing flagged'}</div></div></div>
  <p class="overview-intro">Background components can run without an app window. This inventory groups related entries so you can see what owns them and how they are launched. ${nonSys} application${nonSys === 1 ? '' : 's'} start automatically.</p>
  <div class="overview-grid">
   <div class="overview-block"><div class="overview-block-head"><strong>Active applications</strong><button onclick="navigate('applications')">View all applications →</button></div>${running.length ? running.map(row).join('') : '<div class="empty-state">No running background apps found.</div>'}</div>
   <div class="overview-block"><div class="overview-block-head"><strong>Review recommended</strong><button onclick="filter='review';navigate('applications');renderApps()">Review →</button></div>${rev.length ? rev.slice(0, 4).map(revRow).join('') : '<div class="overview-row"><div class="service-icon"><i data-lucide="shield-check" style="width:14px;height:14px"></i></div><div class="overview-row-copy"><div class="overview-row-title">Nothing flagged</div><div class="overview-row-sub">No temp-folder launches, missing files or unsigned user-folder programs.</div></div></div>'}
    <div class="overview-row"><div class="service-icon"><i data-lucide="info" style="width:14px;height:14px"></i></div><div class="overview-row-copy"><div class="overview-row-title">Changes are reversible</div><div class="overview-row-sub">Each one is backed up and can be undone from Change history.</div></div></div></div>
   <div class="overview-block"><div class="overview-block-head"><strong>Recent changes</strong><button onclick="navigate('history')">View history →</button></div>${history.length ? history.slice(0, 3).map(h => `<div class="overview-row"><div class="overview-row-copy"><div class="overview-row-title">${esc(h.title)}</div><div class="overview-row-sub">${esc(h.summary)} · ${esc(fmtTime(h.time))}</div></div><span class="badge ${h.status === 'applied' ? 'green' : h.status === 'restored' ? 'gray' : 'amber'}">${esc(h.status)}</span></div>`).join('') : '<div class="empty-state">No changes yet.</div>'}</div>
   <div class="overview-block"><div class="overview-block-head"><strong>Discovery coverage</strong><button onclick="navigate('sources')">View locations →</button></div>${inv.sources.filter(s => s.status !== 'notscanned').slice(0, 5).map(s => `<div class="overview-row"><div class="overview-row-copy"><div class="overview-row-title">${esc(s.title)}</div></div><span class="source-count">${s.count}</span></div>`).join('')}</div>
  </div>`;

  refreshIcons();
  animateCounters();
  animateList('.overview-block');
}

function renderHistory() {
  const ok = history.filter(h => h.status === 'applied' && h.restorable).length;
  $('history-summary').innerHTML = `<div class="summary-item"><strong>${history.length}</strong> changes recorded</div><span class="summary-separator"></span><div class="summary-item"><strong>${ok}</strong> can be restored</div>`;
  const ST = { applied: ['green', 'Applied'], restored: ['gray', 'Restored'], failed: ['red', 'Failed'], rolledback: ['amber', 'Rolled back'], pending: ['amber', 'Interrupted'] };
  
  $('history-list').innerHTML = history.length ? history.map(h => {
    const st = ST[h.status] || ['gray', h.status];
    return `<div class="history-row"><div class="history-symbol"><i data-lucide="history" style="width:13px;height:13px"></i></div><div class="history-copy"><div class="history-title">${esc(h.title)} <span class="badge ${st[0]}">${st[1]}</span></div><div class="history-meta">${esc(h.appName)} · ${esc(h.summary)}${h.error ? ' · ' + esc(h.error) : ''}</div></div><div class="history-time">${esc(fmtTime(h.time))}</div>${h.status === 'applied' && h.restorable ? `<button class="win-btn" data-hid="${esc(h.id)}" onclick="doRestore(this.dataset.hid)"><i data-lucide="rotate-ccw" style="width:11px;height:11px;margin-right:4px"></i>Restore</button>` : ''}</div>`;
  }).join('') : '<div class="empty-state">No changes yet. Every change you make is saved here first, so you can undo it.</div>';

  refreshIcons();
  animateList('#history-list .history-row');
}

function toggleOn(c) {
  return c.kind === 'service' ? c.autoStart : c.enabled;
}

function controlHtml(c) {
  const badge = c.locked === 'yes' ? '<span class="badge amber">Locked</span>' : c.review ? '<span class="badge amber">Review</span>' : '';
  if (!CHANGEABLE.includes(c.kind)) return badge;
  const dis = c.control !== 'yes';
  return `<span style="display:flex;gap:8px;align-items:center">${badge}<input class="toggle" type="checkbox" data-cid="${esc(c.id)}" ${toggleOn(c) ? 'checked' : ''} ${dis ? 'disabled' : ''} title="${esc(dis ? (c.controlNote || '') : 'Starts automatically')}" aria-label="Start automatically" onchange="requestChange(this.dataset.cid, this.checked ? 'enable' : 'disable')"></span>`;
}

function actionButtons(c) {
  if (!CHANGEABLE.includes(c.kind)) return '';
  const b = (a, t) => `<button class="win-btn" style="height:26px;font-size:10px" data-cid="${esc(c.id)}" onclick="requestChange(this.dataset.cid,'${a}')">${t}</button>`;
  let out = '';
  if (c.control === 'yes') {
    if (c.kind === 'service') out += c.autoStart ? b('disable', 'Set to Manual') : b('enable', 'Set to Automatic');
    else out += toggleOn(c) ? b('disable', c.kind === 'task' ? 'Disable task' : 'Disable at sign-in') : b('enable', c.kind === 'task' ? 'Enable task' : 'Enable at sign-in');
  }
  if (c.kind === 'service' && c.canStop) out += c.running ? b('stop', 'Stop now') : b('start', 'Start now');
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
