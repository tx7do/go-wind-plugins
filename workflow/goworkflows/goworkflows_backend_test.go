package goworkflows

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cschleiden/go-workflows/backend"
	"github.com/cschleiden/go-workflows/backend/history"
	"github.com/cschleiden/go-workflows/backend/metrics"
	"github.com/cschleiden/go-workflows/core"
	"github.com/cschleiden/go-workflows/worker"
	"github.com/cschleiden/go-workflows/workflow"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

////////////////////////////////////////////////////////////////////////////////
/// Mock backend
////////////////////////////////////////////////////////////////////////////////

// mockBackend is a minimal in-memory backend.Backend used to exercise the
// wrapper logic without any workflow execution. Only the methods the wrapper
// reaches are implemented; the rest are inherited (and panic if ever called,
// which would be a test bug).
type mockBackend struct {
	backend.Backend

	mu            sync.Mutex
	opts          *backend.Options
	closed        bool
	createCount   int
	cancelCount   int
	signalCount   int
	removeCount   int
	removeMany    bool
	state         core.WorkflowInstanceState
	createErr     error
	cancelErr     error
	signalErr     error
	stateErr      error
	removeErr     error
	removeManyErr error
	closeErr      error
}

func newMockBackend() *mockBackend {
	return &mockBackend{
		opts:  &backend.DefaultOptions,
		state: core.WorkflowInstanceStateFinished,
	}
}

func (b *mockBackend) Options() *backend.Options {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.opts
}

func (b *mockBackend) Tracer() trace.Tracer {
	return noop.NewTracerProvider().Tracer("test")
}

func (b *mockBackend) Metrics() metrics.Client { return noopMetricsClient{} }

func (b *mockBackend) FeatureSupported(_ backend.Feature) bool { return false }

// noopMetricsClient is a metrics.Client that discards everything.
type noopMetricsClient struct{}

func (noopMetricsClient) Counter(_ string, _ metrics.Tags, _ int64)        {}
func (noopMetricsClient) Distribution(_ string, _ metrics.Tags, _ float64) {}
func (noopMetricsClient) Gauge(_ string, _ metrics.Tags, _ int64)          {}
func (noopMetricsClient) Timing(_ string, _ metrics.Tags, _ time.Duration) {}
func (c noopMetricsClient) WithTags(_ metrics.Tags) metrics.Client         { return c }

func (b *mockBackend) CreateWorkflowInstance(_ context.Context, _ *workflow.Instance, _ *history.Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.createCount++
	return b.createErr
}

func (b *mockBackend) CancelWorkflowInstance(_ context.Context, _ *workflow.Instance, _ *history.Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cancelCount++
	return b.cancelErr
}

func (b *mockBackend) SignalWorkflow(_ context.Context, _ string, _ *history.Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.signalCount++
	return b.signalErr
}

func (b *mockBackend) GetWorkflowInstanceState(_ context.Context, _ *workflow.Instance) (core.WorkflowInstanceState, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state, b.stateErr
}

func (b *mockBackend) RemoveWorkflowInstance(_ context.Context, _ *workflow.Instance) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.removeCount++
	return b.removeErr
}

func (b *mockBackend) RemoveWorkflowInstances(_ context.Context, _ ...backend.RemovalOption) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.removeMany = true
	return b.removeManyErr
}

func (b *mockBackend) PrepareWorkflowQueues(_ context.Context, _ []workflow.Queue) error { return nil }
func (b *mockBackend) PrepareActivityQueues(_ context.Context, _ []workflow.Queue) error { return nil }

