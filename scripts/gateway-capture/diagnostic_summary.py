"""Bounded offline observations; HTTP packets and Gateway IDs remain separate."""
from collections import Counter, defaultdict, deque

from analyze_capture import PATH_CATEGORIES
from logs import CLIENT_FAMILIES, _TIMINGS, bounded_decimal

_DECISIONS = {"direct_play_redirect", "direct_play_fallback", "playback_fallback", "playback_rejected", "playback_canceled"}
_AUTH_CODES = {"authentication_request_shape", "authentication_upstream_response", "application_header_parse_failed"}
_TIMING_FIELDS = {name + "Ms" for name in _TIMINGS} | {"fallbackHeadersMs"}
_CACHES = {"mediaResolutionCache": "mediaCacheEvictions", "downloadURLCache": "downloadCacheEvictions", "preIDCache": None}


def counted_rows(counter, fields):
    """Render fixed-category counters without constructing keys from raw request text."""
    return [dict(zip(fields, key), count=count) for key, count in sorted(counter.items())]


def status_value(value):
    """Keep observed HTTP statuses only; zero or absent response evidence is unknown."""
    value = str(value)
    return value if bounded_decimal(value, 599, minimum=100) else "unknown"


def http_distribution(events):
    """Pair HTTP/1 final headers by connection/order, leaving missing responses unknown."""
    queues, requests, statuses = defaultdict(deque), [], {}
    unpaired_responses = 0
    for event in events:
        if event["kind"] == "request":
            queues[event["connection"]].append(len(requests))
            requests.append(event)
            continue
        if event["status"] < 200 and event["status"] != 101:
            continue
        source, target = event["connection"].split(">")
        queue = queues[f"{target}>{source}"]
        if not queue:
            unpaired_responses += 1
            continue
        statuses[queue.popleft()] = status_value(event["status"])
    counts = Counter()
    for index, event in enumerate(requests):
        family = event.get("clientFamily", "unknown")
        route = event.get("routeCategory", "unknown")
        counts[(event["edge"], family if family in CLIENT_FAMILIES else "unknown",
                route if route in PATH_CATEGORIES else "unknown", statuses.get(index, "unknown"))] += 1
    return {"requestCount": len(requests), "unpairedResponseCount": unpaired_responses,
            "distribution": counted_rows(counts, ("edge", "clientFamily", "route", "status"))}


def session_refs_consistent(rows):
    """Allow only the explicit proof-to-client lease binding to explain two session refs."""
    sessions = {row["sessionRef"] for row in rows if "sessionRef" in row}
    bindings = [row for row in rows if row["code"] == "playback_lease_session_bound"]
    if len(bindings) > 1:
        return False
    if len(sessions) <= 1:
        return True
    if len(sessions) != 2 or len(bindings) != 1:
        return False
    binding = bindings[0]
    proof, client = binding.get("proofSessionRef"), binding.get("sessionRef")
    if binding.get("reasonCode") != "verified_client_session" or {proof, client} != sessions:
        return False
    for row in rows:
        if row["code"] == "video_context_resolved" and row.get("sessionRef", proof) != proof:
            return False
        if row["code"] in _DECISIONS | {"direct_play_step"} and row.get("sessionRef", client) != client:
            return False
    return True


def ambiguous_group(rows):
    """Reject observable ID reuse instead of inventing cross-request causal links."""
    counts = Counter(row["code"] for row in rows)
    singletons = {"request_completed", "video_request_started"} | _AUTH_CODES
    if any(counts[code] > 1 for code in singletons) or sum(counts[code] for code in _DECISIONS) > 1:
        return True
    if not session_refs_consistent(rows):
        return True
    for field in ("route", "userAgentFamily", "itemRef"):
        if len({row[field] for row in rows if field in row}) > 1:
            return True
    completed = next((row for row in rows if row["code"] == "request_completed"), {})
    route = completed.get("route")
    # Authentication sidecars and video decisions cannot belong to a completed
    # request of another route, even if a supplied JSON file reuses its ID.
    if counts["authentication_upstream_response"] or counts["authentication_request_shape"]:
        if route and route != "authentication":
            return True
        if any(counts[code] for code in _DECISIONS):
            return True
    if any(counts[code] for code in _DECISIONS) and route and route != "video":
        return True
    terminal = [row for row in rows if row["code"] in _DECISIONS | {"request_completed", "application_header_parse_failed"}]
    return len({row["statusCode"] for row in terminal if row.get("statusCode", "0") != "0"}) > 1


