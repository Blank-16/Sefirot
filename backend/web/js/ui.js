function showPrompt({
  title,
  description,
  value = "",
  placeholder = "",
  confirm = "Continue",
} = {}) {
  $("#promptTitle").textContent = title || "Enter a value";
  $("#promptDescription").textContent = description || "";
  const input = $("#promptInput");
  input.value = value;
  input.placeholder = placeholder;
  $("#promptConfirm").textContent = confirm;
  $("#promptOverlay").classList.remove("hidden");
  $("#promptOverlay").classList.add("flex");
  input.focus();
  input.select();
  return new Promise((resolve) => {
    state.promptResolve = resolve;
  });
}
function closePrompt(value) {
  $("#promptOverlay").classList.add("hidden");
  $("#promptOverlay").classList.remove("flex");
  if (state.promptResolve) {
    state.promptResolve(value);
    state.promptResolve = null;
  }
}
function showError(error) {
  console.error(error);
  $("#globalStatus").textContent = error.message || String(error);
  $("#globalStatus").classList.remove("hidden");
  setTimeout(() => $("#globalStatus").classList.add("hidden"), 4500);
}

async function createNote(initialPath = "") {
  let suggested = initialPath || "Untitled.html";
  const entered = await showPrompt({
    title: "Create note",
    description:
      "Enter a note name. You can include folders, for example: Projects/Ideas.html",
    value: suggested,
    confirm: "Create",
  });
  if (!entered) return;
  let notePath = entered.trim().replace(/\\/g, "/");
  if (!notePath.toLowerCase().endsWith(".html")) notePath += ".html";
  if (
    notePath.startsWith("/") ||
    notePath.split("/").some((p) => !p || p === "." || p === "..")
  ) {
    showError(new Error("Use a relative path inside the vault."));
    return;
  }
  if (findPath(state.tree, notePath)) {
    showError(new Error("A note already exists at that path."));
    return;
  }
  try {
    await api("/api/notes", {
      method: "POST",
      body: JSON.stringify({
        path: notePath,
        content: "",
        isMarkdown: true,
        createOnly: true,
      }),
    });
    await refreshTree();
    await openNote(notePath);
    $("#editor").focus();
  } catch (e) {
    showError(e);
  }
}
async function createFolder() {
  const name = await showPrompt({
    title: "Create folder",
    description: "Create a folder inside the active vault.",
    placeholder: "Folder name",
    confirm: "Create",
  });
  if (!name) return;
  try {
    await api("/api/folders", {
      method: "POST",
      body: JSON.stringify({ path: name.trim() }),
    });
    await refreshTree();
  } catch (e) {
    showError(e);
  }
}
async function renameNote(oldPath) {
  const next = await showPrompt({
    title: "Rename note",
    description: "Enter the new path or filename.",
    value: oldPath,
    confirm: "Rename",
  });
  if (!next || next.trim() === oldPath) return;
  if (state.currentPath === oldPath && state.dirty)
    await saveNote().catch(showError);
  let newPath = next.trim().replace(/\\/g, "/");
  if (!newPath.toLowerCase().endsWith(".html")) newPath += ".html";
  try {
    await api("/api/notes/rename", {
      method: "POST",
      body: JSON.stringify({ oldPath, newPath }),
    });
    migrateResearchModePath(oldPath, newPath);
    if (state.currentPath === oldPath) state.currentPath = newPath;
    state.tabs = state.tabs.map((p) => (p === oldPath ? newPath : p));
    persistTabs();
    renderTabs();
    await refreshTree();
    if (state.currentPath === newPath) await openNote(newPath);
  } catch (e) {
    showError(e);
  }
}
async function deleteNote(notePath) {
  if (
    !confirm(
      `Move ${notePath} to the trash? You can restore it from Tools > Trash for 30 days.`,
    )
  )
    return;
  if (state.currentPath === notePath && state.dirty)
    await saveNote().catch(showError);
  try {
    await api(`/api/notes?path=${encodeURIComponent(notePath)}`, {
      method: "DELETE",
    });
    forgetResearchModePath(notePath);
    state.tabs = state.tabs.filter((p) => p !== notePath);
    persistTabs();
    renderTabs();
    if (state.currentPath === notePath) {
      state.currentPath = "";
      state.dirty = false;
      $("#workspace").classList.add("hidden");
      await openHome();
    }
    await refreshTree();
  } catch (e) {
    showError(e);
  }
}

function openSearch() {
  $("#searchOverlay").classList.remove("hidden");
  $("#searchOverlay").classList.add("flex");
  $("#searchInput").focus();
}
function closeSearch() {
  $("#searchOverlay").classList.add("hidden");
  $("#searchOverlay").classList.remove("flex");
}
