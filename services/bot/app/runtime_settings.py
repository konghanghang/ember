import asyncio
import logging
import time
from dataclasses import dataclass
from typing import Optional

from app.clients import api_client
from app.config import TELEGRAM_ADMIN_CHAT_ID, TELEGRAM_GROUP_CHAT_ID

logger = logging.getLogger(__name__)

runtime_settings_keys = [
    "TELEGRAM_ADMIN_CHAT_ID",
    "TELEGRAM_GROUP_CHAT_ID",
    "notify_group_link",
]


def _parse_chat_id(raw: str, fallback: Optional[int]) -> Optional[int]:
    value = raw.strip()
    if not value:
        return fallback
    try:
        return int(value)
    except ValueError:
        logger.warning("Telegram Chat ID 配置无效，回退到环境变量: %s", value)
        return fallback


@dataclass(frozen=True)
class RuntimeSettings:
    admin_chat_id: Optional[int]
    group_chat_id: Optional[int]
    notify_group_link: str


class RuntimeSettingsService:
    def __init__(self, ttl_seconds: int = 30) -> None:
        self._ttl_seconds = ttl_seconds
        self._lock = asyncio.Lock()
        self._expires_at = 0.0
        self._cached = RuntimeSettings(
            admin_chat_id=TELEGRAM_ADMIN_CHAT_ID,
            group_chat_id=TELEGRAM_GROUP_CHAT_ID,
            notify_group_link="",
        )

    async def get(self, force_refresh: bool = False) -> RuntimeSettings:
        if not force_refresh and time.monotonic() < self._expires_at:
            return self._cached

        async with self._lock:
            if not force_refresh and time.monotonic() < self._expires_at:
                return self._cached

            settings = await api_client.get_settings(runtime_settings_keys)
            self._cached = RuntimeSettings(
                admin_chat_id=_parse_chat_id(settings.get("TELEGRAM_ADMIN_CHAT_ID", ""), TELEGRAM_ADMIN_CHAT_ID),
                group_chat_id=_parse_chat_id(settings.get("TELEGRAM_GROUP_CHAT_ID", ""), TELEGRAM_GROUP_CHAT_ID),
                notify_group_link=settings.get("notify_group_link", "").strip(),
            )
            self._expires_at = time.monotonic() + self._ttl_seconds
            return self._cached

    async def get_chat_ids(self) -> tuple[Optional[int], Optional[int]]:
        settings = await self.get()
        return settings.admin_chat_id, settings.group_chat_id

    async def get_notify_group_link(self) -> str:
        settings = await self.get()
        return settings.notify_group_link


runtime_settings_service = RuntimeSettingsService()
