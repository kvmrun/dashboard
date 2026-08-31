// Minimal frontend logic. Most of the dashboard is server-rendered;
// this file adds interactivity on top of the JSON API (/api/v1/...).

async function refreshMachines() {
  const table = document.querySelector("#machines");
  if (!table) return;
  try {
    const res = await fetch("/api/v1/machines");
    if (!res.ok) return;
    const machines = await res.json();
    // TODO: render rows from the JSON response.
    console.log("machines:", machines);
  } catch (err) {
    console.error("failed to load machines:", err);
  }
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
  refreshMachines();
  initTasks();
});

