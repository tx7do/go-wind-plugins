// Package selector provides client-side load-balancing strategies over
// service instance snapshots, plus a [WatchedBalancer] that keeps a live
// view of each watched service backed by a registry [Discovery].
//
// The package depends only on the [wind.Instance] type and the registry
// contracts — it knows nothing about transports, so it works for gRPC,
// HTTP, or hand-rolled clients alike:
//
//	balancer := selector.NewWatchedBalancer(discovery, selector.RoundRobin())
//	defer balancer.Close()
//	inst, err := balancer.Pick(ctx, "user-service")
package selector

import (
	"errors"
	"math/rand"
	"sync/atomic"

	wind "github.com/tx7do/go-wind"
)

// ErrNoInstances is returned when a service has no instances to pick from.
var ErrNoInstances = errors.New("selector: no instances available")

// Strategy picks one instance from a snapshot of a service's instances.
// The snapshot is never nil but may be empty; a Strategy must return nil
// for an empty snapshot.
type Strategy func(instances []*wind.Instance) *wind.Instance

// RoundRobin returns a Strategy that cycles through the snapshot in order.
// Each Strategy instance carries its own counter, so share one Strategy
// across picks to get even distribution.
func RoundRobin() Strategy {
	var counter atomic.Uint64
	return func(instances []*wind.Instance) *wind.Instance {
		if len(instances) == 0 {
			return nil
		}
		return instances[(counter.Add(1)-1)%uint64(len(instances))]
	}
}

// Random returns a Strategy that picks uniformly at random.
func Random() Strategy {
	return func(instances []*wind.Instance) *wind.Instance {
		if len(instances) == 0 {
			return nil
		}
		return instances[rand.Intn(len(instances))]
	}
}

// First returns a Strategy that always picks the first instance. Useful for
// tests and for setups where the registry already performs load balancing.
func First() Strategy {
	return func(instances []*wind.Instance) *wind.Instance {
		if len(instances) == 0 {
			return nil
		}
		return instances[0]
	}
}

// Pick applies the strategy to the snapshot.
func Pick(s Strategy, instances []*wind.Instance) (*wind.Instance, error) {
	if s == nil {
		s = RoundRobin()
	}
	inst := s(instances)
	if inst == nil {
		return nil, ErrNoInstances
	}
	return inst, nil
}
