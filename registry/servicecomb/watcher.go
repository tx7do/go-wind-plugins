package servicecomb

import (
	"context"

	"github.com/go-chassis/sc-client"

	wind "github.com/tx7do/go-wind"
	baseRegistry "github.com/tx7do/go-wind-plugins/registry"
)

var _ baseRegistry.Watcher = (*Watcher)(nil)

type Watcher struct {
	cli RegistryClient
	ch  chan *wind.Instance

	// ctx is canceled by Stop; it makes Put a no-op after Stop (so the SDK
	// callback can never send on a channel nobody reads) and lets Next
	// respect both its own ctx and the watcher's lifecycle.
	ctx    context.Context
	cancel context.CancelFunc
}

func newWatcher(ctx context.Context, cli RegistryClient, serviceName string) (*Watcher, error) {
	// 构建当前服务与目标服务之间的依赖关系，完成discovery
	_, err := cli.FindMicroServiceInstances(curServiceID, appID, serviceName, "")
	if err != nil {
		return nil, err
	}
	wctx, wcancel := context.WithCancel(ctx)
	w := &Watcher{
		cli:    cli,
		ch:     make(chan *wind.Instance),
		ctx:    wctx,
		cancel: wcancel,
	}
	go func() {
		watchErr := w.cli.WatchMicroService(curServiceID, func(event *sc.MicroServiceInstanceChangedEvent) {
			if event.Key.ServiceName != serviceName {
				return
			}
			svcIns := &wind.Instance{
				ID:        event.Instance.InstanceId,
				Name:      event.Key.ServiceName,
				Version:   event.Key.Version,
				Metadata:  event.Instance.Properties,
				Endpoints: event.Instance.Endpoints,
			}
			w.Put(svcIns)
		})
		if watchErr != nil {
			return
		}
	}()
	return w, nil
}

// Put delivers an event to Next. It is a no-op once the watcher is stopped,
// so a late SDK callback can no longer panic on a closed channel.
func (w *Watcher) Put(svcIns *wind.Instance) {
	select {
	case <-w.ctx.Done():
		return
	default:
	}
	select {
	case w.ch <- svcIns:
	case <-w.ctx.Done():
	}
}

func (w *Watcher) Next(ctx context.Context) ([]*wind.Instance, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-w.ctx.Done():
		return nil, w.ctx.Err()
	case svcIns := <-w.ch:
		return []*wind.Instance{svcIns}, nil
	}
}

func (w *Watcher) Stop() error {
	w.cancel()
	return nil
}
