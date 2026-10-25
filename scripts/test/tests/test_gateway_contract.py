"""Test the offline contract validator and its runner without invoking Go."""

import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest import mock


ROOT = Path(__file__).resolve().parents[3]
SPEC = importlib.util.spec_from_file_location("gateway_contract", ROOT / "scripts/test/gateway_contract.py")
contract = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(contract)


def timing_packet():
    """Provide small validator fixtures; the real runner obtains fields from Go."""
    cases = []
    for name, reason, download_reason, evictions in (
        ("miss", "dependent_url_expired", "forced_refresh", (1, 2)),
        ("hit", "", "", (3, 4)),
        ("bypass", "key_unavailable", "cache_unavailable", None),
    ):
        fields = ["directPlayMs=20", "prepareMs=3", "prepareCalls=1", "preIDWaitMs=8",
                  "preIDWaitCalls=2", "otherMs=9", "preIDCache=" + name,
                  "mediaResolutionCache=" + name, "downloadURLCache=" + name]
        if reason:
            fields += ["mediaResolutionCacheReason=" + reason, "downloadURLCacheReason=" + download_reason]
        if evictions:
            fields += ["mediaCacheEvictions=" + str(evictions[0]), "downloadCacheEvictions=" + str(evictions[1])]
        cases.append({"name": name, "fields": fields})
    return {"kind": "timing", "cases": cases}


def gateway_packet():
    """Exercise checker behavior with bounded samples, without a backend or network."""
    proof, client = "1111111111111111", "2222222222222222"
    cases = {
        "authentication": [
            "code=authentication_request_shape contentType=json bodyState=valid_json usernameState=present pwState=present passwordState=missing",
            "code=authentication_upstream_response statusCode=401",
            "code=request_completed route=authentication statusCode=401 method=POST outcome=failure userAgentFamily=senplayer userAgentVersion=6.0.4 host=private-host path=/private-path queryKeys=private-key",
        ],
        "header_parse": [
            "code=application_header_parse_failed reasonCode=unterminated_quote field=client offset=32 characterClass=end",
            "code=request_completed route=protected statusCode=401 method=GET userAgentFamily=senplayer userAgentVersion=6.0.4",
        ],
        "fallback": [
            "code=direct_play_fallback decision=fallback stage=direct_play reasonCode=source_read_timeout statusCode=206 upstreamStatus=206 durationMs=12",
            "code=request_completed route=video statusCode=206 method=GET userAgentFamily=senplayer userAgentVersion=6.0.4",
        ],
        "bound": [
            "code=video_context_resolved sessionRef=" + proof,
            "code=playback_lease_session_bound reasonCode=verified_client_session sessionRef=" + client + " proofSessionRef=" + proof,
            "code=direct_play_redirect decision=redirect stage=direct_play reasonCode=direct_play_ready statusCode=302 sessionRef=" + client + " durationMs=7",
            "code=request_completed route=video statusCode=302 method=GET userAgentFamily=senplayer userAgentVersion=6.0.4",
        ],
    }
    return {"kind": "gateway", "cases": [
        {"name": name, "lines": ["[PlaybackGateway] requestId=0123456789abcdef-" + str(index) + " " + line
                                 for line in lines]}
        for index, (name, lines) in enumerate(cases.items(), 1)
    ]}


def producer_events():
    """Model successful go test -json events, including each test and package pass."""
    events = []
    for (package, test), kind in contract.PRODUCERS.items():
        packet = timing_packet() if kind == "timing" else gateway_packet()
        common = {"Package": package, "Test": test}
        events += [dict(common, Action="run"),
                   dict(common, Action="output", Output="    diagnostic_contract_test.go:1: " + contract.MARKER + " " + json.dumps(packet) + "\n"),
                   dict(common, Action="pass"), {"Package": package, "Action": "pass"}]
    return events


def event_stream(events):
    """Serialize synthetic go test events for the same streaming parser used by CI."""
    return io.StringIO("".join(json.dumps(event) + "\n" for event in events))


