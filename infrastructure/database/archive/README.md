# 数据库迁移历史归档

本目录仅用于追溯历史 SQL 迁移演化路径，**不属于任何现行执行链路**。

普通使用者不需要关注本目录。新装库或升级请按 [`infrastructure/database/README.md`](../README.md) 走顶层 baseline。

## 截点目录

按 baseline 截点划分，命名格式 `pre-<YYYYMMDD>` 表示"该日期截点 baseline 完整覆盖之前的所有迁移已归档于此"：

- `pre-20260415/`：v1.2.x 至 v1.3.0 期间的早期迁移，被首轮 baseline `20260415_00_schema_baseline.sql` 完整覆盖。
- `pre-20260422/`：v1.3.1 截点的旧 baseline 与同期增量，被次轮 baseline `20260422_00_schema_baseline.sql` 完整覆盖。
- `pre-20260502/`：v1.4.0 截点的旧 baseline 与 23 个增量，被合并式 baseline `00000000_baseline_20260502.sql`（曾命名为 `20260502_00_schema_baseline.sql`）完整覆盖。
- `pre-20260605/`：2026-05-02 fresh-install baseline 与 4 个后续增量，被当前 baseline `00000000_baseline_20260605.sql` 完整覆盖。

## 边界

- 归档文件名保持原样，不允许在归档时改名。
- 归档文件不再参与新装库初始化、Docker initdb、生产升级等任何运行时链路。
- 当前 baseline 作为唯一现行入口，只要它能在新装空库上跑通，归档目录是否完整都不影响系统运行。
- 排错或验证字段历史时，可在此查阅原始迁移 SQL；其它场景应优先查 git log。
