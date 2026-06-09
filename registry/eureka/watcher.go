package eureka

import (
	"context"

	wind "github.com/tx7do/go-wind"
	baseRegistry "github.com/tx7do/go-wind-plugins/registry"
)

var _ baseRegistry.Watcher = (*watcher)(nil)

type watcher struct {
	ctx        context.Context
	cancel     context.CancelFunc
	cli        *API
	watchChan  chan struct{}
	serverName string
}

func newWatch(ctx context.Context, cli *API, serverName string) (*watcher, error) {
	w := &watcher{
		ctx:        ctx,
		cli:        cli,
		serverName: serverName,
		watchChan:  make(chan struct{}, 1),
	}
	w.ctx, w.cancel = context.WithCancel(ctx)
	e := w.cli.Subscribe(
		serverName,
		func() {
			w.watchChan <- struct{}{}
		},
	)
	return w, e
}

func (w *watcher) Next(ctx context.Context) (services []*wind.Instance, err error) {
	select {
	case <-w.ctx.Done():
		return nil, w.ctx.Err()
	case <-w.watchChan:
		instances := w.cli.GetService(w.ctx, w.serverName)
		services = make([]*wind.Instance, 0, len(instances))
		for _, instance := range instances {
			services = append(services, &wind.Instance{
				ID:        instance.Metadata["ID"],
				Name:      instance.Metadata["Name"],
				Version:   instance.Metadata["Version"],
				Endpoints: []string{instance.Metadata["Endpoints"]},
				Metadata:  instance.Metadata,
			})
		}
		return
	}
}

func (w *watcher) Stop() error {
	w.cancel()
	w.cli.Unsubscribe(w.serverName)
	return nil
}
