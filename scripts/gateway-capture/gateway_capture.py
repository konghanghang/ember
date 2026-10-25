#!/usr/bin/env python3
"""Portable CLI: explicit remote collection, offline analysis, no service startup."""
import argparse
from datetime import datetime, timezone
import json
from pathlib import Path
import re
import subprocess

from analyze_capture import analyze, video_pairs
from analyze_tls import analyze_tls
from capture import fetch_logs, private_directory, private_json, run_capture, ssh_command
from diagnostic_summary import build_summary
from logs import filter_record


def bounded_integer(minimum, maximum):
    """Validate finite CLI limits before any SSH command or artifact creation."""
    def parse(value):
        """Return the integer only when it belongs to the declared inclusive range."""
        try:
            result = int(value)
        except ValueError as error:
            raise argparse.ArgumentTypeError("需要整数") from error
        if not minimum <= result <= maximum:
            raise argparse.ArgumentTypeError(f"范围必须是 {minimum}..{maximum}")
        return result
    return parse


def add_remote_options(parser):
    """Share SSH aliases and existing-container options without storing credentials."""
    parser.add_argument("--host", required=True, help="最终主机的 SSH 别名")
    parser.add_argument("--via", action="append", default=[], help="按本机到远端顺序填写跳板，可重复")
    parser.add_argument("--container", default="ember-gateway", help="已有 Gateway 容器名")
    parser.add_argument("--sudo", action="store_true", help="最终主机使用 sudo -n，不读取密码")
    parser.add_argument("--output", required=True, type=Path, help="仓库外的本机私有输出目录")


def add_ports(parser):
    """Use ports as seen inside the Gateway namespace, not published host ports."""
    parser.add_argument("--gateway-port", type=bounded_integer(1, 65535), default=8081)
    parser.add_argument("--emby-port", type=bounded_integer(1, 65535), default=18096)
    parser.add_argument("--tls-port", type=bounded_integer(1, 65535), default=443)


def build_parser():
    """Define explicit collection subcommands; help and analyze never contact a remote host."""
    parser = argparse.ArgumentParser(description="Gateway 有界抓包与离线证据分析（Python 标准库）")
    commands = parser.add_subparsers(dest="command", required=True)
    collect = commands.add_parser("capture", help="显式通过 SSH 采集现有容器流量")
    add_remote_options(collect)
    add_ports(collect)
    collect.add_argument("--duration", type=bounded_integer(1, 3600), default=240, help="秒，默认 240")
    collect.add_argument("--max-mib", type=bounded_integer(1, 1024), default=128)
    collect.add_argument("--snaplen", type=bounded_integer(128, 8192), default=8192)
    collect.add_argument("--interface", default="eth0")
    collect.add_argument("--dry-run", action="store_true", help="只显示计划，不连接 SSH 或创建目录")
    logs = commands.add_parser("logs", help="提取有界时间段内的已过滤 Gateway 日志")
    add_remote_options(logs)
    logs.add_argument("--metadata", type=Path, help="从采集元数据读取开始/结束时间")
    logs.add_argument("--since", help="含时区的 ISO 8601 开始时间")
    logs.add_argument("--until", help="含时区的 ISO 8601 结束时间")
    logs.add_argument("--tail", type=bounded_integer(1, 20000), default=10000)
    offline = commands.add_parser("analyze", help="只读取本机 PCAP，生成过滤后的 JSON")
    offline.add_argument("--pcap", required=True, type=Path)
    offline.add_argument("--logs", type=Path, help="可选的工具生成日志 JSON")
    offline.add_argument("--output", required=True, type=Path)
    add_ports(offline)
    return parser


def capture_interval(path):
    """Convert external PCAP epoch evidence to explicit UTC Docker log bounds."""
    metadata = json.loads(path.read_text(encoding="utf-8"))
    ready = next((event for event in metadata["events"] if event.get("captureReady")), None)
    finished = next((event for event in metadata["events"] if event.get("captureFinished")), None)
    if not ready or not finished:
        raise ValueError("capture_interval_missing")
    # UTC here is the wire/Docker timestamp contract, not an Ember business timezone.
    return tuple(datetime.fromtimestamp(epoch, timezone.utc).isoformat() for epoch in
                 [ready["startedUtcEpoch"], finished["endedUtcEpoch"]])


