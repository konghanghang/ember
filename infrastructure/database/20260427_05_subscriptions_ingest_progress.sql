-- subscriptions：增加 ingestProgress 列，用于整剧（season=0）订阅的入库进度展示。
--
-- 业务背景：
--   原 `MarkSubscriptionsIngestedByWebhook` 在收到任意一集的 Emby webhook 时就把整剧
--   订阅 (season=0) 收口为 INGESTED，让用户和管理员误以为整剧入库完成；实际上还有大量
--   集数缺失。本批次拆分整剧 / 单季的命中策略：
--     - 单季订阅 (season=N)：webhook 命中即 INGEST（与原行为一致，只是查询条件收紧）
--     - 整剧订阅 (season=0)：webhook 仅更新 ingestProgress，不收口为 INGESTED
--   ingestProgress 用 "Si Ej" 记录最近一集的命中信息，便于前端 / 管理员观察整剧进度。
--   由管理员在前端通过 `MarkSubscriptionIngestedAsAdmin` 显式收口整剧入库状态。
--
-- 字段说明：
--   ingestProgress  整剧订阅的入库进度文本，最多 50 字符，可空（单季订阅永远为空）
--
-- 幂等：ADD COLUMN IF NOT EXISTS。

ALTER TABLE subscriptions
    ADD COLUMN IF NOT EXISTS "ingestProgress" varchar(50);
