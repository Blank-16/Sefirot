function safeSnippet(snippet) {
  return escapeHTML(snippet)
    .replace(/&lt;mark&gt;/g, "<mark>")
    .replace(/&lt;\/mark&gt;/g, "</mark>");
}

async function activateTreeRow(row) {
  if (!row) return;
  const p = row.dataset.path;
  if (row.dataset.type === "folder") {
    state.expanded.has(p) ? state.expanded.delete(p) : state.expanded.add(p);
    renderTree();
  } else {
    try {
      await openNote(p);
      closeMobileVaultIndex();
    } catch (e) {
      showError(e);
    }
  }
}
$("#fileTree").addEventListener("click", async (event) => {
  const action = event.target.closest("[data-action]");
  if (action) {
    event.stopPropagation();
    if (action.dataset.action === "rename")
      await renameNote(action.dataset.path);
    if (action.dataset.action === "delete")
      await deleteNote(action.dataset.path);
    return;
  }
  await activateTreeRow(event.target.closest(".tree-row"));
});
$("#fileTree").addEventListener("keydown", async (event) => {
  const row = event.target.closest(".tree-row");
  if (!row || event.target !== row || !["Enter", " "].includes(event.key))
    return;
  event.preventDefault();
  await activateTreeRow(row);
});
document.body.addEventListener("click", async (event) => {
  const open = event.target.closest("[data-open-path]");
  if (open) {
    closeSearch();
    try {
      await openNote(open.dataset.openPath);
    } catch (e) {
      showError(e);
    }
  }
});
$("#editor").addEventListener("input", markDirty);
$("#editor").addEventListener("keydown", (event) => {
  if (event.key === "Tab") {
    event.preventDefault();
    const el = event.currentTarget,
      start = el.selectionStart,
      end = el.selectionEnd;
    el.setRangeText("  ", start, end, "end");
    markDirty();
  }
});
$("#authorMode").addEventListener("change", async (event) => {
  if (state.dirty) await saveNote().catch(showError);
  state.isMarkdown = event.target.value === "markdown";
  $("#editorModeLabel").textContent = state.isMarkdown
    ? "Markdown source"
    : "HTML source";
  $("#propFormat").textContent = state.isMarkdown
    ? "Markdown → HTML"
    : "Raw HTML";
  state.dirty = true;
  await saveNote().catch(showError);
});
document.querySelectorAll(".view-btn").forEach((btn) =>
  btn.addEventListener("click", () => {
    state.view = btn.dataset.view;
    applyView();
  }),
);
$("#newNote").addEventListener("click", () => createNote());
$("#emptyNewNote").addEventListener("click", () => createNote());
$("#newFolder").addEventListener("click", createFolder);
$("#refreshTree").addEventListener("click", () =>
  refreshTree().catch(showError),
);
$("#topSearch").addEventListener("click", openSearch);
$("#searchInput").addEventListener("input", (event) => {
  clearTimeout(state.searchTimer);
  state.searchTimer = setTimeout(() => runSearch(event.target.value), 150);
});
$("#searchMode").addEventListener("change", () =>
  runSearch($("#searchInput").value).catch(showError),
);
$("#searchOverlay").addEventListener("click", (event) => {
  if (event.target === $("#searchOverlay")) closeSearch();
});
$("#promptCancel").addEventListener("click", () => closePrompt(null));
$("#promptConfirm").addEventListener("click", () =>
  closePrompt($("#promptInput").value),
);
$("#promptInput").addEventListener("keydown", (event) => {
  if (event.key === "Enter") closePrompt($("#promptInput").value);
  if (event.key === "Escape") closePrompt(null);
});
$("#toggleRight").addEventListener("click", () => {
  state.rightOpen = !state.rightOpen;
  $("#rightSidebar").classList.toggle("hidden-pane", !state.rightOpen);
});
function closeMobileVaultIndex() {
  $("#leftSidebar").classList.remove("mobile-open");
  $("#mobileMenu").setAttribute("aria-expanded", "false");
}
$("#mobileMenu").addEventListener("click", () => {
  const open = $("#leftSidebar").classList.toggle("mobile-open");
  $("#mobileMenu").setAttribute("aria-expanded", String(open));
});
document.addEventListener("click", (event) => {
  if (
    window.matchMedia("(max-width: 680px)").matches &&
    $("#leftSidebar").classList.contains("mobile-open") &&
    !$("#leftSidebar").contains(event.target) &&
    !$("#mobileMenu").contains(event.target)
  )
    closeMobileVaultIndex();
});
$("#preview").addEventListener("load", () => {});
window.addEventListener("message", async (event) => {
  if (
    event.source === $("#preview").contentWindow &&
    event.data &&
    event.data.type === "vault-scroll" &&
    Number.isFinite(event.data.y)
  ) {
    state.previewScroll = { path: state.currentPath, y: event.data.y };
    return;
  }
  if (
    event.source !== $("#preview").contentWindow ||
    !event.data ||
    event.data.type !== "vault-wikilink"
  )
    return;
  const target = String(event.data.target || "").trim();
  if (!target) return;
  try {
    const resolved = await api(
      "/api/resolve?target=" + encodeURIComponent(target),
    );
    let destination = resolved.path;
    if (!destination && resolved.candidates.length) {
      const choice = await showPrompt({
        title: `“${target}” matches several notes`,
        description: resolved.candidates.join("\n"),
        value: resolved.candidates[0],
        confirm: "Open",
      });
      if (!choice || !resolved.candidates.includes(choice.trim())) return;
      destination = choice.trim();
    }
    if (destination) {
      await openNote(destination);
      return;
    }
    if (!confirm(`“${target}” does not exist. Create it?`)) return;
    const clean = target
      .replace(/\\/g, "/")
      .replace(/^\/+/, "")
      .replace(/\.html$/i, "");
    const folder = state.currentPath.includes("/")
      ? state.currentPath.slice(0, state.currentPath.lastIndexOf("/") + 1)
      : "";
    const created = `${folder}${clean}.html`;
    await api("/api/notes", {
      method: "POST",
      body: JSON.stringify({
        path: created,
        content: "",
        isMarkdown: true,
        createOnly: true,
      }),
    });
    await refreshTree();
    await openNote(created);
  } catch (e) {
    showError(e);
  }
});
window.addEventListener("keydown", (event) => {
  if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
    event.preventDefault();
    openSearch();
  }
  if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "p") {
    event.preventDefault();
    openCommandPalette();
  }
  if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "o") {
    event.preventDefault();
    openSearch();
  }
  if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "s") {
    event.preventDefault();
    saveNote().catch(showError);
  }
  if (event.key === "Escape") {
    closeSearch();
    closeCommandPalette();
    if (!$("#promptOverlay").classList.contains("hidden")) closePrompt(null);
  }
});
window.addEventListener("beforeunload", (event) => {
  if (state.dirty) {
    event.preventDefault();
    event.returnValue = "";
  }
});

api("/api/semantic/status")
  .then((s) => {
    $("#semanticStatus").textContent = s.enabled
      ? `Local embeddings · ${s.model} · ${s.indexed} indexed · ${s.pending} pending`
      : `Semantic search unavailable · ${s.model} · start Ollama and pull the model to enable`;
  })
  .catch(() => {
    $("#semanticStatus").textContent = "Semantic search status unavailable";
  });
refreshTree()
  .then(() => openHome())
  .catch(showError);
setInterval(async () => {
  if (!document.hidden) {
    try {
      await refreshTree();
      await refreshCurrentNote();
    } catch {}
  }
}, 7000);
