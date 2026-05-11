#!/usr/bin/env python3

from __future__ import annotations

import argparse
import re
import subprocess
from dataclasses import dataclass
from pathlib import Path


CONVENTIONAL_COMMIT_RE = re.compile(r"^(?P<type>[a-z]+)(?:\((?P<scope>[^)]+)\))?: (?P<description>.+)$")

SCOPE_LABELS = {
    "api": "API",
    "auth": "认证",
    "billing": "支付",
    "bot": "Bot",
    "config": "配置",
    "console": "控制台",
    "docs": "文档",
    "media": "媒体",
    "redemption": "兑换码",
    "settings": "设置中心",
    "subscription": "订阅",
    "system": "系统",
    "tv-calendar": "追剧日历",
    "web": "Web",
}

NOISE_ONLY_PREFIXES = (
    ".claude/",
    ".codex/",
    "docs/archive/",
    "docs/plan/",
    "docs/proposals/",
)

NOISE_ONLY_FILES = {
    "AGENTS.md",
    "CLAUDE.md",
    "GEMINI.md",
}

INTERNAL_ONLY_SCOPES = {
    "build",
    "ci",
    "codex",
    "release",
}

WEB_FORM_FILES = {
    "services/web/src/views/LoginView.vue",
    "services/web/src/views/console/AccountCenterView.vue",
    "services/web/src/views/admin/SettingsView.vue",
}

BOT_POLLING_RUNTIME_FILES = {
    "services/bot/app/config.py",
    "services/bot/app/server.py",
}

SETTINGS_CLEANUP_CORE_FILES = {
    "services/api/internal/config/config.go",
    "services/api/internal/handlers/config.go",
    "services/web/src/views/admin/SettingsView.vue",
}


@dataclass(frozen=True)
class Commit:
    sha: str
    short_sha: str
    subject: str
    commit_type: str
    scope: str
    description: str
    files: tuple[str, ...]


def run_git(*args: str) -> str:
    completed = subprocess.run(
        ["git", *args],
        check=True,
        capture_output=True,
        text=True,
    )
    return completed.stdout.strip()


def parse_commit_subject(subject: str) -> tuple[str, str, str]:
    match = CONVENTIONAL_COMMIT_RE.match(subject)
    if not match:
        return "", "", subject.strip()
    return (
        match.group("type") or "",
        match.group("scope") or "",
        match.group("description").strip(),
    )


def tag_exists(tag: str) -> bool:
    try:
        run_git("rev-parse", "-q", "--verify", f"refs/tags/{tag}")
    except subprocess.CalledProcessError:
        return False
    return True


def is_ancestor(tag: str, ref: str = "HEAD") -> bool:
    try:
        run_git("merge-base", "--is-ancestor", tag, ref)
    except subprocess.CalledProcessError:
        return False
    return True


def find_previous_reachable_tag(current_tag: str) -> str | None:
    output = run_git("tag", "--merged", "HEAD", "--sort=-v:refname")
    for line in output.splitlines():
        tag = line.strip()
        if not tag or tag == current_tag:
            continue
        return tag
    return None


def resolve_previous_tag(current_tag: str, requested_previous_tag: str | None) -> str | None:
    if requested_previous_tag:
        if not tag_exists(requested_previous_tag):
            print(f"[release-notes] previous tag {requested_previous_tag} does not exist, falling back to reachable tags")
        elif is_ancestor(requested_previous_tag):
            return requested_previous_tag
        else:
            print(
                f"[release-notes] previous tag {requested_previous_tag} is not reachable from HEAD, "
                "falling back to nearest reachable tag"
            )

    return find_previous_reachable_tag(current_tag)


def load_commits(previous_tag: str | None) -> list[Commit]:
    revision = "HEAD" if not previous_tag else f"{previous_tag}..HEAD"
    raw = run_git("log", revision, "--reverse", "--pretty=format:%H%x1f%h%x1f%s%x1e")
    commits: list[Commit] = []
    for record in raw.split("\x1e"):
        if not record.strip():
            continue
        sha, short_sha, subject = record.strip().split("\x1f", 2)
        commit_type, scope, description = parse_commit_subject(subject)
        files = tuple(
            line.strip()
            for line in run_git("diff-tree", "--no-commit-id", "--name-only", "-r", sha).splitlines()
            if line.strip()
        )
        commits.append(
            Commit(
                sha=sha,
                short_sha=short_sha,
                subject=subject,
                commit_type=commit_type,
                scope=scope,
                description=description,
                files=files,
            )
        )
    return commits


def load_changed_files(previous_tag: str | None) -> set[str]:
    if not previous_tag:
        output = run_git("ls-tree", "-r", "--name-only", "HEAD")
    else:
        output = run_git("diff", "--name-only", f"{previous_tag}..HEAD")
    return {line.strip() for line in output.splitlines() if line.strip()}


