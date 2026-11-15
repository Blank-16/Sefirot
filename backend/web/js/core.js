'use strict';

const $ = (selector, root = document) => root.querySelector(selector);
const activeVaultPath = document.querySelector('meta[name="vault-path"]')?.content;
if (activeVaultPath) { const label = $('#vaultPathLabel'); const terminalLabel = $('#terminalCwdLabel'); if (label) { label.textContent = activeVaultPath; label.title = activeVaultPath; } if (terminalLabel) terminalLabel.textContent = activeVaultPath; }
const loadTabs = () => { try { const value = JSON.parse(localStorage.getItem('sefirot-tabs') || localStorage.getItem('html-vault-tabs') || '[]'); return Array.isArray(value) ? value.filter(v => typeof v === 'string') : []; } catch { return []; } };
const state = {
  tree: [], currentPath: '', content: '', renderedHtml: '', isMarkdown: true, contentHash:'', externalConflict:false, tabs: loadTabs(), headings:[],
  view: 'split', researchMode: false, dirty: false, saving: false, saveTimer: null, searchTimer: null,
  expanded: new Set(['']), rightOpen: true, promptResolve: null, savePromise: null, graph: {nodes:[],edges:[],selected:'',query:'',positions:new Map(),panX:0,panY:0,scale:1,scope:'global',drag:null,lastPointer:null}, page: 'notes',
};

const escapeHTML = value => String(value ?? '').replace(/[&<>"']/g, ch => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch]));
const api = async (url, options = {}) => {
  const response = await fetch(url, {headers:{'Content-Type':'application/json', ...(options.headers || {})}, ...options});
  const data = await response.json().catch(() => ({}));
  if (!response.ok) { const error=new Error(data.error || `Request failed (${response.status})`); error.status=response.status; throw error; }
  return data;
};
const setStatus = (text, color = 'text-zinc-500') => {
  const el = $('#saveStatus');
  el.textContent = text;
  el.className = 'save-status';
  el.dataset.state = color.includes('red') ? 'failed' : color.includes('amber') ? 'saving' : color.includes('emerald') ? 'saved' : 'idle';
};
const basename = p => p.split('/').pop() || p;
const withoutExt = p => p.replace(/\.html$/i, '');

