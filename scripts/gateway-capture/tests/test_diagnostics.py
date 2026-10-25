"""Lock bounded diagnostic contracts with synthetic logs and packets only."""
import json
from pathlib import Path
import sys
import tempfile
import unittest
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import analyze_capture
import gateway_capture
import logs
from test_tools import packet, write_pcap


STAMP = "2026-01-01T00:00:00.000Z"
SECRET = "SYNTHETIC_CREDENTIAL_MUST_NOT_APPEAR"


def log_line(code, **fields):
    """Generate logger-shaped fixtures, never real credentials or external calls."""
    values = dict(code=code, **fields)
    return STAMP + " [PlaybackGateway] " + " ".join(
        key + "=" + json.dumps(str(value)) for key, value in values.items())


def offline_result(frames, rows=None):
    """Exercise the real CLI and artifact filter while making subprocess use fail."""
    with tempfile.TemporaryDirectory() as directory:
        root = Path(directory)
        pcap = root / "fixture.pcap"
        write_pcap(pcap, frames)
        argv = ["analyze", "--pcap", str(pcap), "--output", str(root / "analysis")]
        if rows is not None:
            source = root / "logs.json"
            source.write_text(json.dumps(rows))
            argv += ["--logs", str(source)]
        with mock.patch("subprocess.Popen", side_effect=AssertionError("offline only")), \
                mock.patch("subprocess.check_output", side_effect=AssertionError("offline only")), \
                mock.patch("builtins.print"):
            result = gateway_capture.main(argv)
        if result != 0:
            raise AssertionError("offline analysis failed")
        return json.loads((root / "analysis" / "fixture.analysis.json").read_text())


