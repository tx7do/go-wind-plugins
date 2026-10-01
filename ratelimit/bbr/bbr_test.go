package bbr

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tx7do/go-wind-plugins/ratelimit"
)

// ---------------------------------------------------------------------------
// New / options
// ---------------------------------------------------------------------------

func TestNew_Defaults(t *testing.T) {
	l := New()
	defer l.Close()

	if l.cfg.cpuThreshold != defaultCPUThreshold {
		t.Errorf("cpuThreshold = %v, want %v", l.cfg.cpuThreshold, defaultCPUThreshold)
	}
	if l.cfg.window != defaultWindow {
		t.Errorf("window = %v, want %v", l.cfg.window, defaultWindow)
	}
	if l.cfg.bucketCount != defaultBucketCount {
		t.Errorf("bucketCount = %v, want %v", l.cfg.bucketCount, defaultBucketCount)
	}
	if l.cfg.minQPS != defaultMinQPS {
		t.Errorf("minQPS = %v, want %v", l.cfg.minQPS, defaultMinQPS)
	}
	if len(l.buckets) != defaultBucketCount {
		t.Errorf("len(buckets) = %d, want %d", len(l.buckets), defaultBucketCount)
	}
	if l.bucketDuration != defaultWindow/defaultBucketCount {
		t.Errorf("bucketDuration = %v, want %v", l.bucketDuration, defaultWindow/defaultBucketCount)
	}
}

