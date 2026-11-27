// --- Main Application Controller & Event Wiring ---

function toast(m) {
  const t = $('toast');
  t.textContent = m;
  t.classList.add('show');
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => t.classList.remove('show'), 3200);
}

function renderCommandBar(p) {
  const host = $('commandbar-actions');
  if (!host) return;
  const presets = {
    overview: [
      { id: 'scan-btn', cls: 'btn-scan', icon: 'refresh-cw', label: 'Scan now', fn: 'doScan()' },
      { icon: 'zap', label: 'Focus Mode', fn: 'requestFocusMode()' },
      { icon: 'download', label: 'Export', fn: 'openExportModal()' },
    ],
    processes: [
      { icon: 'refresh-cw', label: 'Refresh', fn: 'loadLiveProcesses()' },
      { icon: 'zap', label: 'Focus Mode', fn: 'requestFocusMode()' },
    ],
    applications: [
      { id: 'scan-btn', cls: 'btn-scan', icon: 'refresh-cw', label: 'Scan now', fn: 'doScan()' },
      { icon: 'plus', label: 'New Startup Entry', fn: 'openCustomStartupModal()' },
      { icon: 'zap', label: 'Focus Mode', fn: 'requestFocusMode()' },
      { icon: 'folder-search', label: 'Open location', fn: 'doReveal()' },
      { icon: 'download', label: 'Export', fn: 'openExportModal()' },
    ],
    uninstaller: [
      { icon: 'refresh-cw', label: 'Rescan Programs', fn: 'loadInstalledApps()' },
      { icon: 'download', label: 'Export', fn: 'openExportModal()' },
    ],
    services: [
      { id: 'scan-btn', cls: 'btn-scan', icon: 'refresh-cw', label: 'Scan now', fn: 'doScan()' },
      { icon: 'download', label: 'Export', fn: 'openExportModal()' },
    ],
    tasks: [
      { id: 'scan-btn', cls: 'btn-scan', icon: 'refresh-cw', label: 'Scan now', fn: 'doScan()' },
      { icon: 'clock', label: 'New Delayed Task', fn: 'openCustomStartupModal()' },
      { icon: 'download', label: 'Export', fn: 'openExportModal()' },
    ],
    sources: [
      { id: 'scan-btn', cls: 'btn-scan', icon: 'refresh-cw', label: 'Scan now', fn: 'doScan()' },
      { icon: 'plus', label: 'New Startup Entry', fn: 'openCustomStartupModal()' },
      { icon: 'download', label: 'Export', fn: 'openExportModal()' },
    ],
    locks: [
      { icon: 'folder-open', label: 'Browse File', fn: 'browseLockedFile()' },
      { icon: 'search', label: 'Check Locks', fn: 'checkFileLocks()' },
    ],
    snapshots: [
      { icon: 'camera', label: 'Take Snapshot', fn: 'promptCreateSnapshot()' },
      { icon: 'refresh-cw', label: 'Refresh', fn: 'loadSnapshots()' },
    ],
    history: [
      { icon: 'refresh-cw', label: 'Refresh', fn: 'loadHistory()' },
      { icon: 'download', label: 'Export', fn: 'openExportModal()' },
    ],
    settings: [
      { icon: 'shield-plus', label: 'Create Restore Point', fn: 'doCreateRestorePoint()' },
      { icon: 'monitor', label: 'System Info', fn: 'openSystemAboutModal()' },
    ],
  };
  const items = presets[p] || presets.overview;
  host.innerHTML = items.map(b =>
    `<button class="command-btn ${b.cls || ''}"${b.id ? ` id="${b.id}"` : ''} onclick="${b.fn}"><i data-lucide="${b.icon}"></i>${b.label}</button>`
  ).join('');
}