class DiagnosticLogTests(unittest.TestCase):
    def test_request_completion_keeps_fixed_shape_and_drops_raw_metadata(self):
        row = logs.filter_line(log_line(
            "request_completed", requestId="0123456789abcdef-1", method="GET", route="item_image",
            pathMode="passthrough", statusCode=404, outcome="failure", durationMs=20,
            xEmbyTokenCount=0, xEmbyTokenState="missing", xMediaBrowserTokenCount=1,
            xMediaBrowserTokenState="present", xEmbyAuthorizationCount=0,
            xMediaBrowserAuthorizationCount=0, authorizationCount=1, applicationScheme="media_browser",
            embeddedTokenState="present", apiKeyQueryPresent="false", queryTokenSourceCount=1,
            queryTokenState="present", userAgentFamily="yamby", userAgentVersion="1.2.3-4",
            queryKeyCount=3, queryKeysTruncated="false", pathTruncated="false", host=SECRET,
            path="/private/" + SECRET, queryKeys=SECRET, Token=SECRET, message=SECRET))
        self.assertIsNotNone(row)
        self.assertEqual(row["route"], "item_image")
        self.assertEqual(row["userAgentVersion"], "1.2.3-4")
        self.assertEqual(row["queryTokenSourceCount"], "1")
        self.assertNotIn(SECRET, json.dumps(row))
        self.assertNotIn("host", row)
        self.assertNotIn("queryKeys", row)

    def test_authentication_and_parser_diagnostics_have_independent_fixed_schemas(self):
        fixtures = [
            ("authentication_request_shape", dict(contentType="json", bodyState="valid_json",
                usernameState="present", pwState="empty", passwordState="duplicate")),
            ("authentication_upstream_response", dict(statusCode="401")),
            ("application_header_parse_failed", dict(route="protected", pathMode="emby_prefixed",
                statusCode="401", reasonCode="expected_quote", field="token", offset="12",
                characterClass="single_quote")),
        ]
        for code, fields in fixtures:
            with self.subTest(code=code):
                row = logs.filter_line(log_line(code, requestId="0123456789abcdef-2", **fields))
                self.assertIsNotNone(row)
                for key, value in fields.items():
                    self.assertEqual(row[key], value)
                poisoned = logs.filter_line(log_line(code, requestId="0123456789abcdef-2",
                    **{key: SECRET for key in fields}, source=SECRET, method=SECRET, body=SECRET))
                self.assertNotIn(SECRET, json.dumps(poisoned))

    def test_unknown_codes_enums_versions_and_noncanonical_counts_are_dropped(self):
        self.assertIsNone(logs.filter_line(log_line(SECRET, route="protected")))
        keys = ["route", "pathMode", "outcome", "userAgentFamily", "userAgentVersion",
                "applicationScheme", "xEmbyTokenState", "xMediaBrowserTokenState",
                "embeddedTokenState", "queryTokenState", "method"]
        row = logs.filter_line(log_line("request_completed", **dict.fromkeys(keys, SECRET)))
        self.assertIsNotNone(row)
        self.assertNotIn(SECRET, json.dumps(row))
        for value in ["-1", "+1", "1.5", "01", "1e3", "9" * 30, "true", SECRET]:
            with self.subTest(value=value):
                row = logs.filter_line(log_line("request_completed", queryKeyCount=value,
                    xEmbyTokenCount=value, durationMs=value))
                self.assertNotIn("queryKeyCount", row)
                self.assertNotIn("xEmbyTokenCount", row)
                self.assertNotIn("durationMs", row)
        for value in ["-2", "99999999999999999999", SECRET]:
            row = logs.filter_line(log_line("application_header_parse_failed", offset=value))
            self.assertNotIn("offset", row)
        row = logs.filter_line(log_line("application_header_parse_failed", offset=-1))
        self.assertEqual(row["offset"], "-1")

    def test_duplicate_fields_and_large_lines_fail_closed(self):
        line = log_line("request_completed", requestId="0123456789abcdef-1", route="protected")
        self.assertIsNone(logs.filter_line(line + " requestId=0123456789abcdef-2"))
        self.assertIsNone(logs.filter_line(line + " ignored=" + SECRET * 3000))
        self.assertIsNone(logs.filter_line(line + " code=authentication_upstream_response"))

    def test_nested_json_enum_carriers_are_discarded_without_aborting_analysis(self):
        for invalid in [[], {}, [SECRET], {"token": SECRET}, None, True, 1, 1.5]:
            with self.subTest(invalid=invalid):
                rows = [dict(dockerTimestamp=STAMP, code="request_completed",
                             requestId="0123456789abcdef-1", userAgentFamily=invalid, userAgentVersion="1.0",
                             route=invalid, statusCode=invalid, durationMs=invalid),
                        dict(dockerTimestamp=STAMP, code="direct_play_redirect",
                             requestId="0123456789abcdef-2", decision=invalid, stage=invalid,
                             mediaResolutionCache=invalid, mediaResolutionCacheReason="entry_missing",
                             mediaCacheEvictions="12", downloadURLCache=invalid,
                             downloadURLCacheReason="expired", downloadCacheEvictions="34", preIDCache=invalid)]
                result = offline_result([], rows)
                self.assertEqual(len(result["gatewayDiagnostics"]), 2)
                self.assertNotIn("userAgentFamily", result["gatewayDiagnostics"][0])
                self.assertNotIn("userAgentVersion", result["gatewayDiagnostics"][0])
                self.assertNotIn("mediaResolutionCacheReason", result["gatewayDiagnostics"][1])
                self.assertNotIn("mediaCacheEvictions", result["gatewayDiagnostics"][1])
                self.assertNotIn(SECRET, json.dumps(result))

    def test_preid_steps_do_not_accept_each_others_phase_contract(self):
        for step, phase in [("preIDWait", "hit"), ("preIDCache", "finished")]:
            row = logs.filter_line(log_line("direct_play_step", step=step, phase=phase, durationMs=1))
            self.assertNotIn("phase", row)

    def test_video_fallback_reasons_are_fixed_not_arbitrary_error_text(self):
        for code in ["playback_fallback", "direct_play_fallback", "playback_rejected", "playback_canceled"]:
            row = logs.filter_line(log_line(code, decision="fallback", stage="proof",
                reasonCode="playback_proof_missing", fallbackSource="client_request", statusCode=206))
            self.assertIsNotNone(row)
            self.assertEqual(row["reasonCode"], "playback_proof_missing")
            poisoned = logs.filter_line(log_line(code, stage=SECRET, reasonCode=SECRET, decision=SECRET))
            self.assertNotIn(SECRET, json.dumps(poisoned))

    def test_existing_playback_resolution_failures_remain_observable(self):
        # Literal producer contracts from video.go, playback_info_resolver.go,
        # playback_revalidation.go, playback_session_lease.go and transfer_trigger.go.
        producers = {
            "playback_fallback": ["container_recovered", "route_not_accelerated", "request_not_eligible",
                "playback_proof_missing", "playback_proof_expired", "playback_proof_mismatch", "media_not_direct_play", "direct_play_disabled"],
            "direct_play_fallback": ["invalid_request", "path_not_mapped", "account_unavailable", "personal_account_missing",
                "account_concurrency_exceeded", "redis_unavailable", "head_lease_missing", "playback_route_changed",
                "playback_lease_lost", "playback_resolve_timeout", "source_read_timeout", "transfer_quota_exceeded",
                "playback_intent_required", "transfer_quota_commit_failed", "accounts_same", "provider_unavailable",
                "rapid_upload_unavailable", "target_unavailable", "file_unavailable", "download_incompatible",
                "store_unavailable", "lock_unavailable", "provider_protocol"],
            "playback_info_finished": ["invalid_request", "playback_info_busy", "playback_info_superseded", "request_canceled",
                "deadline_exceeded", "upstream_unavailable", "upstream_status", "response_invalid", "response_too_large",
                "response_decode_failed", "response_unusable", "internal_failure"],
            "playback_info_revalidation_failed": ["internal_failure", "media_identity_mismatch", "request_canceled", "playback_info_superseded"],
            "playback_lease_skipped": ["snapshot_unavailable", "service_unavailable"],
            "playback_lease_update_failed": ["lease_update_failed", "redis_unavailable", "session_identity_invalid"],
            "playback_lease_session_bound": ["verified_client_session"],
            "transfer_policy_unavailable": ["config_unavailable"],
            "playback_transfer_intent_revoked": ["playback_stopped"],
            "playback_transfer_intent_retained": ["client_playback_requested"],
            "playback_transfer_intent_recorded": ["client_playback_requested"],
            "playback_transfer_intent_inherited": ["matching_client_intent"],
            "playback_info_revalidated": ["verified_live_session"],
        }
        for code, reasons in producers.items():
            for reason in reasons:
                with self.subTest(code=code, reason=reason):
                    row = logs.filter_line(log_line(code, reasonCode=reason, requestId="0123456789abcdef-1", durationMs=4))
                    self.assertEqual(row["reasonCode"], reason)

    def test_cache_diagnostics_accept_only_exact_reasons_and_uint64_snapshots(self):
        contracts = {
            "mediaResolutionCache": ("mediaResolutionCacheReason", "mediaCacheEvictions", ["entry_missing", "expired",
                "clock_rollback", "freshness_expired", "dependent_url_missing", "dependent_url_expired",
                "dependent_url_clock_rollback", "dependent_url_host_rejected", "key_unavailable", "cache_unavailable"]),
            "downloadURLCache": ("downloadURLCacheReason", "downloadCacheEvictions", ["entry_missing", "expired",
                "clock_rollback", "host_rejected", "key_unavailable", "cache_unavailable", "forced_refresh"]),
        }
        for cache, (reason_field, counter, reasons) in contracts.items():
            for reason in reasons:
                with self.subTest(cache=cache, reason=reason):
                    row = logs.filter_line(log_line("direct_play_redirect", **{
                        cache: "miss", reason_field: reason, counter: str(2**64 - 1)}))
                    self.assertEqual(row[reason_field], reason)
                    self.assertEqual(row[counter], str(2**64 - 1))
            for bad in ["-1", "+1", "01", "1.5", str(2**64), "9" * 100, SECRET]:
                row = logs.filter_line(log_line("direct_play_redirect", **{
                    cache: "bypass", reason_field: SECRET, counter: bad}))
                self.assertNotIn(reason_field, row)
                self.assertNotIn(counter, row)
                self.assertNotIn(SECRET, json.dumps(row))
            row = logs.filter_line(log_line("direct_play_redirect", **{cache: "hit", reason_field: reasons[0]}))
            self.assertNotIn(reason_field, row)
        row = logs.filter_line(log_line("direct_play_redirect", preIDCache="hit"))
        self.assertEqual(row["preIDCache"], "hit")
        row = logs.filter_line(log_line("direct_play_step", step="preIDWait", phase="finished", durationMs=4))
        self.assertEqual(row["step"], "preIDWait")
        row = logs.filter_line(log_line("direct_play_redirect", preIDCache=SECRET))
        self.assertNotIn("preIDCache", row)


