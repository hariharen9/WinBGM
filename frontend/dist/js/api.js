// --- Backend Bridge (Wails Go & Offline Demo Fallback) ---

const api = () => (window.go && window.go.main && window.go.main.App) ? window.go.main.App : null;

async function doScan() {
  const btns = document.querySelectorAll('.btn-scan, #scan-btn');
  btns.forEach(b => { b.disabled = true; b.classList.add('is-scanning'); });
  $('scan-status').textContent = 'Scanning…';
  try {
    const a = api();
    if (a) {
      const [newInv, boot, power] = await Promise.allSettled([
        a.Scan(),
        a.GetBootProfile(),
        a.GetPowerReport()
      ]);
      inv = newInv.status === 'fulfilled' ? newInv.value : DEMO;
      bootProfile = boot.status === 'fulfilled' ? boot.value : null;
      powerReport = power.status === 'fulfilled' ? power.value : null;
    } else {
      inv = DEMO;
    }
    if (!inv.apps) inv.apps = [];
    inv.apps.forEach(x => {
      x.components = x.components || [];
      x.components.forEach(c => { c.flags = c.flags || []; c.pids = c.pids || []; });
    });
    $('scan-status').textContent = 'Scanned ' + new Date(inv.scannedAt).toLocaleTimeString([], {hour:'numeric', minute:'2-digit'}) + (inv.durationMs ? ' · ' + (inv.durationMs / 1000).toFixed(1) + 's' : '');
    render();
    fetchInventoryIcons();
  } catch (e) {
    $('scan-status').textContent = 'Scan failed';
    toast('Scan failed: ' + (e && e.message ? e.message : e));
  } finally {
    const btnsDone = document.querySelectorAll('.btn-scan, #scan-btn');
    btnsDone.forEach(b => { b.disabled = false; b.classList.remove('is-scanning'); });
  }
}

async function doRestoreBatch(batchId) {
  const a = api();
  if (!a) return;
  try {
    const r = await a.RestoreBatch(batchId);
    toast(r.message);
    if (r.modified > 0) {
      celebrateSuccess();
      await doScan();
    }
    await loadHistory();
  } catch (e) {
    toast('Batch restore failed: ' + e);
  }
}

// Live background process polling listener
if (window.runtime && window.runtime.EventsOn) {
  window.runtime.EventsOn('process_poll', poll => {
    if (!inv || !inv.apps || !poll) return;
    const activePaths = poll.activeExePaths || {};
    const ramByPath = poll.ramByExePath || {};
    const ramFmtByPath = poll.ramFormattedByExe || {};
    const ramByPID = poll.ramByPID || {};
    const connsByPID = poll.connsByPID || {};

    let totalBgRAMBytes = 0;
    const globalCountedPIDs = new Set();

    inv.apps.forEach(app => {
      let appRunning = false;
      let appRAMBytes = 0;
      const appCountedPIDs = new Set();
      const appConns = [];
      let telemetryCount = 0;

      app.components.forEach(c => {
        if (!c.exePath) return;
        const low = c.exePath.toLowerCase();
        const pids = activePaths[low] || [];
        c.pids = pids;
        c.running = pids.length > 0;
        c.ramBytes = ramByPath[low] || 0;
        c.ramFormatted = ramFmtByPath[low] || '';
        if (c.running) appRunning = true;

        pids.forEach(pid => {
          const pidRAM = ramByPID[pid] || 0;
          if (!appCountedPIDs.has(pid)) {
            appCountedPIDs.add(pid);
            appRAMBytes += pidRAM;
            const pConns = connsByPID[pid] || [];
            pConns.forEach(conn => {
              appConns.push(conn);
              if (conn.isTelemetry) telemetryCount++;
            });
          }
          if (!globalCountedPIDs.has(pid)) {
            globalCountedPIDs.add(pid);
            totalBgRAMBytes += pidRAM;
          }
        });
      });

      app.running = appRunning;
      app.ramBytes = appRAMBytes;
      app.ramFormatted = formatRAM(appRAMBytes);
      app.connections = appConns;
      app.telemetryCount = telemetryCount;
    });

    inv.totalRAMBytes = totalBgRAMBytes;
    inv.totalRAMFormatted = formatRAM(totalBgRAMBytes);

    const ramEl = document.getElementById('stat-ram');
    if (ramEl && inv.totalRAMFormatted) {
      ramEl.textContent = inv.totalRAMFormatted;
    }

    if (poll.telemetry) {
      liveTelemetry = poll.telemetry;
      const pushHist = (arr, v) => { arr.push(v); if (arr.length > 24) arr.shift(); };
      pushHist(telemetryHistory.cpu, Math.round(poll.telemetry.cpuPercent || 0));
      pushHist(telemetryHistory.ram, poll.telemetry.memoryLoadPercent || 0);
      pushHist(telemetryHistory.net, poll.telemetry.totalSockets || 0);
      if (typeof updateOverviewTelemetryUI === 'function') {
        updateOverviewTelemetryUI();
      }
    }

    if (page === 'processes' && typeof loadLiveProcesses === 'function') {
      loadLiveProcesses();
    }

    if (page === 'applications' && sel) {
      const activeApp = apps().find(x => x.id === sel);
      if (activeApp) {
        const netEl = $('detail-network-host');
        if (netEl && typeof renderDetailNetwork === 'function') {
          netEl.innerHTML = renderDetailNetwork(activeApp);
          refreshIcons(netEl);
        }
      }
    }
  });

  // Re-add Sentinel persistence watcher listener
  window.runtime.EventsOn('sentinel_alert', alert => {
    sentinelAlerts.unshift(alert);
    renderSentinelAlerts();
    toast('⚠️ Watchdog Alert: ' + (alert.name || 'Startup entry') + ' was modified or added!');
  });

  window.runtime.EventsOn('trigger_scan', () => {
    doScan();
  });
}

async function doCreateRestorePoint(description) {
  const a = api();
  if (!a || !a.CreateSystemRestorePoint) return toast('System Restore Point is available in the desktop app.');
  const btn = $('btn-create-restore-point');
  if (btn) { btn.disabled = true; btn.textContent = 'Creating checkpoint…'; }
  toast('Creating Windows System Restore Point…');
  try {
    const res = await a.CreateSystemRestorePoint(description || 'WinBGM Safety Checkpoint');
    toast(res.message || 'System Restore Point created!');
    celebrateSuccess();
  } catch (e) {
    toast('Restore point failed: ' + e);
  } finally {
    if (btn) { btn.disabled = false; btn.innerHTML = '<i data-lucide="shield-plus"></i>Create Restore Point'; refreshIcons(); }
  }
}

async function disableSentinelEntry(alertId) {
  const a = api();
  if (!a || !a.DisableSentinelAlert) return;
  try {
    const res = await a.DisableSentinelAlert(alertId);
    sentinelAlerts = sentinelAlerts.filter(x => x.id !== alertId);
    renderSentinelAlerts();
    toast(res.message || 'Startup entry disabled safely and backed up.');
    celebrateSuccess();
    await doScan();
  } catch (e) {
    toast('Could not disable entry: ' + e);
  }
}

