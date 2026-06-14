package moviepilot

import "errors"

// ErrMoviePilotBusinessRejected 表示 MoviePilot 在业务层拒绝下载请求（如重复添加、种子校验失败）。
//
// 与基础设施错误（网络超时 / HTTP 5xx）的区别：此时上游服务正常，仅业务规则不满足。
// 错误信息已脱敏（只保留 MoviePilot 返回的 message 文本，不含请求 URL / api_key），
// 可安全透传给管理员或写入 last_dispatch_error。
//
// 调用方通过 errors.Is(err, ErrMoviePilotBusinessRejected) 判定，将业务拒绝映射为
// 客户端可处理的状态码（如 409 Conflict），而不是 500 基础设施故障。
var ErrMoviePilotBusinessRejected = errors.New("MoviePilot 拒绝下载请求")
