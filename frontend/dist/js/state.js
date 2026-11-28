// --- Application State & Data Mappings ---

const $ = id => document.getElementById(id);
const esc = s => String(s == null ? '' : s).replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));

const KIND = {
  service: 'Windows service',
  run: 'Registry Run entry',
  folder: 'Startup folder item',
  task: 'Scheduled task',
  process: 'Background process',
  storeapp: 'Store app permission',
  winlogon: 'Winlogon configuration',
  wmi: 'WMI event consumer',
  shellex: 'Shell extension',
  nativemessaging: 'Browser messaging host',
  driver: 'Kernel / hardware driver',
  activesetup: 'Active Setup hook'
};

const KIND_ICON = {
  service: 'cog',
  run: 'zap',
  folder: 'folder-open',
  task: 'calendar-clock',
  process: 'activity',
  storeapp: 'package',
  winlogon: 'shield-alert',
  wmi: 'terminal',
  shellex: 'mouse-pointer-click',
  nativemessaging: 'globe',
  driver: 'hard-drive',
  activesetup: 'terminal'
};

const SOURCE_ICONS = {
  run: 'zap',
  folder: 'folder-open',
  service: 'cog',
  task: 'calendar-clock',
  storeapp: 'package',
  process: 'activity',
  winlogon: 'shield-alert',
  wmi: 'terminal',
  shellex: 'mouse-pointer-click',
  nativemessaging: 'globe',
  driver: 'hard-drive',
  activesetup: 'terminal'
};

const HOWTO = {
  service: 'Open Services (services.msc), find this service and change its startup type or stop it. Some packaged services are locked by their publisher.',
  run: 'Task Manager → Startup apps, or the app’s own “launch at sign-in” setting.',
  folder: 'Task Manager → Startup apps, or remove the shortcut from the Startup folder.',
  task: 'Open Task Scheduler (taskschd.msc) and disable the task. Updates may recreate it.',
  process: 'Close it from Task Manager, or look for a “run in background” / “close to tray” option in the app. Windows may also start it for notifications.',
  storeapp: 'Settings → Apps → Installed apps → the app → Advanced options → Background apps permissions.',
  winlogon: 'Winlogon entries are system hooks executed during Windows logon. Investigate the key in Registry Editor under Microsoft\\Windows NT\\CurrentVersion\\Winlogon.',
  wmi: 'WMI persistence can be queried and managed using PowerShell (Get-CimInstance -Namespace root/subscription -ClassName __EventConsumer).',
  shellex: 'Right-click context menu handlers load directly into File Explorer. Disable via WinBGM to block the CLSID without deleting registry keys.',
  nativemessaging: 'Native messaging hosts allow browser extensions to launch local programs. Inspect under Software\\Google\\Chrome\\NativeMessagingHosts or equivalent.',
  driver: 'Kernel filter or peripheral driver. Managed via Windows Device Manager and DriverQuery.',
  activesetup: 'Per-user setup scripts executed once when creating user desktop sessions.'
};

const KNOWN = [
  { m: /whatsapp/i, t: 'Windows can start WhatsApp in the background to receive push notifications, even when it is not at sign-in. For Store apps this is controlled under Settings → Apps → Installed apps → WhatsApp → Advanced options → Background apps permissions.' },
  { m: /claude|cowork/i, t: 'The Cowork service appears to support Claude’s sandboxed environment and starts automatically with Windows. If it is marked Locked, its startup type cannot be changed from here or from Services.' },
  { m: /codex|chatgpt|openai/i, t: 'This looks like the sandbox service used by the ChatGPT/Codex desktop app and starts automatically with Windows. If it is marked Locked, its startup type cannot be changed from here or from Services.' },
  { m: /ollama/i, t: 'Ollama runs a background local LLM inference engine and REST API server (localhost:11434). It starts automatically to serve model requests on demand.' },
  { m: /lm studio|lms/i, t: 'LM Studio includes a local model execution daemon and CLI server for local AI acceleration.' },
  { m: /docker/i, t: 'Docker Desktop runs the background engine, WSL2 integration services, and virtualization daemons.' },
  { m: /steam/i, t: 'Steam Client WebHelper runs Chromium Embedded Framework (CEF) background processes to handle store rendering, friend activity, and cloud sync.' },
  { m: /epic/i, t: 'Epic Online Services and Epic Games Launcher background helpers maintain game entitlement checks, friend presence, and background updates.' },
  { m: /spotify/i, t: 'Spotify Web Helper runs in the background for media key control, URI handling, and desktop client integration.' },
  { m: /adobe|creative cloud|coresync/i, t: 'Adobe Creative Cloud uses multiple background synchronization daemons (CoreSync, CCXProcess) for cloud assets, font licensing, and automatic updates.' },
  { m: /discord/i, t: 'Discord launches in the background to listen for notifications, voice channels, and handle background hotkeys.' }
];

