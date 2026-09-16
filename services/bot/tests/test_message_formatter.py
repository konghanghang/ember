import unittest
import sys
import types
from html.parser import HTMLParser
from unittest.mock import patch

if "telegram" not in sys.modules:
    telegram_stub = types.ModuleType("telegram")

    class InlineKeyboardButton:
        def __init__(self, text: str, callback_data: str | None = None) -> None:
            self.text = text
            self.callback_data = callback_data

    class InlineKeyboardMarkup:
        def __init__(self, inline_keyboard):
            self.inline_keyboard = inline_keyboard

    telegram_stub.InlineKeyboardButton = InlineKeyboardButton
    telegram_stub.InlineKeyboardMarkup = InlineKeyboardMarkup
    sys.modules["telegram"] = telegram_stub

from app.formatters.message_formatter import (
    TELEGRAM_CAPTION_LIMIT,
    _clamp_html_to_text_budget,
    _html_text_length,
    format_account_info,
    format_auto_approved_subscription_message,
    format_payment_message,
    format_result_message,
    format_search_detail,
    format_search_results,
    format_subscription_message,
    format_subscription_result_message,
)


class _TelegramHTMLValidator(HTMLParser):
    def __init__(self) -> None:
        super().__init__(convert_charrefs=False)
        self._stack: list[str] = []

    def handle_starttag(self, tag: str, attrs) -> None:
        self._stack.append(tag)

    def handle_endtag(self, tag: str) -> None:
        if not self._stack or self._stack[-1] != tag:
            raise AssertionError(f"broken html tag: {tag}")
        self._stack.pop()

    def close(self) -> None:
        super().close()
        if self._stack:
            raise AssertionError(f"unclosed html tags: {self._stack}")


def _assert_valid_html(text: str) -> None:
    parser = _TelegramHTMLValidator()
    parser.feed(text)
    parser.close()


