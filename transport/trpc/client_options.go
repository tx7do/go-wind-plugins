package trpc

import (
	"crypto/tls"
	"time"

	trpcClient "trpc.group/trpc-go/trpc-go/client"
	"trpc.group/trpc-go/trpc-go/filter"
)

type ClientOption func(o *Client)

// WithTarget 设置目标服务地址（ip:port 或服务发现标识）。
func WithTarget(target string) ClientOption {
	return func(c *Client) {
		c.clientOptions = append(c.clientOptions, trpcClient.WithTarget(target))
	}
}

// WithServiceName 设置调用目标服务名称。
func WithClientServiceName(name string) ClientOption {
	return func(c *Client) {
		c.clientOptions = append(c.clientOptions, trpcClient.WithServiceName(name))
	}
}

// WithClientNamespace 设置调用方命名空间。
func WithClientNamespace(namespace string) ClientOption {
	return func(c *Client) {
		c.clientOptions = append(c.clientOptions, trpcClient.WithNamespace(namespace))
	}
}

// WithClientEnvName 设置调用方环境名称。
func WithClientEnvName(envName string) ClientOption {
	return func(c *Client) {
		c.clientOptions = append(c.clientOptions, trpcClient.WithCallerEnvName(envName))
	}
}

// WithClientSetName 设置调用方分组名称。
func WithClientSetName(setName string) ClientOption {
	return func(c *Client) {
		c.clientOptions = append(c.clientOptions, trpcClient.WithCallerSetName(setName))
	}
}

// WithClientTimeout 设置客户端调用超时时间。
func WithClientTimeout(timeout time.Duration) ClientOption {
	return func(c *Client) {
		c.clientOptions = append(c.clientOptions, trpcClient.WithTimeout(timeout))
	}
}

// WithClientNetwork 设置网络协议（tcp/udp 等）。
func WithClientNetwork(network string) ClientOption {
	return func(c *Client) {
		c.clientOptions = append(c.clientOptions, trpcClient.WithNetwork(network))
	}
}

// WithClientProtocol 设置应用层协议（trpc/http 等）。
func WithClientProtocol(protocol string) ClientOption {
	return func(c *Client) {
		c.clientOptions = append(c.clientOptions, trpcClient.WithProtocol(protocol))
	}
}

// WithClientTLS 设置客户端 TLS 证书。
func WithClientTLS(certFile, keyFile, caFile, serverName string) ClientOption {
	return func(c *Client) {
		c.clientOptions = append(c.clientOptions, trpcClient.WithTLS(certFile, keyFile, caFile, serverName))
	}
}

// WithClientFilter 注入客户端过滤器（拦截器）。
func WithClientFilter(f filter.ClientFilter) ClientOption {
	return func(c *Client) {
		c.filters = append(c.filters, f)
	}
}

// WithClientFilters 批量注入客户端过滤器。
func WithClientFilters(fs []filter.ClientFilter) ClientOption {
	return func(c *Client) {
		c.filters = append(c.filters, fs...)
	}
}

// WithMetaData 设置元数据。
func WithMetaData(key string, val []byte) ClientOption {
	return func(c *Client) {
		c.clientOptions = append(c.clientOptions, trpcClient.WithMetaData(key, val))
	}
}

// WithDiscoveryName 设置服务发现名称。
func WithDiscoveryName(name string) ClientOption {
	return func(c *Client) {
		c.clientOptions = append(c.clientOptions, trpcClient.WithDiscoveryName(name))
	}
}

// WithBalancerName 设置负载均衡名称。
func WithBalancerName(name string) ClientOption {
	return func(c *Client) {
		c.clientOptions = append(c.clientOptions, trpcClient.WithBalancerName(name))
	}
}

// WithCalleeMethod 设置调用目标方法名。
func WithCalleeMethod(method string) ClientOption {
	return func(c *Client) {
		c.clientOptions = append(c.clientOptions, trpcClient.WithCalleeMethod(method))
	}
}

// WithClientOptions 直接注入 tRPC 原生 Client 选项。
func WithClientOptions(opts ...trpcClient.Option) ClientOption {
	return func(c *Client) {
		c.clientOptions = append(c.clientOptions, opts...)
	}
}

// WithClientTLSConfig 使用 *tls.Config 设置 TLS。
// 注意：tRPC 框架不直接支持 *tls.Config，保留接口以兼容。
func WithClientTLSConfig(_ *tls.Config) ClientOption {
	return func(c *Client) {
		// tRPC 不直接支持 *tls.Config，保留接口以兼容
	}
}
