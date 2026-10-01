package minio

import "crypto/tls"

// Config holds the configuration for connecting to a MinIO server.
type Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Token     string
	UseSsl    bool

	// Region 区域（部分 S3 兼容存储签名 v4 必需），留空由 SDK 决定。
	Region string
	// ForcePathStyle 强制 path-style 寻址（兼容部分 S3 实现），默认 false。
	ForcePathStyle bool
	// TLSClientConfig 可选 TLS 配置（自签名 CA、跳过校验等场景）。
	// 设置后 SDK 的 HTTP 传输将携带该配置。
	TLSClientConfig *tls.Config
}
