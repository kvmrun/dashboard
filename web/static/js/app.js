// Machines page: master-detail layout. The left sidebar lists all VMs
// (fetched from /api/v1/machines); the right pane shows the selected VM
// (fetched from /api/v1/machines/{name}). The selection is kept in the URL
// (/machines/{name}) so pages are shareable and the back button works.

function vmIcon(name) {
  const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  svg.classList.add("ic");
  const use = document.createElementNS("http://www.w3.org/2000/svg", "use");
  use.setAttribute("href", `#${name}`);
  svg.appendChild(use);
  return svg;
}

function vmEl(tag, cls, text) {
  const node = document.createElement(tag);
  if (cls) node.className = cls;
  if (text !== undefined) node.textContent = text;
  return node;
}

// showErrorDialog shows a modal error dialog: a dark panel with a red stripe
// on the left, the error text (as returned by kvmrun) and a single "Close"
// button. Clicking the button or the overlay removes the dialog.
function showErrorDialog(message) {
  const overlay = vmEl("div", "error-dialog-overlay");
  const dialog = vmEl("div", "error-dialog");
  dialog.append(vmEl("div", "error-dialog-stripe"));
  dialog.append(vmEl("h3", "error-dialog-title", "Error"));
  const msg = vmEl("pre", "error-dialog-msg");
  msg.textContent = message || "Unknown error";
  const closeBtn = vmEl("button", "error-dialog-close", "Close");
  closeBtn.addEventListener("click", () => overlay.remove());
  dialog.append(msg, closeBtn);
  overlay.append(dialog);
  overlay.addEventListener("click", (e) => {
    if (e.target === overlay) overlay.remove();
  });
  document.body.append(overlay);
}

