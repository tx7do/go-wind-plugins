package health

import (
	"context"
	"net/http"
	"time"
)

// defaultClient 是 HTTPChecker 使用的默认 HTTP 客户端。
var defaultClient = &http.Client{
	Timeout: 5 * time.Second,
}

// httpClient 返回默认 HTTP 客户端。
// 虽然字段名以小写开头（不导出），但函数名导出以便未来可注入自定义客户端。
func httpClient() *http.Client {
	return defaultClient
}

// newHTTPRequest 构建一个 GET 请求。
func newHTTPRequest(ctx context.Context, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "go-wind-health-checker")
	return req, nil
}
