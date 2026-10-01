package conductor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/conductor-sdk/conductor-go/sdk/model"

	"github.com/tx7do/go-wind/log"
)

////////////////////////////////////////////////////////////////////////////////
/// Fake Conductor Server (httptest, 127.0.0.1 only)
////////////////////////////////////////////////////////////////////////////////

type recordedRequest struct {
	Method string
	Path   string
	Query  string
	Body   string
}

// fakeConductor is a minimal Conductor REST server for exercising the client
// wrapper hermetically.
type fakeConductor struct {
	srv *httptest.Server

	mu       sync.Mutex
	requests []recordedRequest
	forceSt  int // when non-zero, API calls fail with this status
	body     string
}

func newFakeConductor(t *testing.T) (*fakeConductor, *WorkflowClient) {
	t.Helper()
	fc := &fakeConductor{}
	fc.srv = httptest.NewServer(http.HandlerFunc(fc.handle))
	t.Cleanup(fc.srv.Close)

	client, err := NewClient(ClientOptions{ServerURL: fc.srv.URL})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return fc, client
}

func (fc *fakeConductor) handle(w http.ResponseWriter, r *http.Request) {
	rec := recordedRequest{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery}
	if r.Body != nil {
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		rec.Body = string(buf[:n])
	}
	fc.mu.Lock()
	fc.requests = append(fc.requests, rec)
	forceSt := fc.forceSt
	fc.mu.Unlock()

	// Task poll endpoints just report "no task".
	if strings.HasPrefix(r.URL.Path, "/tasks/poll") {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if forceSt != 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(forceSt)
		_, _ = w.Write([]byte("forced failure"))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/workflow":
		// StartWorkflow returns a bare JSON string workflow id.
		_, _ = w.Write([]byte(`"wf-id-123"`))
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/workflow/execute/"):
		// Execute returns a SignalResponse-shaped body.
		_, _ = w.Write([]byte(`{"workflowId":"run-1","status":"COMPLETED"}`))
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/workflow/"):
		id := strings.TrimPrefix(r.URL.Path, "/workflow/")
		_, _ = w.Write([]byte(`{"workflowId":"` + id + `","status":"COMPLETED"}`))
	default:
		w.WriteHeader(http.StatusOK)
	}
}

func (fc *fakeConductor) setFailure(status int) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.forceSt = status
}

func (fc *fakeConductor) lastRequest() recordedRequest {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	if len(fc.requests) == 0 {
		return recordedRequest{}
	}
	return fc.requests[len(fc.requests)-1]
}

// okHandler is a trivial task handler for worker tests.
func okHandler(task *model.Task) (interface{}, error) { return map[string]interface{}{}, nil }

////////////////////////////////////////////////////////////////////////////////
/// StartWorkflow (async)
////////////////////////////////////////////////////////////////////////////////

func TestStartWorkflowViaHTTP(t *testing.T) {
	fc, client := newFakeConductor(t)

	id, err := client.StartWorkflow(context.Background(), StartWorkflowOptions{
		Name:          "my_workflow",
		Input:         map[string]interface{}{"k": "v"},
		CorrelationID: "corr-1",
	})
	if err != nil {
		t.Fatalf("StartWorkflow returned error: %v", err)
	}
	if id != "wf-id-123" {
		t.Errorf("expected workflow id wf-id-123, got %s", id)
	}

	rec := fc.lastRequest()
	if rec.Method != http.MethodPost || rec.Path != "/workflow" {
		t.Errorf("unexpected request %+v", rec)
	}
	if !strings.Contains(rec.Body, `"name":"my_workflow"`) || !strings.Contains(rec.Body, `"correlationId":"corr-1"`) {
		t.Errorf("expected workflow name and correlation id in body, got %s", rec.Body)
	}
}

func TestStartWorkflowError(t *testing.T) {
	fc, client := newFakeConductor(t)
	fc.setFailure(http.StatusInternalServerError)

	_, err := client.StartWorkflow(context.Background(), StartWorkflowOptions{Name: "wf"})
	if err == nil || !strings.Contains(err.Error(), "start workflow error") {
		t.Errorf("expected wrapped start error, got %v", err)
	}
}

////////////////////////////////////////////////////////////////////////////////
/// StartWorkflowSync
////////////////////////////////////////////////////////////////////////////////

