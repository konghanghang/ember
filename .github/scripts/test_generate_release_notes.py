#!/usr/bin/env python3

from __future__ import annotations

import importlib.util
import sys
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
SCRIPT_PATH = ROOT / ".github" / "scripts" / "generate_release_notes.py"


def load_module():
    spec = importlib.util.spec_from_file_location("generate_release_notes", SCRIPT_PATH)
    module = importlib.util.module_from_spec(spec)
    sys.modules["generate_release_notes"] = module
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


grn = load_module()


def make_commit(*, sha: str, subject: str, commit_type: str = "", scope: str = "", description: str = "", files: tuple[str, ...] = ()) -> grn.Commit:
    return grn.Commit(
        sha=sha,
        short_sha=sha[:7],
        subject=subject,
        commit_type=commit_type,
        scope=scope,
        description=description or subject,
        files=files,
    )


class BuildTopicMatchesTest(unittest.TestCase):
    def test_env_example_change_does_not_trigger_polling_topic(self) -> None:
        commit = make_commit(
            sha="a" * 40,
            subject="docs(config): 收口环境变量示例与配置文档",
            commit_type="docs",
            scope="config",
            description="收口环境变量示例与配置文档",
            files=("services/bot/.env.example",),
        )

        matches = grn.build_topic_matches([commit], set(commit.files))

        self.assertEqual(matches["polling"], set())

    def test_subscription_view_tweak_does_not_trigger_season_subscription_topic(self) -> None:
        commit = make_commit(
            sha="b" * 40,
            subject="fix(web): 收口支付中心样式并修正刷新按钮对齐",
            commit_type="fix",
            scope="web",
            description="收口支付中心样式并修正刷新按钮对齐",
            files=("services/web/src/views/console/SubscriptionsView.vue",),
        )

        matches = grn.build_topic_matches([commit], set(commit.files))

        self.assertEqual(matches["season_subscription"], set())

    def test_shared_admin_api_change_does_not_trigger_settings_cleanup(self) -> None:
        commit = make_commit(
            sha="c" * 40,
            subject="feat(admin): 支持后台创建用户并设置套餐组与到期时间",
            commit_type="feat",
            scope="admin",
            description="支持后台创建用户并设置套餐组与到期时间",
            files=("services/web/src/api/admin.ts",),
        )

        matches = grn.build_topic_matches([commit], set(commit.files))

        self.assertEqual(matches["settings_cleanup"], set())

    def test_polling_runtime_change_still_triggers_polling_topic(self) -> None:
        commit = make_commit(
            sha="d" * 40,
            subject="feat(bot): 支持 polling 模式",
            commit_type="feat",
            scope="bot",
            description="支持 polling 模式",
            files=("services/bot/app/server.py",),
        )

        matches = grn.build_topic_matches([commit], set(commit.files))

        self.assertEqual(matches["polling"], {commit.sha})

    def test_season_subscription_keyword_still_triggers_topic(self) -> None:
        commit = make_commit(
            sha="e" * 40,
            subject="feat(subscription): 支持按季订阅",
            commit_type="feat",
            scope="subscription",
            description="支持按季订阅",
            files=("services/web/src/views/console/NewSubscriptionView.vue",),
        )

        matches = grn.build_topic_matches([commit], set(commit.files))

        self.assertEqual(matches["season_subscription"], {commit.sha})


class BuildFallbackLinesTest(unittest.TestCase):
    def test_env_examples_and_markdown_are_treated_as_documentation_only(self) -> None:
        commit = make_commit(
            sha="0" * 40,
            subject="docs(config): 收口环境变量示例与配置文档",
            commit_type="docs",
            scope="config",
            description="收口环境变量示例与配置文档",
            files=(
                "docs/reference/configuration-reference.md",
                "services/api/.env.example",
                "services/api/README.md",
            ),
        )

        self.assertTrue(grn.is_documentation_only_commit(commit))

    def test_codex_only_commit_is_filtered_as_noise(self) -> None:
        commit = make_commit(
            sha="f" * 40,
            subject="fix(codex): 减少 git add 重复审批",
            commit_type="fix",
            scope="codex",
            description="减少 git add 重复审批",
            files=(".codex/rules/default.rules",),
        )

        features, fixes, improvements = grn.build_fallback_lines([commit], set())

        self.assertEqual(features, [])
        self.assertEqual(fixes, [])
        self.assertEqual(improvements, [])

    def test_internal_scope_commit_is_filtered_from_fallback_lines(self) -> None:
        commit = make_commit(
            sha="3" * 40,
            subject="refactor(ci): 停止自动提交覆盖率徽章",
            commit_type="refactor",
            scope="ci",
            description="停止自动提交覆盖率徽章",
            files=(".github/workflows/test.yml",),
        )

        features, fixes, improvements = grn.build_fallback_lines([commit], set())

        self.assertEqual(features, [])
        self.assertEqual(fixes, [])
        self.assertEqual(improvements, [])

    def test_test_commit_is_not_rendered_in_reference_section(self) -> None:
        commit = make_commit(
            sha="2" * 40,
            subject="test(billing-redemption): 补充套餐分组高层测试",
            commit_type="test",
            scope="billing-redemption",
            description="补充套餐分组高层测试",
            files=("services/api/internal/services/payment/service_test.go",),
        )

        lines = grn.render_reference_commits("konghanghang/ember", [commit])

        self.assertNotIn(commit.subject, "\n".join(lines))

    def test_test_commit_is_not_exposed_in_release_notes(self) -> None:
        commit = make_commit(
            sha="1" * 40,
            subject="test(billing-redemption): 补充套餐分组高层测试",
            commit_type="test",
            scope="billing-redemption",
            description="补充套餐分组高层测试",
            files=("services/api/internal/services/payment/service_test.go",),
        )

        features, fixes, improvements = grn.build_fallback_lines([commit], set())

        self.assertEqual(features, [])
        self.assertEqual(fixes, [])
        self.assertEqual(improvements, [])


class BuildUpgradeLinesTest(unittest.TestCase):
    def test_only_top_level_incremental_migrations_are_rendered(self) -> None:
        changed_files = {
            "infrastructure/database/20260415_00_schema_baseline.sql",
            "infrastructure/database/20260416_01_subscription_status_and_review_fields.sql",
            "infrastructure/database/20260418_01_media_gaps.sql",
            "infrastructure/database/archive/pre-20260415/20260305_03_add_tv_calendar_tables.sql",
            "infrastructure/database/archive/pre-20260415/20260321_01_add_playback_ranking_batch_fields.sql",
        }

        lines = grn.build_upgrade_lines(
            changed_files,
            {
                "polling": set(),
                "season_subscription": set(),
                "web_forms": set(),
                "settings_cleanup": set(),
                "favicon": set(),
            },
        )

        self.assertEqual(
            lines,
            [
                "- 本版本包含数据库 migration。升级前请先执行 `infrastructure/database/20260416_01_subscription_status_and_review_fields.sql`、`infrastructure/database/20260418_01_media_gaps.sql`，否则新链路无法完整生效。"
            ],
        )

    def test_internal_scope_commit_is_filtered_from_reference_commits(self) -> None:
        commit = make_commit(
            sha="4" * 40,
            subject="fix(release): 收口迁移说明与覆盖率忽略",
            commit_type="fix",
            scope="release",
            description="收口迁移说明与覆盖率忽略",
            files=("docs/runbooks/deployment.md", ".gitignore"),
        )

        lines = grn.render_reference_commits("konghanghang/ember", [commit])

        self.assertNotIn(commit.subject, "\n".join(lines))


if __name__ == "__main__":
    unittest.main()