const SCALES = ['100%', '110%', '120%', '130%', '140%'];

// State
let history = [];
let snapshots = [];
let sentinelAlerts = [];
let pending = null;
let inv = null;
let sel = null;
let filter = 'all';
let query = '';
let sortMode = 'name';
let showSystem = false;
let page = 'overview';
let toastTimer = null;
const expanded = new Set();

try { showSystem = localStorage.getItem('bgm.showSystem') === '1'; } catch (e) {}
let uiScale = '100%';
try { uiScale = localStorage.getItem('bgm.uiScale') || '100%'; } catch (e) {}

const THEMES = ['system', 'dark', 'light', 'amoled'];
const ACCENTS = ['blue', 'purple', 'emerald', 'amber', 'rose', 'cyan'];

let currentTheme = 'dark';
try { currentTheme = localStorage.getItem('bgm.theme') || 'dark'; } catch (e) {}
let currentAccent = 'blue';
try { currentAccent = localStorage.getItem('bgm.accent') || 'blue'; } catch (e) {}

function isEffectiveLight() {
  if (currentTheme === 'light') return true;
  if (currentTheme === 'system') {
    return window.matchMedia && window.matchMedia('(prefers-color-scheme: light)').matches;
  }
  return false;
}

function applyTheme(theme) {
  if (!THEMES.includes(theme)) theme = 'dark';
  currentTheme = theme;
  try { localStorage.setItem('bgm.theme', theme); } catch (e) {}
  document.documentElement.dataset.theme = theme;
  const el = $('set-theme');
  if (el && el.value !== theme) el.value = theme;
}

function applyAccent(accent) {
  if (!ACCENTS.includes(accent)) accent = 'blue';
  currentAccent = accent;
  try { localStorage.setItem('bgm.accent', accent); } catch (e) {}
  document.documentElement.dataset.accent = accent;
  document.querySelectorAll('.accent-swatch').forEach(sw => {
    sw.classList.toggle('active', sw.dataset.color === accent);
  });
}

// Queries
function formatRAM(bytes) {
  if (!bytes || bytes <= 0) return '0 KB';
  const kb = 1024;
  const mb = 1024 * kb;
  const gb = 1024 * mb;
  if (bytes >= gb) return (bytes / gb).toFixed(2) + ' GB';
  if (bytes >= mb) return (bytes / mb).toFixed(1) + ' MB';
  return Math.round(bytes / kb) + ' KB';
}

function hue(s) {
  let h = 0;
  for (const c of String(s)) h = (h * 31 + c.charCodeAt(0)) % 360;
  return h;
}
function iconStyle(name) {
  const h = hue(name);
  if (isEffectiveLight()) {
    return `background:hsl(${h} 50% 92%);color:hsl(${h} 70% 32%);border-color:hsl(${h} 40% 82%)`;
  }
  return `background:hsl(${h} 22% 20%);color:hsl(${h} 65% 80%);border-color:hsl(${h} 25% 30%)`;
}
function apps() {
  return inv ? inv.apps.filter(a => showSystem || !a.system) : [];
}
function comps(kind) {
  return apps().flatMap(a => a.components.map(c => Object.assign({}, c, {appId: a.id, appName: a.name}))).filter(c => c.kind === kind && (showSystem || !c.system));
}
function base(p) {
  return String(p || '').split('\\').pop();
}
function fmtTime(iso) {
  if (!iso) return '—';
  const d = new Date(iso);
  return isNaN(d) ? '—' : d.toLocaleString([], {month:'short', day:'numeric', hour:'numeric', minute:'2-digit'});
}
function cfgText(c) {
  return c.kind === 'service' ? c.startType : c.kind === 'process' ? 'Running now' : c.kind === 'storeapp' ? (c.enabled ? 'Allowed' : 'Blocked') : (c.enabled ? 'Enabled' : 'Disabled');
}
let bootProfile = null;
let powerReport = null;

// Uninstaller & Residual Cleanup State
let installedApps = [];
let uninstallerSummary = {
  totalApps: 0,
  totalSizeBytes: 0,
  totalSizeFormatted: '0 B',
  heavyAppsCount: 0,
  userAppsCount: 0,
  systemAppsCount: 0
};
let uninstallerFilter = 'all'; // 'all', 'heavy', 'large', 'user'
let uninstallerSearch = '';
let uninstallerSort = 'size'; // 'size', 'name', 'date'
let currentResidualReport = null;
let selectedResidualIds = new Set();

