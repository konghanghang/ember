#!/usr/bin/env bash

set -euo pipefail

usage() {
  cat <<'EOF'
Usage: release_preflight.sh <vX.Y.Z> <prepare|materials|tag>

Runs read-only Ember release checks. It never edits files or Git refs.
EOF
}

if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
  usage
  exit 0
fi

if [[ $# -ne 2 ]]; then
  usage >&2
  exit 2
fi

version="$1"
phase="$2"

if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "ERROR: version must match vX.Y.Z: $version" >&2
  exit 2
fi

case "$phase" in
  prepare|materials|tag) ;;
  *)
    echo "ERROR: phase must be prepare, materials, or tag: $phase" >&2
    exit 2
    ;;
esac

repo_root="$(git rev-parse --show-toplevel 2>/dev/null || true)"
if [[ -z "$repo_root" ]]; then
  echo "ERROR: not inside a Git repository" >&2
  exit 1
fi

cd "$repo_root"

failures=0

ok() {
  printf 'OK: %s\n' "$1"
}

fail() {
  printf 'ERROR: %s\n' "$1" >&2
  failures=$((failures + 1))
}

require_file() {
  if [[ -f "$1" ]]; then
    ok "found $1"
  else
    fail "missing $1"
  fi
}

require_text() {
  local file="$1"
  local text="$2"
  local label="$3"

  if [[ -f "$file" ]] && grep -Fq "$text" "$file"; then
    ok "$label"
  else
    fail "$label"
  fi
}

check_materials() {
  local release_file="docs/releases/${version}.md"

  require_file "$release_file"
  require_text "$release_file" "# Ember ${version}" "release title matches ${version}"
  require_text "docs/releases/README.md" "稳定版：\`${version}.md\`" "release index points to ${version}"
  require_text "infrastructure/docker/docker-compose.yml" "ghcr.io/konghanghang/ember-api:${version}" "API image defaults to ${version}"
  require_text "infrastructure/docker/docker-compose.yml" "ghcr.io/konghanghang/ember-web:${version}" "Web image defaults to ${version}"
  require_text "infrastructure/docker/docker-compose.yml" "ghcr.io/konghanghang/ember-bot:${version}" "Bot image defaults to ${version}"
  require_text "$release_file" "compare/${previous_tag}...${version}" "release compare range is ${previous_tag}...${version}"
  require_text "$release_file" "ember-api:${version}" "release notes contain API image ${version}"
  require_text "$release_file" "ember-web:${version}" "release notes contain Web image ${version}"
  require_text "$release_file" "ember-bot:${version}" "release notes contain Bot image ${version}"
}

require_file "AGENTS.md"
require_file "docs/runbooks/release-process.md"
require_file "docs/releases/release-template.md"
require_file "infrastructure/docker/docker-compose.yml"

branch="$(git branch --show-current)"
if [[ "$branch" == "master" ]]; then
  ok "current branch is master"
else
  fail "current branch is $branch, expected master"
fi

previous_tag="$(git tag --list 'v[0-9]*' --sort=-version:refname | awk -v target="$version" '$0 != target { print; exit }')"
if [[ -n "$previous_tag" ]]; then
  ok "previous release tag is $previous_tag"
else
  fail "no previous release tag found"
fi

if git show-ref --verify --quiet "refs/tags/${version}"; then
  fail "local tag ${version} already exists"
else
  ok "local tag ${version} does not exist"
fi

remote_tag_output=""
if remote_tag_output="$(git ls-remote --tags origin "refs/tags/${version}" 2>/dev/null)"; then
  if [[ -n "$remote_tag_output" ]]; then
    fail "remote tag ${version} already exists"
  else
    ok "remote tag ${version} does not exist"
  fi
else
  fail "unable to query origin for tag ${version}"
fi

if [[ "$phase" == "prepare" || "$phase" == "tag" ]]; then
  if [[ -z "$(git status --porcelain)" ]]; then
    ok "worktree is clean"
  else
    fail "worktree is not clean"
  fi

  if git show-ref --verify --quiet refs/remotes/origin/master; then
    head_commit="$(git rev-parse HEAD)"
    origin_commit="$(git rev-parse origin/master)"
    if [[ "$head_commit" == "$origin_commit" ]]; then
      ok "HEAD matches origin/master at $head_commit"
    else
      fail "HEAD $head_commit differs from origin/master $origin_commit"
    fi
  else
    fail "origin/master is unavailable; fetch remote refs first"
  fi
fi

if [[ "$phase" == "materials" || "$phase" == "tag" ]]; then
  check_materials
fi

if git diff --check; then
  ok "tracked diff passes git diff --check"
else
  fail "tracked diff has whitespace errors"
fi

if [[ -n "$previous_tag" ]]; then
  echo
  echo "Release commits (${previous_tag}..HEAD):"
  git log --oneline "${previous_tag}..HEAD"

  echo
  echo "Release files (${previous_tag}..HEAD):"
  git diff --name-status "${previous_tag}..HEAD"

  echo
  echo "Database changes (${previous_tag}..HEAD):"
  database_changes="$(git diff --name-only "${previous_tag}..HEAD" -- infrastructure/database || true)"
  if [[ -n "$database_changes" ]]; then
    printf '%s\n' "$database_changes"
  else
    echo "none"
  fi
fi

if (( failures > 0 )); then
  echo
  echo "Preflight failed with ${failures} error(s)." >&2
  exit 1
fi

echo
echo "Preflight passed for ${version} (${phase})."
