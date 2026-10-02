package servicecomb

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"sync"
	"time"

	"github.com/go-chassis/cari/discovery"
	"github.com/go-chassis/cari/pkg/errsvc"
	"github.com/go-chassis/sc-client"
	"github.com/gofrs/uuid"

	wind "github.com/tx7do/go-wind"
	baseRegistry "github.com/tx7do/go-wind-plugins/registry"
)

func init() {
	appID = os.Getenv(appIDVar)
	if appID == "" {
		appID = "default"
	}
	env = os.Getenv(envVar)
}

var (
	_ baseRegistry.Registrar = (*Registry)(nil)
	_ baseRegistry.Discovery = (*Registry)(nil)
)

var (
	curServiceID string
	appID        string
	env          string
)

const (
	appIDKey         = "appId"
	envKey           = "environment"
	envVar           = "CAS_ENVIRONMENT_ID"
	appIDVar         = "CAS_APPLICATION_NAME"
	frameWorkName    = "wind"
	frameWorkVersion = "v2"
)

type RegistryClient interface {
	GetMicroServiceID(appID, microServiceName, version, env string, opts ...sc.CallOption) (string, error)
	FindMicroServiceInstances(consumerID, appID, microServiceName, versionRule string, opts ...sc.CallOption) ([]*discovery.MicroServiceInstance, error)
	RegisterService(microService *discovery.MicroService) (string, error)
	RegisterMicroServiceInstance(microServiceInstance *discovery.MicroServiceInstance) (string, error)
	Heartbeat(microServiceID, microServiceInstanceID string) (bool, error)
	UnregisterMicroServiceInstance(microServiceID, microServiceInstanceID string) (bool, error)
	WatchMicroService(microServiceID string, callback func(*sc.MicroServiceInstanceChangedEvent)) error
}

// Registry is servicecomb registry.
type Registry struct {
	cli RegistryClient

	mu    sync.Mutex
	stops map[string]chan struct{} // instance ID → heartbeat stop signal
}

func New(client RegistryClient) *Registry {
	r := &Registry{
		cli:   client,
		stops: make(map[string]chan struct{}),
	}
	return r
}

func (r *Registry) GetService(_ context.Context, serviceName string) ([]*wind.Instance, error) {
	instances, err := r.cli.FindMicroServiceInstances("", appID, serviceName, "")
	if err != nil {
		return nil, err
	}
	svcInstances := make([]*wind.Instance, 0, len(instances))
	for _, instance := range instances {
		svcInstances = append(svcInstances, &wind.Instance{
			ID:        instance.InstanceId,
			Name:      serviceName,
			Metadata:  instance.Properties,
			Endpoints: instance.Endpoints,
			Version:   instance.ServiceId,
		})
	}
	return svcInstances, nil
}

func (r *Registry) Watch(ctx context.Context, serviceName string) (baseRegistry.Watcher, error) {
	return newWatcher(ctx, r.cli, serviceName)
}

func (r *Registry) Register(_ context.Context, svcIns *wind.Instance) error {
	fw := &discovery.FrameWork{
		Name:    frameWorkName,
		Version: frameWorkVersion,
	}
	ms := &discovery.MicroService{
		ServiceName: svcIns.Name,
		AppId:       appID,
		Version:     svcIns.Version,
		Environment: env,
		Framework:   fw,
	}
	// 先尝试创建微服务
	sid, err := r.cli.RegisterService(ms)
	// 若失败，说明服务可能已注册
	if err != nil {
		registryException, ok := err.(*sc.RegistryException)
		if !ok {
			return err
		}
		var svcErr errsvc.Error
		parseErr := json.Unmarshal([]byte(registryException.Message), &svcErr)
		if parseErr != nil {
			return parseErr
		}
		// 若错误码显示服务未注册，直接返回
		if svcErr.Code != discovery.ErrServiceAlreadyExists {
			return err
		}
		sid, err = r.cli.GetMicroServiceID(appID, ms.ServiceName, ms.Version, ms.Environment)
		if err != nil {
			return err
		}
	} else {
		// 保存当前版本微服务对应的sid
		curServiceID = sid
	}
	if svcIns.ID == "" {
		var id uuid.UUID
		id, err = uuid.NewV4()
		if err != nil {
			return err
		}
		svcIns.ID = id.String()
	}
	props := map[string]string{
		appIDKey: appID,
		envKey:   env,
	}
	_, err = r.cli.RegisterMicroServiceInstance(&discovery.MicroServiceInstance{
		InstanceId: svcIns.ID,
		ServiceId:  sid,
		Endpoints:  svcIns.Endpoints,
		HostName:   svcIns.ID,
		Properties: props,
		Version:    svcIns.Version,
	})
	if err != nil {
		return err
	}
	r.startHeartbeat(sid, svcIns.ID)
	return nil
}

// startHeartbeat launches the periodic 30s heartbeat loop for one instance.
// The loop exits when Deregister closes the instance's stop signal; without
// it, every Register call leaked an unstoppable goroutine.
func (r *Registry) startHeartbeat(sid, instanceID string) {
	stop := make(chan struct{})
	r.mu.Lock()
	if old, ok := r.stops[instanceID]; ok {
		close(old)
	}
	r.stops[instanceID] = stop
	r.mu.Unlock()

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
			}
			if _, err := r.cli.Heartbeat(sid, instanceID); err != nil {
				log.Printf("failed to send heartbeat: %v", err)
			}
		}
	}()
}

func (r *Registry) Deregister(_ context.Context, svcIns *wind.Instance) error {
	// 先停掉该实例的心跳循环，再注销实例。
	r.mu.Lock()
	if stop, ok := r.stops[svcIns.ID]; ok {
		close(stop)
		delete(r.stops, svcIns.ID)
	}
	r.mu.Unlock()

	sid, err := r.cli.GetMicroServiceID(appID, svcIns.Name, svcIns.Version, env)
	if err != nil {
		return err
	}
	_, err = r.cli.UnregisterMicroServiceInstance(sid, svcIns.ID)
	if err != nil {
		return err
	}
	return nil
}
