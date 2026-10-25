"""Filter known diagnostic fields before raw Docker logs leave the Gateway host."""
import json
import re
import shlex
import subprocess
import sys

_CODES = {"direct_play_redirect", "direct_play_fallback", "provider_operation_completed", "source_range_completed",
          "playback_session_started", "playback_progress_reported", "playback_session_stopped",
          "playback_lease_updated", "playback_lease_not_found", "playback_lease_update_failed",
          "playback_event_received", "video_fallback_headers", "playback_lease_session_bound",
          "transfer_quota_committed", "transfer_quota_commit_failed"}
_CODES.update({"direct_play_step", "video_request_started", "video_context_resolved", "video_fallback_started",
               "video_fallback_completed", "playback_info_request_observed", "playback_info_response_observed",
               "playback_info_reused_on_demand", "playback_info_started", "playback_info_finished",
               "playback_lease_skipped", "playback_lease_update_started", "playback_transfer_intent_revoked",
               "playback_transfer_intent_retained", "playback_transfer_intent_recorded",
               "playback_transfer_intent_inherited", "source_directory_resolved"})
_CODES.update({"video_original_mp4_accepted", "video_original_mkv_accepted"})
_CODES.update({"transfer_policy_unavailable", "playback_info_revalidation_started",
               "playback_info_revalidated", "playback_info_revalidation_failed"})
_CODES.update({"request_completed", "authentication_request_shape", "authentication_upstream_response",
               "application_header_parse_failed", "playback_fallback", "playback_rejected", "playback_canceled"})
_NUMERIC = {"itemId", "statusCode", "upstreamStatus", "rangeStart", "rangeEnd", "rangeLength",
            "positionTicks", "proofCount", "accountReservedStreams", "accountActiveStreams",
            "accountOccupiedStreams", "userReservedStreams", "userActiveStreams", "userOccupiedStreams",
            "providerHTTPStatus", "listPages", "pending", "used", "hourlyUsed", "dailyUsed",
            "transferHourlyUsed", "transferHourlyLimit", "transferDailyUsed", "transferDailyLimit",
            "count", "callers", "entries", "ttlSeconds"}
_TIMINGS = {"duration", "directPlay", "prepare", "sourceResolve", "targetSearch", "lockWait", "preIDWait", "preID",
            "rapidUpload", "targetVerify", "downloadURL", "other", "transferCommit", "challenge",
            "parentResolve", "list", "dns", "connect", "tls", "firstByte", "sourceDownload", "rangeRead"}
_NUMERIC.update(prefix + suffix for prefix in _TIMINGS for suffix in ["Ms", "Calls"])
_NUMERIC.add("fallbackHeadersMs")
_TEXT = {"code", "level", "event", "result", "forwardState", "decision", "reasonCode", "stage",
         "fallbackResult", "directPlayResult", "targetState", "step", "phase", "rangeKind",
         "playbackContext", "snapshotState", "state", "proofRejectReason", "intentSource",
         "mediaResolutionCache", "downloadURLCache", "providerOperation", "providerStage", "providerReason",
         "accountRole", "playbackMode", "reason", "networkPhase", "source", "method", "bodyState",
         "purpose", "secPurpose"}
_FIELD_STATES = {"isPlayback", "enableDirectPlay", "enableDirectStream", "enableTranscoding",
                 "autoOpenLiveStream", "startTimeTicks", "currentPlaySessionId", "mediaSourceId", "deviceProfile"}
_STATE_VALUES = {"missing", "null", "true", "false", "zero", "positive", "empty", "present", "invalid"}
_TRANSFER_TRIGGERS = {"explicit_playback", "compatible_request", "existing_target", "policy_denied"}
_BOOL = {"found", "isPaused", "positionPresent", "proofAccepted", "intentGranted", "IsPlayback",
         "success", "connectionReused", "shared"}

# These fixed labels mirror request_log.go and routing.go, not client-supplied
# names. Keep this module standalone: it is also sent to the remote log filter.
CLIENT_PREFIXES = (("Infuse-Direct/", "infuse_direct"), ("Infuse-Library/", "infuse_library"),
                   ("Infuse/", "infuse"), ("SenPlayer/", "senplayer"), ("Yamby/", "yamby"),
                   ("VidHub/", "vidhub"), ("Fileball/", "fileball"), ("Conflux/", "conflux"),
                   ("Emby/", "emby"))
