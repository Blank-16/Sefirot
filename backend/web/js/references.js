'use strict';

let settingsDoiLookup = false;

const ORIGIN_LABELS = {
  manual: 'Entered manually',
  bibtex: 'Imported from BibTeX; the original record is kept unchanged',
  ris: 'Imported from RIS; the original record is kept unchanged',
  crossref: 'Retrieved from Crossref; the original response is kept unchanged',
};

function describeOrigin(origin) {
  const base = (origin || 'manual').replace(/-edited$/, '');
  const label = ORIGIN_LABELS[base] || 'Unknown origin';
  return origin && origin.endsWith('-edited') ? `${label.split(';')[0]}, then edited here` : label;
}

function childBadge(origin) {
  if (origin === 'ai') return '<span class="ml-2 rounded border border-zinc-700 px-1 text-[9px] uppercase tracking-wide text-zinc-400">AI-generated</span>';
  if (origin === 'extracted') return '<span class="ml-2 rounded border border-zinc-700 px-1 text-[9px] uppercase tracking-wide text-zinc-400">Extracted</span>';
  return '';
}

function initReferencesToolbar() {
  $('#researchList').insertAdjacentHTML('beforebegin', `
    <div id="referencesTools" class="mb-3 space-y-2 border-b border-zinc-800 pb-3 text-[11px] text-zinc-400">
      <div class="flex flex-wrap gap-2">
        <button type="button" id="refImport" class="rounded border border-zinc-700 px-2 py-1">Import .bib / .ris</button>
        <button type="button" id="refExportBib" class="rounded border border-zinc-700 px-2 py-1">Export BibTeX</button>
        <button type="button" id="refExportRis" class="rounded border border-zinc-700 px-2 py-1">Export RIS</button>
        <select id="refStyle" aria-label="Citation style" class="rounded border border-zinc-700 bg-transparent px-1 py-1"><option value="apa">APA</option><option value="ieee">IEEE</option><option value="plain">Plain</option></select>
        <button type="button" id="refBibliography" class="rounded border border-zinc-700 px-2 py-1">Copy bibliography</button>
      </div>
      <div class="flex gap-2"><input id="refDOI" placeholder="Look up a DOI" class="min-w-0 flex-1 rounded border border-zinc-800 bg-zinc-900 px-2 py-1 text-zinc-100"><button type="button" id="refLookup" class="rounded border border-zinc-700 px-2 py-1">Look up</button></div>
      <label class="flex items-start gap-2"><input type="checkbox" id="refAllowLookup" class="mt-0.5"><span>Allow DOI lookup. This sends the DOI to api.crossref.org; nothing else leaves your machine.</span></label>
      <input type="file" id="refFile" accept=".bib,.bibtex,.ris,.txt" class="hidden">
    </div>`);
  $('#researchForm').insertAdjacentHTML('afterbegin', '<div id="researchProvenance" class="hidden text-[10px] leading-relaxed text-zinc-500"></div>');
  $('#refAllowLookup').checked = settingsDoiLookup;
}

function feedback(message) { $('#researchFeedback').textContent = message; }

async function importReferenceFile(file) {
  if (file.size > 5 * 1024 * 1024) { feedback('Reference files are limited to 5 MiB.'); return; }
  const report = await api('/api/research/import', { method: 'POST', body: await file.text() });
  const parts = [`Imported ${report.imported.length}`];
  if (report.duplicates.length) parts.push(`${report.duplicates.length} already in the library (same DOI)`);
  if (report.errors.length) parts.push(`${report.errors.length} problem${report.errors.length === 1 ? '' : 's'}: ${report.errors.slice(0, 2).join('; ')}`);
  feedback(parts.join(' · '));
  await refreshResearch();
}

async function fetchText(url) {
  const response = await fetch(url);
  const text = await response.text();
  if (!response.ok) { let message = text; try { message = JSON.parse(text).error || text; } catch {} throw new Error(message); }
  return text;
}