function navigate(p) {
  page = p;
  document.querySelectorAll('.page').forEach(x => x.classList.toggle('active', x.id === 'page-' + p));
  document.querySelectorAll('.nav-item').forEach(b => b.classList.toggle('active', b.dataset.page === p));
  const labels = {
    overview: 'Overview',
    processes: 'Live Processes & Efficiency Mode',
    applications: 'Applications',
    uninstaller: 'Installed Programs & Deep Uninstaller',
    services: 'Services',
    tasks: 'Scheduled tasks',
    sources: 'Startup locations',
    locks: 'File Lock Finder',
    snapshots: 'Snapshots & Diff Comparison',
    history: 'Change history',
    settings: 'Settings'
  };
  $('commandbar-label').textContent = labels[p] || 'WinBGM';
  renderCommandBar(p);
  $('content-host').scrollTop = 0;
  if (p === 'applications') renderApps();
  if (p === 'processes') loadLiveProcesses();
  if (p === 'uninstaller') loadInstalledApps();
  if (p === 'history') loadHistory();
  if (p === 'snapshots') loadSnapshots();
  if (p === 'overview') animateCounters();
  refreshIcons();
}

function applyScale(scale) {
  if (!SCALES.includes(scale)) scale = '100%';
  uiScale = scale;
  document.documentElement.style.zoom = scale;
  try { localStorage.setItem('bgm.uiScale', scale); } catch (e) {}
  const el = $('set-ui-scale');
  if (el && el.value !== scale) el.value = scale;
}

function stepScale(delta) {
  let idx = SCALES.indexOf(uiScale);
  if (idx === -1) idx = 0;
  const next = Math.max(0, Math.min(SCALES.length - 1, idx + delta));
  applyScale(SCALES[next]);
  toast('Text size: ' + SCALES[next]);
}

function rowToggle(e, fn) {
  const r = e.target.closest('tr.clickable');
  if (!r) return;
  const id = r.dataset.id;
  expanded.has(id) ? expanded.delete(id) : expanded.add(id);
  fn();
}

function openExportModal() {
  $('modal-title').textContent = 'Export Background Inventory Report';
  $('modal-body').innerHTML = `
    <p style="margin-bottom:12px">Choose an export format for this machine's background inventory:</p>
    <div style="display:flex;flex-direction:column;gap:10px">
      <button class="win-btn" onclick="closeModal();doExport()" style="display:flex;align-items:center;justify-content:flex-start;padding:10px 14px;height:auto;gap:10px;text-align:left">
        <i data-lucide="file-json" style="width:18px;height:18px;color:#38bdf8;flex:none"></i>
        <div><strong style="display:block;font-size:13px">JSON Format (*.json)</strong><span style="font-size:11px;color:var(--text-dim)">Complete raw system inventory data for backups or automated tools</span></div>
      </button>
      <button class="win-btn" onclick="closeModal();doExportMarkdown()" style="display:flex;align-items:center;justify-content:flex-start;padding:10px 14px;height:auto;gap:10px;text-align:left">
        <i data-lucide="file-text" style="width:18px;height:18px;color:#34d399;flex:none"></i>
        <div><strong style="display:block;font-size:13px">Markdown Audit Report (*.md)</strong><span style="font-size:11px;color:var(--text-dim)">Structured tables showing apps, RAM usage, and security status</span></div>
      </button>
      <button class="win-btn" onclick="closeModal();doExportCSV()" style="display:flex;align-items:center;justify-content:flex-start;padding:10px 14px;height:auto;gap:10px;text-align:left">
        <i data-lucide="table" style="width:18px;height:18px;color:#fbbf24;flex:none"></i>
        <div><strong style="display:block;font-size:13px">CSV Spreadsheet (*.csv)</strong><span style="font-size:11px;color:var(--text-dim)">Tabular spreadsheet of all background components for Excel/Sheets</span></div>
      </button>
    </div>
  `;
  $('modal-confirm').style.display = 'none';
  $('modal-cancel').textContent = 'Close';
  openModal();
  refreshIcons();
}

// Event Listeners
document.querySelectorAll('.nav-item').forEach(b => b.addEventListener('click', () => navigate(b.dataset.page)));

$('app-filters').addEventListener('click', e => {
  const b = e.target.closest('button[data-filter]');
  if (b) { filter = b.dataset.filter; renderApps(); }
});

$('app-list').addEventListener('click', e => {
  const r = e.target.closest('.app-row');
  if (r) {
    sel = r.dataset.id;
    document.querySelectorAll('#app-list .app-row').forEach(row => {
      row.classList.toggle('selected', row.dataset.id === sel);
    });
    const currentApp = apps().find(a => a.id === sel);
    renderDetail(currentApp);
  }
});

