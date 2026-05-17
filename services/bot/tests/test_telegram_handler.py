import os
import sys
import types
import unittest
from unittest.mock import AsyncMock, patch

os.environ["TELEGRAM_BOT_TOKEN"] = "test-token"
os.environ["INTERNAL_API_SECRET"] = "0123456789abcdef0123456789abcdef"
os.environ["TELEGRAM_UPDATE_MODE"] = "polling"

if "httpx" not in sys.modules:
    httpx_stub = types.ModuleType("httpx")

    class Request:
        def __init__(self, method: str, url: str) -> None:
            self.method = method
            self.url = url

    class Response:
        def __init__(self, status_code: int = 200, *, request=None, json=None) -> None:
            self.status_code = status_code
            self.request = request
            self._json = {} if json is None else json

        def json(self):
            return self._json

    class AsyncClient:
        async def request(self, *args, **kwargs):
            raise RuntimeError("httpx stub should not send requests in tests")

        async def aclose(self) -> None:
            return None

    class Limits:
        def __init__(self, *args, **kwargs) -> None:
            self.args = args
            self.kwargs = kwargs

    class Timeout:
        def __init__(self, *args, **kwargs) -> None:
            self.args = args
            self.kwargs = kwargs

    class HTTPStatusError(Exception):
        def __init__(self, message: str, request=None, response=None) -> None:
            super().__init__(message)
            self.request = request
            self.response = response

    httpx_stub.Request = Request
    httpx_stub.Response = Response
    httpx_stub.AsyncClient = AsyncClient
    httpx_stub.Limits = Limits
    httpx_stub.Timeout = Timeout
    httpx_stub.HTTPStatusError = HTTPStatusError
    sys.modules["httpx"] = httpx_stub

if "dotenv" not in sys.modules:
    dotenv_stub = types.ModuleType("dotenv")

    def load_dotenv(*args, **kwargs) -> bool:
        return False

    dotenv_stub.load_dotenv = load_dotenv
    sys.modules["dotenv"] = dotenv_stub

telegram_stub = sys.modules.get("telegram")
if telegram_stub is None:
    telegram_stub = types.ModuleType("telegram")
    sys.modules["telegram"] = telegram_stub


class InputMediaPhoto:
    def __init__(self, media: str, caption: str | None = None, parse_mode: str | None = None) -> None:
        self.media = media
        self.caption = caption
        self.parse_mode = parse_mode


class Update:
    pass


class InlineKeyboardButton:
    def __init__(self, text: str, callback_data: str | None = None) -> None:
        self.text = text
        self.callback_data = callback_data


class InlineKeyboardMarkup:
    def __init__(self, inline_keyboard):
        self.inline_keyboard = inline_keyboard


class _ScopeBase:
    def __init__(self, *args, **kwargs) -> None:
        self.args = args
        self.kwargs = kwargs


telegram_stub.InputMediaPhoto = getattr(telegram_stub, "InputMediaPhoto", InputMediaPhoto)
telegram_stub.Update = getattr(telegram_stub, "Update", Update)
telegram_stub.InlineKeyboardButton = getattr(telegram_stub, "InlineKeyboardButton", InlineKeyboardButton)
telegram_stub.InlineKeyboardMarkup = getattr(telegram_stub, "InlineKeyboardMarkup", InlineKeyboardMarkup)
telegram_stub.BotCommandScopeAllGroupChats = getattr(telegram_stub, "BotCommandScopeAllGroupChats", _ScopeBase)
telegram_stub.BotCommandScopeChat = getattr(telegram_stub, "BotCommandScopeChat", _ScopeBase)
telegram_stub.BotCommandScopeChatAdministrators = getattr(telegram_stub, "BotCommandScopeChatAdministrators", _ScopeBase)
telegram_stub.BotCommandScopeChatMember = getattr(telegram_stub, "BotCommandScopeChatMember", _ScopeBase)
telegram_stub.BotCommandScopeDefault = getattr(telegram_stub, "BotCommandScopeDefault", _ScopeBase)

telegram_ext_stub = sys.modules.get("telegram.ext")
if telegram_ext_stub is None:
    telegram_ext_stub = types.ModuleType("telegram.ext")
    sys.modules["telegram.ext"] = telegram_ext_stub


