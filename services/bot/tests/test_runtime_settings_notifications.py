import sys
import types
import unittest
from unittest.mock import AsyncMock, patch

telegram_stub = sys.modules.get("telegram") or types.ModuleType("telegram")


class InlineKeyboardButton:
    def __init__(self, text: str, callback_data: str | None = None) -> None:
        self.text = text
        self.callback_data = callback_data


class InlineKeyboardMarkup:
    def __init__(self, inline_keyboard):
        self.inline_keyboard = inline_keyboard


class InputMediaPhoto:
    def __init__(self, media, caption: str | None = None, parse_mode: str | None = None) -> None:
        self.media = media
        self.caption = caption
        self.parse_mode = parse_mode


class BotCommand:
    def __init__(self, command: str, description: str) -> None:
        self.command = command
        self.description = description


class BotCommandScopeChat:
    def __init__(self, chat_id: int) -> None:
        self.chat_id = chat_id


class BotCommandScopeChatAdministrators(BotCommandScopeChat):
    pass


class BotCommandScopeAllGroupChats:
    pass


class BotCommandScopeDefault:
    pass


class BotCommandScopeChatMember(BotCommandScopeChat):
    def __init__(self, chat_id: int, user_id: int) -> None:
        super().__init__(chat_id)
        self.user_id = user_id


telegram_stub.BotCommand = getattr(telegram_stub, "BotCommand", BotCommand)
telegram_stub.BotCommandScopeAllGroupChats = getattr(
    telegram_stub,
    "BotCommandScopeAllGroupChats",
    BotCommandScopeAllGroupChats,
)
telegram_stub.BotCommandScopeChat = getattr(telegram_stub, "BotCommandScopeChat", BotCommandScopeChat)
telegram_stub.BotCommandScopeChatAdministrators = getattr(
    telegram_stub,
    "BotCommandScopeChatAdministrators",
    BotCommandScopeChatAdministrators,
)
telegram_stub.BotCommandScopeChatMember = getattr(
    telegram_stub,
    "BotCommandScopeChatMember",
    BotCommandScopeChatMember,
)
telegram_stub.BotCommandScopeDefault = getattr(telegram_stub, "BotCommandScopeDefault", BotCommandScopeDefault)
telegram_stub.InlineKeyboardButton = getattr(telegram_stub, "InlineKeyboardButton", InlineKeyboardButton)
telegram_stub.InlineKeyboardMarkup = getattr(telegram_stub, "InlineKeyboardMarkup", InlineKeyboardMarkup)
telegram_stub.InputMediaPhoto = getattr(telegram_stub, "InputMediaPhoto", InputMediaPhoto)
telegram_stub.Update = getattr(telegram_stub, "Update", object)
sys.modules["telegram"] = telegram_stub

if "telegram.constants" not in sys.modules:
    constants_stub = types.ModuleType("telegram.constants")
    constants_stub.ChatType = types.SimpleNamespace(PRIVATE="private", GROUP="group", SUPERGROUP="supergroup")
    sys.modules["telegram.constants"] = constants_stub

if "telegram.error" not in sys.modules:
    error_stub = types.ModuleType("telegram.error")

    class TelegramError(Exception):
        pass

    error_stub.TelegramError = TelegramError
    sys.modules["telegram.error"] = error_stub

if "telegram.ext" not in sys.modules:
    ext_stub = types.ModuleType("telegram.ext")

    class ContextTypes:
        DEFAULT_TYPE = object

    ext_stub.ContextTypes = ContextTypes
    sys.modules["telegram.ext"] = ext_stub

from app.handlers import telegram_handler
from app.runtime_settings import RuntimeSettings


class RuntimeSettingsNotificationTestCase(unittest.IsolatedAsyncioTestCase):
    async def test_ranking_notification_falls_back_to_admin_after_group_is_cleared(self) -> None:
        bot = types.SimpleNamespace(send_message=AsyncMock())
        runtime_values = [
            RuntimeSettings(
                admin_chat_id=1001,
                approval_admin_ids=(1001,),
                group_chat_id=-2002,
                notify_group_link="",
                welcome_message_template="",
            ),
            RuntimeSettings(
                admin_chat_id=1001,
                approval_admin_ids=(1001,),
                group_chat_id=None,
                notify_group_link="",
                welcome_message_template="",
            ),
        ]

        async def get_chat_ids() -> tuple[int | None, int | None]:
            settings = runtime_values.pop(0)
            return settings.admin_chat_id, settings.group_chat_id

        with patch.object(
            telegram_handler.runtime_settings_service,
            "get_chat_ids",
            side_effect=get_chat_ids,
        ):
            await telegram_handler.send_ranking_notification(bot, {"period": "daily"})
            await telegram_handler.send_ranking_notification(bot, {"period": "daily"})

        self.assertEqual(bot.send_message.await_args_list[0].kwargs["chat_id"], -2002)
        self.assertEqual(bot.send_message.await_args_list[1].kwargs["chat_id"], 1001)


if __name__ == "__main__":
    unittest.main()
