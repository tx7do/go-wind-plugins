package resolver

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/serviceconfig"

	wind "github.com/tx7do/go-wind"
	baseRegistry "github.com/tx7do/go-wind-plugins/registry"
)

// fakeWatcher is a Watcher driven by a channel of snapshots.
type fakeWatcher struct {
	ch     chan []*wind.Instance
	stopCh chan struct{}
}

func (w *fakeWatcher) Next(ctx context.Context) ([]*wind.Instance, error) {
	select {
	case insts := <-w.ch:
		return insts, nil
	case <-w.stopCh:
		return nil, errors.New("watcher stopped")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (w *fakeWatcher) Stop() error {
	select {
	case <-w.stopCh:
	default:
		close(w.stopCh)
	}
	return nil
}

// fakeDiscovery hands out watchers backed by a shared broadcast channel.
type fakeDiscovery struct {
	ch chan []*wind.Instance
}

func newFakeDiscovery() *fakeDiscovery {
	return &fakeDiscovery{ch: make(chan []*wind.Instance, 16)}
}

func (d *fakeDiscovery) GetService(ctx context.Context, name string) ([]*wind.Instance, error) {
	return nil, errors.New("not implemented")
}

func (d *fakeDiscovery) Watch(ctx context.Context, name string) (baseRegistry.Watcher, error) {
	return &fakeWatcher{ch: d.ch, stopCh: make(chan struct{})}, nil
}

// fakeClientConn captures UpdateState calls.
type fakeClientConn struct {
	states   chan resolver.State
	updated  chan struct{}
	reported chan error
}

func newFakeClientConn() *fakeClientConn {
	return &fakeClientConn{
		states:   make(chan resolver.State, 8),
		updated:  make(chan struct{}, 8),
		reported: make(chan error, 8),
	}
}

func (cc *fakeClientConn) UpdateState(state resolver.State) error {
	cc.states <- state
	cc.updated <- struct{}{}
	return nil
}

func (cc *fakeClientConn) ReportError(err error) { cc.reported <- err }

func (cc *fakeClientConn) ParseServiceConfig(json string) *serviceconfig.ParseResult {
	return nil
}

func (cc *fakeClientConn) NewAddress([]resolver.Address) {}

func inst(id, ep string) *wind.Instance {
	return &wind.Instance{ID: id, Name: "svc", Endpoints: []string{ep}}
}

func TestBuilderScheme(t *testing.T) {
	b := NewBuilder(newFakeDiscovery())
	if b.Scheme() != DefaultScheme {
		t.Fatalf("Scheme = %q, want %q", b.Scheme(), DefaultScheme)
	}
	if s := NewBuilder(newFakeDiscovery(), "discovery").Scheme(); s != "discovery" {
		t.Fatalf("custom scheme = %q, want discovery", s)
	}
}

func TestBuildAndPushStates(t *testing.T) {
	d := newFakeDiscovery()
	cc := newFakeClientConn()
	b := NewBuilder(d)

	targetURL, err := url.Parse("wind:///user-service")
	if err != nil {
		t.Fatalf("parse target url: %v", err)
	}
	target := resolver.Target{URL: *targetURL}
	r, err := b.Build(target, cc, resolver.BuildOptions{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer r.Close()

	// Initial snapshot.
	d.ch <- []*wind.Instance{inst("1", "http://10.0.0.1:8080"), inst("2", "10.0.0.2:8080")}
	select {
	case state := <-cc.states:
		if len(state.Addresses) != 2 {
			t.Fatalf("initial addresses = %+v, want 2", state.Addresses)
		}
		if state.Addresses[0].Addr != "10.0.0.1:8080" || state.Addresses[1].Addr != "10.0.0.2:8080" {
			t.Fatalf("addresses = %+v", state.Addresses)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for initial state")
	}

	// Update: scheme-stripped and duplicate addresses collapse.
	d.ch <- []*wind.Instance{
		inst("1", "https://10.0.0.1:8443"),
		inst("3", "https://10.0.0.1:8443"),
		inst("4", "no-port"), // dropped
	}
	select {
	case state := <-cc.states:
		if len(state.Addresses) != 1 || state.Addresses[0].Addr != "10.0.0.1:8443" {
			t.Fatalf("updated addresses = %+v, want [10.0.0.1:8443]", state.Addresses)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for updated state")
	}
}

func TestParseEndpointsEdgeCases(t *testing.T) {
	got := parseEndpoints([]*wind.Instance{
		nil,
		{ID: "1"},
		{ID: "2", Endpoints: []string{"", "  "}},
		{ID: "3", Endpoints: []string{"host:9090"}},
	})
	if len(got) != 1 || got[0].Addr != "host:9090" {
		t.Fatalf("parseEndpoints = %+v, want [host:9090]", got)
	}
}
