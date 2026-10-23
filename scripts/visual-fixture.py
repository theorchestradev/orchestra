#!/usr/bin/env python3
"""Create an isolated workspace and fake CLIs for visual QA. No real agents or gh calls."""
import json, os, pathlib, shutil, subprocess, sys, uuid
root = pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else '/tmp/orchestra-visual')
root.mkdir(parents=True, exist_ok=True)
folder = root / 'project'
folder.mkdir(exist_ok=True)
subprocess.run(['/usr/bin/git', 'init', '-q', str(folder)], check=True)
subprocess.run(['/usr/bin/git', '-C', str(folder), 'config', '--local', 'remote.origin.url', 'https://github.com/example/orchestra.git'], check=True)
bin_dir = root / 'bin'
bin_dir.mkdir(exist_ok=True)
python = shutil.which('python3')
agent = root / 'agent.py'
agent.write_text('''import os, sys, tty, uuid
# Sign-in commands only touch a marker in the profile's config folder; no account is contacted.
command, args = sys.argv[1], sys.argv[2:]
home = os.environ.get({'claude':'CLAUDE_CONFIG_DIR','codex':'CODEX_HOME'}.get(command,'')) or os.path.dirname(os.path.abspath(__file__))
marker = os.path.join(home, command + '-signed-in')
if args[:2] in (['auth','status'], ['login','status']): sys.exit(0 if os.path.exists(marker) else 1)
if args[:2] == ['auth','login'] or args[:1] == ['login']:
 open(marker,'w').write('fixture'); print('Fixture: signed in. No account was contacted.'); sys.exit(0)
identifier = next((sys.argv[i+1] for i,x in enumerate(sys.argv[:-1]) if x in ('--resume','--session-id','--session','--conversation')), str(uuid.uuid4()))
open(os.environ['ORCHESTRA_AGENT_ID_FILE'],'w').write(identifier)
open(os.environ['ORCHESTRA_STATUS_FILE'],'w').write('waiting')
tty.setraw(sys.stdin.fileno())
sys.stdout.write('\\x1b[?2004h\\x1b[32mIsolated Orchestra fixture\\x1b[0m\\r\\nNo model service is connected.\\r\\n❯ '); sys.stdout.flush()
while True:
 data = os.read(0,32768)
 if not data: break
 if b'\\x03' in data or b'\\x04' in data: break
 os.write(1,data.replace(b'\\r',b'\\r\\n'))
 if b'\\r' in data: os.write(1,b'\\r\\nFixture received terminal input.\\r\\n> ')
''')
for command in ['codex','claude','pi','vibe','opencode','agy','grok','cline','omp','kilo']:
 path=bin_dir/command
 path.write_text(f'#!/bin/sh\nexec "{python}" "{agent}" {command} "$@"\n')
 path.chmod(0o700)
