"""Exercise reusable capture tools with synthetic packets and mocked SSH only."""
import json
import io
from pathlib import Path
import socket
import struct
import sys
import tempfile
import unittest
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import analyze_capture
import analyze_tls
import capture
import gateway_capture
import logs
import remote_capture


def packet(source_port, destination_port, sequence, payload, reverse=False, flags=0x18):
    """Build an Ethernet/IPv4/TCP fixture without making any network requests."""
    source, target = ("192.0.2.1", "192.0.2.2")
    if reverse:
        source, target = target, source
    ip = struct.pack("!BBHHHBBH4s4s", 0x45, 0, 40 + len(payload), 1, 0, 64, 6, 0,
                     socket.inet_aton(source), socket.inet_aton(target))
    tcp = struct.pack("!HHIIBBHHH", source_port, destination_port, sequence, 0,
                      0x50, flags, 65535, 0, 0)
    return bytes(12) + b"\x08\x00" + ip + tcp + payload


def write_pcap(path, records):
    """Persist a complete synthetic PCAP in the test's temporary directory."""
    with path.open("wb") as output:
        output.write(struct.pack("<IHHIIII", 0xA1B2C3D4, 2, 4, 0, 0, 8192, 1))
        for timestamp, frame in records:
            output.write(struct.pack("<IIII", timestamp, 0, len(frame), len(frame)))
            output.write(frame)