def is_noise_only_commit(commit: Commit) -> bool:
    if not commit.files:
        return False
    for file_path in commit.files:
        if file_path in NOISE_ONLY_FILES:
            continue
        if any(file_path.startswith(prefix) for prefix in NOISE_ONLY_PREFIXES):
            continue
        return False
    return True


def is_documentation_only_commit(commit: Commit) -> bool:
    if not commit.files:
        return False
    return all(
        file_path.startswith("docs/")
        or file_path.endswith(".md")
        or file_path.endswith(".env.example")
        or file_path in NOISE_ONLY_FILES
        for file_path in commit.files
    )


def is_internal_only_commit(commit: Commit) -> bool:
    return commit.scope in INTERNAL_ONLY_SCOPES


def format_commit_label(commit: Commit) -> str:
    description = commit.description or commit.subject
    scope_label = SCOPE_LABELS.get(commit.scope, commit.scope)
    if not scope_label:
        return description
    lowered = description.lower()
    if scope_label.lower() in lowered:
        return description
    return f"{scope_label} {description}"


def commit_link(repo: str, commit: Commit) -> str:
    return f"https://github.com/{repo}/commit/{commit.sha}"


def build_topic_matches(commits: list[Commit], changed_files: set[str]) -> dict[str, set[str]]:
    matches = {
        "polling": set(),
        "season_subscription": set(),
        "web_forms": set(),
        "settings_cleanup": set(),
        "favicon": set(),
    }

    for commit in commits:
        description_lower = commit.description.lower()
        files = set(commit.files)

        # Topic summaries must prefer precision over recall; shared env or docker
        # files are too noisy to infer a user-visible feature from them.
        if "polling" in description_lower or files & BOT_POLLING_RUNTIME_FILES:
            matches["polling"].add(commit.sha)

        # "按季订阅" is easy to mention explicitly in conventional commits. Generic
        # subscription view tweaks should fall back to commit bullets instead of
        # being misclassified as the full season-subscription feature.
        if "按季" in commit.description or "season subscription" in description_lower:
            matches["season_subscription"].add(commit.sha)

        if files & WEB_FORM_FILES:
            matches["web_forms"].add(commit.sha)

        # admin.ts is shared by many admin features and produced false cleanup
        # warnings in releases that merely touched admin billing/user flows.
        if commit.scope == "settings" or files & SETTINGS_CLEANUP_CORE_FILES:
            matches["settings_cleanup"].add(commit.sha)

        if "services/web/public/favicon.png" in files:
            matches["favicon"].add(commit.sha)

    if "services/web/public/favicon.png" in changed_files:
        matches["favicon"].add("file::favicon")

    return matches


def build_feature_lines(matches: dict[str, set[str]]) -> list[str]:
    lines: list[str] = []
    if matches["polling"]:
        lines.append(
            "- Telegram Bot 新增 `polling` 模式，可通过 `TELEGRAM_UPDATE_MODE` 在 `webhook` 和 `polling` 之间切换；`polling` 模式不再依赖 Telegram 使用的公网 Webhook 地址。"
        )
    if matches["season_subscription"]:
        lines.append(
            "- 支持电视剧按季订阅。网站和 Bot 都会先选择具体季数再提交，订阅去重规则同步升级为 `type + tmdbId + season`，避免不同季互相冲突。"
        )
    return lines


def build_improvement_lines(matches: dict[str, set[str]]) -> list[str]:
    lines: list[str] = []
    if matches["web_forms"]:
        lines.append("- 登录页、账号中心和设置中心的表单布局与交互收口，减少迁移期遗留噪音。")
    if matches["favicon"]:
        lines.append("- Web 端补充站点 `favicon`，浏览器标签页识别更直接。")
    return lines


def build_upgrade_lines(changed_files: set[str], matches: dict[str, set[str]]) -> list[str]:
    lines: list[str] = []
    migration_files = sorted(
        file_path
        for file_path in changed_files
        if file_path.startswith("infrastructure/database/")
        and file_path.endswith(".sql")
        and Path(file_path).parent == Path("infrastructure/database")
        and not Path(file_path).name.endswith("_schema_baseline.sql")
    )
    if migration_files:
        joined = "、".join(f"`{file_path}`" for file_path in migration_files)
        lines.append(f"- 本版本包含数据库 migration。升级前请先执行 {joined}，否则新链路无法完整生效。")
    if matches["polling"]:
        lines.append("- 若要启用 Bot `polling` 模式，请设置 `TELEGRAM_UPDATE_MODE=polling`；如果继续使用 `webhook`，现有部署可保持不变。")
    if matches["settings_cleanup"]:
        lines.append("- 设置中心继续清理历史兼容入口；如果你的部署仍依赖旧回退或旧导入方式，升级后需要按当前配置边界重新核对。")
    return lines


