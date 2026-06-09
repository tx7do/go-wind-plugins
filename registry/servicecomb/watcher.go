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
}

func newWatcher(_ context.Context, cli RegistryClient, serviceName string) (*Watcher, error) {
	// 构建当前服务与目标服务之间的依赖关系，完成discovery
	_, err := cli.FindMicroServiceInstances(curServiceID, appID, serviceName, "")
	if err != nil {
		return nil, err
	}
	w := &Watcher{
		cli: cli,
		ch:  make(chan *wind.Instance),
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

// Put only for UT
func (w *Watcher) Put(svcIns *wind.Instance) {
	w.ch <- svcIns
}

func (w *Watcher) Next(ctx context.Context) ([]*wind.Instance, error) {
	var svcInstances []*wind.Instance
	svcIns := <-w.ch
	svcInstances = append(svcInstances, svcIns)
	return svcInstances, nil
}

func (w *Watcher) Stop() error {
	close(w.ch)
	return nil
}
