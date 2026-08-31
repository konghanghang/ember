import os
import sys
import unittest
from pathlib import Path
from unittest.mock import AsyncMock, patch

import httpx

BOT_ROOT = Path(__file__).resolve().parents[1]
BOT_ROOT_STR = str(BOT_ROOT)

if BOT_ROOT_STR not in sys.path:
    sys.path.insert(0, BOT_ROOT_STR)

os.environ.setdefault("TELEGRAM_BOT_TOKEN", "test-token")
os.environ.setdefault("INTERNAL_API_SECRET", "0123456789abcdef0123456789abcdef")
os.environ.setdefault("TELEGRAM_UPDATE_MODE", "polling")

from app.clients import api_client


class _StubClient:
    def __init__(self, responses) -> None:
        self._responses = list(responses)
        self.request = AsyncMock(side_effect=self._request)

    async def _request(self, *args, **kwargs):
        if not self._responses:
            raise AssertionError("unexpected request")
        response = self._responses.pop(0)
        if isinstance(response, Exception):
            raise response
        return response


class ApiClientRetryTestCase(unittest.IsolatedAsyncioTestCase):
    async def test_request_retries_get_on_5xx(self) -> None:
        request = httpx.Request("GET", "https://example.com")
        first = httpx.Response(503, request=request, json={"error": "temporary"})
        second = httpx.Response(200, request=request, json={"ok": True})
        client = _StubClient([first, second])

        with (
            patch.object(api_client, "_client", client),
            patch("app.clients.api_client.asyncio.sleep", AsyncMock()),
        ):
            response, _ = await api_client._request(
                "test_endpoint",
                "GET",
                "https://example.com",
                timeout=5.0,
            )

        self.assertIsNotNone(response)
        self.assertEqual(response.status_code, 200)
        self.assertEqual(client.request.await_count, 2)

    async def test_request_does_not_retry_post_on_5xx(self) -> None:
        request = httpx.Request("POST", "https://example.com")
        response_500 = httpx.Response(503, request=request, json={"error": "temporary"})
        client = _StubClient([response_500])

        with patch.object(api_client, "_client", client):
            response, _ = await api_client._request(
                "test_endpoint",
                "POST",
                "https://example.com",
                timeout=5.0,
                json={"hello": "world"},
            )

        self.assertIsNotNone(response)
        self.assertEqual(response.status_code, 503)
        self.assertEqual(client.request.await_count, 1)

    async def test_pop_pending_reject_posts_admin_user_id(self) -> None:
        request = httpx.Request("POST", "https://example.com")
        response = httpx.Response(
            200,
            request=request,
            json={"subscriptionId": "sub_123", "adminUserId": "1001"},
        )

        with patch.object(api_client, "_request", AsyncMock(return_value=(response, 12.0))) as request_mock:
            payload = await api_client.pop_pending_reject(2002, "1001")

        self.assertEqual(payload, {"subscriptionId": "sub_123", "adminUserId": "1001"})
        request_mock.assert_awaited_once()
        _, _, url = request_mock.await_args.args
        self.assertTrue(url.endswith("/api/v1/internal/telegram/reject-request/pop"))
        self.assertEqual(
            request_mock.await_args.kwargs["json"],
            {"chatId": 2002, "adminUserId": "1001"},
        )
        self.assertEqual(
            request_mock.await_args.kwargs["log_fields"],
            {"chatId": 2002, "adminUserId": "1001"},
        )

    async def test_get_media_library_settings_posts_telegram_id(self) -> None:
        request = httpx.Request("POST", "https://example.com")
        response = httpx.Response(200, request=request, json={"data": {"enabledCount": 1}})

        with patch.object(api_client, "_request", AsyncMock(return_value=(response, 12.0))) as request_mock:
            payload = await api_client.get_media_library_settings(1001)

        self.assertEqual(payload, {"data": {"enabledCount": 1}})
        _, method, url = request_mock.await_args.args
        self.assertEqual(method, "POST")
        self.assertTrue(url.endswith("/api/v1/internal/telegram/media-libraries"))
        self.assertEqual(request_mock.await_args.kwargs["json"], {"telegramId": 1001})

    async def test_toggle_media_library_quotes_library_id(self) -> None:
        request = httpx.Request("PUT", "https://example.com")
        response = httpx.Response(200, request=request, json={"data": {"enabledCount": 0}})

        with patch.object(api_client, "_request", AsyncMock(return_value=(response, 12.0))) as request_mock:
            payload = await api_client.toggle_media_library(1001, "folder/a b")

        self.assertEqual(payload, {"data": {"enabledCount": 0}})
        _, method, url = request_mock.await_args.args
        self.assertEqual(method, "PUT")
        self.assertTrue(url.endswith("/api/v1/internal/telegram/media-libraries/folder%2Fa%20b/toggle"))
        self.assertEqual(request_mock.await_args.kwargs["json"], {"telegramId": 1001})

    async def test_reset_media_library_preferences_deletes_with_telegram_id(self) -> None:
        request = httpx.Request("DELETE", "https://example.com")
        response = httpx.Response(200, request=request, json={"data": {"customized": False}})

        with patch.object(api_client, "_request", AsyncMock(return_value=(response, 12.0))) as request_mock:
            payload = await api_client.reset_media_library_preferences(1001)

        self.assertEqual(payload, {"data": {"customized": False}})
        _, method, url = request_mock.await_args.args
        self.assertEqual(method, "DELETE")
        self.assertTrue(url.endswith("/api/v1/internal/telegram/media-libraries/preferences"))
        self.assertEqual(request_mock.await_args.kwargs["json"], {"telegramId": 1001})

    async def test_search_tmdb_uses_internal_proxy_headers(self) -> None:
        request = httpx.Request("GET", "https://example.com")
        response = httpx.Response(200, request=request, json={"data": [], "total": 0})

        with patch.object(api_client, "_request", AsyncMock(return_value=(response, 12.0))) as request_mock:
            payload = await api_client.search_tmdb("dark", "tv")

        self.assertEqual(payload, {"data": [], "total": 0})
        _, method, url = request_mock.await_args.args
        self.assertEqual(method, "GET")
        self.assertTrue(url.endswith("/api/v1/internal/tmdb/search"))
        self.assertEqual(
            request_mock.await_args.kwargs["headers"],
            {"X-Internal-Secret": os.environ["INTERNAL_API_SECRET"]},
        )
        self.assertEqual(
            request_mock.await_args.kwargs["params"],
            {"query": "dark", "type": "tv"},
        )

    async def test_get_tmdb_tv_seasons_uses_internal_proxy_headers(self) -> None:
        request = httpx.Request("GET", "https://example.com")
        response = httpx.Response(200, request=request, json={"data": {"seasons": [1, 2]}})

        with patch.object(api_client, "_request", AsyncMock(return_value=(response, 12.0))) as request_mock:
            payload = await api_client.get_tmdb_tv_seasons(1399)

        self.assertEqual(payload, {"data": {"seasons": [1, 2]}})
        _, method, url = request_mock.await_args.args
        self.assertEqual(method, "GET")
        self.assertTrue(url.endswith("/api/v1/internal/tmdb/tv/1399/seasons"))
        self.assertEqual(
            request_mock.await_args.kwargs["headers"],
            {"X-Internal-Secret": os.environ["INTERNAL_API_SECRET"]},
        )


if __name__ == "__main__":
    unittest.main()