// GetWorkflowTask blocks until the context is cancelled, so a started worker
// simply idles without touching anything.
func (b *mockBackend) GetWorkflowTask(ctx context.Context, _ []workflow.Queue) (*backend.WorkflowTask, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// GetActivityTask mirrors GetWorkflowTask for the activity pollers.
func (b *mockBackend) GetActivityTask(ctx context.Context, _ []workflow.Queue) (*backend.ActivityTask, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (b *mockBackend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	return b.closeErr
}

func (b *mockBackend) snapshot() (create, cancel, signal, remove int, removeMany, closed bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.createCount, b.cancelCount, b.signalCount, b.removeCount, b.removeMany, b.closed
}

// sampleWorkflow is a valid workflow function for the client APIs.
func sampleWorkflow(ctx workflow.Context) error {
	return nil
}

////////////////////////////////////////////////////////////////////////////////
/// Client construction and accessors
////////////////////////////////////////////////////////////////////////////////

func TestNewClientWithMockBackend(t *testing.T) {
	mb := newMockBackend()
	wc, err := NewClient(mb)
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	if wc.Backend() == nil {
		t.Error("expected Backend() to return the mock backend")
	}
	if wc.InnerClient() == nil {
		t.Error("expected InnerClient() to be non-nil")
	}

	if err := wc.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if _, _, _, _, _, closed := mb.snapshot(); !closed {
		t.Error("expected backend Close to be delegated")
	}
}

func TestCloseWithNilBackend(t *testing.T) {
	wc := &WorkflowClient{}
	if err := wc.Close(); err != nil {
		t.Errorf("expected nil error closing nil-backend client, got %v", err)
	}
	if wc.Backend() != nil {
		t.Error("expected nil Backend() on zero client")
	}
	if wc.InnerClient() != nil {
		t.Error("expected nil InnerClient() on zero client")
	}
}

////////////////////////////////////////////////////////////////////////////////
/// CreateWorkflowInstance
////////////////////////////////////////////////////////////////////////////////

func TestCreateWorkflowInstanceSuccess(t *testing.T) {
	mb := newMockBackend()
	wc, err := NewClient(mb)
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	instance, err := wc.CreateWorkflowInstance(
		context.Background(),
		CreateWorkflowOptions{InstanceID: "instance-1"},
		sampleWorkflow,
	)
	if err != nil {
		t.Fatalf("CreateWorkflowInstance returned error: %v", err)
	}
	if instance == nil {
		t.Fatal("expected non-nil instance")
	}
	if instance.InstanceID != "instance-1" {
		t.Errorf("expected instance ID instance-1, got %s", instance.InstanceID)
	}
	if instance.ExecutionID == "" {
		t.Error("expected non-empty execution ID")
	}
	if create, _, _, _, _, _ := mb.snapshot(); create != 1 {
		t.Errorf("expected backend CreateWorkflowInstance to be called once, got %d", create)
	}
}

func TestCreateWorkflowInstanceQueueDefaulting(t *testing.T) {
	mb := newMockBackend()
	wc, _ := NewClient(mb)

	if _, err := wc.CreateWorkflowInstance(
		context.Background(),
		CreateWorkflowOptions{InstanceID: "i", Queue: workflow.QueueDefault},
		sampleWorkflow,
	); err != nil {
		t.Fatalf("CreateWorkflowInstance returned error: %v", err)
	}
}

func TestCreateWorkflowInstanceEmptyInstanceID(t *testing.T) {
	mb := newMockBackend()
	wc, _ := NewClient(mb)

	_, err := wc.CreateWorkflowInstance(context.Background(), CreateWorkflowOptions{}, sampleWorkflow)
	if err == nil {
		t.Fatal("expected error for empty InstanceID")
	}
	if !errors.Is(err, err) || !contains(err.Error(), "InstanceID must be set") {
		t.Errorf("expected upstream InstanceID error, got %v", err)
	}
}

func TestCreateWorkflowInstanceNilInnerClient(t *testing.T) {
	wc := &WorkflowClient{}
	_, err := wc.CreateWorkflowInstance(
		context.Background(),
		CreateWorkflowOptions{InstanceID: "i"},
		sampleWorkflow,
	)
	if err == nil || !contains(err.Error(), "client is nil") {
		t.Errorf("expected client-is-nil error, got %v", err)
	}
}

func TestCreateWorkflowInstanceBackendError(t *testing.T) {
	mb := newMockBackend()
	mb.createErr = errors.New("storage down")
	wc, _ := NewClient(mb)

	_, err := wc.CreateWorkflowInstance(
		context.Background(),
		CreateWorkflowOptions{InstanceID: "i"},
		sampleWorkflow,
	)
	if err == nil || !contains(err.Error(), "create workflow instance error") {
		t.Errorf("expected wrapped backend error, got %v", err)
	}
}

////////////////////////////////////////////////////////////////////////////////
/// CancelWorkflowInstance
////////////////////////////////////////////////////////////////////////////////

func TestCancelWorkflowInstance(t *testing.T) {
	mb := newMockBackend()
	wc, _ := NewClient(mb)
	instance := core.NewWorkflowInstance("instance-1", "execution-1")

	if err := wc.CancelWorkflowInstance(context.Background(), instance); err != nil {
		t.Fatalf("CancelWorkflowInstance returned error: %v", err)
	}
	if _, cancel, _, _, _, _ := mb.snapshot(); cancel != 1 {
		t.Errorf("expected backend cancel to be called once, got %d", cancel)
	}

	mb.cancelErr = errors.New("not running")
	if err := wc.CancelWorkflowInstance(context.Background(), instance); err == nil ||
		!contains(err.Error(), "cancel workflow instance error") {
		t.Errorf("expected wrapped cancel error, got %v", err)
	}
}

func TestCancelWorkflowInstanceNilClient(t *testing.T) {
	wc := &WorkflowClient{}
	err := wc.CancelWorkflowInstance(context.Background(), core.NewWorkflowInstance("i", "e"))
	if err == nil || !contains(err.Error(), "client is nil") {
		t.Errorf("expected client-is-nil error, got %v", err)
	}
}

////////////////////////////////////////////////////////////////////////////////
/// SignalWorkflow
////////////////////////////////////////////////////////////////////////////////

func TestSignalWorkflow(t *testing.T) {
	mb := newMockBackend()
	wc, _ := NewClient(mb)

	if err := wc.SignalWorkflow(context.Background(), "instance-1", "greeting", "hello"); err != nil {
		t.Fatalf("SignalWorkflow returned error: %v", err)
	}
	if _, _, signal, _, _, _ := mb.snapshot(); signal != 1 {
		t.Errorf("expected backend signal to be called once, got %d", signal)
	}

	mb.signalErr = errors.New("instance gone")
	if err := wc.SignalWorkflow(context.Background(), "instance-1", "greeting", "hello"); err == nil ||
		!contains(err.Error(), "signal workflow error") {
		t.Errorf("expected wrapped signal error, got %v", err)
	}
}

func TestSignalWorkflowNilClient(t *testing.T) {
	wc := &WorkflowClient{}
	if err := wc.SignalWorkflow(context.Background(), "i", "sig", nil); err == nil ||
		!contains(err.Error(), "client is nil") {
		t.Errorf("expected client-is-nil error, got %v", err)
	}
}

////////////////////////////////////////////////////////////////////////////////
/// GetWorkflowInstanceState / WaitForWorkflowInstance
////////////////////////////////////////////////////////////////////////////////

func TestGetWorkflowInstanceState(t *testing.T) {
	mb := newMockBackend()
	wc, _ := NewClient(mb)
	instance := core.NewWorkflowInstance("instance-1", "execution-1")

	state, err := wc.GetWorkflowInstanceState(context.Background(), instance)
	if err != nil {
		t.Fatalf("GetWorkflowInstanceState returned error: %v", err)
	}
	if state != core.WorkflowInstanceStateFinished {
		t.Errorf("expected finished state, got %v", state)
	}

	mb.stateErr = errors.New("state unavailable")
	if _, err := wc.GetWorkflowInstanceState(context.Background(), instance); err == nil ||
		!contains(err.Error(), "get workflow state error") {
		t.Errorf("expected wrapped state error, got %v", err)
	}
}

func TestGetWorkflowInstanceStateNilClient(t *testing.T) {
	wc := &WorkflowClient{}
	_, err := wc.GetWorkflowInstanceState(context.Background(), core.NewWorkflowInstance("i", "e"))
	if err == nil || !contains(err.Error(), "client is nil") {
		t.Errorf("expected client-is-nil error, got %v", err)
	}
}

func TestWaitForWorkflowInstanceFinished(t *testing.T) {
	mb := newMockBackend()
	wc, _ := NewClient(mb)
	instance := core.NewWorkflowInstance("instance-1", "execution-1")

	// timeout 0 falls back to the default; the mock reports Finished on the
	// first poll so the call returns immediately.
	if err := wc.WaitForWorkflowInstance(context.Background(), instance, 0); err != nil {
		t.Fatalf("WaitForWorkflowInstance returned error: %v", err)
	}
}

func TestWaitForWorkflowInstanceError(t *testing.T) {
	mb := newMockBackend()
	mb.stateErr = errors.New("boom")
	wc, _ := NewClient(mb)
	instance := core.NewWorkflowInstance("instance-1", "execution-1")

	err := wc.WaitForWorkflowInstance(context.Background(), instance, time.Second)
	if err == nil || !contains(err.Error(), "wait for workflow instance error") {
		t.Errorf("expected wrapped wait error, got %v", err)
	}
}

func TestWaitForWorkflowInstanceNilClient(t *testing.T) {
	wc := &WorkflowClient{}
	err := wc.WaitForWorkflowInstance(context.Background(), core.NewWorkflowInstance("i", "e"), time.Second)
	if err == nil || !contains(err.Error(), "client is nil") {
		t.Errorf("expected client-is-nil error, got %v", err)
	}
}

////////////////////////////////////////////////////////////////////////////////
/// RemoveWorkflowInstance(s)
////////////////////////////////////////////////////////////////////////////////

func TestRemoveWorkflowInstance(t *testing.T) {
	mb := newMockBackend()
	wc, _ := NewClient(mb)
	instance := core.NewWorkflowInstance("instance-1", "execution-1")

	if err := wc.RemoveWorkflowInstance(context.Background(), instance); err != nil {
		t.Fatalf("RemoveWorkflowInstance returned error: %v", err)
	}
	if _, _, _, remove, _, _ := mb.snapshot(); remove != 1 {
		t.Errorf("expected backend remove to be called once, got %d", remove)
	}

	mb.removeErr = errors.New("still running")
	if err := wc.RemoveWorkflowInstance(context.Background(), instance); err == nil ||
		!contains(err.Error(), "remove workflow instance error") {
		t.Errorf("expected wrapped remove error, got %v", err)
	}
}

func TestRemoveWorkflowInstances(t *testing.T) {
	mb := newMockBackend()
	wc, _ := NewClient(mb)

	if err := wc.RemoveWorkflowInstances(context.Background()); err != nil {
		t.Fatalf("RemoveWorkflowInstances returned error: %v", err)
	}
	if _, _, _, _, removeMany, _ := mb.snapshot(); !removeMany {
		t.Error("expected backend RemoveWorkflowInstances to be called")
	}

	mb.removeManyErr = errors.New("removal failed")
	if err := wc.RemoveWorkflowInstances(context.Background()); err == nil ||
		!contains(err.Error(), "remove workflow instances error") {
		t.Errorf("expected wrapped remove-many error, got %v", err)
	}
}

func TestRemoveWorkflowNilClients(t *testing.T) {
	wc := &WorkflowClient{}
	instance := core.NewWorkflowInstance("i", "e")

	if err := wc.RemoveWorkflowInstance(context.Background(), instance); err == nil ||
		!contains(err.Error(), "client is nil") {
		t.Errorf("expected client-is-nil error, got %v", err)
	}
	if err := wc.RemoveWorkflowInstances(context.Background()); err == nil ||
		!contains(err.Error(), "client is nil") {
		t.Errorf("expected client-is-nil error, got %v", err)
	}
}

////////////////////////////////////////////////////////////////////////////////
/// Worker construction
////////////////////////////////////////////////////////////////////////////////

func TestNewWorkerWithMockBackend(t *testing.T) {
	mb := newMockBackend()
	ww, err := NewWorker(mb, &WorkerOptions{WorkflowPollers: 1, ActivityPollers: 1})
	if err != nil {
		t.Fatalf("NewWorker returned error: %v", err)
	}
	if ww == nil {
		t.Fatal("expected non-nil worker")
	}
	if ww.IsRunning() {
		t.Error("expected worker to be idle before Start")
	}
}

func TestNewWorkflowOnlyWorkerWithMockBackend(t *testing.T) {
	mb := newMockBackend()
	ww, err := NewWorkflowOnlyWorker(mb, nil)
	if err != nil {
		t.Fatalf("NewWorkflowOnlyWorker returned error: %v", err)
	}
	if ww == nil {
		t.Fatal("expected non-nil workflow-only worker")
	}
}

func TestNewActivityOnlyWorkerWithMockBackend(t *testing.T) {
	mb := newMockBackend()
	ww, err := NewActivityOnlyWorker(mb, &WorkerOptions{ActivityPollers: 1})
	if err != nil {
		t.Fatalf("NewActivityOnlyWorker returned error: %v", err)
	}
	if ww == nil {
		t.Fatal("expected non-nil activity-only worker")
	}
}

////////////////////////////////////////////////////////////////////////////////
/// Worker start/stop lifecycle against the blocking mock backend
////////////////////////////////////////////////////////////////////////////////

func TestWorkerStartStopLifecycle(t *testing.T) {
	mb := newMockBackend()
	ww, err := NewWorker(mb, &WorkerOptions{WorkflowPollers: 1, ActivityPollers: 1})
	if err != nil {
		t.Fatalf("NewWorker returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := ww.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if !ww.IsRunning() {
		t.Error("expected worker to be running after Start")
	}

	// Second start must be rejected.
	if err := ww.Start(ctx); err == nil || !contains(err.Error(), "worker is already running") {
		t.Errorf("expected already-running error, got %v", err)
	}

	// Stop (cancels the worker context) before draining in-flight work.
	ww.Stop()
	if ww.IsRunning() {
		t.Error("expected worker to be stopped after Stop")
	}

	// WaitForCompletion must return once pollers have wound down.
	select {
	case err := <-waitCompletion(ww):
		if err != nil {
			t.Errorf("WaitForCompletion returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("WaitForCompletion did not return within 5s")
	}

	// Cancel the context as well so any leftover pollers wind down.
	cancel()
}

// waitCompletion runs WaitForCompletion on its own goroutine so callers can
// bound the wait.
func waitCompletion(ww *WorkflowWorker) <-chan error {
	ch := make(chan error, 1)
	go func() { ch <- ww.WaitForCompletion() }()
	return ch
}

func TestWorkerStopTwiceIsSafe(t *testing.T) {
	mb := newMockBackend()
	ww, _ := NewWorker(mb, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := ww.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	ww.Stop()
	ww.Stop() // must not panic or re-enable the worker
	if ww.IsRunning() {
		t.Error("expected worker to stay stopped")
	}
}

// NOTE: WaitForCompletion is only safe to call after a worker was started and
// stopped; on a never-started worker it blocks indefinitely (upstream go-workflows
// behavior), so there is deliberately no test for that case.

////////////////////////////////////////////////////////////////////////////////
/// toWorkerOptions full coverage
////////////////////////////////////////////////////////////////////////////////

func TestToWorkerOptionsAllFields(t *testing.T) {
	opts := &WorkerOptions{
		WorkflowPollers:           7,
		MaxParallelWorkflowTasks:  11,
		WorkflowHeartbeatInterval: 31 * time.Second,
		WorkflowPollingInterval:   210 * time.Millisecond,
		WorkflowExecutorCacheSize: 256,
		WorkflowExecutorCacheTTL:  12 * time.Second,
		WorkflowQueues:            []workflow.Queue{workflow.QueueDefault, "custom"},
		ActivityPollers:           5,
		MaxParallelActivityTasks:  9,
		ActivityHeartbeatInterval: 27 * time.Second,
		ActivityPollingInterval:   190 * time.Millisecond,
		ActivityQueues:            []workflow.Queue{"activity-q"},
		SingleWorkerMode:          true,
	}

	result := opts.toWorkerOptions()

	if result.WorkflowPollers != 7 {
		t.Errorf("WorkflowPollers = %d, want 7", result.WorkflowPollers)
	}
	if result.MaxParallelWorkflowTasks != 11 {
		t.Errorf("MaxParallelWorkflowTasks = %d, want 11", result.MaxParallelWorkflowTasks)
	}
	if result.WorkflowHeartbeatInterval != 31*time.Second {
		t.Errorf("WorkflowHeartbeatInterval = %v, want 31s", result.WorkflowHeartbeatInterval)
	}
	if result.WorkflowPollingInterval != 210*time.Millisecond {
		t.Errorf("WorkflowPollingInterval = %v, want 210ms", result.WorkflowPollingInterval)
	}
	if result.WorkflowExecutorCacheSize != 256 {
		t.Errorf("WorkflowExecutorCacheSize = %d, want 256", result.WorkflowExecutorCacheSize)
	}
	if result.WorkflowExecutorCacheTTL != 12*time.Second {
		t.Errorf("WorkflowExecutorCacheTTL = %v, want 12s", result.WorkflowExecutorCacheTTL)
	}
	if len(result.WorkflowQueues) != 2 {
		t.Errorf("WorkflowQueues = %v, want 2 queues", result.WorkflowQueues)
	}
	if result.ActivityPollers != 5 {
		t.Errorf("ActivityPollers = %d, want 5", result.ActivityPollers)
	}
	if result.MaxParallelActivityTasks != 9 {
		t.Errorf("MaxParallelActivityTasks = %d, want 9", result.MaxParallelActivityTasks)
	}
	if result.ActivityHeartbeatInterval != 27*time.Second {
		t.Errorf("ActivityHeartbeatInterval = %v, want 27s", result.ActivityHeartbeatInterval)
	}
	if result.ActivityPollingInterval != 190*time.Millisecond {
		t.Errorf("ActivityPollingInterval = %v, want 190ms", result.ActivityPollingInterval)
	}
	if len(result.ActivityQueues) != 1 || result.ActivityQueues[0] != "activity-q" {
		t.Errorf("ActivityQueues = %v, want [activity-q]", result.ActivityQueues)
	}
	if !result.SingleWorkerMode {
		t.Error("SingleWorkerMode = false, want true")
	}
}

func TestToWorkerOptionsZeroValuesKeepDefaults(t *testing.T) {
	result := (&WorkerOptions{SingleWorkerMode: false}).toWorkerOptions()
	defaults := worker.DefaultOptions

	if result.WorkflowPollers != defaults.WorkflowPollers {
		t.Errorf("expected default WorkflowPollers, got %d", result.WorkflowPollers)
	}
	if result.ActivityPollers != defaults.ActivityPollers {
		t.Errorf("expected default ActivityPollers, got %d", result.ActivityPollers)
	}
	if result.SingleWorkerMode {
		t.Error("expected SingleWorkerMode false")
	}
}

// contains is a tiny helper to keep assertions readable without testify.
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
