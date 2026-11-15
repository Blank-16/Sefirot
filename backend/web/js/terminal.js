let terminal = { term: null, fit: null, socket: null, id: '', token: '', closing:false };
function setTheme(theme) {
  if (theme !== 'dark' && theme !== 'light') theme = 'dark';
  document.documentElement.dataset.theme = theme;
  document.documentElement.classList.toggle('dark', theme === 'dark');
  document.documentElement.classList.toggle('light', theme === 'light');
  const meta = document.querySelector('meta[name="color-scheme"]'); if (meta) meta.content = theme;
  try { localStorage.setItem('sefirot-theme', theme); } catch {}
  if (typeof queueSettingsSync === 'function') queueSettingsSync();
  if (typeof state !== 'undefined' && state.currentPath) renderPreview();
  $('#themeToggle').textContent = theme === 'dark' ? '◐' : '◑';
}
setTheme(localStorage.getItem('sefirot-theme') || localStorage.getItem('html-vault-theme') || 'dark');
$('#themeToggle').addEventListener('click', () => setTheme(document.documentElement.dataset.theme === 'dark' ? 'light' : 'dark'));

$('#backupExport').addEventListener('click',async()=>{try{if(state.dirty)await saveNote();window.location.href='/api/export'}catch(e){showError(e)}});
$('#backupImport').addEventListener('click',()=>$('#importInput').click());
$('#importInput').addEventListener('change',async event=>{const file=event.target.files?.[0];if(!file)return;if(!confirm(`Import ${file.name}? Existing files will not be overwritten. Only import archives you trust: HTML notes may execute JavaScript in the isolated preview.`)){event.target.value='';return;}try{const response=await fetch('/api/import',{method:'POST',headers:{'Content-Type':'application/zip'},body:file});const result=await response.json().catch(()=>({}));if(!response.ok)throw new Error(result.error||`Import failed (${response.status})`);await refreshTree();$('#globalStatus').textContent=`Imported ${result.count} files · skipped ${result.skipped?.length||0}${result.indexErrors?.length?` · ${result.indexErrors.length} could not be indexed`:''}`;$('#globalStatus').classList.remove('hidden');}catch(e){showError(e)}finally{event.target.value=''}});


async function openTerminal() {
  $('#terminalOverlay').classList.remove('hidden'); $('#terminalOverlay').classList.add('flex');
  const cfg = await api('/api/terminal/config'); terminal.token = cfg.token;
  const select = $('#terminalProfile');
  [...select.options].forEach(option => {
    const profile = cfg.profiles.find(p => p.id === option.value);
    option.disabled = !!profile && profile.available !== 'true';
    option.title = option.disabled ? 'Executable not found on server PATH' : '';
  });
  if (!terminal.term) {
    if (!window.Terminal) { $('#terminalStatus').textContent = 'Terminal UI library unavailable. Check the network connection.'; $('#terminalStart').disabled=true; return; }
    $('#terminalStart').disabled=false;
    terminal.term = new Terminal({cursorBlink:true,convertEol:true,fontFamily:'ui-monospace, SFMono-Regular, Menlo, monospace',fontSize:13,theme:{background:'#000000',foreground:'#f5f5f5',cursor:'#f5f5f5',selectionBackground:'#333333'}});
    terminal.term.open($('#terminalOutput'));
    if (window.FitAddon) { terminal.fit = new FitAddon.FitAddon(); terminal.term.loadAddon(terminal.fit); terminal.fit.fit(); }
    terminal.term.onData(data => { if (terminal.socket && terminal.socket.readyState === WebSocket.OPEN) terminal.socket.send(new TextEncoder().encode(data)); });
    terminal.term.onResize(sendTerminalResize);
    terminal.term.writeln('\x1b[90mSEFIROT terminal · Select a profile and start a session.\x1b[0m');
  }
  setTimeout(() => { if (terminal.fit) terminal.fit.fit(); if (terminal.term) terminal.term.focus(); }, 30);
}
async function startTerminal() {
  try {
    if (terminal.socket) terminal.socket.close();
    if (terminal.term) terminal.term.clear();
    $('#terminalStatus').textContent = 'Starting…';
    const started = await api('/api/terminal/start', {method:'POST',headers:{'X-Vault-Token':terminal.token},body:JSON.stringify({profile:$('#terminalProfile').value})});
    terminal.id = started.id;
    const scheme = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const ws = new WebSocket(`${scheme}//${location.host}/api/terminal/ws?id=${encodeURIComponent(terminal.id)}&token=${encodeURIComponent(terminal.token)}`);
    ws.binaryType = 'arraybuffer'; terminal.socket = ws;
    ws.onopen = () => { $('#terminalStatus').textContent = 'Connected'; terminal.term.focus(); sendTerminalResize(); };
    ws.onmessage = event => { if (event.data instanceof ArrayBuffer) terminal.term.write(new Uint8Array(event.data)); else terminal.term.write(event.data); };
    ws.onclose = () => { $('#terminalStatus').textContent = 'Disconnected'; terminal.id=''; if(!terminal.closing&&state.page==='home')openHome().catch(()=>{}); };
    ws.onerror = () => { $('#terminalStatus').textContent = 'Connection error'; closeTerminal(); };
  } catch (e) { $('#terminalStatus').textContent = e.message; }
}
function sendTerminalResize() {
  if (terminal.fit) terminal.fit.fit();
  if (terminal.socket && terminal.socket.readyState === WebSocket.OPEN && terminal.term) terminal.socket.send(JSON.stringify({type:'resize',cols:terminal.term.cols,rows:terminal.term.rows}));
}
async function closeTerminal() {
  $('#terminalOverlay').classList.add('hidden'); $('#terminalOverlay').classList.remove('flex');
  const id=terminal.id;const socket=terminal.socket;terminal.closing=true;terminal.socket=null;terminal.id='';
  if(socket)socket.close();
  if(id&&terminal.token){try{await fetch('/api/terminal/stop',{method:'POST',headers:{'Content-Type':'application/json','X-Vault-Token':terminal.token},body:JSON.stringify({id})})}catch{}}
  $('#terminalStatus').textContent='Not connected';terminal.closing=false;if(state.page==='home')openHome().catch(()=>{});
}
$('#terminalOpen').addEventListener('click',()=>openTerminal().catch(showError));
$('#terminalStart').addEventListener('click',startTerminal);
function hideTerminal(){$('#terminalOverlay').classList.add('hidden');$('#terminalOverlay').classList.remove('flex')}
$('#terminalClose').addEventListener('click',hideTerminal);
$('#terminalStop').addEventListener('click',()=>closeTerminal());
$('#terminalOverlay').addEventListener('click',event=>{if(event.target===$('#terminalOverlay'))hideTerminal()});
window.addEventListener('resize',sendTerminalResize);