class _FakeApplication:
    def __init__(self) -> None:
        self.bot = types.SimpleNamespace(
            set_webhook=AsyncMock(),
            delete_webhook=AsyncMock(),
            delete_my_commands=AsyncMock(),
            set_my_commands=AsyncMock(),
        )
        self.updater = None

    async def initialize(self) -> None:
        return None

    async def start(self) -> None:
        return None

    async def stop(self) -> None:
        return None

    async def shutdown(self) -> None:
        return None

    def add_handler(self, *args, **kwargs) -> None:
        return None

    async def process_update(self, update) -> None:
        return None


class _Builder:
    def token(self, *args, **kwargs):
        return self

    def build(self):
        return _FakeApplication()


class Application:
    @staticmethod
    def builder():
        return _Builder()


class _ContextTypes:
    DEFAULT_TYPE = object


class _Handler:
    def __init__(self, *args, **kwargs) -> None:
        pass


class _FilterValue:
    def __and__(self, other):
        return self

    def __invert__(self):
        return self


class _Filters:
    TEXT = _FilterValue()
    COMMAND = _FilterValue()

    class StatusUpdate:
        NEW_CHAT_MEMBERS = _FilterValue()


telegram_ext_stub.Application = getattr(telegram_ext_stub, "Application", Application)
telegram_ext_stub.CallbackQueryHandler = getattr(telegram_ext_stub, "CallbackQueryHandler", _Handler)
telegram_ext_stub.CommandHandler = getattr(telegram_ext_stub, "CommandHandler", _Handler)
telegram_ext_stub.ContextTypes = getattr(telegram_ext_stub, "ContextTypes", _ContextTypes)
telegram_ext_stub.MessageHandler = getattr(telegram_ext_stub, "MessageHandler", _Handler)
telegram_ext_stub.TypeHandler = getattr(telegram_ext_stub, "TypeHandler", _Handler)
telegram_ext_stub.filters = getattr(telegram_ext_stub, "filters", _Filters)

from app.formatters.message_formatter import format_result_message
from app import bot_admin
from app import menu_sync
from app.handlers import telegram_handler


class _StubUser:
    def __init__(self, user_id: int) -> None:
        self.id = user_id


class _StubBot:
    def __init__(self) -> None:
        self.id = 5001
        self.edit_message_text = AsyncMock()
        self.edit_message_caption = AsyncMock()
        self.get_chat_member = AsyncMock()


class _StubMessage:
    def __init__(
        self,
        *,
        chat_id: int,
        user_id: int,
        text: str | None = None,
        message_id: int = 0,
        photo=None,
        text_html: str | None = None,
        bot: _StubBot | None = None,
    ) -> None:
        self.chat_id = chat_id
        self.from_user = _StubUser(user_id)
        self.text = text
        self.message_id = message_id
        self.photo = photo or []
        self.text_html = text_html
        self.caption_html = None
        self.text = text
        self.caption = None
        self.reply_text = AsyncMock()
        self._bot = bot or _StubBot()

    def get_bot(self) -> _StubBot:
        return self._bot


class _StubQuery:
    def __init__(self, *, data: str, user_id: int, message: _StubMessage | None) -> None:
        self.data = data
        self.from_user = _StubUser(user_id)
        self.message = message
        self.answer = AsyncMock()
        self.edit_message_text = AsyncMock()
        self.edit_message_caption = AsyncMock()


class _StubUpdate:
    def __init__(self, *, callback_query: _StubQuery | None = None, message: _StubMessage | None = None) -> None:
        self.callback_query = callback_query
        self.message = message


