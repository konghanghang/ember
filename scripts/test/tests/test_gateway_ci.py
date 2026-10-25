"""Verify Gateway CI runners with fake commands and no service or network access."""

import fnmatch
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import unittest


REPO_ROOT = Path(__file__).resolve().parents[3]
RACE_PACKAGES = {
    "./internal/playbackgateway",
    "./internal/services/directplay",
    "./internal/services/p115account",
    "./internal/services/p115quota",
    "./internal/integrations/p115",
}


class RunnerTests(unittest.TestCase):
    """Exercise command selection, failure propagation, and isolated output paths."""

    def setUp(self):
        """Create disposable fake executables instead of invoking Python tests or Go."""
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.bin_dir = self.root / "bin"
        self.bin_dir.mkdir()
        self.record = self.root / "commands.jsonl"
        self.results = self.root / "results"
        for name in ("python3", "python3.11", "selected-python", "go"):
            executable = self.bin_dir / name
            executable.write_text(
                "#!" + sys.executable + "\n"
                "import json, os, sys\n"
                "from pathlib import Path\n"
                "with open(os.environ['FAKE_COMMAND_RECORD'], 'a') as output:\n"
                "    output.write(json.dumps({'command': Path(sys.argv[0]).name, "
                "'args': sys.argv[1:], 'database': os.environ.get('EMBER_INTEGRATION_DATABASE_URL')}) + '\\n')\n"
                "print('synthetic command output', flush=True)\n"
                "code = os.environ.get('FAKE_EXIT_CODE', '0')\n"
                "if any(arg.endswith('/scripts/test/tests') for arg in sys.argv):\n"
                "    code = os.environ.get('FAKE_RUNNER_EXIT_CODE', code)\n"
                "sys.exit(int(code))\n",
                encoding="utf-8",
            )
            executable.chmod(0o700)

    def run_suite(self, suite, **overrides):
        """Run one real shell entrypoint while all language commands are fake."""
        environment = os.environ.copy()
        environment.pop("PYTHON", None)
        environment.update({
            "PATH": str(self.bin_dir) + os.pathsep + environment["PATH"],
            "FAKE_COMMAND_RECORD": str(self.record),
            "GATEWAY_TEST_RESULTS_ROOT": str(self.results),
            "EMBER_INTEGRATION_DATABASE_URL": "synthetic-database-must-not-be-used",
        })
        environment.update(overrides)
        return subprocess.run(
            ["bash", str(REPO_ROOT / "scripts/test" / (suite + ".sh"))],
            cwd=self.root,
            env=environment,
            capture_output=True,
            text=True,
            timeout=15,
        )

    def read_commands(self):
        """Read each fake invocation in execution order."""
        return [json.loads(line) for line in self.record.read_text().splitlines()]

    def test_capture_uses_selected_python_for_tools_and_runner_tests(self):
        """The CI Python matrix must use its selected interpreter for both suites."""
        result = self.run_suite("gateway-capture", PYTHON="selected-python")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        commands = self.read_commands()
        self.assertEqual(len(commands), 2)
        self.assertEqual({item["command"] for item in commands}, {"selected-python"})
        self.assertIn(str(REPO_ROOT / "scripts/gateway-capture/tests"), commands[0]["args"])
        self.assertIn(str(REPO_ROOT / "scripts/test/tests"), commands[1]["args"])
        for command in commands:
            self.assertEqual(command["args"][:4], ["-B", "-m", "unittest", "discover"])

    def test_capture_default_is_python3(self):
        """A preinstalled python3.11 must not override the default python3."""
        result = self.run_suite("gateway-capture")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual({item["command"] for item in self.read_commands()}, {"python3"})

    def test_invalid_explicit_python_fails_without_fallback(self):
        """An invalid explicit interpreter must fail with an actionable failed meta."""
        result = self.run_suite("gateway-capture", PYTHON=str(self.root / "missing-python"))
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse(self.record.exists())
        meta = json.loads((self.results / "gateway-capture/meta.json").read_text())
        self.assertEqual((meta["status"], meta["exitCode"]), ("failed", result.returncode))

    def test_capture_runner_test_failure_is_not_hidden(self):
        """Failures in the second test stage must fail the suite after tools pass."""
        result = self.run_suite("gateway-capture", FAKE_RUNNER_EXIT_CODE="23")
        self.assertEqual(result.returncode, 23, result.stdout + result.stderr)
        self.assertEqual(len(self.read_commands()), 2)
        meta = json.loads((self.results / "gateway-capture/meta.json").read_text())
        self.assertEqual((meta["status"], meta["exitCode"]), ("failed", 23))

    def test_race_is_bounded_and_clears_inherited_database(self):
        """The race entrypoint keeps the five-package, non-database acceptance scope."""
        result = self.run_suite("gateway-race")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        command, = self.read_commands()
        self.assertEqual(command["command"], "go")
        self.assertEqual(command["database"], "")
        self.assertEqual(command["args"][:3], ["-C", str(REPO_ROOT / "services/api"), "test"])
        arguments = command["args"][3:]
        self.assertIn("-race", arguments)
        self.assertIn("-count=1", arguments)
        self.assertIn("-timeout=180s", arguments)
        self.assertEqual(arguments[arguments.index("-skip") + 1], "Integration|PostgreSQL")
        self.assertEqual({arg for arg in arguments if arg.startswith("./")}, RACE_PACKAGES)

    def test_exit_code_logs_and_unrelated_artifacts_are_preserved(self):
        """A failing child survives tee and cannot erase another result or retry evidence."""
        for suite in ("gateway-capture", "gateway-race"):
            for exit_code in (0, 19):
                with self.subTest(suite=suite, exit_code=exit_code):
                    result_dir = self.results / suite
                    result_dir.mkdir(parents=True, exist_ok=True)
                    sentinel = result_dir / "keep.txt"
                    sentinel.write_text("preserved", encoding="utf-8")
                    result = self.run_suite(suite, FAKE_EXIT_CODE=str(exit_code))
                    self.assertEqual(result.returncode, exit_code, result.stdout + result.stderr)
                    meta = json.loads((result_dir / "meta.json").read_text())
                    self.assertEqual(meta["suite"], suite)
                    self.assertEqual(meta["exitCode"], exit_code)
                    self.assertEqual(meta["status"], "failed" if exit_code else "passed")
                    self.assertEqual(sentinel.read_text(), "preserved")
                    logs = "".join(path.read_text() for path in result_dir.glob("*.log"))
                    self.assertIn("synthetic command output", logs)


