package sentinel

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"

	sentinelapi "github.com/alibaba/sentinel-golang/api"
	"github.com/alibaba/sentinel-golang/core/base"
	sentinelconfig "github.com/alibaba/sentinel-golang/core/config"
	"github.com/alibaba/sentinel-golang/core/flow"

	"github.com/tx7do/go-wind-plugins/ratelimit"
)

// TestMain initializes Sentinel with a minimal in-memory configuration: no
// metric log task, no system-statistic collectors, and a throwaway log
// directory. No dashboard or external service is contacted.
func TestMain(m *testing.M) {
	logDir, err := os.MkdirTemp("", "sentinel-test-logs")
	if err != nil {
		logDir = os.TempDir()
	}

	conf := sentinelconfig.NewDefaultConfig()
	conf.Sentinel.Log.Dir = logDir
	conf.Sentinel.Log.Metric.FlushIntervalSec = 0                 // disable metric log task
	conf.Sentinel.Stat.System = sentinelconfig.SystemStatConfig{} // disable system collectors

	_ = sentinelapi.InitWithConfig(conf)

	code := m.Run()

	_ = os.RemoveAll(logDir)
	os.Exit(code)
}

// loadFlowRule installs a Direct/Reject QPS flow rule for the resource.
func loadFlowRule(t *testing.T, resource string, threshold float64) {
	t.Helper()
	ok, err := flow.LoadRules([]*flow.Rule{{
		Resource:               resource,
		TokenCalculateStrategy: flow.Direct,
		ControlBehavior:        flow.Reject,
		Threshold:              threshold,
		StatIntervalInMs:       1000,
	}})
	if err != nil || !ok {
		t.Fatalf("flow.LoadRules() = (%v, %v), want (true, nil)", ok, err)
	}
	t.Cleanup(func() { _ = flow.ClearRulesOfResource(resource) })
}

// ---------------------------------------------------------------------------
// New / options
// ---------------------------------------------------------------------------

func TestNew_Defaults(t *testing.T) {
	l := New("test-resource-defaults")
	defer l.Close()

	if l.resource != "test-resource-defaults" {
		t.Errorf("resource = %q, want %q", l.resource, "test-resource-defaults")
	}
	if l.cfg.trafficType != base.Inbound {
		t.Errorf("trafficType = %v, want Inbound", l.cfg.trafficType)
	}
	if l.cfg.waitInterval != 10*time.Millisecond {
		t.Errorf("waitInterval = %v, want 10ms", l.cfg.waitInterval)
	}
}

func TestWithTrafficType(t *testing.T) {
	l := New("r", WithTrafficType(base.Outbound))
	defer l.Close()

	if l.cfg.trafficType != base.Outbound {
		t.Errorf("trafficType = %v, want Outbound", l.cfg.trafficType)
	}
}

func TestWithWaitInterval(t *testing.T) {
	l := New("r", WithWaitInterval(25*time.Millisecond))
	defer l.Close()
	if l.cfg.waitInterval != 25*time.Millisecond {
		t.Errorf("waitInterval = %v, want 25ms", l.cfg.waitInterval)
	}

	// Non-positive values must be ignored.
	l2 := New("r", WithWaitInterval(0))
	defer l2.Close()
	if l2.cfg.waitInterval != 10*time.Millisecond {
		t.Errorf("waitInterval = %v, want default 10ms", l2.cfg.waitInterval)
	}
}

func TestWithEntryOptions(t *testing.T) {
	l := New("r", WithEntryOptions(sentinelapi.WithAcquireCount(2)))
	defer l.Close()

	if len(l.cfg.entryOpts) != 1 {
		t.Fatalf("len(entryOpts) = %d, want 1", len(l.cfg.entryOpts))
	}
}

// ---------------------------------------------------------------------------
// Allow — no rules configured
// ---------------------------------------------------------------------------

func TestAllow_NoRulesAlwaysAdmits(t *testing.T) {
	l := New("test-resource-no-rules")
	defer l.Close()

	for i := 0; i < 5; i++ {
		ok, err := l.Allow()
		if !ok || err != nil {
			t.Fatalf("Allow() #%d = (%v, %v), want (true, nil)", i, ok, err)
		}
	}
}