class TelegramHandlerTestCase(unittest.IsolatedAsyncioTestCase):
    def test_pending_reject_requests_removed(self) -> None:
        self.assertFalse(hasattr(telegram_handler, "pending_reject_requests"))

    async def test_handle_callback_reject_enqueues_api_record_and_prompts(self) -> None:
        message = _StubMessage(
            chat_id=2002,
            user_id=1001,
            message_id=77,
            text="原始审批消息",
            text_html="<b>原始审批消息</b>",
        )
        query = _StubQuery(data="reject:sub_123", user_id=1001, message=message)
        update = _StubUpdate(callback_query=query)

        with (
            patch.object(
                telegram_handler.runtime_settings_service,
                "get_chat_ids",
                AsyncMock(return_value=(1001, None)),
            ),
            patch.object(
                telegram_handler.api_client,
                "enqueue_pending_reject",
                AsyncMock(return_value=True),
            ) as enqueue_mock,
        ):
            await telegram_handler.handle_callback(update, None)

        enqueue_mock.assert_awaited_once_with(
            2002,
            "1001",
            "sub_123",
            message_id=77,
            has_photo=False,
            original_text="<b>原始审批消息</b>",
        )
        message.reply_text.assert_awaited_once_with(
            "请直接回复这条消息输入拒绝原因，5 分钟内有效。\n发送的下一条普通文本会作为拒绝原因提交。"
        )
        query.answer.assert_awaited_once_with("请发送拒绝原因")

    async def test_handle_callback_reject_denies_non_configured_group_admin(self) -> None:
        message = _StubMessage(
            chat_id=2002,
            user_id=1001,
            message_id=77,
            text="原始审批消息",
        )
        query = _StubQuery(data="reject:sub_123", user_id=1001, message=message)
        update = _StubUpdate(callback_query=query)

        message.get_bot().get_chat_member.side_effect = [
            types.SimpleNamespace(status="administrator"),
            types.SimpleNamespace(status="administrator"),
        ]

        with (
            patch.object(
                telegram_handler.runtime_settings_service,
                "get_chat_ids",
                AsyncMock(return_value=(9999, None)),
            ),
            patch.object(
                telegram_handler.api_client,
                "enqueue_pending_reject",
                AsyncMock(return_value=True),
            ) as enqueue_mock,
        ):
            await telegram_handler.handle_callback(update, types.SimpleNamespace(bot=message.get_bot()))

        query.answer.assert_awaited_once_with("你没有权限操作", show_alert=True)
        enqueue_mock.assert_not_awaited()

    async def test_menu_refresh_allows_group_admin(self) -> None:
        bot = _StubBot()
        bot.get_chat_member.side_effect = [
            types.SimpleNamespace(status="administrator"),
            types.SimpleNamespace(status="administrator"),
        ]

        with patch.object(
            bot_admin.runtime_settings_service,
            "get_chat_ids",
            AsyncMock(return_value=(9999, None)),
        ):
            allowed, reason = await menu_sync.is_menu_refresh_allowed(bot, 2002, 1001)

        self.assertTrue(allowed)
        self.assertIsNone(reason)

    async def test_handle_pending_reject_reason_uses_api_payload_to_reject_and_edit_message(self) -> None:
        bot = _StubBot()
        message = _StubMessage(chat_id=2002, user_id=1001, text="  资源重复  ", bot=bot)
        update = _StubUpdate(message=message)
        popped_payload = {
            "subscriptionId": "sub_123",
            "messageId": 77,
            "chatId": 2002,
            "hasPhoto": False,
            "originalText": "<b>原始审批消息</b>",
            "expiresAt": "2026-04-29T12:00:00Z",
        }

        with (
            patch.object(
                telegram_handler.runtime_settings_service,
                "get_chat_ids",
                AsyncMock(return_value=(1001, None)),
            ),
            patch.object(
                telegram_handler.api_client,
                "pop_pending_reject",
                AsyncMock(return_value=popped_payload),
            ) as pop_mock,
            patch.object(
                telegram_handler.api_client,
                "reject_subscription",
                AsyncMock(return_value={"ok": True}),
            ) as reject_mock,
        ):
            await telegram_handler.handle_pending_reject_reason(update, None)

        pop_mock.assert_awaited_once_with(2002, "1001")
        reject_mock.assert_awaited_once_with("sub_123", "资源重复")
        message.reply_text.assert_awaited_once_with("已提交拒绝原因并完成拒绝。")
        bot.edit_message_text.assert_awaited_once_with(
            chat_id=2002,
            message_id=77,
            text=format_result_message("<b>原始审批消息</b>", "reject", "资源重复"),
            parse_mode="HTML",
        )
        bot.edit_message_caption.assert_not_called()

    async def test_handle_pending_reject_reason_ignores_text_without_api_record(self) -> None:
        message = _StubMessage(chat_id=2002, user_id=1001, text="普通聊天")
        update = _StubUpdate(message=message)

        with (
            patch.object(
                telegram_handler.runtime_settings_service,
                "get_chat_ids",
                AsyncMock(return_value=(1001, None)),
            ),
            patch.object(
                telegram_handler.api_client,
                "pop_pending_reject",
                AsyncMock(return_value=None),
            ) as pop_mock,
            patch.object(
                telegram_handler.api_client,
                "reject_subscription",
                AsyncMock(),
            ) as reject_mock,
        ):
            await telegram_handler.handle_pending_reject_reason(update, None)

        pop_mock.assert_awaited_once_with(2002, "1001")
        reject_mock.assert_not_awaited()
        message.reply_text.assert_not_called()


if __name__ == "__main__":
    unittest.main()
