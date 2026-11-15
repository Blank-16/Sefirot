async function refreshTree() {
  const data = await api('/api/tree');
  state.tree = data.children || [];
  const available=new Set();const collect=nodes=>nodes.forEach(n=>{if(n.type==='note')available.add(n.path);else collect(n.children||[])});collect(state.tree);state.tabs=state.tabs.filter(p=>available.has(p));persistTabs();renderTabs();
  if(state.currentPath&&!available.has(state.currentPath)&&!state.dirty){state.currentPath='';state.content='';state.renderedHtml='';if(state.page==='notes')openHome().catch(showError)}
  renderTree();
}
function renderTree() {
  const host = $('#fileTree');
  const count = countNotes(state.tree);
  $('#fileCount').textContent = `${count} note${count === 1 ? '' : 's'}`;
  if (!state.tree.length) { host.innerHTML = '<p class="px-2 py-3 text-xs text-zinc-600">No notes yet.</p>'; return; }
  host.innerHTML = state.tree.map(node => renderNode(node, 0)).join('');
}
function countNotes(nodes) { return nodes.reduce((n, x) => n + (x.type === 'note' ? 1 : countNotes(x.children || [])), 0); }
function renderNode(node, depth) {
  const isFolder = node.type === 'folder';
  const open = state.expanded.has(node.path);
  const active = node.path === state.currentPath;
  const label = isFolder ? node.name : (node.title || withoutExt(node.name));
  const icon = isFolder ? (open ? '⌄' : '›') : '◦';
  return `<div class="tree-node">
    <div class="tree-row ${active ? 'active' : ''}" role="treeitem" tabindex="0" aria-selected="${active}" ${isFolder ? `aria-expanded="${open}"` : ''} data-path="${escapeHTML(node.path)}" data-type="${node.type}" style="padding-left:${8 + depth * 10}px">
      <span class="w-3 text-center text-zinc-600">${icon}</span><span class="min-w-0 flex-1 truncate">${escapeHTML(label)}</span>
      ${isFolder ? '' : `<span class="actions items-center gap-0.5"><button class="icon-btn !h-6 !w-6" data-action="rename" data-path="${escapeHTML(node.path)}" title="Rename">✎</button><button class="icon-btn !h-6 !w-6 hover:!text-red-300" data-action="delete" data-path="${escapeHTML(node.path)}" title="Delete">×</button></span>`}
    </div>
    ${isFolder && open ? `<div class="tree-children">${(node.children || []).map(child => renderNode(child, depth + 1)).join('')}</div>` : ''}
  </div>`;
}