$('app-search').addEventListener('input', e => { query = e.target.value; renderApps(); });
$('app-sort').addEventListener('change', e => { sortMode = e.target.value; renderApps(); });

$('svc-search').addEventListener('input', renderServices);
$('svc-type').addEventListener('change', renderServices);
$('task-search').addEventListener('input', renderTasks);
if ($('inspector-search')) {
  $('inspector-search').addEventListener('input', e => {
    inspectorSearch = e.target.value;
    renderSourceInspectorItems();
  });
}

// Uninstaller Toolbar Listeners
if ($('uninstaller-filters')) {
  $('uninstaller-filters').addEventListener('click', e => {
    const btn = e.target.closest('button[data-filter]');
    if (btn) {
      document.querySelectorAll('#uninstaller-filters button').forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      uninstallerFilter = btn.dataset.filter;
      renderUninstaller();
    }
  });
}

if ($('uninstaller-search')) {
  $('uninstaller-search').addEventListener('input', e => {
    uninstallerSearch = e.target.value;
    renderUninstaller();
  });
}

if ($('uninstaller-sort')) {
  $('uninstaller-sort').addEventListener('change', e => {
    uninstallerSort = e.target.value;
    renderUninstaller();
  });
}

// Live Processes Toolbar Listeners
if ($('proc-filters')) {
  $('proc-filters').addEventListener('click', e => {
    const btn = e.target.closest('button[data-filter]');
    if (btn) {
      document.querySelectorAll('#proc-filters button').forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      procFilter = btn.dataset.filter;
      renderLiveProcesses();
    }
  });
}

if ($('proc-search')) {
  $('proc-search').addEventListener('input', e => {
    procSearch = e.target.value;
    renderLiveProcesses();
  });
}

if ($('proc-sort')) {
  $('proc-sort').addEventListener('change', e => {
    procSort = e.target.value;
    renderLiveProcesses();
  });
}

if ($('lock-path-input')) {
  $('lock-path-input').addEventListener('keydown', e => {
    if (e.key === 'Enter') checkFileLocks();
  });
}

$('svc-body').addEventListener('click', e => rowToggle(e, renderServices));
$('task-body').addEventListener('click', e => rowToggle(e, renderTasks));

$('modal-confirm').addEventListener('click', confirmChange);
$('modal-cancel').addEventListener('click', closeModal);
$('confirm-modal').addEventListener('click', e => { if (e.target === $('confirm-modal')) closeModal(); });
if ($('residual-modal')) {
  $('residual-modal').addEventListener('click', e => { if (e.target === $('residual-modal')) closeResidualModal(); });
}
if ($('sysabout-modal')) {
  $('sysabout-modal').addEventListener('click', e => { if (e.target === $('sysabout-modal')) closeSystemAboutModal(); });
}
if ($('custom-startup-modal')) {
  $('custom-startup-modal').addEventListener('click', e => { if (e.target === $('custom-startup-modal')) closeCustomStartupModal(); });
}
document.addEventListener('keydown', e => {
  if (e.key !== 'Escape') return;
  if ($('custom-startup-modal') && $('custom-startup-modal').classList.contains('show')) {
    e.preventDefault();
    closeCustomStartupModal();
    return;
  }
  if ($('sysabout-modal') && $('sysabout-modal').classList.contains('show')) {
    e.preventDefault();
    closeSystemAboutModal();
    return;
  }
  if ($('residual-modal') && $('residual-modal').classList.contains('show')) {
    e.preventDefault();
    closeResidualModal();
    return;
  }
  if ($('confirm-modal') && $('confirm-modal').classList.contains('show')) {
    e.preventDefault();
    closeModal();
    return;
  }
  if (page === 'sources' && $('source-inspector') && $('source-inspector').style.display !== 'none') {
    e.preventDefault();
    closeSourceInspector();
    return;
  }
  if (page === 'snapshots' && $('diff-container') && $('diff-container').style.display !== 'none') {
    e.preventDefault();
    $('diff-container').style.display = 'none';
    return;
  }
  const active = document.activeElement;
  if (active && (active.tagName === 'INPUT' || active.tagName === 'TEXTAREA' || active.tagName === 'SELECT')) {
    active.blur();
    return;
  }
  if (expanded.size > 0) {
    expanded.clear();
    render();
    return;
  }
  if (page === 'uninstaller' && typeof selectedUninstallIds !== 'undefined' && selectedUninstallIds.size > 0) {
    clearUninstallSelection();
    return;
  }
  if (page !== 'overview') {
    navigate('overview');
  }
});

