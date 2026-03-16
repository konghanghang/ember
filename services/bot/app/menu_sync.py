import asyncio
import logging

from telegram import (
    BotCommandScopeAllGroupChats,
    BotCommandScopeChat,
    BotCommandScopeChatAdministrators,
    BotCommandScopeDefault,
    Update,
)

from app.runtime_settings import runtime_settings_service

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


async def is_menu_refresh_allowed(bot, chat_id: int, user_id: int) -> tuple[bool, str | None]:
    admin_chat_id, _ = await runtime_settings_service.get_chat_ids()
    if admin_chat_id is not None and user_id == admin_chat_id:
        return True, None

    try:
        bot_member = await bot.get_chat_member(chat_id=chat_id, user_id=bot.id)
    except Exception:
        logger.exception("查询 Bot 群权限失败: chat_id=%s", chat_id)
        return False, "当前无法校验群管理员身份，请使用配置的管理员账号执行 /refresh_menu"

    bot_status = getattr(bot_member, "status", "")
    if bot_status not in {"administrator", "creator"}:
        return False, "Bot 当前不是群管理员，无法校验普通群管理员身份；请使用配置的管理员账号执行 /refresh_menu"

    try:
        member = await bot.get_chat_member(chat_id=chat_id, user_id=user_id)
    except Exception:
        logger.exception("查询群管理员身份失败: chat_id=%s user_id=%s", chat_id, user_id)
        return False, "当前无法校验群管理员身份，请稍后重试"

    if getattr(member, "status", "") in {"administrator", "creator"}:
        return True, None
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
