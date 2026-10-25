"""Check actual Go diagnostic producers against the standalone capture consumer."""

import json
from pathlib import Path
import sys

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "gateway-capture"))
import diagnostic_summary
import logs


MARKER = "GATEWAY_DIAGNOSTIC_CONTRACT"
PRODUCERS = {
    ("github.com/konghang/ember/backend/internal/services/directplay", "TestGatewayDiagnosticContractTimingProducer"): "timing",
    ("github.com/konghang/ember/backend/internal/playbackgateway", "TestGatewayDiagnosticContractLogProducer"): "gateway",
}
# Synthetic Docker framing only; no business dates or clock-based correlation.
STAMP = "2000-01-01T00:00:00Z"
PRIVATE_FIELDS = {"host", "path", "queryKeys", "mediaPath", "embyPathPrefix", "mappedRelativePath",
                  "userId", "mappingId", "deviceId", "clientName", "taskId", "sourceRootId",
                  "token", "cookie", "authorization", "url", "body", "password"}


class ContractError(ValueError):
    """Report bounded contract failures without echoing raw diagnostic input."""


def require(condition, message):
    """Keep validation active even when Python runs with assertions disabled."""
    if not condition:
        raise ContractError(message)


def read_producers(lines):
    """Accept one marker from each named, freshly run and successful Go producer."""
    started, passed, packages, output = set(), set(), set(), {key: [] for key in PRODUCERS}
    for index, line in enumerate(lines):
        require(index < 10000 and len(line) <= 262144, "Go event input exceeds contract bounds")
        try:
            event = json.loads(line)
        except (ValueError, TypeError):
            raise ContractError("invalid Go event JSON") from None
        require(isinstance(event, dict) and isinstance(event.get("Action"), str)
                and isinstance(event.get("Package"), str), "invalid Go event shape")
        action, package = event["Action"], event["Package"]
        require(action != "fail", "Go test or package failed")
        if action == "output":
            require(isinstance(event.get("Output"), str), "invalid Go output shape")
        key = (package, event.get("Test"))
        require(key[1] is None or isinstance(key[1], str), "invalid Go test name")
        if key in PRODUCERS:
            if action == "run":
                require(key not in started, "duplicate producer run")
                started.add(key)
            elif action == "output":
                require(key in started and key not in passed, "producer output outside its run")
                output[key].append(event["Output"])
            elif action == "pass":
                require(key in started and key not in passed, "producer pass without one run")
                passed.add(key)
            elif action == "skip":
                raise ContractError("producer was skipped")
        elif action == "output":
            require(MARKER not in event["Output"], "marker from an unexpected producer")
        if key[1] is None and action == "pass":
            packages.add(package)
    require(passed == set(PRODUCERS), "missing successful producer")
    require({key[0] for key in PRODUCERS} <= packages, "missing successful producer package")
    result = {}
    for key, kind in PRODUCERS.items():
        records = [line.split(MARKER + " ", 1)[1] for line in "".join(output[key]).splitlines()
                   if MARKER + " " in line]
        require(len(records) == 1, "missing or duplicate producer marker")
        try:
            packet = json.loads(records[0])
        except ValueError:
            raise ContractError("invalid producer JSON") from None
        require(isinstance(packet, dict) and packet.get("kind") == kind, "invalid producer payload")
        result[kind] = packet
    return result


def named_cases(packet, names):
    """Require the small, explicit fixture set instead of allowing empty success."""
    require(isinstance(packet, dict) and isinstance(packet.get("cases"), list), "invalid producer cases")
    cases = packet["cases"]
    require(len(cases) == len(names), "missing or duplicate contract case")
    require(all(isinstance(case, dict) and isinstance(case.get("name"), str) for case in cases),
            "invalid contract case")
    result = {case["name"]: case for case in cases}
    require(set(result) == set(names), "missing or duplicate contract case")
    return result


def summary_for(rows):
    """Use the real offline summary and preserve its evidence boundaries."""
    summary = diagnostic_summary.build_summary([], rows)
    require(summary["correlation"] == "gateway_request_id_only" and summary["playbackSuccess"] == "unknown"
            and summary["http"]["requestCount"] == 0, "summary crossed an evidence boundary")
    return summary["gateway"]


