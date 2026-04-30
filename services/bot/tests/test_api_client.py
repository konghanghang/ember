import types
import unittest
from unittest.mock import AsyncMock, patch

import httpx

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


if __name__ == "__main__":
    unittest.main()
