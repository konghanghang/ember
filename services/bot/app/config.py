import os
from dataclasses import dataclass
from pathlib import Path
from typing import Optional

from dotenv import load_dotenv

BOT_ROOT = Path(__file__).resolve().parent.parent
load_dotenv(BOT_ROOT / ".env.local", override=False)
load_dotenv(BOT_ROOT / ".env", override=False)

TMDB_IMAGE_BASE = "https://image.tmdb.org/t/p/w300"
TMDB_IMAGE_BASE_W500 = "https://image.tmdb.org/t/p/w500"
TMDB_NO_POSTER_URL = "https://image.tmdb.org/t/p/w500/wwemzKWzjKYJFfCeiB57q3r4Bcm.png"
TELEGRAM_UPDATE_MODE_WEBHOOK = "webhook"
TELEGRAM_UPDATE_MODE_POLLING = "polling"


def _parse_optional_int(name: str) -> Optional[int]:
    raw = os.getenv(name, "").strip()
    if not raw:
        return None
    return int(raw)


def _parse_required_str(name: str) -> str:
    value = os.getenv(name, "").strip()
    if not value:
        raise RuntimeError(f"{name} is required")
    return value


def _parse_optional_str(name: str) -> str:
    return os.getenv(name, "").strip()


def _parse_update_mode() -> str:
    raw = os.getenv("TELEGRAM_UPDATE_MODE", TELEGRAM_UPDATE_MODE_WEBHOOK).strip().lower()
    if raw in {TELEGRAM_UPDATE_MODE_WEBHOOK, TELEGRAM_UPDATE_MODE_POLLING}:
        return raw
    raise RuntimeError(
        "TELEGRAM_UPDATE_MODE must be one of: "
        f"{TELEGRAM_UPDATE_MODE_WEBHOOK}, {TELEGRAM_UPDATE_MODE_POLLING}"
    )


@dataclass(frozen=True)
class BootstrapConfig:
    telegram_bot_token: str
    telegram_admin_chat_id: Optional[int]
    telegram_group_chat_id: Optional[int]
    telegram_webhook_secret: str
    internal_api_secret: str
    webhook_url: str
    api_url: str
    bot_port: int
    telegram_update_mode: str


def load_bootstrap_config() -> BootstrapConfig:
    telegram_update_mode = _parse_update_mode()
    telegram_webhook_secret = _parse_optional_str("TELEGRAM_WEBHOOK_SECRET")
    webhook_url = _parse_optional_str("WEBHOOK_URL").rstrip("/")

    if telegram_update_mode == TELEGRAM_UPDATE_MODE_WEBHOOK:
        if not telegram_webhook_secret:
            raise RuntimeError("TELEGRAM_WEBHOOK_SECRET is required when TELEGRAM_UPDATE_MODE=webhook")
        if not webhook_url:
            raise RuntimeError("WEBHOOK_URL is required when TELEGRAM_UPDATE_MODE=webhook")

    return BootstrapConfig(
        telegram_bot_token=_parse_required_str("TELEGRAM_BOT_TOKEN"),
        telegram_admin_chat_id=_parse_optional_int("TELEGRAM_ADMIN_CHAT_ID"),
        telegram_group_chat_id=_parse_optional_int("TELEGRAM_GROUP_CHAT_ID"),
        telegram_webhook_secret=telegram_webhook_secret,
        internal_api_secret=_parse_required_str("INTERNAL_API_SECRET"),
        webhook_url=webhook_url,
        api_url=os.getenv("API_URL", "http://localhost:8080").rstrip("/"),
        bot_port=int(os.getenv("BOT_PORT", "8000")),
        telegram_update_mode=telegram_update_mode,
    )


BOOTSTRAP_CONFIG = load_bootstrap_config()

TELEGRAM_BOT_TOKEN = BOOTSTRAP_CONFIG.telegram_bot_token
TELEGRAM_ADMIN_CHAT_ID = BOOTSTRAP_CONFIG.telegram_admin_chat_id
TELEGRAM_GROUP_CHAT_ID = BOOTSTRAP_CONFIG.telegram_group_chat_id
TELEGRAM_WEBHOOK_SECRET = BOOTSTRAP_CONFIG.telegram_webhook_secret
INTERNAL_API_SECRET = BOOTSTRAP_CONFIG.internal_api_secret
WEBHOOK_URL = BOOTSTRAP_CONFIG.webhook_url
API_URL = BOOTSTRAP_CONFIG.api_url
BOT_PORT = BOOTSTRAP_CONFIG.bot_port
TELEGRAM_UPDATE_MODE = BOOTSTRAP_CONFIG.telegram_update_mode
