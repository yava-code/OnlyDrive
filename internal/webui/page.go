package webui

// indexHTML is the single-page control panel. Plain HTML/JS, no dependencies.
//
// Design direction: "clean ops dashboard" chosen with the user (antislop R-37):
// ENERGY 1 / RHYTHM 2 / MOTION 1, dark theme for a developer tool (R-21 legit
// reason), one accent color (R-29: neutrals + 1 accent).
//
// Reasons (R-31): dark = dev tool runs on desktops, low glare; single accent
// (green) = status-first tool, green means healthy, used nowhere else (R-13/29);
// tabular numbers = storage figures must align for comparison; system font
// stack = zero deps, native rendering, matches CLI-first product.
const indexHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>gd control panel</title>
<style>
  :root {
    --bg: #16181d;
    --surface: #1e2127;
    --border: #303540;
    --text: #e8eaed;
    --muted: #9aa0a6;
    --accent: #4cc38a;
    --accent-ink: #0d1117;
    --danger: #f28b82;
  }
  * { box-sizing: border-box; }
  html, body { margin: 0; }
  body {
    background: var(--bg);
    color: var(--text);
    font: 15px/1.5 system-ui, -apple-system, "Segoe UI", sans-serif;
    padding: 24px;
  }
  header {
    display: flex;
    flex-wrap: wrap;
    gap: 12px;
    align-items: baseline;
    margin-bottom: 20px;
  }
  h1 { font-size: 20px; margin: 0; font-weight: 600; }
  #verdict { color: var(--muted); font-size: 14px; }
  .verdict-ok { color: var(--accent); }
  .verdict-bad { color: var(--danger); }

  /* One surface type for groups; hierarchy from spacing + headings (R-14). */
  section {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 16px;
    margin-bottom: 16px;
    max-width: 860px;
  }
  h2 { font-size: 15px; margin: 0 0 12px; font-weight: 600; }

  button {
    font: inherit;
    background: transparent;
    color: var(--text);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 10px 14px;
    min-height: 44px;
    cursor: pointer;
  }
  button:hover { border-color: var(--muted); }
  /* Visible focus ring, never removed (R-32). */
  button:focus-visible {
    outline: 2px solid var(--accent);
    outline-offset: 2px;
  }
  button.primary {
    background: var(--accent);
    border-color: var(--accent);
    color: var(--accent-ink);
    font-weight: 600;
  }
  button.danger { color: var(--danger); border-color: var(--danger); }
  button:disabled { opacity: .5; cursor: wait; }

  .row {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    align-items: center;
  }

  table { width: 100%; border-collapse: collapse; }
  th, td {
    text-align: left;
    padding: 8px 8px 8px 0;
    border-bottom: 1px solid var(--border);
  }
  th { color: var(--muted); font-weight: 500; }
  td.num { font-variant-numeric: tabular-nums; } /* storage numbers must align */

  /* R-25: muted text is 9aa0a6 on 1e2127 = ~7:1, passes AA. */
  .muted { color: var(--muted); }

  /* R-27: loading / empty / error states for the data area. */
  #status {
    min-height: 24px;
    margin: 0 0 12px;
    font-size: 14px;
  }
  #status:empty { display: none; }
  #status.error { color: var(--danger); }
  #status.busy::before {
    content: "loading…";
    color: var(--muted);
  }
  .empty { color: var(--muted); padding: 8px 0; }

  /* R-03: single column stack on narrow screens; 44px targets kept. */
  @media (max-width: 640px) {
    body { padding: 12px; }
    section { padding: 12px; }
    td, th { padding-right: 4px; }
  }
</style>
</head>
<body>
<header>
  <h1>gd control panel</h1>
  <span id="verdict" aria-live="polite">checking…</span>
</header>

<div id="status" role="status" aria-live="polite"></div>

<main id="app">
  <section>
    <h2>Google accounts</h2>
    <div id="accounts" class="empty">loading…</div>
    <div class="row" style="margin-top:12px">
      <button id="btn-add" class="primary">Add Google account</button>
    </div>
  </section>

  <section>
    <h2>Daemon</h2>
    <div class="row">
      <span id="daemon-state" class="muted">checking…</span>
      <button id="btn-daemon-start">Start</button>
      <button id="btn-daemon-stop">Stop</button>
    </div>
  </section>

  <section>
    <h2>S3 endpoint</h2>
    <div id="s3-state" class="muted">checking…</div>
    <div class="row" style="margin-top:12px">
      <button id="btn-s3-start">Start S3</button>
      <button id="btn-s3-stop">Stop S3</button>
    </div>
  </section>

  <section>
    <h2>Maintenance</h2>
    <div class="row">
      <button id="btn-autostart-on">Autostart on</button>
      <button id="btn-autostart-off">Autostart off</button>
      <button id="btn-doctor">Run doctor</button>
      <button id="btn-doctor-fix">Run doctor (fix)</button>
      <button id="btn-refresh">Refresh</button>
    </div>
  </section>
</main>

<footer class="muted" style="max-width:860px">
  <p>gd ui listens on 127.0.0.1 only. Set GD_UI_TOKEN to require an access token.</p>
</footer>

