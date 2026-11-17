async function openHistory() {
  if (!state.currentPath) return;
  $("#historyOverlay").classList.remove("hidden");
  $("#historyOverlay").classList.add("flex");
  $("#historyList").innerHTML =
    '<p class="text-xs text-zinc-500">Loading revisions…</p>';
  const revisions = await api(
    `/api/notes/revisions?path=${encodeURIComponent(state.currentPath)}`,
  );
  $("#historyList").innerHTML = revisions.length
    ? revisions
        .map(
          (rev) =>
            `<div class="flex items-center gap-3 border-b border-zinc-800 py-3"><div class="min-w-0 flex-1"><div class="text-xs text-zinc-200">${escapeHTML(new Date(rev.createdAt).toLocaleString())}</div><div class="mt-1 truncate font-mono text-[10px] text-zinc-600">${escapeHTML(rev.contentHash.slice(0, 16))}</div></div><button class="rounded-md border border-zinc-700 px-2.5 py-1.5 text-[11px] text-zinc-300 hover:bg-zinc-800" data-restore-revision="${rev.id}">Restore</button></div>`,
        )
        .join("")
    : '<p class="py-4 text-xs text-zinc-500">No earlier revisions. A revision is captured when an indexed document changes.</p>';
}
$("#historyOpen").addEventListener("click", () =>
  openHistory().catch(showError),
);
$("#historyClose").addEventListener("click", () => {
  $("#historyOverlay").classList.add("hidden");
  $("#historyOverlay").classList.remove("flex");
});
$("#historyList").addEventListener("click", async (event) => {
  const btn = event.target.closest("[data-restore-revision]");
  if (!btn) return;
  if (
    !confirm(
      "Restore this revision? The current version will be preserved in history.",
    )
  )
    return;
  try {
    const note = await api("/api/notes/restore", {
      method: "POST",
      body: JSON.stringify({
        path: state.currentPath,
        revisionId: Number(btn.dataset.restoreRevision),
      }),
    });
    state.content = note.content;
    state.renderedHtml = note.renderedHtml;
    state.isMarkdown = note.isMarkdown;
    state.dirty = false;
    $("#editor").value = note.content;
    $("#authorMode").value = note.isMarkdown ? "markdown" : "html";
    renderPreview();
    await openNote(note.path);
    await openHistory();
  } catch (e) {
    showError(e);
  }
});

$("#noteOutline").addEventListener("click", (event) => {
  const btn = event.target.closest("[data-outline-heading]");
  if (!btn) return;
  const heading = state.headings?.[Number(btn.dataset.outlineHeading)];
  if (heading)
    $("#preview").contentWindow.postMessage(
      { type: "vault-scroll-heading", text: heading.text },
      "*",
    );
});
$("#noteTags").addEventListener("click", (event) => {
  const tag = event.target.closest("[data-tag-search]")?.dataset.tagSearch;
  if (!tag) return;
  closeSearch();
  openSearch();
  $("#searchInput").value = `tag:${tag}`;
  runSearch(`tag:${tag}`).catch(showError);
});
