"""Foreground, cooperative resource guard; NOT a supervisor or filesystem quota.
Run every local heavy command through this entry point. Never deletes worktrees.
"""
import argparse
import fcntl
import json
import math
import os
import pathlib
import shutil
import signal
import subprocess
import tempfile
import threading
import time
import uuid

GIB = 1024 ** 3


def disk_failure(stats):
    total, free, available, inodes, ifree = stats
    if available < 30 * GIB or math.ceil(100 * (total - free) / (total - free + available)) >= 90:
        return 'disk below 30 GiB available or at least 90% used'
    if not inodes or math.ceil(100 * (inodes - ifree) / inodes) >= 90:
        return 'inode usage at least 90% or unavailable'
    return None


def preflight(paths, role=None):
    for path in paths:
        s = os.statvfs(path)
        problem = disk_failure((s.f_blocks*s.f_frsize, s.f_bfree*s.f_frsize,
                                s.f_bavail*s.f_frsize, s.f_files, s.f_ffree))
        if problem:
            raise RuntimeError(str(path) + ': ' + problem)
    size = int(subprocess.check_output(['du', '-scB1', *map(str, paths)], text=True,
                                      timeout=30).splitlines()[-1].split()[0])
    if role and shutil.which('docker'):
        images = subprocess.check_output(['docker', 'images', '-q', '--filter', 'label=dream.test.role='+role], text=True, timeout=30).split()
        for image in set(images):
            size += int(subprocess.check_output(['docker', 'image', 'inspect', '--format={{.Size}}', image], text=True, timeout=30))
    if size >= 10 * GIB:
        raise RuntimeError('role accounted footprint reached 10 GiB')
    return size


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--role', required=True, choices=['codex', 'claude'])
    parser.add_argument('--account', action='append', default=[], help='additional existing role footprint, including old caches/clones; never cleaned')
    parser.add_argument('command', nargs=argparse.REMAINDER)
    args = parser.parse_args()
    command = args.command
    if command[:1] == ['--']:
        command = command[1:]
    if not command:
        parser.error('command required')
    root = pathlib.Path.home()/'.local/state/dream-tests'/args.role
    root.mkdir(parents=True, exist_ok=True, mode=0o700)
    # Same role cannot run two guarded commands, including from another checkout.
    with (root/'exclusive.lock').open('a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        work = pathlib.Path.cwd().resolve()
        paths = [root, work, pathlib.Path.home()/'go'] + [pathlib.Path(p).resolve() for p in args.account]
        # Avoid counting descendants twice; shared caches are conservatively charged.
        paths = [p for p in dict.fromkeys(paths) if not any(q != p and q in p.parents for q in paths)]
        preflight(paths, args.role)
        cache = root/'go-build'
        cache.mkdir(exist_ok=True)
        (root/'tool-cache').mkdir(exist_ok=True)
        run = args.role + '-' + uuid.uuid4().hex
        env = dict(os.environ, GOCACHE=str(cache), DREAM_TEST_ROLE=args.role, DREAM_TEST_RUN=run, DOCKER_BUILDKIT="0", XDG_CACHE_HOME=str(root/"tool-cache"))
        interrupted = threading.Event()
        for sig in (signal.SIGINT, signal.SIGTERM):
            signal.signal(sig, lambda *_: interrupted.set())
        # Bounded logs remain outside disposable scratch and rotate on every run.
        for i in range(3, 0, -1):
            old = root/('output.log' if i == 1 else f'output.log.{i-1}')
            if old.exists():
                old.replace(root/f'output.log.{i}')
        with tempfile.TemporaryDirectory(prefix=run+'-', dir=root) as scratch:
            env.update(TMPDIR=scratch, GOTMPDIR=scratch)
            process = None
            failure = None
            code = 1
            try:
                process = subprocess.Popen(command, env=env, start_new_session=True,
                                           stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
                def record():
                    with (root/'output.log').open('wb') as log:
                        count = 0
                        while chunk := process.stdout.read1(8192):
                            if count + len(chunk) > 2*1024*1024:
                                log.seek(0); log.truncate(); count = 0
                            log.write(chunk); log.flush(); count += len(chunk)
                reader = threading.Thread(target=record, daemon=True)
                reader.start()
                next_check = time.monotonic()
                while process.poll() is None:
                    if interrupted.is_set():
                        raise RuntimeError('interrupted')
                    if time.monotonic() >= next_check:
                        size = preflight(paths, args.role)
                        (root/'checkpoint.json').write_text(json.dumps({'run':run,'pid':process.pid,'accounted_bytes':size,'checked_at':time.time(),'status':'running','command':command}))
                        next_check = time.monotonic() + 60
                    interrupted.wait(1)
                code = process.returncode
                reader.join(timeout=5)
            except (RuntimeError, OSError, subprocess.SubprocessError) as error:
                failure = str(error)
            finally:
                if process is not None and process.poll() is None:
                    os.killpg(process.pid, signal.SIGTERM)
                    try:
                        process.wait(timeout=20)
                    except subprocess.TimeoutExpired:
                        os.killpg(process.pid, signal.SIGKILL)
                        process.wait(timeout=10)
                # Only exact unique-run labels, never global prune or name guesses.
                if shutil.which('docker'):
                    for kind, listing, removal in [('container','ps','rm'), ('image','images','rmi')]:
                        found = subprocess.run(['docker',listing,'-aq','--filter','label=dream.test.run='+run], capture_output=True,text=True,timeout=30)
                        for item in set(found.stdout.split()):
                            cmd = ['docker',removal] + (['--force'] if kind == 'container' else []) + [item]
                            subprocess.run(cmd,capture_output=True,timeout=30)
                (root/'checkpoint.json').write_text(json.dumps({'run':run,'status':'blocked' if failure else 'finished','exit_code':code,'blocker':failure,'finished_at':time.time()}))
            print('Guard:', failure or ('exit '+str(code)), 'log='+str(root/'output.log'), flush=True)
            return 1 if failure else code


if __name__ == '__main__':
    raise SystemExit(main())
