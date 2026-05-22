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
    "telegram_approval_admin_ids",
    "TELEGRAM_GROUP_CHAT_ID",
    "notify_group_link",
    "telegram_welcome_message_template",
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


def _parse_approval_admin_ids(raw: str, fallback_admin_chat_id: Optional[int]) -> tuple[int, ...]:
    value = raw.strip()
    if not value:
        return (fallback_admin_chat_id,) if fallback_admin_chat_id is not None else ()

    result: list[int] = []
    seen: set[int] = set()
    for part in value.split(","):
        item = part.strip()
        if not item:
            continue
        try:
            admin_id = int(item)
        except ValueError:
            logger.warning("Telegram 审批人员 ID 配置无效，忽略该项: %s", item)
            continue
        if admin_id <= 0:
            logger.warning("Telegram 审批人员 ID 必须为正整数，忽略该项: %s", item)
            continue
        if admin_id in seen:
            continue
        seen.add(admin_id)
        result.append(admin_id)
    return tuple(result)


@dataclass(frozen=True)
class RuntimeSettings:
    admin_chat_id: Optional[int]
    approval_admin_ids: tuple[int, ...]
    group_chat_id: Optional[int]
    notify_group_link: str
    welcome_message_template: str


class RuntimeSettingsService:
    def __init__(self, ttl_seconds: int = 30) -> None:
        self._ttl_seconds = ttl_seconds
        self._lock = asyncio.Lock()
        self._expires_at = 0.0
        self._cached = RuntimeSettings(
            admin_chat_id=TELEGRAM_ADMIN_CHAT_ID,
            approval_admin_ids=(TELEGRAM_ADMIN_CHAT_ID,) if TELEGRAM_ADMIN_CHAT_ID is not None else (),
            group_chat_id=TELEGRAM_GROUP_CHAT_ID,
            notify_group_link="",
            welcome_message_template="",
        )

    async def get(self, force_refresh: bool = False) -> RuntimeSettings:
        if not force_refresh and time.monotonic() < self._expires_at:
            return self._cached

        async with self._lock:
            if not force_refresh and time.monotonic() < self._expires_at:
                return self._cached

            try:
                settings = await api_client.get_settings(runtime_settings_keys)
                current = self._cached
                admin_chat_id = (
                    _parse_chat_id(
                        settings["TELEGRAM_ADMIN_CHAT_ID"],
                        current.admin_chat_id,
                    )
                    if "TELEGRAM_ADMIN_CHAT_ID" in settings
                    else current.admin_chat_id
                )
                approval_admin_ids = (
                    _parse_approval_admin_ids(
                        settings.get("telegram_approval_admin_ids", ""),
                        admin_chat_id,
                    )
                    if "telegram_approval_admin_ids" in settings
                    else (
                        current.approval_admin_ids
                        if current.approval_admin_ids
                        else ((admin_chat_id,) if admin_chat_id is not None else ())
                    )
                )
                self._cached = RuntimeSettings(
                    admin_chat_id=admin_chat_id,
                    approval_admin_ids=approval_admin_ids,
                    group_chat_id=_parse_chat_id(
                        settings["TELEGRAM_GROUP_CHAT_ID"],
                        current.group_chat_id,
                    ) if "TELEGRAM_GROUP_CHAT_ID" in settings else current.group_chat_id,
                    notify_group_link=settings.get("notify_group_link", current.notify_group_link).strip(),
                    welcome_message_template=settings.get(
                        "telegram_welcome_message_template",
                        current.welcome_message_template,
                    ).strip(),
                )
                self._expires_at = time.monotonic() + self._ttl_seconds
            except Exception as err:
                logger.warning("运行期配置刷新失败，保留旧值：%s", err)

            return self._cached

    async def get_chat_ids(self) -> tuple[Optional[int], Optional[int]]:
        settings = await self.get()
        return settings.admin_chat_id, settings.group_chat_id

    async def get_approval_admin_ids(self) -> tuple[int, ...]:
        settings = await self.get()
        if settings.approval_admin_ids:
            return settings.approval_admin_ids
        return (settings.admin_chat_id,) if settings.admin_chat_id is not None else ()

    async def get_notify_group_link(self) -> str:
        settings = await self.get()
        return settings.notify_group_link

    async def get_welcome_message_template(self) -> str:
        settings = await self.get()
        return settings.welcome_message_template


runtime_settings_service = RuntimeSettingsService()
