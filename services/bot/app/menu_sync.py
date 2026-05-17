import asyncio
import logging

from telegram import (
    BotCommandScopeAllGroupChats,
    BotCommandScopeChat,
    BotCommandScopeChatAdministrators,
    BotCommandScopeChatMember,
    BotCommandScopeDefault,
    Update,
)

from app.bot_admin import is_bot_admin

logger = logging.getLogger(__name__)

BOT_MENU_VERSION = "2026-03-16-group-menu-v1"

_VALID_CHAT_TYPES = {"group", "supergroup"}
_synced_chat_versions: dict[int, str] = {}
_in_flight_chats: set[int] = set()
_lock = asyncio.Lock()


async def schedule_group_menu_sync(bot, update: Update) -> None:
    chat = update.effective_chat
    if chat is None or chat.type not in _VALID_CHAT_TYPES:
        return

    chat_id = int(chat.id)
    async with _lock:
        if _synced_chat_versions.get(chat_id) == BOT_MENU_VERSION:
            return
        if chat_id in _in_flight_chats:
            return
        _in_flight_chats.add(chat_id)

    asyncio.create_task(_sync_group_menu(bot, chat_id, chat.type, force=False))


async def force_group_menu_sync(bot, chat_id: int, chat_type: str) -> tuple[bool, str]:
    if chat_type not in _VALID_CHAT_TYPES:
        return False, "仅支持 group / supergroup"
    return await _sync_group_menu(bot, int(chat_id), chat_type, force=True)


async def force_chat_member_menu_cleanup(bot, chat_id: int, user_id: int) -> tuple[bool, str]:
    target_chat_id = int(chat_id)
    target_user_id = int(user_id)
    scopes = [
        BotCommandScopeChat(chat_id=target_chat_id),
        BotCommandScopeChatAdministrators(chat_id=target_chat_id),
        BotCommandScopeChatMember(chat_id=target_chat_id, user_id=target_user_id),
    ]

    errors: list[str] = []
    for scope in scopes:
        try:
            await bot.delete_my_commands(scope=scope)
        except Exception as err:
            logger.warning(
                "定向清理菜单作用域失败: chat_id=%s user_id=%s scope=%s err=%s",
                target_chat_id,
                target_user_id,
                type(scope).__name__,
                err,
            )
            errors.append(f"{type(scope).__name__}: {err}")

    if errors:
        return False, "定向菜单清理失败，请稍后重试"

    async with _lock:
        _synced_chat_versions.pop(target_chat_id, None)

    logger.info("定向菜单作用域已清理: chat_id=%s user_id=%s", target_chat_id, target_user_id)
    return True, "定向菜单作用域已清理"


async def is_menu_refresh_allowed(bot, chat_id: int, user_id: int) -> tuple[bool, str | None]:
    allowed, reason = await is_bot_admin(
        bot,
        chat_id=chat_id,
        user_id=user_id,
        allow_group_admin=True,
    )
    if allowed:
        return True, None
    if reason:
        if reason == "只有群管理员或配置的管理员账号可以执行此操作":
            return False, "只有群管理员或配置的管理员账号可以刷新菜单"
        return False, reason.replace("执行", "执行 /refresh_menu")
    return False, "只有群管理员或配置的管理员账号可以刷新菜单"


async def _sync_group_menu(bot, chat_id: int, chat_type: str, force: bool) -> tuple[bool, str]:
    try:
        if not force and _synced_chat_versions.get(chat_id) == BOT_MENU_VERSION:
            return True, "群菜单已是当前版本"

        errors: list[str] = []
        scopes = []
        if force:
            scopes.extend([
                BotCommandScopeDefault(),
                BotCommandScopeAllGroupChats(),
            ])
        scopes.extend([
            BotCommandScopeChat(chat_id=chat_id),
            BotCommandScopeChatAdministrators(chat_id=chat_id),
        ])
        for scope in scopes:
            try:
                await bot.delete_my_commands(scope=scope)
            except Exception as err:
                logger.warning(
                    "清理群菜单作用域失败: chat_id=%s scope=%s err=%s",
                    chat_id,
                    type(scope).__name__,
                    err,
                )
                errors.append(f"{type(scope).__name__}: {err}")

        if errors:
            return False, "群菜单清理失败，请稍后重试"

        async with _lock:
            _synced_chat_versions[chat_id] = BOT_MENU_VERSION

        logger.info("群菜单作用域已同步: chat_id=%s chat_type=%s", chat_id, chat_type)
        return True, "群菜单已同步"
    finally:
        async with _lock:
            _in_flight_chats.discard(chat_id)