class AnalysisTests(unittest.TestCase):
    def test_senplayer_original_streams_pair_without_exposing_credentials(self):
        """Recognize the two supported original routes without opening unknown file names."""
        secret = "SYNTHETIC_CREDENTIAL_MUST_NOT_APPEAR"
        for container in ["mp4", "mkv"]:
            with self.subTest(container=container), tempfile.TemporaryDirectory() as directory:
                route = f"/emby/videos/123/original.{container}"
                request = (f"GET {route}?api_key={secret}&MediaSourceId=source HTTP/1.1\r\n"
                           f"User-Agent: SenPlayer/6.0.4 {secret}\r\nRange: bytes=10-\r\n"
                           f"X-Emby-Token: {secret}\r\n\r\n").encode()
                response = b"HTTP/1.1 206 Partial Content\r\nContent-Range: bytes 10-25/26\r\nContent-Length: 16\r\n\r\n"
                path = Path(directory) / "fixture.pcap"
                write_pcap(path, [(1, packet(10000, 8081, 100, request)),
                                  (2, packet(8081, 10000, 200, response, reverse=True))])
                result = analyze_capture.analyze(path)
                self.assertEqual(result["http"][0]["path"], route)
                self.assertEqual(result["http"][0]["client"], "SenPlayer/6.0.4")
                pairs = analyze_capture.video_pairs(result["http"])
                self.assertEqual(len(pairs), 1)
                self.assertEqual((pairs[0]["status"], pairs[0]["range"], pairs[0]["packetHeadersMs"]),
                                 (206, "bytes=10-", 1000))
                self.assertNotIn(secret, json.dumps(result))
        self.assertEqual(analyze_capture.safe_path("/Videos/123/original.avi"), "[other]")
        self.assertEqual(analyze_capture.safe_path(f"/Videos/123/{secret}.mkv"), "[other]")

    def test_nondefault_ports_reassembly_and_credential_redaction(self):
        secret = "SYNTHETIC_CREDENTIAL_MUST_NOT_APPEAR"
        body = json.dumps({"IsPlayback": True, "PlaySessionId": secret}).encode()
        request = (f"POST /Items/123/PlaybackInfo?api_key={secret} HTTP/1.1\r\n"
                   f"Authorization: Emby Token=\"{secret}\"\r\n"
                   f"Content-Length: {len(body)}\r\n\r\n").encode() + body
        response = (f"HTTP/1.1 302 Found\r\nLocation: https://example.invalid/{secret}?signature={secret}\r\n"
                    "Content-Length: 0\r\n\r\n").encode()
        split = 37
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "fixture.pcap"
            write_pcap(path, [(1, packet(10000, 28081, 100, request[:split])),
                              (2, packet(10000, 28081, 100 + split, request[split:])),
                              (3, packet(28081, 10000, 200, response, reverse=True))])
            result = analyze_capture.analyze(path, gateway_port=28081, emby_port=28096, tls_port=8443)
        self.assertEqual(len(result["http"]), 2)
        self.assertTrue(result["http"][0]["IsPlayback"])
        self.assertEqual(result["http"][0]["edge"], "client_gateway")
        self.assertEqual(result["http"][1]["locationHost"], "example.invalid")
        self.assertNotIn(secret, json.dumps(result))

    def test_unknown_paths_and_query_keys_do_not_echo_credentials(self):
        secret = "SYNTHETIC_CREDENTIAL_MUST_NOT_APPEAR"
        request = f"GET /private/{secret}?{secret}=1 HTTP/1.1\r\nCookie: session={secret}\r\n\r\n".encode()
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "fixture.pcap"
            write_pcap(path, [(1, packet(10000, 8081, 100, request))])
            result = analyze_capture.analyze(path)
        self.assertEqual(len(result["http"]), 1)
        self.assertNotIn(secret, json.dumps(result))

    def test_invalid_tcp_lengths_and_truncated_pcap_are_controlled(self):
        malformed = bytearray(packet(10000, 8081, 100, b"GET / HTTP/1.1\r\n\r\n"))
        malformed[14] = 0x4F
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "fixture.pcap"
            write_pcap(path, [(1, bytes(malformed))])
            result = analyze_capture.analyze(path)
            self.assertEqual(result["http"], [])
            path.write_bytes(path.read_bytes()[:-2])
            with self.assertRaisesRegex(ValueError, "truncated_pcap"):
                analyze_capture.analyze(path)

    def test_tls_sni_and_syn_retransmissions_on_nondefault_port(self):
        name = b"example.invalid"
        entry = b"\x00" + struct.pack("!H", len(name)) + name
        sni = struct.pack("!H", len(entry)) + entry
        extension = struct.pack("!HH", 0, len(sni)) + sni
        hello = b"\x03\x03" + bytes(32) + b"\x00\x00\x02\x13\x01\x01\x00" + struct.pack("!H", len(extension)) + extension
        handshake = b"\x01" + len(hello).to_bytes(3, "big") + hello
        record = b"\x16\x03\x01" + struct.pack("!H", len(handshake)) + handshake
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "fixture.pcap"
            write_pcap(path, [(1, packet(10000, 8443, 100, b"", flags=2)),
                              (2, packet(10000, 8443, 100, b"", flags=2)),
                              (3, packet(8443, 10000, 200, b"", reverse=True, flags=0x12)),
                              (4, packet(10000, 8443, 101, record))])
            result = analyze_tls.analyze_tls(path, tls_port=8443)
        self.assertEqual(len(result), 1)
        self.assertEqual(result[0]["hostname"], "example.invalid")
        self.assertEqual(result[0]["synRetransmissions"], 1)
        self.assertEqual(result[0]["tcpConnectMs"], 2000)
        self.assertIsNone(analyze_tls.client_hello_hostname(record[:45]))

    def test_video_pairing_keeps_interim_responses_out_of_the_final_result(self):
        request = b"GET /Videos/123/stream.mkv HTTP/1.1\r\nRange: bytes=0-\r\n\r\n"
        flow = ("192.0.2.1", 10000, "192.0.2.2", 8081)
        reverse = (flow[2], flow[3], flow[0], flow[1])
        events = [analyze_capture.http_summary(flow, 1, request),
                  analyze_capture.http_summary(reverse, 2, b"HTTP/1.1 100 Continue\r\n\r\n"),
                  analyze_capture.http_summary(reverse, 3, b"HTTP/1.1 206 Partial Content\r\n\r\n")]
        pairs = analyze_capture.video_pairs(events)
        self.assertEqual(len(pairs), 1)
        self.assertEqual(pairs[0]["packetHeadersMs"], 2000)
        self.assertEqual(pairs[0]["status"], 206)


