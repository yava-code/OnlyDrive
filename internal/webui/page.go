package webui

// indexHTML is the single-page control panel. Plain HTML/JS, no build step;
// fonts load from Google Fonts, the logo is embedded in the binary.
//
// Design direction: "editorial tech journal" from the user's DESIGN.md
// (antislop R-37): light warm canvas, hairline borders, JetBrains Mono for
// numbers and labels, Inter for UI text, one ink-black accent for primary
// actions and quota bars. Dials ENERGY 1 / RHYTHM 2 / MOTION 1: staggered
// entrance on load, LED pulse on the running state, everything else calm.
// Reasons (R-31): warm off-white canvas reads as paper for an editorial
// identity; mono for figures keeps storage numbers aligned and legible;
// ink-black is the single accent (status-first tool); hairline borders make
// sections breathe without cards shouting.
const indexHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>OnlyDrive control panel</title>
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600&family=JetBrains+Mono:wght@400;500&display=swap" rel="stylesheet">
<style>
  :root {
    --canvas: #fafafa;
    --surface: #ffffff;
    --hairline: #e5e5e5;
    --hairline-soft: #f0f0f0;
    --ink: #111111;
    --ink-soft: #525252;
    --muted: #737373;
    --faint: #a3a3a3;
    --ok: #10b981;
    --ok-soft: #ecfdf5;
    --ok-ink: #065f46;
    --warn: #d97706;
    --danger: #dc2626;
    --danger-soft: #fef2f2;
  }
  * { box-sizing: border-box; }
  html, body { margin: 0; }
  body {
    background: var(--canvas);
    color: var(--ink);
    font: 14px/1.5 Inter, system-ui, "Segoe UI", sans-serif;
    -webkit-font-smoothing: antialiased;
  }
  .mono { font-family: "JetBrains Mono", ui-monospace, Menlo, monospace; }

  /* Header: sticky, hairline, logo + verdict + refresh */
  header {
    position: sticky; top: 0; z-index: 40;
    background: rgba(255,255,255,.85);
    backdrop-filter: blur(8px);
    border-bottom: 1px solid var(--hairline);
  }
  .bar {
    max-width: 1024px; margin: 0 auto; padding: 0 24px;
    height: 56px; display: flex; align-items: center; gap: 12px;
  }
  .logo { height: 30px; width: auto; display: block; }
  .slash { color: var(--faint); font-weight: 300; }
  .brand-sub { font-size: 13px; font-weight: 500; letter-spacing: -0.01em; }
  .spacer { flex: 1; }
  #verdict { font-family: "JetBrains Mono", monospace; font-size: 12px; }
  .led {
    width: 7px; height: 7px; border-radius: 50%;
    display: inline-block; margin-right: 6px; vertical-align: 1px;
  }
  .led-ok { background: var(--ok); }
  .led-off { background: var(--faint); }
  .verdict-ok { color: var(--ok-ink); }
  .verdict-bad { color: var(--muted); }

  @keyframes ping {
    0% { transform: scale(.9); opacity: .8; }
    60%, 100% { transform: scale(2.2); opacity: 0; }
  }
  .led-ping { position: relative; }
  .led-ping::after {
    content: ""; position: absolute; inset: 0; border-radius: 50%;
    background: var(--ok); animation: ping 2.6s cubic-bezier(0,0,.2,1) infinite;
  }

  button {
    font: inherit; cursor: pointer; border-radius: 6px;
    transition: background .15s, border-color .15s, color .15s, transform .1s;
  }
  button:active { transform: scale(.98); }
  button:focus-visible { outline: 2px solid var(--ink); outline-offset: 2px; }
  button:disabled { opacity: .55; cursor: wait; }
  .btn {
    font-family: "JetBrains Mono", monospace; font-size: 12px; font-weight: 500;
    padding: 7px 12px; border: 1px solid var(--hairline);
    background: var(--surface); color: var(--ink-soft); min-height: 36px;
  }
  .btn:hover { border-color: #d4d4d4; color: var(--ink); }
  .btn.primary {
    background: var(--ink); border-color: var(--ink); color: #fff;
  }
  .btn.primary:hover { background: #262626; }
  .btn.danger-text { color: var(--muted); }
  .btn.danger-text:hover { color: var(--danger); background: var(--danger-soft); border-color: #fecaca; }

  main {
    max-width: 1024px; margin: 0 auto; padding: 36px 24px 0;
    display: flex; flex-direction: column; gap: 36px;
  }
  h2 { font-size: 14px; font-weight: 500; letter-spacing: -0.01em; margin: 0; }
  .sec-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; }

  .card {
    background: var(--surface); border: 1px solid var(--hairline);
    border-radius: 8px; overflow: hidden;
  }
  .pad { padding: 18px; }

  /* Entrance: one staggered reveal per section, then stillness */
  @keyframes rise { from { opacity: 0; transform: translateY(6px); } to { opacity: 1; transform: none; } }
  .enter { animation: rise .4s cubic-bezier(.16,1,.3,1) both; }
  .d1 { animation-delay: .05s; } .d2 { animation-delay: .1s; } .d3 { animation-delay: .15s; }

  /* Accounts table */
  table { width: 100%; border-collapse: collapse; font-size: 13px; }
  thead th {
    font-family: "JetBrains Mono", monospace; font-size: 11px; font-weight: 500;
    color: var(--muted); text-transform: uppercase; letter-spacing: .04em;
    text-align: left; padding: 10px 16px; background: #fbfbfb;
    border-bottom: 1px solid var(--hairline-soft);
  }
  tbody td { padding: 12px 16px; border-bottom: 1px solid var(--hairline-soft); }
  tbody tr:last-child td { border-bottom: 0; }
  tbody tr:hover { background: #fafafa; }
  td.mono, .mono-td { font-family: "JetBrains Mono", monospace; font-size: 12px; }
  .acc-name { font-weight: 500; }
  .mount-dot { width: 7px; height: 7px; border-radius: 50%; display: inline-block; margin-right: 8px; vertical-align: 1px; }
  .on { background: var(--ok); }
  .off { background: var(--faint); }

  /* Quota meter */
  .quota { width: 190px; display: flex; flex-direction: column; gap: 6px; }
  .quota-nums { display: flex; justify-content: space-between; color: var(--muted); font-size: 11px; }
  .quota-track { height: 4px; border-radius: 999px; background: #eeeeee; overflow: hidden; }
  .quota-fill { height: 100%; background: var(--ink); border-radius: 999px; transition: width .7s ease-out; }
  .quota-fill.hot { background: var(--warn); }

  /* Service cards: daemon + s3 side by side */
  .grid2 { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
  @media (max-width: 700px) { .grid2 { grid-template-columns: 1fr; } }
  .svc-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 8px; }
  .svc-sub { font-family: "JetBrains Mono", monospace; font-size: 12px; color: var(--muted); margin-top: 2px; }
  .badge {
    display: inline-flex; align-items: center; gap: 6px;
    font-family: "JetBrains Mono", monospace; font-size: 11px; font-weight: 500;
    padding: 3px 9px; border: 1px solid var(--hairline); border-radius: 999px;
    background: #fafafa; color: var(--muted);
  }
  .badge.ok { background: var(--ok-soft); border-color: #a7f3d0; color: var(--ok-ink); }
  .badge.warn { color: var(--warn); border-color: #fde68a; background: #fffbeb; }
  .btn-row { display: flex; gap: 8px; margin-top: 16px; padding-top: 14px; border-top: 1px solid var(--hairline-soft); }
  .btn-row .btn { flex: 1; }

  /* Maintenance row */
  .seg { display: inline-flex; padding: 2px; background: #f5f5f5; border: 1px solid var(--hairline); border-radius: 6px; }
  .seg button {
    font-family: "JetBrains Mono", monospace; font-size: 12px; font-weight: 500;
    padding: 6px 12px; border: 0; background: transparent; color: var(--muted); border-radius: 5px;
  }
  .seg button.active { background: #fff; color: var(--ink); box-shadow: 0 1px 2px rgba(0,0,0,.06); }
  .maint-row { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 12px; }

  /* Doctor drawer: quiet mono output */
  #doctor-panel { margin-top: 14px; padding-top: 14px; border-top: 1px solid var(--hairline-soft); display: none; }
  #doctor-panel.open { display: block; }
  .doctor-head {
    display: flex; justify-content: space-between; align-items: center;
    font-family: "JetBrains Mono", monospace; font-size: 11px; color: var(--faint); margin-bottom: 8px;
  }
  .doctor-head button { border: 0; background: none; color: var(--faint); font: inherit; padding: 2px 6px; border-radius: 4px; }
  .doctor-head button:hover { color: var(--ink); background: #f0f0f0; }
  #doctor-output {
    margin: 0; padding: 14px; border: 1px solid var(--hairline-soft); border-radius: 6px;
    background: #fbfbfb; font-family: "JetBrains Mono", monospace; font-size: 11.5px;
    line-height: 1.7; color: #262626; max-height: 220px; overflow-y: auto; white-space: pre-wrap;
  }
  #doctor-output .ok-line { color: var(--ok-ink); }
  #doctor-output .bad-line { color: var(--danger); }

  /* Status line + toasts */
  #status { min-height: 20px; margin: -18px 0 0; font-family: "JetBrains Mono", monospace; font-size: 12px; color: var(--muted); }
  #status.error { color: var(--danger); }
  #status:empty { display: none; }
  #toasts { position: fixed; right: 20px; bottom: 20px; display: flex; flex-direction: column; gap: 8px; z-index: 60; }
  .toast {
    display: flex; align-items: center; gap: 8px;
    background: #fff; border: 1px solid var(--hairline); border-radius: 8px;
    box-shadow: 0 4px 16px rgba(0,0,0,.06); padding: 10px 14px;
    font-family: "JetBrains Mono", monospace; font-size: 12px; color: var(--ink);
    opacity: 0; transform: translateY(8px); transition: all .2s;
  }
  .toast.show { opacity: 1; transform: none; }

  /* Footnote */
  .footnote {
    display: flex; align-items: center; gap: 10px;
    border: 1px solid var(--hairline); background: var(--surface);
    border-radius: 8px; padding: 12px 16px;
    font-family: "JetBrains Mono", monospace; font-size: 12px; color: var(--muted);
  }
  .footnote b { color: var(--ink); font-weight: 500; }
  footer {
    border-top: 1px solid var(--hairline); margin-top: 40px; background: var(--surface);
  }
  footer .bar { height: 48px; font-family: "JetBrains Mono", monospace; font-size: 11px; color: var(--faint); }

  .empty { color: var(--muted); padding: 16px; font-family: "JetBrains Mono", monospace; font-size: 12px; }

  @media (max-width: 640px) {
    .bar, main { padding-left: 14px; padding-right: 14px; }
    main { padding-top: 24px; gap: 24px; }
    .quota { width: 130px; }
    .brand-sub, .slash { display: none; }
  }
  @media (prefers-reduced-motion: reduce) {
    * { animation-duration: .01ms !important; transition-duration: .01ms !important; }
  }
</style>
</head>
<body>
<header>
  <div class="bar">
    <img class="logo" src="/logo.png" alt="OnlyDrive">
    <span class="slash">/</span>
    <span class="brand-sub">control panel</span>
    <span class="spacer"></span>
    <span id="verdict" aria-live="polite"><span class="led led-off"></span>checking…</span>
    <button class="btn" id="btn-refresh" type="button">Refresh</button>
  </div>
</header>

<main>
  <div id="status" role="status" aria-live="polite"></div>

  <section class="enter d1">
    <div class="sec-head" style="margin-bottom:12px">
      <h2>Google accounts</h2>
      <button class="btn primary" id="btn-add" type="button">+ Add Google account</button>
    </div>
    <div class="card">
      <div id="accounts" class="empty">loading…</div>
    </div>
  </section>

  <section class="enter d2">
    <div class="grid2">
      <div class="card pad">
        <div class="svc-head">
          <div>
            <h2>Daemon</h2>
            <div class="svc-sub">127.0.0.1:5572</div>
          </div>
          <span class="badge" id="daemon-badge"><span class="led led-off" id="daemon-led"></span><span id="daemon-state">checking…</span></span>
        </div>
        <div class="btn-row">
          <button class="btn" id="btn-daemon-start" type="button">Start</button>
          <button class="btn" id="btn-daemon-stop" type="button">Stop</button>
        </div>
      </div>
      <div class="card pad">
        <div class="svc-head">
          <div>
            <h2>S3 endpoint</h2>
            <div class="svc-sub">127.0.0.1:9000</div>
          </div>
          <span class="badge" id="s3-badge"><span class="led led-off" id="s3-led"></span><span id="s3-state">checking…</span></span>
        </div>
        <div class="btn-row">
          <button class="btn" id="btn-s3-start" type="button">Start S3</button>
          <button class="btn" id="btn-s3-stop" type="button">Stop S3</button>
        </div>
      </div>
    </div>
  </section>

  <section class="enter d2">
    <div class="grid2">
      <div class="card pad">
        <div class="svc-head">
          <div>
            <h2>WebDAV</h2>
            <div class="svc-sub">127.0.0.1:9864 · mount on macOS/Linux</div>
          </div>
          <span class="badge" id="dav-badge"><span class="led led-off" id="dav-led"></span><span id="dav-state">checking…</span></span>
        </div>
        <div class="btn-row">
          <button class="btn" id="btn-dav-start" type="button">Start WebDAV</button>
          <button class="btn" id="btn-dav-stop" type="button">Stop WebDAV</button>
        </div>
      </div>
      <div class="card pad" style="visibility:hidden" aria-hidden="true">
      </div>
    </div>
  </section>

  <section class="enter d3">
    <div class="sec-head" style="margin-bottom:12px">
      <h2>Maintenance</h2>
    </div>
    <div class="card pad">
      <div class="maint-row">
        <div class="seg" role="group" aria-label="Autostart">
          <button id="btn-autostart-on" type="button">Autostart on</button>
          <button id="btn-autostart-off" type="button">Autostart off</button>
        </div>
        <div style="display:flex;gap:8px">
          <button class="btn" id="btn-doctor" type="button">Run doctor</button>
          <button class="btn" id="btn-doctor-fix" type="button">Run doctor (fix)</button>
        </div>
      </div>
      <div id="doctor-panel">
        <div class="doctor-head">
          <span id="doctor-title">diagnostics</span>
          <button id="btn-doctor-close" type="button">dismiss [x]</button>
        </div>
        <pre id="doctor-output"></pre>
      </div>
    </div>
  </section>

  <div class="footnote enter d3">
    <span>lock</span>
    <p style="margin:0">gd ui listens on <b>127.0.0.1</b> only. Set <b>GD_UI_TOKEN</b> to require an access token.</p>
  </div>
</main>

<footer>
  <div class="bar">
    <span>OnlyDrive <span id="foot-ver">v0.1.2</span></span>
    <span class="spacer"></span>
    <span><span class="led led-off" id="foot-led"></span><span id="foot-state">connecting</span></span>
  </div>
</footer>

<div id="toasts" aria-live="polite"></div>

<script>
"use strict";
(function () {
  var status = document.getElementById("status");

  function setBusy(msg) {
    status.classList.remove("error");
    status.textContent = msg || "working…";
  }
  function setError(msg) {
    status.classList.add("error");
    status.textContent = msg;
  }
  function clearStatus() {
    status.textContent = "";
    status.classList.remove("error");
  }

  function toast(msg, kind) {
    var box = document.getElementById("toasts");
    var t = document.createElement("div");
    t.className = "toast";
    var dot = document.createElement("span");
    dot.className = "led " + (kind === "bad" ? "off" : "led-ok");
    if (kind === "bad") dot.style.background = "#dc2626";
    t.appendChild(dot);
    t.appendChild(document.createTextNode(msg));
    box.appendChild(t);
    requestAnimationFrame(function () { t.classList.add("show"); });
    setTimeout(function () {
      t.classList.remove("show");
      setTimeout(function () { t.remove(); }, 250);
    }, 3200);
  }

  function fmtGB(n) {
    if (!n || n <= 0) return "";
    var gb = n / 1073741824;
    return gb >= 1024 ? (gb / 1024).toFixed(2) + " TB" : gb.toFixed(1) + " GB";
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

  function setLed(id, on, pulse) {
    var el = document.getElementById(id);
    if (!el) return;
    el.className = "led " + (on ? "led-ok" : "led-off") + (on && pulse ? " led-ping" : "");
  }

  function refresh(keepStatus) {
    if (!keepStatus) setBusy("loading…");
    fetch("/api/state")
      .then(function (res) {
        if (!res.ok) throw new Error("HTTP " + res.status);
        return res.json();
      })
      .then(function (st) {
        render(st);
        if (!keepStatus) clearStatus();
      })
      .catch(function (e) {
        setError("failed to load state: " + e.message);
        setLed("foot-led", false);
        document.getElementById("foot-state").textContent = "offline";
      });
  }

  function render(st) {
    var v = document.getElementById("verdict");
    v.innerHTML = "";
    var led = document.createElement("span");
    led.className = "led " + (st.daemon_running ? "led-ok led-ping" : "led-off");
    v.appendChild(led);
    v.appendChild(document.createTextNode(st.daemon_running ? "daemon running" : "daemon stopped"));
    v.className = st.daemon_running ? "verdict-ok" : "verdict-bad";

    setLed("daemon-led", st.daemon_running, true);
    var d = document.getElementById("daemon-state");
    d.textContent = st.daemon_running ? "running" : "stopped";
    document.getElementById("daemon-badge").className = "badge" + (st.daemon_running ? " ok" : "");

    setLed("s3-led", st.s3_running, true);
    var s3 = document.getElementById("s3-state");
    s3.textContent = st.s3_running ? "running " + st.s3_addr : "idle";
    document.getElementById("s3-badge").className = "badge" + (st.s3_running ? " ok" : "");
    setLed("dav-led", st.webdav_running, true);
    var dav = document.getElementById("dav-state");
    dav.textContent = st.webdav_running ? "running " + st.webdav_addr : "idle";
    document.getElementById("dav-badge").className = "badge" + (st.webdav_running ? " ok" : "");

    setLed("foot-led", st.daemon_running, true);
    document.getElementById("foot-state").textContent = st.daemon_running ? "local rpc active" : "rpc stopped";

    var el = document.getElementById("accounts");
    el.className = "";
    if (!st.accounts || st.accounts.length === 0) {
      el.className = "empty";
      el.textContent = "No accounts yet. Click \"Add Google account\" and allow access in the browser.";
      return;
    }
    el.innerHTML = "";
    var tbl = document.createElement("table");
    tbl.innerHTML = "<thead><tr><th>Name</th><th>Email</th><th>Quota</th><th style=\"text-align:right\">Actions</th></tr></thead>";
    var tb = document.createElement("tbody");
    st.accounts.forEach(function (a) {
      var tr = document.createElement("tr");

      var td1 = document.createElement("td");
      td1.className = "mono acc-name";
      var dot = document.createElement("span");
      dot.className = "mount-dot " + (a.mounted ? "on" : "off");
      dot.title = a.mounted ? "mounted" : "not mounted";
      td1.appendChild(dot);
      td1.appendChild(document.createTextNode(a.name));

      var td2 = document.createElement("td");
      td2.className = "mono";
      td2.style.color = "var(--ink-soft)";
      td2.textContent = a.email;

      var td3 = document.createElement("td");
      var q = document.createElement("div");
      q.className = "quota mono";
      var nums = document.createElement("div");
      nums.className = "quota-nums";
      var pct = (a.quota_ok && a.total > 0) ? Math.min(100, a.used / a.total * 100) : 0;
      nums.innerHTML = "<span>" + (a.quota_ok ? fmtGB(a.used) + " used" : "unavailable") + "</span><span>" + fmtGB(a.total) + "</span>";
      var track = document.createElement("div");
      track.className = "quota-track";
      var fill = document.createElement("div");
      fill.className = "quota-fill" + (pct > 80 ? " hot" : "");
      fill.style.width = Math.max(pct, 0.8) + "%";
      track.appendChild(fill);
      q.appendChild(nums);
      q.appendChild(track);
      td3.appendChild(q);

      var td4 = document.createElement("td");
      td4.style.textAlign = "right";
      var mkBtn = function (label, path, body, cls) {
        var b = document.createElement("button");
        b.type = "button";
        b.textContent = label;
        b.className = "btn " + (cls || "");
        b.style.minHeight = "30px";
        b.style.padding = "4px 10px";
        b.addEventListener("click", function () { act(path, body); });
        return b;
      };
      td4.appendChild(mkBtn(a.mounted ? "Unmount" : "Mount", a.mounted ? "/api/unmount" : "/api/mount", { account: a.name }));
      td4.appendChild(document.createTextNode(" "));
      td4.appendChild(mkBtn("Remove", "/api/remove", { account: a.name }, "danger-text"));
      tr.appendChild(td1); tr.appendChild(td2); tr.appendChild(td3); tr.appendChild(td4);
      tb.appendChild(tr);
    });
    tbl.appendChild(tb);
    el.appendChild(tbl);
  }

  function act(path, body) {
    setBusy();
    var btns = document.querySelectorAll("button");
    btns.forEach(function (b) { b.disabled = true; });
    post(path, body).then(function (res) {
      clearStatus();
      toast(res.message || "done");
      refresh(true);
    }).catch(function (e) {
      setError(e.message);
      toast(e.message, "bad");
      refresh(true);
    }).finally(function () {
      btns.forEach(function (b) { b.disabled = false; });
    });
  }

  // Doctor drawer: run diagnostics, print checks line by line.
  function runDoctor(fix) {
    var panel = document.getElementById("doctor-panel");
    var out = document.getElementById("doctor-output");
    document.getElementById("doctor-title").textContent = fix ? "doctor --fix (repair mode)" : "doctor --check";
    out.textContent = "scanning environment…";
    panel.classList.add("open");
    post("/api/doctor", { fix: fix }).then(function (res) {
      out.textContent = "";
      (res.checks || []).forEach(function (c) {
        var line = document.createElement("div");
        var mark = c.ok ? "[ok]   " : "[FAIL] ";
        line.textContent = mark + c.name + ": " + c.detail;
        line.className = c.ok ? "ok-line" : "bad-line";
        out.appendChild(line);
      });
      toast("doctor finished");
    }).catch(function (e) {
      out.textContent = "doctor failed: " + e.message;
      toast(e.message, "bad");
    });
  }

  // Wire every control to a real endpoint (R-26).
  document.getElementById("btn-add").addEventListener("click", function () { act("/api/add", {}); });
  document.getElementById("btn-daemon-start").addEventListener("click", function () { act("/api/daemon", { action: "start" }); });
  document.getElementById("btn-daemon-stop").addEventListener("click", function () { act("/api/daemon", { action: "stop" }); });
  document.getElementById("btn-s3-start").addEventListener("click", function () { act("/api/serve-s3", { action: "start" }); });
  document.getElementById("btn-s3-stop").addEventListener("click", function () { act("/api/serve-s3", { action: "stop" }); });
  document.getElementById("btn-dav-start").addEventListener("click", function () { act("/api/serve-webdav", { action: "start" }); });
  document.getElementById("btn-dav-stop").addEventListener("click", function () { act("/api/serve-webdav", { action: "stop" }); });
  document.getElementById("btn-autostart-on").addEventListener("click", function (e) {
    e.target.classList.add("active");
    document.getElementById("btn-autostart-off").classList.remove("active");
    act("/api/autostart", { action: "on" });
  });
  document.getElementById("btn-autostart-off").addEventListener("click", function (e) {
    e.target.classList.add("active");
    document.getElementById("btn-autostart-on").classList.remove("active");
    act("/api/autostart", { action: "off" });
  });
  document.getElementById("btn-doctor").addEventListener("click", function () { runDoctor(false); });
  document.getElementById("btn-doctor-fix").addEventListener("click", function () { runDoctor(true); });
  document.getElementById("btn-doctor-close").addEventListener("click", function () {
    document.getElementById("doctor-panel").classList.remove("open");
  });
  document.getElementById("btn-refresh").addEventListener("click", function () { refresh(); });
  refresh();
})();
</script>
</body>
</html>`
