package trpc

import (
	"context"
	"time"

	trpcClient "trpc.group/trpc-go/trpc-go/client"
	"trpc.group/trpc-go/trpc-go/filter"
)

// ClientFilter 是 tRPC 客户端过滤器（拦截器）的类型别名。
type ClientFilter = filter.ClientFilter

// Client 是 tRPC 客户端的封装，提供统一的 RPC 调用能力。
type Client struct {
	Client trpcClient.Client

	clientOptions []trpcClient.Option
	filters       []ClientFilter

	err error
}

func NewClient(opts ...ClientOption) *Client {
	c := &Client{
		Client: trpcClient.New(),
	}

	c.init(opts...)

	return c
}

func (c *Client) init(opts ...ClientOption) {
	for _, o := range opts {
		o(c)
	}
}

// Use 注册客户端过滤器，按注册顺序执行。
func (c *Client) Use(filters ...ClientFilter) {
	c.filters = append(c.filters, filters...)
}

// Invoke 发起一次 tRPC RPC 调用。
// ctx 为上下文，req 为请求消息体，rsp 为响应消息体指针。
func (c *Client) Invoke(ctx context.Context, method string, req, rsp interface{}) error {
	opts := c.clientOptions

	// 追加过滤器
	if len(c.filters) > 0 {
		opts = append(opts, trpcClient.WithFilters(c.filters))
	}

	// 追加方法名
	if method != "" {
		opts = append(opts, trpcClient.WithCalleeMethod(method))
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()

	return c.Client.Invoke(ctx, req, rsp, opts...)
}

func (c *Client) timeout() time.Duration {
	// 默认不设置额外超时，由 tRPC 框架内部或 context 控制
	return 0
}