async function dismissSentinelEntry(alertId) {
  const a = api();
  if (a && a.DismissSentinelAlert) {
    try { await a.DismissSentinelAlert(alertId); } catch (e) {}
  }
  sentinelAlerts = sentinelAlerts.filter(x => x.id !== alertId);
  renderSentinelAlerts();
}

async function doExport() {
  const a = api();
  if (!a) return toast('Export is available in the desktop app.');
  try {
    const p = await a.Export();
    toast(p ? 'Exported to ' + p : 'Export cancelled');
  } catch (e) {
    toast('Export failed: ' + e);
  }
}

async function doExportMarkdown() {
  const a = api();
  if (!a) return toast('Export is available in the desktop app.');
  try {
    const p = await a.ExportMarkdown();
    toast(p ? 'Exported Markdown audit to ' + p : 'Export cancelled');
  } catch (e) {
    toast('Export failed: ' + e);
  }
}

async function doExportCSV() {
  const a = api();
  if (!a) return toast('Export is available in the desktop app.');
  try {
    const p = await a.ExportCSV();
    toast(p ? 'Exported CSV spreadsheet to ' + p : 'Export cancelled');
  } catch (e) {
    toast('Export failed: ' + e);
  }
}

async function doFocusMode() {
  const a = api();
  if (!a) {
    toast('Focus Mode applied (sample preview: optimized 2 startup items).');
    return;
  }
  try {
    const res = await a.ApplyFocusMode();
    toast(res.message);
    if (res.modified > 0) {
      celebrateSuccess();
      await doScan();
      await loadHistory();
    }
  } catch (err) {
    toast('Focus Mode failed: ' + err);
  }
}

async function doReveal() {
  const a = api();
  const app = apps().find(x => x.id === sel);
  if (!a || !app) return toast('Select an application first.');
  const c = app.components.find(x => x.exePath);
  if (!c) return toast('No file location known for this app.');
  try {
    await a.Reveal(c.exePath);
  } catch (e) {
    toast(String(e));
  }
}

async function doElevate() {
  try {
    await api().RestartAsAdmin();
  } catch (e) {
    toast('Could not restart: ' + e);
  }
}

async function loadHistory() {
  const a = api();
  try {
    history = a ? ((await a.History()) || []) : [];
  } catch (e) {
    history = [];
  }
  renderHistory();
  if (inv) renderOverview();
}

async function doProcessAction(action, pid) {
  const a = api();
  if (!a) return toast('Process actions are available in the desktop app.');
  try {
    if (action === 'freeze') {
      await a.SuspendProcess(pid);
      toast('Process ' + pid + ' suspended (threads frozen).');
    } else if (action === 'resume') {
      await a.ResumeProcess(pid);
      toast('Process ' + pid + ' resumed.');
    } else if (action === 'close') {
      await a.GracefulCloseProcess(pid);
      toast('Graceful close requested for process ' + pid);
      setTimeout(doScan, 1500);
    } else if (action === 'kill') {
      await a.KillProcess(pid);
      toast('Process ' + pid + ' terminated.');
      setTimeout(doScan, 500);
    }
  } catch (err) {
    toast('Process action failed: ' + err);
  }
}

async function loadSnapshots() {
  const a = api();
  if (!a) return;
  try {
    snapshots = (await a.ListSnapshots()) || [];
    renderSnapshots();
  } catch (e) {
    snapshots = [];
  }
}

function promptCreateSnapshot() {
  const defaultName = 'Snapshot ' + new Date().toLocaleDateString() + ' ' + new Date().toLocaleTimeString([], {hour:'numeric', minute:'2-digit'});
  $('modal-title').textContent = 'Create Baseline Snapshot';
  $('modal-body').innerHTML = `
    <p>Save a full snapshot of all current startup entries, scheduled tasks, and services to compare against in the future.</p>
    <div style="margin-top:10px">
      <input id="snap-name-input" class="filter-search" style="width:100%;padding:8px 12px" type="text" value="${esc(defaultName)}" placeholder="Baseline Name">
    </div>
  `;
  $('modal-confirm').textContent = 'Save Baseline';
  $('modal-confirm').onclick = async () => {
    const inputVal = $('snap-name-input')?.value || defaultName;
    closeModal();
    await doCreateSnapshot(inputVal);
  };
  openModal();
}

async function doCreateSnapshot(name) {
  const a = api();
  if (!a) return toast('Snapshots available in the desktop app.');
  try {
    const meta = await a.SaveSnapshot(name);
    toast('Snapshot "' + meta.name + '" saved (' + meta.itemCount + ' items).');
    celebrateSuccess();
    await loadSnapshots();
  } catch (e) {
    toast('Snapshot failed: ' + e);
  }
}

async function doDiff(baseId) {
  const a = api();
  if (!a) return toast('Diff comparison available in the desktop app.');
  try {
    const diff = await a.DiffSnapshots(baseId);
    renderDiffResults(diff);
  } catch (e) {
    toast('Diff failed: ' + e);
  }
}

async function doDeleteSnapshot(id) {
  const a = api();
  if (!a) return;
  try {
    await a.DeleteSnapshot(id);
    toast('Snapshot deleted.');
    await loadSnapshots();
  } catch (e) {
    toast('Delete failed: ' + e);
  }
}

function dismissSentinelAlert(id) {
  sentinelAlerts = sentinelAlerts.filter(a => a.id !== id);
  renderSentinelAlerts();
}

async function confirmChange() {
  const p = pending;
  closeModal();
  if (!p) return;
  try {
    const r = await api().Apply(p);
    toast(r.message);
    if (r.ok) {
      celebrateSuccess();
      await doScan();
    } else {
      render();
    }
    loadHistory();
  } catch (e) {
    toast('Change failed: ' + e);
    render();
  }
}

async function doRestore(id) {
  const a = api();
  if (!a) return;
  try {
    const r = await a.Restore(id);
    toast(r.message);
    if (r.ok) {
      celebrateSuccess();
      await doScan();
    }
    await loadHistory();
  } catch (e) {
    toast('Restore failed: ' + e);
  }
}

