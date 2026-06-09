package loki

import (
	"net/http"
	"time"
)

type options struct {
	endpoint      string
	labels        map[string]string
	httpClient    *http.Client
	batchSize     int
	flushInterval time.Duration
}

type Option func(*options)

func defaultOptions() *options {
	return &options{
		labels:        map[string]string{"app": "default"},
		batchSize:     100,
		flushInterval: 5 * time.Second,
	}
}

// WithEndpoint 设置 Loki HTTP Push API 端点地址。
// 例如 "http://loki:3100/loki/api/v1/push"
func WithEndpoint(endpoint string) Option {
	return func(o *options) {
		o.endpoint = endpoint
	}
}

// WithLabel 添加一个 Loki 流标签。
// 可以多次调用以添加多个标签。
func WithLabel(key, value string) Option {
	return func(o *options) {
		if o.labels == nil {
			o.labels = make(map[string]string)
		}
		o.labels[key] = value
	}
}

// WithBatchSize 设置批量推送阈值。
// 默认 100 条。
func WithBatchSize(size int) Option {
	return func(o *options) {
		o.batchSize = size
	}
}

// WithFlushInterval 设置 HTTP 请求超时时间。
// 默认 5 秒。
func WithFlushInterval(d time.Duration) Option {
	return func(o *options) {
		o.flushInterval = d
	}
}

// WithHTTPClient 设置自定义 HTTP 客户端。
func WithHTTPClient(c *http.Client) Option {
	return func(o *options) {
		o.httpClient = c
	}
}
