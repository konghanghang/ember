# MoviePilot 集成

Ember 可以在求片审批通过后，把订阅同步到 MoviePilot。

## 适用场景

- 用户发起求片订阅
- 管理员审批通过
- 希望自动把请求同步到 MoviePilot

## 关键配置

- `MOVIEPILOT_URL`
- `MOVIEPILOT_USERNAME`
- `MOVIEPILOT_PASSWORD`

## 说明

- 这项能力是可选的
- 未配置时，不影响 Ember 本身的订阅流程
- 但不会自动把订阅同步到 MoviePilot

## 继续阅读

- [配置说明](../configuration.md)
