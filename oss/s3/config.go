package s3

import "crypto/tls"

// Config holds the configuration for connecting to an S3-compatible storage.
type Config struct {
	Endpoint       string
	Region         string
	AccessKey      string
	SecretKey      string
	Token          string
	UseSsl         bool
	ForcePathStyle bool
	Bucket         string

	// TLSClientConfig 可选 TLS 配置（自签名 CA、跳过校验等场景），
	// 应用于自定义 HTTP 客户端。
	TLSClientConfig *tls.Config
	// MaxAttempts 命令失败最大重试次数，0 表示使用 SDK 默认值。
	MaxAttempts int
}