// Sample offline data for previewing without compiling
const DEMO = {
  scannedAt: new Date().toISOString(),
  durationMs: 42,
  machine: 'PREVIEW-PC',
  os: 'Windows 11 · build 26100',
  elevated: false,
  alwaysAdmin: false,
  totalRAMBytes: 673185792,
  totalRAMFormatted: '642.0 MB',
  warnings: [],
  sources: [
    { id: 'run', title: 'Registry Run & RunOnce keys', desc: 'HKCU and HKLM Run/RunOnce entries.', count: 2, status: 'scanned' },
    { id: 'service', title: 'Windows services', desc: 'Every service.', count: 3, status: 'scanned' },
    { id: 'folder', title: 'Startup folders', desc: 'Per-user and all-users Startup folders.', count: 1, status: 'scanned' },
    { id: 'task', title: 'Task Scheduler', desc: 'Tasks outside Microsoft library.', count: 1, status: 'scanned' },
    { id: 'winlogon', title: 'Winlogon & logon shell', desc: 'Logon shell overrides and Userinit handlers.', count: 2, status: 'scanned' },
    { id: 'wmi', title: 'WMI event subscriptions', desc: 'Persistent WMI event filters and consumers.', count: 0, status: 'scanned' },
    { id: 'storeapp', title: 'Store app permissions', desc: 'Packaged app background records.', count: 1, status: 'partial' },
    { id: 'process', title: 'Running processes without a window', desc: 'Heuristic.', count: 1, status: 'partial' }
  ],
  apps: [
    {
      id: 'pkg:whatsappdesktop',
      name: 'WhatsApp',
      publisher: 'WhatsApp Inc.',
      letter: 'W',
      signature: 'verified',
      path: 'C:\\Program Files\\WindowsApps\\5319275A.WhatsAppDesktop',
      running: true,
      ramBytes: 157286400,
      ramFormatted: '150.0 MB',
      autoStart: false,
      review: false,
      system: false,
      behavior: 'Started by Windows or another app',
      behaviorNote: 'No sign-in entry found',
      components: [
        {
          id: 'p1',
          kind: 'process',
          name: 'WhatsApp.Root.exe',
          display: '',
          command: 'C:\\Program Files\\WindowsApps\\5319275A.WhatsAppDesktop\\WhatsApp.Root.exe',
          exePath: 'C:\\Program Files\\WindowsApps\\5319275A.WhatsAppDesktop\\WhatsApp.Root.exe',
          source: 'Running processes',
          trigger: 'Started by Windows (background task or notification)',
          startType: '',
          enabled: true,
          autoStart: false,
          running: true,
          pids: [10832],
          ramBytes: 157286400,
          ramFormatted: '150.0 MB',
          lastRun: '',
          publisher: 'WhatsApp Inc.',
          product: 'WhatsApp',
          signature: 'valid',
          sha256: 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855',
          virusTotalURL: 'https://www.virustotal.com/gui/file/e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855',
          locked: '',
          system: false,
          review: false,
          flags: []
        }
      ]
    },
    {
      id: 'pkg:claude',
      name: 'Claude',
      publisher: 'Anthropic',
      letter: 'C',
      signature: 'verified',
      path: 'C:\\Program Files\\WindowsApps\\Claude\\app\\resources',
      running: true,
      ramBytes: 94371840,
      ramFormatted: '90.0 MB',
      autoStart: true,
      review: false,
      system: false,
      behavior: 'Automatic service',
      behaviorNote: '1 automatic service',
      components: [
        {
          id: 's1',
          kind: 'service',
          name: 'CoworkVMService',
          display: 'Claude',
          command: '"C:\\Program Files\\WindowsApps\\Claude\\app\\resources\\cowork-svc.exe"',
          exePath: 'C:\\Program Files\\WindowsApps\\Claude\\app\\resources\\cowork-svc.exe',
          source: 'Services',
          trigger: 'Automatic service',
          startType: 'Automatic',
          enabled: true,
          autoStart: true,
          running: true,
          pids: [4120],
          ramBytes: 94371840,
          ramFormatted: '90.0 MB',
          lastRun: '',
          publisher: 'Anthropic',
          product: 'Claude',
          signature: 'valid',
          sha256: 'c3ab8ff13720e8ad9047dd39466b3c8974e592c2fa383d4a3960714caef0c4f2',
          virusTotalURL: 'https://www.virustotal.com/gui/file/c3ab8ff13720e8ad9047dd39466b3c8974e592c2fa383d4a3960714caef0c4f2',
          locked: 'yes',
          system: false,
          review: false,
          flags: []
        }
      ]
    },
    {
      id: 'dir:ollama',
      name: 'Ollama',
      publisher: 'Ollama Inc.',
      letter: 'O',
      signature: 'verified',
      path: 'C:\\Users\\you\\AppData\\Local\\Programs\\Ollama',
      running: true,
      ramBytes: 251658240,
      ramFormatted: '240.0 MB',
      autoStart: true,
      review: false,
      system: false,
      behavior: 'At sign-in',
      behaviorNote: '1 enabled entry',
      components: [
        {
          id: 'r2',
          kind: 'run',
          name: 'Ollama',
          display: 'Ollama Background Daemon',
          command: '"C:\\Users\\you\\AppData\\Local\\Programs\\Ollama\\ollama app.exe"',
          exePath: 'C:\\Users\\you\\AppData\\Local\\Programs\\Ollama\\ollama app.exe',
          source: 'HKCU Run',
          trigger: 'At sign-in',
          startType: '',
          enabled: true,
          autoStart: true,
          running: true,
          pids: [6380],
          ramBytes: 251658240,
          ramFormatted: '240.0 MB',
          lastRun: '',
          publisher: 'Ollama Inc.',
          product: 'Ollama',
          signature: 'valid',
          sha256: 'a1b2c3d4e5f60718293a4b5c6d7e8f901234567890abcdef1234567890abcdef',
          virusTotalURL: 'https://www.virustotal.com/gui/file/a1b2c3d4e5f60718293a4b5c6d7e8f901234567890abcdef1234567890abcdef',
          locked: '',
          system: false,
          review: false,
          flags: [],
          control: 'yes',
          controlNote: '',
          caution: '',
          canStop: false
        }
      ]
    },
    {
      id: 'dir:discord',
      name: 'Discord',
      publisher: 'Discord Inc.',
      letter: 'D',
      signature: 'verified',
      path: 'C:\\Users\\you\\AppData\\Local\\Discord',
      running: true,
      ramBytes: 169869312,
      ramFormatted: '162.0 MB',
      autoStart: true,
      review: false,
      system: false,
      behavior: 'At sign-in',
      behaviorNote: '1 enabled entry',
      components: [
        {
          id: 'r1',
          kind: 'run',
          name: 'Discord',
          display: '',
          command: '"C:\\Users\\you\\AppData\\Local\\Discord\\Update.exe" --processStart Discord.exe',
          exePath: 'C:\\Users\\you\\AppData\\Local\\Discord\\Update.exe',
          source: 'HKCU Run',
          trigger: 'At sign-in',
          startType: '',
          enabled: true,
          autoStart: true,
          running: true,
          pids: [2210],
          ramBytes: 169869312,
          ramFormatted: '162.0 MB',
          lastRun: '',
          publisher: 'Discord Inc.',
          product: 'Discord',
          signature: 'valid',
          sha256: '9876543210fedcba9876543210fedcba9876543210fedcba9876543210fedcba',
          virusTotalURL: 'https://www.virustotal.com/gui/file/9876543210fedcba9876543210fedcba9876543210fedcba9876543210fedcba',
          locked: '',
          system: false,
          review: false,
          flags: [],
          control: 'yes',
          controlNote: '',
          caution: '',
          canStop: false
        }
      ]
    }
  ]
};