def validate_args(args):
    """Reject unsafe names, contradictory ports and ambiguous log windows before execution."""
    if args.command in {"capture", "logs"}:
        ssh_command(args.host, args.via, ["true"])
        if args.container.startswith("-") or not re.fullmatch(r"[A-Za-z0-9_.-]{1,128}", args.container):
            raise ValueError("invalid_container_name")
    if args.command in {"capture", "analyze"}:
        if len({args.gateway_port, args.emby_port, args.tls_port}) != 3:
            raise ValueError("ports_must_be_distinct")
    if args.command == "capture" and not re.fullmatch(r"[A-Za-z0-9_.:-]{1,32}", args.interface):
        raise ValueError("invalid_interface")
    if args.command == "logs":
        if args.metadata:
            if args.since or args.until:
                raise ValueError("choose_metadata_or_explicit_interval")
            args.since, args.until = capture_interval(args.metadata)
        if not args.since or not args.until:
            raise ValueError("logs_require_since_and_until_or_metadata")
        bounds = [datetime.fromisoformat(value.replace("Z", "+00:00")) for value in [args.since, args.until]]
        if any(value.utcoffset() is None for value in bounds) or bounds[0] >= bounds[1]:
            raise ValueError("invalid_log_interval")


def run_analysis(args):
    """Analyze local evidence without SSH; do not automatically join concurrent logs by proximity."""
    directory = private_directory(args.output)
    ports = {"gateway_port": args.gateway_port, "emby_port": args.emby_port, "tls_port": args.tls_port}
    http = analyze(args.pcap, **ports)
    result = {"formatVersion": 1, "ports": ports, "httpEvidence": http,
              "tlsConnections": analyze_tls(args.pcap, tls_port=args.tls_port),
              "packetVideoPairs": video_pairs(http["http"])}
    filtered = []
    if args.logs:
        if args.logs.stat().st_size > 16 * 1024 * 1024:
            raise ValueError("log_json_too_large")
        rows = json.loads(args.logs.read_text(encoding="utf-8"))
        if not isinstance(rows, list) or len(rows) > 20000:
            raise ValueError("invalid_log_json")
        for row in rows:
            safe = filter_record(row)
            if safe:
                filtered.append(safe)
        result["gatewayDiagnostics"] = filtered
    result["diagnosticSummary"] = build_summary(http["http"], filtered)
    path = directory / (args.pcap.stem + ".analysis.json")
    private_json(path, result)
    print(json.dumps({"analysisPath": str(path), "httpEvents": len(http["http"]),
                      "videoPairs": len(result["packetVideoPairs"]),
                      "tlsConnections": len(result["tlsConnections"])}))


def main(argv=None):
    """Dispatch only the requested operation and report errors without raw SSH/log text."""
    args = build_parser().parse_args(argv)
    try:
        validate_args(args)
        if args.command == "capture" and args.dry_run:
            print(json.dumps({"mode": "dry_run", "host": args.host, "via": args.via,
                              "container": args.container, "interface": args.interface,
                              "durationSeconds": args.duration, "maxMiB": args.max_mib,
                              "gatewayPort": args.gateway_port, "embyPort": args.emby_port,
                              "tlsPort": args.tls_port, "output": str(args.output.expanduser().resolve())}))
        elif args.command == "capture":
            run_capture(args)
        elif args.command == "logs":
            fetch_logs(args)
        else:
            run_analysis(args)
        return 0
    except (OSError, ValueError, KeyError, TypeError, OverflowError, subprocess.SubprocessError) as error:
        code = str(error) if re.fullmatch(r"[a-z_]{1,100}", str(error)) else "operation_failed"
        print(json.dumps({"errorCode": code, "errorType": type(error).__name__}))
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