func TestStartWorkflowSyncViaHTTP(t *testing.T) {
	fc, client := newFakeConductor(t)

	run, err := client.StartWorkflowSync(
		context.Background(),
		StartWorkflowOptions{Name: "sync_workflow", Input: map[string]interface{}{"a": 1}},
		"wait_task",
	)
	if err != nil {
		t.Fatalf("StartWorkflowSync returned error: %v", err)
	}
	if run == nil {
		t.Fatal("expected non-nil workflow run")
	}
	if run.WorkflowId != "run-1" {
		t.Errorf("expected run id run-1, got %s", run.WorkflowId)
	}

	rec := fc.lastRequest()
	if !strings.HasPrefix(rec.Path, "/workflow/execute/sync_workflow/") {
		t.Errorf("unexpected execute path %s", rec.Path)
	}
	if !strings.Contains(rec.Query, "waitUntilTaskRef=wait_task") {
		t.Errorf("expected waitUntilTaskRef in query, got %s", rec.Query)
	}
}

func TestStartWorkflowSyncError(t *testing.T) {
	fc, client := newFakeConductor(t)
	fc.setFailure(http.StatusBadRequest)

	run, err := client.StartWorkflowSync(context.Background(), StartWorkflowOptions{Name: "wf"}, "task")
	if run != nil {
		t.Error("expected nil run on error")
	}
	if err == nil || !strings.Contains(err.Error(), "execute workflow error") {
		t.Errorf("expected wrapped execute error, got %v", err)
	}
}

////////////////////////////////////////////////////////////////////////////////
/// GetWorkflow
////////////////////////////////////////////////////////////////////////////////

func TestGetWorkflowViaHTTP(t *testing.T) {
	fc, client := newFakeConductor(t)

	wf, err := client.GetWorkflow(context.Background(), "get-wf", true)
	if err != nil {
		t.Fatalf("GetWorkflow returned error: %v", err)
	}
	if wf.WorkflowId != "get-wf" {
		t.Errorf("expected workflow id get-wf, got %s", wf.WorkflowId)
	}
	if wf.Status != model.CompletedWorkflow {
		t.Errorf("expected COMPLETED status, got %s", wf.Status)
	}

	rec := fc.lastRequest()
	if rec.Method != http.MethodGet || rec.Path != "/workflow/get-wf" {
		t.Errorf("unexpected request %+v", rec)
	}
	if !strings.Contains(rec.Query, "includeTasks=true") {
		t.Errorf("expected includeTasks=true in query, got %s", rec.Query)
	}
}

func TestGetWorkflowError(t *testing.T) {
	fc, client := newFakeConductor(t)
	fc.setFailure(http.StatusNotFound)

	_, err := client.GetWorkflow(context.Background(), "missing", false)
	if err == nil || !strings.Contains(err.Error(), "get workflow error") {
		t.Errorf("expected wrapped get error, got %v", err)
	}
}

////////////////////////////////////////////////////////////////////////////////
/// Terminate / Pause / Resume / Restart / Retry
////////////////////////////////////////////////////////////////////////////////

func TestWorkflowControlOperations(t *testing.T) {
	tests := []struct {
		name       string
		call       func(c *WorkflowClient) error
		wantMethod string
		wantPath   string
	}{
		{
			name:       "terminate",
			call:       func(c *WorkflowClient) error { return c.Terminate(context.Background(), "wf-1", "done") },
			wantMethod: http.MethodDelete,
			wantPath:   "/workflow/wf-1",
		},
		{
			name:       "pause",
			call:       func(c *WorkflowClient) error { return c.Pause(context.Background(), "wf-1") },
			wantMethod: http.MethodPut,
			wantPath:   "/workflow/wf-1/pause",
		},
		{
			name:       "resume",
			call:       func(c *WorkflowClient) error { return c.Resume(context.Background(), "wf-1") },
			wantMethod: http.MethodPut,
			wantPath:   "/workflow/wf-1/resume",
		},
		{
			name:       "restart",
			call:       func(c *WorkflowClient) error { return c.Restart(context.Background(), "wf-1", true) },
			wantMethod: http.MethodPost,
			wantPath:   "/workflow/wf-1/restart",
		},
		{
			name:       "retry",
			call:       func(c *WorkflowClient) error { return c.Retry(context.Background(), "wf-1", false) },
			wantMethod: http.MethodPost,
			wantPath:   "/workflow/wf-1/retry",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fc, client := newFakeConductor(t)
			if err := tt.call(client); err != nil {
				t.Fatalf("%s returned error: %v", tt.name, err)
			}
			rec := fc.lastRequest()
			if rec.Method != tt.wantMethod {
				t.Errorf("expected %s, got %s", tt.wantMethod, rec.Method)
			}
			if rec.Path != tt.wantPath {
				t.Errorf("expected path %s, got %s", tt.wantPath, rec.Path)
			}
		})
	}
}

