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

document.addEventListener("DOMContentLoaded", () => {
  refreshMachines();
});
