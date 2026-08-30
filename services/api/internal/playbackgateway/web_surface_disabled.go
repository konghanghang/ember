package playbackgateway

import (
	"io"
	"net/http"
	"strconv"
)

const webSurfaceDisabledPage = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Emby 网页访问已关闭</title>
  <style>
    :root {
      color-scheme: light;
      font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      color: #111827;
      background: #f9fafb;
    }
    * {
      box-sizing: border-box;
    }
    body {
      min-height: 100vh;
      margin: 0;
      display: grid;
      place-items: center;
      padding: 24px;
      background: #f9fafb;
    }
    main {
      width: min(100%, 440px);
      padding: 40px 32px;
      text-align: center;
      background: #ffffff;
      border: 1px solid #f3f4f6;
      border-radius: 16px;
      box-shadow: 0 10px 30px rgba(17, 24, 39, 0.08);
    }
    .status-icon {
      width: 56px;
      height: 56px;
      margin: 0 auto;
      display: grid;
      place-items: center;
      color: #e50914;
      background: #fef2f2;
      border: 1px solid #fee2e2;
      border-radius: 50%;
    }
    .status-icon svg {
      width: 28px;
      height: 28px;
    }
    h1 {
      margin: 20px 0 0;
      font-size: 24px;
      line-height: 1.3;
      font-weight: 700;
      letter-spacing: -0.02em;
    }
    p {
      margin: 12px auto 0;
      color: #6b7280;
      font-size: 14px;
      line-height: 1.7;
    }
    @media (max-width: 480px) {
      main {
        padding: 32px 24px;
      }
      h1 {
        font-size: 22px;
      }
    }
  </style>
</head>
<body>
  <main aria-labelledby="page-title">
    <div class="status-icon" aria-hidden="true">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
        <rect x="5" y="10" width="14" height="10" rx="2"></rect>
        <path d="M8 10V7a4 4 0 0 1 8 0v3"></path>
      </svg>
    </div>
    <h1 id="page-title">Emby 网页访问已关闭</h1>
    <p>当前服务器不提供网页访问，请使用受支持的 Emby 客户端，或联系管理员。</p>
  </main>
</body>
</html>
`

// writeWebSurfaceDisabledResponse returns a self-contained, non-cacheable
// status page without depending on the disabled Emby Web surface or upstream.
func writeWebSurfaceDisabledResponse(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Language", "zh-CN")
	writer.Header().Set("Content-Length", strconv.Itoa(len(webSurfaceDisabledPage)))
	writer.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(http.StatusNotFound)
	if request != nil && request.Method == http.MethodHead {
		return
	}
	_, _ = io.WriteString(writer, webSurfaceDisabledPage)
}
