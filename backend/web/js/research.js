"use strict";

const researchFields = {
  id: "#researchId",
  title: "#researchTitle",
  authors: "#researchAuthors",
  abstract: "#researchAbstract",
  doi: "#researchDOI",
  publication: "#researchPublication",
  publishedAt: "#researchPublished",
  status: "#researchStatus",
  url: "#researchURL",
  keywords: "#researchKeywords",
  notes: "#researchNotes",
};
let researchItems = [];
let selectedResearchID = 0;
const researchValue = (key) => $(researchFields[key]).value;
function resetResearchForm() {
  selectedResearchID = 0;
  Object.values(researchFields).forEach((selector) => {
    $(selector).value = "";
  });
  $("#researchStatus").value = "unread";
  $("#researchPDF").value = "";
  $("#researchDelete").classList.add("hidden");
  $("#researchFeedback").textContent =
    "Create a record for a paper, preprint, thesis or technical report.";
  $("#researchCitationList").textContent =
    "Save a paper before adding citations.";
  $("#researchAnnotationList").textContent =
    "Save a paper before adding annotations.";
  $("#researchPDFStatus").textContent =
    "PDF files are stored locally, unchanged.";
}
function populateResearchForm(p) {
  selectedResearchID = p.id;
  $("#researchId").value = p.id;
  $("#researchPDF").value = "";
  $("#researchTitle").value = p.title || "";
  $("#researchAuthors").value = (p.authors || []).join("\n");
  $("#researchAbstract").value = p.abstract || "";
  $("#researchDOI").value = p.doi || "";
  $("#researchPublication").value = p.publication || "";
  $("#researchPublished").value = p.publishedAt || "";
  $("#researchStatus").value = p.readingStatus || "unread";
  $("#researchURL").value = p.url || "";
  $("#researchKeywords").value = (p.keywords || []).join(", ");
  $("#researchNotes").value = p.personalNotes || "";
  $("#researchDelete").classList.remove("hidden");
  $("#researchFeedback").textContent =
    `Updated ${new Date(p.updatedAt).toLocaleString()}`;
  if (p.sourceFile) {
    const url =
      "/vault-assets/" +
      p.sourceFile.split("/").map(encodeURIComponent).join("/");
    $("#researchPDFStatus").innerHTML =
      `PDF stored locally · <a class=\"text-indigo-300 underline\" target=\"_blank\" rel=\"noopener\" href=\"${url}\">Open original PDF</a>`;
  } else {
    $("#researchPDFStatus").textContent = "No PDF attached.";
  }
  loadResearchDetails(p.id).catch((error) => {
    $("#researchFeedback").textContent = error.message;
  });
}
async function loadResearchDetails(id) {
  const data = await api(`/api/research/${id}`);
  $("#researchCitationList").innerHTML =
    (data.citations || [])
      .map(
        (c) =>
          `<div class=\"rounded bg-zinc-900 px-2 py-1\">${escapeHTML(c.citationText)}${c.locator ? ` · ${escapeHTML(c.locator)}` : ""}</div>`,
      )
      .join("") || "<span>No citations added.</span>";
  $("#researchAnnotationList").innerHTML =
    (data.annotations || [])
      .map(
        (a) =>
          `<div class=\"rounded bg-zinc-900 p-2\"><span class=\"text-zinc-400\">Page ${a.page}</span> · ${escapeHTML(a.quote)}<p class=\"mt-1 text-zinc-500\">${escapeHTML(a.note)}</p></div>`,
      )
      .join("") || "<span>No annotations added.</span>";
}
function renderResearchList() {
  const host = $("#researchList");
  if (!researchItems.length) {
    host.innerHTML =
      '<p class="rounded border border-dashed border-zinc-800 p-4 text-xs text-zinc-500">No research papers yet. Add a paper to start a literature library.</p>';
    return;
  }
  host.innerHTML = researchItems
    .map(
      (p) =>
        `<button type="button" data-research-id="${p.id}" class="block w-full rounded-lg border ${p.id === selectedResearchID ? "border-indigo-500 bg-zinc-900" : "border-zinc-800 bg-zinc-900/40"} p-3 text-left"><span class="block text-xs font-medium text-zinc-200">${escapeHTML(p.title)}</span><span class="mt-1 block text-[10px] text-zinc-500">${escapeHTML((p.authors || []).join(", ") || "Unknown authors")}</span><span class="mt-2 flex items-center gap-2 text-[10px] text-zinc-600"><span>${escapeHTML(p.readingStatus)}</span>${p.sourceFile ? "<span>PDF attached</span>" : ""}${p.doi ? `<span>DOI ${escapeHTML(p.doi)}</span>` : ""}</span></button>`,
    )
    .join("");
}
async function refreshResearch() {
  const result = await api("/api/research");
  researchItems = result.items || [];
  renderResearchList();
}
async function openResearch() {
  $("#researchOverlay").classList.remove("hidden");
  $("#researchOverlay").classList.add("flex");
  await refreshResearch();
  resetResearchForm();
}
function closeResearch() {
  $("#researchOverlay").classList.add("hidden");
  $("#researchOverlay").classList.remove("flex");
}
$("#researchOpen").addEventListener("click", () =>
  openResearch().catch(showError),
);
$("#researchClose").addEventListener("click", closeResearch);
$("#researchNew").addEventListener("click", resetResearchForm);
$("#researchReset").addEventListener("click", resetResearchForm);
$("#researchList").addEventListener("click", (event) => {
  const button = event.target.closest("[data-research-id]");
  if (!button) return;
  const paper = researchItems.find(
    (p) => p.id === Number(button.dataset.researchId),
  );
  if (!paper) return;
  populateResearchForm(paper);
  renderResearchList();
});
$("#researchForm").addEventListener("submit", async (event) => {
  event.preventDefault();
  const id = Number(researchValue("id")) || 0;
  const payload = {
    title: researchValue("title").trim(),
    authors: researchValue("authors")
      .split("\n")
      .map((s) => s.trim())
      .filter(Boolean),
    abstract: researchValue("abstract"),
    doi: researchValue("doi").trim(),
    publication: researchValue("publication"),
    publishedAt: researchValue("publishedAt"),
    readingStatus: researchValue("status"),
    url: researchValue("url").trim(),
    keywords: researchValue("keywords")
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean),
    personalNotes: researchValue("notes"),
  };
  try {
    const saved = await api(id ? `/api/research/${id}` : "/api/research", {
      method: id ? "PUT" : "POST",
      body: JSON.stringify(payload),
    });
    await refreshResearch();
    populateResearchForm(saved);
    renderResearchList();
    $("#researchFeedback").textContent = "Paper metadata saved locally.";
  } catch (error) {
    $("#researchFeedback").textContent = error.message;
  }
});
$("#researchDelete").addEventListener("click", async () => {
  const id = Number(researchValue("id"));
  if (
    !id ||
    !confirm("Delete this research record and its annotations/citations?")
  )
    return;
  try {
    await api(`/api/research/${id}`, { method: "DELETE" });
    await refreshResearch();
    resetResearchForm();
  } catch (error) {
    $("#researchFeedback").textContent = error.message;
  }
});

