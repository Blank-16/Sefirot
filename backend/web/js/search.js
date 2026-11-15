async function runSearch(query) {
  if (!query.trim()) { $('#searchResults').innerHTML='<p class="px-3 py-4 text-xs text-zinc-600">Type to search across your vault.</p>'; return; }
  try {
    let results;
    if ($('#searchMode').value === 'semantic') {
      const data = await api(`/api/semantic/search?q=${encodeURIComponent(query)}`);
      results = (data.results || []).map(item => ({...item, snippet:`Semantic similarity ${(item.similarity*100).toFixed(1)}% · ${item.summary}`}));
    } else if ($('#searchMode').value === 'hybrid') {
      const data = await api(`/api/hybrid/search?q=${encodeURIComponent(query)}`);
      results = (data.results || []).map(item => ({...item, snippet:`${item.semanticSimilarity ? `Semantic ${(item.semanticSimilarity*100).toFixed(1)}% · ` : ''}${item.snippet}`}));
    } else { results = await api(`/api/search?q=${encodeURIComponent(query)}`); }
    $('#searchResults').innerHTML = results.length ? results.map(item => `<button class="search-result block w-full rounded-lg px-3 py-2.5 text-left hover:bg-zinc-800" data-open-path="${escapeHTML(item.path)}"><div class="text-xs font-medium text-zinc-200">${escapeHTML(item.title)}</div><div class="mt-1 truncate text-[10px] text-zinc-600">${escapeHTML(item.path)}</div><div class="search-mark mt-1 line-clamp-2 text-xs leading-5 text-zinc-400">${safeSnippet(item.snippet)}</div></button>`).join('') : '<p class="px-3 py-4 text-xs text-zinc-600">No matching notes.</p>';
  } catch(e) { $('#searchResults').innerHTML=`<p class="px-3 py-4 text-xs text-red-300">${escapeHTML(e.message)}</p>`; }
}
