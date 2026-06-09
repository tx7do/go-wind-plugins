package eureka

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	wind "github.com/tx7do/go-wind"
	baseRegistry "github.com/tx7do/go-wind-plugins/registry"
)

var (
	_ baseRegistry.Registrar = (*Registry)(nil)
	_ baseRegistry.Discovery = (*Registry)(nil)
)

type Registry struct {
	ctx               context.Context
	api               *API
	heartbeatInterval time.Duration
	refreshInterval   time.Duration
	eurekaPath        string
}

func New(eurekaUrls []string, opts ...Option) (*Registry, error) {
	r := &Registry{
		ctx:               context.Background(),
		heartbeatInterval: heartbeatTime,
		refreshInterval:   refreshTime,
		eurekaPath:        "eureka/v2",
	}

	for _, o := range opts {
		o(r)
	}

	client := NewClient(eurekaUrls, WithHeartbeatInterval(r.heartbeatInterval), WithClientContext(r.ctx), WithNamespace(r.eurekaPath))
	r.api = NewAPI(r.ctx, client, r.refreshInterval)
	return r, nil
}

// Register 这里的Context是每个注册器独享的
func (r *Registry) Register(ctx context.Context, service *wind.Instance) error {
	return r.api.Register(ctx, service.Name, r.Endpoints(service)...)
}

// Deregister registry service to zookeeper.
func (r *Registry) Deregister(ctx context.Context, service *wind.Instance) error {
	return r.api.Deregister(ctx, r.Endpoints(service))
}

// GetService get services from zookeeper
func (r *Registry) GetService(ctx context.Context, serviceName string) ([]*wind.Instance, error) {
	instances := r.api.GetService(ctx, serviceName)
	items := make([]*wind.Instance, 0, len(instances))
	for _, instance := range instances {
		items = append(items, &wind.Instance{
			ID:        instance.Metadata["ID"],
			Name:      instance.Metadata["Name"],
			Version:   instance.Metadata["Version"],
			Endpoints: []string{instance.Metadata["Endpoints"]},
			Metadata:  instance.Metadata,
		})
	}

	return items, nil
}

// Watch 是独立的ctx
func (r *Registry) Watch(ctx context.Context, serviceName string) (baseRegistry.Watcher, error) {
	return newWatch(ctx, r.api, serviceName)
}

func (r *Registry) Endpoints(service *wind.Instance) []Endpoint {
	res := make([]Endpoint, 0, len(service.Endpoints))
	for _, ep := range service.Endpoints {
		start := strings.Index(ep, "//")
		end := strings.LastIndex(ep, ":")
		appID := strings.ToUpper(service.Name)
		ip := ep[start+2 : end]
		sport := ep[end+1:]
		port, _ := strconv.Atoi(sport)
		securePort := 443
		homePageURL := fmt.Sprintf("%s/", ep)
		statusPageURL := fmt.Sprintf("%s/info", ep)
		healthCheckURL := fmt.Sprintf("%s/health", ep)
		instanceID := strings.Join([]string{ip, appID, sport}, ":")
		metadata := make(map[string]string)
		if len(service.Metadata) > 0 {
			metadata = service.Metadata
		}
		if s, ok := service.Metadata["securePort"]; ok {
			securePort, _ = strconv.Atoi(s)
		}
		if s, ok := service.Metadata["homePageURL"]; ok {
			homePageURL = s
		}
		if s, ok := service.Metadata["statusPageURL"]; ok {
			statusPageURL = s
		}
		if s, ok := service.Metadata["healthCheckURL"]; ok {
			healthCheckURL = s
		}
		metadata["ID"] = service.ID
		metadata["Name"] = service.Name
		metadata["Version"] = service.Version
		metadata["Endpoints"] = ep
		metadata["agent"] = "go-eureka-client"
		res = append(res, Endpoint{
			AppID:          appID,
			IP:             ip,
			Port:           port,
			SecurePort:     securePort,
			HomePageURL:    homePageURL,
			StatusPageURL:  statusPageURL,
			HealthCheckURL: healthCheckURL,
			InstanceID:     instanceID,
			MetaData:       metadata,
		})
	}

	return res
}