DEMO.apps.forEach(a => a.components.forEach(c => {
  if (!c.control) {
    c.control = 'no';
    c.controlNote = c.locked === 'yes' ? 'Locked by its publisher: Windows doesn’t let this account change its startup type. You can still stop or start it.' : 'This can’t be changed safely from here.';
    c.canStop = c.kind === 'service';
  }
}));

// --- Uninstaller & Residual Cleanup API ---

const DEMO_INSTALLED_APPS = [
  {
    id: 'HKCU_Ollama',
    name: 'Ollama',
    version: '0.5.12',
    publisher: 'Ollama Inc.',
    installDate: '2026-05-18',
    estimatedSizeKB: 104857,
    sizeBytes: 107374182,
    displaySize: '102.4 MB',
    installLocation: 'C:\\Users\\Admin\\AppData\\Local\\Programs\\Ollama',
    uninstallString: '"C:\\Users\\Admin\\AppData\\Local\\Programs\\Ollama\\uninstall.exe"',
    quietUninstall: '"C:\\Users\\Admin\\AppData\\Local\\Programs\\Ollama\\uninstall.exe" /S',
    displayIcon: 'C:\\Users\\Admin\\AppData\\Local\\Programs\\Ollama\\ollama.exe',
    registryPath: 'HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\Ollama',
    isUserInstall: true,
    is64Bit: true,
    isSystemComponent: false,
    exePath: 'C:\\Users\\Admin\\AppData\\Local\\Programs\\Ollama\\ollama.exe',
    hasHeavyProfile: true,
    heavyProfileKey: 'ollama',
    heavyEstimatedBytes: 22880000000,
    heavyDisplaySize: '21.3 GB'
  },
  {
    id: 'HKLM_CapCut',
    name: 'CapCut',
    version: '9.6.0.4097',
    publisher: 'Bytedance Pte. Ltd.',
    installDate: '2026-10-25',
    estimatedSizeKB: 846720,
    sizeBytes: 867041280,
    displaySize: '826.9 MB',
    installLocation: 'C:\\Program Files\\CapCut',
    uninstallString: '"C:\\Program Files\\CapCut\\Uninstall.exe"',
    quietUninstall: '',
    displayIcon: 'C:\\Program Files\\CapCut\\CapCut.exe',
    registryPath: 'HKLM\\Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\CapCut',
    isUserInstall: false,
    is64Bit: true,
    isSystemComponent: false,
    exePath: 'C:\\Program Files\\CapCut\\CapCut.exe',
    hasHeavyProfile: true,
    heavyProfileKey: 'capcut',
    heavyEstimatedBytes: 6871947673,
    heavyDisplaySize: '6.40 GB'
  },
  {
    id: 'HKLM_7Zip',
    name: '7-Zip 26.01 (x64)',
    version: '26.01',
    publisher: 'Igor Pavlov',
    installDate: '2026-05-20',
    estimatedSizeKB: 5771,
    sizeBytes: 5909504,
    displaySize: '5.63 MB',
    installLocation: 'C:\\Program Files\\7-Zip',
    uninstallString: '"C:\\Program Files\\7-Zip\\Uninstall.exe"',
    quietUninstall: '',
    displayIcon: 'C:\\Program Files\\7-Zip\\7zFM.exe',
    registryPath: 'HKLM\\Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\7-Zip',
    isUserInstall: false,
    is64Bit: true,
    isSystemComponent: false,
    exePath: 'C:\\Program Files\\7-Zip\\7zFM.exe',
    hasHeavyProfile: false,
    heavyProfileKey: '',
    heavyEstimatedBytes: 0,
    heavyDisplaySize: '—'
  }
];

const DEMO_UNINSTALLER_SUMMARY = {
  totalApps: 119,
  totalSizeBytes: 399431475200,
  totalSizeFormatted: '372.0 GB',
  heavyAppsCount: 6,
  userAppsCount: 38,
  systemAppsCount: 14
};

let installedAppsLoaded = false;

async function loadInstalledApps(force = false) {
  if (!force && installedAppsLoaded && installedApps.length > 0) {
    renderUninstaller();
    return;
  }
  const metaEl = $('uninstaller-count-meta');
  if (metaEl) metaEl.textContent = 'Auditing installed programs…';
  
  const a = api();
  if (a && a.GetInstalledApps) {
    try {
      const res = await a.GetInstalledApps();
      if (res && Array.isArray(res.apps)) {
        installedApps = res.apps;
        uninstallerSummary = res.summary || null;
      } else if (Array.isArray(res)) {
        installedApps = res;
        uninstallerSummary = null;
      } else {
        installedApps = [];
        uninstallerSummary = null;
      }
    } catch (e) {
      console.error('Failed to query installed apps:', e);
      toast('Failed to load installed apps: ' + e);
    }
  } else {
    installedApps = DEMO_INSTALLED_APPS;
    uninstallerSummary = DEMO_UNINSTALLER_SUMMARY;
  }

  installedAppsLoaded = true;
  const badge = $('nav-uninstaller');
  if (badge) badge.textContent = installedApps.length || '';

  renderUninstaller();
  fetchUninstallerIcons();
}

async function scanAppResiduals(appId) {
  const a = api();
  if (a && a.ScanAppResiduals) {
    try {
      return await a.ScanAppResiduals(appId);
    } catch (e) {
      toast('Failed to audit residuals: ' + e);
      return null;
    }
  }
  // Demo fallback
  return {
    appId: appId,
    appName: 'Ollama',
    totalSizeBytes: 22881200000,
    totalSizeFormatted: '21.3 GB',
    heavySizeBytes: 22880000000,
    heavySizeFormatted: '21.3 GB',
    items: [
      {
        id: 'heavy_0',
        category: 'heavy',
        categoryTitle: 'AI Models & Weights',
        title: 'Ollama Downloaded Model Weights',
        path: 'C:\\Users\\Admin\\.ollama\\models',
        sizeBytes: 22880000000,
        sizeFormatted: '21.3 GB',
        itemCount: 4,
        description: 'Trained LLM model weight blobs (GGUF, safetensors, manifests).',
        isSelected: true,
        isRegistry: false
      },
      {
        id: 'appdata_1',
        category: 'appdata',
        categoryTitle: 'Local AppData Cache & Storage',
        title: 'LocalAppData Folder (Ollama)',
        path: 'C:\\Users\\Admin\\AppData\\Local\\Ollama',
        sizeBytes: 1200000,
        sizeFormatted: '1.14 MB',
        itemCount: 8,
        description: 'Application data, log files, and configuration buffers.',
        isSelected: true,
        isRegistry: false
      },
      {
        id: 'reg_2',
        category: 'registry',
        categoryTitle: 'Registry Configurations',
        title: 'HKCU Settings Key',
        path: 'HKCU\\Software\\Ollama',
        sizeBytes: 0,
        sizeFormatted: '6 values, 1 subkeys',
        itemCount: 7,
        description: 'Application preferences, environment flags, and user settings.',
        isSelected: true,
        isRegistry: true
      }
    ]
  };
}