CLIENT_FAMILIES = {family for _, family in CLIENT_PREFIXES} | {"missing", "other"}
ROUTES = {"unclassified", "authentication", "system_info_public", "public_bootstrap", "playback_info",
          "item_detail", "video", "emby_web", "item_image", "protected"}
STEP_NAMES = {"prepare", "sourceResolve", "targetSearch", "lockWait", "preID", "rapidUpload", "challenge",
              "targetVerify", "transferCommit", "downloadURL", "sourceLocation", "sessionWait", "mediaWait",
              "downloadWait", "routing", "leaseAdmission", "accounts", "leaseConfirm", "accessTouch",
              "healthUpdate", "transferAdmission", "taskBegin", "mediaResolutionCache", "downloadURLCache",
              "preIDCache", "preIDWait"}
VIDEO_STAGES = {"identity", "user_state", "route", "proof", "eligibility", "direct_play"}
VIDEO_REASONS = {
    "token_missing", "token_invalid", "token_ambiguous", "token_unmapped", "token_revoked", "identity_mismatch",
    "identity_store_unavailable", "request_canceled", "request_deadline_exceeded", "user_unavailable", "user_expired",
    "route_not_accelerated", "request_not_eligible", "container_recovered", "playback_proof_missing", "playback_proof_expired",
    "playback_proof_mismatch", "direct_play_disabled", "client_incompatible", "concurrency_limited",
    "media_not_direct_play", "invalid_request", "path_not_mapped", "account_unavailable", "accounts_same",
    "provider_unavailable", "provider_protocol", "file_unavailable", "rapid_upload_unavailable",
    "target_unavailable", "download_incompatible", "store_unavailable", "lock_unavailable", "direct_play_ready",
    "personal_account_missing", "account_concurrency_exceeded", "redis_unavailable", "head_lease_missing",
    "playback_route_changed", "playback_lease_lost", "playback_resolve_timeout", "source_read_timeout",
    "transfer_quota_exceeded", "playback_intent_required", "transfer_quota_commit_failed",
}
_PARSE_REASONS = {"empty_header", "header_too_long", "invalid_utf8", "header_line_break", "unsupported_scheme",
                  "expected_field", "invalid_field", "duplicate_field", "expected_equals", "invalid_value",
                  "expected_comma", "expected_value", "expected_quote", "invalid_escape", "control_character",
                  "unterminated_quote"}
_AUTH_FIELD_STATES = {"unknown", "missing", "null", "empty", "present", "invalid_type", "duplicate"}
_AUTH_BODY_STATES = {"valid_json", "read_failed", "too_large", "incomplete", "empty", "encoded",
                     "unsupported_media_type", "invalid_json"}
_COUNTS = {"queryKeyCount", "xEmbyTokenCount", "xMediaBrowserTokenCount", "xEmbyAuthorizationCount",
           "xMediaBrowserAuthorizationCount", "authorizationCount", "queryTokenSourceCount"}
