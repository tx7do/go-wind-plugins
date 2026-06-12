// ratelimit-basic demonstrates how to use the token-bucket rate limiter
// with both non-blocking (Allow) and blocking (Wait) modes.
package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tx7do/go-wind-plugins/ratelimit"
	"github.com/tx7do/go-wind-plugins/ratelimit/tokenbucket"
)

func main() {
	// 1. Create a token-bucket limiter: 10 tokens/second, burst of 5.
	limiter, err := tokenbucket.New(10, 5) // rate=10/s, burst=5
	if err != nil {
		log.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	// --- Example 1: Non-blocking Allow() ---
	fmt.Println("=== Example 1: Non-blocking Allow() ===")
	var allowed, rejected atomic.Int32
	var wg sync.WaitGroup

	// Send 20 requests concurrently.
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ok, err := limiter.Allow()
			if err != nil {
				fmt.Printf("  request %02d: error: %v\n", id, err)
				return
			}
			if ok {
				allowed.Add(1)
				fmt.Printf("  request %02d: allowed\n", id)
			} else {
				rejected.Add(1)
				fmt.Printf("  request %02d: rejected (rate limited)\n", id)
			}
		}(i)
	}
	wg.Wait()
	fmt.Printf("  summary: %d allowed, %d rejected\n\n", allowed.Load(), rejected.Load())

	// --- Example 2: Blocking Wait() ---
	fmt.Println("=== Example 2: Blocking Wait() ===")
	limiter2, err := tokenbucket.New(100, 3) // rate=100/s, burst=3
	if err != nil {
		log.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter2.Close()

	// Drain the burst first.
	for i := 0; i < 3; i++ {
		_, _ = limiter2.Allow()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	for i := 0; i < 5; i++ {
		start := time.Now()
		if err := limiter2.Wait(ctx); err != nil {
			fmt.Printf("  request %d: wait failed: %v\n", i, err)
			break
		}
		fmt.Printf("  request %d: waited %s\n", i, time.Since(start).Round(time.Millisecond))
	}

	// --- Example 3: Demonstrate ErrLimited ---
	fmt.Println("\n=== Example 3: Rate limit error ===")
	fmt.Printf("  ErrLimited = %v\n", ratelimit.ErrLimited)

	log.Println("all examples completed")
}
