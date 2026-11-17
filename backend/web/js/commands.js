const paletteCommands = [
  ["New note", "Create a document", () => createNote()],
  ["New folder", "Create a folder", () => createFolder()],
  ["Open search", "Search the vault", () => openSearch()],
  ["Knowledge graph", "Explore document relationships", () => openGraph()],
  ["Terminal", "Open a local CLI session", () => openTerminal()],
  ["Home", "Open recent documents", () => openHome()],
  ["Save note", "Save the current document", () => saveNote()],
  [
    "Toggle theme",
    "Switch light and dark mode",
    () =>
      setTheme(
        document.documentElement.dataset.theme === "dark" ? "light" : "dark",
      ),
  ],
  [
    "Export vault",
    "Download a ZIP backup",
    async () => {
      if (state.dirty) await saveNote();
      window.location.href = "/api/export";
    },
  ],
];
function openCommandPalette() {
  const host = $("#commandList");
  host.innerHTML = paletteCommands
    .map(
      (c, i) =>
        `<button class="block w-full rounded-md px-3 py-2 text-left hover:bg-zinc-800" data-command="${i}"><span class="block text-xs text-zinc-200">${escapeHTML(c[0])}</span><span class="block text-[10px] text-zinc-600">${escapeHTML(c[1])}</span></button>`,
    )
    .join("");
  $("#commandOverlay").classList.remove("hidden");
  $("#commandOverlay").classList.add("flex");
  $("#commandInput").value = "";
  $("#commandInput").focus();
}
function closeCommandPalette() {
  $("#commandOverlay").classList.add("hidden");
  $("#commandOverlay").classList.remove("flex");
}
$("#commandInput").addEventListener("input", (e) => {
  const q = e.target.value.toLowerCase();
  $("#commandList")
    .querySelectorAll("[data-command]")
    .forEach((btn) => {
      const c = paletteCommands[Number(btn.dataset.command)];
      btn.classList.toggle(
        "hidden",
        !`${c[0]} ${c[1]}`.toLowerCase().includes(q),
      );
    });
});
$("#commandList").addEventListener("click", async (e) => {
  const btn = e.target.closest("[data-command]");
  if (!btn) return;
  const command = paletteCommands[Number(btn.dataset.command)];
  closeCommandPalette();
  try {
    await command[2]();
  } catch (err) {
    showError(err);
  }
});
$("#commandOverlay").addEventListener("click", (e) => {
  if (e.target === $("#commandOverlay")) closeCommandPalette();
});

// Integrated local PTY terminal. The server executes commands as the current OS user.
