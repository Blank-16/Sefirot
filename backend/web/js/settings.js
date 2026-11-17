"use strict";

let settingsReady = false;
let settingsTimer = null;

function collectSettings() {
  let macros = {};
  try {
    macros = JSON.parse(localStorage.getItem("sefirot-math-macros") || "{}");
  } catch {
    macros = {};
  }
  return {
    theme:
      document.documentElement.dataset.theme === "light" ? "light" : "dark",
    tabs: state.tabs.slice(0, 50),
    researchDocs: Object.keys(loadResearchModeMap()),
    macros,
    doiLookup:
      typeof settingsDoiLookup === "boolean" ? settingsDoiLookup : false,
    editorNoWrap: !editorWrapEnabled(),
  };
}

function queueSettingsSync() {
  if (!settingsReady) return;
  clearTimeout(settingsTimer);
  settingsTimer = setTimeout(pushSettings, 600);
}

async function pushSettings() {
  try {
    await api("/api/settings", {
      method: "PUT",
      body: JSON.stringify(collectSettings()),
    });
  } catch (error) {
    console.warn("Settings were not saved to the server.", error);
  }
}

async function pullSettings() {
  const remote = await api("/api/settings");
  const serverEmpty =
    !remote.tabs.length &&
    !remote.researchDocs.length &&
    !Object.keys(remote.macros).length;
  const local = collectSettings();
  let storedWrap = null;
  try {
    storedWrap = localStorage.getItem(EDITOR_WRAP_KEY);
  } catch {}
  const localHasData =
    local.tabs.length ||
    local.researchDocs.length ||
    Object.keys(local.macros).length ||
    storedWrap !== null;
  if (serverEmpty && localHasData) {
    settingsReady = true;
    await pushSettings();
    return;
  }
  if (typeof settingsDoiLookup === "boolean") {
    settingsDoiLookup = !!remote.doiLookup;
    const box = document.querySelector("#refAllowLookup");
    if (box) box.checked = settingsDoiLookup;
  }
  try {
    localStorage.setItem(EDITOR_WRAP_KEY, remote.editorNoWrap ? "off" : "on");
  } catch {}
  applyEditorWrap(!remote.editorNoWrap);
  const modes = {};
  remote.researchDocs.forEach((p) => {
    modes[p] = true;
  });
  try {
    localStorage.setItem(RESEARCH_MODE_STORAGE_KEY, JSON.stringify(modes));
    localStorage.setItem("sefirot-math-macros", JSON.stringify(remote.macros));
    localStorage.setItem("sefirot-tabs", JSON.stringify(remote.tabs));
  } catch {}
  state.tabs = remote.tabs;
  setTheme(remote.theme);
  renderTabs();
  if (state.currentPath) {
    applyResearchMode(isResearchMode(state.currentPath));
    if (state.page === "notes" && !state.dirty) renderPreview();
  }
  settingsReady = true;
}

pullSettings().catch((error) =>
  console.warn("Server settings unavailable; using browser storage.", error),
);