func TestWorkflowControlOperationsError(t *testing.T) {
	fc, client := newFakeConductor(t)
	fc.setFailure(http.StatusConflict)

	if err := client.Terminate(context.Background(), "wf-1", "x"); err == nil ||
		!strings.Contains(err.Error(), "terminate workflow error") {
		t.Errorf("expected wrapped terminate error, got %v", err)
	}
	if err := client.Pause(context.Background(), "wf-1"); err == nil ||
		!strings.Contains(err.Error(), "pause workflow error") {
		t.Errorf("expected wrapped pause error, got %v", err)
	}
	if err := client.Resume(context.Background(), "wf-1"); err == nil ||
		!strings.Contains(err.Error(), "resume workflow error") {
		t.Errorf("expected wrapped resume error, got %v", err)
	}
	if err := client.Restart(context.Background(), "wf-1", false); err == nil ||
		!strings.Contains(err.Error(), "restart workflow error") {
		t.Errorf("expected wrapped restart error, got %v", err)
	}
	if err := client.Retry(context.Background(), "wf-1", true); err == nil ||
		!strings.Contains(err.Error(), "retry workflow error") {
		t.Errorf("expected wrapped retry error, got %v", err)
	}
}

////////////////////////////////////////////////////////////////////////////////
/// MonitorExecution
////////////////////////////////////////////////////////////////////////////////

func TestMonitorExecutionReceivesCompletedWorkflow(t *testing.T) {
	_, client := newFakeConductor(t)

	ch, err := client.MonitorExecution("mon-wf")
	if err != nil {
		t.Fatalf("MonitorExecution returned error: %v", err)
	}

	// The server reports the workflow COMPLETED, so the monitor daemon should
	// deliver it promptly; bound the wait to keep the test hang-free.
	select {
	case wf, ok := <-ch:
		if !ok {
			t.Fatal("execution channel closed without a result")
		}
		if wf.WorkflowId != "mon-wf" {
			t.Errorf("expected workflow id mon-wf, got %s", wf.WorkflowId)
		}
		if wf.Status != model.CompletedWorkflow {
			t.Errorf("expected COMPLETED status, got %s", wf.Status)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for monitored workflow completion")
	}
}

func TestMonitorExecutionNilExecutor(t *testing.T) {
	wc := &WorkflowClient{}
	_, err := wc.MonitorExecution("wf")
	if err == nil || !strings.Contains(err.Error(), "workflow executor is nil") {
		t.Errorf("expected executor-is-nil error, got %v", err)
	}
}

func TestStartWorkflowNilExecutor(t *testing.T) {
	wc := &WorkflowClient{}
	_, err := wc.StartWorkflow(context.Background(), StartWorkflowOptions{Name: "wf"})
	if err == nil || !strings.Contains(err.Error(), "workflow executor is nil") {
		t.Errorf("expected executor-is-nil error, got %v", err)
	}
	_, err = wc.StartWorkflowSync(context.Background(), StartWorkflowOptions{Name: "wf"}, "task")
	if err == nil || !strings.Contains(err.Error(), "workflow executor is nil") {
		t.Errorf("expected executor-is-nil error, got %v", err)
	}
}

////////////////////////////////////////////////////////////////////////////////
/// Workers
////////////////////////////////////////////////////////////////////////////////

func TestStartWorkerNilAPIClient(t *testing.T) {
	wc := &WorkflowClient{}
	if _, err := wc.StartWorker("task", okHandler, 1, time.Millisecond); err == nil ||
		!strings.Contains(err.Error(), "api client is nil") {
		t.Errorf("expected api-client-is-nil error, got %v", err)
	}
	if _, err := wc.StartWorkerWithConfig(WorkerConfig{TaskType: "task"}, okHandler); err == nil ||
		!strings.Contains(err.Error(), "api client is nil") {
		t.Errorf("expected api-client-is-nil error, got %v", err)
	}
}

func TestTaskWorkerStopTwiceIsSafe(t *testing.T) {
	_, client := newFakeConductor(t)

	tw, err := client.StartWorker("twice_task", okHandler, 1, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("StartWorker returned error: %v", err)
	}
	if tw.TaskType() != "twice_task" {
		t.Errorf("expected task type twice_task, got %s", tw.TaskType())
	}
	if !tw.IsRunning() {
		t.Error("expected worker to be running before Stop")
	}
	tw.Stop()
	tw.Stop() // must be idempotent
	if tw.IsRunning() {
		t.Error("expected worker to stay stopped")
	}
}

func TestStartWorkerWithConfigDefaults(t *testing.T) {
	_, client := newFakeConductor(t)

	tw, err := client.StartWorkerWithConfig(WorkerConfig{
		TaskType: "cfg_task",
		Domain:   "d1",
	}, okHandler)
	if err != nil {
		t.Fatalf("StartWorkerWithConfig returned error: %v", err)
	}
	defer tw.Stop()

	if tw.config.Concurrency != defaultConcurrency {
		t.Errorf("expected default concurrency, got %d", tw.config.Concurrency)
	}
	if tw.config.PollInterval != defaultPollInterval {
		t.Errorf("expected default poll interval, got %v", tw.config.PollInterval)
	}
	if tw.config.Domain != "d1" {
		t.Errorf("expected domain to be preserved, got %s", tw.config.Domain)
	}
}

////////////////////////////////////////////////////////////////////////////////
/// Client construction, accessors, Close
////////////////////////////////////////////////////////////////////////////////

func TestNewClientFromEnv(t *testing.T) {
	t.Setenv("CONDUCTOR_SERVER_URL", "http://127.0.0.1:8081/api")
	t.Setenv("CONDUCTOR_AUTH_KEY", "env-key")
	t.Setenv("CONDUCTOR_AUTH_SECRET", "env-secret")

	client, err := NewClientFromEnv()
	if err != nil {
		t.Fatalf("NewClientFromEnv returned error: %v", err)
	}
	if client.APIClient() == nil {
		t.Error("expected non-nil APIClient")
	}
	if client.WorkflowExecutor() == nil {
		t.Error("expected non-nil WorkflowExecutor")
	}
	if err := client.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}
	client.mu.RLock()
	running := client.running
	client.mu.RUnlock()
	if running {
		t.Error("expected client to be stopped after Close")
	}
}