def build_fallback_lines(commits: list[Commit], used_shas: set[str]) -> tuple[list[str], list[str], list[str]]:
    features: list[str] = []
    fixes: list[str] = []
    improvements: list[str] = []

    for commit in commits:
        if commit.sha in used_shas or is_noise_only_commit(commit) or is_internal_only_commit(commit):
            continue

        if is_documentation_only_commit(commit):
            continue

        if commit.commit_type == "test":
            continue

        label = format_commit_label(commit)
        if commit.commit_type == "feat":
            features.append(f"- {label}")
        elif commit.commit_type == "fix":
            fixes.append(f"- {label}")
        else:
            improvements.append(f"- {label}")

    return features, fixes, improvements


def render_reference_commits(repo: str, commits: list[Commit]) -> list[str]:
    lines = [
        "<details>",
        "<summary>参考提交</summary>",
        "",
    ]
    for commit in commits:
        if is_noise_only_commit(commit) or is_documentation_only_commit(commit) or is_internal_only_commit(commit):
            continue
        if commit.commit_type == "test":
            continue
        lines.append(f"- [{commit.short_sha}]({commit_link(repo, commit)}) {commit.subject}")
    lines.extend(["", "</details>"])
    return lines


def render_markdown(current_tag: str, previous_tag: str | None, repo: str) -> str:
    commits = load_commits(previous_tag)
    changed_files = load_changed_files(previous_tag)
    matches = build_topic_matches(commits, changed_files)

    used_shas = set().union(
        matches["polling"],
        matches["season_subscription"],
        matches["web_forms"],
        matches["settings_cleanup"],
        {sha for sha in matches["favicon"] if not sha.startswith("file::")},
    )

    feature_lines = build_feature_lines(matches)
    improvement_lines = build_improvement_lines(matches)
    upgrade_lines = build_upgrade_lines(changed_files, matches)
    fallback_features, fallback_fixes, fallback_improvements = build_fallback_lines(commits, used_shas)

    feature_lines.extend(fallback_features)
    improvement_lines.extend(fallback_improvements)

    compare_url = (
        ""
        if not previous_tag
        else f"https://github.com/{repo}/compare/{previous_tag}...{current_tag}"
    )

    summary_range = f"{previous_tag}..{current_tag}" if previous_tag else "全部历史"

    lines = [
        "## 本次更新",
        "",
        f"`{current_tag}` 已自动根据 `{summary_range}` 的改动生成发布摘要。建议在发布前重点核对升级说明，但不需要再手工从 commit 列表重写一遍。",
        "",
    ]

    if feature_lines:
        lines.extend(["### 重点功能", "", *feature_lines, ""])

    if fallback_fixes:
        lines.extend(["### 问题修复", "", *fallback_fixes, ""])

    if improvement_lines:
        lines.extend(["### 体验优化", "", *improvement_lines, ""])

    if upgrade_lines:
        lines.extend(["### 升级说明", "", *upgrade_lines, ""])

    lines.extend(
        [
            "### Docker 镜像",
            "",
            "本版本的 Docker 镜像会由 GitHub Actions 自动构建并推送到 GitHub Container Registry：",
            "",
            "```bash",
            f"docker pull ghcr.io/{repo.split('/')[0]}/ember-api:{current_tag}",
            f"docker pull ghcr.io/{repo.split('/')[0]}/ember-web:{current_tag}",
            f"docker pull ghcr.io/{repo.split('/')[0]}/ember-bot:{current_tag}",
            "```",
            "",
            "### 完整变更",
            "",
        ]
    )

    if compare_url:
        lines.append(f"- Compare: {compare_url}")
        lines.append(f"- Full Changelog: {compare_url}")
    else:
        lines.append("- 首次发布，无历史版本对比。")

    lines.extend(["", *render_reference_commits(repo, commits), ""])
    return "\n".join(lines)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Generate Ember release notes from git history.")
    parser.add_argument("--current-tag", required=True, help="Current release tag, for example v1.2.10")
    parser.add_argument("--previous-tag", default="", help="Previous release tag")
    parser.add_argument("--repo", required=True, help="GitHub repository in owner/name format")
    parser.add_argument("--output", required=True, help="Output markdown file path")
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    previous_tag = resolve_previous_tag(
        current_tag=args.current_tag,
        requested_previous_tag=args.previous_tag or None,
    )
    markdown = render_markdown(
        current_tag=args.current_tag,
        previous_tag=previous_tag,
        repo=args.repo,
    )
    Path(args.output).write_text(markdown, encoding="utf-8")


if __name__ == "__main__":
    main()
