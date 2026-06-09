package cloudwatch

import "time"

type options struct {
	region        string
	logGroup      string
	logStream     string
	batchSize     int
	flushInterval time.Duration
}

type Option func(*options)

func defaultOptions() *options {
	return &options{
		region:        "us-east-1",
		logGroup:      "app",
		logStream:     "default",
		batchSize:     100,
		flushInterval: 5 * time.Second,
	}
}

// WithRegion 设置 AWS 区域。
func WithRegion(region string) Option {
	return func(o *options) {
		o.region = region
	}
}

// WithLogGroup 设置 CloudWatch 日志组名称。
func WithLogGroup(group string) Option {
	return func(o *options) {
		o.logGroup = group
	}
}

// WithLogStream 设置 CloudWatch 日志流名称。
func WithLogStream(stream string) Option {
	return func(o *options) {
		o.logStream = stream
	}
}

// WithBatchSize 设置批量刷新阈值（缓冲日志条目数达到此值时自动刷新）。
// 默认 100 条。
func WithBatchSize(size int) Option {
	return func(o *options) {
		o.batchSize = size
	}
}

// WithFlushInterval 设置定时刷新间隔。
// 默认 5 秒。设置为 0 可禁用定时刷新（仅按 batchSize 触发）。
func WithFlushInterval(d time.Duration) Option {
	return func(o *options) {
		o.flushInterval = d
	}
}
