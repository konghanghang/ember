import unittest
from unittest.mock import AsyncMock, patch

from app.runtime_settings import RuntimeSettingsService


class RuntimeSettingsServiceTestCase(unittest.IsolatedAsyncioTestCase):
    async def test_group_chat_id_empty_setting_clears_cached_group(self) -> None:
        service = RuntimeSettingsService(ttl_seconds=30)

        with patch(
            "app.runtime_settings.api_client.get_settings",
            new=AsyncMock(
                return_value={
                    "TELEGRAM_ADMIN_CHAT_ID": "1001",
                    "TELEGRAM_GROUP_CHAT_ID": "-2002",
                }
            ),
        ):
            settings = await service.get(force_refresh=True)

        self.assertEqual(settings.admin_chat_id, 1001)
        self.assertEqual(settings.group_chat_id, -2002)

        with patch(
            "app.runtime_settings.api_client.get_settings",
            new=AsyncMock(return_value={"TELEGRAM_GROUP_CHAT_ID": ""}),
        ):
            settings = await service.get(force_refresh=True)

        self.assertEqual(settings.admin_chat_id, 1001)
        self.assertIsNone(settings.group_chat_id)

    async def test_group_chat_id_missing_or_refresh_failure_keeps_cached_group(self) -> None:
        service = RuntimeSettingsService(ttl_seconds=30)

        with patch(
            "app.runtime_settings.api_client.get_settings",
            new=AsyncMock(
                return_value={
                    "TELEGRAM_ADMIN_CHAT_ID": "1001",
                    "TELEGRAM_GROUP_CHAT_ID": "-2002",
                }
            ),
        ):
            await service.get(force_refresh=True)

        with patch(
            "app.runtime_settings.api_client.get_settings",
            new=AsyncMock(return_value={"notify_group_link": "https://example.test/group"}),
        ):
            settings = await service.get(force_refresh=True)

        self.assertEqual(settings.group_chat_id, -2002)
        self.assertEqual(settings.notify_group_link, "https://example.test/group")

        with patch(
            "app.runtime_settings.api_client.get_settings",
            new=AsyncMock(side_effect=RuntimeError("boom")),
        ):
            settings = await service.get(force_refresh=True)

        self.assertEqual(settings.group_chat_id, -2002)

    async def test_cleared_group_does_not_restore_environment_on_missing_invalid_or_failed_refresh(self) -> None:
        with patch("app.runtime_settings.TELEGRAM_GROUP_CHAT_ID", -3003):
            service = RuntimeSettingsService(ttl_seconds=30)
        with patch("app.runtime_settings.api_client.get_settings", AsyncMock(return_value={"TELEGRAM_GROUP_CHAT_ID": ""})):
            self.assertIsNone((await service.get(force_refresh=True)).group_chat_id)
        for response in ({}, {"TELEGRAM_GROUP_CHAT_ID": "invalid"}):
            with patch("app.runtime_settings.api_client.get_settings", AsyncMock(return_value=response)):
                self.assertIsNone((await service.get(force_refresh=True)).group_chat_id)
        with patch("app.runtime_settings.api_client.get_settings", AsyncMock(side_effect=RuntimeError("unavailable"))):
            self.assertIsNone((await service.get(force_refresh=True)).group_chat_id)


if __name__ == "__main__":
    unittest.main()