shell = bin_dir/'fixture-shell'
shell.write_text('#!/bin/sh\nexec /bin/sh -c "$4"\n'); shell.chmod(0o700)
gh = bin_dir/'gh'
gh.write_text(f'''#!{python}
import datetime, json, sys
endpoint = next((x for x in sys.argv if x.startswith('repos/')), '')
if sys.argv[1:3] == ['pr','list']:
 head = sys.argv[sys.argv.index('--head')+1]
 now = datetime.datetime.now(datetime.timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ')
 run = lambda name, workflow, conclusion, job, start='2026-10-21T17:00:00Z', end='2026-10-21T17:07:15Z': {{'__typename':'CheckRun','name':name,'workflowName':workflow,'status':'COMPLETED' if conclusion else 'IN_PROGRESS','conclusion':conclusion,'detailsUrl':f'https://github.com/example/orchestra/actions/runs/7/job/{{job}}','startedAt':start,'completedAt':end if conclusion else '0001-01-01T00:00:00Z'}}
 checks = [run('changes','CI','SUCCESS',1,end='2026-10-21T17:00:11Z'), run('server-tests (ubuntu-latest)','CI','FAILURE',2), run('build','A workflow with a very long name for compact windows','',3,start=now), run('lint','CI','SUCCESS',4,end='2026-10-21T17:01:43Z'), run('app-tests (macos)','CI','SUCCESS',5,end='2026-10-21T17:05:02Z'), run('deploy-preview','CI','SKIPPED',6), {{'__typename':'StatusContext','context':'docs/preview','state':'PENDING','targetUrl':'https://example.invalid/docs'}}]
 print(json.dumps([{{'number':43,'url':'https://github.com/example/orchestra/pull/43','title':'Update session activity indicators so each running agent shows a readable status in narrow windows','state':'OPEN','isDraft':False,'headRefName':head,'headRefOid':'0'*40,'isCrossRepository':False,'reviewDecision':'CHANGES_REQUESTED','statusCheckRollup':checks}}]))
 sys.exit(0)
if endpoint.endswith('/logs'):
 print('\\n'.join([f'2026-10-21T17:00:{{i:02d}}.0000000Z step {{i}}' for i in range(40)] + ['2026-10-21T17:07:14.0000000Z --- FAIL: TestFixture (0.01s)', '2026-10-21T17:07:15.0000000Z ##[error]Process completed with exit code 1.']))
 sys.exit(0)
if '/annotations' in endpoint:
 print(json.dumps([{{'path':'internal/core/session.go','start_line':12,'end_line':12,'annotation_level':'failure','message':'Fixture failure'}}])); sys.exit(0)
if '/check-runs/' in endpoint:
 print(json.dumps({{'name':'server-tests (ubuntu-latest)','conclusion':'failure','html_url':'https://github.com/example/orchestra/actions/runs/7/job/2','output':{{'title':'Tests failed','summary':'1 test failed.'}}}})); sys.exit(0)
if '/reviews' in endpoint:
 print(json.dumps([[{{'id':7,'state':'CHANGES_REQUESTED','user':{{'login':'reviewer-with-a-long-handle'}},'submitted_at':'2026-10-21T18:00:00Z','body':'Cover the empty state.','body_html':'<p>Cover the empty state.</p>','html_url':'https://github.com/example/orchestra/pull/43#pullrequestreview-7'}},{{'id':8,'state':'APPROVED','user':{{'login':'maintainer'}},'submitted_at':'2026-10-21T19:00:00Z','body':'','html_url':'https://github.com/example/orchestra/pull/43#pullrequestreview-8'}}]])); sys.exit(0)
item = {{'number':42,'title':'Preserve terminal sessions when switching projects','state':'open','html_url':'https://github.com/example/orchestra/issues/42','updated_at':'2026-10-06T12:00:00Z','created_at':'2026-10-06T12:00:00Z','user':{{'login':'contributor'}},'labels':[{{'name':'enhancement','color':'58a6ff'}}],'assignees':[]}}
if '/releases' in endpoint: result = [{{'tag_name':'v9.0.0','name':'Orchestra 9.0.0','body':'## What changed\\n\\n* Illustrative fixture release with a long note that wraps in the update dialog by @contributor in #42','html_url':'https://github.com/example/orchestra/releases/tag/v9.0.0','prerelease':False,'draft':False,'published_at':'2026-10-06T12:00:00Z'}}]
elif '/comments' in endpoint: result = [[{{'id':1,'user':{{'login':'reviewer'}},'created_at':'2026-10-06T12:00:00Z','body':'The session owns the process.','body_html':'<p>The session owns the process.</p>'}}]]
elif '?' in endpoint: result = [[item,{{**item,'number':43,'title':'Update session activity indicators','labels':[],'pull_request':{{'html_url':'https://github.com/example/orchestra/pull/43'}}}}]] if 'state=closed' not in endpoint else [[]]
else:
 result = {{**item,'body_html':'<p>Keep processes and terminal scrollback alive when navigating between sessions.</p><pre><code>Switch projects → return to the same conversation</code></pre>'}}
 if '/pulls/' in endpoint: result.update(number=43,title='Update session activity indicators',head={{'label':'feature/status'}},base={{'label':'main'}},changed_files=3,additions=52,deletions=12)
print(json.dumps(result))
''')
gh.chmod(0o700)
pid, sid, peer, work, side = (str(uuid.uuid4()) for _ in range(5))
# Default and the Work profile's Claude account are signed in; the long-named profile is not.
profiles = [{'id':work,'name':'Work','emoji':'💼','folders':{a:str(root/'profiles'/'work'/a) for a in ('claude','codex')}},{'id':side,'name':'Side Projects and Open Source Contributions Account','folders':{a:str(root/'profiles'/'side-projects-and-open-source-contribut'/a) for a in ('claude','codex')}}]
for profile in profiles:
 for folder_path in profile['folders'].values(): pathlib.Path(folder_path).mkdir(parents=True, exist_ok=True, mode=0o700)
