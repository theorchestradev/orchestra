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
for command in ['codex','claude','pi','vibe','opencode','agy','grok']:
 path=bin_dir/command
 path.write_text(f'#!/bin/sh\nexec "{python}" "{agent}" "$@"\n')
 path.chmod(0o700)
shell = bin_dir/'fixture-shell'
shell.write_text('#!/bin/sh\nexec /bin/sh -c "$4"\n'); shell.chmod(0o700)
gh = bin_dir/'gh'
gh.write_text(f'''#!{python}
import json, sys
endpoint = next((x for x in sys.argv if x.startswith('repos/')), '')
item = {{'number':42,'title':'Preserve terminal sessions when switching projects','state':'open','html_url':'https://github.com/example/orchestra/issues/42','updated_at':'2026-10-06T12:00:00Z','created_at':'2026-10-06T12:00:00Z','user':{{'login':'contributor'}},'labels':[{{'name':'enhancement','color':'58a6ff'}}],'assignees':[]}}
if '/releases' in endpoint: result = [{{'tag_name':'v9.0.0','name':'Orchestra 9.0.0','body':'## What changed\\n\\n* Illustrative fixture release with a long note that wraps in the update dialog by @contributor in #42','html_url':'https://github.com/example/orchestra/releases/tag/v9.0.0','prerelease':False,'draft':False,'published_at':'2026-10-06T12:00:00Z'}}]
elif '/comments' in endpoint: result = [[{{'id':1,'user':{{'login':'reviewer'}},'created_at':'2026-10-06T12:00:00Z','body_html':'<p>The session owns the process.</p>'}}]]
elif '?' in endpoint: result = [[item,{{**item,'number':43,'title':'Update session activity indicators','labels':[],'pull_request':{{'html_url':'https://github.com/example/orchestra/pull/43'}}}}]] if 'state=closed' not in endpoint else [[]]
else:
 result = {{**item,'body_html':'<p>Keep processes and terminal scrollback alive when navigating between sessions.</p><pre><code>Switch projects → return to the same conversation</code></pre>'}}
 if '/pulls/' in endpoint: result.update(number=43,title='Update session activity indicators',head={{'label':'feature/status'}},base={{'label':'main'}},changed_files=3,additions=52,deletions=12)
print(json.dumps(result))
''')
gh.chmod(0o700)
pid, sid, peer = (str(uuid.uuid4()) for _ in range(3))
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
state = {'version':1,'selectedProjectID':pid,'preferences':{'appearance':'dark','theme':'system','terminalFontSize':14,'terminalFontSmoothing':True,'terminalPaddingX':12,'terminalPaddingY':8,'sidebarWidth':280},'projects':[{'id':pid,'name':'Orchestra','directory':str(folder),'isExpanded':True,'hasScratchpad':True,'scratchpadText':'','showsScratchpad':True,'showsGitHub':False,'selectedSessionID':sid,'sessions':[{'id':sid,'name':'Claude Code 1','agent':'claude','agentSessionID':sid,'hasLaunched':True,'worktreeDirectory':str(worktree),'worktreeBranch':branch,'worktreeBase':base},{'id':peer,'name':'Codex 1','agent':'codex','hasLaunched':False}]}]}
(root/'state.json').write_text(json.dumps(state,indent=2))
print(f'Fixture created at {root}. Launch with ORCHESTRA_STATE_FILE={root}/state.json, SHELL={shell}, and PATH={bin_dir}:$PATH.')