async function openNote(notePath) {
  if (state.dirty && state.currentPath) await saveNote();
  const note = await api(`/api/notes?path=${encodeURIComponent(notePath)}`);
  if (state.currentPath !== note.path) state.previewScroll = {path: note.path, y: 0};
  state.currentPath = note.path; state.content = note.content; state.renderedHtml = note.renderedHtml; state.contentHash=note.contentHash||'';state.headings=note.headings||[];state.externalConflict=false;
  applyResearchMode(isResearchMode(note.path));
  if (!state.tabs.includes(note.path)) state.tabs.push(note.path); persistTabs(); renderTabs();
  state.isMarkdown = note.isMarkdown; state.dirty = false;
  $('#editor').value = state.content; $('#authorMode').value = state.isMarkdown ? 'markdown' : 'html';
  $('#editorModeLabel').textContent = state.isMarkdown ? 'Markdown source' : 'HTML source';
  $('#breadcrumb').textContent = note.path; $('#propPath').textContent = note.path; renderSummary(note); $('#noteOutline').innerHTML=(note.headings||[]).map((h,i)=>`<button class="block w-full truncate text-left text-[10px] text-zinc-500 hover:text-zinc-200" style="padding-left:${Math.min((h.level||1)-1,5)*8}px" data-outline-heading="${i}">${escapeHTML(h.text||h)}</button>`).join('')||'<span class="text-[10px] text-zinc-600">No headings</span>';
  $('#propFormat').textContent = note.isMarkdown ? 'Markdown → HTML' : 'Raw HTML';
  $('#noteType').textContent = note.type || 'note'; $('#noteUpdated').textContent = note.updatedAt ? new Date(note.updatedAt).toLocaleString() : '—';
  $('#noteTags').innerHTML = (note.tags || []).map(tag => `<button class="rounded-full border border-zinc-800 px-2 py-1 text-[10px] text-zinc-500 hover:text-zinc-200" data-tag-search="${escapeHTML(tag)}">${escapeHTML(tag)}</button>`).join('');
  $('#graphView').classList.add('hidden'); $('#homeView').classList.add('hidden'); state.page='notes'; $('#emptyState').classList.add('hidden-pane'); $('#documentToolbar').classList.remove('hidden'); if (state.rightOpen) $('#rightSidebar').classList.remove('hidden-pane'); $('#workspace').classList.remove('hidden');
  state.expanded.add(note.path.split('/').slice(0,-1).join('/'));
  renderTree(); renderBacklinks(note.backlinks || []); renderPreview(); applyView(); setStatus('Saved');
}
function renderSummary(note) {
  const short = note.summaryShort || note.summary || 'No summary available.';
  const detail = note.summaryDetail || '';
  $('#noteSummary').textContent = short;
  const redundant = !detail || detail.startsWith(short.replace(/…$|\.\.\.$/, '').slice(0, 60)) || short.startsWith(detail.slice(0, 60));
  $('#noteSummaryDetail').textContent = redundant ? '' : detail;
}
function renderBacklinks(items) {
  const el = $('#backlinks');
  el.innerHTML = items.length ? items.map(item => `<button class="block w-full truncate rounded px-2 py-1.5 text-left text-xs text-zinc-400 hover:bg-zinc-900 hover:text-zinc-100" data-open-path="${escapeHTML(item.path)}">${escapeHTML(item.title)}</button>`).join('') : '<p class="px-1 py-1 text-xs text-zinc-600">No incoming links.</p>';
}
function makePreviewDocument() {
  let html = state.renderedHtml || '';
  if (state.isMarkdown && /^\s*(<!doctype\s+html|<html\b)/i.test(html)) { const parsed=new DOMParser().parseFromString(html,'text/html'); html=parsed.body.innerHTML; }
  const scrollY = state.previewScroll && state.previewScroll.path === state.currentPath ? Math.max(0, Number(state.previewScroll.y) || 0) : 0;
  const bridge = `<script>(function(){var restore=${scrollY};if(restore){var apply=function(){window.scrollTo(0,restore)};window.addEventListener('load',function(){apply();setTimeout(apply,120);setTimeout(apply,500)})}var tick=null;window.addEventListener('scroll',function(){if(tick)return;tick=setTimeout(function(){tick=null;parent.postMessage({type:'vault-scroll',y:window.scrollY},'*')},150)});document.addEventListener('click',function(e){const a=e.target.closest('[data-wikilink]');if(!a)return;e.preventDefault();parent.postMessage({type:'vault-wikilink',target:a.getAttribute('data-wikilink')},'*')});window.addEventListener('message',function(e){if(!e.data||e.data.type!=='vault-scroll-heading')return;const target=String(e.data.text||'').trim();const heading=[...document.querySelectorAll('h1,h2,h3,h4,h5,h6')].find(h=>h.textContent.trim()===target);if(heading)heading.scrollIntoView({behavior:'smooth',block:'start'})})})();<\/script>`;
  const folder=state.currentPath.includes('/')?state.currentPath.split('/').slice(0,-1).map(encodeURIComponent).join('/')+'/':'';
  const baseTag = `<base href="/vault-assets/${folder}">`;
  const mathMacros = JSON.stringify(loadMathMacros()).replace(/[<>&\u2028\u2029]/g, ch => ({"<":"\\u003c", ">":"\\u003e", "&":"\\u0026", "\u2028":"\\u2028", "\u2029":"\\u2029"}[ch]));
  const mathJax = `<script>window.MathJax={tex:{inlineMath:[['\\\\(','\\\\)'],['$','$']],displayMath:[['\\\\[','\\\\]'],['$$','$$']],tags:'ams',packages:{'[+]':['ams','mhchem','mathtools','braket']},macros:${mathMacros}},options:{skipHtmlTags:['script','noscript','style','textarea','pre','code']},chtml:{matchFontHeight:false}};<\/script><script defer src="/vendor/mathjax/tex-mml-chtml.js"><\/script>`;
  const mathJaxMarkup = state.researchMode ? mathJax : '';
  const isDocument = /^\s*(<!doctype\s+html|<html\b)/i.test(html);
  if (!isDocument) {
    const light = document.documentElement.dataset.theme === 'light';
    const style = `<style>
      :root{color-scheme:${light ? 'light' : 'dark'}}
      *{box-sizing:border-box}
      body{margin:0 auto;max-width:860px;padding:48px clamp(22px,5vw,64px) 96px;background:${light ? '#FAF9F3' : '#090B09'};color:${light ? '#20251F' : '#E9E8DF'};font:17px/1.78 Georgia,'Palatino Linotype',serif;-webkit-font-smoothing:antialiased}
      h1,h2,h3,h4{color:${light ? '#20251F' : '#F0EFE7'};font-family:Georgia,'Palatino Linotype',serif;font-weight:400;line-height:1.2;scroll-margin-top:28px}
      h1{margin:0 0 1.05em;font-size:2.5rem;letter-spacing:-.035em}
      h2{margin:2.1em 0 .65em;padding-bottom:.28em;border-bottom:1px solid ${light ? '#D6D3C7' : '#252821'};font-size:1.55rem}
      h3{margin:1.65em 0 .55em;font-size:1.22rem}
      h4{font-size:1.08rem}
      p,li{max-width:74ch}
      a{color:${light ? '#526356' : '#B69B5B'};text-decoration-color:${light ? '#A88E52' : '#74613A'};text-underline-offset:3px}
      code,pre{font-family:ui-monospace,'Cascadia Code',Consolas,monospace}
      code{background:${light ? '#E8E6DC' : '#11140F'};padding:.13em .32em;font-size:.84em}
      pre{overflow:auto;padding:16px 19px;border-left:2px solid ${light ? '#A88E52' : '#B69B5B'};background:${light ? '#F0EEE5' : '#070907'};font-size:.82rem;line-height:1.65}
      blockquote{margin:1.5em 0;padding:.15em 0 .15em 1.1em;border-left:2px solid ${light ? '#A88E52' : '#B69B5B'};color:${light ? '#77796E' : '#A09F93'};font-style:italic}
      img{max-width:100%;height:auto;border:1px solid ${light ? '#D6D3C7' : '#252821'}}
      table{width:100%;border-collapse:collapse;font-size:.9em}
      td,th{padding:.55em .7em;border-bottom:1px solid ${light ? '#D6D3C7' : '#252821'};text-align:left}
      th{color:${light ? '#526356' : '#B69B5B'};font-weight:600}
      hr{margin:2em 0;border:0;border-top:1px solid ${light ? '#D6D3C7' : '#252821'}}
      mjx-container{overflow-x:auto;overflow-y:hidden;max-width:100%}
      @media(max-width:600px){body{padding:28px 19px 60px;font-size:16px}h1{font-size:2rem}}
    </style>`;
    return `<!doctype html><html><head><meta charset="utf-8">${baseTag}${style}${mathJaxMarkup}</head><body>${html}${bridge}</body></html>`;
  }
  if (/<\/head\s*>/i.test(html)) html = html.replace(/<\/head\s*>/i, () => `${mathJaxMarkup}</head>`);
  else html = mathJaxMarkup + html;
  if (/<\/body\s*>/i.test(html)) return html.replace(/<\/body\s*>/i, () => `${bridge}</body>`);
  if (/<\/html\s*>/i.test(html)) return html.replace(/<\/html\s*>/i, () => `${bridge}</html>`);
  return html + bridge;
}
function renderPreview() { $('#preview').srcdoc = makePreviewDocument(); }
const narrowScreen = window.matchMedia('(max-width: 800px)');
narrowScreen.addEventListener('change', () => { if (state.currentPath) applyView(); });
function effectiveView() { return state.view === 'split' && narrowScreen.matches ? 'edit' : state.view; }
function applyView() {
  const view = effectiveView();
  const edit = $('#editorPane'), preview = $('#previewPane');
  edit.classList.toggle('hidden-pane', view === 'preview');
  preview.classList.toggle('hidden-pane', view === 'edit');
  $('#panes').classList.toggle('split-grid', view === 'split');
  $('#panes').style.display = view === 'edit' ? 'flex' : 'grid';
  document.querySelectorAll('.view-btn').forEach(btn => {
    const active = btn.dataset.view === view;
    btn.className = `view-btn ${active ? 'bg-zinc-700 text-zinc-100' : 'text-zinc-400 hover:text-zinc-200'}`;
    btn.setAttribute('aria-pressed', String(active));
  });
}
function markDirty() {
  state.content = $('#editor').value; state.dirty = true; setStatus('Unsaved', 'text-amber-400');
  if (state.saveTimer) clearTimeout(state.saveTimer);
  if (!state.externalConflict) state.saveTimer = setTimeout(() => saveNote().catch(showError), 800);
  if (state.isMarkdown) {
    // Live Markdown preview is rendered by the server on save to keep the parser authoritative.
    renderLocalMarkdownPreview();
  }
}
function renderLocalMarkdownPreview() {
  // A lightweight placeholder avoids a second Markdown implementation in the browser.
  // The authoritative Goldmark preview refreshes immediately after autosave.
}
async function saveNote(force = false) {
  if (!state.currentPath || !state.dirty) return;
  if (state.savePromise) {
    await state.savePromise;
    if (state.dirty) return saveNote(force);
    return;
  }
  if (state.saveTimer) { clearTimeout(state.saveTimer); state.saveTimer = null; }
  const pathAtSave = state.currentPath;
  const contentAtSave = $('#editor').value;
  const modeAtSave = state.isMarkdown;
  state.saving = true;
  setStatus('Saving…', 'text-amber-400');
  const task = (async () => {
    const note = await api('/api/notes', {method:'POST',body:JSON.stringify({path:pathAtSave,content:contentAtSave,isMarkdown:modeAtSave,expectedHash:state.contentHash,force})});
    state.renderedHtml = note.renderedHtml; state.contentHash=note.contentHash||'';state.headings=note.headings||[];state.externalConflict=false;if(!state.tabs.includes(pathAtSave)){state.tabs.push(pathAtSave);persistTabs();renderTabs()}
    if (state.currentPath === pathAtSave && $('#editor').value === contentAtSave && state.isMarkdown === modeAtSave) {
      state.content = contentAtSave;
      state.dirty = false;
      renderPreview();
      renderBacklinks(note.backlinks || []); renderSummary(note); $('#noteOutline').innerHTML=(note.headings||[]).map((h,i)=>`<button class="block w-full truncate text-left text-[10px] text-zinc-500 hover:text-zinc-200" style="padding-left:${Math.min((h.level||1)-1,5)*8}px" data-outline-heading="${i}">${escapeHTML(h.text||h)}</button>`).join('')||'<span class="text-[10px] text-zinc-600">No headings</span>'; $('#noteType').textContent=note.type||'note';$('#noteUpdated').textContent=note.updatedAt?new Date(note.updatedAt).toLocaleString():'—';$('#noteTags').innerHTML=(note.tags||[]).map(tag=>`<button class="rounded-full border border-zinc-800 px-2 py-1 text-[10px] text-zinc-500 hover:text-zinc-200" data-tag-search="${escapeHTML(tag)}">${escapeHTML(tag)}</button>`).join('');
      setStatus('Saved', 'text-emerald-400');
    } else {
      state.dirty = true;
      setStatus('Unsaved', 'text-amber-400');
    }
    await refreshTree();
  })();
  state.savePromise = task.finally(() => {
    state.saving = false;
    state.savePromise = null;
    if (state.dirty && !state.saveTimer && !state.externalConflict) state.saveTimer = setTimeout(() => saveNote().catch(showError), 250);
  });
  try { await state.savePromise; }
  catch (error) { setStatus('Save failed', 'text-red-400'); if(error.status===409){state.externalConflict=true;$('#conflictOverlay').classList.remove('hidden');$('#conflictOverlay').classList.add('flex')} throw error; }
}

