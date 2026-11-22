// --- Backend Bridge (Wails Go & Offline Demo Fallback) ---

const api = () => (window.go && window.go.main && window.go.main.App) ? window.go.main.App : null;

async function doScan() {
  const btns = document.querySelectorAll('.btn-scan, #scan-btn');
  btns.forEach(b => { b.disabled = true; b.classList.add('is-scanning'); });
  $('scan-status').textContent = 'Scanning…';
  try {
    const a = api();
    inv = a ? await a.Scan() : DEMO;
    if (!inv.apps) inv.apps = [];
    inv.apps.forEach(x => {
      x.components = x.components || [];
      x.components.forEach(c => { c.flags = c.flags || []; c.pids = c.pids || []; });
    });
    $('scan-status').textContent = 'Scanned ' + new Date(inv.scannedAt).toLocaleTimeString([], {hour:'numeric', minute:'2-digit'}) + (inv.durationMs ? ' · ' + (inv.durationMs / 1000).toFixed(1) + 's' : '');
    render();
  } catch (e) {
    $('scan-status').textContent = 'Scan failed';
    toast('Scan failed: ' + (e && e.message ? e.message : e));
  } finally {
    const btnsDone = document.querySelectorAll('.btn-scan, #scan-btn');
    btnsDone.forEach(b => { b.disabled = false; b.classList.remove('is-scanning'); });
  }
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