$('set-system').addEventListener('change', e => {
  showSystem = e.target.checked;
  try { localStorage.setItem('bgm.showSystem', showSystem ? '1' : '0'); } catch (x) {}
  render();
});

if ($('set-theme')) {
  $('set-theme').value = currentTheme;
  $('set-theme').addEventListener('change', e => {
    applyTheme(e.target.value);
    toast('Theme set to ' + e.target.options[e.target.selectedIndex].text);
    renderApps();
  });
}

if ($('accent-picker')) {
  $('accent-picker').addEventListener('click', e => {
    const swatch = e.target.closest('.accent-swatch');
    if (swatch && swatch.dataset.color) {
      applyAccent(swatch.dataset.color);
      toast('Accent color set to ' + swatch.title);
    }
  });
}

if (window.matchMedia) {
  window.matchMedia('(prefers-color-scheme: light)').addEventListener('change', () => {
    if (currentTheme === 'system') {
      renderApps();
    }
  });
}

if ($('set-ui-scale')) {
  $('set-ui-scale').addEventListener('change', e => {
    applyScale(e.target.value);
    toast('Text size: ' + e.target.value);
  });
}

if ($('set-always-admin')) {
  $('set-always-admin').addEventListener('change', async e => {
    const on = e.target.checked;
    const a = api();
    if (!a) {
      if (inv) inv.alwaysAdmin = on;
      toast(on ? 'Always start as administrator enabled (sample preview).' : 'Always start as administrator disabled (sample preview).');
      return;
    }
    try {
      await a.SetAlwaysAdmin(on);
      if (inv) inv.alwaysAdmin = on;
      if (on) {
        toast('Always start as administrator enabled.');
        if (inv && !inv.elevated) {
          pending = null;
          $('modal-title').textContent = 'Restart as administrator?';
          $('modal-body').innerHTML = '<p>WinBGM has been configured to always run with administrator rights.</p><p>Would you like to restart the app as administrator right now to unlock service and task management?</p>';
          $('modal-confirm').textContent = 'Restart as administrator';
          $('modal-confirm').onclick = async () => {
            closeModal();
            await doElevate();
          };
          openModal();
        }
      } else {
        toast('Always start as administrator disabled.');
      }
    } catch (err) {
      toast('Failed to change setting: ' + err);
      e.target.checked = !on;
    }
  });
}

if ($('set-minimize-tray')) {
  const a = api();
  if (a && a.GetMinimizeToTray) {
    a.GetMinimizeToTray().then(v => { $('set-minimize-tray').checked = !!v; }).catch(() => {});
  }
  $('set-minimize-tray').addEventListener('change', async e => {
    const on = e.target.checked;
    const a = api();
    if (!a || !a.SetMinimizeToTray) {
      toast(on ? 'Minimize to System Tray enabled (sample preview).' : 'Minimize to System Tray disabled (sample preview).');
      return;
    }
    try {
      await a.SetMinimizeToTray(on);
      toast(on ? 'WinBGM will stay active in the System Tray when closed.' : 'Closing the window will now exit WinBGM.');
    } catch (err) {
      toast('Failed to update tray setting: ' + err);
      e.target.checked = !on;
    }
  });
}

$('global-search').addEventListener('keydown', e => {
  if (e.key === 'Enter') {
    navigate('applications');
    query = e.target.value;
    $('app-search').value = query;
    renderApps();
  }
});

document.addEventListener('keydown', e => {
  if (e.ctrlKey || e.metaKey) {
    if (e.key.toLowerCase() === 'k') {
      e.preventDefault();
      $('global-search').focus();
    } else if (e.key === '=' || e.key === '+') {
      e.preventDefault();
      stepScale(1);
    } else if (e.key === '-' || e.key === '_') {
      e.preventDefault();
      stepScale(-1);
    } else if (e.key === '0') {
      e.preventDefault();
      applyScale('100%');
      toast('Text size reset to 100%');
    }
  }
});

let lastSystemAbout = null;

