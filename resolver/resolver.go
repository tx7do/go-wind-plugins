// Package resolver adapts a registry [Discovery] to the gRPC resolver
// extension point, so gRPC clients can dial services by name through any
// registry plugin (etcd, consul, nacos, ...):
//
//	builder := resolver.NewBuilder(discovery) // scheme "wind"
//	conn, err := grpc.NewClient("wind:///user-service",
//	    grpc.WithResolvers(builder),
//	    grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`),
//	)
//
// The builder watches each dialed service and pushes address updates to the
// gRPC channel as instances come and go; load balancing itself is left to
// gRPC's built-in policies (pick_first by default, round_robin as above).
package resolver

import (
	"context"
	"net"
	"net/url"
	"strings"
	"time"

	"google.golang.org/grpc/resolver"

	wind "github.com/tx7do/go-wind"
	baseRegistry "github.com/tx7do/go-wind-plugins/registry"
)

// DefaultScheme is the URL scheme handled by builders created with
// [NewBuilder] without an explicit scheme.
const DefaultScheme = "wind"

// Builder is a [resolver.Builder] backed by a registry Discovery.
type Builder struct {
	discovery baseRegistry.Discovery
	scheme    string
}

// NewBuilder returns a resolver builder over the given discovery. An
// optional scheme overrides [DefaultScheme].
func NewBuilder(discovery baseRegistry.Discovery, scheme ...string) *Builder {
	s := DefaultScheme
	if len(scheme) > 0 && scheme[0] != "" {
		s = scheme[0]
	}
	return &Builder{discovery: discovery, scheme: s}
}

// Scheme implements [resolver.Builder].
func (b *Builder) Scheme() string { return b.scheme }

// Build implements [resolver.Builder]. The target endpoint is the service
// name, e.g. "wind:///user-service" resolves "user-service".
func (b *Builder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	service := target.Endpoint()

	ctx, cancel := context.WithCancel(context.Background())
	r := &discoveryResolver{cancel: cancel}

	go r.watch(ctx, b.discovery, service, cc)
	return r, nil
}

// discoveryResolver drives one watched service for one gRPC channel.
type discoveryResolver struct {
	cancel context.CancelFunc
}

// ResolveNow implements [resolver.Resolver]. Updates are watch-driven, so
// this is a no-op.
func (r *discoveryResolver) ResolveNow(resolver.ResolveNowOptions) {}

// Close implements [resolver.Resolver].
func (r *discoveryResolver) Close() { r.cancel() }

// watch streams instance updates for the service into the ClientConn until
// the context is cancelled. On a watch failure it rebuilds the watcher after
// a short pause; the last pushed state remains active in the meantime.
func (r *discoveryResolver) watch(ctx context.Context, d baseRegistry.Discovery, service string, cc resolver.ClientConn) {
	watcher, err := d.Watch(ctx, service)
	if err != nil {
		cc.ReportError(err)
		return
	}

	for ctx.Err() == nil {
		insts, err := watcher.Next(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			time.Sleep(time.Second)
			if ctx.Err() != nil {
				return
			}
			nw, werr := d.Watch(ctx, service)
			if werr != nil {
				continue
			}
			_ = watcher.Stop()
			watcher = nw
			continue
		}

		state := resolver.State{Addresses: parseEndpoints(insts)}
		if err := cc.UpdateState(state); err != nil {
			return
		}
	}
}

// parseEndpoints converts instance endpoints ("http://10.0.0.1:8080" or bare
// "10.0.0.1:8080") into gRPC addresses. Instances without a parseable
// host:port endpoint are skipped.
func parseEndpoints(insts []*wind.Instance) []resolver.Address {
	var addrs []resolver.Address
	seen := make(map[string]struct{})
	for _, inst := range insts {
		if inst == nil {
			continue
		}
		for _, ep := range inst.Endpoints {
			addr := endpointAddr(ep)
			if addr == "" {
				continue
			}
			if _, ok := seen[addr]; ok {
				continue
			}
			seen[addr] = struct{}{}
			addrs = append(addrs, resolver.Address{Addr: addr})
		}
	}
	return addrs
}

// endpointAddr extracts the host:port portion of one endpoint string.
func endpointAddr(ep string) string {
	ep = strings.TrimSpace(ep)
	if ep == "" {
		return ""
	}
	if strings.Contains(ep, "://") {
		u, err := url.Parse(ep)
		if err != nil || u.Host == "" {
			return ""
		}
		ep = u.Host
	}
	if _, _, err := net.SplitHostPort(ep); err != nil {
		return ""
	}
	return ep
}
