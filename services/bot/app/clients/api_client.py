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
        return {"error": resp.json().get("error", "查询失败")}
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