class ContractTests(unittest.TestCase):
    """Fail closed on missing producer evidence and observable consumer drift."""

    def test_success_uses_real_filter_and_summary(self):
        """A complete stream checks all timing cases and both session identities."""
        result = contract.check_contract(event_stream(producer_events()))
        self.assertEqual(result["status"], "passed")
        self.assertEqual((result["timingCases"], result["gatewayCases"]), (3, 4))
        self.assertGreaterEqual(result["sessionConflictCases"], 5)

    def test_incomplete_failed_or_duplicate_producers_are_rejected(self):
        """Neither empty output, cached skips nor a printed marker proves a pass."""
        original = producer_events()
        variants = {
            "empty": [],
            "missing_producer": original[:4],
            "missing_test_pass": [event for event in original if not (event["Action"] == "pass" and event.get("Test"))],
            "missing_package_pass": [event for event in original if event.get("Test")],
            "failure_after_output": original + [dict(original[0], Action="fail")],
            "skip": [dict(event, Action="skip") if event["Action"] == "pass" and event.get("Test") else event for event in original],
            "duplicate_output": original[:2] + [original[1]] + original[2:],
            "duplicate_run": [original[0]] + original,
            "output_before_run": [original[1], original[0]] + original[2:],
        }
        for name, events in variants.items():
            with self.subTest(name=name), self.assertRaises(contract.ContractError):
                contract.check_contract(event_stream(events))

    def test_malformed_json_and_payload_shapes_are_rejected(self):
        """Input errors must fail without copying untrusted input into diagnostics."""
        for text in ("not-json-private-value\n", "[]\n", '{"Action":"output","Output":[]}\n'):
            with self.subTest(text=text), self.assertRaises(contract.ContractError) as error:
                contract.check_contract(io.StringIO(text))
            self.assertNotIn("private-value", str(error.exception))
        for packet in (None, {}, {"kind": "timing", "cases": []},
                       {"kind": "timing", "cases": [{"name": "miss", "fields": [None]}]}):
            events = producer_events()
            events[1]["Output"] = contract.MARKER + " " + json.dumps(packet) + "\n"
            with self.subTest(packet=packet), self.assertRaises(contract.ContractError):
                contract.check_contract(event_stream(events))

    def test_all_emitted_timing_fields_must_survive(self):
        """A future Go field cannot silently disappear from the Python allowlist."""
        packet = timing_packet()
        packet["cases"][0]["fields"].append("futureStageMs=5")
        with self.assertRaisesRegex(contract.ContractError, "timing field"):
            contract.validate_timing(packet)
        packet = timing_packet()
        packet["cases"][0]["fields"].append("prepareMs=99")
        with self.assertRaisesRegex(contract.ContractError, "duplicate"):
            contract.validate_timing(packet)

    def test_missing_preid_wait_and_cache_reason_fail(self):
        """The new wait counters and miss reasons remain required fixture evidence."""
        for field in ("preIDWaitCalls", "mediaResolutionCacheReason"):
            packet = timing_packet()
            packet["cases"][0]["fields"] = [value for value in packet["cases"][0]["fields"]
                                           if not value.startswith(field + "=")]
            with self.subTest(field=field), self.assertRaises(contract.ContractError):
                contract.validate_timing(packet)

    def test_summary_drift_and_sensitive_field_leaks_fail(self):
        """Contract checks reject cumulative counters treated as deltas and leaks."""
        original_summary = contract.diagnostic_summary.build_summary

        def changed_summary(events, rows):
            """Simulate a consumer regression without changing production modules."""
            result = original_summary(events, rows)
            result["gateway"]["caches"]["mediaResolutionCache"]["capacityEvictions"]["intervalDelta"] = 3
            return result

        with mock.patch.object(contract.diagnostic_summary, "build_summary", side_effect=changed_summary):
            with self.assertRaises(contract.ContractError):
                contract.validate_timing(timing_packet())
        original_filter = contract.logs.filter_record

        def leaking_filter(values):
            """Simulate an unsafe newly allowed credential field."""
            result = original_filter(values)
            if result is not None and "token" in values:
                result["token"] = values["token"]
            return result

        with mock.patch.object(contract.logs, "filter_record", side_effect=leaking_filter):
            with self.assertRaises(contract.ContractError):
                contract.validate_gateway(gateway_packet())

    def test_real_bound_group_and_unexplained_session_conflict_differ(self):
        """An arbitrary pair of refs cannot impersonate the verified binding."""
        packet = gateway_packet()
        bound = packet["cases"][-1]
        bound["lines"] = [line for line in bound["lines"] if "code=playback_lease_session_bound" not in line]
        with self.assertRaises(contract.ContractError):
            contract.validate_gateway(packet)

    def test_wrong_gateway_reason_or_auth_shape_fails(self):
        """Renamed enums and lost credential-shape states cannot silently pass."""
        for old, new in (("reasonCode=source_read_timeout", "reasonCode=renamed_reason"),
                         ("pwState=present", "pwState=missing")):
            packet = gateway_packet()
            for case in packet["cases"]:
                case["lines"] = [line.replace(old, new) for line in case["lines"]]
            with self.subTest(old=old), self.assertRaises(contract.ContractError):
                contract.validate_gateway(packet)


