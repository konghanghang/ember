import os
import sys
from pathlib import Path


BOT_ROOT = Path(__file__).resolve().parents[1]
BOT_ROOT_STR = str(BOT_ROOT)

if BOT_ROOT_STR not in sys.path:
    sys.path.insert(0, BOT_ROOT_STR)


os.environ.setdefault("TELEGRAM_BOT_TOKEN", "test-token")
os.environ.setdefault("INTERNAL_API_SECRET", "test-secret")