function formatDriveBytes(b) {
  const tb = 1024 ** 4;
  const gb = 1024 ** 3;
  if (b >= tb) return (b / tb).toFixed(2) + ' TB';
  return (b / gb).toFixed(1) + ' GB';
}

async function openSystemAboutModal() {
  const modal = $('sysabout-modal');
  const body = $('sysabout-body');
  const sub = $('sysabout-subtitle');
  if (!modal || !body) return;

  modal.classList.add('show');
  if (!lastSystemAbout) {
    body.innerHTML = '<div style="padding: 20px; text-align: center; color: var(--text-2); font-size: 12px;">Reading hardware &amp; Windows specifications…</div>';
  }
  try {
    const info = await window.go.main.App.GetSystemAbout();
    lastSystemAbout = info;
    $('sysabout-title').textContent = info.deviceName || 'This PC';
    const board = [info.manufacturer, info.motherboard].filter(Boolean).join(' · ');
    sub.textContent = board ? `${board} · ${info.edition || 'Windows'}` : (info.edition || 'Windows');

    const cpuSub = [info.processorSpeed, info.logicalCores ? `${info.logicalCores} logical processors` : ''].filter(Boolean).join(' · ');
    const ramDisplay = info.ramSummary || 'Unknown';
    const gpuVal = info.gpuVram || 'Integrated';
    const gpuSub = info.gpuName || 'Display adapter';

    const deviceRows = [
      ['Device name', info.deviceName],
      ['Motherboard', board],
      ['Processor', info.processorName + (info.processorSpeed ? ` (${info.processorSpeed})` : '')],
      ['Installed RAM', info.ramSummary + (info.ramSpeed ? ` (${info.ramSpeed.replace(/^Speed:\s*/i, '')})` : '')],
      ['Graphics card', info.gpuName + (info.gpuVram ? ` (${info.gpuVram})` : '')],
      ['Storage', info.storageUsed || info.storageTotal],
      ['Device ID', info.deviceId],
      ['Product ID', info.productId],
      ['System type', info.systemType],
    ].filter(([, v]) => v);

    const winRows = [
      ['Edition', info.edition],
      ['Version', info.version],
      ['Installed on', info.installedOn],
      ['OS build', info.osBuild],
      ['Experience', info.experience],
    ].filter(([, v]) => v);

    const drivesHtml = (info.drives || []).map(d => {
      const pct = d.total > 0 ? Math.min(100, Math.round((d.used / d.total) * 100)) : 0;
      return `
        <div class="sysabout-drive-row">
          <strong style="color: var(--text)">${esc(d.letter)}</strong>
          <div class="sysabout-drive-bar"><div class="sysabout-drive-fill" style="width:${pct}%"></div></div>
          <span style="text-align:right; color: var(--text-2)">${formatDriveBytes(d.used)} / ${formatDriveBytes(d.total)}</span>
        </div>`;
    }).join('');

    body.innerHTML = `
      <div class="sysabout-cards">
        <div class="sysabout-hw-card">
          <div class="sysabout-hw-label"><i data-lucide="cpu"></i> Processor</div>
          <div class="sysabout-hw-val">${esc(info.processorName || 'Unknown')}</div>
          <div class="sysabout-hw-sub">${esc(cpuSub)}</div>
        </div>
        <div class="sysabout-hw-card">
          <div class="sysabout-hw-label"><i data-lucide="memory-stick"></i> Installed RAM</div>
          <div class="sysabout-hw-val">${esc(ramDisplay)}</div>
          <div class="sysabout-hw-sub">${esc(info.ramSpeed || 'System memory')}</div>
        </div>
        <div class="sysabout-hw-card">
          <div class="sysabout-hw-label"><i data-lucide="monitor"></i> Graphics card</div>
          <div class="sysabout-hw-val">${esc(gpuVal)}</div>
          <div class="sysabout-hw-sub">${esc(gpuSub)}</div>
        </div>
        <div class="sysabout-hw-card">
          <div class="sysabout-hw-label"><i data-lucide="hard-drive"></i> Storage</div>
          <div class="sysabout-hw-val">${esc(info.storageTotal || 'Unknown')}</div>
          <div class="sysabout-hw-sub">${esc(info.storageUsed || '')}</div>
        </div>
      </div>

      <div class="sysabout-section">
        <div class="sysabout-section-head">
          <div class="sysabout-section-title"><i data-lucide="info"></i> Device specifications</div>
          <button class="win-btn" style="padding: 3px 10px; font-size: 11px" onclick="copySystemAboutSection('device')">Copy</button>
        </div>
        <div class="sysabout-grid">
          ${deviceRows.map(([k, v]) => `<div class="sysabout-k">${esc(k)}</div><div class="sysabout-v">${esc(v)}</div>`).join('')}
        </div>
      </div>

      <div class="sysabout-section">
        <div class="sysabout-section-head">
          <div class="sysabout-section-title"><i data-lucide="layout-grid"></i> Windows specifications</div>
          <button class="win-btn" style="padding: 3px 10px; font-size: 11px" onclick="copySystemAboutSection('windows')">Copy</button>
        </div>
        <div class="sysabout-grid">
          ${winRows.map(([k, v]) => `<div class="sysabout-k">${esc(k)}</div><div class="sysabout-v">${esc(v)}</div>`).join('')}
        </div>
      </div>

      ${drivesHtml ? `
      <div class="sysabout-section">
        <div class="sysabout-section-head" style="margin-bottom: 8px">
          <div class="sysabout-section-title"><i data-lucide="hard-drive"></i> Local drives</div>
        </div>
        <div class="sysabout-drives">${drivesHtml}</div>
      </div>` : ''}
    `;

    if (window.lucide && window.lucide.createIcons) {
      window.lucide.createIcons({ nodes: [body] });
    }
  } catch (err) {
    body.innerHTML = `<div style="padding: 16px; color: #ef4444; font-size: 12px;">Failed to load system details: ${esc(String(err))}</div>`;
  }
}