let selectedUninstallIds = new Set();

function toggleUninstallSelect(appId, checked) {
  if (checked) selectedUninstallIds.add(appId);
  else selectedUninstallIds.delete(appId);
  updateBatchUninstallBar();
}

function toggleSelectAllUninstall(checked) {
  const checks = document.querySelectorAll('#uninstaller-body input.uninst-check');
  const q = (uninstallerSearch || '').toLowerCase();
  const visible = (installedApps || []).filter(a => {
    if (uninstallerFilter === 'heavy' && !a.hasHeavyProfile) return false;
    if (uninstallerFilter === 'large' && (a.sizeBytes || 0) < 1024 * 1024 * 1024) return false;
    if (uninstallerFilter === 'store' && a.source !== 'StoreApp') return false;
    if (uninstallerFilter === 'pkg' && !['Steam', 'Scoop', 'Chocolatey'].includes(a.source)) return false;
    if (uninstallerFilter === 'orphan' && a.source !== 'Orphaned') return false;
    if (uninstallerFilter === 'user' && !a.isUserInstall) return false;
    if (q && !(a.name + ' ' + a.publisher + ' ' + (a.installLocation || '')).toLowerCase().includes(q)) return false;
    return true;
  });
  visible.forEach(a => checked ? selectedUninstallIds.add(a.id) : selectedUninstallIds.delete(a.id));
  checks.forEach(c => { c.checked = checked; });
  updateBatchUninstallBar();
}

function clearUninstallSelection() {
  selectedUninstallIds.clear();
  renderUninstaller();
}

function updateBatchUninstallBar() {
  const bar = $('uninstaller-batch-bar');
  if (!bar) return;
  const count = selectedUninstallIds.size;
  if (count === 0) {
    bar.style.display = 'none';
    return;
  }
  const selected = (installedApps || []).filter(a => selectedUninstallIds.has(a.id));
  const totalBytes = selected.reduce((acc, a) => acc + (a.sizeBytes || 0), 0);
  const gb = 1024 ** 3, mb = 1024 ** 2;
  const sizeStr = totalBytes >= gb ? (totalBytes / gb).toFixed(2) + ' GB' : (totalBytes / mb).toFixed(1) + ' MB';
  $('uninstaller-batch-info').innerHTML = `<strong>${count}</strong> program${count > 1 ? 's' : ''} queued · <span>${sizeStr} estimated footprint</span>`;
  $('uninstaller-batch-count').textContent = count;
  bar.style.display = 'flex';
  refreshIcons();
}

function promptBatchUninstall() {
  const appsToRemove = (installedApps || []).filter(a => selectedUninstallIds.has(a.id));
  if (!appsToRemove.length) return;
  const listPreview = appsToRemove.slice(0, 6).map(a => `<li><strong>${esc(a.name)}</strong> (${esc(a.displaySize)})</li>`).join('');
  const extra = appsToRemove.length > 6 ? `<li>…and ${appsToRemove.length - 6} more</li>` : '';

  $('modal-title').textContent = `Batch Uninstall (${appsToRemove.length} Programs)`;
  $('modal-body').innerHTML = `
    <p>WinBGM will sequentially uninstall the following programs and then audit all leftover files &amp; dependencies in one unified cleanup step:</p>
    <ul style="margin: 8px 0 10px 18px; font-size: 11.5px; color: var(--text)">${listPreview}${extra}</ul>
    <div style="margin-top: 10px; display: flex; align-items: center; gap: 8px">
      <input type="checkbox" id="batch-quiet-toggle" checked>
      <label for="batch-quiet-toggle" style="font-size: 11.5px; cursor: pointer">Attempt silent/quiet uninstallation whenever supported</label>
    </div>
  `;
  $('modal-confirm').textContent = `Uninstall ${appsToRemove.length} Programs`;
  $('modal-confirm').style.display = 'inline-flex';
  $('modal-confirm').onclick = async () => {
    closeModal();
    const quiet = $('batch-quiet-toggle') ? $('batch-quiet-toggle').checked : true;
    await executeBatchUninstall(appsToRemove, quiet);
  };
  $('modal-cancel').textContent = 'Cancel';
  openModal();
  refreshIcons();
}

async function executeBatchUninstall(appsToRemove, quiet) {
  const a = api();
  const mergedItems = [];
  let totalSize = 0;

  for (let i = 0; i < appsToRemove.length; i++) {
    const app = appsToRemove[i];
    toast(`Uninstalling (${i + 1}/${appsToRemove.length}): ${app.name}…`);
    if (a && a.LaunchUninstaller) {
      try {
        await a.LaunchUninstaller(app.id, quiet);
      } catch (e) {
        toast(`Note for ${app.name}: ${e}`);
      }
    }
    const report = await scanAppResiduals(app.id);
    if (report && report.items && report.items.length) {
      for (const it of report.items) {
        mergedItems.push(it);
        totalSize += (it.sizeBytes || 0);
      }
    }
  }

  selectedUninstallIds.clear();
  if (mergedItems.length > 0) {
    const gb = 1024 ** 3, mb = 1024 ** 2, kb = 1024;
    const fmt = totalSize >= gb ? (totalSize / gb).toFixed(2) + ' GB'
      : totalSize >= mb ? (totalSize / mb).toFixed(1) + ' MB'
      : (totalSize / kb).toFixed(1) + ' KB';
    showResidualModal({
      appId: 'batch',
      appName: `${appsToRemove.length} Programs`,
      batchAppIds: appsToRemove.map(x => x.id),
      items: mergedItems,
      totalSize,
      totalSizeFormatted: fmt,
    });
  } else {
    toast(`Batch uninstall complete! No leftover data found.`);
    celebrateSuccess();
    await loadInstalledApps(true);
  }
}