async function reloadExternalVersion(){const path=state.currentPath;try{state.dirty=false;const note=await api(`/api/notes?path=${encodeURIComponent(path)}`);$('#conflictOverlay').classList.add('hidden');$('#conflictOverlay').classList.remove('flex');await openNote(path)}catch(e){$('#conflictOverlay').classList.add('hidden');$('#conflictOverlay').classList.remove('flex');state.externalConflict=false;state.currentPath='';state.dirty=false;await openHome();if(e.status!==404)showError(e)}}
$('#conflictReload').addEventListener('click',()=>reloadExternalVersion().catch(showError));
$('#conflictOverwrite').addEventListener('click',async()=>{state.externalConflict=false;$('#conflictOverlay').classList.add('hidden');$('#conflictOverlay').classList.remove('flex');try{await saveNote(true)}catch(e){showError(e)}});
async function refreshCurrentNote(){if(!state.currentPath||state.page!=='notes')return;const path=state.currentPath;try{const note=await api(`/api/notes?path=${encodeURIComponent(path)}`);if(path!==state.currentPath||note.contentHash===state.contentHash)return;if(state.dirty){if(!state.externalConflict){state.externalConflict=true;$('#conflictOverlay').classList.remove('hidden');$('#conflictOverlay').classList.add('flex')}}else{await openNote(path)}}catch(e){if(e.status===404&&state.dirty&&!state.externalConflict){state.externalConflict=true;$('#conflictOverlay').classList.remove('hidden');$('#conflictOverlay').classList.add('flex')}}}