function fmtDuration(totalSec) {
  if (!totalSec || totalSec <= 0) return "—";
  const d = Math.floor(totalSec / 86400);
  const h = Math.floor((totalSec % 86400) / 3600);
  const m = Math.floor((totalSec % 3600) / 60);
  if (d > 0) return `${d}d ${h}h ${m}m`;
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m`;
  return `${totalSec}s`;
}

function vmStateClass(state) {
  return `st-${(state || "unknown").toLowerCase()}`;
}

function initMachines() {
  const itemsEl = document.querySelector("#vm-items");
  if (!itemsEl) return;
  const detailEl = document.querySelector("#vm-detail");
  const countEl = document.querySelector("#vm-count");
  const searchEl = document.querySelector("#vm-search");

  const machines = [];
  let selected = null;
  let detailSeq = 0;

  function renderList() {
    const q = (searchEl.value || "").trim().toLowerCase();
    countEl.textContent = machines.length;
    const frag = document.createDocumentFragment();
    for (const m of machines) {
      const li = vmEl("li", "vm-item");
      li.dataset.name = m.name;
      if (m.name === selected) li.classList.add("selected");
      li.append(vmEl("span", `vm-dot ${vmStateClass(m.state)}`));
      const a = vmEl("a", "vm-name", m.name);
      a.href = `/machines/${encodeURIComponent(m.name)}`;
      li.append(a);
      if (q && !m.name.toLowerCase().includes(q)) li.style.display = "none";
      frag.appendChild(li);
    }
    itemsEl.replaceChildren(frag);
  }

  function select(name, push) {
    selected = name;
    for (const li of itemsEl.children) {
      li.classList.toggle("selected", li.dataset.name === name);
    }
    if (push) {
      history.pushState(null, "", `/machines/${encodeURIComponent(name)}`);
    }
    loadDetail(name);
  }

  function nameFromUrl() {
    const m = location.pathname.match(/^\/machines\/([^/]+)$/);
    return m ? decodeURIComponent(m[1]) : null;
  }

  function kvRow(ic, label, value, valueCls) {
    const row = vmEl("div", "kv-row");
    row.append(vmIcon(ic), vmEl("span", "kv-label", label));
    const v = vmEl("span", "kv-value", value);
    if (valueCls) v.classList.add(valueCls);
    row.append(v);
    return row;
  }

  // Disk sub-row under the "Disks" counter: mono name (full path in the
  // tooltip), muted details (driver, bootindex) and a kebab ("⋮") button.
  // The menu is a stub: its only item "clear bitmap" is disabled until the
  // daemon exposes the action.
  function diskRow(d) {
    const row = vmEl("div", "kv-row disk-row");
    const name = vmEl("span", "disk-name", d.name || d.path);
    if (d.path) name.title = d.path;
    const parts = [];
    if (d.driver) parts.push(d.driver);
    if (d.bootindex) parts.push(`boot ${d.bootindex}`);
    row.append(name, vmEl("span", "disk-details", parts.join(" · ")));
    const kebab = vmEl("button", "kebab-btn");
    kebab.setAttribute("aria-haspopup", "true");
    kebab.setAttribute("aria-expanded", "false");
    kebab.append(vmEl("span", "kebab-dots", "⋮"));
    const menu = vmEl("div", "disk-menu");
    const item = vmEl("button", "disk-menu-item", "clear bitmap");
    item.disabled = true;
    item.title = "Coming soon";
    menu.append(item);
    row.append(kebab, menu);
    return row;
  }

  function closeDropdownMenus() {
    for (const menu of detailEl.querySelectorAll(".disk-menu.open, .more-menu.open")) {
      menu.classList.remove("open");
      const trigger = menu.parentElement.querySelector(".kebab-btn, .vm-more .btn-small");
      if (trigger) trigger.setAttribute("aria-expanded", "false");
    }
  }

  function statCard(ic, label, value, sub, valueCls) {
    const card = vmEl("div", "stat-card");
    const top = vmEl("div", "stat-top");
    top.append(vmIcon(ic), vmEl("span", null, label));
    const val = vmEl("div", "stat-value", value);
    if (valueCls) val.classList.add(valueCls);
    card.append(top, val);
    if (sub) card.append(vmEl("div", "stat-sub", sub));
    return card;
  }

  async function loadDetail(name) {
    const seq = ++detailSeq;
    try {
      const [res, netRes, comRes] = await Promise.all([
        fetch(`/api/v1/machines/${encodeURIComponent(name)}`),
        fetch(`/api/v1/machines/${encodeURIComponent(name)}/networks`),
        fetch(`/api/v1/machines/${encodeURIComponent(name)}/comment`),
      ]);
      if (seq !== detailSeq || name !== selected) return; // stale response
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      // A failed networks fetch does not fail the whole page — the panel
      // shows the error instead.
      let schemes = [];
      let netErr = null;
      if (netRes.ok) {
        schemes = (await netRes.json()) || [];
      } else {
        netErr = `HTTP ${netRes.status}`;
      }
      // Same for the comment: a failed fetch only affects the comment panel.
      let comment = "";
      let comErr = null;
      if (comRes.ok) {
        comment = ((await comRes.json()) || {}).comment || "";
      } else {
        comErr = `HTTP ${comRes.status}`;
      }
      renderDetail(await res.json(), schemes, netErr, comment, comErr);
    } catch (err) {
      if (seq !== detailSeq || name !== selected) return;
      const p = vmEl("p", "error", `Failed to load ${name}: ${err.message}`);
      detailEl.replaceChildren(p);
    }
  }



  // Consoles (VNC and the agent built-in SSH): while open, the detail
  // blocks under the VM head (stat cards and panels) are replaced by the
  // console UI. The console is closed by the "Back to details" button or
  // automatically when the VM stops.
  let consoleOpen = false;
  let consoleEl = null;
  let savedChildren = [];
  let consoleMsgHandler = null;
  let statePollTimer = null;

  // Agent built-in SSH console state (xterm.js + WebSocket).
  let sshTerm = null;
  let sshFit = null;
  let sshWS = null;
  let sshRO = null;

  // consoleUrl builds the noVNC page URL: autoconnect with the WS proxy as
  // the "path" (noVNC resolves it against the page URL); the password goes
  // in the fragment — noVNC reads it from there and the fragment is never
  // sent to the server or written to logs.
  function consoleUrl(name, reqs) {
    const params = new URLSearchParams({
      autoconnect: "1",
      resize: "scale",
      path: `/api/v1/machines/${encodeURIComponent(name)}/vnc-ws?port=${reqs.port}`,
    });
    return `/novnc/vnc.html?${params}#password=${encodeURIComponent(reqs.password || "")}`;
  }

  async function openConsole(m, mode) {
    if (consoleOpen) return;
    if (mode === "ssh") {
      await openSSHConsole(m);
      return;
    }
    // VNC
    try {
      const res = await fetch(`/api/v1/machines/${encodeURIComponent(m.name)}/vnc`, {
        method: "POST",
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const reqs = await res.json();
      if (!reqs || !reqs.port) throw new Error("no VNC port in the response");
      showConsole(m, reqs);
    } catch (err) {
      console.error("failed to open VNC console:", err);
      const p = vmEl("p", "error", `Failed to open console: ${err.message}`);
      detailEl.prepend(p);
      setTimeout(() => p.remove(), 5000);
    }
  }

  function showConsole(m, reqs) {
    const head = detailEl.querySelector(".vm-head");

    const strip = vmEl("div", "vm-console-strip");
    const back = vmEl("button", "vm-console-back", "\u2190 Back to details");
    back.addEventListener("click", () => closeConsole());
    const addr = vmEl("span", "vm-console-addr mono", `127.0.0.1:${reqs.port}`);
    const status = vmEl("span", "vm-console-status", "Connecting\u2026");
    strip.append(back, addr, status);

    const frame = document.createElement("iframe");
    frame.className = "vm-console-frame";
    frame.src = consoleUrl(m.name, reqs);

    consoleEl = vmEl("div", "vm-console");
    consoleEl.append(strip, frame);

    // The embedded noVNC relays its connect/disconnect events here.
    consoleMsgHandler = (e) => {
      if (e.source !== frame.contentWindow) return;
      if (!e.data || e.data.type !== "novnc") return;
      const connected = e.data.state === "connect";
      status.textContent = connected ? "Connected" : "Disconnected";
      status.classList.toggle("vm-console-status-ok", connected);
      status.classList.toggle("vm-console-status-off", !connected);
    };
    window.addEventListener("message", consoleMsgHandler);

    // Keep the VM head, hide the Console/More buttons, and swap the rest
    // of the detail content for the console.
    const actions = head ? head.querySelector(".vm-actions") : null;
    if (actions) {
      for (const child of actions.children) {
        if (child.classList.contains("btn-icon") || child.classList.contains("vm-more")) {
          child.classList.add("vm-console-hidden");
        }
      }
    }
    savedChildren = [];
    for (const child of [...detailEl.children]) {
      if (child !== head) {
        savedChildren.push(child);
        child.remove();
      }
    }
    detailEl.append(consoleEl);
    consoleOpen = true;

    // The detail pane is not polled, so while the console is open poll the
    // VM state and close the console automatically when the VM stops.
    startStatePolling(m);
  }

  // --- Agent built-in SSH console (xterm.js over the WS proxy) ---

  // loadScriptOnce / loadCssOnce load an asset once and resolve when it is
  // ready, so reopening the console does not re-download xterm.js.
  function loadScriptOnce(src) {
    if (document.querySelector(`script[src="${src}"]`)) return Promise.resolve();
    return new Promise((resolve, reject) => {
      const s = document.createElement("script");
      s.src = src;
      s.onload = () => resolve();
      s.onerror = () => reject(new Error(`failed to load ${src}`));
      document.head.appendChild(s);
    });
  }

  function loadCssOnce(href) {
    if (document.querySelector(`link[href="${href}"]`)) return;
    const l = document.createElement("link");
    l.rel = "stylesheet";
    l.href = href;
    document.head.appendChild(l);
  }

  async function openSSHConsole(m) {
    try {
      loadCssOnce("/xterm/xterm.css");
      await loadScriptOnce("/xterm/xterm.js");
      await loadScriptOnce("/xterm/addon-fit.js");
    } catch (err) {
      console.error("failed to load xterm.js:", err);
      const p = vmEl("p", "error", `Failed to load terminal: ${err.message}`);
      detailEl.prepend(p);
      setTimeout(() => p.remove(), 5000);
      return;
    }
    showSSHConsole(m);
  }

  function showSSHConsole(m) {
    const head = detailEl.querySelector(".vm-head");

    const strip = vmEl("div", "vm-console-strip");
    const back = vmEl("button", "vm-console-back", "\u2190 Back to details");
    back.addEventListener("click", () => closeConsole());
    // Endpoint shown to the user: the SSH user (root) and the VM's vsock
    // context ID (the port is fixed by the guest agent).
    const endpoint = `root@${m.name}`;
    const addr = vmEl("span", "vm-console-addr mono",
      m.vsock_cid ? `${endpoint} \u00b7 vsock ${m.vsock_cid}:4949` : endpoint);
    const status = vmEl("span", "vm-console-status", "Connecting\u2026");
    // Single Connect/Disconnect toggle: red outline ("Disconnect") while the
    // session is up, green outline ("Connect") after it drops, disabled with
    // a CSS spinner ("Connecting…") while (re)connecting.
    const toggle = vmEl("button", "vm-console-toggle", "Connecting\u2026");
    toggle.disabled = true;

    // setSSHState updates the status text and the toggle for one of the
    // three session states: "connecting", "connected", "disconnected".
    function setSSHState(state) {
      status.textContent = state === "connected" ? "Connected"
        : state === "disconnected" ? "Disconnected"
        : "Connecting\u2026";
      status.classList.toggle("vm-console-status-ok", state === "connected");
      status.classList.toggle("vm-console-status-off", state === "disconnected");
      toggle.textContent = state === "connected" ? "Disconnect"
        : state === "disconnected" ? "Connect"
        : "Connecting\u2026";
      toggle.classList.toggle("vm-console-toggle-connect", state === "disconnected");
      toggle.classList.toggle("vm-console-toggle-disconnect", state === "connected");
      toggle.disabled = state === "connecting";
    }

    // connectSSH opens (or re-opens) the WebSocket to the SSH proxy and
    // wires its lifecycle to the status and the toggle. Binary frames carry
    // terminal I/O; the terminal sends resize updates as JSON text frames.
    function connectSSH() {
      setSSHState("connecting");
      const wsProto = location.protocol === "https:" ? "wss" : "ws";
      const ws = new WebSocket(
        `${wsProto}://${location.host}/api/v1/machines/${encodeURIComponent(m.name)}/ssh-ws`);
      sshWS = ws;
      ws.binaryType = "arraybuffer";
      ws.onopen = () => {
        if (sshWS !== ws) return; // superseded by a newer connect attempt
        setSSHState("connected");
        sendSSHResize();
      };
      ws.onmessage = (e) => {
        if (sshWS !== ws) return;
        if (e.data instanceof ArrayBuffer) sshTerm.write(new Uint8Array(e.data));
      };
      ws.onclose = () => {
        if (sshWS === ws) setSSHState("disconnected");
      };
      ws.onerror = () => {
        if (sshWS === ws) setSSHState("disconnected");
      };
    }

    toggle.addEventListener("click", () => {
      if (sshWS && sshWS.readyState <= WebSocket.OPEN) {
        // Connected: end the session. onclose flips the state back to
        // "Disconnected" and re-enables the button as "Connect".
        sshWS.close();
      } else {
        // Disconnected (or the previous socket is still closing): reconnect.
        connectSSH();
      }
    });
    strip.append(back, addr, status, toggle);

    const termHost = vmEl("div", "vm-console-terminal");
    consoleEl = vmEl("div", "vm-console vm-console-ssh");
    consoleEl.append(strip, termHost);

    // xterm.js terminal + fit addon, sized to its container.
    sshTerm = new Terminal({
      cursorBlink: true,
      fontSize: 14,
      fontFamily: "ui-monospace, 'SF Mono', 'Cascadia Mono', Menlo, Consolas, monospace",
      theme: { background: "#0d1117" },
      scrollback: 1000,
    });
    sshFit = new FitAddon.FitAddon();
    sshTerm.loadAddon(sshFit);
    sshTerm.open(termHost);

    // Terminal input -> WS (binary frame). xterm emits a string; the proxy
    // expects bytes, so encode UTF-8.
    sshTerm.onData((data) => {
      if (sshWS && sshWS.readyState === WebSocket.OPEN) {
        sshWS.send(new TextEncoder().encode(data));
      }
    });

    // Terminal resize -> WS. fit() (driven by a ResizeObserver on the
    // container) triggers onResize, which sends the new dimensions.
    sshTerm.onResize(() => sendSSHResize());
    sshRO = new ResizeObserver(() => sshFit.fit());
    sshRO.observe(termHost);
    sshFit.fit();

    // Hide all the head action buttons (power, Console, More) while the SSH
    // console is open.
    const actions = head ? head.querySelector(".vm-actions") : null;
    if (actions) {
      for (const child of actions.children) {
        child.classList.add("vm-console-hidden");
      }
    }
    savedChildren = [];
    for (const child of [...detailEl.children]) {
      if (child !== head) {
        savedChildren.push(child);
        child.remove();
      }
    }
    detailEl.append(consoleEl);
    consoleOpen = true;

    connectSSH();
    startStatePolling(m);
  }

  function sendSSHResize() {
    if (sshWS && sshWS.readyState === WebSocket.OPEN && sshTerm) {
      sshWS.send(JSON.stringify({ type: "resize", cols: sshTerm.cols, rows: sshTerm.rows }));
    }
  }

  // While a console is open the detail pane is not re-rendered, so poll the
  // VM state and close the console automatically when the VM stops.
  function startStatePolling(m) {
    stopStatePolling();
    statePollTimer = setInterval(async () => {
      if (!consoleOpen) return stopStatePolling();
      try {
        const res = await fetch(`/api/v1/machines/${encodeURIComponent(m.name)}`);
        if (!res.ok) return;
        const d = await res.json();
        if ((d.state || "").toLowerCase() !== "running") {
          closeConsole();
          loadDetail(m.name);
        }
      } catch (err) {
        // Transient fetch error — keep the console open.
      }
    }, 5000);
  }

  function closeConsole() {
    if (!consoleOpen) return;
    consoleOpen = false;
    stopStatePolling();
    if (consoleMsgHandler) {
      window.removeEventListener("message", consoleMsgHandler);
      consoleMsgHandler = null;
    }
    // SSH console teardown.
    if (sshRO) { sshRO.disconnect(); sshRO = null; }
    if (sshWS) {
      try { sshWS.close(); } catch (err) { /* already closed */ }
      sshWS = null;
    }
    if (sshTerm) {
      try { sshTerm.dispose(); } catch (err) { /* already disposed */ }
      sshTerm = null;
      sshFit = null;
    }
    if (consoleEl) {
      consoleEl.remove();
      consoleEl = null;
    }
    for (const child of savedChildren) detailEl.append(child);
    savedChildren = [];
    const actions = detailEl.querySelector(".vm-head .vm-actions");
    if (actions) {
      for (const child of actions.children) child.classList.remove("vm-console-hidden");
    }
  }

  function stopStatePolling() {
    if (statePollTimer) {
      clearInterval(statePollTimer);
      statePollTimer = null;
    }
  }

  // Network scheme type icon (colors are set by .net-type-* CSS classes).
  const NET_TYPE_ICONS = {
    bridge: "ic-bridge",
    routed: "ic-routed",
    vxlan: "ic-vxlan",
    vlan: "ic-vlan",
  };

  function renderScheme(s) {
    const type = (s.type || "unknown").toLowerCase();
    const block = vmEl("div", "net-scheme");
    const head = vmEl("div", "net-scheme-head");
    head.append(vmEl("span", "net-scheme-iface", s.ifname || "?"));
    const typeEl = vmEl("span", `net-scheme-type net-type-${type}`);
    const iconId = NET_TYPE_ICONS[type];
    if (iconId) typeEl.append(vmIcon(iconId));
    typeEl.append(vmEl("span", null, type.toUpperCase()));
    head.append(typeEl);
    block.append(head);

    // Rows mirror the "vmm nets" console output: MTU, addresses,
    // gateways, then scheme-specific parameters.
    const rows = vmEl("div", "kv-list");
    rows.append(kvRow("ic-net", "MTU", String(s.mtu ?? 0)));
    for (const addr of s.addrs || []) {
      rows.append(kvRow("ic-net", "Address", addr, "wrap"));
    }
    if (s.gateway4) rows.append(kvRow("ic-net", "IPv4 gateway", s.gateway4));
    if (s.gateway6) rows.append(kvRow("ic-net", "IPv6 gateway", s.gateway6));
    switch (s.type) {
      case "routed":
        if (s.bind_interface) rows.append(kvRow("ic-box", "In/Out device", s.bind_interface));
        rows.append(kvRow("ic-net", "Incoming limit", `${s.in_limit ?? 0} mbit/s`));
        rows.append(kvRow("ic-net", "Outgoing limit", `${s.out_limit ?? 0} mbit/s`));
        break;
      case "vxlan":
        if (s.bind_interface) rows.append(kvRow("ic-box", "Tunnel device", s.bind_interface));
        rows.append(kvRow("ic-net", "VNI", String(s.vni ?? 0)));
        break;
      case "vlan":
        if (s.parent_interface) rows.append(kvRow("ic-box", "Parent device", s.parent_interface));
        rows.append(kvRow("ic-net", "VLAN ID", String(s.vlan_id ?? 0)));
        break;
      case "bridge":
        if (s.bridge_name) rows.append(kvRow("ic-box", "Bridge device", s.bridge_name));
        break;
    }
    block.append(rows);
    return block;
  }

  function renderDetail(m, schemes, netErr, comment, comErr) {
    // A fresh render rebuilds the head and blocks from scratch, so an open
    // console (VNC or SSH, if any) is torn down first.
    closeConsole();
    const sCls = vmStateClass(m.state);

    // Header: name + colored state pill + power actions in one row
    // (running VM: Stop/Restart/Reset, otherwise: Start) + runtime meta.
    const head = vmEl("div", "vm-head");
    const titleRow = vmEl("div", "vm-title-row");
    titleRow.append(vmEl("h1", "vm-title", m.name));
    const pill = vmEl("span", `state-pill ${sCls}`);
    pill.append(vmEl("span", "pill-dot"), vmEl("span", null, m.state || "UNKNOWN"));
    titleRow.append(pill);

    const running = (m.state || "").toLowerCase() === "running";
    const actions = vmEl("div", "vm-actions");
    for (const act of running ? ["stop", "restart", "reset"] : ["start"]) {
      const form = vmEl("form", "inline");
      form.method = "post";
      form.action = `/machines/${encodeURIComponent(m.name)}/${act}`;
      form.append(vmEl("button", "btn-small", act[0].toUpperCase() + act.slice(1)));
      actions.append(form);
    }
    // Console: a dropdown to pick the transport — the embedded noVNC client
    // (VNC server over the dashboard's WS proxy) or the guest agent's
    // built-in SSH (xterm.js over the WS proxy, AF_VSOCK). Available only
    // for running VMs.
    const consoleWrap = vmEl("div", "vm-more vm-console-btn");
    const consoleBtn = vmEl("button", "btn-small");
    consoleBtn.setAttribute("aria-haspopup", "true");
    consoleBtn.setAttribute("aria-expanded", "false");
    if (!running) {
      consoleBtn.disabled = true;
      consoleBtn.title = "Console is available only for running VMs";
    }
    consoleBtn.append(vmIcon("ic-terminal"), vmEl("span", "more-label", "Console"), vmEl("span", "more-caret"));
    const consoleMenu = vmEl("div", "more-menu");
    const vncItem = vmEl("button", "more-menu-item has-icon");
    vncItem.append(vmIcon("ic-monitor"), vmEl("span", null, "VNC"));
    vncItem.addEventListener("click", () => openConsole(m, "vnc"));
    const sshItem = vmEl("button", "more-menu-item has-icon");
    sshItem.append(vmIcon("ic-terminal"), vmEl("span", null, "Agent Built-in SSH"));
    if (!m.vsock_cid) {
      sshItem.disabled = true;
      sshItem.title = "The VM has no vsock device — agent built-in SSH is not available";
    } else {
      sshItem.addEventListener("click", () => openConsole(m, "ssh"));
    }
    consoleMenu.append(vncItem, sshItem);
    consoleWrap.append(consoleBtn, consoleMenu);
    actions.append(consoleWrap);
    // More: slightly separated overflow button (Migrate / Re-build CI-drive).
    // Items are stubs until the daemon exposes the actions.
    const moreWrap = vmEl("div", "vm-more");
    const moreBtn = vmEl("button", "btn-small");
    moreBtn.setAttribute("aria-haspopup", "true");
    moreBtn.setAttribute("aria-expanded", "false");
    moreBtn.append(vmEl("span", "more-label", "More"), vmEl("span", "more-caret"));
    const moreMenu = vmEl("div", "more-menu");
    for (const label of ["Migrate", "Re-build CI-drive"]) {
      const item = vmEl("button", "more-menu-item", label);
      item.disabled = true;
      item.title = "Coming soon";
      moreMenu.append(item);
    }
    moreWrap.append(moreBtn, moreMenu);
    actions.append(moreWrap);
    titleRow.append(actions);

    const metaParts = [];
    if (m.pid) metaParts.push(`PID ${m.pid}`);
    if (m.lifetime > 0) metaParts.push(`up ${fmtDuration(m.lifetime)}`);
    head.append(titleRow, vmEl("p", "vm-meta", metaParts.length ? metaParts.join("  ·  ") : "no runtime data"));

    // Stat cards. CPU time is a placeholder: the daemon does not report it yet.
    const cards = vmEl("div", "stat-cards");
    cards.append(
      statCard("ic-power", "STATE", m.state || "—",
        m.lifetime > 0 ? `up ${fmtDuration(m.lifetime)}` : "", sCls),
      statCard("ic-mem", "MEMORY (in MiB)", `${m.mem_actual} / ${m.mem_total}`, "actual / total"),
      statCard("ic-cpu", "VCPU", `${m.cpus_actual} / ${m.cpus_total}`, "actual / total"),
      statCard("ic-clock", "TOTAL ELAPSED HOST CPU TIME", "—", "lifetime · not reported yet"),
    );

    // Configuration panel with icons next to each parameter.
    const cfg = vmEl("section", "panel-card");
    const cfgTitle = vmEl("h2", "panel-title");
    cfgTitle.append(vmIcon("ic-box"), vmEl("span", null, "Configuration"));
    const cfgBody = vmEl("div", "kv-list");
    cfgBody.append(
      kvRow("ic-power", "State", m.state || "—", sCls),
      kvRow("ic-mem", "Memory", `${m.mem_actual} / ${m.mem_total} MiB`),
      kvRow("ic-cpu", "vCPU", `${m.cpus_actual} / ${m.cpus_total}${m.cpu_quota ? ` · ${m.cpu_quota}% quota` : ""}`),
      kvRow("ic-cpu", "vCPU model", m.cpu_model || "—"),
      kvRow("ic-box", "Machine type", m.machine_type || "—"),
      kvRow("ic-box", "Firmware image", m.firmware_image || "—"),
      kvRow("ic-box", "Firmware flash", m.firmware_flash || "—"),
      kvRow("ic-monitor", "VGA", m.vga_type || "—"),
      kvRow("ic-disk", "Disks", String(m.disks ?? 0)),
    );
    for (const d of m.disk_list || []) cfgBody.append(diskRow(d));
    cfg.append(cfgTitle, cfgBody);

    // Network panel: one block per interface scheme from the network
    // service GetConf (the same data the "vmm nets" console prints).
    const net = vmEl("section", "panel-card");
    const netTitle = vmEl("h2", "panel-title");
    netTitle.append(vmIcon("ic-net"), vmEl("span", null, "Network"));
    const netBody = vmEl("div", "net-schemes");
    if (netErr) {
      netBody.append(vmEl("p", "hint", `Failed to load networks: ${netErr}`));
    } else if (!schemes || schemes.length === 0) {
      netBody.append(vmEl("p", "hint", "No networks configured"));
    } else {
      for (const s of schemes) netBody.append(renderScheme(s));
    }
    net.append(netTitle, netBody);

    // Comments block: read-only text with a pencil button in the title row.
    // Clicking the pencil swaps the text for a textarea and reveals the
    // "Update" button in the bottom-right corner. Errors from kvmrun are
    // shown in a modal dialog with a single "Close" button.
    const com = vmEl("section", "panel-card");
    const comTitle = vmEl("h2", "panel-title");
    comTitle.append(vmIcon("ic-comment"), vmEl("span", null, "Comments"));
    const editBtn = vmEl("button", "comment-edit-btn");
    editBtn.setAttribute("aria-label", "Edit comment");
    editBtn.title = "Edit comment";
    editBtn.append(vmIcon("ic-edit"));
    comTitle.append(editBtn);

    const box = vmEl("div", "comments-box");
    const textEl = vmEl("pre", "comment-text");
    const textarea = vmEl("textarea", "comment-editor");
    const footer = vmEl("div", "comment-actions");
    const updateBtn = vmEl("button", "comment-update-btn", "Update");

    let currentText = comment || "";
    function renderCommentText() {
      textEl.replaceChildren(currentText
        ? document.createTextNode(currentText)
        : vmEl("span", "hint", "No comments yet"));
      textEl.classList.toggle("comment-empty", !currentText);
    }
    renderCommentText();
    if (comErr) {
      box.append(vmEl("p", "hint", `Failed to load comment: ${comErr}`));
    }
    textarea.value = currentText;
    textarea.style.display = "none";
    updateBtn.style.display = "none";
    footer.append(updateBtn);
    box.append(textEl, textarea, footer);

    editBtn.addEventListener("click", () => {
      textarea.value = currentText;
      textarea.style.display = "";
      textEl.style.display = "none";
      updateBtn.style.display = "";
      textarea.focus();
    });

    updateBtn.addEventListener("click", async () => {
      updateBtn.disabled = true;
      try {
        const res = await fetch(`/api/v1/machines/${encodeURIComponent(m.name)}/comment`, {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ comment: textarea.value }),
        });
        let body = null;
        try { body = await res.json(); } catch (_) { /* ignore */ }
        if (!res.ok) {
          showErrorDialog((body && body.error) || `HTTP ${res.status}`);
          return;
        }
        currentText = textarea.value;
        renderCommentText();
        textarea.style.display = "none";
        textEl.style.display = "";
        updateBtn.style.display = "none";
      } catch (err) {
        showErrorDialog(err.message);
      } finally {
        updateBtn.disabled = false;
      }
    });

    com.append(comTitle, box);

    // Two columns: Configuration + Comments stacked on the left (same
    // width), Network on the right — it can grow in height freely.
    const panels = vmEl("div", "vm-panels");
    const col = vmEl("div", "vm-panels-col");
    col.append(cfg, com);
    panels.append(col, net);

    detailEl.replaceChildren(head, cards, panels);
  }

  async function loadList() {
    try {
      const res = await fetch("/api/v1/machines");
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      machines.length = 0;
      machines.push(...(await res.json()));
      renderList();
      return true;
    } catch (err) {
      countEl.textContent = "";
      itemsEl.replaceChildren(vmEl("li", "vm-item error", "Failed to load machines"));
      console.error("failed to load machines:", err);
      return false;
    }
  }

  searchEl.addEventListener("input", renderList);
  itemsEl.addEventListener("click", (e) => {
    const a = e.target.closest("a.vm-name");
    if (!a) return;
    e.preventDefault();
    const name = a.closest("li").dataset.name;
    if (name && name !== selected) select(name, true);
  });
  window.addEventListener("popstate", () => {
    const name = nameFromUrl();
    if (name && name !== selected) select(name, false);
  });
  // Dropdown menus (disk kebab, More): click a trigger to toggle its menu;
  // click anywhere else or press Escape to close. Only one menu is open at a
  // time.
  document.addEventListener("click", (e) => {
    const kebab = e.target.closest(".kebab-btn");
    const moreBtn = e.target.closest(".vm-more .btn-small");
    const trigger = kebab || moreBtn;
    if (!trigger) {
      closeDropdownMenus();
      return;
    }
    const menu = trigger.nextElementSibling;
    const willOpen = !menu.classList.contains("open");
    closeDropdownMenus();
    if (willOpen) menu.classList.add("open");
    trigger.setAttribute("aria-expanded", String(willOpen));
  });
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape") closeDropdownMenus();
  });

  // Initial load: preselected VM from the URL (or the server-rendered
  // data-selected attribute), otherwise the first machine in the list.
  loadList().then((ok) => {
    if (!ok) return;
    const pre = detailEl.dataset.selected || nameFromUrl();
    const target = pre && machines.some((m) => m.name === pre) ? pre : machines[0] && machines[0].name;
    if (target) select(target, false);
  });
}


