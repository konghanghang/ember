import httpx

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