class DiagnosticPacketTests(unittest.TestCase):
    def test_large_encoded_and_ambiguous_packet_inputs_remain_bounded(self):
        requests = [
            (f"GET /private/{SECRET * 100}?%41ccessToken={SECRET}&PlaySessionId={SECRET}&playsessionid=other HTTP/1.1\r\n"
             f"User-Agent: Emby/{SECRET * 1000}\r\n\r\n").encode(),
            ("GET /Users/AuthenticateByName?" + "&".join("key=" + SECRET for _ in range(257))
             + " HTTP/1.1\r\n\r\n").encode(),
        ]
        frames = []
        for index, request in enumerate(requests):
            for offset in range(0, len(request), 4096):
                frames.append((len(frames) + 1, packet(10000 + index, 8081, 100 + offset, request[offset:offset + 4096])))
        result = offline_result(frames)
        self.assertNotIn(SECRET, json.dumps(result))
        http = result["httpEvidence"]
        self.assertEqual(http["counts"]["unparsedMessages"], 1)
        self.assertEqual(len(http["http"]), 1)
        self.assertEqual(http["http"][0]["clientVersion"], "invalid")
        self.assertEqual(http["http"][0]["path"], "[other]")
        self.assertNotIn("sessionRef", http["http"][0])

    def test_client_families_match_gateway_and_keep_existing_client_values(self):
        clients = [("Infuse", "infuse"), ("Infuse-Direct", "infuse_direct"),
                   ("Infuse-Library", "infuse_library"), ("SenPlayer", "senplayer"),
                   ("Yamby", "yamby"), ("VidHub", "vidhub"), ("Fileball", "fileball"),
                   ("Conflux", "conflux"), ("Emby", "emby")]
        frames = []
        for index, (name, _) in enumerate(clients):
            request = (f"GET /System/Info HTTP/1.1\r\nUser-Agent: {name}/1.2.3 {SECRET}\r\n\r\n").encode()
            frames.append((index + 1, packet(10000 + index, 8081, 100, request)))
        result = offline_result(frames)
        for event, (name, family) in zip(result["httpEvidence"]["http"], clients):
            self.assertEqual(event["clientFamily"], family)
            self.assertEqual(event["clientVersion"], "1.2.3")
            self.assertEqual(event["client"], name + "/1.2.3")
        self.assertNotIn(SECRET, json.dumps(result))

    def test_user_agent_version_is_never_a_partial_match_or_embedded_family(self):
        flow = ("192.0.2.1", 10000, "192.0.2.2", 8081)
        for agent, family, version in [
            ("Yamby/1.2" + SECRET, "yamby", "invalid"),
            ("SenPlayer/1.2-" + SECRET, "senplayer", "invalid"),
            ("Emby/" + "1" * 33, "emby", "invalid"),
            (SECRET + " Infuse/1.2", "other", ""),
            ("infuse/1.2", "other", ""), ("", "missing", ""),
        ]:
            with self.subTest(agent=agent):
                event = analyze_capture.http_summary(flow, 1,
                    f"GET /System/Info HTTP/1.1\r\nUser-Agent: {agent}\r\n\r\n".encode())
                self.assertEqual((event["clientFamily"], event["clientVersion"]), (family, version))
                self.assertEqual(event["client"], "other")
                self.assertNotIn(SECRET, json.dumps(event))

    def test_new_routes_emit_only_templates_and_fixed_categories(self):
        cases = [
            ("/eMbY/users/AuthenticateByName", "/Users/AuthenticateByName", "authentication"),
            (f"/emby/Items/{SECRET}/Images/Primary", "/Items/{Id}/Images/{Type}", "item_image"),
            (f"/Items/{SECRET}/Images/Backdrop/0", "/Items/{Id}/Images/{Type}/{Index}", "item_image"),
            (f"/Users/{SECRET}/Images/Primary", "/Users/{UserId}/Images/{Type}", "user_image"),
            (f"/Videos/123/{SECRET}/Subtitles/2/Stream.srt",
                "/Videos/{Id}/{MediaSourceId}/Subtitles/{Index}/Stream.{Format}", "subtitle"),
            (f"/emby/Items/123/{SECRET}/Subtitles/2/999/Stream.vtt",
                "/Items/{Id}/{MediaSourceId}/Subtitles/{Index}/{StartPositionTicks}/Stream.{Format}", "subtitle"),
        ]
        frames = []
        for index, (path, _, _) in enumerate(cases):
            request = (f"GET {path}?X-MediaBrowser-Token={SECRET}&AccessToken={SECRET} HTTP/1.1\r\n"
                       f"Host: {SECRET}\r\n\r\n").encode()
            frames.append((index + 1, packet(10000 + index, 8081, 100, request)))
        result = offline_result(frames)
        for event, (_, template, category) in zip(result["httpEvidence"]["http"], cases):
            self.assertEqual((event["path"], event["routeCategory"]), (template, category))
            self.assertEqual(event["queryKeys"], ["accesstoken", "x-mediabrowser-token"])
        self.assertNotIn(SECRET, json.dumps(result))
        for path in [f"/Items/%53{SECRET}/Images/Primary", "/Users%2fAuthenticateByName",
                     "/emby/emby/Users/AuthenticateByName", "/Users/AuthenticateByName/",
                     f"/Videos/123/{SECRET}/Subtitles/2/Stream.srt/private"]:
            self.assertEqual(analyze_capture.safe_path(path), "[other]")