def validate_timing(packet):
    """Check every actual LogFields entry, including future fields and cache snapshots."""
    cases = named_cases(packet, ("miss", "hit", "bypass"))
    rows, field_names = [], set()
    for index, (name, case) in enumerate(cases.items(), 1):
        fields = case.get("fields")
        require(isinstance(fields, list) and 0 < len(fields) <= 256, "invalid timing fields")
        values = {}
        for field in fields:
            require(isinstance(field, str) and "=" in field and "\n" not in field, "invalid timing field")
            key, value = field.split("=", 1)
            require(key and key not in values, "duplicate or empty timing field")
            values[key] = value
        framing = dict(dockerTimestamp=STAMP, code="direct_play_redirect", requestId=f"0123456789abcdef-{index}",
                       decision="redirect", stage="direct_play", statusCode="302")
        row = logs.filter_line(STAMP + " " + " ".join(f"{key}={value}" for key, value in framing.items()
                                                     if key != "dockerTimestamp") + " " + " ".join(fields))
        require(row is not None, "timing record was dropped")
        for key, value in values.items():
            require(row.get(key) == value, "timing field lost or changed: " + key)
        require(logs.filter_record(dict(framing, **values)) == row, "timing JSON/log filters disagree")
        require(all(values.get(cache) == name for cache in ("preIDCache", "mediaResolutionCache", "downloadURLCache"))
                and values.get("preIDWaitCalls") == "2" and "preIDWaitMs" in values, "required timing evidence missing")
        reasons = {"miss": ("dependent_url_expired", "forced_refresh"), "hit": (None, None),
                   "bypass": ("key_unavailable", "cache_unavailable")}[name]
        require((values.get("mediaResolutionCacheReason"), values.get("downloadURLCacheReason")) == reasons,
                "cache reason contract changed")
        snapshots = {"miss": ("1", "2"), "hit": ("3", "4"), "bypass": (None, None)}[name]
        require((values.get("mediaCacheEvictions"), values.get("downloadCacheEvictions")) == snapshots,
                "cache snapshot evidence changed")
        timings = {entry["field"]: entry for entry in summary_for([row])["timings"]}
        for key, value in values.items():
            if key.endswith("Ms"):
                expected = {"code": "direct_play_redirect", "stage": "direct_play", "field": key, "count": 1,
                            "minMs": int(value), "p50Ms": int(value), "p95Ms": int(value), "maxMs": int(value)}
                require(timings.get(key) == expected, "timing summary field lost or changed: " + key)
        rows.append(row)
        field_names.update(values)
    caches = summary_for(rows)["caches"]
    for cache, maximum, reasons in (("mediaResolutionCache", 3, ("key_unavailable", "dependent_url_expired")),
                                    ("downloadURLCache", 4, ("cache_unavailable", "forced_refresh")),
                                    ("preIDCache", None, None)):
        expected = [{"outcome": name, "count": 1} for name in ("bypass", "hit", "miss")]
        if reasons:
            expected[0]["reason"], expected[2]["reason"] = reasons
        require(caches[cache]["observations"] == expected and caches[cache]["source"] == "final_decision",
                "cache outcome/reason summary changed")
        if maximum is not None:
            require(caches[cache]["capacityEvictions"] == {"maxObserved": maximum, "intervalDelta": "unknown",
                    "semantics": "process_cumulative_capacity_only"}, "cache cumulative snapshot semantics changed")
    return len(field_names)


def filtered_gateway_rows(lines):
    """Run real text and JSON filters, checking credentials and raw metadata are removed."""
    require(isinstance(lines, list) and 0 < len(lines) <= 100, "invalid Gateway log fixture")
    rows = []
    for line in lines:
        require(isinstance(line, str) and "\n" not in line, "invalid Gateway log line")
        row = logs.filter_line(STAMP + " " + line)
        if row is None:
            continue
        require(not PRIVATE_FIELDS.intersection(row), "private Gateway field survived filtering")
        require(logs.filter_record(row) == row, "Gateway JSON/log filters disagree")
        injected = dict(row, token="synthetic-private", cookie="synthetic-private", authorization="synthetic-private",
                        url="https://fixture.invalid/private", password="synthetic-private", body={"private": "value"})
        require(logs.filter_record(injected) == row, "sensitive JSON field survived filtering")
        require(logs.filter_line(STAMP + " " + line + " token=synthetic-private cookie=synthetic-private"
                                " authorization=synthetic-private url=https://fixture.invalid/private"
                                " password=synthetic-private body=synthetic-private") == row,
                "sensitive log field survived filtering")
        rows.append(row)
    return rows


def one_row(rows, code):
    """Require one actual event for each expected Gateway logger branch."""
    found = [row for row in rows if row.get("code") == code]
    require(len(found) == 1, "missing or duplicate Gateway event: " + code)
    return found[0]