def authentication_observation(rows, completed):
    """Distinguish explicit upstream replies from rejections with an unknown origin."""
    upstream = next((row for row in rows if row["code"] == "authentication_upstream_response"), {})
    shape = next((row for row in rows if row["code"] == "authentication_request_shape"), {})
    parse = next((row for row in rows if row["code"] == "application_header_parse_failed"), {})
    final_status, upstream_status = status_value(completed.get("statusCode")), status_value(upstream.get("statusCode"))
    if not upstream and not shape and not parse and completed.get("route") != "authentication" and final_status not in {"401", "403"}:
        return None
    category = "unknown"
    if upstream_status in {"401", "403"}:
        category = "upstream_rejected"
    elif upstream_status != "unknown":
        category = "upstream_failure" if int(upstream_status) >= 400 else "upstream_response"
    elif final_status in {"401", "403"}:
        category = "rejected_with_header_parse_failure" if parse else "rejection_origin_unknown"
    elif parse:
        category = "header_parse_failure_observed"
    return (completed.get("userAgentFamily", "unknown"), completed.get("route", "unknown"),
            category, final_status, upstream_status, parse.get("reasonCode", "unknown"),
            shape.get("bodyState", "unknown"), shape.get("usernameState", "unknown"),
            shape.get("pwState", "unknown"), shape.get("passwordState", "unknown"))


def record_timings(rows, samples):
    """Keep independently measured spans separate; nested stages are never summed."""
    for row in rows:
        if row["code"] == "direct_play_step" and row.get("phase") != "finished":
            continue
        stage = row.get("step", "unknown") if row["code"] == "direct_play_step" else row.get("stage", "unknown")
        for field in _TIMING_FIELDS & row.keys():
            if bounded_decimal(row[field], 2**63 - 1):
                samples[(row["code"], stage, field)].append(int(row[field]))


def timing_distribution(samples):
    """Use nearest-rank p50/p95 over observed milliseconds, including explicit zeroes."""
    result = []
    for (code, stage, field), values in sorted(samples.items()):
        values.sort()
        count = len(values)
        result.append({"code": code, "stage": stage, "field": field, "count": count,
                       "minMs": values[0], "p50Ms": values[(count * 50 + 99) // 100 - 1],
                       "p95Ms": values[(count * 95 + 99) // 100 - 1], "maxMs": values[-1]})
    return result


def cache_summary(decisions):
    """Count final-decision outcomes; cumulative eviction samples are maxima, never deltas."""
    result = {}
    for cache, counter_field in _CACHES.items():
        observations, snapshots = Counter(), []
        for row in decisions:
            outcome = row.get(cache)
            if outcome not in {"hit", "miss", "bypass"}:
                continue
            # Hit has no reason. PreID has no reason contract for any outcome.
            reason = row.get(cache + "Reason", "unknown") if cache != "preIDCache" and outcome != "hit" else ""
            observations[(outcome, reason)] += 1
            value = row.get(counter_field)
            if bounded_decimal(value, 2**64 - 1):
                snapshots.append(int(value))
        output = counted_rows(observations, ("outcome", "reason"))
        for row in output:
            if not row["reason"]:
                del row["reason"]
        result[cache] = {"observations": output, "source": "final_decision"}
        if counter_field:
            result[cache]["capacityEvictions"] = {"maxObserved": max(snapshots) if snapshots else None,
                "intervalDelta": "unknown", "semantics": "process_cumulative_capacity_only"}
    return result


def gateway_summary(rows):
    """Correlate only explicit IDs; unassociated and conflicting rows remain counted gaps."""
    groups = defaultdict(list)
    for row in rows:
        if row.get("requestId"):
            groups[row["requestId"]].append(row)
    authentication, images, fallbacks = Counter(), Counter(), Counter()
    samples = defaultdict(list)
    decisions = []
    ambiguous = 0
    for group in groups.values():
        if ambiguous_group(group):
            ambiguous += 1
            continue
        completed = next((row for row in group if row["code"] == "request_completed"), {})
        observation = authentication_observation(group, completed)
        if observation:
            authentication[observation] += 1
        if completed.get("route") == "item_image":
            images[(completed.get("userAgentFamily", "unknown"), status_value(completed.get("statusCode")), "unknown")] += 1
        for row in group:
            if row["code"] in _DECISIONS:
                decisions.append(row)
            if row["code"] in {"direct_play_fallback", "playback_fallback"} and row.get("decision") == "fallback":
                fallbacks[(row.get("stage", "unknown"), row.get("reasonCode", "unknown"),
                           status_value(row.get("statusCode")), status_value(row.get("upstreamStatus")))] += 1
        record_timings(group, samples)
    return {"logRows": len(rows), "requestGroups": len(groups),
            "uncorrelatedRows": sum(1 for row in rows if not row.get("requestId")),
            "ambiguousRequestGroups": ambiguous,
            "authentication": counted_rows(authentication, ("clientFamily", "route", "classification", "finalStatus",
                "upstreamStatus", "headerParseReason", "bodyState", "usernameState", "pwState", "passwordState")),
            "images": counted_rows(images, ("clientFamily", "finalStatus", "upstreamStatus")),
            "videoFallbacks": counted_rows(fallbacks, ("stage", "reasonCode", "finalStatus", "upstreamStatus")),
            "timings": timing_distribution(samples), "caches": cache_summary(decisions)}


def build_summary(events, rows):
    """Add observations only; timestamps, 302 and packet refs do not prove a playback session."""
    return {"correlation": "gateway_request_id_only", "playbackSuccess": "unknown",
            "http": http_distribution(events), "gateway": gateway_summary(rows)}
