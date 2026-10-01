package selector

import (
	"context"
	"errors"
	"testing"
	"time"

	wind "github.com/tx7do/go-wind"
	baseRegistry "github.com/tx7do/go-wind-plugins/registry"
)

func inst(id string) *wind.Instance { return &wind.Instance{ID: id, Name: "svc"} }

func ids(instances []*wind.Instance) []string {
	out := make([]string, 0, len(instances))
	for _, i := range instances {
		out = append(out, i.ID)
	}
	return out
}

func TestRoundRobinCycles(t *testing.T) {
	snap := []*wind.Instance{inst("a"), inst("b"), inst("c")}
	rr := RoundRobin()
	want := []string{"a", "b", "c", "a", "b"}
	for _, w := range want {
		got, err := Pick(rr, snap)
		if err != nil {
			t.Fatalf("Pick: %v", err)
		}
		if got.ID != w {
			t.Fatalf("RoundRobin picked %q, want %q", got.ID, w)
		}
	}
}

func TestRandomPicksWithinSnapshot(t *testing.T) {
	snap := []*wind.Instance{inst("a"), inst("b"), inst("c")}
	r := Random()
	valid := map[string]bool{"a": true, "b": true, "c": true}
	for range 20 {
		got, err := Pick(r, snap)
		if err != nil {
			t.Fatalf("Pick: %v", err)
		}
		if !valid[got.ID] {
			t.Fatalf("Random picked %q, not in snapshot", got.ID)
		}
	}
}

func TestPickEmptySnapshot(t *testing.T) {
	if _, err := Pick(RoundRobin(), nil); !errors.Is(err, ErrNoInstances) {
		t.Fatalf("Pick(nil) err = %v, want ErrNoInstances", err)
	}
}

// fakeWatcher pushes snapshots from a channel; first Next returns the seed.
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

type fakeDiscovery struct {
	ch      chan []*wind.Instance
	watches int
}

func newFakeDiscovery(seed []*wind.Instance) *fakeDiscovery {
	d := &fakeDiscovery{ch: make(chan []*wind.Instance, 8)}
	d.ch <- seed
	return d
}

func (d *fakeDiscovery) GetService(ctx context.Context, name string) ([]*wind.Instance, error) {
	return nil, errors.New("not implemented")
}

func (d *fakeDiscovery) Watch(ctx context.Context, name string) (baseRegistry.Watcher, error) {
	d.watches++
	return &fakeWatcher{ch: d.ch, stopCh: make(chan struct{})}, nil
}

func TestWatchedBalancerSeedsAndUpdates(t *testing.T) {
	d := newFakeDiscovery([]*wind.Instance{inst("a"), inst("b")})
	b := NewWatchedBalancer(d, First())
	defer func() { _ = b.Close() }()

	got, err := b.Pick(context.Background(), "svc")
	if err != nil {
		t.Fatalf("first Pick: %v", err)
	}
	if got.ID != "a" {
		t.Fatalf("first Pick = %q, want a (seed snapshot)", got.ID)
	}

	// Push an update; Pick must reflect it without another Watch.
	d.ch <- []*wind.Instance{inst("c")}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if got, _ := b.Pick(context.Background(), "svc"); got != nil && got.ID == "c" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("update never became visible")
}

func TestWatchedBalancerPerServiceIsolation(t *testing.T) {
	d := &fakeDiscovery{ch: make(chan []*wind.Instance, 8)}
	b := NewWatchedBalancer(d, RoundRobin())
	defer func() { _ = b.Close() }()

	d.ch <- []*wind.Instance{inst("a")}
	if _, err := b.Pick(context.Background(), "svc1"); err != nil {
		t.Fatalf("Pick svc1: %v", err)
	}
	d.ch <- []*wind.Instance{inst("b")}
	if _, err := b.Pick(context.Background(), "svc2"); err != nil {
		t.Fatalf("Pick svc2: %v", err)
	}
	if d.watches != 2 {
		t.Fatalf("watches = %d, want 2 (one per service)", d.watches)
	}
}

func TestWatchedBalancerClose(t *testing.T) {
	d := newFakeDiscovery([]*wind.Instance{inst("a")})
	b := NewWatchedBalancer(d, First())
	if _, err := b.Pick(context.Background(), "svc"); err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if err := b.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := b.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if _, err := b.Pick(context.Background(), "svc"); !errors.Is(err, ErrNoInstances) {
		t.Fatalf("Pick after Close err = %v, want ErrNoInstances", err)
	}
}