class MessageFormatterTestCase(unittest.TestCase):
    def test_format_subscription_message_includes_season_and_note(self) -> None:
        text, keyboard = format_subscription_message(
            {
                "id": "sub_123",
                "type": "TV",
                "name": "Test <Show>",
                "userName": "ember-user",
                "tmdbId": 42,
                "season": 2,
                "note": "  need asap  ",
            }
        )

        self.assertIn("电视剧", text)
        self.assertIn("第 2 季", text)
        self.assertIn("need asap", text)
        self.assertEqual(keyboard.inline_keyboard[0][0].callback_data, "approve:sub_123")
        self.assertEqual(keyboard.inline_keyboard[0][1].callback_data, "reject:sub_123")

    def test_format_payment_message_formats_currency_and_expiry(self) -> None:
        with patch.dict("os.environ", {"TZ": "Asia/Shanghai"}):
            text = format_payment_message(
                {
                    "userName": "ember-user",
                    "planName": "季度套餐",
                    "amount": 1299,
                    "currency": "cny",
                    "days": 90,
                    "paymentId": "pay_123",
                    "oldExpiresAt": "2026-04-15T08:00:00Z",
                    "newExpiresAt": "2026-07-14T08:00:00Z",
                }
            )

        self.assertIn("¥12.99", text)
        self.assertIn("2026-04-15 16:00:00", text)
        self.assertIn("2026-07-14 16:00:00", text)
        self.assertIn("季度套餐", text)
        self.assertNotIn("📧", text)
        self.assertNotIn("Session", text)

    def test_format_account_info_marks_expired_users(self) -> None:
        with patch.dict("os.environ", {"TZ": "Asia/Shanghai"}):
            text = format_account_info(
                {
                    "username": "ember-user",
                    "email": "user@example.com",
                    "isExpired": True,
                    "expiresAt": "2026-04-15T16:00:00Z",
                }
            )

        self.assertIn("已过期", text)
        self.assertIn("/redeem", text)
        self.assertIn("2026-04-16", text)

    def test_format_subscription_message_truncates_long_note_for_caption_limit(self) -> None:
        text, _ = format_subscription_message(
            {
                "id": "sub_123",
                "type": "TV",
                "name": "超长剧名" * 80,
                "userName": "ember-user",
                "tmdbId": 42,
                "season": 2,
                "note": "x" * 2000,
            }
        )

        self.assertLessEqual(len(text), 1024)
        self.assertIn("...", text)

    def test_format_auto_approved_subscription_message_includes_quota_reason(self) -> None:
        with patch.dict("os.environ", {"TZ": "Asia/Shanghai"}):
            text = format_auto_approved_subscription_message(
                {
                    "id": "sub_123",
                    "type": "TV",
                    "name": "Test Show",
                    "userName": "ember-user",
                    "tmdbId": 42,
                    "season": 2,
                    "planGroupName": "VIP",
                    "autoApprovedOrdinal": 1,
                    "dailyLimit": 2,
                    "reviewedAt": "2026-07-07T08:00:00Z",
                }
            )

        self.assertIn("订阅已自动通过", text)
        self.assertIn("VIP", text)
        self.assertIn("今日第 1/2 条", text)
        self.assertIn("2026-07-07 16:00:00", text)

    def test_format_result_message_truncates_long_reason(self) -> None:
        text = format_result_message("<b>原始审批消息</b>", "reject", "原因" * 400)

        self.assertLessEqual(_html_text_length(text), TELEGRAM_CAPTION_LIMIT)
        self.assertIn("📝 原因：", text)
        self.assertIn("...", text)

    def test_clamp_html_to_text_budget_handles_small_limits_and_closes_tags(self) -> None:
        self.assertEqual(_clamp_html_to_text_budget("<b>xyz</b>", 2), "..")
        self.assertEqual(_clamp_html_to_text_budget("<b>xyzx</b>", 3), "...")
        self.assertEqual(_html_text_length(_clamp_html_to_text_budget('<a href="u">xyzxy</a>', 4)), 4)
        _assert_valid_html(_clamp_html_to_text_budget('<a href="u">xyzxy</a>', 4))

    def test_format_result_message_keeps_html_valid_when_original_caption_is_long(self) -> None:
        original_text = (
            "📌 <b>"
            + ("片名&" * 400)
            + "</b>\n🔗 <a href='https://www.themoviedb.org/movie/42'>#42</a>"
        )

        text = format_result_message(original_text, "reject", "最终拒绝原因")

        self.assertLessEqual(_html_text_length(text), TELEGRAM_CAPTION_LIMIT)
        self.assertIn("❌ 已拒绝", text)
        self.assertIn("最终拒绝原因", text)
        self.assertNotIn("&a...", text)
        self.assertNotIn("&am...", text)
        self.assertNotIn("&amp...", text)
        _assert_valid_html(text)

    def test_format_subscription_result_message_truncates_long_reject_reason(self) -> None:
        with patch.dict("os.environ", {"TZ": "Asia/Shanghai"}):
            text = format_subscription_result_message(
                {
                    "status": "REJECTED",
                    "type": "TV",
                    "name": "Test Show",
                    "tmdbId": 42,
                    "season": 1,
                    "rejectReason": "资源重复" * 300,
                    "reviewedAt": "2026-04-15T08:00:00Z",
                }
            )

        self.assertLessEqual(_html_text_length(text), TELEGRAM_CAPTION_LIMIT)
        self.assertIn("已被拒绝", text)
        self.assertIn("2026-04-15 16:00:00", text)
        self.assertIn("...", text)
        _assert_valid_html(text)

    def test_format_subscription_result_message_truncates_reason_before_escaping_entities(self) -> None:
        with patch.dict("os.environ", {"TZ": "Asia/Shanghai"}):
            text = format_subscription_result_message(
                {
                    "status": "REJECTED",
                    "type": "MOVIE",
                    "name": "Test Movie",
                    "tmdbId": 42,
                    "rejectReason": "&" * 700,
                    "reviewedAt": "2026-04-15T08:00:00Z",
                }
            )

        self.assertLessEqual(_html_text_length(text), TELEGRAM_CAPTION_LIMIT)
        self.assertIn("当前状态：已拒绝", text)
        self.assertNotIn("&a...", text)
        self.assertNotIn("&am...", text)
        self.assertNotIn("&amp...", text)
        _assert_valid_html(text)

    def test_format_subscription_result_message_uses_parsed_text_budget_for_entities(self) -> None:
        with patch.dict("os.environ", {"TZ": "Asia/Shanghai"}):
            text = format_subscription_result_message(
                {
                    "status": "REJECTED",
                    "type": "MOVIE",
                    "name": '"' * 160,
                    "tmdbId": 42,
                    "rejectReason": '"' * 160,
                    "reviewedAt": "2026-04-15T08:00:00Z",
                }
            )

        self.assertLessEqual(_html_text_length(text), TELEGRAM_CAPTION_LIMIT)
        self.assertGreater(text.count("&quot;"), 250)
        self.assertIn("当前状态：已拒绝", text)
        _assert_valid_html(text)

    def test_format_result_message_counts_emoji_as_one_parsed_character(self) -> None:
        text = format_result_message("<b>" + "🎬" * 1100 + "</b>", "approve")

        self.assertLessEqual(_html_text_length(text), TELEGRAM_CAPTION_LIMIT)
        self.assertIn("✅ 已通过", text)
        _assert_valid_html(text)

    def test_format_search_results_keeps_buttons_and_clamps_caption_fields(self) -> None:
        results = [
            {
                "id": i + 1,
                "title": "标题<&>" * 80,
                "originalTitle": "Original & Quoted " * 80,
                "releaseDate": "2026-01-01",
                "mediaType": "movie" if i % 2 == 0 else "tv",
            }
            for i in range(10)
        ]

        caption, keyboard = format_search_results(results, '"' * 200, is_caption=True)

        self.assertLessEqual(_html_text_length(caption), TELEGRAM_CAPTION_LIMIT)
        for i in range(10):
            self.assertIn(f"<b>{i + 1}.</b>", caption)
            self.assertEqual(keyboard.inline_keyboard[i // 4][i % 4].callback_data, f"sub:pick:{i}")
        self.assertIn("...", caption)
        self.assertNotIn("&a...", caption)
        _assert_valid_html(caption)

    def test_format_search_detail_clamps_long_title_to_caption_budget(self) -> None:
        caption = format_search_detail(
            {
                "id": 42,
                "title": "超长标题<&>" * 300,
                "originalTitle": "Original & Quoted " * 200,
                "releaseDate": "2026-01-01",
                "mediaType": "movie",
                "overview": "剧情简介" * 200,
            }
        )

        self.assertLessEqual(_html_text_length(caption), TELEGRAM_CAPTION_LIMIT)
        self.assertIn("TMDB #42", caption)
        self.assertIn("...", caption)
        self.assertNotIn("&a...", caption)
        _assert_valid_html(caption)


if __name__ == "__main__":
    unittest.main()
