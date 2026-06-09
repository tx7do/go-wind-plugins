package consul

import (
	"context"

	wind "github.com/tx7do/go-wind"
	baseRegistry "github.com/tx7do/go-wind-plugins/registry"
)

var _ baseRegistry.Watcher = (*watcher)(nil)

type watcher struct {
	event chan struct{}
	set   *serviceSet

	// for cancel
	ctx    context.Context
	cancel context.CancelFunc
}

func (w *watcher) Next(ctx context.Context) (services []*wind.Instance, err error) {
	select {
	case <-w.ctx.Done():
		err = w.ctx.Err()
		return
	case <-w.event:
	}

	ss, ok := w.set.services.Load().([]*wind.Instance)

	if ok {
		services = append(services, ss...)
	}
	return
}

func (w *watcher) Stop() error {
	w.cancel()
	w.set.lock.Lock()
	defer w.set.lock.Unlock()
	delete(w.set.watcher, w)
	return nil
}