// Tasks page: an auto-refreshing list of tasks reported by the daemon.
// The refresh interval and the number of rows per page are user-tunable
// (persisted in localStorage); the "updated Ns ago" label tracks the last
// successful fetch.
const TASKS_REFRESH_KEY = "tasks-refresh-interval-s";
const TASKS_PAGE_SIZE_KEY = "tasks-page-size";

function initTasks() {
  const table = document.querySelector("#tasks");
  if (!table) return;
  const tbody = table.querySelector("tbody");
  const refreshSelect = document.querySelector("#tasks-refresh");
  const pageSizeSelect = document.querySelector("#tasks-page-size");
  const agoLabel = document.querySelector("#tasks-updated-ago");

  restoreSelect(refreshSelect, TASKS_REFRESH_KEY);
  restoreSelect(pageSizeSelect, TASKS_PAGE_SIZE_KEY);

  let lastTasks = [];
  let lastUpdated = 0;
  let pollTimer = null;

  function renderRow(task) {
    const tr = document.createElement("tr");
    const state = task.state || "UNKNOWN";
    const cells = [
      [task.task_id || "", "mono"],
      [state, `state-${state.toLowerCase()}`],
      [task.state_desc || "", "hint"],
      [`${task.progress ?? 0}%`, "mono"],
    ];
    for (const [value, cls] of cells) {
      const td = document.createElement("td");
      td.textContent = value;
      if (cls) td.className = cls;
      tr.appendChild(td);
    }
    return tr;
  }

  function render() {
    const pageSize = Number(pageSizeSelect.value);
    const rows = lastTasks.slice(0, pageSize).map(renderRow);
    if (rows.length === 0) {
      const tr = document.createElement("tr");
      const td = document.createElement("td");
      td.colSpan = 4;
      td.textContent = "No tasks.";
      td.className = "hint";
      tr.appendChild(td);
      rows.push(tr);
    }
    tbody.replaceChildren(...rows);
  }

  function updateAgo() {
    if (!agoLabel) return;
    if (!lastUpdated) {
      agoLabel.textContent = "not updated yet";
      return;
    }
    const secs = Math.floor((Date.now() - lastUpdated) / 1000);
    agoLabel.textContent = secs < 2 ? "updated just now" : `updated ${secs}s ago`;
  }

  async function poll() {
    try {
      const res = await fetch("/api/v1/tasks");
      if (!res.ok) return;
      lastTasks = await res.json();
      lastUpdated = Date.now();
      render();
      updateAgo();
    } catch (err) {
      console.error("failed to load tasks:", err);
    }
  }

  function schedulePolling() {
    if (pollTimer) clearInterval(pollTimer);
    pollTimer = setInterval(poll, Number(refreshSelect.value) * 1000);
  }

  refreshSelect.addEventListener("change", () => {
    localStorage.setItem(TASKS_REFRESH_KEY, refreshSelect.value);
    schedulePolling();
  });
  pageSizeSelect.addEventListener("change", () => {
    localStorage.setItem(TASKS_PAGE_SIZE_KEY, pageSizeSelect.value);
    render();
  });

  poll();
  schedulePolling();
  setInterval(updateAgo, 1000);
}

// Re-apply a stored select value if it is still one of the offered options.
function restoreSelect(select, key) {
  const stored = localStorage.getItem(key);
  if (stored && [...select.options].some((option) => option.value === stored)) {
    select.value = stored;
  }
}

document.addEventListener("DOMContentLoaded", () => {
  initMachines();
  initTasks();
});

