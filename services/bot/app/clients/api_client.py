import asyncio
import logging
import time
from typing import Any, Optional

import httpx

from app.config import API_URL, INTERNAL_API_SECRET

logger = logging.getLogger(__name__)

_DEFAULT_TIMEOUT = 10.0
_SETTING_TIMEOUT = 5.0
_INTERNAL_HEADERS = {"X-Internal-Secret": INTERNAL_API_SECRET}
_client: httpx.AsyncClient | None = None


async def init() -> None:
    global _client

    if _client is not None:
        return

    try:
        _client = httpx.AsyncClient(
            limits=httpx.Limits(max_keepalive_connections=10, max_connections=20),
            timeout=httpx.Timeout(10.0, connect=5.0),
        )
    except Exception:
        logger.exception("初始化 Bot API HTTP client 失败")
        raise


async def close() -> None:
    global _client

    client = _client
    _client = None
    if client is None:
        return

    try:
        await client.aclose()
    except Exception as err:
        logger.warning("关闭 Bot API HTTP client 失败 errorType=%s", type(err).__name__)


def _get_client() -> httpx.AsyncClient:
    if _client is None:
        raise RuntimeError("Bot API HTTP client 尚未初始化")
    return _client


def _log_request_issue(
    endpoint: str,
    method: str,
    *,
    elapsed_ms: int,
    error: Exception,
    status_code: int | None = None,
    **fields: Any,
) -> None:
    context = " ".join(
        f"{key}={value}" for key, value in fields.items() if value is not None and value != ""
    )
    logger.warning(
        "Bot API 请求异常 endpoint=%s method=%s status=%s elapsedMs=%d errorType=%s %s",
        endpoint,
        method,
        status_code if status_code is not None else "-",
        elapsed_ms,
        type(error).__name__,
        context,
    )


async def _request(
    endpoint: str,
    method: str,
    url: str,
    *,
    timeout: float,
    headers: dict[str, str] | None = None,
    json: dict[str, Any] | None = None,
    params: dict[str, Any] | None = None,
    log_fields: dict[str, Any] | None = None,
) -> tuple[httpx.Response | None, int]:
    start = time.monotonic()
    context = log_fields or {}

    try:
        response = await _get_client().request(
            method,
            url,
            headers=headers,
            json=json,
            params=params,
            timeout=timeout,
        )
    except Exception as err:
        elapsed_ms = int((time.monotonic() - start) * 1000)
        _log_request_issue(endpoint, method, elapsed_ms=elapsed_ms, error=err, **context)
        return None, elapsed_ms

    elapsed_ms = int((time.monotonic() - start) * 1000)
    if response.status_code >= 500:
        _log_request_issue(
            endpoint,
            method,
            elapsed_ms=elapsed_ms,
            error=httpx.HTTPStatusError(
                "upstream server error",
                request=response.request,
                response=response,
            ),
            status_code=response.status_code,
            **context,
        )

    return response, elapsed_ms


def _load_json(
    response: httpx.Response,
    endpoint: str,
    method: str,
    *,
    elapsed_ms: int,
    **fields: Any,
) -> dict[str, Any] | None:
    try:
        payload = response.json()
    except Exception as err:
        _log_request_issue(
            endpoint,
            method,
            elapsed_ms=elapsed_ms,
            error=err,
            status_code=response.status_code,
            **fields,
        )
        return None

    if not isinstance(payload, dict):
        _log_request_issue(
            endpoint,
            method,
            elapsed_ms=elapsed_ms,
            error=TypeError("response payload is not an object"),
            status_code=response.status_code,
            **fields,
        )
        return None

    return payload


async def _call_subscription_action(
    subscription_id: str,
    action: str,
    payload: dict[str, Any] | None = None,
) -> dict[str, Any] | None:
    endpoint = f"subscription_{action}"
    url = f"{API_URL}/api/v1/internal/subscriptions/{subscription_id}/{action}"
    response, elapsed_ms = await _request(
        endpoint,
        "PUT",
        url,
        timeout=_DEFAULT_TIMEOUT,
        headers=_INTERNAL_HEADERS,
        json=payload,
        log_fields={"subscriptionId": subscription_id},
    )
    if response is None:
        return None
    if response.status_code == 200:
        return {"ok": True}
    # 4xx 透传业务错误字段给调用方
    if 400 <= response.status_code < 500:
        data = _load_json(response, endpoint, "PUT", elapsed_ms=elapsed_ms, subscriptionId=subscription_id)
        error_msg = (data.get("error", "操作失败") if data else "操作失败")
        return {"error": error_msg, "status": response.status_code}
    # 5xx fallback
    return {"error": "操作失败，请重试", "status": response.status_code}


