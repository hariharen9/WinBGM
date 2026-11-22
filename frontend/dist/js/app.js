// --- Main Application Controller & Event Wiring ---

function toast(m) {
  const t = $('toast');
  t.textContent = m;
  t.classList.add('show');
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => t.classList.remove('show'), 3200);
}

function navigate(p) {
  page = p;
  document.querySelectorAll('.page').forEach(x => x.classList.toggle('active', x.id === 'page-' + p));
  document.querySelectorAll('.nav-item').forEach(b => b.classList.toggle('active', b.dataset.page === p));
  const labels = {
    overview: 'Overview',
    applications: 'Applications',
    services: 'Services',
    tasks: 'Scheduled tasks',
    sources: 'Startup locations',
    history: 'Change history',
    settings: 'Settings'
  };
  $('commandbar-label').textContent = labels[p] || 'WinBGM';
  $('content-host').scrollTop = 0;
  if (p === 'history') loadHistory();
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

$('svc-body').addEventListener('click', e => rowToggle(e, renderServices));
$('task-body').addEventListener('click', e => rowToggle(e, renderTasks));

$('modal-confirm').addEventListener('click', confirmChange);
$('modal-cancel').addEventListener('click', closeModal);
$('confirm-modal').addEventListener('click', e => { if (e.target === $('confirm-modal')) closeModal(); });
document.addEventListener('keydown', e => { if (e.key === 'Escape' && pending) closeModal(); });

$('set-system').addEventListener('change', e => {
  showSystem = e.target.checked;
  try { localStorage.setItem('bgm.showSystem', showSystem ? '1' : '0'); } catch (x) {}
  render();
});

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

// Bootstrap
document.documentElement.style.zoom = uiScale;
doScan().then(loadHistory);