class ContractRunnerTests(unittest.TestCase):
    """Run only the shell orchestration, replacing both language executables."""

    def setUp(self):
        """Create isolated fake commands and a caller-owned artifact sentinel."""
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        self.bin = self.root / "bin"
        self.bin.mkdir()
        self.record = self.root / "commands.jsonl"
        self.results = self.root / "results"
        self.suite = self.results / "gateway-contract"
        self.suite.mkdir(parents=True)
        (self.suite / "keep.txt").write_text("caller-owned", encoding="utf-8")
        for name in ("go", "python3", "selected-python"):
            executable = self.bin / name
            executable.write_text(
                "#!" + sys.executable + "\n"
                "import json, os, sys\n"
                "from pathlib import Path\n"
                "name = Path(sys.argv[0]).name\n"
                "with open(os.environ['CONTRACT_COMMANDS'], 'a') as output:\n"
                "    output.write(json.dumps({'name': name, 'args': sys.argv[1:], "
                "'database': os.environ.get('EMBER_INTEGRATION_DATABASE_URL'), "
                "'bytecode': os.environ.get('PYTHONDONTWRITEBYTECODE')}) + '\\n')\n"
                "print('{}', flush=True)\n"
                "if name == 'go':\n"
                "    print('go: downloading offline-fixture', file=sys.stderr, flush=True)\n"
                "sys.exit(int(os.environ.get('CONTRACT_GO_EXIT' if name == 'go' else 'CONTRACT_PYTHON_EXIT', '0')))\n",
                encoding="utf-8",
            )
            executable.chmod(0o700)

    def run_runner(self, **overrides):
        """Invoke the real runner with fake Go/Python and no usable database DSN."""
        environment = os.environ.copy()
        environment.pop("PYTHON", None)
        environment.update(PATH=str(self.bin) + os.pathsep + environment["PATH"],
                           CONTRACT_COMMANDS=str(self.record), GATEWAY_TEST_RESULTS_ROOT=str(self.results),
                           EMBER_INTEGRATION_DATABASE_URL="fixture-do-not-connect")
        environment.update(overrides)
        result = subprocess.run(["bash", str(ROOT / "scripts/test/gateway-contract.sh")],
                                cwd=self.root, env=environment, capture_output=True, text=True, timeout=15)
        meta = json.loads((self.suite / "meta.json").read_text())
        self.assertEqual(meta["exitCode"], result.returncode, result.stdout + result.stderr)
        self.assertEqual(meta["status"], "passed" if result.returncode == 0 else "failed")
        self.assertEqual((self.suite / "keep.txt").read_text(), "caller-owned")
        self.assertTrue((self.suite / "go-test.jsonl").exists())
        self.assertTrue((self.suite / "go-stderr.log").exists())
        self.assertTrue((self.suite / "validator.log").exists())
        commands = [json.loads(line) for line in self.record.read_text().splitlines()] if self.record.exists() else []
        return result, commands

    def test_success_uses_exact_producers_and_default_python(self):
        """Only the two offline producers run; no inherited integration DSN survives."""
        result, commands = self.run_runner()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual([item["name"] for item in commands], ["go", "python3"])
        arguments = commands[0]["args"]
        self.assertEqual(arguments[:4], ["-C", str(ROOT / "services/api"), "test", "-json"])
        self.assertIn("-count=1", arguments)
        self.assertIn("-timeout=120s", arguments)
        self.assertEqual(set(arguments[-2:]), {"./internal/services/directplay", "./internal/playbackgateway"})
        self.assertEqual(arguments[arguments.index("-run") + 1],
                         "^(TestGatewayDiagnosticContractTimingProducer|TestGatewayDiagnosticContractLogProducer)$")
        self.assertEqual(commands[0]["database"], "")
        self.assertEqual(commands[1]["bytecode"], "1")
        self.assertIn(str(self.suite / "go-test.jsonl"), commands[1]["args"])
        self.assertEqual(json.loads((self.suite / "go-test.jsonl").read_text()), {})
        self.assertIn("go: downloading", (self.suite / "go-stderr.log").read_text())

    def test_explicit_python_is_respected(self):
        """CI's selected interpreter is used without probing another version."""
        result, commands = self.run_runner(PYTHON="selected-python")
        self.assertEqual(result.returncode, 0)
        self.assertEqual(commands[-1]["name"], "selected-python")

    def test_go_failure_stops_before_python(self):
        """A failed producer run must not validate stale or partial output."""
        result, commands = self.run_runner(CONTRACT_GO_EXIT="17")
        self.assertEqual(result.returncode, 17)
        self.assertEqual([item["name"] for item in commands], ["go"])

    def test_python_failure_propagates(self):
        """A contract mismatch fails the shell suite and persisted metadata."""
        result, commands = self.run_runner(CONTRACT_PYTHON_EXIT="19")
        self.assertEqual(result.returncode, 19)
        self.assertEqual(len(commands), 2)

    def test_invalid_python_does_not_fallback(self):
        """An invalid explicit executable remains a visible suite failure."""
        result, commands = self.run_runner(PYTHON=str(self.root / "absent-python"))
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual([item["name"] for item in commands], ["go"])


if __name__ == "__main__":
    unittest.main()