async def approve_subscription(subscription_id: str) -> dict[str, Any] | None:
    return await _call_subscription_action(subscription_id, "approve")


async def reject_subscription(subscription_id: str, reason: str) -> dict[str, Any] | None:
    return await _call_subscription_action(
        subscription_id,
        "reject",
        payload={"reason": reason},
    )


async def verify_telegram_bind(telegram_id: int, code: str) -> Optional[dict]:
    endpoint = "verify_telegram_bind"
    url = f"{API_URL}/api/v1/internal/telegram/bind"
    response, elapsed_ms = await _request(
        endpoint,
        "POST",
        url,
        timeout=_DEFAULT_TIMEOUT,
        headers=_INTERNAL_HEADERS,
        json={"telegramId": telegram_id, "code": code},
        log_fields={"telegramId": telegram_id},
    )
    if response is None:
        return None

    payload = _load_json(
        response,
        endpoint,
        "POST",
        elapsed_ms=elapsed_ms,
        telegramId=telegram_id,
    )
    if payload is None:
        return None
    if response.status_code == 200:
        return payload
    return {"error": payload.get("error", "绑定失败")}


async def get_account_info(telegram_id: int) -> Optional[dict]:
    endpoint = "get_account_info"
    url = f"{API_URL}/api/v1/internal/telegram/info"
    response, elapsed_ms = await _request(
        endpoint,
        "POST",
        url,
        timeout=_DEFAULT_TIMEOUT,
        headers=_INTERNAL_HEADERS,
        json={"telegramId": telegram_id},
        log_fields={"telegramId": telegram_id},
    )
    if response is None:
        return None

    payload = _load_json(
        response,
        endpoint,
        "POST",
        elapsed_ms=elapsed_ms,
        telegramId=telegram_id,
    )
    if payload is None:
        return None
    if response.status_code == 200:
        return payload
    return {"error": payload.get("error", "查询失败"), "status": response.status_code}


async def get_media_stats() -> Optional[dict]:
    endpoint = "get_media_stats"
    url = f"{API_URL}/api/v1/internal/media/stats"
    response, elapsed_ms = await _request(
        endpoint,
        "GET",
        url,
        timeout=_DEFAULT_TIMEOUT,
        headers=_INTERNAL_HEADERS,
    )
    if response is None:
        return None

    payload = _load_json(
        response,
        endpoint,
        "GET",
        elapsed_ms=elapsed_ms,
    )
    if payload is None:
        return None
    if response.status_code == 200:
        return payload
    return {"error": payload.get("error", "查询媒体库统计失败"), "status": response.status_code}


async def redeem_by_telegram(telegram_id: int, code: str) -> Optional[dict]:
    endpoint = "redeem_by_telegram"
    url = f"{API_URL}/api/v1/internal/telegram/redeem"
    response, elapsed_ms = await _request(
        endpoint,
        "POST",
        url,
        timeout=_DEFAULT_TIMEOUT,
        headers=_INTERNAL_HEADERS,
        json={"telegramId": telegram_id, "code": code},
        log_fields={"telegramId": telegram_id},
    )
    if response is None:
        return None

    payload = _load_json(
        response,
        endpoint,
        "POST",
        elapsed_ms=elapsed_ms,
        telegramId=telegram_id,
    )
    if payload is None:
        return None
    if response.status_code == 200:
        return payload
    return {"error": payload.get("error", "兑换失败")}


async def reset_password_by_telegram(telegram_id: int, new_password: str) -> Optional[dict]:
    """通过 Telegram 身份重置密码"""
    endpoint = "reset_password_by_telegram"
    url = f"{API_URL}/api/v1/internal/telegram/reset-password"
    response, elapsed_ms = await _request(
        endpoint,
        "POST",
        url,
        timeout=_DEFAULT_TIMEOUT,
        headers=_INTERNAL_HEADERS,
        json={"telegramId": telegram_id, "newPassword": new_password},
        log_fields={"telegramId": telegram_id},
    )
    if response is None:
        return None

    payload = _load_json(
        response,
        endpoint,
        "POST",
        elapsed_ms=elapsed_ms,
        telegramId=telegram_id,
    )
    if payload is None:
        return None
    if response.status_code == 200:
        return payload
    return {"error": payload.get("error", "密码重置失败")}


async def get_setting(key: str) -> str:
    endpoint = "get_setting"
    url = f"{API_URL}/api/v1/internal/settings/{key}"
    response, elapsed_ms = await _request(
        endpoint,
        "GET",
        url,
        timeout=_SETTING_TIMEOUT,
        headers=_INTERNAL_HEADERS,
        log_fields={"key": key},
    )
    if response is None or response.status_code != 200:
        return ""

    payload = _load_json(
        response,
        endpoint,
        "GET",
        elapsed_ms=elapsed_ms,
        key=key,
    )
    if payload is None:
        return ""
    return str(payload.get("value", ""))