func TestOptions(t *testing.T) {
	tests := []struct {
		name  string
		opt   Option
		check func(*testing.T, *Limiter)
	}{
		{
			name: "WithCPUThreshold valid",
			opt:  WithCPUThreshold(0.6),
			check: func(t *testing.T, l *Limiter) {
				if l.cfg.cpuThreshold != 0.6 {
					t.Errorf("cpuThreshold = %v, want 0.6", l.cfg.cpuThreshold)
				}
			},
		},
		{
			name: "WithCPUThreshold zero ignored",
			opt:  WithCPUThreshold(0),
			check: func(t *testing.T, l *Limiter) {
				if l.cfg.cpuThreshold != defaultCPUThreshold {
					t.Errorf("cpuThreshold = %v, want default %v", l.cfg.cpuThreshold, defaultCPUThreshold)
				}
			},
		},
		{
			name: "WithCPUThreshold >= 1 ignored",
			opt:  WithCPUThreshold(1.0),
			check: func(t *testing.T, l *Limiter) {
				if l.cfg.cpuThreshold != defaultCPUThreshold {
					t.Errorf("cpuThreshold = %v, want default %v", l.cfg.cpuThreshold, defaultCPUThreshold)
				}
			},
		},
		{
			name: "WithWindow valid",
			opt:  WithWindow(5 * time.Second),
			check: func(t *testing.T, l *Limiter) {
				if l.cfg.window != 5*time.Second {
					t.Errorf("window = %v, want 5s", l.cfg.window)
				}
			},
		},
		{
			name: "WithWindow zero ignored",
			opt:  WithWindow(0),
			check: func(t *testing.T, l *Limiter) {
				if l.cfg.window != defaultWindow {
					t.Errorf("window = %v, want default %v", l.cfg.window, defaultWindow)
				}
			},
		},
		{
			name: "WithBucketCount valid",
			opt:  WithBucketCount(10),
			check: func(t *testing.T, l *Limiter) {
				if l.cfg.bucketCount != 10 {
					t.Errorf("bucketCount = %d, want 10", l.cfg.bucketCount)
				}
				if len(l.buckets) != 10 {
					t.Errorf("len(buckets) = %d, want 10", len(l.buckets))
				}
			},
		},
		{
			name: "WithBucketCount negative ignored",
			opt:  WithBucketCount(-1),
			check: func(t *testing.T, l *Limiter) {
				if l.cfg.bucketCount != defaultBucketCount {
					t.Errorf("bucketCount = %d, want default %d", l.cfg.bucketCount, defaultBucketCount)
				}
			},
		},
		{
			name: "WithMinQPS valid",
			opt:  WithMinQPS(50),
			check: func(t *testing.T, l *Limiter) {
				if l.cfg.minQPS != 50 {
					t.Errorf("minQPS = %v, want 50", l.cfg.minQPS)
				}
			},
		},
		{
			name: "WithMinQPS zero ignored",
			opt:  WithMinQPS(0),
			check: func(t *testing.T, l *Limiter) {
				if l.cfg.minQPS != defaultMinQPS {
					t.Errorf("minQPS = %v, want default %v", l.cfg.minQPS, defaultMinQPS)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := New(tt.opt)
			defer l.Close()
			tt.check(t, l)
		})
	}
}

// ---------------------------------------------------------------------------
// Allow
// ---------------------------------------------------------------------------

func TestAllow_FirstRequestAdmitted(t *testing.T) {
	l := New()
	defer l.Close()

	ok, err := l.Allow()
	if !ok || err != nil {
		t.Errorf("Allow() = (%v, %v), want (true, nil)", ok, err)
	}
}

func TestAllow_RejectsAtInflightCapacity(t *testing.T) {
	l := New() // empty window: maxQPS = minQPS (1.0) → maxInflight = max(int(1.0*0.8), 1) = 1
	defer l.Close()

	ok, err := l.Allow()
	if !ok || err != nil {
		t.Fatalf("first Allow() = (%v, %v), want (true, nil)", ok, err)
	}

	ok, err = l.Allow()
	if ok {
		t.Error("second Allow() without Done should be rejected at capacity")
	}
	if !errors.Is(err, ratelimit.ErrLimited) {
		t.Errorf("second Allow() err = %v, want ErrLimited", err)
	}
}

func TestAllow_AdmittedAgainAfterDone(t *testing.T) {
	l := New()
	defer l.Close()

	ok, _ := l.Allow()
	if !ok {
		t.Fatal("first Allow() should be admitted")
	}

	l.Done(5 * time.Millisecond)

	ok, err := l.Allow()
	if !ok || err != nil {
		t.Errorf("Allow() after Done = (%v, %v), want (true, nil)", ok, err)
	}
}

func TestAllow_RejectsAfterClose(t *testing.T) {
	l := New()
	_ = l.Close()

	ok, err := l.Allow()
	if ok {
		t.Error("Allow() after Close should return false")
	}
	if !errors.Is(err, ratelimit.ErrLimited) {
		t.Errorf("Allow() err after Close = %v, want ErrLimited", err)
	}
}

func TestAllow_MinQPSRaisesCapacity(t *testing.T) {
	// With an empty window maxQPS = minQPS, so maxInflight = minQPS * cpuThreshold.
	l := New(WithMinQPS(100), WithCPUThreshold(0.5))
	defer l.Close()

	for i := 0; i < 50; i++ {
		ok, err := l.Allow()
		if !ok || err != nil {
			t.Fatalf("Allow() #%d = (%v, %v), want (true, nil)", i, ok, err)
		}
	}

	if got := l.MaxInflight(); got != 50 {
		t.Errorf("MaxInflight() = %d, want 50 (minQPS 100 * cpuThreshold 0.5)", got)
	}
}

// ---------------------------------------------------------------------------
// MaxInflight
// ---------------------------------------------------------------------------

func TestMaxInflight_SetAfterAllow(t *testing.T) {
	l := New()
	defer l.Close()

	if got := l.MaxInflight(); got != 0 {
		t.Errorf("MaxInflight() before first Allow = %d, want 0", got)
	}

	if ok, _ := l.Allow(); !ok {
		t.Fatal("Allow() should be admitted")
	}

	if got := l.MaxInflight(); got < 1 {
		t.Errorf("MaxInflight() after Allow = %d, want >= 1", got)
	}
}

// ---------------------------------------------------------------------------
// Wait
// ---------------------------------------------------------------------------

func TestWait_SucceedsWhenCapacityAvailable(t *testing.T) {
	l := New(WithMinQPS(100)) // plenty of inflight capacity
	defer l.Close()

	if err := l.Wait(context.Background()); err != nil {
		t.Errorf("Wait() = %v, want nil", err)
	}
}

func TestWait_ContextCancelled(t *testing.T) {
	l := New() // maxInflight = 1
	defer l.Close()

	// Occupy the single inflight slot so Wait can never be admitted.
	if ok, _ := l.Allow(); !ok {
		t.Fatal("first Allow() should be admitted")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := l.Wait(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Wait() with exhausted capacity and short deadline = %v, want DeadlineExceeded", err)
	}
}

// ---------------------------------------------------------------------------
// Done — records statistics into the current bucket
// ---------------------------------------------------------------------------

func TestDone_RecordsBucketStats(t *testing.T) {
	// Large window so the test and Done() land in the same bucket.
	l := New(WithWindow(100*time.Second), WithBucketCount(40))
	defer l.Close()

	if ok, _ := l.Allow(); !ok {
		t.Fatal("Allow() should be admitted")
	}
	l.Done(5 * time.Millisecond)

	idx := l.currentBucketIndexLocked(time.Now())
	if l.buckets[idx].count != 1 {
		t.Errorf("bucket count = %d, want 1", l.buckets[idx].count)
	}
	if l.buckets[idx].totalRTT != int64(5*time.Millisecond) {
		t.Errorf("bucket totalRTT = %d, want %d", l.buckets[idx].totalRTT, int64(5*time.Millisecond))
	}
}

// ---------------------------------------------------------------------------
// Close
// ---------------------------------------------------------------------------

func TestClose_ReturnsNil(t *testing.T) {
	l := New()
	if err := l.Close(); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
}

// ---------------------------------------------------------------------------
// Interface compliance
// ---------------------------------------------------------------------------

var _ ratelimit.Limiter = (*Limiter)(nil)

// ---------------------------------------------------------------------------
// Concurrent use
// ---------------------------------------------------------------------------

func TestConcurrentAllowAndDone(t *testing.T) {
	l := New(WithMinQPS(200))
	defer l.Close()

	done := make(chan struct{}, 50)
	for i := 0; i < 50; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 10; j++ {
				if ok, _ := l.Allow(); ok {
					l.Done(time.Millisecond)
				}
			}
		}()
	}
	for i := 0; i < 50; i++ {
		<-done
	}

	if inflight := l.inflight; inflight < 0 {
		t.Errorf("inflight = %d after balanced Allow/Done, want >= 0", inflight)
	}
}
