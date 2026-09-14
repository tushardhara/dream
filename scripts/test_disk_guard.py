import importlib.util
import pathlib
import unittest
spec = importlib.util.spec_from_file_location('guard', pathlib.Path(__file__).with_name('disk-guard.py'))
guard = importlib.util.module_from_spec(spec)
spec.loader.exec_module(guard)

class DiskGuardTest(unittest.TestCase):
    def test_thresholds(self):
        gib = guard.GIB
        self.assertIsNone(guard.disk_failure((400*gib, 50*gib, 49*gib, 1000, 200)))
        for stats in [(400*gib, 50*gib, 29*gib, 1000, 200),
                      (400*gib, 40*gib, 40*gib, 1000, 200),
                      (400*gib, 43*gib, 43*gib, 1000, 200),
                      (400*gib, 50*gib, 49*gib, 1000, 109),
                      (400*gib, 50*gib, 49*gib, 1000, 100)]:
            self.assertIsNotNone(guard.disk_failure(stats))

    def test_preflight_denial_is_durable_and_never_launches(self):
        import contextlib
        import io
        import json
        import tempfile
        from unittest import mock
        with tempfile.TemporaryDirectory() as directory:
            home = pathlib.Path(directory)
            with mock.patch.object(pathlib.Path, 'home', return_value=home), mock.patch('sys.argv', ['disk-guard.py','--role','codex','--','never-launch']), mock.patch.object(guard, 'preflight', side_effect=RuntimeError('disk below 30 GiB')), mock.patch.object(guard.subprocess, 'Popen') as launch, contextlib.redirect_stdout(io.StringIO()):
                self.assertEqual(guard.main(), 1)
                launch.assert_not_called()
            record = json.loads((home/'.local/state/dream-tests/codex/checkpoint.json').read_text())
            self.assertEqual(record['status'], 'blocked')
            self.assertEqual(record['phase'], 'preflight')
            self.assertEqual(record['blocker'], 'disk below 30 GiB')

    def test_lock_denial_preserves_active_checkpoint(self):
        import contextlib
        import io
        import json
        import tempfile
        from unittest import mock
        with tempfile.TemporaryDirectory() as directory:
            home = pathlib.Path(directory)
            root = home/'.local/state/dream-tests/codex'
            root.mkdir(parents=True)
            active = '{"run":"first","status":"running"}'
            (root/'checkpoint.json').write_text(active)
            with (root/'exclusive.lock').open('a') as lock:
                guard.fcntl.flock(lock, guard.fcntl.LOCK_EX | guard.fcntl.LOCK_NB)
                with mock.patch.object(pathlib.Path, 'home', return_value=home), mock.patch('sys.argv', ['disk-guard.py','--role','codex','--','never-launch']), mock.patch.object(guard.subprocess, 'Popen') as launch, contextlib.redirect_stdout(io.StringIO()):
                    self.assertEqual(guard.main(), 1)
                    launch.assert_not_called()
            self.assertEqual((root/'checkpoint.json').read_text(), active)
            denied = json.loads((root/'denied.json').read_text())
            self.assertEqual(denied['status'], 'blocked')
            self.assertEqual(denied['phase'], 'lock')

    def test_unavailable_docker_inventory_fails_closed(self):
        from types import SimpleNamespace
        from unittest import mock
        gib = guard.GIB
        stats = SimpleNamespace(f_blocks=400*gib, f_bfree=100*gib, f_bavail=100*gib,
                                f_frsize=1, f_files=1000, f_ffree=900)
        failure = guard.subprocess.CalledProcessError(1, ['docker','images'])
        with mock.patch.object(guard.os, 'statvfs', return_value=stats), mock.patch.object(guard.shutil, 'which', return_value='docker'), mock.patch.object(guard.subprocess, 'check_output', side_effect=['0 total\n', failure]):
            with self.assertRaisesRegex(RuntimeError, 'Docker footprint inventory unavailable'):
                guard.preflight([pathlib.Path('.')], 'codex')