// Live Process Manager, Telemetry Sparklines & File Lock Finder State
let liveProcessGroups = [];
let liveTelemetry = null;
let procFilter = 'all';
let procSearch = '';
let procSort = 'ram';
const telemetryHistory = {
  cpu: [],
  ram: [],
  net: []
};
let lockResults = [];
const appIconCache = {}; // lowercase path/spec -> data:image/png;base64,... or ''

// Junk & Cache Cleaner State
let junkReport = null;
let junkScanning = false;
let selectedJunkIds = new Set();

const JUNK_CATS = {
  temp:       { icon: 'file-clock',  color: '#38bdf8' },
  browser:    { icon: 'globe',       color: '#a78bfa' },
  update:     { icon: 'package',     color: '#34d399' },
  shader:     { icon: 'sparkles',    color: '#fbbf24' },
  thumbnails: { icon: 'image',       color: '#f472b6' },
  logs:       { icon: 'bug',         color: '#f87171' },
  windowsold: { icon: 'archive',     color: '#fb923c' },
  recyclebin: { icon: 'trash-2',     color: '#94a3b8' },
  orphanreg:  { icon: 'ghost',       color: '#c084fc' }
};


function sigText(c) {
  if (c.signer) {
    const isExp = (c.flags || []).includes('expired_cert');
    if (isExp) return 'Expired certificate (' + c.signer + ')';
    return (c.signature === 'valid' ? 'Signed by ' + c.signer : 'Unverified (' + c.signer + ')');
  }
  return ({valid:'Valid digital signature', unsigned:'Signature not verified', missing:'File not found', 'n/a':'Not applicable'})[c.signature] || '—';
}
function findComp(id) {
  if (!inv || !inv.apps) return null;
  for (const a of inv.apps) {
    for (const c of a.components) {
      if (c.id === id) return { c, a };
    }
  }
  return null;
}
function describe(c, action) {
  const n = c.display || c.name;
  const startup = {
    disable: ['Stop ' + n + ' starting at sign-in?', 'It will no longer launch by itself when you sign in. You can still open it yourself. This is the same switch Task Manager’s Startup apps uses.', 'Disable at sign-in'],
    enable: ['Let ' + n + ' start at sign-in?', 'It will launch automatically every time you sign in.', 'Enable at sign-in']
  };
  const T = {
    run: startup,
    folder: startup,
    task: {
      disable: ['Disable the task “' + n + '”?', 'It will stop running on its schedule or triggers. The task is kept, not deleted. Some updaters recreate their tasks.', 'Disable task'],
      enable: ['Enable the task “' + n + '”?', 'It will run again on its schedule or triggers.', 'Enable task']
    },
    shellex: {
      disable: ['Disable context menu extension “' + n + '”?', 'It will no longer appear when right-clicking files in File Explorer. It is blocked safely via Shell Extensions\\Blocked without registry deletions.', 'Disable extension'],
      enable: ['Enable context menu extension “' + n + '”?', 'It will appear in File Explorer right-click menus again.', 'Enable extension']
    },
    service: {
      disable: ['Set ' + n + ' to Manual start?', 'It will no longer start automatically with Windows. Windows or the app can still start it when needed. It is not stopped now.', 'Set to Manual'],
      enable: ['Set ' + n + ' to Automatic start?', 'It will start automatically with Windows.', 'Set to Automatic'],
      stop: ['Stop ' + n + ' now?', 'The service stops until it is started again. If it is set to Automatic it starts again at the next boot. Apps that need it may stop working meanwhile.', 'Stop service'],
      start: ['Start ' + n + ' now?', 'The service starts and keeps running until stopped or Windows restarts.', 'Start service']
    }
  };
  const t = (T[c.kind] || {})[action];
  if (!t) return null;
  const caution = c.caution && c.control === 'yes' ? `<p style="color:#e6c27e">${esc(c.caution)}</p>` : '';
  const vtInfo = c.virusTotalURL ? `<p style="margin-top:10px;font-size:11px"><a href="${esc(c.virusTotalURL)}" target="_blank" rel="noopener" style="color:var(--accent);text-decoration:underline;display:inline-flex;align-items:center;gap:4px"><i data-lucide="shield-check" style="width:13px;height:13px"></i>Verify file safety &amp; hash on VirusTotal</a></p>` : '';
  return {
    title: t[0],
    confirm: t[2],
    body: `<p>${esc(t[1])}</p>${caution}${vtInfo}<p style="margin-top:8px">The current setting is saved first. You can undo this from <strong>Change history</strong>.</p>`
  };
}
