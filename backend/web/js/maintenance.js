"use strict";

const FOCUSABLE =
  'a[href],button:not([disabled]),input:not([disabled]),select:not([disabled]),textarea:not([disabled]),[tabindex]:not([tabindex="-1"])';

function initDialogs() {
  document.querySelectorAll(".modal-backdrop").forEach((overlay) => {
    const panel = overlay.firstElementChild;
    if (panel) {
      panel.setAttribute("role", "dialog");
      panel.setAttribute("aria-modal", "true");
      if (
        !panel.hasAttribute("aria-labelledby") &&
        !panel.hasAttribute("aria-label")
      ) {
        const heading = panel.querySelector("h2,h3,strong");
        if (heading) {
          heading.id = heading.id || `${overlay.id}Title`;
          panel.setAttribute("aria-labelledby", heading.id);
        }
      }
    }
    let opener = null;
    new MutationObserver(() => {
      const open = !overlay.classList.contains("hidden");
      if (open && !opener) {
        opener = document.activeElement;
        const target =
          overlay.querySelector("input:not([type=hidden]),select,textarea") ||
          overlay.querySelector(FOCUSABLE);
        if (target && overlay.id !== "terminalOverlay")
          setTimeout(() => target.focus(), 0);
      } else if (!open && opener) {
        if (opener.isConnected) opener.focus();
        opener = null;
      }
    }).observe(overlay, { attributes: true, attributeFilter: ["class"] });
    overlay.addEventListener("keydown", (event) => {
      if (event.key !== "Tab" || overlay.id === "terminalOverlay") return;
      const items = [...overlay.querySelectorAll(FOCUSABLE)].filter(
        (el) => el.offsetParent !== null,
      );
      if (!items.length) return;
      const first = items[0],
        last = items[items.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    });
  });
}

function bindListNavigation(input, list, itemSelector) {
  const items = () =>
    [...list.querySelectorAll(itemSelector)].filter(
      (el) => !el.classList.contains("hidden"),
    );
  const setActive = (index, all) => {
    all.forEach((el) => el.classList.remove("kbd-active"));
    if (index < 0 || index >= all.length) return;
    all[index].classList.add("kbd-active");
    all[index].scrollIntoView({ block: "nearest" });
  };
  input.addEventListener("input", () =>
    items().forEach((el) => el.classList.remove("kbd-active")),
  );
  input.addEventListener("keydown", (event) => {
    const all = items();
    const current = all.findIndex((el) => el.classList.contains("kbd-active"));
    if (event.key === "ArrowDown") {
      event.preventDefault();
      setActive(Math.min(all.length - 1, current + 1), all);
    } else if (event.key === "ArrowUp") {
      event.preventDefault();
      setActive(Math.max(0, current - 1), all);
    } else if (event.key === "Enter") {
      const target = all[current] || all[0];
      if (target) {
        event.preventDefault();
        target.click();
      }
    }
  });
}

bindListNavigation($("#searchInput"), $("#searchResults"), ".search-result");
bindListNavigation($("#commandInput"), $("#commandList"), "[data-command]");

async function openTrash() {
  const overlay = $("#trashOverlay");
  overlay.classList.remove("hidden");
  overlay.classList.add("flex");
  await renderTrash();
}

function closeTrash() {
  $("#trashOverlay").classList.add("hidden");
  $("#trashOverlay").classList.remove("flex");
}

async function renderTrash() {
  const host = $("#trashList");
  host.textContent = "Loading…";
  const data = await api("/api/trash");
  $("#trashNote").textContent =
    `Deleted notes are kept for ${data.retentionDays} days, then removed permanently.`;
  host.innerHTML = data.items.length
    ? data.items
        .map(
          (item) =>
            `<div class="flex items-center gap-3 border-b border-zinc-800 py-3"><div class="min-w-0 flex-1"><div class="truncate text-xs text-zinc-200">${escapeHTML(item.path)}</div><div class="mt-1 text-[10px] text-zinc-600">${escapeHTML(new Date(item.deletedAt).toLocaleString())}</div></div><button class="rounded-md border border-zinc-700 px-2.5 py-1.5 text-[11px] text-zinc-300" data-trash-restore="${escapeHTML(item.id)}" data-trash-path="${escapeHTML(item.path)}">Restore</button><button class="rounded-md px-2.5 py-1.5 text-[11px] text-zinc-500" data-trash-purge="${escapeHTML(item.id)}">Delete forever</button></div>`,
        )
        .join("")
    : '<p class="py-4 text-xs text-zinc-500">The trash is empty.</p>';
}

async function restoreTrashItem(id, originalPath) {
  try {
    await api("/api/trash/restore", {
      method: "POST",
      body: JSON.stringify({ id }),
    });
  } catch (error) {
    if (error.status !== 409) throw error;
    const target = await showPrompt({
      title: "Path already in use",
      description: "Choose a different path for the restored item.",
      value: originalPath.replace(/(\.[^./]+)$/, "-restored$1"),
      confirm: "Restore",
    });
    if (!target) return;
    await api("/api/trash/restore", {
      method: "POST",
      body: JSON.stringify({ id, path: target.trim() }),
    });
  }
  await refreshTree();
  await renderTrash();
}

$("#trashOpen").addEventListener("click", () => {
  document.querySelector(".tool-menu")?.removeAttribute("open");
  openTrash().catch(showError);
});
$("#trashClose").addEventListener("click", closeTrash);
$("#trashOverlay").addEventListener("click", (event) => {
  if (event.target === $("#trashOverlay")) closeTrash();
});
$("#trashList").addEventListener("click", async (event) => {
  const restore = event.target.closest("[data-trash-restore]");
  const purge = event.target.closest("[data-trash-purge]");
  try {
    if (restore)
      await restoreTrashItem(
        restore.dataset.trashRestore,
        restore.dataset.trashPath,
      );
    if (
      purge &&
      confirm("Permanently delete this item? It cannot be recovered.")
    ) {
      await api(
        `/api/trash?id=${encodeURIComponent(purge.dataset.trashPurge)}`,
        { method: "DELETE" },
      );
      await renderTrash();
    }
  } catch (error) {
    showError(error);
  }
});
paletteCommands.push(["Trash", "Restore deleted notes", () => openTrash()]);
window.addEventListener("keydown", (event) => {
  if (event.key === "Escape") closeTrash();
});

async function refreshIntegrity() {
  const section = $("#homeIntegritySection");
  try {
    const report = await api("/api/integrity");
    const rows = [];
    report.brokenLinks
      .slice(0, 6)
      .forEach((l) =>
        rows.push([
          "Broken link",
          `${l.source} links to a missing note: ${l.target}`,
          l.source,
        ]),
      );
    report.ambiguousLinks
      .slice(0, 4)
      .forEach((l) =>
        rows.push([
          "Ambiguous link",
          `${l.source}: "${l.target}" matches ${l.candidates.length} notes`,
          l.source,
        ]),
      );
    if (report.notes >= 5)
      report.orphans
        .slice(0, 6)
        .forEach((p) => rows.push(["Unlinked note", p, p]));
    const orphanCount = report.notes >= 5 ? report.orphans.length : 0;
    const extra =
      report.brokenLinks.length +
      report.ambiguousLinks.length +
      orphanCount -
      rows.length;
    section.classList.toggle("hidden", rows.length === 0);
    $("#homeIntegrity").innerHTML =
      rows
        .map(
          ([kind, text, open]) =>
            `<button data-open-path="${escapeHTML(open)}"><span class="recent-date">${escapeHTML(kind)}</span><span class="recent-summary">${escapeHTML(text)}</span></button>`,
        )
        .join("") +
      (extra > 0
        ? `<div class="px-4 py-3 text-[10px] text-zinc-600">${extra} more</div>`
        : "");
  } catch {
    section.classList.add("hidden");
  }
}

const openHomeBase = openHome;
openHome = async function openHomeWithIntegrity() {
  await openHomeBase();
  await refreshIntegrity();
};

initDialogs();

document
  .querySelector(".tool-menu-popover")
  ?.addEventListener("click", (event) => {
    if (event.target.closest("button"))
      document.querySelector(".tool-menu")?.removeAttribute("open");
  });
document.addEventListener("click", (event) => {
  const menu = document.querySelector(".tool-menu");
  if (menu && menu.open && !menu.contains(event.target))
    menu.removeAttribute("open");
});