function bindReferencesToolbar() {
  const run = fn => async () => { try { await fn(); } catch (error) { feedback(error.message); } };
  $('#refImport').addEventListener('click', () => $('#refFile').click());
  $('#refFile').addEventListener('change', run(async () => {
    const file = $('#refFile').files[0];
    $('#refFile').value = '';
    if (file) await importReferenceFile(file);
  }));
  $('#refExportBib').addEventListener('click', () => { window.location.href = '/api/research/export?format=bibtex'; });
  $('#refExportRis').addEventListener('click', () => { window.location.href = '/api/research/export?format=ris'; });
  $('#refBibliography').addEventListener('click', run(async () => {
    const text = await fetchText(`/api/research/bibliography?style=${encodeURIComponent($('#refStyle').value)}`);
    if (!text.trim()) { feedback('The library is empty.'); return; }
    await navigator.clipboard.writeText(text);
    feedback(`Copied ${text.trim().split('\n').length} references.`);
  }));
  $('#refAllowLookup').addEventListener('change', () => {
    settingsDoiLookup = $('#refAllowLookup').checked;
    if (typeof pushSettings === 'function') pushSettings();
  });
  $('#refLookup').addEventListener('click', run(async () => {
    const doi = $('#refDOI').value.trim();
    if (!doi) return;
    if (!settingsDoiLookup) { feedback('Turn on DOI lookup first. It sends the DOI to api.crossref.org.'); return; }
    const result = await api('/api/research/lookup-doi', { method: 'POST', body: JSON.stringify({ doi }) });
    feedback(result.duplicate ? 'This DOI is already in the library.' : `Added “${result.paper.title}” from Crossref.`);
    $('#refDOI').value = '';
    await refreshResearch();
    populateResearchForm(result.paper);
  }));
}

const populateResearchFormBase = populateResearchForm;
populateResearchForm = function populateWithProvenance(p) {
  populateResearchFormBase(p);
  const host = $('#researchProvenance');
  host.classList.remove('hidden');
  host.innerHTML = `${escapeHTML(describeOrigin(p.metadataOrigin))}${p.citeKey ? ` · key ${escapeHTML(p.citeKey)}` : ''}${p.sourceRecord ? '<details class="mt-1"><summary class="cursor-pointer">Original record</summary><pre class="mt-1 max-h-40 overflow-auto whitespace-pre-wrap rounded bg-zinc-900 p-2 text-[10px] text-zinc-400"></pre></details>' : ''}`;
  const pre = host.querySelector('pre');
  if (pre) pre.textContent = p.sourceRecord.length > 6000 ? `${p.sourceRecord.slice(0, 6000)}\n…` : p.sourceRecord;
};

const resetResearchFormBase = resetResearchForm;
resetResearchForm = function resetWithProvenance() {
  resetResearchFormBase();
  $('#researchProvenance')?.classList.add('hidden');
};

loadResearchDetails = async function loadWithDeleteButtons(id) {
  const data = await api(`/api/research/${id}`);
  $('#researchCitationList').innerHTML = (data.citations || []).map(c => `<div class="flex items-start gap-2 rounded bg-zinc-900 px-2 py-1"><span class="min-w-0 flex-1">${escapeHTML(c.citationText)}${c.locator ? ` · ${escapeHTML(c.locator)}` : ''}${childBadge(c.origin)}</span><button type="button" class="text-zinc-500" data-delete-citation="${c.id}" aria-label="Remove citation">Remove</button></div>`).join('') || '<span>No citations added.</span>';
  $('#researchAnnotationList').innerHTML = (data.annotations || []).map(a => `<div class="rounded bg-zinc-900 p-2"><div class="flex items-start gap-2"><span class="min-w-0 flex-1"><span class="text-zinc-400">Page ${a.page}</span> · ${escapeHTML(a.quote)}${childBadge(a.origin)}</span><button type="button" class="text-zinc-500" data-delete-annotation="${a.id}" aria-label="Remove annotation">Remove</button></div><p class="mt-1 text-zinc-500">${escapeHTML(a.note)}</p></div>`).join('') || '<span>No annotations added.</span>';
};

$('#researchForm').addEventListener('click', async event => {
  const cite = event.target.closest('[data-delete-citation]');
  const note = event.target.closest('[data-delete-annotation]');
  const id = $('#researchId').value;
  if (!id || (!cite && !note)) return;
  try {
    const path = cite ? `citations/${cite.dataset.deleteCitation}` : `annotations/${note.dataset.deleteAnnotation}`;
    await api(`/api/research/${id}/${path}`, { method: 'DELETE' });
    await loadResearchDetails(id);
  } catch (error) { feedback(error.message); }
});

initReferencesToolbar();
bindReferencesToolbar();
