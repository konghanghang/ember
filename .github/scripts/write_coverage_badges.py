#!/usr/bin/env python3

import json
import os
from pathlib import Path


BADGES = {
    "api": {"label": "API Coverage", "env": "API_COVERAGE"},
    "web": {"label": "Web Coverage", "env": "WEB_COVERAGE"},
    "bot": {"label": "Bot Coverage", "env": "BOT_COVERAGE"},
}


def badge_color(percent: float) -> str:
    if percent >= 80:
        return "brightgreen"
    if percent >= 60:
        return "yellowgreen"
    if percent >= 40:
        return "yellow"
    if percent >= 20:
        return "orange"
    return "red"


def build_badge(label: str, raw_value: str) -> dict[str, object]:
    value = (raw_value or "").strip()
    if not value:
        return {
            "schemaVersion": 1,
            "label": label,
            "message": "pending",
            "color": "lightgrey",
        }

    percent = round(float(value), 1)
    return {
        "schemaVersion": 1,
        "label": label,
        "message": f"{percent:.1f}%",
        "color": badge_color(percent),
    }


def main() -> None:
    output_dir = Path(".github/badges")
    output_dir.mkdir(parents=True, exist_ok=True)

    for name, config in BADGES.items():
        badge = build_badge(config["label"], os.getenv(config["env"], ""))
        target = output_dir / f"{name}-coverage.json"
        target.write_text(json.dumps(badge, ensure_ascii=True, indent=2) + "\n", encoding="utf-8")


if __name__ == "__main__":
    main()