<script>
"use strict";
(function () {
  var status = document.getElementById("status");

  function setBusy(msg) {
    status.classList.remove("error");
    if (msg) {
      status.textContent = msg;
    } else {
      status.textContent = "";
      status.classList.add("busy");
    }
  }
  function setError(msg) {
    status.classList.remove("busy");
    status.classList.add("error");
    status.textContent = msg;
  }
  function clearStatus() {
    status.textContent = "";
    status.classList.remove("error", "busy");
  }

  function fmtGB(n) {
    if (!n || n <= 0) return "";
    var gb = n / 1073741824;
    return gb >= 1000 ? (gb / 1024).toFixed(2) + " TB" : gb.toFixed(1) + " GB";
  }

  function post(path, body) {
    return fetch(path, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body || {})
    }).then(function (res) {
      return res.json().then(function (data) {
        if (!res.ok) throw new Error(data.error || ("HTTP " + res.status));
        return data;
      });
    });
  }

  // refresh reloads the state. keepStatus leaves the action feedback line
  // alone, so a POST result does not flash away when the table re-renders.
  function refresh(keepStatus) {
    if (!keepStatus) setBusy();
    fetch("/api/state")
      .then(function (res) {
        if (!res.ok) throw new Error("HTTP " + res.status);
        return res.json();
      })
      .then(function (st) {
        render(st);
        if (!keepStatus) clearStatus();
      })
      .catch(function (e) { setError("failed to load state: " + e.message); });
  }

  function render(st) {
    var v = document.getElementById("verdict");
    v.textContent = st.daemon_running ? "daemon running" : "daemon stopped";
    v.className = st.daemon_running ? "verdict-ok" : "verdict-bad";

    var d = document.getElementById("daemon-state");
    d.textContent = st.daemon_running ? "running on 127.0.0.1:5572" : "stopped";
    d.className = st.daemon_running ? "verdict-ok" : "muted";

    var s3 = document.getElementById("s3-state");
    if (st.s3_running) {
      s3.textContent = "running on " + st.s3_addr + " (keys in the start response or gd serve status)";
      s3.className = "verdict-ok";
    } else {
      s3.textContent = "stopped";
      s3.className = "muted";
    }

    var el = document.getElementById("accounts");
    el.className = "";
    if (!st.accounts || st.accounts.length === 0) {
      el.className = "empty";
      el.textContent = "No accounts yet. Click \"Add Google account\" and allow access in the browser.";
      return;
    }
    el.innerHTML = "";
    var tbl = document.createElement("table");
    tbl.innerHTML = "<thead><tr><th>Name</th><th>Email</th><th>Quota</th><th>Actions</th></tr></thead>";
    var tb = document.createElement("tbody");
    st.accounts.forEach(function (a) {
      var tr = document.createElement("tr");
      var td1 = document.createElement("td");
      td1.textContent = a.name;
      var td2 = document.createElement("td");
      td2.textContent = a.email;
      var td3 = document.createElement("td");
      td3.className = "num";
      td3.textContent = a.quota_ok ? fmtGB(a.used) + " / " + fmtGB(a.total) : "unavailable";
      var td4 = document.createElement("td");
      var mkBtn = function (label, path, body, cls) {
        var b = document.createElement("button");
        b.textContent = label;
        if (cls) b.className = cls;
        b.addEventListener("click", function () { act(path, body); });
        return b;
      };
      td4.appendChild(mkBtn(a.mounted ? "Unmount" : "Mount", a.mounted ? "/api/unmount" : "/api/mount", { account: a.name }));
      td4.appendChild(document.createTextNode(" "));
      td4.appendChild(mkBtn("Remove", "/api/remove", { account: a.name }, "danger"));
      tr.appendChild(td1); tr.appendChild(td2); tr.appendChild(td3); tr.appendChild(td4);
      tb.appendChild(tr);
    });
    tbl.appendChild(tb);
    el.appendChild(tbl);
  }

  function act(path, body) {
    setBusy("working…");
    var btns = document.querySelectorAll("button");
    btns.forEach(function (b) { b.disabled = true; });
    post(path, body).then(function (res) {
      status.classList.remove("busy");
      status.textContent = res.message || "done";
      refresh(true);
    }).catch(function (e) {
      setError(e.message);
      refresh(true);
    }).finally(function () {
      btns.forEach(function (b) { b.disabled = false; });
    });
  }

  // Wire every button to a real endpoint (R-26).
  document.getElementById("btn-add").addEventListener("click", function () {
    act("/api/add", {});
  });
  document.getElementById("btn-daemon-start").addEventListener("click", function () {
    act("/api/daemon", { action: "start" });
  });
  document.getElementById("btn-daemon-stop").addEventListener("click", function () {
    act("/api/daemon", { action: "stop" });
  });
  document.getElementById("btn-s3-start").addEventListener("click", function () {
    act("/api/serve-s3", { action: "start" });
  });
  document.getElementById("btn-s3-stop").addEventListener("click", function () {
    act("/api/serve-s3", { action: "stop" });
  });
  document.getElementById("btn-autostart-on").addEventListener("click", function () {
    act("/api/autostart", { action: "on" });
  });
  document.getElementById("btn-autostart-off").addEventListener("click", function () {
    act("/api/autostart", { action: "off" });
  });
  document.getElementById("btn-doctor").addEventListener("click", function () {
    act("/api/doctor", { fix: false });
  });
  document.getElementById("btn-doctor-fix").addEventListener("click", function () {
    act("/api/doctor", { fix: true });
  });
  document.getElementById("btn-refresh").addEventListener("click", refresh);
  refresh();
})();
</script>
</body>
</html>`