class DiagnosticSummaryTests(unittest.TestCase):
    def test_only_explicit_verified_session_binding_explains_two_session_refs(self):
        proof, client, third = "1111111111111111", "2222222222222222", "3333333333333333"
        common = dict(dockerTimestamp=STAMP, requestId="0123456789abcdef-1")
        rows = [dict(common, code="video_context_resolved", sessionRef=proof, playbackContext="supplemented"),
                dict(common, code="playback_lease_session_bound", sessionRef=client, proofSessionRef=proof, reasonCode="verified_client_session"),
                dict(common, code="direct_play_step", sessionRef=client, step="preIDWait", phase="finished", durationMs="8"),
                dict(common, code="direct_play_fallback", sessionRef=client, decision="fallback", stage="direct_play",
                     reasonCode="source_read_timeout", statusCode="206", preIDWaitMs="8", preIDWaitCalls="1"),
                dict(common, code="request_completed", route="video", statusCode="206")]
        result = offline_result([], rows)
        gateway = result["diagnosticSummary"]["gateway"]
        self.assertEqual(gateway["ambiguousRequestGroups"], 0)
        self.assertEqual(gateway["videoFallbacks"][0]["reasonCode"], "source_read_timeout")
        final = next(row for row in result["gatewayDiagnostics"] if row["code"] == "direct_play_fallback")
        self.assertEqual((final["preIDWaitMs"], final["preIDWaitCalls"]), ("8", "1"))
        self.assertTrue(any(row["field"] == "preIDWaitMs" and row["minMs"] == 8 for row in gateway["timings"]))
        variants = {
            "missing_binding": rows[:1] + rows[2:],
            "duplicate_binding": rows + [rows[1]],
            "wrong_reason": [rows[0], dict(rows[1], reasonCode="unverified")] + rows[2:],
            "conflicting_proof": [rows[0], dict(rows[1], proofSessionRef=third)] + rows[2:],
            "third_session": rows + [dict(common, code="video_fallback_started", sessionRef=third)],
        }
        for name, variant in variants.items():
            with self.subTest(name=name):
                gateway = offline_result([], variant)["diagnosticSummary"]["gateway"]
                self.assertEqual(gateway["ambiguousRequestGroups"], 1)
                self.assertEqual(gateway["videoFallbacks"], [])
                self.assertEqual(gateway["timings"], [])

    def test_cache_snapshot_maxima_do_not_sum_or_explain_misses(self):
        rows = []
        for index, (media, download) in enumerate([(7, 4), (7, 6), (3, 2)]):
            rows.append(dict(dockerTimestamp=STAMP, code="direct_play_redirect",
                requestId=f"0123456789abcdef-{index + 1}", decision="redirect", stage="direct_play",
                statusCode="302", mediaResolutionCache="miss", mediaResolutionCacheReason="entry_missing",
                downloadURLCache="hit", preIDCache="bypass", mediaCacheEvictions=str(media),
                downloadCacheEvictions=str(download)))
        summary = offline_result([], rows)["diagnosticSummary"]["gateway"]["caches"]
        self.assertEqual(summary["mediaResolutionCache"]["capacityEvictions"]["maxObserved"], 7)
        self.assertEqual(summary["downloadURLCache"]["capacityEvictions"]["maxObserved"], 6)
        self.assertEqual(summary["mediaResolutionCache"]["capacityEvictions"]["intervalDelta"], "unknown")
        self.assertEqual(summary["mediaResolutionCache"]["observations"],
                         [dict(outcome="miss", reason="entry_missing", count=3)])
        self.assertEqual(summary["downloadURLCache"]["observations"], [dict(outcome="hit", count=3)])
        self.assertEqual(summary["preIDCache"]["observations"], [dict(outcome="bypass", count=3)])
        empty = offline_result([])["diagnosticSummary"]["gateway"]["caches"]
        self.assertIsNone(empty["mediaResolutionCache"]["capacityEvictions"]["maxObserved"])

    def test_summary_preserves_legacy_shape_and_does_not_infer_missing_status(self):
        request = b"POST /Users/AuthenticateByName HTTP/1.1\r\nUser-Agent: Yamby/1.0\r\n\r\n"
        response = b"HTTP/1.1 401 Unauthorized\r\nContent-Length: 0\r\n\r\n"
        frames = [(1, packet(10000, 8081, 100, request)),
                  (2, packet(8081, 10000, 200, response, reverse=True)),
                  (3, packet(10001, 8081, 100, b"GET /Items/123/Images/Primary HTTP/1.1\r\n\r\n"))]
        result = offline_result(frames)
        self.assertEqual(result["formatVersion"], 1)
        self.assertIn("packetVideoPairs", result)
        summary = result["diagnosticSummary"]
        self.assertEqual(summary["correlation"], "gateway_request_id_only")
        self.assertEqual(summary["playbackSuccess"], "unknown")
        self.assertIn(dict(edge="client_gateway", clientFamily="yamby", route="authentication",
                           status="401", count=1), summary["http"]["distribution"])
        self.assertIn(dict(edge="client_gateway", clientFamily="missing", route="item_image",
                           status="unknown", count=1), summary["http"]["distribution"])
        self.assertEqual(summary["gateway"]["authentication"], [])

    def test_request_scoped_auth_images_fallback_and_timings_use_explicit_evidence(self):
        rows = []
        def add(code, request, **fields):
            """Use raw fixture dictionaries to exercise the second filtering boundary."""
            rows.append(dict(dockerTimestamp=STAMP, code=code, requestId=request, **fields))
        auth = "0123456789abcdef-1"
        add("authentication_request_shape", auth, contentType="json", bodyState="valid_json",
            usernameState="present", pwState="empty", passwordState="missing")
        add("authentication_upstream_response", auth, statusCode="401")
        add("request_completed", auth, route="authentication", statusCode="401", userAgentFamily="yamby", durationMs="10")
        parse = "0123456789abcdef-2"
        add("application_header_parse_failed", parse, reasonCode="expected_quote", field="token",
            characterClass="single_quote", statusCode="200", route="authentication")
        add("authentication_upstream_response", parse, statusCode="200")
        add("request_completed", parse, route="authentication", statusCode="200", durationMs="30")
        add("request_completed", "0123456789abcdef-3", route="item_image", statusCode="502", durationMs="20")
        video = "0123456789abcdef-4"
        add("playback_fallback", video, decision="fallback", stage="route", reasonCode="request_not_eligible",
            statusCode="206", upstreamStatus="206", durationMs="40", fallbackHeadersMs="15")
        add("request_completed", video, route="video", statusCode="206", durationMs="50")
        add("direct_play_step", video, step="sourceResolve", phase="started", durationMs="0")
        add("direct_play_step", video, step="sourceResolve", phase="finished", durationMs="12")
        rows.append(dict(dockerTimestamp=STAMP, code="source_range_completed", rangeReadMs="999", durationMs="999"))
        result = offline_result([], rows)
        summary = result["diagnosticSummary"]["gateway"]
        self.assertEqual(summary["requestGroups"], 4)
        self.assertEqual(summary["uncorrelatedRows"], 1)
        self.assertEqual({row["classification"] for row in summary["authentication"]},
                         {"upstream_rejected", "upstream_response"})
        image = summary["images"][0]
        self.assertEqual((image["finalStatus"], image["upstreamStatus"]), ("502", "unknown"))
        self.assertEqual(summary["videoFallbacks"][0]["reasonCode"], "request_not_eligible")
        timing = next(row for row in summary["timings"] if row["code"] == "request_completed")
        self.assertEqual((timing["count"], timing["minMs"], timing["p50Ms"], timing["p95Ms"], timing["maxMs"]),
                         (4, 10, 20, 50, 50))
        step = next(row for row in summary["timings"] if row["code"] == "direct_play_step")
        self.assertEqual((step["stage"], step["count"], step["minMs"]), ("sourceResolve", 1, 12))
        self.assertFalse(any(row["code"] == "source_range_completed" for row in summary["timings"]))

    def test_repeated_request_ids_and_missing_ids_cannot_supply_joined_causality(self):
        rows = [dict(dockerTimestamp=STAMP, code="request_completed", requestId="0123456789abcdef-1",
                     route="authentication", statusCode="401", durationMs="1") for _ in range(2)]
        rows += [dict(dockerTimestamp=STAMP, code="authentication_upstream_response",
                      requestId="0123456789abcdef-1", statusCode="401"),
                 dict(dockerTimestamp=STAMP, code="authentication_upstream_response", statusCode="401"),
                 dict(dockerTimestamp=STAMP, code="request_completed", requestId="0123456789abcdef-2",
                      route="protected", statusCode="401")]
        gateway = offline_result([], rows)["diagnosticSummary"]["gateway"]
        self.assertEqual(gateway["ambiguousRequestGroups"], 1)
        self.assertEqual(gateway["uncorrelatedRows"], 1)
        self.assertEqual(len(gateway["authentication"]), 1)
        self.assertEqual(gateway["authentication"][0]["classification"], "rejection_origin_unknown")
        self.assertEqual(gateway["timings"], [])

    def test_offline_json_keys_cannot_inject_whitelisted_fields(self):
        row = dict(dockerTimestamp=STAMP, code="request_completed", route=SECRET, userAgentFamily=SECRET,
                   userAgentVersion=SECRET, host=SECRET, path="/" + SECRET, queryKeys=SECRET, body=SECRET)
        row["ignored route"] = "authentication"
        result = offline_result([], [row])
        self.assertNotIn(SECRET, json.dumps(result))
        self.assertNotIn("route", result["gatewayDiagnostics"][0])

    def test_redirect_observation_never_claims_playback(self):
        request = b"GET /Videos/123/stream.mkv HTTP/1.1\r\n\r\n"
        response = b"HTTP/1.1 302 Found\r\nContent-Length: 0\r\n\r\n"
        result = offline_result([(1, packet(10000, 8081, 100, request)),
                                 (2, packet(8081, 10000, 200, response, reverse=True))])
        self.assertEqual(result["packetVideoPairs"][0]["status"], 302)
        self.assertEqual(result["diagnosticSummary"]["playbackSuccess"], "unknown")


if __name__ == "__main__":
    unittest.main()