(root/'claude-signed-in').write_text('fixture'); (root/'codex-signed-in').write_text('fixture')
(root/'profiles'/'work'/'claude'/'claude-signed-in').write_text('fixture')
# A dirty worktree for the Changes pane: a commit, edits, a deletion, a rename and untracked files.
git = lambda *args, cwd=folder: subprocess.run(['/usr/bin/git', '-C', str(cwd), '-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid', '-c', 'commit.gpgsign=false', *args], check=True, capture_output=True)
if not (folder / 'README.md').exists():
 (folder / 'README.md').write_text('# Orchestra fixture\n\nIsolated visual QA project.\n')
 (folder / 'legacy.txt').write_text('Remove me\n')
 (folder / 'NOTES.md').write_text('Notes that move into docs/.\n' * 8)
 git('add', '-A'); git('commit', '-q', '-m', 'Fixture')
branch = 'orchestra/claude-' + sid[:8]
worktree = root / 'Worktrees' / 'project' / sid[:8]
base = git('rev-parse', 'HEAD').stdout.decode().strip()
git('worktree', 'add', '-q', '-b', branch, str(worktree), base)
(worktree / 'docs').mkdir()
git('mv', 'NOTES.md', 'docs/notes.md', cwd=worktree)
(worktree / 'CHANGELOG.md').write_text('Committed by the session.\n')
git('add', '-A', cwd=worktree); git('commit', '-q', '-m', 'Session work', cwd=worktree)
(worktree / 'README.md').write_text('# Orchestra fixture\n\nIsolated visual QA project with a dirty worktree.\n')
(worktree / 'legacy.txt').unlink()
long = worktree / 'frontend' / 'src' / 'components' / 'a-very-long-folder-name-for-compact-window-verification'
long.mkdir(parents=True)
(long / 'SessionDiff.vue').write_text('<template><aside /></template>\n')
(worktree / 'preview.png').write_bytes(b'\x89PNG\r\n\x1a\n\x00\x00')
state = {'version':1,'selectedProjectID':pid,'preferences':{'appearance':'dark','theme':'system','terminalFontSize':14,'terminalFontSmoothing':True,'terminalPaddingX':12,'terminalPaddingY':8,'sidebarWidth':280},'projects':[{'id':pid,'name':'Orchestra','directory':str(folder),'isExpanded':True,'hasScratchpad':True,'scratchpadText':'','showsScratchpad':True,'showsGitHub':False,'selectedSessionID':sid,'profileID':work,'sessions':[{'id':sid,'name':'Claude Code 1','agent':'claude','agentSessionID':sid,'hasLaunched':True,'worktreeDirectory':str(worktree),'worktreeBranch':branch,'worktreeBase':base,'profileID':work},{'id':peer,'name':'Codex 1','agent':'codex','hasLaunched':False,'profileID':side}]}],'profiles':profiles}
(root/'state.json').write_text(json.dumps(state,indent=2))
print(f'Fixture created at {root}. Launch with ORCHESTRA_STATE_FILE={root}/state.json, SHELL={shell}, and PATH={bin_dir}:$PATH.')
