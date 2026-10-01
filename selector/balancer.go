package selector

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	wind "github.com/tx7do/go-wind"
	baseRegistry "github.com/tx7do/go-wind-plugins/registry"
)

// Balancer picks live instances of a named service.
type Balancer interface {
	// Pick returns one instance of the named service.
	Pick(ctx context.Context, service string) (*wind.Instance, error)
	// Close releases all watchers held by the balancer.
	Close() error
}

// WatchedBalancer maintains a live view of every watched service using a
// registry [Discovery]'s watch stream, so Pick is a pure in-memory operation
// with no network round-trip.
type WatchedBalancer struct {
	discovery baseRegistry.Discovery
	strategy  Strategy

	mu       sync.Mutex
	services map[string]*watchedService
	closed   bool
}

type watchedService struct {
	cancel   context.CancelFunc
	watcher  baseRegistry.Watcher
	snapshot atomic.Pointer[[]*wind.Instance]
}

// NewWatchedBalancer creates a balancer over the given discovery. The
// strategy defaults to [RoundRobin] when nil.
//
// The balancer lazily starts watching a service on the first Pick call and
// keeps watching until Close.
func NewWatchedBalancer(discovery baseRegistry.Discovery, strategy Strategy) *WatchedBalancer {
	if strategy == nil {
		strategy = RoundRobin()
	}
	return &WatchedBalancer{
		discovery: discovery,
		strategy:  strategy,
		services:  map[string]*watchedService{},
	}
}

// Pick returns one instance of the named service from the live view. The
// first Pick on a service blocks until the initial snapshot is fetched.
func (b *WatchedBalancer) Pick(ctx context.Context, service string) (*wind.Instance, error) {
	ws, err := b.service(ctx, service)
	if err != nil {
		return nil, err
	}
	if insts := ws.snapshot.Load(); insts != nil {
		return Pick(b.strategy, *insts)
	}
	return nil, ErrNoInstances
}

// Close stops all watchers. Safe to call multiple times.
func (b *WatchedBalancer) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	for _, ws := range b.services {
		ws.cancel()
		_ = ws.watcher.Stop()
	}
	b.services = map[string]*watchedService{}
	return nil
}

// service returns the watched service handle, creating and seeding it on
// first use.
func (b *WatchedBalancer) service(ctx context.Context, service string) (*watchedService, error) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil, ErrNoInstances
	}
	if ws, ok := b.services[service]; ok {
		b.mu.Unlock()
		return ws, nil
	}

	hctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	watcher, err := b.discovery.Watch(hctx, service)
	if err != nil {
		cancel()
		b.mu.Unlock()
		return nil, err
	}
	ws := &watchedService{cancel: cancel, watcher: watcher}
	b.services[service] = ws
	b.mu.Unlock()

	// Seed synchronously: the watcher contract delivers the current
	// snapshot on the first Next call, so the first Pick sees live data.
	if insts, err := watcher.Next(hctx); err == nil {
		ws.snapshot.Store(&insts)
	}
	go b.consume(hctx, service, ws)
	return ws, nil
}

// consume keeps applying watcher updates to the snapshot until the context
// is cancelled. On a watch failure it rebuilds the watcher after a short
// pause and keeps the last snapshot available for picks in the meantime.
func (b *WatchedBalancer) consume(ctx context.Context, service string, ws *watchedService) {
	for ctx.Err() == nil {
		insts, err := ws.watcher.Next(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			time.Sleep(time.Second)
			if ctx.Err() != nil {
				return
			}
			nw, werr := b.discovery.Watch(ctx, service)
			if werr != nil {
				continue
			}
			_ = ws.watcher.Stop()
			ws.watcher = nw
			continue
		}
		ws.snapshot.Store(&insts)
	}
}
