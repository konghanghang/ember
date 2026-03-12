import httpx
from typing import Optional

from app.config import API_URL, INTERNAL_API_SECRET


async def _call_subscription_action(subscription_id: str, action: str) -> bool:
    url = f"{API_URL}/api/v1/internal/subscriptions/{subscription_id}/{action}"
    headers = {"X-Internal-Secret": INTERNAL_API_SECRET}

    try:
        async with httpx.AsyncClient(timeout=10.0) as client:
            resp = await client.put(url, headers=headers)
        return resp.status_code == 200
    except Exception:
        return False


async def approve_subscription(subscription_id: str) -> bool:
    return await _call_subscription_action(subscription_id, "approve")


async def reject_subscription(subscription_id: str) -> bool:
    return await _call_subscription_action(subscription_id, "reject")


async def verify_telegram_bind(telegram_id: int, code: str) -> Optional[dict]:
    url = f"{API_URL}/api/v1/internal/telegram/bind"
    headers = {"X-Internal-Secret": INTERNAL_API_SECRET}

    try:
        async with httpx.AsyncClient(timeout=10.0) as client:
            resp = await client.post(
                url,
                headers=headers,
                json={
                    "telegramId": telegram_id,
                    "code": code,
                },
            )
        if resp.status_code == 200:
            return resp.json()
        return {"error": resp.json().get("error", "绑定失败")}
    except Exception:
        return None


async def get_account_info(telegram_id: int) -> Optional[dict]:
    url = f"{API_URL}/api/v1/internal/telegram/info"
    headers = {"X-Internal-Secret": INTERNAL_API_SECRET}

    try:
        async with httpx.AsyncClient(timeout=10.0) as client:
            resp = await client.post(
                url,
                headers=headers,
                json={"telegramId": telegram_id},
            )
        if resp.status_code == 200:
            return resp.json()
        return {"error": resp.json().get("error", "查询失败"), "status": resp.status_code}
    except Exception:
        return None


async def redeem_by_telegram(telegram_id: int, code: str) -> Optional[dict]:
    url = f"{API_URL}/api/v1/internal/telegram/redeem"
    headers = {"X-Internal-Secret": INTERNAL_API_SECRET}

    try:
        async with httpx.AsyncClient(timeout=10.0) as client:
            resp = await client.post(
                url,
                headers=headers,
                json={
                    "telegramId": telegram_id,
                    "code": code,
                },
            )
        if resp.status_code == 200:
            return resp.json()
        return {"error": resp.json().get("error", "兑换失败")}
    except Exception:
        return None


async def reset_password_by_telegram(telegram_id: int, new_password: str) -> Optional[dict]:
    """通过 Telegram 身份重置密码"""
    url = f"{API_URL}/api/v1/internal/telegram/reset-password"
    headers = {"X-Internal-Secret": INTERNAL_API_SECRET}

    try:
        async with httpx.AsyncClient(timeout=10.0) as client:
            resp = await client.post(
                url,
                headers=headers,
                json={
                    "telegramId": telegram_id,
                    "newPassword": new_password,
                },
            )
        if resp.status_code == 200:
            return resp.json()
        return {"error": resp.json().get("error", "密码重置失败")}
    except Exception:
        return None


async def get_setting(key: str) -> str:
    url = f"{API_URL}/api/v1/internal/settings/{key}"
    headers = {"X-Internal-Secret": INTERNAL_API_SECRET}

    try:
        async with httpx.AsyncClient(timeout=5.0) as client:
            resp = await client.get(url, headers=headers)
        if resp.status_code == 200:
            return str(resp.json().get("value", ""))
    except Exception:
        pass

    return ""


async def get_settings(keys: list[str]) -> dict[str, str]:
    results: dict[str, str] = {}
    for key in keys:
        results[key] = await get_setting(key)
    return results


async def search_tmdb(query: str, media_type: str = "movie") -> Optional[dict]:
    """调用公开 TMDB 搜索 API（无需鉴权，直接 GET）"""
    url = f"{API_URL}/api/v1/tmdb/search"
    params = {"query": query, "type": media_type}

    try:
        async with httpx.AsyncClient(timeout=10.0) as client:
            resp = await client.get(url, params=params)
        if resp.status_code == 200:
            return resp.json()
        return {"error": resp.json().get("error", "搜索失败")}
    except Exception:
        return None


async def subscribe_by_telegram(
    telegram_id: int,
    media_type: str,
    name: str,
    tmdb_id: str,
    poster_path: str = "",
    note: str = "",
) -> Optional[dict]:
    """通过 Telegram 身份创建求片订阅"""
    url = f"{API_URL}/api/v1/internal/telegram/subscribe"
    headers = {"X-Internal-Secret": INTERNAL_API_SECRET}

    payload = {
        "telegramId": telegram_id,
        "type": media_type,
        "name": name,
        "tmdbId": str(tmdb_id),
        "posterPath": poster_path,
        "note": note,
    }

    try:
        async with httpx.AsyncClient(timeout=10.0) as client:
            resp = await client.post(url, headers=headers, json=payload)
        if resp.status_code == 200:
            return resp.json()
        return {"error": resp.json().get("error", "订阅失败"), "status": resp.status_code}
    except Exception:
        return None