def validate_gateway(packet):
    """Lock request/auth/reason enums and only the explicit two-session exception."""
    cases = named_cases(packet, ("authentication", "header_parse", "fallback", "bound"))
    groups = {name: filtered_gateway_rows(case.get("lines")) for name, case in cases.items()}
    summaries = {name: summary_for(rows) for name, rows in groups.items()}
    for name, rows in groups.items():
        summary = summaries[name]
        require(summary["requestGroups"] == 1 and summary["ambiguousRequestGroups"] == 0
                and summary["uncorrelatedRows"] == 0, "Gateway fixture is not uniquely correlated")
        completed = one_row(rows, "request_completed")
        route, status = {"authentication": ("authentication", "401"), "header_parse": ("protected", "401"),
                         "fallback": ("video", "206"), "bound": ("video", "302")}[name]
        require((completed.get("route"), completed.get("statusCode"), completed.get("userAgentFamily"),
                 completed.get("userAgentVersion")) == (route, status, "senplayer", "6.0.4"),
                "Gateway request fields lost or changed")
    shape = one_row(groups["authentication"], "authentication_request_shape")
    require(all(shape.get(key) == value for key, value in {"contentType": "json", "bodyState": "valid_json",
            "usernameState": "present", "pwState": "present", "passwordState": "missing"}.items()),
            "authentication shape lost or changed")
    require(one_row(groups["authentication"], "authentication_upstream_response").get("statusCode") == "401",
            "authentication upstream status missing")
    for name, classification, reason in (("authentication", "upstream_rejected", "unknown"),
                                        ("header_parse", "rejected_with_header_parse_failure", "unterminated_quote")):
        observations = summaries[name]["authentication"]
        require(len(observations) == 1 and observations[0]["classification"] == classification
                and observations[0]["headerParseReason"] == reason and observations[0]["count"] == 1,
                "authentication summary changed")
    fallback = one_row(groups["fallback"], "direct_play_fallback")
    require(fallback.get("reasonCode") == "source_read_timeout", "Gateway fallback reason lost or changed")
    require(summaries["fallback"]["videoFallbacks"] == [{"stage": "direct_play", "reasonCode": "source_read_timeout",
            "finalStatus": "206", "upstreamStatus": "206", "count": 1}], "Gateway fallback summary changed")
    bound = groups["bound"]
    binding = one_row(bound, "playback_lease_session_bound")
    context, decision = one_row(bound, "video_context_resolved"), one_row(bound, "direct_play_redirect")
    proof, client = binding.get("proofSessionRef"), binding.get("sessionRef")
    require(proof and client and proof != client and binding.get("reasonCode") == "verified_client_session"
            and context.get("sessionRef") == proof and decision.get("sessionRef") == client
            and decision.get("reasonCode") == "direct_play_ready", "verified two-session binding lost or changed")
    third = "f" * 16 if "f" * 16 not in {proof, client} else "e" * 16
    conflicts = [
        [row for row in bound if row is not binding], bound + [binding],
        [dict(row, reasonCode="verified_live_session") if row is binding else row for row in bound],
        [dict(row, proofSessionRef=third) if row is binding else row for row in bound],
        [dict(row, sessionRef=proof) if row is decision else row for row in bound],
        bound + [dict(binding, code="video_fallback_started", sessionRef=third)],
    ]
    for rows in conflicts:
        summary = summary_for(rows)
        require(summary["ambiguousRequestGroups"] == 1 and summary["timings"] == []
                and all(not cache["observations"] for cache in summary["caches"].values()),
                "unverified session conflict was correlated")
    return len(conflicts)


def check_contract(lines):
    """Validate producer success before passing its real output to capture modules."""
    packets = read_producers(lines)
    fields = validate_timing(packets["timing"])
    conflicts = validate_gateway(packets["gateway"])
    return {"status": "passed", "timingCases": 3, "timingFields": fields,
            "gatewayCases": 4, "sessionConflictCases": conflicts}


def main(arguments):
    """Exit nonzero on invalid/missing evidence while retaining no raw report data."""
    if len(arguments) != 1:
        print("usage: gateway_contract.py GO_TEST_JSONL", file=sys.stderr)
        return 2
    try:
        with open(arguments[0], encoding="utf-8") as source:
            result = check_contract(source)
    except (OSError, UnicodeError, ContractError) as error:
        message = str(error) if isinstance(error, ContractError) else "cannot read Go contract output"
        print("Gateway contract failed: " + message, file=sys.stderr)
        return 1
    print(json.dumps(result, sort_keys=True))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
