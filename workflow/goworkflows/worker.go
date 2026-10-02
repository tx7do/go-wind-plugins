package goworkflows

import (
	"context"
	"fmt"
	"sync"

	"github.com/cschleiden/go-workflows/backend"
	"github.com/cschleiden/go-workflows/worker"
	"github.com/cschleiden/go-workflows/workflow"
)

// WorkflowWorker manages workflow and activity execution using a go-workflows worker.
type WorkflowWorker struct {
	mu      sync.RWMutex
	backend backend.Backend
	worker  *worker.Worker
	opts    *WorkerOptions
	ctx     context.Context
	cancel  context.CancelFunc
	running bool
	// started records whether the underlying go-workflows worker was ever
	// started successfully. Its completion signals only exist after Start,
	// so waiting on a never-started worker would block forever.
	started bool
}

// NewWorker creates a new WorkflowWorker that processes both workflows and activities.
func NewWorker(b backend.Backend, opts *WorkerOptions) (*WorkflowWorker, error) {
	if b == nil {
		return nil, fmt.Errorf("backend is nil")
	}

	workerOpts := opts.toWorkerOptions()
	w := worker.New(b, &workerOpts)

	return &WorkflowWorker{
		backend: b,
		worker:  w,
		opts:    opts,
	}, nil
}

// NewWorkflowOnlyWorker creates a new worker that only processes workflows (not activities).
func NewWorkflowOnlyWorker(b backend.Backend, opts *WorkerOptions) (*WorkflowWorker, error) {
	if b == nil {
		return nil, fmt.Errorf("backend is nil")
	}

	var workflowOpts *worker.WorkflowWorkerOptions
	if opts != nil {
		workerOpts := opts.toWorkerOptions()
		workflowOpts = &workerOpts.WorkflowWorkerOptions
	}

	w := worker.NewWorkflowWorker(b, workflowOpts)

	return &WorkflowWorker{
		backend: b,
		worker:  w,
		opts:    opts,
	}, nil
}

// NewActivityOnlyWorker creates a new worker that only processes activities (not workflows).
func NewActivityOnlyWorker(b backend.Backend, opts *WorkerOptions) (*WorkflowWorker, error) {
	if b == nil {
		return nil, fmt.Errorf("backend is nil")
	}

	var activityOpts *worker.ActivityWorkerOptions
	if opts != nil {
		workerOpts := opts.toWorkerOptions()
		activityOpts = &workerOpts.ActivityWorkerOptions
	}

	w := worker.NewActivityWorker(b, activityOpts)

	return &WorkflowWorker{
		backend: b,
		worker:  w,
		opts:    opts,
	}, nil
}

// RegisterWorkflow registers a workflow function with the worker.
func (ww *WorkflowWorker) RegisterWorkflow(wf workflow.Workflow) error {
	if err := ww.worker.RegisterWorkflow(wf); err != nil {
		return fmt.Errorf("register workflow error: %w", err)
	}
	return nil
}

// RegisterActivity registers an activity function or struct with the worker.
// Activities can be registered as functions or as structs (all public methods become activities).
func (ww *WorkflowWorker) RegisterActivity(a workflow.Activity) error {
	if err := ww.worker.RegisterActivity(a); err != nil {
		return fmt.Errorf("register activity error: %w", err)
	}
	return nil
}

// Start starts the worker. Blocks until the context is cancelled.
func (ww *WorkflowWorker) Start(ctx context.Context) error {
	ww.mu.Lock()
	if ww.running {
		ww.mu.Unlock()
		return fmt.Errorf("worker is already running")
	}

	ctx, cancel := context.WithCancel(ctx)
	ww.ctx = ctx
	ww.cancel = cancel
	ww.running = true
	ww.mu.Unlock()

	LogInfo("starting worker")

	if err := ww.worker.Start(ctx); err != nil {
		ww.mu.Lock()
		ww.running = false
		ww.cancel()
		ww.mu.Unlock()
		return fmt.Errorf("start worker error: %w", err)
	}

	ww.mu.Lock()
	ww.started = true
	ww.mu.Unlock()

	return nil
}

// Stop signals the worker to stop by cancelling the context.
func (ww *WorkflowWorker) Stop() {
	ww.mu.Lock()
	defer ww.mu.Unlock()

	if ww.cancel != nil {
		ww.cancel()
	}
	ww.running = false

	LogInfo("worker stopped")
}

// WaitForCompletion waits for all active tasks to complete.
// Should be called after Stop() to gracefully drain in-flight work.
//
// It returns an error immediately when the worker was never started: the
// underlying go-workflows worker only completes after Start, so waiting on a
// fresh (or failed-to-start) worker would block forever.
func (ww *WorkflowWorker) WaitForCompletion() error {
	ww.mu.RLock()
	started := ww.started
	ww.mu.RUnlock()

	if !started {
		return fmt.Errorf("worker was never started: WaitForCompletion would block forever")
	}

	return ww.worker.WaitForCompletion()
}

// IsRunning returns whether the worker is currently running.
func (ww *WorkflowWorker) IsRunning() bool {
	ww.mu.RLock()
	defer ww.mu.RUnlock()
	return ww.running
}
