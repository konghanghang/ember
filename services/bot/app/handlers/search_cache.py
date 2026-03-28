"""每用户搜索会话缓存，TTL 自动清理"""

import time
from dataclasses import dataclass, field


@dataclass
class SearchSession:
    """单个用户的搜索会话"""

    results: list[dict]
    media_type: str
    query: str
    selected_index: int = -1
    waiting_for_note: bool = False
    message_id: int = 0
    chat_id: int = 0
    created_at: float = field(default_factory=time.time)


_cache: dict[int, SearchSession] = {}

SESSION_TTL = 600


def get_session(user_id: int) -> SearchSession | None:
    """获取用户的搜索会话，过期则返回 None"""
    session = _cache.get(user_id)
    if session is None:
        return None
    if time.time() - session.created_at > SESSION_TTL:
        del _cache[user_id]
        return None
    return session


def set_session(user_id: int, session: SearchSession) -> None:
    """设置用户的搜索会话（覆盖旧会话），同时惰性清理过期条目"""
    _cleanup_expired()
    _cache[user_id] = session


def delete_session(user_id: int) -> None:
    """删除用户的搜索会话"""
    _cache.pop(user_id, None)


def _cleanup_expired() -> None:
    """惰性清理：每次 set 时顺便清理过期条目，避免内存泄漏"""
    now = time.time()
    expired = [uid for uid, session in _cache.items() if now - session.created_at > SESSION_TTL]
    for uid in expired:
        del _cache[uid]
