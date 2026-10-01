package eureka

import (
	"context"
	"time"
)

type Option func(o *Registry)

// WithContext with registry context.
func WithContext(ctx context.Context) Option {
	return func(o *Registry) { o.ctx = ctx }
}

func WithHeartbeat(interval time.Duration) Option {
	return func(o *Registry) { o.heartbeatInterval = interval }
}

func WithRefresh(interval time.Duration) Option {
	return func(o *Registry) { o.refreshInterval = interval }
}

func WithEurekaPath(path string) Option {
	return func(o *Registry) { o.eurekaPath = path }
}

// MaxRetry with client request retry count. Named without the With prefix to
// avoid clashing with the ClientOption WithMaxRetry in this package.
func MaxRetry(maxRetry int) Option {
	return func(o *Registry) { o.maxRetry = maxRetry }
}