function persistTabs(){try{localStorage.setItem('sefirot-tabs',JSON.stringify(state.tabs))}catch{}if(typeof queueSettingsSync==='function')queueSettingsSync()}
function renderTabs(){const host=$('#tabBar');host.classList.toggle('hidden',state.tabs.length===0);host.classList.toggle('flex',state.tabs.length>0);host.innerHTML=state.tabs.map(p=>`<div class="flex h-7 shrink-0 items-center rounded-md ${p===state.currentPath?'bg-zinc-800 text-zinc-100':'text-zinc-500 hover:bg-zinc-900'}"><button class="max-w-40 truncate px-2 text-[11px]" data-tab-path="${escapeHTML(p)}">${escapeHTML(withoutExt(basename(p)))}</button><button class="px-1 text-xs text-zinc-500 hover:text-zinc-100" data-close-tab="${escapeHTML(p)}" title="Close tab">×</button></div>`).join('')}
async function openHome(){if(state.dirty)await saveNote();state.page='home';$('#workspace').classList.add('hidden');$('#graphView').classList.add('hidden');$('#documentToolbar').classList.add('hidden');$('#rightSidebar').classList.add('hidden-pane');$('#emptyState').classList.add('hidden-pane');$('#homeView').classList.remove('hidden');try{const [recent,runs]=await Promise.all([api('/api/recent'),api('/api/agents/runs?limit=12')]);$('#homeRecentCount').textContent=`${recent.length} documents`;$('#homeRecent').innerHTML=recent.length?recent.map(item=>`<button class="flex w-full flex-col gap-1 px-4 py-3 text-left hover:bg-zinc-900" data-open-path="${escapeHTML(item.path)}"><span class="recent-title">${escapeHTML(item.title)}</span><time class="recent-date" datetime="${escapeHTML(item.updatedAt)}">Updated ${escapeHTML(new Date(item.updatedAt).toLocaleDateString())}</time><span class="recent-path">${escapeHTML(item.path)}</span><span class="recent-summary">${escapeHTML(item.summary)}</span></button>`).join(''):'<div class="p-4 text-xs text-zinc-500">No documents yet. Create a note to start the vault.</div>';$('#homeAgentRuns').innerHTML=runs.length?runs.map(run=>`<div class="flex items-center justify-between gap-3 px-4 py-3"><div class="min-w-0"><div class="text-xs font-medium text-zinc-200">${escapeHTML(run.profile)}</div><div class="truncate text-[10px] text-zinc-600">${escapeHTML(run.workingDirectory)}</div><time class="agent-date">${escapeHTML(new Date(run.startedAt).toLocaleString())}</time></div><span class="shrink-0 rounded-full border border-zinc-800 px-2 py-1 text-[10px] ${run.status==='completed'?'text-emerald-500':run.status==='running'?'text-amber-500':'text-zinc-500'}">${escapeHTML(run.status)}</span></div>`).join(''):'<div class="p-4 text-xs text-zinc-500">No agent sessions yet. Start a CLI tool from the terminal.</div>'}catch(e){$('#homeRecent').textContent=e.message}}
$('#homeOpen').addEventListener('click',()=>openHome().catch(showError));$('#homeNewNote').addEventListener('click',()=>createNote().catch(showError));$('#homeGraph').addEventListener('click',()=>openGraph().catch(showError));$('#homeTerminal').addEventListener('click',()=>openTerminal().catch(showError));
$('#tabBar').addEventListener('click',async e=>{const close=e.target.closest('[data-close-tab]');if(close){const p=close.dataset.closeTab;if(p===state.currentPath&&state.dirty)await saveNote();state.tabs=state.tabs.filter(x=>x!==p);persistTabs();renderTabs();if(p===state.currentPath){const next=state.tabs[state.tabs.length-1];if(next)await openNote(next);else{state.currentPath='';await openHome()}}return}const tab=e.target.closest('[data-tab-path]');if(tab)try{await openNote(tab.dataset.tabPath)}catch(err){showError(err)}});

