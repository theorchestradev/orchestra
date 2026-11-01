#!/usr/bin/env python3
"""Package the current build as a per-worktree demo with an isolated launcher."""
import argparse
import fcntl
import hashlib
import os
from pathlib import Path
import plistlib
import re
import shlex
import shutil
import subprocess
import sys


def demo_paths(repo):
    repo = repo.resolve()
    identity = hashlib.sha256(os.fsencode(repo)).hexdigest()[:12]
    label = re.sub(r'[^a-zA-Z0-9._-]', '-', repo.name) or 'worktree'
    # Unix sockets need a short path; the OS may clear this state with other temporary files.
    data = Path('/tmp') / f'orchestra-demo-{os.getuid()}-{identity}'
    return data, f'Orchestra Demo - {label}', f'dev.theorchestra.Orchestra.demo.w{identity}'


def demo_environment(data):
    home = data / 'home'
    env = {
        'HOME': str(home),
        'PATH': str(data / 'bin') + ':/usr/bin:/bin:/usr/sbin:/sbin',
        'SHELL': str(data / 'bin' / 'fixture-shell'),
        'ORCHESTRA_STATE_FILE': str(data / 'state.json'),
        'TMPDIR': str(data / 'tmp') + '/',
        'XDG_CONFIG_HOME': str(home / '.config'),
        'XDG_CACHE_HOME': str(home / '.cache'),
        'XDG_DATA_HOME': str(home / '.local' / 'share'),
        'XDG_STATE_HOME': str(home / '.local' / 'state'),
        'XDG_RUNTIME_DIR': str(data / 'runtime'),
        'CODEX_HOME': str(home / '.codex'),
        'CLAUDE_CONFIG_DIR': str(home / '.claude'),
        'CLINE_DIR': str(home / '.cline'),
        'GH_CONFIG_DIR': str(home / '.config' / 'gh'),
        'GIT_CONFIG_GLOBAL': '/dev/null',
        'GIT_CONFIG_NOSYSTEM': '1',
        'LANG': 'en_US.UTF-8',
    }
    # Linux needs its display/session bus to open a window. No agent or account env is inherited.
    for key in ('DISPLAY', 'WAYLAND_DISPLAY', 'DBUS_SESSION_BUS_ADDRESS', 'XAUTHORITY'):
        if key in os.environ:
            env[key] = os.environ[key]
    if env.get('WAYLAND_DISPLAY') and not Path(env['WAYLAND_DISPLAY']).is_absolute():
        runtime = os.environ.get('XDG_RUNTIME_DIR', '')
        if not runtime:
            raise RuntimeError('Wayland requires XDG_RUNTIME_DIR to locate its display socket.')
        env['WAYLAND_DISPLAY'] = str(Path(runtime) / env['WAYLAND_DISPLAY'])
    return env


def prepare_data(repo, data):
    if data.is_symlink():
        raise RuntimeError(f'Refusing a symlink for demo data: {data}')
    data.mkdir(mode=0o700, parents=True, exist_ok=True)
    if data.stat().st_uid != os.getuid():
        raise RuntimeError(f'Demo data belongs to another user: {data}')
    data.chmod(0o700)
    env = demo_environment(data)
    for key in ('HOME', 'TMPDIR', 'XDG_CONFIG_HOME', 'XDG_CACHE_HOME', 'XDG_DATA_HOME',
                'XDG_STATE_HOME', 'XDG_RUNTIME_DIR', 'CODEX_HOME', 'CLAUDE_CONFIG_DIR',
                'CLINE_DIR', 'GH_CONFIG_DIR'):
        Path(env[key]).mkdir(mode=0o700, parents=True, exist_ok=True)
    # Keep both locks through packaging. Never replace an executable used by a demo or its host.
    locks = []
    try:
        for relative in ('run/app.lock', 'host/host.lock'):
            path = data / relative
            path.parent.mkdir(mode=0o700, exist_ok=True)
            lock = path.open('a')
            locks.append(lock)
            try:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            except BlockingIOError:
                raise RuntimeError('This worktree demo is running. Quit it and stop its background sessions before rebuilding.') from None
        fresh = not (data / 'state.json').exists()
        subprocess.run([sys.executable, str(repo / 'scripts/visual-fixture.py'), str(data)] +
                       ([] if fresh else ['--tools-only']), env=env, check=True)
        if fresh:
            for agent, key in (('codex', 'CODEX_HOME'), ('claude', 'CLAUDE_CONFIG_DIR')):
                (Path(env[key]) / (agent + '-signed-in')).write_text('fixture')
        # Skill installation is simulated too; never invoke a globally installed npx.
        npx = data / 'bin' / 'npx'
        npx.write_text('#!/bin/sh\nprintf "%s\\n" "Demo: simulated skill installation. No packages were downloaded."\n')
        npx.chmod(0o700)
        return env, locks
    except BaseException:
        for lock in locks:
            lock.close()
        raise


