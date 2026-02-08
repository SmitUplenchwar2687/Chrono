package server

// DashboardHTML is the embedded single-page dashboard for Chrono.
// It connects via WebSocket and displays rate limit decisions in real time.
const DashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Chrono Dashboard</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, monospace;
    background: #0d1117; color: #c9d1d9; padding: 20px;
  }
  h1 { color: #58a6ff; margin-bottom: 4px; font-size: 1.5em; }
  .subtitle { color: #8b949e; margin-bottom: 20px; font-size: 0.9em; }
  .status-bar {
    display: flex; gap: 20px; margin-bottom: 20px; padding: 12px 16px;
    background: #161b22; border: 1px solid #30363d; border-radius: 6px;
  }
  .status-item { display: flex; flex-direction: column; }
  .status-label { font-size: 0.75em; color: #8b949e; text-transform: uppercase; }
  .status-value { font-size: 1.1em; font-weight: 600; }
  .status-value.connected { color: #3fb950; }
  .status-value.disconnected { color: #f85149; }
  .stats {
    display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
    gap: 12px; margin-bottom: 20px;
  }
  .stat-card {
    background: #161b22; border: 1px solid #30363d; border-radius: 6px;
    padding: 16px; text-align: center;
  }
  .stat-number { font-size: 2em; font-weight: 700; }
  .stat-number.allowed { color: #3fb950; }
  .stat-number.denied { color: #f85149; }
  .stat-number.total { color: #58a6ff; }
  .stat-number.rate { color: #d2a8ff; }
  .stat-label { font-size: 0.8em; color: #8b949e; margin-top: 4px; }
  .event-log {
    background: #161b22; border: 1px solid #30363d; border-radius: 6px;
    max-height: 500px; overflow-y: auto;
  }
  .event-header {
    padding: 12px 16px; border-bottom: 1px solid #30363d;
    font-weight: 600; color: #58a6ff; position: sticky; top: 0;
    background: #161b22; display: flex; justify-content: space-between;
  }
  .event-row {
    display: grid; grid-template-columns: 180px 120px 1fr 80px 100px;
    padding: 8px 16px; border-bottom: 1px solid #21262d;
    font-size: 0.85em; align-items: center;
    animation: fadeIn 0.3s ease;
  }
  .event-row:hover { background: #1c2128; }
  .badge {
    display: inline-block; padding: 2px 8px; border-radius: 12px;
    font-size: 0.75em; font-weight: 600;
  }
  .badge.allow { background: #23312e; color: #3fb950; }
  .badge.deny { background: #3d1f20; color: #f85149; }
  .remaining-bar {
    width: 60px; height: 6px; background: #21262d; border-radius: 3px;
    display: inline-block; overflow: hidden; vertical-align: middle;
  }
  .remaining-fill {
    height: 100%; border-radius: 3px; transition: width 0.3s;
  }
  .remaining-fill.high { background: #3fb950; }
  .remaining-fill.mid { background: #d29922; }
  .remaining-fill.low { background: #f85149; }
  .empty-state {
    text-align: center; padding: 60px 20px; color: #8b949e;
  }
  .empty-state .icon { font-size: 3em; margin-bottom: 10px; }
  .key-cell { color: #d2a8ff; }
  .endpoint-cell { color: #c9d1d9; }
  .time-cell { color: #8b949e; }
  #clear-btn {
    background: #21262d; color: #c9d1d9; border: 1px solid #30363d;
    padding: 4px 12px; border-radius: 4px; cursor: pointer; font-size: 0.8em;
  }
  #clear-btn:hover { background: #30363d; }
  @keyframes fadeIn { from { opacity: 0; transform: translateY(-4px); } to { opacity: 1; transform: translateY(0); } }
</style>
</head>
<body>
<h1>Chrono Dashboard</h1>
<p class="subtitle">Real-time rate limit decision viewer</p>

<div class="status-bar">
  <div class="status-item">
    <span class="status-label">Connection</span>
    <span class="status-value disconnected" id="conn-status">Disconnected</span>
  </div>
  <div class="status-item">
    <span class="status-label">Events/sec</span>
    <span class="status-value" id="events-per-sec">0</span>
  </div>
</div>

<div class="stats">
  <div class="stat-card">
    <div class="stat-number total" id="stat-total">0</div>
    <div class="stat-label">Total Requests</div>
  </div>
  <div class="stat-card">
    <div class="stat-number allowed" id="stat-allowed">0</div>
    <div class="stat-label">Allowed</div>
  </div>
  <div class="stat-card">
    <div class="stat-number denied" id="stat-denied">0</div>
    <div class="stat-label">Denied</div>
  </div>
  <div class="stat-card">
    <div class="stat-number rate" id="stat-rate">0%</div>
    <div class="stat-label">Deny Rate</div>
  </div>
</div>

<div class="event-log">
  <div class="event-header">
    <span>Live Events</span>
    <button id="clear-btn" onclick="clearEvents()">Clear</button>
  </div>
  <div id="events">
    <div class="empty-state">
      <div class="icon">&#9201;</div>
      <p>Waiting for rate limit events...</p>
      <p style="margin-top:8px;font-size:0.85em">Send requests to /api/check/{key} to see decisions here</p>
    </div>
  </div>
</div>

<script>
let total = 0, allowed = 0, denied = 0;
let recentTimestamps = [];
const eventsDiv = document.getElementById('events');
const MAX_EVENTS = 200;

function connect() {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  const ws = new WebSocket(proto + '//' + location.host + '/ws');

  ws.onopen = () => {
    document.getElementById('conn-status').textContent = 'Connected';
    document.getElementById('conn-status').className = 'status-value connected';
  };

  ws.onclose = () => {
    document.getElementById('conn-status').textContent = 'Disconnected';
    document.getElementById('conn-status').className = 'status-value disconnected';
    setTimeout(connect, 2000);
  };

  ws.onmessage = (e) => {
    const event = JSON.parse(e.data);
    addEvent(event);
  };
}

function addEvent(event) {
  // Remove empty state on first event.
  const empty = eventsDiv.querySelector('.empty-state');
  if (empty) empty.remove();

  total++;
  if (event.decision.allowed) allowed++;
  else denied++;

  // Track events per second.
  const now = Date.now();
  recentTimestamps.push(now);
  recentTimestamps = recentTimestamps.filter(t => now - t < 1000);

  updateStats();

  const row = document.createElement('div');
  row.className = 'event-row';

  const time = new Date(event.time).toLocaleTimeString('en-US', {hour12: false, hour:'2-digit', minute:'2-digit', second:'2-digit', fractionalSecondDigits: 3});
  const status = event.decision.allowed ? '<span class="badge allow">ALLOW</span>' : '<span class="badge deny">DENY</span>';
  const pct = event.decision.limit > 0 ? (event.decision.remaining / event.decision.limit) * 100 : 0;
  const fillClass = pct > 50 ? 'high' : pct > 20 ? 'mid' : 'low';

  row.innerHTML =
    '<span class="time-cell">' + time + '</span>' +
    '<span class="key-cell">' + escHtml(event.record.key) + '</span>' +
    '<span class="endpoint-cell">' + escHtml(event.record.endpoint) + '</span>' +
    '<span>' + status + '</span>' +
    '<span>' + event.decision.remaining + '/' + event.decision.limit + ' <span class="remaining-bar"><span class="remaining-fill ' + fillClass + '" style="width:' + pct + '%"></span></span></span>';

  eventsDiv.insertBefore(row, eventsDiv.firstChild);

  // Cap displayed events.
  while (eventsDiv.children.length > MAX_EVENTS) {
    eventsDiv.removeChild(eventsDiv.lastChild);
  }
}

function updateStats() {
  document.getElementById('stat-total').textContent = total;
  document.getElementById('stat-allowed').textContent = allowed;
  document.getElementById('stat-denied').textContent = denied;
  const rate = total > 0 ? ((denied / total) * 100).toFixed(1) + '%' : '0%';
  document.getElementById('stat-rate').textContent = rate;
  document.getElementById('events-per-sec').textContent = recentTimestamps.length;
}

function clearEvents() {
  total = 0; allowed = 0; denied = 0; recentTimestamps = [];
  eventsDiv.innerHTML = '<div class="empty-state"><div class="icon">&#9201;</div><p>Waiting for rate limit events...</p></div>';
  updateStats();
}

function escHtml(s) {
  const d = document.createElement('div');
  d.textContent = s;
  return d.innerHTML;
}

connect();
</script>
</body>
</html>`
