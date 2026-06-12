// circuitbreaker-basic demonstrates how to use the SRE circuit breaker
// with Allow/MarkSuccess/MarkFailure and the convenience Execute method.
package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/tx7do/go-wind-plugins/circuitbreaker"
	"github.com/tx7do/go-wind-plugins/circuitbreaker/sres"
)

func main() {
	// 1. Create an SRE circuit breaker.
	cb := sres.New(
		sres.WithK(1.5),                 // sensitivity factor (lower = more aggressive)
		sres.WithWindow(10*time.Second), // statistical window
		sres.WithBucketCount(10),        // buckets within window
	)
	defer cb.Close()

	// --- Example 1: Execute() convenience method ---
	fmt.Println("=== Example 1: Execute() — auto success/failure marking ===")
	for i := 0; i < 10; i++ {
		err := cb.Execute(context.Background(), func() error {
			// Simulate 30% failure rate.
			if rand.Float64() < 0.3 {
				return fmt.Errorf("upstream error")
			}
			return nil
		})
		state := cb.State()
		if err != nil {
			if err == circuitbreaker.ErrCircuitOpen {
				fmt.Printf("  request %02d: REJECTED (circuit open)\n", i)
			} else {
				fmt.Printf("  request %02d: failed: %v  [state=%s]\n", i, err, state)
			}
		} else {
			fmt.Printf("  request %02d: success  [state=%s]\n", i, state)
		}
	}

	// --- Example 2: Manual Allow/MarkSuccess/MarkFailure ---
	fmt.Println("\n=== Example 2: Manual Allow() + MarkSuccess/MarkFailure ===")
	for i := 0; i < 5; i++ {
		if err := cb.Allow(); err != nil {
			fmt.Printf("  request %02d: rejected: %v\n", i, err)
			continue
		}
		// Simulate work.
		if rand.Float64() < 0.5 {
			cb.MarkSuccess()
			fmt.Printf("  request %02d: marked success\n", i)
		} else {
			cb.MarkFailure()
			fmt.Printf("  request %02d: marked failure\n", i)
		}
	}

	// --- Example 3: Force the circuit open by flooding failures ---
	fmt.Println("\n=== Example 3: Force circuit open ===")
	cb2 := sres.New(
		sres.WithK(0.5), // very aggressive
		sres.WithWindow(5*time.Second),
		sres.WithBucketCount(5),
	)
	defer cb2.Close()

	// Send many failures to trip the circuit.
	for i := 0; i < 20; i++ {
		err := cb2.Execute(context.Background(), func() error {
			return fmt.Errorf("failure")
		})
		state := cb2.State()
		if i%5 == 0 || err == circuitbreaker.ErrCircuitOpen {
			fmt.Printf("  request %02d: err=%v  [state=%s]\n", i, err, state)
		}
		if err == circuitbreaker.ErrCircuitOpen {
			break
		}
	}
	fmt.Printf("  final state: %s\n", cb2.State())

	log.Println("all examples completed")
}
