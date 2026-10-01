package polaris

import (
	"context"
	"testing"

	"github.com/polarismesh/polaris-go/pkg/model"

	wind "github.com/tx7do/go-wind"
)

// watcherFixture builds a Watcher with a controllable event channel,
// bypassing the polaris consumer API entirely.
type watcherFixture struct {
	w  *Watcher
	ch chan model.SubScribeEvent
}

func newWatcherFixture(t *testing.T, initial ...model.Instance) *watcherFixture {
	t.Helper()
	ch := make(chan model.SubScribeEvent, 8)
	w := &Watcher{
		Namespace:        "ns",
		ServiceName:      "svc",
		Channel:          ch,
		ServiceInstances: instancesToServiceInstances(initial),
	}
	w.Ctx, w.Cancel = context.WithCancel(context.Background())
	t.Cleanup(w.Cancel)
	return &watcherFixture{w: w, ch: ch}
}

func (f *watcherFixture) sendEvent(t *testing.T, ev model.SubScribeEvent) {
	t.Helper()
	select {
	case f.ch <- ev:
	default:
		t.Fatal("watcher event channel full")
	}
}

func (f *watcherFixture) next(t *testing.T) []*wind.Instance {
	t.Helper()
	got, err := f.w.Next(context.Background())
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	return got
}

func fixture(id string) model.Instance {
	return &mockInstance{
		id:      id,
		service: "svc",
		host:    "10.0.0." + id,
		port:    8080,
		healthy: true,
		metadata: map[string]string{
			"kind":    "grpc",
			"version": "1.0.0",
		},
	}
}

func ids(instances []*wind.Instance) []string {
	out := make([]string, 0, len(instances))
	for _, i := range instances {
		out = append(out, i.ID)
	}
	return out
}

func TestWatcherNext_AddEventAppends(t *testing.T) {
	f := newWatcherFixture(t, fixture("1"))

	f.sendEvent(t, &model.InstanceEvent{
		AddEvent: &model.InstanceAddEvent{Instances: []model.Instance{fixture("2"), fixture("3")}},
	})

	got := f.next(t)
	want := []string{"1", "2", "3"}
	if len(got) != len(want) {
		t.Fatalf("Next() IDs = %v, want %v", ids(got), want)
	}
	for i, id := range want {
		if got[i].ID != id {
			t.Errorf("Next() ID at %d = %q, want %q", i, got[i].ID, id)
		}
	}
}

func TestWatcherNext_DeleteEventRemoves(t *testing.T) {
	f := newWatcherFixture(t, fixture("1"), fixture("2"), fixture("3"))

	f.sendEvent(t, &model.InstanceEvent{
		DeleteEvent: &model.InstanceDeleteEvent{Instances: []model.Instance{fixture("2")}},
	})

	got := f.next(t)
	if len(got) != 2 || got[0].ID != "1" || got[1].ID != "3" {
		t.Errorf("Next() IDs = %v, want [1 3]", ids(got))
	}
}

func TestWatcherNext_DeleteEventLastInstance(t *testing.T) {
	f := newWatcherFixture(t, fixture("1"))

	// Deleting the only remaining instance must leave an empty (non-nil
	// crash-free) list without index panics.
	f.sendEvent(t, &model.InstanceEvent{
		DeleteEvent: &model.InstanceDeleteEvent{Instances: []model.Instance{fixture("1")}},
	})

	got := f.next(t)
	if len(got) != 0 {
		t.Errorf("Next() = %v, want empty", ids(got))
	}
}

func TestWatcherNext_UpdateEventReplaces(t *testing.T) {
	f := newWatcherFixture(t, fixture("1"), fixture("2"))

	// Update of instance "2": same ID, new host after the update.
	f.sendEvent(t, &model.InstanceEvent{
		UpdateEvent: &model.InstanceUpdateEvent{
			UpdateList: []model.OneInstanceUpdate{
				{
					Before: fixture("2"),
					After: &mockInstance{
						id:       "2",
						service:  "svc",
						host:     "10.9.9.9",
						port:     9999,
						healthy:  true,
						metadata: map[string]string{"kind": "grpc", "version": "1.0.0"},
					},
				},
			},
		},
	})

	got := f.next(t)
	if len(got) != 2 {
		t.Fatalf("Next() returned %d instances, want 2", len(got))
	}
	var updated *wind.Instance
	for _, inst := range got {
		if inst.ID == "2" {
			updated = inst
		}
	}
	if updated == nil {
		t.Fatal("Next() lost instance 2 after update")
	}
	if updated.Endpoints[0] != "grpc://10.9.9.9:9999" {
		t.Errorf("updated endpoint = %q, want grpc://10.9.9.9:9999", updated.Endpoints[0])
	}
}

func TestWatcherNext_StopReturnsError(t *testing.T) {
	f := newWatcherFixture(t)

	if err := f.w.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	if _, err := f.w.Next(context.Background()); err == nil {
		t.Error("Next() after Stop() should return the context error")
	}
}

func TestWatcherNext_BlocksWithoutEvents(t *testing.T) {
	f := newWatcherFixture(t, fixture("1"))

	// Without an event (and with a live context) Next must block rather than
	// return the stale instance list.
	done := make(chan struct{})
	go func() {
		_, _ = f.w.Next(context.Background())
		close(done)
	}()

	select {
	case <-done:
		t.Error("Next() returned without an event or cancellation")
	default:
	}
	// Clean up: cancel to release the goroutine.
	f.w.Cancel()
}