function closeSystemAboutModal() {
  const modal = $('sysabout-modal');
  if (modal) modal.classList.remove('show');
}

function copySystemAboutSection(section) {
  if (!lastSystemAbout) return;
  const info = lastSystemAbout;
  let lines = [];
  if (section === 'device') {
    lines = [
      `Device name\t${info.deviceName || ''}`,
      `Motherboard\t${[info.manufacturer, info.motherboard].filter(Boolean).join(' ')}`,
      `Processor\t${info.processorName || ''} ${info.processorSpeed ? '(' + info.processorSpeed + ')' : ''}`,
      `Installed RAM\t${info.ramSummary || ''} ${info.ramSpeed ? '(' + info.ramSpeed + ')' : ''}`,
      `Graphics card\t${info.gpuName || ''} ${info.gpuVram ? '(' + info.gpuVram + ')' : ''}`,
      `Storage\t${info.storageUsed || info.storageTotal || ''}`,
      `Device ID\t${info.deviceId || ''}`,
      `Product ID\t${info.productId || ''}`,
      `System type\t${info.systemType || ''}`,
    ];
  } else {
    lines = [
      `Edition\t${info.edition || ''}`,
      `Version\t${info.version || ''}`,
      `Installed on\t${info.installedOn || ''}`,
      `OS build\t${info.osBuild || ''}`,
      `Experience\t${info.experience || ''}`,
    ];
  }
  navigator.clipboard.writeText(lines.join('\n')).then(() => {
    toast(section === 'device' ? 'Device specifications copied' : 'Windows specifications copied');
  });
}

function openCustomStartupModal() {
  const modal = $('custom-startup-modal');
  if (!modal) return;
  $('cs-name').value = '';
  $('cs-command').value = '';
  $('cs-args').value = '';
  $('cs-delay').value = '30';
  $('cs-hidden').checked = false;
  $('cs-elevated').checked = false;
  modal.classList.add('show');
  refreshIcons();
  setTimeout(() => $('cs-name').focus(), 40);
}

function closeCustomStartupModal() {
  const modal = $('custom-startup-modal');
  if (modal) modal.classList.remove('show');
}

function setCustomStartupDelay(sec) {
  if ($('cs-delay')) $('cs-delay').value = String(sec);
}

async function browseStartupCommand() {
  try {
    const p = await window.go.main.App.BrowseStartupExecutable();
    if (p) {
      $('cs-command').value = p;
      if (!$('cs-name').value.trim()) {
        const baseName = p.split(/[\\/]/).pop().replace(/\.[^.]+$/, '');
        $('cs-name').value = baseName;
      }
    }
  } catch (err) {
    toast('Failed to open file picker: ' + err);
  }
}