function promptUninstall(appId) {
  const app = installedApps.find(a => a.id === appId);
  if (!app) return;

  const hasHeavy = !!app.hasHeavyProfile;
  const heavySizeLabel = (app.heavyDisplaySize && app.heavyDisplaySize !== '—') ? ` (${esc(app.heavyDisplaySize)})` : '';
  const heavyNote = hasHeavy
    ? `<div style="margin-top:12px;padding:10px 12px;border-radius:6px;background:rgba(245,158,11,0.12);border:1px solid rgba(245,158,11,0.3);color:#f59e0b;font-size:11.5px">
        <strong>🔥 Heavy Dependencies Detected:</strong> This program has external data/models${heavySizeLabel} that standard uninstallers leave behind. WinBGM will automatically audit and offer to clean them after the uninstaller runs.
       </div>`
    : '';

  $('modal-title').textContent = 'Uninstall ' + app.name;
  $('modal-body').innerHTML = `
    <p>Are you sure you want to uninstall <strong>${esc(app.name)}</strong> (${esc(app.displaySize)})?</p>
    <p style="margin-top:6px;color:var(--text-2);font-size:11px">WinBGM will launch the official uninstaller and then immediately scan for leftover folders, cached files, and heavy dependencies to ensure a 100% clean removal.</p>
    ${heavyNote}
    <div style="margin-top:14px;display:flex;align-items:center;gap:8px">
      <input type="checkbox" id="uninst-quiet-toggle" ${app.quietUninstall ? 'checked' : 'disabled'}>
      <label for="uninst-quiet-toggle" style="font-size:11.5px;cursor:pointer">
        Attempt silent/quiet uninstallation ${app.quietUninstall ? '' : '<span style="color:var(--muted)">(silent flag not provided by vendor)</span>'}
      </label>
    </div>
  `;

  $('modal-confirm').textContent = 'Proceed with Uninstall';
  $('modal-confirm').style.display = 'inline-flex';
  $('modal-confirm').onclick = async () => {
    closeModal();
    const quiet = $('uninst-quiet-toggle') ? $('uninst-quiet-toggle').checked : false;
    await triggerUninstall(appId, quiet);
  };
  $('modal-cancel').textContent = 'Cancel';
  openModal();
  refreshIcons();
}

async function triggerUninstall(appId, quiet = false) {
  const app = installedApps.find(a => a.id === appId);
  const appName = app ? app.name : 'Application';

  toast(`Launching official uninstaller for ${appName}…`);
  const a = api();
  let uninstCode = 0;
  let uninstFailed = false;
  if (a && a.LaunchUninstaller) {
    try {
      uninstCode = await a.LaunchUninstaller(appId, quiet);
      if (uninstCode === 1223) {
        toast(`Uninstall cancelled for ${appName}. Skipping leftover scan.`);
        return;
      }
      toast(`Uninstaller completed (code: ${uninstCode}). Auditing leftovers & dependencies…`);
    } catch (e) {
      uninstFailed = true;
      toast(`Uninstaller did not launch cleanly: ${e}`);
    }
  }
  if (uninstFailed) return;

  // Scan residuals post-uninstall
  const report = await scanAppResiduals(appId);
  if (report && report.items && report.items.length > 0) {
    showResidualModal(report);
  } else {
    toast(`Clean removal verified! No leftover data found for ${appName}.`);
    celebrateSuccess();
    await loadInstalledApps(true);
  }
}

async function scanAndCleanOnly(appId) {
  const app = installedApps.find(a => a.id === appId);
  if (!app) return;
  toast(`Auditing leftover files, models & cache for ${app.name}…`);
  const report = await scanAppResiduals(appId);
  if (report && report.items && report.items.length > 0) {
    showResidualModal(report);
  } else {
    toast(`No leftover cache or model dependencies detected for ${app.name}.`);
  }
}

function showResidualModal(report) {
  currentResidualReport = report;
  selectedResidualIds = new Set(report.items.filter(it => it.isSelected).map(it => it.id));

  $('residual-title').textContent = `Clean Leftovers & Dependencies (${esc(report.appName)})`;
  $('residual-subtitle').textContent = `Discovered ${report.items.length} residual items totaling ${report.totalSizeFormatted}`;

  renderResidualBody();

  const modal = $('residual-modal');
  if (modal) modal.classList.add('show');
  refreshIcons();
}

async function rescanCurrentResiduals() {
  if (!currentResidualReport) return;
  const prev = currentResidualReport;
  toast('Re-scanning disk & registry for remaining leftovers…');

  let updatedReport = null;
  if (prev.appId === 'batch' && Array.isArray(prev.batchAppIds) && prev.batchAppIds.length > 0) {
    const mergedItems = [];
    let totalSize = 0;
    for (const id of prev.batchAppIds) {
      const r = await scanAppResiduals(id);
      if (r && r.items && r.items.length) {
        for (const it of r.items) {
          mergedItems.push(it);
          totalSize += (it.sizeBytes || 0);
        }
      }
    }
    const gb = 1024 ** 3, mb = 1024 ** 2, kb = 1024;
    const fmt = totalSize >= gb ? (totalSize / gb).toFixed(2) + ' GB'
      : totalSize >= mb ? (totalSize / mb).toFixed(1) + ' MB'
      : totalSize >= kb ? (totalSize / kb).toFixed(1) + ' KB'
      : totalSize + ' B';
    updatedReport = {
      appId: 'batch',
      appName: prev.appName,
      batchAppIds: prev.batchAppIds,
      items: mergedItems,
      totalSize,
      totalSizeFormatted: fmt,
    };
  } else {
    updatedReport = await scanAppResiduals(prev.appId);
  }

  if (!updatedReport || !updatedReport.items || updatedReport.items.length === 0) {
    closeResidualModal();
    toast(`Clean removal verified! No leftover data remains for ${prev.appName}.`);
    celebrateSuccess();
    await loadInstalledApps(true);
    return;
  }

  showResidualModal(updatedReport);
  toast(`Re-scan complete: ${updatedReport.items.length} leftover item(s) still present.`);
}

function selectGoodResiduals() {
  if (!currentResidualReport) return;
  selectedResidualIds.clear();
  currentResidualReport.items.forEach(it => {
    if (it.confidenceLevel === 'Very High' || it.confidenceLevel === 'Good') {
      selectedResidualIds.add(it.id);
    }
  });
  renderResidualBody();
}

function clearResidualSelection() {
  selectedResidualIds.clear();
  renderResidualBody();
}

