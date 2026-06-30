import unittest
import sys
import types
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
    format_account_info,
    format_payment_message,
    format_result_message,
    format_subscription_message,
    format_subscription_result_message,
)


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

    def test_format_result_message_truncates_long_reason(self) -> None:
        text = format_result_message("<b>原始审批消息</b>", "reject", "原因" * 400)

        self.assertLessEqual(len(text), 1024)
        self.assertIn("📝 原因：", text)
        self.assertIn("...", text)

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

        self.assertLessEqual(len(text), 1024)
        self.assertIn("已被拒绝", text)
        self.assertIn("2026-04-15 16:00:00", text)
        self.assertIn("...", text)


if __name__ == "__main__":
    unittest.main()
