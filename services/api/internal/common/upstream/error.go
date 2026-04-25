package upstream

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// SafeUpstreamError 把来自上游的 Go 错误转换为不含敏感细节（请求 URL / api_key / 请求体）的安全错误，
// 用于回写日志或包装到 API 响应。仅保留 system 名称与操作粒度。
//
// system 取值示例：tmdb / moviepilot / emby / stripe / smtp。
func SafeUpstreamError(err error, system string) error {
	if err == nil {
		return nil
	}

	system = strings.TrimSpace(system)
	if system == "" {
		system = "unknown"
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		op := strings.TrimSpace(urlErr.Op)
		if op == "" {
			op = "request"
		}
		return fmt.Errorf("upstream %s failed: op=%s", system, op)
	}

	return fmt.Errorf("upstream %s unavailable", system)
}

// SafeUpstreamHTTPError 把上游 HTTP 非 2xx 响应转换为不含响应体的安全错误，
// 仅保留 system 与 statusCode。响应体可能含上游凭证或被回显的请求 URL，禁止透传。
func SafeUpstreamHTTPError(system string, statusCode int) error {
	system = strings.TrimSpace(system)
	if system == "" {
		system = "unknown"
	}
	return fmt.Errorf("upstream %s http %d", system, statusCode)
}
