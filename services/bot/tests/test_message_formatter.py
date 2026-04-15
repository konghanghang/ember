import unittest
import sys
import types

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
    format_subscription_message,
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
        text = format_payment_message(
            {
                "userName": "ember-user",
                "email": "user@example.com",
                "planName": "季度套餐",
                "amount": 1299,
                "currency": "cny",
                "days": 90,
                "paymentId": "pay_123",
                "stripeSessionId": "sess_456",
                "oldExpiresAt": "2026-04-15T08:00:00Z",
                "newExpiresAt": "2026-07-14T08:00:00Z",
            }
        )

        self.assertIn("¥12.99", text)
        self.assertIn("2026-04-15 08:00:00", text)
        self.assertIn("2026-07-14 08:00:00", text)
        self.assertIn("季度套餐", text)

    def test_format_account_info_marks_expired_users(self) -> None:
        text = format_account_info(
            {
                "username": "ember-user",
                "email": "user@example.com",
                "isExpired": True,
                "expiresAt": "2026-04-15T08:00:00Z",
            }
        )

        self.assertIn("已过期", text)
        self.assertIn("/redeem", text)
        self.assertIn("2026-04-15", text)


if __name__ == "__main__":
    unittest.main()