def write_launcher(path, binary, env, data):
    # env -i protects Finder/Desktop launches too, regardless of the parent process's environment.
    assignments = ' '.join(shlex.quote(f'{key}={value}') for key, value in env.items())
    path.write_text('#!/bin/sh\nset -eu\n' +
                    f'cd {shlex.quote(str(data))}\n' +
                    f'exec /usr/bin/env -i {assignments} {shlex.quote(str(binary))} "$@"\n')
    path.chmod(0o700)


def package_demo(repo, data, name, bundle_id, env):
    output = repo / 'build/bin/demo'
    output.mkdir(parents=True, exist_ok=True)
    if sys.platform == 'darwin':
        app = output / (name + '.app')
        # Only this worktree's generated demo bundle is replaced; data lives separately.
        if app.exists():
            shutil.rmtree(app)
        shutil.copytree(repo / 'build/bin/Orchestra.app', app)
        plist = app / 'Contents/Info.plist'
        info = plistlib.loads(plist.read_bytes(), fmt=plistlib.FMT_XML)
        executable = app / 'Contents/MacOS' / info['CFBundleExecutable']
        binary = executable.with_name('orchestra-demo-bin')
        executable.rename(binary)
        write_launcher(executable, binary, env, data)
        info.update(CFBundleName=name, CFBundleDisplayName=name, CFBundleIdentifier=bundle_id)
        # A demo never claims production URL or document associations.
        info.pop('CFBundleURLTypes', None)
        info.pop('CFBundleDocumentTypes', None)
        plist.write_bytes(plistlib.dumps(info))
        subprocess.run(['/usr/bin/codesign', '--force', '--deep', '--sign', '-', str(app)], check=True)
        subprocess.run(['/usr/bin/codesign', '--verify', '--deep', '--strict', str(app)], check=True)
        return app
    launcher = output / 'orchestra-demo'
    binary = output / 'orchestra-demo-bin'
    shutil.copy2(repo / 'build/bin/orchestra', binary)
    write_launcher(launcher, binary, env, data)
    return launcher


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--no-launch', action='store_true')
    args = parser.parse_args()
    repo = Path(__file__).resolve().parent.parent
    data, name, bundle_id = demo_paths(repo)
    env, locks = prepare_data(repo, data)
    try:
        app = package_demo(repo, data, name, bundle_id, env)
    finally:
        for lock in locks:
            lock.close()
    print(f'Demo: {app}\nIsolated data: {data}', flush=True)
    if not args.no_launch:
        if sys.platform == 'darwin':
            subprocess.run(['/usr/bin/open', str(app)], check=True)
        else:
            with (data / 'launcher.log').open('ab') as log:
                subprocess.Popen([str(app)], stdin=subprocess.DEVNULL, stdout=log,
                                 stderr=log, start_new_session=True)


if __name__ == '__main__':
    try:
        main()
    except (RuntimeError, subprocess.CalledProcessError) as error:
        sys.exit(f'Demo failed: {error}')
