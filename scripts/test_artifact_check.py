import pathlib
import subprocess
import sys
import tempfile
import unittest

SCRIPT = pathlib.Path(__file__).with_name("artifact-check.py").resolve()

class ArtifactGateTest(unittest.TestCase):
    def run_gate(self, mode, filename=None, content=""):
        with tempfile.TemporaryDirectory() as root:
            for directory in ("api/proto", "migrations"):
                pathlib.Path(root, directory).mkdir(parents=True)
            if filename:
                p = pathlib.Path(root, filename)
                p.parent.mkdir(parents=True, exist_ok=True)
                p.write_text(content)
            return subprocess.run([sys.executable, str(SCRIPT), mode], cwd=root,
                                  capture_output=True, text=True)

    def test_empty_is_explicitly_not_applicable(self):
        for mode in ("generated", "migrations"):
            result = self.run_gate(mode)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertIn("N/A:", result.stdout)

    def test_new_inputs_require_real_validation(self):
        for mode, path, content in (
            ("generated", "api/proto/service.proto", 'syntax = "proto3";'),
            ("generated", "api/gen/service.pb.go", "package gen"),
            ("generated", "core/generate.go", "//go:generate echo surprise"),
            ("migrations", "migrations/001.sql", "SELECT 1;"),
        ):
            with self.subTest(path=path):
                result = self.run_gate(mode, path, content)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("BLOCKED:", result.stderr)