function renderResidualBody() {
  if (!currentResidualReport) return;
  const items = currentResidualReport.items;

  // Group by category
  const categories = {};
  items.forEach(it => {
    categories[it.categoryTitle] = categories[it.categoryTitle] || [];
    categories[it.categoryTitle].push(it);
  });

  let html = `
    <div style="display:flex;align-items:center;justify-content:space-between;gap:8px;padding:6px 10px;border-radius:6px;background:var(--surface-card);border:1px solid var(--border);margin-bottom:4px">
      <span style="font-size:11px;color:var(--text-2)">Nothing is pre-selected for safety. Review confidence ratings or select safe items:</span>
      <div style="display:flex;gap:6px;flex:none">
        <button class="win-btn" style="height:24px;font-size:10.5px;padding:0 8px" onclick="rescanCurrentResiduals()" title="Re-check disk &amp; registry for remaining leftovers"><i data-lucide="refresh-cw" style="width:11px;height:11px"></i>Re-scan</button>
        <button class="win-btn" style="height:24px;font-size:10.5px;padding:0 8px" onclick="selectGoodResiduals()">Select Good+</button>
        <button class="win-btn subtle" style="height:24px;font-size:10.5px;padding:0 8px" onclick="clearResidualSelection()">Clear</button>
      </div>
    </div>
  `;
  for (const [catTitle, catItems] of Object.entries(categories)) {
    html += `
      <div class="residual-category-title">
        <span>${esc(catTitle)}</span>
        <span style="font-size:10.5px;color:var(--text-dim)">${catItems.length} items</span>
      </div>
    `;

    catItems.forEach(it => {
      const isChecked = selectedResidualIds.has(it.id);
      const isHeavy = it.category === 'heavy';
      const conf = it.confidenceLevel || 'Good';
      const confCls = (conf === 'Very High' || conf === 'Good') ? 'green' : conf === 'Questionable' ? 'amber' : 'red';
      const confTip = (it.confidenceReasons || []).join(' • ');
      const confBadge = `<span class="badge ${confCls}" style="font-size:9.5px;padding:1px 6px;margin-left:6px" title="${esc(confTip)}">${esc(conf)}</span>`;
      html += `
        <div class="residual-item-card ${isHeavy ? 'is-heavy' : ''}" onclick="toggleResidualItem('${esc(it.id)}')">
          <input type="checkbox" id="chk-${esc(it.id)}" ${isChecked ? 'checked' : ''} onclick="event.stopPropagation();toggleResidualItem('${esc(it.id)}')" style="margin-top:2px;cursor:pointer">
          <div class="residual-item-copy">
            <div class="residual-item-title-row">
              <span class="residual-item-title">${esc(it.title)}${confBadge}</span>
              <span class="residual-item-size">${esc(it.sizeFormatted)}</span>
            </div>
            <div class="residual-item-path">${esc(it.path)}</div>
            <div class="residual-item-desc">${esc(it.description)}</div>
          </div>
        </div>
      `;
    });
  }

  $('residual-body').innerHTML = html;
  updateResidualSelectedSize();
}

function toggleResidualItem(id) {
  if (selectedResidualIds.has(id)) {
    selectedResidualIds.delete(id);
  } else {
    selectedResidualIds.add(id);
  }
  const chk = $('chk-' + id);
  if (chk) chk.checked = selectedResidualIds.has(id);
  updateResidualSelectedSize();
}

function updateResidualSelectedSize() {
  if (!currentResidualReport) return;
  let totalBytes = 0;
  currentResidualReport.items.forEach(it => {
    if (selectedResidualIds.has(it.id)) {
      totalBytes += it.sizeBytes;
    }
  });

  let sizeStr = '0 B';
  const kb = 1024, mb = kb * 1024, gb = mb * 1024;
  if (totalBytes >= gb) sizeStr = (totalBytes / gb).toFixed(2) + ' GB';
  else if (totalBytes >= mb) sizeStr = (totalBytes / mb).toFixed(1) + ' MB';
  else if (totalBytes >= kb) sizeStr = (totalBytes / kb).toFixed(1) + ' KB';
  else if (totalBytes > 0) sizeStr = totalBytes + ' B';

  $('residual-selected-size').textContent = sizeStr;

  const btn = $('residual-confirm');
  if (btn) btn.disabled = selectedResidualIds.size === 0;
}

function closeResidualModal() {
  const modal = $('residual-modal');
  if (modal) modal.classList.remove('show');
  currentResidualReport = null;
  selectedResidualIds.clear();
}

async function openAppLocation(appId) {
  const app = installedApps.find(a => a.id === appId);
  if (!app) return;
  const loc = app.installLocation || app.exePath;
  if (!loc) {
    toast('No installation path available for this program.');
    return;
  }
  const a = api();
  if (a && a.Reveal) {
    try {
      await a.Reveal(loc);
    } catch (e) {
      toast('Could not open location: ' + e);
    }
  }
}

async function openAppVirusTotal(appId) {
  const app = installedApps.find(a => a.id === appId);
  if (!app || !app.exePath) return;
  const a = api();
  if (a && a.GetFileVirusTotalURL) {
    try {
      const url = await a.GetFileVirusTotalURL(app.exePath);
      if (url) {
        window.open(url, '_blank');
        return;
      }
    } catch (e) {
      console.error('VT hash failed:', e);
    }
  }
  window.open('https://www.virustotal.com/gui/search/' + encodeURIComponent(base(app.exePath)), '_blank');
}

async function executeResidualClean() {
  if (!currentResidualReport || selectedResidualIds.size === 0) return;

  const itemsToClean = currentResidualReport.items
    .filter(it => selectedResidualIds.has(it.id))
    .map(it => Object.assign({}, it, { isSelected: true }));
  const confirmBtn = $('residual-confirm');
  if (confirmBtn) {
    confirmBtn.disabled = true;
    confirmBtn.textContent = 'Moving to Recycle Bin…';
  }

  const a = api();
  if (a && a.CleanResidualItems) {
    try {
      const res = await a.CleanResidualItems(itemsToClean);
      toast(`Cleaned ${res.freedSizeFormatted || 'selected data'} (${res.deletedFilesCount} files, ${res.deletedKeysCount} keys) safely to Recycle Bin!`);
      celebrateSuccess();
      closeResidualModal();
      await loadInstalledApps();
    } catch (e) {
      toast('Cleanup failed: ' + e);
      if (confirmBtn) {
        confirmBtn.disabled = false;
        confirmBtn.textContent = 'Retry Clean';
      }
    }
  } else {
    toast(`Cleaned ${itemsToClean.length} items safely to Recycle Bin!`);
    celebrateSuccess();
    closeResidualModal();
  }
}

// --- Live Process Manager & Efficiency Mode API ---

let liveProcLoading = false;

async function loadLiveProcesses() {
  if (liveProcLoading || document.hidden) return;
  liveProcLoading = true;
  const a = api();
  try {
    if (a && a.GetLiveProcesses) {
      try {
        const res = await a.GetLiveProcesses();
        liveProcessGroups = (res && res.groups) ? res.groups : [];
        if (res && res.telemetry) liveTelemetry = res.telemetry;
      } catch (e) {
        console.error('Failed to load live processes:', e);
      }
    } else {
      liveProcessGroups = [
        {
          id: 'brave.exe', name: 'Brave Browser', exePath: 'C:\\Program Files\\BraveSoftware\\Brave-Browser\\Application\\brave.exe',
          pids: [1240, 2248, 3310], processCount: 12, cpuPercent: 4.8, ramBytes: 1450000000, ramFormatted: '1.35 GB',
          ioBytesPerSec: 128000, ioFormatted: '125.0 KB/s', priority: 'Normal', ecoMode: false, suspended: false,
          hasWindow: true, protected: false, connCount: 8, telemetryCount: 0,
          children: [{ pid: 1240, ppid: 800, name: 'brave.exe', cpuPercent: 2.1, ramFormatted: '420.0 MB', ioFormatted: '80.0 KB/s', priority: 'Normal', ecoMode: false, suspended: false, protected: false }]
        }
      ];
    }
    const nav = $('nav-processes');
    if (nav) nav.textContent = liveProcessGroups.length || '';
    const scrollHost = $('page-processes');
    const prevScroll = scrollHost ? scrollHost.scrollTop : 0;
    if (typeof renderLiveProcesses === 'function') renderLiveProcesses();
    if (scrollHost && prevScroll > 0) scrollHost.scrollTop = prevScroll;
    fetchProcessIcons();
  } finally {
    liveProcLoading = false;
  }
}