_CACHE_REASONS = {
    "mediaResolutionCacheReason": {"entry_missing", "expired", "clock_rollback", "freshness_expired",
        "dependent_url_missing", "dependent_url_expired", "dependent_url_clock_rollback", "dependent_url_host_rejected",
        "key_unavailable", "cache_unavailable"},
    "downloadURLCacheReason": {"entry_missing", "expired", "clock_rollback", "host_rejected",
                               "key_unavailable", "cache_unavailable", "forced_refresh"},
}
_CACHE_COUNTERS = {"mediaCacheEvictions": "mediaResolutionCache", "downloadCacheEvictions": "downloadURLCache"}
_BOOL.update({"pathTruncated", "queryKeysTruncated", "apiKeyQueryPresent"})
_ENUMS = {
    "code": _CODES,
    "level": {"debug", "info", "warn", "error"},
    "method": {"GET", "HEAD", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "CONNECT", "TRACE", "unknown"},
    "route": ROUTES,
    "pathMode": {"root", "emby_prefixed", "passthrough"},
    "outcome": {"success", "failure", "unknown"},
    "userAgentFamily": CLIENT_FAMILIES,
    "xEmbyTokenState": {"missing", "empty", "present", "ambiguous"},
    "xMediaBrowserTokenState": {"missing", "empty", "present", "ambiguous"},
    "applicationScheme": {"missing", "ambiguous", "other", "emby", "media_browser"},
    "embeddedTokenState": {"missing", "empty", "present", "ambiguous", "unparseable"},
    "queryTokenState": {"missing", "present", "invalid", "ambiguous"},
    "contentType": {"missing", "invalid", "json", "form", "xml", "other"},
    "usernameState": _AUTH_FIELD_STATES,
    "pwState": _AUTH_FIELD_STATES,
    "passwordState": _AUTH_FIELD_STATES,
    "field": {"other", "none", "client", "device", "device_id", "version", "user_id", "token"},
    "characterClass": {"unavailable", "end", "double_quote", "single_quote", "backslash", "comma", "semicolon",
                       "equals", "percent", "space", "tab", "control", "other"},
    "decision": {"redirect", "fallback", "reject"},
    "stage": VIDEO_STAGES | {"source_parent", "source_list", "cookie_get", "download_url", "source_range",
                            "rapid_upload"},
    "reasonCode": VIDEO_REASONS | {"config_unavailable", "verified_live_session", "playback_info_superseded",
        "verified_client_session", "matching_client_intent", "playback_stopped", "client_playback_requested",
        "snapshot_unavailable", "service_unavailable", "lease_update_failed", "session_identity_invalid",
        "internal_failure", "media_identity_mismatch", "upstream_status", "upstream_unavailable",
        "response_invalid", "response_too_large", "playback_info_busy", "deadline_exceeded",
        "response_decode_failed", "response_unusable"},
    "step": STEP_NAMES,
    "phase": {"started", "finished", "created", "reused", "hit", "miss", "bypass"},
    "mediaResolutionCache": {"hit", "miss", "bypass"},
    "downloadURLCache": {"hit", "miss", "bypass"},
    "preIDCache": {"hit", "miss", "bypass"},
    "fallbackSource": {"client_request", "container_recovered", "playback_info_direct_stream",
                       "playback_info_extension_stream", "playback_info_augmented_stream"},
    "fallbackTarget": {"emby"},
    "fallbackPhase": {"response_headers"},
    "proxyErrorCode": {"request_canceled", "upstream_unavailable"},
}
_COMMON_FIELDS = {"code", "level", "requestId"}
_STRICT_FIELDS = {
    "request_completed": _COMMON_FIELDS | _COUNTS | {"method", "pathTruncated", "queryKeysTruncated", "route",
        "pathMode", "statusCode", "outcome", "durationMs", "xEmbyTokenState", "xMediaBrowserTokenState",
        "applicationScheme", "embeddedTokenState", "apiKeyQueryPresent", "queryTokenState", "userAgentFamily",
        "userAgentVersion"},
    "authentication_request_shape": _COMMON_FIELDS | {"contentType", "bodyState", "usernameState", "pwState", "passwordState"},
    "authentication_upstream_response": _COMMON_FIELDS | {"statusCode"},
    "application_header_parse_failed": _COMMON_FIELDS | {"route", "pathMode", "statusCode", "reasonCode", "field",
                                                          "offset", "characterClass"},
}


def safe_version(value):
    """Match the Gateway's complete numeric version token, never a partial UA prefix."""
    return isinstance(value, str) and re.fullmatch(r"[0-9][0-9.-]{0,31}", value) is not None


def is_enum(value, allowed):
    """Reject nested or non-string JSON values before testing fixed-label membership."""
    return isinstance(value, str) and value in allowed


def bounded_decimal(value, maximum, minimum=0):
    """Accept canonical decimal counters only, including -1 solely for parser offsets."""
    if not isinstance(value, str) or not re.fullmatch(r"0|[1-9][0-9]{0,19}|-1", value):
        return False
    return minimum <= int(value) <= maximum