async def get_settings(keys: list[str]) -> dict[str, str]:
    values = await asyncio.gather(*(get_setting(key) for key in keys))
    return {key: value for key, value in zip(keys, values)}


async def search_tmdb(query: str, media_type: str = "movie") -> Optional[dict]:
    """调用公开 TMDB 搜索 API（无需鉴权，直接 GET）"""
    endpoint = "search_tmdb"
    url = f"{API_URL}/api/v1/tmdb/search"
    response, elapsed_ms = await _request(
        endpoint,
        "GET",
        url,
        timeout=_DEFAULT_TIMEOUT,
        params={"query": query, "type": media_type},
        log_fields={"mediaType": media_type},
    )
    if response is None:
        return None

    payload = _load_json(
        response,
        endpoint,
        "GET",
        elapsed_ms=elapsed_ms,
        mediaType=media_type,
    )
    if payload is None:
        return None
    if response.status_code == 200:
        return payload
    return {"error": payload.get("error", "搜索失败")}


async def get_tmdb_tv_seasons(tmdb_id: str | int) -> Optional[dict]:
    endpoint = "get_tmdb_tv_seasons"
    url = f"{API_URL}/api/v1/tmdb/tv/{tmdb_id}/seasons"
    response, elapsed_ms = await _request(
        endpoint,
        "GET",
        url,
        timeout=_DEFAULT_TIMEOUT,
        log_fields={"tmdbId": str(tmdb_id)},
    )
    if response is None:
        return None

    payload = _load_json(
        response,
        endpoint,
        "GET",
        elapsed_ms=elapsed_ms,
        tmdbId=str(tmdb_id),
    )
    if payload is None:
        return None
    if response.status_code == 200:
        return payload
    return {"error": payload.get("error", "获取季列表失败")}


async def subscribe_by_telegram(
    telegram_id: int,
    media_type: str,
    name: str,
    tmdb_id: str,
    season: int = 0,
    poster_path: str = "",
) -> Optional[dict]:
    """通过 Telegram 身份创建求片订阅"""
    endpoint = "subscribe_by_telegram"
    url = f"{API_URL}/api/v1/internal/telegram/subscribe"

    payload = {
        "telegramId": telegram_id,
        "type": media_type,
        "name": name,
        "tmdbId": str(tmdb_id),
        "season": season,
        "posterPath": poster_path,
    }

    response, elapsed_ms = await _request(
        endpoint,
        "POST",
        url,
        timeout=_DEFAULT_TIMEOUT,
        headers=_INTERNAL_HEADERS,
        json=payload,
        log_fields={"telegramId": telegram_id, "tmdbId": str(tmdb_id)},
    )
    if response is None:
        return None

    data = _load_json(
        response,
        endpoint,
        "POST",
        elapsed_ms=elapsed_ms,
        telegramId=telegram_id,
        tmdbId=str(tmdb_id),
    )
    if data is None:
        return None
    if response.status_code == 200:
        return data
    return {"error": data.get("error", "订阅失败"), "status": response.status_code}


async def enqueue_pending_reject(chat_id: int, admin_user_id: str, subscription_id: str) -> bool:
    """入队拒绝待确认记录"""
    endpoint = "enqueue_pending_reject"
    url = f"{API_URL}/api/v1/internal/telegram/reject-request/enqueue"
    response, elapsed_ms = await _request(
        endpoint,
        "POST",
        url,
        timeout=_DEFAULT_TIMEOUT,
        headers=_INTERNAL_HEADERS,
        json={"chatId": chat_id, "adminUserId": admin_user_id, "subscriptionId": subscription_id},
        log_fields={"chatId": chat_id, "subscriptionId": subscription_id},
    )
    return response is not None and response.status_code == 200


async def pop_pending_reject(chat_id: int) -> Optional[dict]:
    """弹出最新未过期的拒绝待确认记录"""
    endpoint = "pop_pending_reject"
    url = f"{API_URL}/api/v1/internal/telegram/reject-request/pop"
    response, elapsed_ms = await _request(
        endpoint,
        "POST",
        url,
        timeout=_DEFAULT_TIMEOUT,
        headers=_INTERNAL_HEADERS,
        json={"chatId": chat_id},
        log_fields={"chatId": chat_id},
    )
    if response is None:
        return None

    payload = _load_json(response, endpoint, "POST", elapsed_ms=elapsed_ms, chatId=chat_id)
    if payload is None:
        return None
    if response.status_code == 200:
        return payload
    if response.status_code == 404:
        return None
    return None
