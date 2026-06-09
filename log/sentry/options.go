package sentry

type options struct {
	dsn         string
	environment string
	release     string
	serverName  string
}

type Option func(*options)

func defaultOptions() *options {
	return &options{
		environment: "development",
	}
}

// WithDSN 设置 Sentry 项目 DSN。
// 格式：https://<publickey>@o<orgid>.ingest.sentry.io/<projectid>
func WithDSN(dsn string) Option {
	return func(o *options) {
		o.dsn = dsn
	}
}

// WithEnvironment 设置运行环境标签。
// 例如 "production"、"staging"、"development"（默认）。
func WithEnvironment(env string) Option {
	return func(o *options) {
		o.environment = env
	}
}

// WithRelease 设置发布版本标签。
// 例如 "myapp@1.0.0"。
func WithRelease(release string) Option {
	return func(o *options) {
		o.release = release
	}
}

// WithServerName 设置服务器名称标签。
func WithServerName(name string) Option {
	return func(o *options) {
		o.serverName = name
	}
}