def filter_record(values):
    """Recheck JSON dictionaries directly; never serialize untrusted keys into log syntax."""
    if not isinstance(values, dict) or len(values) > 256:
        return None
    timestamp, code = values.get("dockerTimestamp"), values.get("code")
    if not isinstance(timestamp, str) or not re.fullmatch(r"[0-9T:Z.+-]{20,40}", timestamp):
        return None
    if not isinstance(code, str) or code not in _CODES:
        return None
    row = {"dockerTimestamp": timestamp}
    allowed = _STRICT_FIELDS.get(code)
    for key, value in values.items():
        if not isinstance(key, str) or (allowed is not None and key not in allowed):
            continue
        if type(value) is int:
            value = str(value)
        elif type(value) is bool:
            value = str(value).lower()
        if not isinstance(value, str) or len(value) > 128:
            continue
        if code == "application_header_parse_failed" and key == "reasonCode":
            if value in _PARSE_REASONS:
                row[key] = value
        elif code == "authentication_request_shape" and key == "bodyState":
            if value in _AUTH_BODY_STATES:
                row[key] = value
        elif key in _CACHE_REASONS:
            if is_enum(values.get(key[:-len("Reason")]), {"miss", "bypass"}) and value in _CACHE_REASONS[key]:
                row[key] = value
        elif key in _CACHE_COUNTERS:
            if is_enum(values.get(_CACHE_COUNTERS[key]), {"hit", "miss", "bypass"}) and bounded_decimal(value, 2**64 - 1):
                row[key] = value
        elif key == "phase" and values.get("step") == "preIDWait":
            if value in {"started", "finished"}:
                row[key] = value
        elif key == "phase" and values.get("step") == "preIDCache":
            if value in {"hit", "miss", "bypass"}:
                row[key] = value
        elif key in _ENUMS:
            if value in _ENUMS[key]:
                row[key] = value
        elif key == "userAgentVersion":
            if is_enum(values.get("userAgentFamily"), CLIENT_FAMILIES - {"missing", "other"}) and (value == "invalid" or safe_version(value)):
                row[key] = value
        elif key in _COUNTS:
            if bounded_decimal(value, 2**31 - 1):
                row[key] = value
        elif key == "offset":
            if bounded_decimal(value, 8192, minimum=-1):
                row[key] = value
        elif key in {"statusCode", "upstreamStatus", "providerHTTPStatus"}:
            if value == "0" or bounded_decimal(value, 599, minimum=100):
                row[key] = value
        elif key in _NUMERIC and bounded_decimal(value, 2**63 - 1):
            row[key] = value
        elif key in _BOOL and value in {"true", "false"}:
            row[key] = value
        elif key in _FIELD_STATES and value in _STATE_VALUES:
            row[key] = value
        elif key == "evidence" and value in {"local_mp4_proof", "local_mkv_proof"}:
            row[key] = value
        elif key == "transferTrigger" and value in _TRANSFER_TRIGGERS:
            row[key] = value
        elif key in _TEXT and re.fullmatch(r"[A-Za-z][A-Za-z0-9_]{0,79}", value):
            row[key] = value
        elif key in {"requestId", "itemRef", "sessionRef", "proofSessionRef"} and re.fullmatch(r"[a-fA-F0-9-]{12,64}", value):
            row[key] = value
    return row


def filter_line(line):
    """Drop raw paths, secrets, free-form messages and unknown codes or fields."""
    if not isinstance(line, str) or len(line) > 65536:
        return None
    try:
        tokens = shlex.split(line)
    except ValueError:
        return None
    if not tokens or not re.fullmatch(r"[0-9T:Z.+-]{20,40}", tokens[0]):
        return None
    values = {"dockerTimestamp": tokens[0]}
    for token in tokens[1:]:
        if "=" not in token:
            continue
        key, value = token.split("=", 1)
        if key in values:
            return None
        values[key] = value
    return filter_record(values)


def collect(options):
    """Read a finite Docker log interval with a tail limit and source-side field filtering."""
    command = (["sudo", "-n"] if options["sudo"] else []) + ["docker", "logs", "--since", options["since"],
               "--until", options["until"], "--tail", str(options["tail"]), "--timestamps", options["container"]]
    process = subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    rows = []
    try:
        while True:
            line = process.stdout.readline(65537)
            if not line:
                break
            if len(line) > 65536:
                while line and not line.endswith("\n"):
                    line = process.stdout.readline(65537)
                continue
            row = filter_line(line)
            if row:
                rows.append(row)
                if len(rows) > 20000:
                    raise ValueError("too_many_log_rows")
        if process.wait():
            raise ValueError("docker_logs_failed")
    finally:
        process.stdout.close()
        if process.poll() is None:
            process.terminate()
            process.wait()
    return rows


if __name__ == "__main__":
    result = json.dumps(collect(json.loads(sys.argv[1])))
    if len(result.encode()) > 16 * 1024 * 1024:
        raise ValueError("log_json_too_large")
    print(result)
