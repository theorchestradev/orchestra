#!/usr/bin/env python3
"""Exercise demo packaging, persistent fixtures and clean launch environments without a UI."""
import fcntl
import json
import os
from pathlib import Path
import plistlib
import pty
import select
import time
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

sys.dont_write_bytecode = True
import demo


class DemoSmoke(unittest.TestCase):
    def test_demo_isolation_and_persistence(self):
        with tempfile.TemporaryDirectory(prefix='orchestra-demo-test-') as temporary:
            root = Path(temporary)
            repo = root / "worktree with spaces and 'quotes'"
            (repo / 'scripts').mkdir(parents=True)
            shutil.copy2(Path(__file__).with_name('visual-fixture.py'), repo / 'scripts/visual-fixture.py')
            data = root / 'isolated data'
            real_home = root / 'real-home'
            real_home.mkdir()
            sentinel = real_home / 'settings.json'
            sentinel.write_text('original')
            parent = {'HOME': str(real_home), 'CODEX_HOME': str(real_home / '.codex'),
                      'ORCHESTRA_STATE_FILE': str(real_home / 'state.json'),
                      'ORCHESTRA_SESSION_ID': 'production-session',
                      'ORCHESTRA_SOCKET': 'production-socket', 'OPENAI_API_KEY': 'not-a-real-key',
                      'GH_TOKEN': 'not-a-real-token'}
            with patch.dict(os.environ, parent):
                env, locks = demo.prepare_data(repo, data)
                try:
                    state = data / 'state.json'
                    saved = json.loads(state.read_text())
                    saved['preferences']['hideStatusBar'] = True
                    state.write_text(json.dumps(saved))
                    before = state.read_bytes()
                    with self.assertRaisesRegex(RuntimeError, 'demo is running'):
                        demo.prepare_data(repo, data)
                finally:
                    for lock in locks:
                        lock.close()
                # Rebuilding preserves state and the sample repositories, rather than regenerating them.
                env, locks = demo.prepare_data(repo, data)
                for lock in locks:
                    lock.close()
                self.assertEqual(state.read_bytes(), before)
                original = repo / 'build/bin/Orchestra.app'
                executable = original / 'Contents/MacOS/orchestra'
                executable.parent.mkdir(parents=True)
                # The stand-in binary reveals the environment the packaged Finder launcher passes.
                executable.write_text(f'#!{sys.executable}\nimport json,os,sys\nprint(json.dumps(dict(env=dict(os.environ), args=sys.argv[1:])))\n')
                executable.chmod(0o700)
                info = {'CFBundleExecutable': 'orchestra', 'CFBundleIdentifier': 'dev.theorchestra.Orchestra',
                        'CFBundleName': 'Orchestra', 'CFBundleURLTypes': [{'CFBundleURLName': 'production'}]}
                (original / 'Contents/Info.plist').write_bytes(plistlib.dumps(info).split(b'\n', 1)[1])
                original_bytes = executable.read_bytes()
                _, name, bundle_id = demo.demo_paths(repo)
                # Both macOS and Linux launchers use the same tested clean-environment boundary.
                for platform in ('darwin', 'linux'):
                    if platform == 'linux':
                        shutil.copy2(executable, repo / 'build/bin/orchestra')
                    with patch.object(demo.sys, 'platform', platform), patch.object(demo.subprocess, 'run') as signing:
                        app = demo.package_demo(repo, data, name, bundle_id, env)
                        if platform == 'darwin':
                            self.assertEqual(signing.call_count, 2)
                    launcher = app
                    if platform == 'darwin':
                        packaged = plistlib.loads((app / 'Contents/Info.plist').read_bytes())
                        self.assertEqual(packaged['CFBundleIdentifier'], bundle_id)
                        self.assertNotEqual(packaged['CFBundleIdentifier'], info['CFBundleIdentifier'])
                        self.assertNotIn('CFBundleURLTypes', packaged)
                        launcher = app / 'Contents/MacOS/orchestra'
                    result = subprocess.run([str(launcher), 'argument with spaces'], check=True, capture_output=True, text=True)
                    observed = json.loads(result.stdout)
                    self.assertEqual(observed['args'], ['argument with spaces'])
                    self.assertEqual(observed['env']['HOME'], str(data / 'home'))
                    self.assertEqual(observed['env']['ORCHESTRA_STATE_FILE'], str(data / 'state.json'))
                    self.assertEqual(observed['env']['CODEX_HOME'], str(data / 'home/.codex'))
                    for key in ('GH_TOKEN', 'OPENAI_API_KEY', 'ORCHESTRA_SOCKET', 'ORCHESTRA_SESSION_ID'):
                        self.assertNotIn(key, observed['env'])
                self.assertEqual(executable.read_bytes(), original_bytes)
                self.assertEqual(plistlib.loads((original / 'Contents/Info.plist').read_bytes(), fmt=plistlib.FMT_XML), info)
                self.assertEqual(sentinel.read_text(), 'original')
                self.assertEqual(list(real_home.iterdir()), [sentinel])
                # The fixture's shell resolves agents in its own bin and never reads login-shell rc files.
                result = subprocess.run([env['SHELL'], '-l', '-i', '-c', 'codex --version'], env=env,
                                        check=True, capture_output=True, text=True)
                self.assertIn('demo CLI', result.stdout)
                self.assertTrue(subprocess.run([env['SHELL'], '-l', '-i', '-c', 'npx anything'], env=env,
                                               check=True, capture_output=True).stdout)

                # Exercise the simulated agent through a real PTY, including raw input and status files.
                master, slave = pty.openpty()
                agent_env = {**env, 'ORCHESTRA_AGENT_ID_FILE': str(data / 'agent-id'),
                             'ORCHESTRA_STATUS_FILE': str(data / 'agent-status')}
                process = subprocess.Popen([env['SHELL'], '-l', '-i', '-c', 'codex'],
                                           env=agent_env, stdin=slave, stdout=slave, stderr=slave)
                os.close(slave)
                def read_until(expected):
                    output = b''
                    deadline = time.monotonic() + 5
                    while expected not in output and time.monotonic() < deadline:
                        if select.select([master], [], [], 0.1)[0]:
                            output += os.read(master, 65536)
                    self.assertIn(expected, output)
                    return output
                try:
                    output = read_until(b'No model service is connected.')
                    self.assertIn(b'\x1b[32m', output)
                    self.assertNotIn(b'\\x1b', output)
                    self.assertEqual((data / 'agent-status').read_text(), 'waiting')
                    os.write(master, b'hello\r')
                    read_until(b'Fixture received terminal input.')
                    os.write(master, b'\x03')
                    self.assertEqual(process.wait(timeout=5), 0)
                finally:
                    if process.poll() is None:
                        process.kill()
                        process.wait()
                    os.close(master)

    def test_worktree_identity_and_unsafe_data_paths(self):
        with tempfile.TemporaryDirectory(prefix='orchestra-demo-test-') as temporary:
            root = Path(temporary)
            self.assertNotEqual(demo.demo_paths(root / 'one')[2], demo.demo_paths(root / 'two')[2])
            self.assertEqual(demo.demo_paths(root / 'one'), demo.demo_paths(root / 'one'))
            target = root / 'target'
            target.mkdir()
            linked = root / 'linked'
            linked.symlink_to(target)
            with self.assertRaisesRegex(RuntimeError, 'symlink'):
                demo.prepare_data(root, linked)
            data = root / 'data'
            (data / 'host').mkdir(parents=True)
            with (data / 'host/host.lock').open('a') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                with self.assertRaisesRegex(RuntimeError, 'background sessions'):
                    demo.prepare_data(root, data)


if __name__ == '__main__':
    unittest.main()