class CollectionTests(unittest.TestCase):
    def test_original_stream_diagnostics_keep_only_fixed_evidence(self):
        """Retain MP4/MKV acceptance markers while dropping arbitrary evidence and secrets."""
        secret = "SYNTHETIC_CREDENTIAL_MUST_NOT_APPEAR"
        prefix = "2026-01-01T00:00:00.000Z [PlaybackGateway] "
        for container in ["mp4", "mkv"]:
            with self.subTest(container=container):
                code = f"video_original_{container}_accepted"
                evidence = f"local_{container}_proof"
                line = prefix + f"code={code} evidence={evidence} token={secret} path=/{secret}"
                result = logs.filter_line(line)
                self.assertIsNotNone(result)
                self.assertEqual(result["code"], code)
                self.assertEqual(result["evidence"], evidence)
                self.assertNotIn(secret, json.dumps(result))
                tampered = logs.filter_line(prefix + f"code={code} evidence={secret}")
                self.assertNotIn("evidence", tampered)

    def test_transfer_trigger_keeps_only_fixed_admission_categories(self):
        """Preserve the four admission outcomes without creating a free-text secret channel."""
        prefix = "2026-01-01T00:00:00.000Z [PlaybackGateway] code=direct_play_redirect "
        for trigger in ["explicit_playback", "compatible_request", "existing_target", "policy_denied"]:
            with self.subTest(trigger=trigger):
                result = logs.filter_line(prefix + f"transferTrigger={trigger}")
                self.assertEqual(result["transferTrigger"], trigger)

        secret = "SYNTHETIC_CREDENTIAL_MUST_NOT_APPEAR"
        for trigger in [secret, "compatible_request_secret", "COMPATIBLE_REQUEST",
                        "", f"https://example.invalid/video?token={secret}"]:
            with self.subTest(rejected=trigger):
                result = logs.filter_line(prefix + "transferTrigger=" + json.dumps(trigger))
                self.assertNotIn("transferTrigger", result)
                self.assertNotIn(secret, json.dumps(result))

        self.assertIsNone(logs.filter_line(
            "2026-01-01T00:00:00.000Z [PlaybackGateway] "
            "code=unknown_transfer_event transferTrigger=compatible_request"))

    def test_revalidation_diagnostics_keep_safe_correlation_only(self):
        """Allow fixed refresh/policy events while discarding credentials and raw failures."""
        secret = "SYNTHETIC_CREDENTIAL_MUST_NOT_APPEAR"
        for code, reason in [
            ("transfer_policy_unavailable", "config_unavailable"),
            ("playback_info_revalidation_started", "verified_live_session"),
            ("playback_info_revalidated", "verified_live_session"),
            ("playback_info_revalidation_failed", "playback_info_superseded"),
        ]:
            with self.subTest(code=code):
                line = (f"2026-01-01T00:00:00.000Z [PlaybackGateway] code={code} "
                        f"reasonCode={reason} requestId=0123456789abcdef "
                        "itemRef=abcdef0123456789 sessionRef=fedcba9876543210 "
                        f"Token={secret} Cookie={secret} URL=https://example.invalid/{secret} "
                        f"errorType={secret} message=\"private error {secret}\"")
                result = logs.filter_line(line)
                self.assertEqual(result, {
                    "dockerTimestamp": "2026-01-01T00:00:00.000Z",
                    "code": code,
                    "reasonCode": reason,
                    "requestId": "0123456789abcdef",
                    "itemRef": "abcdef0123456789",
                    "sessionRef": "fedcba9876543210",
                })
                self.assertNotIn(secret, json.dumps(result))

    def test_capture_orchestration_keeps_partial_artifacts_and_filters_stderr(self):
        secret = "SYNTHETIC_CREDENTIAL_MUST_NOT_APPEAR"
        with tempfile.TemporaryDirectory() as directory:
            args = gateway_capture.build_parser().parse_args(["capture", "--host", "gateway", "--via", "jump",
                         "--output", str(Path(directory) / "private")])
            process = mock.Mock()
            process.stderr = io.StringIO(secret + "\n" + json.dumps(
                {"captureReady": True, "startedUtcEpoch": 1, "unknown": secret}) + "\n" +
                json.dumps({"captureFinished": True, "endedUtcEpoch": 2, "socketDroppedPackets": 0}) + "\n")
            process.wait.return_value = 0
            def fake_process(*args, **kwargs):
                """Replace SSH while still exercising real local artifact writes."""
                kwargs["stdout"].write(struct.pack("<IHHIIII", 0xA1B2C3D4, 2, 4, 0, 0, 8192, 1))
                return process
            with mock.patch("subprocess.check_output", return_value="42 true\n"), \
                 mock.patch("subprocess.Popen", side_effect=fake_process), mock.patch("builtins.print") as output:
                capture.run_capture(args)
            path = next((Path(directory) / "private").glob("*.metadata.json"))
            metadata = json.loads(path.read_text())
            self.assertEqual(metadata["gatewayPid"], 42)
            self.assertEqual(metadata["sshExitCode"], 0)
            self.assertNotIn(secret, json.dumps(metadata))
            self.assertNotIn(secret, str(output.call_args_list))
            self.assertTrue(process.stderr.closed)

    def test_remote_byte_limit_and_socket_cleanup_use_no_real_socket(self):
        options = {"gatewayPort": 8081, "embyPort": 18096, "tlsPort": 443, "snaplen": 8192,
                   "interface": "eth0", "durationSeconds": 10, "maxBytes": 140}
        frame = packet(10000, 8081, 100, b"GET / HTTP/1.1\r\n\r\n")
        reader = mock.Mock()
        reader.recv.side_effect = [frame, frame]
        reader.getsockopt.return_value = struct.pack("II", 2, 0)
        stdout = mock.Mock(buffer=io.BytesIO())
        stderr = io.StringIO()
        with mock.patch.object(remote_capture.socket, "AF_PACKET", 17, create=True), \
             mock.patch("remote_capture.socket.socket", return_value=reader), \
             mock.patch("remote_capture.signal.signal"), mock.patch("remote_capture.time.monotonic", return_value=0), \
             mock.patch("remote_capture.sys.stdout", stdout), mock.patch("remote_capture.sys.stderr", stderr):
            remote_capture.capture(options)
        self.assertLessEqual(len(stdout.buffer.getvalue()), 140)
        self.assertEqual(json.loads(stderr.getvalue().splitlines()[-1])["stopReason"], "byte_limit")
        reader.close.assert_called_once()

    def test_ssh_failure_preserves_metadata_and_does_not_echo_remote_errors(self):
        with tempfile.TemporaryDirectory() as directory:
            args = gateway_capture.build_parser().parse_args(["capture", "--host", "gateway", "--output",
                                                             str(Path(directory) / "private")])
            process = mock.Mock()
            process.stderr = io.StringIO("SYNTHETIC_CREDENTIAL_MUST_NOT_APPEAR\n")
            process.wait.return_value = 7
            with mock.patch("subprocess.check_output", return_value="42 true\n"), \
                 mock.patch("subprocess.Popen", return_value=process), mock.patch("builtins.print") as output:
                with self.assertRaisesRegex(ValueError, "remote_capture_failed"):
                    capture.run_capture(args)
            metadata = json.loads(next((Path(directory) / "private").glob("*.metadata.json")).read_text())
            self.assertEqual(metadata["sshExitCode"], 7)
            self.assertEqual(metadata["events"], [])
            self.assertNotIn("SYNTHETIC_CREDENTIAL_MUST_NOT_APPEAR", str(output.call_args_list))

    def test_source_log_collection_is_bounded_and_filters_presence_fields(self):
        secret = "SYNTHETIC_CREDENTIAL_MUST_NOT_APPEAR"
        prefix = "2026-01-01T00:00:00.000Z [P115] "
        data = prefix + "code=source_range_completed " + secret * 3000 + "\n"
        data += prefix + "code=playback_info_request_observed currentPlaySessionId=" + secret + " mediaSourceId=present\n"
        process = mock.Mock(stdout=io.StringIO(data))
        process.wait.return_value = process.poll.return_value = 0
        options = {"sudo": False, "container": "gateway", "since": "2026-01-01T00:00:00Z",
                   "until": "2026-01-01T00:01:00Z", "tail": 1000}
        with mock.patch("subprocess.Popen", return_value=process) as run:
            rows = logs.collect(options)
        self.assertEqual(len(rows), 1)
        self.assertEqual(rows[0]["mediaSourceId"], "present")
        self.assertNotIn("currentPlaySessionId", rows[0])
        self.assertNotIn(secret, json.dumps(rows))
        command = run.call_args.args[0]
        self.assertEqual(command[command.index("--until") + 1], options["until"])
        self.assertTrue(process.stdout.closed)

    def test_nested_ssh_keeps_hop_keys_and_shell_quotes_remote_arguments(self):
        import shlex
        tokens = ["docker", "inspect", "$(must-not-execute)", "name with spaces"]
        command = capture.ssh_command("gateway", ["jump-one", "jump-two"], tokens)
        self.assertEqual(command[-2], "jump-one")
        second = shlex.split(command[-1])
        self.assertEqual(second[-2], "jump-two")
        final = shlex.split(second[-1])
        self.assertEqual(final[-2], "gateway")
        self.assertEqual(shlex.split(final[-1]), tokens)
        with self.assertRaisesRegex(ValueError, "invalid_ssh_target"):
            capture.ssh_command("-oProxyCommand=bad", [], tokens)

    def test_private_artifacts_reject_git_and_do_not_overwrite_files(self):
        import os
        import stat
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "repo" / ".git").mkdir(parents=True)
            with self.assertRaisesRegex(ValueError, "outside_git"):
                capture.private_directory(root / "repo" / "capture")
            private = capture.private_directory(root / "private")
            path = private / "metadata.json"
            capture.private_json(path, {"synthetic": True})
            self.assertEqual(stat.S_IMODE(path.stat().st_mode), 0o600)
            with self.assertRaises(FileExistsError):
                capture.private_json(path, {})
            os.chmod(private, 0o755)
            with self.assertRaisesRegex(ValueError, "owned_and_private"):
                capture.private_directory(private)

    def test_capture_limits_filtering_and_response_body_sampling(self):
        options = {"gatewayPort": 28081, "embyPort": 28096, "tlsPort": 8443, "snaplen": 8192}
        secret_body = b"SYNTHETIC_RESPONSE_BODY"
        response = b"HTTP/1.1 206 Partial Content\r\nContent-Length: 23\r\n\r\n" + secret_body
        stored = remote_capture.select_frame(packet(28081, 10000, 100, response), options)
        self.assertNotIn(secret_body, stored)
        self.assertIsNone(remote_capture.select_frame(packet(10000, 5432, 100, b"database"), options))
        self.assertFalse(remote_capture.record_fits(24, 100, 139))
        self.assertTrue(remote_capture.record_fits(24, 100, 140))

    def test_dry_run_and_offline_analysis_never_launch_a_subprocess(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "fixture.pcap"
            write_pcap(path, [(1, packet(10000, 8081, 100, b"GET /Videos/123/stream HTTP/1.1\r\n\r\n"))])
            with mock.patch("subprocess.Popen", side_effect=AssertionError("no subprocess permitted")), mock.patch("builtins.print"):
                self.assertEqual(gateway_capture.main(["capture", "--host", "gateway", "--via", "jump",
                                  "--output", str(Path(directory) / "dry"), "--dry-run"]), 0)
                self.assertFalse((Path(directory) / "dry").exists())
                self.assertEqual(gateway_capture.main(["analyze", "--pcap", str(path), "--output",
                                  str(Path(directory) / "analysis")]), 0)

    def test_live_pid_inspection_and_filtered_log_window(self):
        with mock.patch("subprocess.check_output", return_value="42 true\n") as run:
            self.assertEqual(capture.container_pid("gateway", ["jump"], "ember-gateway", True), 42)
            import shlex
            nested = shlex.split(run.call_args.args[0][-1])
            final = shlex.split(nested[-1])
            self.assertEqual(final[:4], ["sudo", "-n", "docker", "inspect"])
            self.assertNotIn("1132802", final)
        line = ('2026-01-01T00:00:00.000Z [P115] code=source_range_completed '
                'sourceDownloadMs=2000 rangeReadMs=3000 durationMs=5000 success=true '
                'cookie=SYNTHETIC_CREDENTIAL_MUST_NOT_APPEAR path=/private/SYNTHETIC_CREDENTIAL_MUST_NOT_APPEAR')
        result = logs.filter_line(line)
        self.assertEqual(result["sourceDownloadMs"], "2000")
        self.assertNotIn("SYNTHETIC_CREDENTIAL_MUST_NOT_APPEAR", json.dumps(result))
        self.assertIsNone(logs.filter_line("unknown raw credentials"))

    def test_metadata_uses_wire_utc_and_cli_rejects_unbounded_or_naive_inputs(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "metadata.json"
            path.write_text(json.dumps({"events": [{"captureReady": True, "startedUtcEpoch": 1},
                                                    {"captureFinished": True, "endedUtcEpoch": 2}]}))
            since, until = gateway_capture.capture_interval(path)
            self.assertEqual(since, "1970-01-01T00:00:01+00:00")
            self.assertEqual(until, "1970-01-01T00:00:02+00:00")
        args = gateway_capture.build_parser().parse_args(["logs", "--host", "gateway", "--output", "/unused",
                  "--since", "2026-01-01T00:00:00", "--until", "2026-01-01T00:01:00"])
        with self.assertRaisesRegex(ValueError, "invalid_log_interval"):
            gateway_capture.validate_args(args)
        with mock.patch("sys.stderr"), self.assertRaises(SystemExit):
            gateway_capture.build_parser().parse_args(["capture", "--host", "gateway", "--output", "/unused",
                                                       "--duration", "0"])


if __name__ == "__main__":
    unittest.main()