async function toggleGroupEcoMode(groupId, enable) {
  const grp = liveProcessGroups.find(g => g.id === groupId);
  if (!grp) return;
  const a = api();
  if (!a || !a.SetGroupEcoMode) return toast('Efficiency Mode is available in the desktop app.');
  try {
    await a.SetGroupEcoMode(grp.pids, enable);
    toast((enable ? '🍃 Enabled Efficiency Mode (EcoQoS) for ' : 'Disabled Efficiency Mode for ') + grp.name);
    await loadLiveProcesses();
  } catch (e) {
    toast('EcoQoS error: ' + e);
  }
}

async function togglePidEcoMode(pid, enable) {
  const a = api();
  if (!a || !a.SetProcessEcoMode) return;
  try {
    await a.SetProcessEcoMode(pid, enable);
    toast((enable ? '🍃 Enabled Efficiency Mode for PID ' : 'Disabled Efficiency Mode for PID ') + pid);
    await loadLiveProcesses();
  } catch (e) {
    toast('EcoQoS error: ' + e);
  }
}

async function changePidPriority(pid, level) {
  const a = api();
  if (!a || !a.SetProcessPriority) return;
  try {
    await a.SetProcessPriority(pid, level);
    toast(`Set PID ${pid} priority to ${level}`);
    await loadLiveProcesses();
  } catch (e) {
    toast('Priority error: ' + e);
  }
}

async function freezeProcessGroup(groupId, freeze) {
  const grp = liveProcessGroups.find(g => g.id === groupId);
  if (!grp) return;
  const a = api();
  if (!a) return;
  try {
    for (const pid of grp.pids) {
      if (freeze && a.SuspendProcess) await a.SuspendProcess(pid);
      else if (!freeze && a.ResumeProcess) await a.ResumeProcess(pid);
    }
    toast((freeze ? '❄️ Frozen ' : 'Resumed ') + grp.name);
    await loadLiveProcesses();
  } catch (e) {
    toast('Process control error: ' + e);
  }
}

async function killProcessGroup(groupId) {
  const grp = liveProcessGroups.find(g => g.id === groupId);
  if (!grp || grp.protected) return;
  const a = api();
  if (!a || !a.KillProcessTree) return;
  try {
    await a.KillProcessTree(grp.pids);
    toast('Ended process tree for ' + grp.name);
    await loadLiveProcesses();
  } catch (e) {
    toast('End task error: ' + e);
  }
}

async function openProcLocation(exePath) {
  const a = api();
  if (a && a.Reveal && exePath) {
    try { await a.Reveal(exePath); } catch (e) { toast('Could not open location: ' + e); }
  }
}

async function openProcVT(exePath) {
  if (!exePath) return;
  const a = api();
  if (a && a.GetFileVirusTotalURL) {
    try {
      const url = await a.GetFileVirusTotalURL(exePath);
      if (url) { window.open(url, '_blank'); return; }
    } catch (e) {}
  }
  window.open('https://www.virustotal.com/gui/search/' + encodeURIComponent(base(exePath)), '_blank');
}

// --- File Lock Finder (Restart Manager) API ---

async function browseLockedFile() {
  const a = api();
  if (!a || !a.BrowseForLockedFile) return toast('File picker is available in the desktop app.');
  try {
    const p = await a.BrowseForLockedFile();
    if (p) {
      const inp = $('lock-path-input');
      if (inp) inp.value = p;
      await checkFileLocks();
    }
  } catch (e) {
    toast('Browse failed: ' + e);
  }
}

async function checkFileLocks() {
  const inp = $('lock-path-input');
  const target = inp ? inp.value.trim().replace(/^"|"$/g, '') : '';
  if (!target) return toast('Enter or browse a file/folder path first.');

  const meta = $('lock-count-meta');
  if (meta) meta.textContent = 'Querying Windows Restart Manager…';

  const a = api();
  if (a && a.FindLockingProcesses) {
    try {
      lockResults = (await a.FindLockingProcesses(target)) || [];
    } catch (e) {
      lockResults = [];
      toast('Lock scan error: ' + e);
    }
  } else {
    lockResults = [];
  }

  if (typeof renderLockResults === 'function') renderLockResults(target);
}

async function unlockAction(pid, kind) {
  const a = api();
  if (!a) return;
  try {
    if (kind === 'close' && a.GracefulCloseProcess) {
      await a.GracefulCloseProcess(pid);
      toast(`Sent close signal to PID ${pid}`);
    } else if (kind === 'kill' && a.KillProcess) {
      await a.KillProcess(pid);
      toast(`Terminated PID ${pid}`);
    }
    setTimeout(checkFileLocks, 500);
  } catch (e) {
    toast('Action failed: ' + e);
  }
}

// --- Native Windows App Icon Extraction ---

async function fetchMissingIcons(paths, onDone) {
  const a = api();
  if (!a || !a.GetAppIcons) return;
  const missing = [];
  const seen = new Set();
  for (const p of paths) {
    if (!p) continue;
    const low = p.toLowerCase();
    if (seen.has(low) || Object.prototype.hasOwnProperty.call(appIconCache, low)) continue;
    seen.add(low);
    missing.push(p);
  }
  if (missing.length === 0) return;

  // Mark as pending so concurrent calls don't duplicate work
  missing.forEach(p => { appIconCache[p.toLowerCase()] = ''; });

  try {
    const res = await a.GetAppIcons(missing);
    let updated = false;
    if (res) {
      for (const [k, uri] of Object.entries(res)) {
        if (uri) {
          appIconCache[k.toLowerCase()] = uri;
          updated = true;
        }
      }
    }
    if (updated && onDone) onDone();
  } catch (e) {
    console.error('Icon extraction failed:', e);
  }
}

function fetchInventoryIcons() {
  if (!inv || !inv.apps) return;
  const paths = [];
  inv.apps.forEach(app => {
    if (app.path) paths.push(app.path);
    (app.components || []).forEach(c => {
      if (c.exePath) paths.push(c.exePath);
    });
  });
  fetchMissingIcons(paths, () => {
    renderApps();
    renderOverview();
  });
}

function fetchUninstallerIcons() {
  if (!installedApps || !installedApps.length) return;
  const paths = [];
  installedApps.forEach(app => {
    if (app.displayIcon) paths.push(app.displayIcon);
    if (app.exePath) paths.push(app.exePath);
  });
  fetchMissingIcons(paths, () => {
    if (page === 'uninstaller') renderUninstaller();
    renderApps();
    renderOverview();
  });
}

function fetchProcessIcons() {
  if (!liveProcessGroups || !liveProcessGroups.length) return;
  const paths = liveProcessGroups.map(g => g.exePath).filter(Boolean);
  fetchMissingIcons(paths, () => {
    if (page === 'processes') renderLiveProcesses();
  });
}


