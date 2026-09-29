#!/usr/bin/env python3
"""在内存 SQLite 中对比当前两次聚合与合并候选；只使用合成数据，不连接服务。

运行：python3 scripts/test/playback-ranking-query-cost.py
结果只描述本机 SQL 成本，不包含网络延迟，不证明目标 Emby 的性能。
"""

import json
from pathlib import Path
import re
import sqlite3
from statistics import median
from time import perf_counter


def load_query_template():
    """读取当前生产 SQL；模板结构变化时明确失败，避免继续比较过时副本。"""
    root = Path(__file__).resolve().parents[2]
    source = (root / "services/api/internal/services/playback/ranking.go").read_text()
    match = re.search(r"sql := fmt.Sprintf\(`\n(SELECT %s AS item_key,.*?)`", source, re.S)
    if not match or match.group(1).count("%s") != 8:
        raise RuntimeError("排行榜生产聚合模板已变化，请重新核对成本实验")
    template = match.group(1)
    if "WHERE ItemType = '%s'" not in template or "GROUP BY %s\n" not in template:
        raise RuntimeError("聚合条件已变化，请重新核对合并候选")
    return template


def build_queries(template, start, end):
    """保留现行聚合表达式、排序与日期边界，仅将候选改为按类型和 ID 一次分组。"""
    key = "NULLIF(TRIM(COALESCE(ItemId, '')), '')"
    name = "NULLIF(TRIM(COALESCE(ItemName, '')), '')"
    current = [
        template % (key, name, source, kind, start, end, key, key)
        for kind, source in (("Movie", "movie_item"), ("Episode", "episode_item"))
    ]
    candidate = template.replace(
        "'%s' AS item_source_type",
        "CASE ItemType WHEN 'Movie' THEN 'movie_item' ELSE 'episode_item' END AS item_source_type",
    ).replace(
        "WHERE ItemType = '%s'", "WHERE ItemType IN ('Movie', 'Episode')"
    ).replace("GROUP BY %s\n", "GROUP BY ItemType, %s\n")
    return current, [candidate % (key, name, start, end, key, key)]


def run_queries(connection, queries):
    """完整读取聚合结果，计时包含 SQLite 执行和 Python 接收结果的成本。"""
    start = perf_counter()
    rows = [row for query in queries for row in connection.execute(query)]
    return rows, (perf_counter() - start) * 1000


def main():
    """构造无附加索引的 24 万条历史记录，交替执行日、周、月窗并比较结果与中位耗时。"""
    template = load_query_template()
    with sqlite3.connect(":memory:") as connection:
        connection.execute("""CREATE TABLE PlaybackActivity (
            DateCreated DATETIME NOT NULL, UserId TEXT, ItemId TEXT, ItemType TEXT,
            ItemName TEXT, PlaybackMethod TEXT, ClientName TEXT, DeviceName TEXT,
            PlayDuration INT, PauseDuration INT, RemoteAddress TEXT, TranscodeReasons TEXT
        )""")
        connection.executemany(
            "INSERT INTO PlaybackActivity(DateCreated, ItemId, ItemType, ItemName, PlayDuration, PauseDuration) VALUES(?,?,?,?,?,?)",
            (
                (f"2026-09-{1 + i % 28:02d} 12:00:00", f"item_{i % 12000}",
                 ("Movie", "Episode", "Audio")[i % 3], f"Name {i % 4000}", i % 3600, i % 30)
                for i in range(240000)
            ),
        )
        print(json.dumps({"sqlite": sqlite3.sqlite_version, "syntheticRows": 240000, "runs": 7, "externalCalls": 0}))
        for window, start, end in (
            ("daily", "2026-09-14 00:00:00", "2026-09-15 00:00:00"),
            ("weekly", "2026-09-14 00:00:00", "2026-09-21 00:00:00"),
            ("monthly", "2026-09-01 00:00:00", "2026-10-01 00:00:00"),
        ):
            current, candidate = build_queries(template, start, end)
            baseline, _ = run_queries(connection, current)
            proposed, _ = run_queries(connection, candidate)
            assert sorted(baseline) == sorted(proposed), f"{window}: 聚合结果不一致"
            durations = {"current": [], "candidate": []}
            for iteration in range(7):
                order = [("current", current), ("candidate", candidate)]
                if iteration % 2:
                    order.reverse()
                for label, queries in order:
                    _, elapsed = run_queries(connection, queries)
                    durations[label].append(elapsed)
            plans = {
                label: [row[3] for query in queries for row in connection.execute("EXPLAIN QUERY PLAN " + query)]
                for label, queries in (("current", current), ("candidate", candidate))
            }
            print(json.dumps({
                "window": window, "aggregateRows": len(baseline), "resultsEqual": True,
                "currentMs": round(median(durations["current"]), 2),
                "candidateMs": round(median(durations["candidate"]), 2), "plans": plans,
            }))


if __name__ == "__main__":
    main()
