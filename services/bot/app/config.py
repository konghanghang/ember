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


def _parse_optional_int(name: str) -> Optional[int]:
    raw = os.getenv(name, "").strip()
    if not raw:
        return None
    return int(raw)


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


def load_bootstrap_config() -> BootstrapConfig:
    return BootstrapConfig(
        telegram_bot_token=os.environ["TELEGRAM_BOT_TOKEN"],
        telegram_admin_chat_id=_parse_optional_int("TELEGRAM_ADMIN_CHAT_ID"),
        telegram_group_chat_id=_parse_optional_int("TELEGRAM_GROUP_CHAT_ID"),
        telegram_webhook_secret=os.environ["TELEGRAM_WEBHOOK_SECRET"],
        internal_api_secret=os.environ["INTERNAL_API_SECRET"],
        webhook_url=os.environ["WEBHOOK_URL"].rstrip("/"),
        api_url=os.getenv("API_URL", "http://localhost:8080").rstrip("/"),
        bot_port=int(os.getenv("BOT_PORT", "8000")),
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
