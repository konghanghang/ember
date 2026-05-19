import logging

from app.runtime_settings import runtime_settings_service

logger = logging.getLogger(__name__)

_GROUP_ADMIN_STATUSES = {"administrator", "creator"}


async def is_bot_admin(
    bot,
    *,
    chat_id: int,
    user_id: int,
    allow_group_admin: bool = False,
) -> tuple[bool, str | None]:
    approval_admin_ids = await runtime_settings_service.get_approval_admin_ids()
    if user_id in approval_admin_ids:
        return True, None

    if not allow_group_admin:
        return False, "只有配置的 Telegram 审批人员可以执行此操作"

    try:
        bot_member = await bot.get_chat_member(chat_id=chat_id, user_id=bot.id)
    except Exception:
        logger.exception("查询 Bot 群权限失败: chat_id=%s", chat_id)
        return False, "当前无法校验群管理员身份，请使用配置的管理员账号执行"

    bot_status = getattr(bot_member, "status", "")
    if bot_status not in _GROUP_ADMIN_STATUSES:
        return False, "Bot 当前不是群管理员，无法校验普通群管理员身份；请使用配置的管理员账号执行"

    try:
        member = await bot.get_chat_member(chat_id=chat_id, user_id=user_id)
    except Exception:
        logger.exception("查询群管理员身份失败: chat_id=%s user_id=%s", chat_id, user_id)
        return False, "当前无法校验群管理员身份，请稍后重试"

    if getattr(member, "status", "") in _GROUP_ADMIN_STATUSES:
        return True, None

    return False, "只有群管理员或配置的管理员账号可以执行此操作"
