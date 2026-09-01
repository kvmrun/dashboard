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

  function closeDiskMenus() {
    for (const menu of detailEl.querySelectorAll(".disk-menu.open")) {
      menu.classList.remove("open");
      const kebab = menu.parentElement.querySelector(".kebab-btn");
      if (kebab) kebab.setAttribute("aria-expanded", "false");
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
      const [res, netRes] = await Promise.all([
        fetch(`/api/v1/machines/${encodeURIComponent(name)}`),
        fetch(`/api/v1/machines/${encodeURIComponent(name)}/networks`),
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
      renderDetail(await res.json(), schemes, netErr);
    } catch (err) {
      if (seq !== detailSeq || name !== selected) return;
      const p = vmEl("p", "error", `Failed to load ${name}: ${err.message}`);
      detailEl.replaceChildren(p);
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

  function renderDetail(m, schemes, netErr) {
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
    // Console: the daemon has no console endpoint yet, so the button is a
    // placeholder until one is wired up.
    const consoleBtn = vmEl("button", "btn-small btn-icon");
    consoleBtn.disabled = true;
    consoleBtn.title = "Console is not available yet";
    consoleBtn.append(vmIcon("ic-terminal"), vmEl("span", null, "Console"));
    actions.append(consoleBtn);
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
      statCard("ic-mem", "MEMORY", `MiB ${m.mem_actual} / ${m.mem_total}`, "actual/total"),
      statCard("ic-cpu", "VCPU", `${m.cpus_actual} / ${m.cpus_total}`, "actual/total"),
      statCard("ic-clock", "CPU TIME", "—", "lifetime · not reported yet"),
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

    // Comments — placeholder, kvmrun has no comment API yet.
    const com = vmEl("section", "panel-card");
    const comTitle = vmEl("h2", "panel-title");
    comTitle.append(vmIcon("ic-comment"), vmEl("span", null, "Comments"));
    const box = vmEl("div", "comments-box");
    box.append(vmIcon("ic-comment"), vmEl("span", "hint", "No comments yet"));
    box.append(vmEl("p", "hint comment-note", "kvmrun has no comment API yet — placeholder"));
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
  // Kebab menu: click the "⋮" button to toggle its menu; click anywhere else
  // or press Escape to close. Only one menu is open at a time.
  document.addEventListener("click", (e) => {
    const kebab = e.target.closest(".kebab-btn");
    if (!kebab) {
      closeDiskMenus();
      return;
    }
    const menu = kebab.nextElementSibling;
    const willOpen = !menu.classList.contains("open");
    closeDiskMenus();
    if (willOpen) menu.classList.add("open");
    kebab.setAttribute("aria-expanded", String(willOpen));
  });
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape") closeDiskMenus();
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