async function submitCustomStartup() {
  const name = ($('cs-name').value || '').trim();
  const command = ($('cs-command').value || '').trim();
  const args = ($('cs-args').value || '').trim();
  const delaySeconds = Math.max(0, parseInt($('cs-delay').value || '0', 10) || 0);
  const runHidden = $('cs-hidden').checked;
  const elevated = $('cs-elevated').checked;

  if (!name || !command) {
    toast('Please enter both an Entry Name and a Program/Command.');
    return;
  }

  const btn = $('cs-submit-btn');
  if (btn) btn.disabled = true;
  try {
    await window.go.main.App.CreateCustomStartup({
      name,
      command,
      args,
      workingDir: '',
      delaySeconds,
      runHidden,
      elevated,
    });
    closeCustomStartupModal();
    toast(delaySeconds > 0
      ? `Created delayed startup "${name}" (${delaySeconds}s after sign-in)`
      : `Created startup entry "${name}"`);
    await doScan();
  } catch (err) {
    toast('Failed to create startup entry: ' + err);
  } finally {
    if (btn) btn.disabled = false;
  }
}

async function checkForAppUpdates() {
  const btn = $('check-updates-btn');
  const label = $('check-updates-label');
  const box = $('about-update-status');
  if (!box) return;

  if (btn) btn.disabled = true;
  if (label) label.textContent = 'Checking…';
  try {
    const res = await window.go.main.App.CheckForUpdates();
    box.style.display = 'flex';
    box.classList.remove('has-update');
    if (res.error) {
      box.innerHTML = `<span>Update check failed: ${esc(res.error)}</span>`;
    } else if (res.updateAvailable) {
      box.classList.add('has-update');
      box.innerHTML = `
        <span><strong>Update available: v${esc(res.latestVersion)}</strong> (current: v${esc(res.currentVersion)})</span>
        <a href="${esc(res.releaseUrl)}" class="win-btn primary" style="padding: 3px 9px; font-size: 11px; text-decoration: none" onclick="return openExternalUrl(event, this.href)">Download Release</a>
      `;
    } else {
      box.innerHTML = `<span>You are on the latest version (<strong>v${esc(res.currentVersion)}</strong>).</span>`;
    }
  } catch (err) {
    box.style.display = 'flex';
    box.innerHTML = `<span>Update check error: ${esc(String(err))}</span>`;
  } finally {
    if (btn) btn.disabled = false;
    if (label) label.textContent = 'Check for updates';
  }
}

function openExternalUrl(e, url) {
  if (e) e.preventDefault();
  if (window.runtime && window.runtime.BrowserOpenURL) {
    window.runtime.BrowserOpenURL(url);
  } else {
    window.open(url, '_blank', 'noopener,noreferrer');
  }
  return false;
}

async function loadBuildInfo() {
  const el = $('about-version-meta');
  if (!el || !window.go?.main?.App?.GetAppBuildInfo) return;
  try {
    const info = await window.go.main.App.GetAppBuildInfo();
    const ver = info.version || '1.1.1';
    const commitPart = info.commit ? ` (${info.commit})` : '';
    el.textContent = `Version ${ver}${commitPart} · All system components protected`;
    if (info.commitMsg) {
      el.title = `Commit ${info.commit || ''}: ${info.commitMsg}`;
      let msgEl = $('about-commit-msg');
      if (!msgEl) {
        msgEl = document.createElement('div');
        msgEl.id = 'about-commit-msg';
        msgEl.style.cssText = 'font-size:11px;color:var(--text-dim);margin-top:3px';
        el.closest('.about-details')?.querySelector('.about-desc')?.after(msgEl);
      }
      msgEl.textContent = `Build: ${info.commit ? info.commit + ' — ' : ''}${info.commitMsg}`;
    }
  } catch {}
}

// Bootstrap
applyTheme(currentTheme);
applyAccent(currentAccent);
document.documentElement.style.zoom = uiScale;
navigate('overview');
loadBuildInfo();
doScan().then(() => {
  loadHistory();
  loadSnapshots();
  loadInstalledApps();
});
