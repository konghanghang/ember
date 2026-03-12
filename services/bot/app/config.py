import os
from pathlib import Path

from dotenv import load_dotenv

BOT_ROOT = Path(__file__).resolve().parent.parent
load_dotenv(BOT_ROOT / ".env.local", override=False)
load_dotenv(BOT_ROOT / ".env", override=False)

TELEGRAM_BOT_TOKEN = os.environ["TELEGRAM_BOT_TOKEN"]
_admin_chat_id = os.getenv("TELEGRAM_ADMIN_CHAT_ID", "").strip()
TELEGRAM_ADMIN_CHAT_ID = int(_admin_chat_id) if _admin_chat_id else None
_group_chat_id = os.getenv("TELEGRAM_GROUP_CHAT_ID", "").strip()
TELEGRAM_GROUP_CHAT_ID = int(_group_chat_id) if _group_chat_id else None
TELEGRAM_WEBHOOK_SECRET = os.environ["TELEGRAM_WEBHOOK_SECRET"]
INTERNAL_API_SECRET = os.environ["INTERNAL_API_SECRET"]
WEBHOOK_URL = os.environ["WEBHOOK_URL"].rstrip("/")

API_URL = os.getenv("API_URL", "http://localhost:8080").rstrip("/")
BOT_PORT = int(os.getenv("BOT_PORT", "8000"))

TMDB_IMAGE_BASE = "https://image.tmdb.org/t/p/w300"
TMDB_IMAGE_BASE_W500 = "https://image.tmdb.org/t/p/w500"
TMDB_NO_POSTER_URL = "https://image.tmdb.org/t/p/w500/wwemzKWzjKYJFfCeiB57q3r4Bcm.png"