func TestAccessors(t *testing.T) {
	_, client := newFakeConductor(t)
	if client.APIClient() == nil {
		t.Error("expected non-nil APIClient")
	}
	if client.WorkflowExecutor() == nil {
		t.Error("expected non-nil WorkflowExecutor")
	}
}

func TestToStartWorkflowRequestVersionOnly(t *testing.T) {
	version := int32(5)
	req := toStartWorkflowRequest(StartWorkflowOptions{Name: "wf", Version: &version})
	if req.Version != 5 {
		t.Errorf("expected version 5, got %d", req.Version)
	}
	if req.CorrelationId != "" {
		t.Errorf("expected empty correlation id, got %s", req.CorrelationId)
	}
	if req.Priority != 0 {
		t.Errorf("expected zero priority, got %d", req.Priority)
	}
}

////////////////////////////////////////////////////////////////////////////////
/// Logging helpers
////////////////////////////////////////////////////////////////////////////////

type recordingLogger struct {
	mu   sync.Mutex
	msgs []string
}

func (l *recordingLogger) record(msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.msgs = append(l.msgs, msg)
}

func (l *recordingLogger) Debug(_ context.Context, msg string, _ ...any) { l.record(msg) }
func (l *recordingLogger) Info(_ context.Context, msg string, _ ...any)  { l.record(msg) }
func (l *recordingLogger) Warn(_ context.Context, msg string, _ ...any)  { l.record(msg) }
func (l *recordingLogger) Error(_ context.Context, msg string, _ ...any) { l.record(msg) }
func (l *recordingLogger) Enabled(_ log.Level) bool                      { return true }
func (l *recordingLogger) With(_ ...any) log.Logger                      { return l }

func TestLogHelpers(t *testing.T) {
	lg := &recordingLogger{}
	SetLogger(lg)
	defer SetLogger(nil)

	LogDebug("debug", 1)
	LogInfo("info", 2)
	LogWarn("warn", 3)
	LogError("error", 4)
	LogFatal("fatal", 5)
	LogDebugf("debug %s", "fmt")
	LogInfof("info %s", "fmt")
	LogWarnf("warn %s", "fmt")
	LogErrorf("error %s", "fmt")
	LogFatalf("fatal %s", "fmt")

	lg.mu.Lock()
	defer lg.mu.Unlock()
	if len(lg.msgs) != 10 {
		t.Errorf("expected 10 log records, got %d", len(lg.msgs))
	}
	for _, m := range lg.msgs {
		if !strings.HasPrefix(m, logKey+" ") {
			t.Errorf("expected message %q to be prefixed with %q", m, logKey)
		}
	}
}

func TestSetLoggerNilRestoresDefault(t *testing.T) {
	SetLogger(&recordingLogger{})
	SetLogger(nil)
	LogInfo("after reset") // must not panic
	if getLogger() == nil {
		t.Error("expected fallback logger from framework")
	}
}