class PathFilterTests(unittest.TestCase):
    """Guard against tool-only changes skipping every Gateway check."""

    def test_gateway_paths_trigger_their_required_suites(self):
        """Evaluate the workflow's simple positive globs for representative changes."""
        workflow = (REPO_ROOT / ".github/workflows/test.yml").read_text()
        filters_text = workflow.split("filters: |", 1)[1].split("\n  test-", 1)[0]
        filters = {}
        suite = None
        for line in filters_text.splitlines():
            heading = re.fullmatch(r"            ([a-z_-]+):", line)
            pattern = re.fullmatch(r"              - '([^']+)'", line)
            if heading:
                suite = heading.group(1)
                filters[suite] = []
            elif pattern:
                filters[suite].append(pattern.group(1))

        cases = {
            "scripts/gateway-capture/logs.py": {"gateway_capture", "gateway_contract"},
            "scripts/gateway-capture/tests/test_tools.py": {"gateway_capture", "gateway_contract"},
            "services/api/internal/services/directplay/timing.go": {"gateway_race", "gateway_contract"},
            "scripts/test/gateway-capture.sh": {"gateway_capture"},
            "scripts/test/gateway-race.sh": {"gateway_capture", "gateway_race"},
            "scripts/test/gateway-contract.sh": {"gateway_capture", "gateway_contract"},
            "scripts/test/gateway_contract.py": {"gateway_contract"},
        }
        all_gateway = {"gateway_capture", "gateway_race", "gateway_contract"}
        for path in ("scripts/test/lib.sh", "scripts/test/all.sh", "scripts/test/tests/test_gateway_ci.py", "Makefile", ".github/workflows/test.yml"):
            cases[path] = all_gateway
        for path, required in cases.items():
            with self.subTest(path=path):
                triggered = {
                    name for name, patterns in filters.items()
                    if any(fnmatch.fnmatchcase(path, pattern) for pattern in patterns)
                }
                self.assertTrue(required.issubset(triggered), (path, required, triggered))


if __name__ == "__main__":
    unittest.main()