// ---------------------------------------------------------------------------
// Allow — QPS flow rule
// ---------------------------------------------------------------------------

func TestAllow_QPSRuleRejectsBeyondThreshold(t *testing.T) {
	const res = "test-resource-qps-1"
	loadFlowRule(t, res, 1) // 1 QPS

	l := New(res)
	defer l.Close()

	ok, err := l.Allow()
	if !ok || err != nil {
		t.Fatalf("first Allow() = (%v, %v), want (true, nil)", ok, err)
	}

	ok, err = l.Allow()
	if ok {
		t.Error("second Allow() beyond the 1 QPS threshold should be rejected")
	}
	if !errors.Is(err, ratelimit.ErrLimited) {
		t.Errorf("second Allow() err = %v, want ErrLimited", err)
	}
}

func TestAllow_QPSRuleAdmitsWithinThreshold(t *testing.T) {
	const res = "test-resource-qps-5"
	loadFlowRule(t, res, 5) // 5 QPS

	l := New(res)
	defer l.Close()

	for i := 0; i < 5; i++ {
		ok, err := l.Allow()
		if !ok || err != nil {
			t.Fatalf("Allow() #%d = (%v, %v), want (true, nil)", i, ok, err)
		}
	}
}

// ---------------------------------------------------------------------------
// AllowEntry / ReleaseEntry
// ---------------------------------------------------------------------------

func TestAllowEntry_AdmittedAndReleased(t *testing.T) {
	const res = "test-resource-entry"
	loadFlowRule(t, res, 2)

	l := New(res)
	defer l.Close()

	e, err := l.AllowEntry()
	if err != nil {
		t.Fatalf("AllowEntry() = %v, want nil", err)
	}
	if e == nil {
		t.Fatal("AllowEntry() returned a nil entry")
	}
	l.ReleaseEntry(e)

	// A blocked attempt (after exhausting the threshold) yields no entry.
	l.AllowEntry()
	l.AllowEntry()
	e2, err := l.AllowEntry()
	if !errors.Is(err, ratelimit.ErrLimited) {
		t.Fatalf("AllowEntry() beyond threshold err = %v, want ErrLimited", err)
	}
	if e2 != nil {
		t.Error("AllowEntry() beyond threshold should return a nil entry")
	}

	// Releasing a nil entry must not panic.
	l.ReleaseEntry(nil)
}

// ---------------------------------------------------------------------------
// Wait
// ---------------------------------------------------------------------------

func TestWait_NoRulesSucceedsImmediately(t *testing.T) {
	l := New("test-resource-wait-ok")
	defer l.Close()

	if err := l.Wait(context.Background()); err != nil {
		t.Errorf("Wait() = %v, want nil", err)
	}
}

func TestWait_ContextCancelledWhileBlocked(t *testing.T) {
	const res = "test-resource-wait-blocked"
	loadFlowRule(t, res, 1) // exhaust after one pass

	l := New(res)
	defer l.Close()

	// Exhaust the 1 QPS threshold so Wait is blocked.
	l.Allow()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := l.Wait(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Wait() while blocked with short deadline = %v, want DeadlineExceeded", err)
	}
}

// ---------------------------------------------------------------------------
// Close
// ---------------------------------------------------------------------------

func TestClose_IsNoOp(t *testing.T) {
	l := New("test-resource-close")
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

func TestConcurrentAllow(t *testing.T) {
	const res = "test-resource-concurrent"
	loadFlowRule(t, res, 1000) // high threshold — most calls pass

	l := New(res)
	defer l.Close()

	var allowed, rejected int32
	done := make(chan struct{}, 50)
	for i := 0; i < 50; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			ok, _ := l.Allow()
			if ok {
				atomic.AddInt32(&allowed, 1)
			} else {
				atomic.AddInt32(&rejected, 1)
			}
		}()
	}
	for i := 0; i < 50; i++ {
		<-done
	}

	if allowed+rejected != 50 {
		t.Errorf("allowed+rejected = %d, want 50", allowed+rejected)
	}
	if rejected > 50 { // sanity — cannot happen
		t.Errorf("rejected = %d, want <= 50", rejected)
	}
}
