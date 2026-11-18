import json, os, sys, time
from playwright.sync_api import sync_playwright
BASE=os.environ.get('SEFIROT_URL','http://127.0.0.1:18080')
SHOTS=os.environ.get('SHOT_DIR','/tmp/sefirot-shots')
os.makedirs(SHOTS, exist_ok=True)
results=[]; errors=[]
def check(name, cond, detail=''):
    results.append((name, bool(cond), str(detail)[:200]))
def main():
  with sync_playwright() as p:
      b=p.chromium.launch()
      ctx=b.new_context(viewport={'width':1440,'height':900})
      page=ctx.new_page()
      page.on('pageerror', lambda e: errors.append('pageerror: '+str(e)))
      page.on('console', lambda m: errors.append('console: '+m.text) if m.type=='error' else None)
      page.on('dialog', lambda d: d.accept())
      page.goto(BASE); page.wait_for_selector('#homeView:not(.hidden)', timeout=8000)
      page.wait_for_timeout(800)
      page.screenshot(path=SHOTS+'/home-dark-wide.png')
      check('home renders', page.locator('#homeView').is_visible())
      # create casual note
      page.click('#homeNewNote'); page.fill('#promptInput','Casual/Prices.html'); page.click('#promptConfirm')
      page.wait_for_selector('#editor', state='visible')
      page.fill('#editor','# Prices\n\nIt costs $5 and later $10 in total.\n'+'\n\n'.join(f'Paragraph {i} filler text for scrolling.' for i in range(80)))
      page.wait_for_function("document.querySelector('#saveStatus').textContent.trim()==='Saved'", timeout=8000)
      fl=page.frame_locator('#preview')
      page.wait_for_timeout(500)
      body=fl.locator('body').inner_text()
      check('casual note: no math rendering of currency', '$5 and later $10' in body and fl.locator('mjx-container').count()==0, body[:80])
      check('casual note: MathJax not loaded', page.evaluate("document.querySelector('#preview').srcdoc.indexOf('mathjax')===-1"))
      page.screenshot(path=SHOTS+'/note-casual-dark.png')
      # research mode
      page.click('#researchModeToggle')
      page.wait_for_timeout(300)
      page.fill('#editor','# Math\n\nInline \\(x^2\\) and display\n\n\\[ \\int_0^1 x\\,dx = \\tfrac12 \\]\n\n'+'\n\n'.join(f'Paragraph {i} filler text for scrolling.' for i in range(120)))
      page.wait_for_function("document.querySelector('#saveStatus').textContent.trim()==='Saved'", timeout=8000)
      try:
          fl.locator('mjx-container').first.wait_for(timeout=8000)
      except Exception as e: pass
      check('research mode: MathJax renders', fl.locator('mjx-container').count()>=2, fl.locator('mjx-container').count())
      page.screenshot(path=SHOTS+'/note-research-dark.png')
      # scroll preservation
      page.evaluate("document.querySelector('#preview').contentWindow.postMessage({type:'noop'},'*')")
      fr=[f for f in page.frames if f!=page.main_frame][0]
      fr.evaluate("window.scrollTo(0, 900)")
      page.wait_for_timeout(500)
      y_before=fr.evaluate("window.scrollY")
      page.click('#editor'); page.keyboard.press('Control+End'); page.keyboard.type(' edit')
      page.wait_for_function("document.querySelector('#saveStatus').textContent.trim()==='Saved'", timeout=8000)
      page.wait_for_timeout(1500)
      fr=[f for f in page.frames if f!=page.main_frame][0]
      y_after=fr.evaluate("window.scrollY")
      check('preview scroll preserved across autosave', y_before>500 and abs(y_after-y_before)<80, (y_before,y_after))
      # settings sync to server, survive cleared storage
      page.wait_for_timeout(1200)
      srv=page.evaluate("fetch('/api/settings').then(r=>r.json())")
      check('settings stored on server', 'Casual/Prices.html' in srv['researchDocs'] and 'Casual/Prices.html' in srv['tabs'], json.dumps(srv)[:150])
      ctx2=b.new_context(viewport={'width':1440,'height':900}); p2=ctx2.new_page()
      p2.on('dialog', lambda d: d.accept())
      p2.goto(BASE); p2.wait_for_selector('#homeView:not(.hidden)'); p2.wait_for_timeout(1000)
      check('tabs restored in fresh browser origin', p2.locator('#tabBar button[data-tab-path="Casual/Prices.html"]').count()==1)
      p2.click('#tabBar button[data-tab-path="Casual/Prices.html"]'); p2.wait_for_timeout(800)
      check('research mode restored in fresh browser', p2.locator('#researchModeToggle').get_attribute('aria-pressed')=='true')
      ctx2.close()
      # delete -> trash -> restore
      page.click('#homeOpen') if page.locator('#homeOpen').count() else None
      page.evaluate("deleteNote('Casual/Prices.html')"); page.wait_for_timeout(800)
      tree=page.evaluate("fetch('/api/tree').then(r=>r.json())")
      check('delete removes note from tree', 'Prices' not in json.dumps(tree))
      page.click('.tool-menu summary'); page.click('#trashOpen'); page.wait_for_selector('#trashList [data-trash-restore]')
      check('trash lists deleted note', 'Casual/Prices.html' in page.inner_text('#trashList'))
      page.screenshot(path=SHOTS+'/trash-dark.png')
      page.click('#trashList [data-trash-restore]'); page.wait_for_timeout(800)
      tree=page.evaluate("fetch('/api/tree').then(r=>r.json())")
      check('restore returns note to tree', 'Prices' in json.dumps(tree))
      page.click('#trashClose')
      # search keyboard navigation
      page.keyboard.press('Control+k'); page.fill('#searchInput','Math'); page.wait_for_selector('.search-result')
      page.keyboard.press('ArrowDown')
      check('search arrow navigation highlights result', page.locator('.search-result.kbd-active').count()==1)
      check('dialog semantics present', page.locator('#searchOverlay [role=dialog][aria-modal=true]').count()==1)
      page.keyboard.press('Enter'); page.wait_for_timeout(800)
      check('enter opens active result', 'Prices' in page.inner_text('#breadcrumb'))
      # integrity panel
      page.evaluate("fetch('/api/notes',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({path:'Hub.html',content:'See [[Ghost Note]] and [[Prices]]',isMarkdown:true})})")
      page.wait_for_timeout(500)
      page.evaluate("openHome()"); page.wait_for_timeout(1000)
      txt=page.inner_text('#homeIntegrity') if page.locator('#homeIntegritySection:not(.hidden)').count() else ''
      check('integrity panel reports broken link', 'Ghost Note'.lower() in txt.lower(), txt[:120])
      page.screenshot(path=SHOTS+'/home-integrity-dark.png')
      # references
      page.evaluate("openResearch()"); page.wait_for_selector('#referencesTools')
      bib="@article{ein1905, author={Einstein, Albert}, title={On the {Electrodynamics} of Moving Bodies}, journal={Annalen}, year={1905}, doi={10.1002/andp.19053221004}}\n@book{doe2001, author={Doe, Jane}, title={A Book}, year={2001}}\n"
      page.set_input_files('#refFile', files=[{'name':'refs.bib','mimeType':'text/plain','buffer':bib.encode()}])
      page.wait_for_function("document.querySelectorAll('#researchList [data-research-id]').length===2", timeout=6000)
      check('bibtex import fills library', 'Imported 2' in page.inner_text('#researchFeedback'), page.inner_text('#researchFeedback'))
      page.click('#researchList [data-research-id]')
      page.wait_for_selector('#researchProvenance:not(.hidden)')
      check('provenance shown for imported paper', 'BibTeX' in page.inner_text('#researchProvenance'), page.inner_text('#researchProvenance'))
      page.fill('#refDOI','10.1234/abcd'); page.click('#refLookup'); page.wait_for_timeout(400)
      check('DOI lookup refuses while disabled', 'Turn on DOI lookup' in page.inner_text('#researchFeedback'), page.inner_text('#researchFeedback'))
      page.fill('#researchCitationText','Some reference'); page.click('#researchAddCitation'); page.wait_for_selector('[data-delete-citation]')
      page.click('[data-delete-citation]'); page.wait_for_timeout(500)
      check('citation can be removed from the UI', page.locator('[data-delete-citation]').count()==0)
      page.screenshot(path=SHOTS+'/research-library-dark.png')
      page.evaluate("closeResearch()")
      # alias and title resolution through the real link handler
      page.evaluate("fetch('/api/notes',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({path:'deep/folder/2024-qm.html',content:'---\\naliases: [QM]\\n---\\n# Quantum Mechanics\\n\\nx',isMarkdown:true})})")
      page.wait_for_timeout(400)
      r=page.evaluate("fetch('/api/resolve?target=QM').then(r=>r.json())")
      check('alias resolves through API', r['path']=='deep/folder/2024-qm.html', r)
      # ui fixes
      page.evaluate("setTheme('dark')")
      page.keyboard.press('Control+k'); page.fill('#searchInput','Math'); page.keyboard.press('Escape')
      page.keyboard.press('Control+k'); page.wait_for_selector('#searchInput', state='visible')
      sel=page.evaluate("(()=>{const i=document.querySelector('#searchInput');return [i.selectionStart,i.selectionEnd,i.value.length]})()")
      check('search reopens without highlighting the old text', sel[0]==sel[1], sel)
      outline=page.evaluate("getComputedStyle(document.querySelector('#searchInput')).outlineStyle")
      check('search input has no outline', outline=='none', outline)
      page.fill('#searchInput','Math'); page.wait_for_selector('.search-result mark')
      markbg=page.evaluate("getComputedStyle(document.querySelector('.search-result mark')).backgroundColor")
      import re as _re
      slash=_re.search(r'/\s*([\d.]+)\)', markbg)
      parts=_re.findall(r'[\d.]+', markbg)
      alpha=float(slash.group(1)) if slash else (float(parts[3]) if len(parts)>=4 and markbg.startswith('rgba') else 1.0)
      check('search highlight is subtle', alpha<0.4, markbg)
      page.keyboard.press('Escape')
      geo=page.evaluate("(()=>{const s=document.querySelector('.tool-menu > summary'),m=document.querySelector('.tool-menu-mark');const a=s.getBoundingClientRect(),b=m.getBoundingClientRect();const r=document.createRange();r.selectNodeContents(s.firstChild.nodeType===3?s.firstChild:s.querySelector('span'));const t=r.getBoundingClientRect();return {summary:getComputedStyle(s).display,markDisplay:getComputedStyle(m).display,dy:Math.abs((b.top+b.height/2)-(a.top+a.height/2))}})()")
      check('tools chevron is centred in the button', geo['summary']=='inline-flex' and geo['markDisplay'] in ('flex','inline-flex') and geo['dy']<=2.5, geo)
      page.evaluate("openNote('Casual/Prices.html')"); page.wait_for_timeout(700)
      page.click('#researchModeToggle') if page.locator('#researchModeToggle').get_attribute('aria-pressed')!='true' else None
      page.wait_for_selector('#documentTemplate', state='visible')
      sel_style=page.evaluate("(()=>{const s=document.querySelector('#documentTemplate');const o=s.querySelector('option');return [getComputedStyle(s).backgroundColor,getComputedStyle(o).backgroundColor,getComputedStyle(s).color]})()")
      check('dark template dropdown uses the dark surface', all(c.startswith('rgb(') and int(c[4:].split(',')[0])<60 for c in sel_style[:2]), sel_style)
      # wrap
      wrap0=page.evaluate("getComputedStyle(document.querySelector('#editor')).whiteSpace")
      page.click('#editorWrapToggle')
      wrap1=page.evaluate("getComputedStyle(document.querySelector('#editor')).whiteSpace")
      check('wrap toggle switches pre-wrap and pre', wrap0=='pre-wrap' and wrap1=='pre', (wrap0,wrap1))
      check('wrap preference persisted', page.evaluate("localStorage.getItem('sefirot-editor-wrap')")=='off')
      page.wait_for_timeout(900)
      srv=page.evaluate("fetch('/api/settings').then(r=>r.json())")
      check('wrap preference stored on server', srv.get('editorNoWrap') is True, srv)
      page.click('#editorWrapToggle')
      # format
      messy="---\naliases: [QM]\n---\n#  Messy   title\n*  one\n*  two\n\nInline $x_1 * y_2$ and \\(a_b\\) end.\n\n$$\n\\sum_{i=1}^n a_i * b_i\n$$\n\n|a|b|\n|-|-|\n|1|2|\n"
      page.fill('#editor', messy)
      page.click('#editorFormat'); page.wait_for_function("document.querySelector('#editor').value.includes('# Messy title')", timeout=8000)
      out=page.input_value('#editor')
      check('format cleans Markdown structure', '- one' in out and '| a   | b   |' in out, out[:120])
      check('format leaves math byte for byte', all(m in out for m in ['$x_1 * y_2$','\\(a_b\\)','\\sum_{i=1}^n a_i * b_i']), out)
      check('format keeps front matter', out.startswith('---\naliases: [QM]\n---'), out[:30])
      page.click('#editor'); page.keyboard.press('Control+z'); page.wait_for_timeout(200)
      check('format can be undone with Ctrl+Z', page.input_value('#editor')==messy, page.input_value('#editor')[:60])
      # light theme preview of a standalone document
      page.evaluate("setTheme('light')"); page.evaluate("openNote('welcome.html')"); page.wait_for_timeout(1200)
      fr=[f for f in page.frames if f!=page.main_frame][0]
      colors=fr.evaluate("[getComputedStyle(document.body).backgroundColor,getComputedStyle(document.body).color,getComputedStyle(document.querySelector('.demo')).backgroundColor]")
      check('standalone document follows the light theme', colors[0]=='rgb(250, 249, 243)' and colors[1]=='rgb(32, 37, 31)' and colors[2]=='rgb(240, 238, 229)', colors)
      page.screenshot(path=SHOTS+'/welcome-light.png')
      page.evaluate("setTheme('dark')")
      page.evaluate("openNote('Casual/Prices.html')"); page.wait_for_timeout(500)
      # terminal hide keeps session
      page.click('.tool-menu summary'); page.click('#terminalOpen'); page.wait_for_timeout(600)
      check('wsl profile is not offered on Linux', page.locator('#terminalProfile option[value=wsl]').count()==0)
      page.select_option('#terminalProfile','shell'); page.click('#terminalStart')
      try:
          page.wait_for_function("document.querySelector('#terminalStatus').textContent==='Connected'", timeout=6000)
      except Exception: pass
      check('terminal connects', page.inner_text('#terminalStatus')=='Connected', page.inner_text('#terminalStatus'))
      page.screenshot(path=SHOTS+'/terminal-dark.png')
      page.mouse.click(5,5); page.wait_for_timeout(500)
      check('backdrop click hides overlay', not page.locator('#terminalOverlay').is_visible())
      check('backdrop click keeps session connected', page.inner_text('#terminalStatus')=='Connected', page.inner_text('#terminalStatus'))
      page.click('.tool-menu summary'); page.click('#terminalOpen'); page.wait_for_timeout(300)
      page.click('#terminalOutput'); page.keyboard.type('echo RELOAD$((6*7))'); page.keyboard.press('Enter'); page.wait_for_timeout(700)
      page.mouse.click(5,5)
      page.reload(); page.wait_for_selector('#homeView:not(.hidden)'); page.wait_for_timeout(800)
      page.click('.tool-menu summary'); page.click('#terminalOpen')
      try:
          page.wait_for_function("document.querySelector('#terminalStatus').textContent.startsWith('Reconnected')", timeout=8000)
      except Exception: pass
      check('terminal reattaches after a page reload', page.inner_text('#terminalStatus').startswith('Reconnected'), page.inner_text('#terminalStatus'))
      page.wait_for_timeout(600)
      check('earlier terminal output is replayed', 'RELOAD42' in page.inner_text('#terminalOutput'), page.inner_text('#terminalOutput')[:200])
      page.click('#terminalStop'); page.wait_for_timeout(600)
      # screenshots: sizes x themes
      for theme in ['dark','light']:
          page.evaluate(f"setTheme('{theme}')")
          for name,(w,h) in {'wide':(1600,900),'compact':(1100,720),'tablet':(820,1100),'mobile':(390,844)}.items():
              page.set_viewport_size({'width':w,'height':h}); page.wait_for_timeout(300)
              page.evaluate("openNote('Casual/Prices.html')"); page.wait_for_timeout(900)
              page.screenshot(path=f'{SHOTS}/note-{theme}-{name}.png')
              page.evaluate("openHome()"); page.wait_for_timeout(700)
              page.screenshot(path=f'{SHOTS}/home-{theme}-{name}.png')
      b.close()
try:
  main()
except Exception as e:
  results.append(('SCRIPT ABORT', False, str(e)[:300]))
for n,ok,d in results: print(('PASS ' if ok else 'FAIL ')+n+('' if ok else '  :: '+d))
print('JS errors:', len(errors)); [print('  ',e[:200]) for e in errors[:8]]
sys.exit(1 if errors or any(not ok for _, ok, _ in results) else 0)
