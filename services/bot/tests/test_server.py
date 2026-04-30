import asyncio
import os
import sys
import types
import unittest
from unittest.mock import AsyncMock, patch

os.environ.setdefault("TELEGRAM_BOT_TOKEN", "test-token")
os.environ.setdefault("INTERNAL_API_SECRET", "test-secret")
os.environ.setdefault("TELEGRAM_UPDATE_MODE", "webhook")
os.environ.setdefault("TELEGRAM_WEBHOOK_SECRET", "test-webhook-secret")
os.environ.setdefault("WEBHOOK_URL", "https://example.com")

if "uvicorn" not in sys.modules:
    uvicorn_stub = types.ModuleType("uvicorn")

    def run(*args, **kwargs):
        return None

    uvicorn_stub.run = run
    sys.modules["uvicorn"] = uvicorn_stub

if "dotenv" not in sys.modules:
    dotenv_stub = types.ModuleType("dotenv")

    def load_dotenv(*args, **kwargs) -> bool:
        return False

    dotenv_stub.load_dotenv = load_dotenv
    sys.modules["dotenv"] = dotenv_stub

if "fastapi" not in sys.modules:
    fastapi_stub = types.ModuleType("fastapi")

    class FastAPI:
        def __init__(self, *args, **kwargs) -> None:
            pass

        def get(self, *args, **kwargs):
            def decorator(fn):
                return fn
            return decorator

        def post(self, *args, **kwargs):
            def decorator(fn):
                return fn
            return decorator

    class Request:
        pass

    class Response:
        def __init__(self, status_code: int = 200) -> None:
            self.status_code = status_code

    fastapi_stub.FastAPI = FastAPI
    fastapi_stub.Request = Request
    fastapi_stub.Response = Response
    sys.modules["fastapi"] = fastapi_stub

if "fastapi.responses" not in sys.modules:
    fastapi_responses_stub = types.ModuleType("fastapi.responses")

    class JSONResponse:
        def __init__(self, status_code: int = 200, content=None) -> None:
            self.status_code = status_code
            self.content = content

    fastapi_responses_stub.JSONResponse = JSONResponse
    sys.modules["fastapi.responses"] = fastapi_responses_stub

if "telegram" not in sys.modules:
    telegram_stub = types.ModuleType("telegram")

    class InputMediaPhoto:
        def __init__(self, media: str, caption: str | None = None, parse_mode: str | None = None) -> None:
            self.media = media
            self.caption = caption
            self.parse_mode = parse_mode

    class Update:
        @staticmethod
        def de_json(data, bot):
            return {"data": data, "bot": bot}

    class BotCommand:
        def __init__(self, *args, **kwargs) -> None:
            pass

    class InlineKeyboardButton:
        def __init__(self, text: str, callback_data: str | None = None) -> None:
            self.text = text
            self.callback_data = callback_data

    class InlineKeyboardMarkup:
        def __init__(self, inline_keyboard) -> None:
            self.inline_keyboard = inline_keyboard

    class _ScopeBase:
        def __init__(self, *args, **kwargs) -> None:
            pass

    telegram_stub.InputMediaPhoto = InputMediaPhoto
    telegram_stub.Update = Update
    telegram_stub.BotCommand = BotCommand
    telegram_stub.InlineKeyboardButton = InlineKeyboardButton
    telegram_stub.InlineKeyboardMarkup = InlineKeyboardMarkup
    telegram_stub.BotCommandScopeChat = _ScopeBase
    telegram_stub.BotCommandScopeChatAdministrators = _ScopeBase
    telegram_stub.BotCommandScopeAllGroupChats = _ScopeBase
    telegram_stub.BotCommandScopeAllPrivateChats = _ScopeBase
    telegram_stub.BotCommandScopeChatMember = _ScopeBase
    telegram_stub.BotCommandScopeDefault = _ScopeBase
    sys.modules["telegram"] = telegram_stub

if "telegram.ext" not in sys.modules:
    telegram_ext_stub = types.ModuleType("telegram.ext")

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

    telegram_ext_stub.Application = Application
    telegram_ext_stub.CallbackQueryHandler = _Handler
    telegram_ext_stub.CommandHandler = _Handler
    telegram_ext_stub.ContextTypes = _ContextTypes
    telegram_ext_stub.MessageHandler = _Handler
    telegram_ext_stub.TypeHandler = _Handler
    telegram_ext_stub.filters = _Filters
    sys.modules["telegram.ext"] = telegram_ext_stub

if "httpx" not in sys.modules:
    httpx_stub = types.ModuleType("httpx")

    class Response:
        status_code = 200
        request = None

        def json(self):
            return {}

    class AsyncClient:
        async def request(self, *args, **kwargs):
            raise RuntimeError("stub")

        async def aclose(self):
            return None

    class Limits:
        def __init__(self, *args, **kwargs) -> None:
            pass

    class Timeout:
        def __init__(self, *args, **kwargs) -> None:
            pass

    class HTTPStatusError(Exception):
        def __init__(self, *args, **kwargs) -> None:
            super().__init__("stub")

    httpx_stub.Response = Response
    httpx_stub.AsyncClient = AsyncClient
    httpx_stub.Limits = Limits
    httpx_stub.Timeout = Timeout
    httpx_stub.HTTPStatusError = HTTPStatusError
    sys.modules["httpx"] = httpx_stub

from app import server


class ServerWebhookRetryTestCase(unittest.IsolatedAsyncioTestCase):
    async def test_register_webhook_with_retry_stops_after_max_attempts(self) -> None:
        stop_event = asyncio.Event()

        with (
            patch.object(server.tg_app.bot, "set_webhook", AsyncMock(side_effect=RuntimeError("network down"))),
            patch.object(server.logger, "warning") as warning_mock,
            patch.object(server.logger, "error") as error_mock,
            patch.object(stop_event, "wait", AsyncMock(side_effect=asyncio.TimeoutError)),
        ):
            result = await server.register_webhook_with_retry(stop_event)

        self.assertFalse(result)
        self.assertEqual(warning_mock.call_count, server.WEBHOOK_REGISTER_MAX_ATTEMPTS)
        error_mock.assert_called_once()

    async def test_health_returns_degraded_when_webhook_not_registered(self) -> None:
        server._webhook_registration_state["mode"] = server.TELEGRAM_UPDATE_MODE_WEBHOOK
        server._webhook_registration_state["registered"] = False
        server._webhook_registration_state["attempts"] = 6
        server._webhook_registration_state["lastError"] = "network down"

        payload = await server.health()

        self.assertEqual(payload["status"], "degraded")
        self.assertFalse(payload["webhookRegistered"])
        self.assertEqual(payload["webhookRegisterAttempts"], 6)
        self.assertEqual(payload["lastWebhookRegisterError"], "network down")

    async def test_health_returns_ok_when_webhook_registered(self) -> None:
        server._webhook_registration_state["mode"] = server.TELEGRAM_UPDATE_MODE_WEBHOOK
        server._webhook_registration_state["registered"] = True
        server._webhook_registration_state["attempts"] = 1
        server._webhook_registration_state["lastError"] = ""

        payload = await server.health()

        self.assertEqual(payload["status"], "ok")
        self.assertTrue(payload["webhookRegistered"])