// Insert TeX snippets at the current caret without disturbing surrounding source.
$("#mathToolbar").addEventListener("click", (event) => {
  const button = event.target.closest("[data-math]");
  if (!button) return;
  const editor = $("#editor");
  const raw = button.dataset.math.replace(/\\n/g, "\n");
  const cursor = raw.indexOf("|");
  const snippet = raw.replace("|", "");
  const start = editor.selectionStart,
    end = editor.selectionEnd;
  editor.setRangeText(snippet, start, end, "end");
  if (cursor >= 0) editor.setSelectionRange(start + cursor, start + cursor);
  editor.focus();
  editor.dispatchEvent(new Event("input", { bubbles: true }));
});

$("#researchUploadPDF").addEventListener("click", async () => {
  const id = Number(researchValue("id"));
  const file = $("#researchPDF").files[0];
  if (!id) {
    $("#researchPDFStatus").textContent =
      "Save the paper record before attaching a PDF.";
    return;
  }
  if (!file) {
    $("#researchPDFStatus").textContent = "Choose a PDF first.";
    return;
  }
  const form = new FormData();
  form.append("pdf", file, file.name);
  $("#researchPDFStatus").textContent = "Uploading to the local vault…";
  try {
    const response = await fetch(`/api/research/${id}/pdf`, {
      method: "POST",
      body: form,
    });
    const result = await response.json().catch(() => ({}));
    if (!response.ok)
      throw new Error(result.error || `Upload failed (${response.status})`);
    await refreshResearch();
    populateResearchForm(result);
    const pdfURL =
      "/vault-assets/" +
      result.sourceFile.split("/").map(encodeURIComponent).join("/");
    $("#researchPDFStatus").innerHTML =
      `Stored locally · <a class=\"text-indigo-300 underline\" target=\"_blank\" rel=\"noopener\" href=\"${pdfURL}\">Open original PDF</a>`;
    $("#researchPDF").value = "";
  } catch (error) {
    $("#researchPDFStatus").textContent = error.message;
  }
});

$("#researchAddCitation").addEventListener("click", async () => {
  const id = Number(researchValue("id"));
  const citationText = $("#researchCitationText").value.trim();
  if (!id || !citationText) {
    $("#researchFeedback").textContent =
      "Save a paper and enter citation text first.";
    return;
  }
  try {
    await api(`/api/research/${id}/citations`, {
      method: "POST",
      body: JSON.stringify({
        citationText,
        locator: $("#researchCitationLocator").value.trim(),
      }),
    });
    $("#researchCitationText").value = "";
    $("#researchCitationLocator").value = "";
    await loadResearchDetails(id);
  } catch (error) {
    $("#researchFeedback").textContent = error.message;
  }
});
$("#researchAddAnnotation").addEventListener("click", async () => {
  const id = Number(researchValue("id"));
  const note = $("#researchAnnotationNote").value.trim();
  if (!id || !note) {
    $("#researchFeedback").textContent =
      "Save a paper and enter an annotation first.";
    return;
  }
  try {
    await api(`/api/research/${id}/annotations`, {
      method: "POST",
      body: JSON.stringify({
        page: Number($("#researchAnnotationPage").value) || 0,
        quote: $("#researchAnnotationQuote").value,
        note,
      }),
    });
    $("#researchAnnotationPage").value = "";
    $("#researchAnnotationQuote").value = "";
    $("#researchAnnotationNote").value = "";
    await loadResearchDetails(id);
  } catch (error) {
    $("#researchFeedback").textContent = error.message;
  }
});
