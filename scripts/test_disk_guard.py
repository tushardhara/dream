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
