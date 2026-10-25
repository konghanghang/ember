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
_NUMERIC = {"itemId", "statusCode", "upstreamStatus", "rangeStart", "rangeEnd", "rangeLength",
            "positionTicks", "proofCount", "accountReservedStreams", "accountActiveStreams",
            "accountOccupiedStreams", "userReservedStreams", "userActiveStreams", "userOccupiedStreams",
            "providerHTTPStatus", "listPages", "pending", "used", "hourlyUsed", "dailyUsed",
            "transferHourlyUsed", "transferHourlyLimit", "transferDailyUsed", "transferDailyLimit",
            "count", "callers", "entries", "ttlSeconds"}
_TIMINGS = {"duration", "directPlay", "prepare", "sourceResolve", "targetSearch", "lockWait", "preID",
            "rapidUpload", "targetVerify", "downloadURL", "other", "transferCommit", "challenge",
            "parentResolve", "list", "dns", "connect", "tls", "firstByte", "sourceDownload", "rangeRead"}
_NUMERIC.update(prefix + suffix for prefix in _TIMINGS for suffix in ["Ms", "Calls"])
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


def filter_line(line):
    """Drop raw paths, secrets, free-form messages and unknown codes or fields."""
    try:
        tokens = shlex.split(line)
    except ValueError:
        return None
    if not tokens or not re.fullmatch(r"[0-9T:Z.+-]{20,40}", tokens[0]):
        return None
    values = dict(token.split("=", 1) for token in tokens[1:] if "=" in token)
    if values.get("code") not in _CODES:
        return None
    row = {"dockerTimestamp": tokens[0]}
    for key, value in values.items():
        if key in _NUMERIC and re.fullmatch(r"-?[0-9]{1,20}", value):
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
