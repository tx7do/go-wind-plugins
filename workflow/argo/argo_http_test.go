package argo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/tx7do/go-wind/log"
)

////////////////////////////////////////////////////////////////////////////////
/// Fake Argo Server (httptest, 127.0.0.1 only)
////////////////////////////////////////////////////////////////////////////////

// recordedRequest captures the essentials of an incoming HTTP request.
type recordedRequest struct {
	Method string
	Path   string
	Query  string
	Body   string
	Token  string
}

// fakeArgo is a minimal in-process Argo Server used to exercise the REST
// client. Responses are configurable per test.
type fakeArgo struct {
	srv *httptest.Server

	mu       sync.Mutex
	last     recordedRequest
	status   int // HTTP status to respond with
	body     string
	respCT   string
	respond  func(rec recordedRequest) (int, string) // optional dynamic handler
	failJSON bool                                    // respond 200 with invalid JSON
}

func newFakeArgo(t *testing.T) (*fakeArgo, *WorkflowClient) {
	t.Helper()
	fa := &fakeArgo{status: http.StatusOK}
	fa.srv = httptest.NewServer(http.HandlerFunc(fa.handle))
	t.Cleanup(fa.srv.Close)

	client, err := NewClient(ClientOptions{ServerURL: fa.srv.URL})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	return fa, client
}

func (fa *fakeArgo) handle(w http.ResponseWriter, r *http.Request) {
	rec := recordedRequest{
		Method: r.Method,
		Path:   r.URL.Path,
		Query:  r.URL.RawQuery,
		Token:  r.Header.Get("Authorization"),
	}
	if r.Body != nil {
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		rec.Body = string(buf[:n])
	}

	fa.mu.Lock()
	fa.last = rec
	status, body := fa.status, fa.body
	respond := fa.respond
	failJSON := fa.failJSON
	fa.mu.Unlock()

	if respond != nil {
		status, body = respond(rec)
	}
	if failJSON {
		body = "{not-valid-json"
	}
	if status == 0 {
		status = http.StatusOK
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func (fa *fakeArgo) request() recordedRequest {
	fa.mu.Lock()
	defer fa.mu.Unlock()
	return fa.last
}

// setResponse configures the canned response for the next request.
func (fa *fakeArgo) setResponse(status int, body string) {
	fa.mu.Lock()
	defer fa.mu.Unlock()
	fa.status = status
	fa.body = body
}

// sampleWorkflowJSON is a minimal Workflow document the fake server returns.
const sampleWorkflowJSON = `{
	"apiVersion": "argoproj.io/v1alpha1",
	"kind": "Workflow",
	"metadata": {"name": "hello-world-abc123", "namespace": "default"},
	"status": {"phase": "Running"}
}`

func sampleWorkflowListJSON() string {
	return `{"items": [{"metadata": {"name": "wf-1"}}, {"metadata": {"name": "wf-2"}}]}`
}

////////////////////////////////////////////////////////////////////////////////
/// Submit
////////////////////////////////////////////////////////////////////////////////

func TestSubmitWorkflowHTTP(t *testing.T) {
	fa, client := newFakeArgo(t)
	fa.setResponse(http.StatusOK, sampleWorkflowJSON)
	defer func() { _ = client.Close() }()

	wf := &Workflow{
		APIVersion: "argoproj.io/v1alpha1",
		Kind:       "Workflow",
		Metadata:   ObjectMeta{GenerateName: "hello-world-"},
	}

	result, err := client.SubmitWorkflow(context.Background(), wf, nil)
	if err != nil {
		t.Fatalf("SubmitWorkflow returned error: %v", err)
	}
	if result.Metadata.Name != "hello-world-abc123" {
		t.Errorf("expected workflow name hello-world-abc123, got %s", result.Metadata.Name)
	}
	if result.Status == nil || result.Status.Phase != PhaseRunning {
		t.Errorf("expected status phase Running, got %+v", result.Status)
	}

	rec := fa.request()
	if rec.Method != http.MethodPost {
		t.Errorf("expected POST, got %s", rec.Method)
	}
	if rec.Path != "/api/v1/workflows/default" {
		t.Errorf("expected path /api/v1/workflows/default, got %s", rec.Path)
	}
	if rec.Query != "" {
		t.Errorf("expected no query parameters, got %s", rec.Query)
	}
	var submitted Workflow
	if err := json.Unmarshal([]byte(rec.Body), &submitted); err != nil {
		t.Fatalf("request body is not valid JSON: %v", err)
	}
	if submitted.Metadata.GenerateName != "hello-world-" {
		t.Errorf("expected generateName hello-world-, got %s", submitted.Metadata.GenerateName)
	}
}

func TestSubmitWorkflowWithOptions(t *testing.T) {
	fa, client := newFakeArgo(t)
	fa.setResponse(http.StatusOK, sampleWorkflowJSON)
	defer func() { _ = client.Close() }()

	wf := &Workflow{Metadata: ObjectMeta{GenerateName: "params-"}}
	opts := &SubmitOptions{
		Namespace:    "custom-ns",
		ServerDryRun: true,
		Parameters:   []string{"message=hello", "count=3"},
	}

	if _, err := client.SubmitWorkflow(context.Background(), wf, opts); err != nil {
		t.Fatalf("SubmitWorkflow returned error: %v", err)
	}

	rec := fa.request()
	if rec.Path != "/api/v1/workflows/custom-ns" {
		t.Errorf("expected custom namespace path, got %s", rec.Path)
	}
	for _, want := range []string{"serverDryRun=true", "entrypointParameters=message%3Dhello", "entrypointParameters=count%3D3"} {
		if !strings.Contains(rec.Query, want) {
			t.Errorf("expected query %q to contain %q", rec.Query, want)
		}
	}
}

func TestSubmitWorkflowServerError(t *testing.T) {
	fa, client := newFakeArgo(t)
	fa.setResponse(http.StatusInternalServerError, "boom")
	defer func() { _ = client.Close() }()

	_, err := client.SubmitWorkflow(context.Background(), &Workflow{}, nil)
	if err == nil {
		t.Fatal("expected error on HTTP 500")
	}
	if !strings.Contains(err.Error(), "submit workflow error") ||
		!strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("expected wrapped HTTP 500 error, got %v", err)
	}
}

////////////////////////////////////////////////////////////////////////////////
/// Get / List / Delete
////////////////////////////////////////////////////////////////////////////////

func TestGetWorkflowHTTP(t *testing.T) {
	fa, client := newFakeArgo(t)
	fa.setResponse(http.StatusOK, sampleWorkflowJSON)
	defer func() { _ = client.Close() }()

	wf, err := client.GetWorkflow(context.Background(), "hello-world-abc123", "")
	if err != nil {
		t.Fatalf("GetWorkflow returned error: %v", err)
	}
	if wf.Metadata.Name != "hello-world-abc123" {
		t.Errorf("unexpected workflow name %s", wf.Metadata.Name)
	}
	if rec := fa.request(); rec.Method != http.MethodGet ||
		rec.Path != "/api/v1/workflows/default/hello-world-abc123" {
		t.Errorf("unexpected request %+v", rec)
	}
}

func TestGetWorkflowNamespaceOverride(t *testing.T) {
	fa, client := newFakeArgo(t)
	fa.setResponse(http.StatusOK, sampleWorkflowJSON)
	defer func() { _ = client.Close() }()

	if _, err := client.GetWorkflow(context.Background(), "wf", "other-ns"); err != nil {
		t.Fatalf("GetWorkflow returned error: %v", err)
	}
	if rec := fa.request(); !strings.HasSuffix(rec.Path, "/other-ns/wf") {
		t.Errorf("expected namespace override in path, got %s", rec.Path)
	}
}

func TestListWorkflowsHTTP(t *testing.T) {
	fa, client := newFakeArgo(t)
	fa.setResponse(http.StatusOK, sampleWorkflowListJSON())
	defer func() { _ = client.Close() }()

	list, err := client.ListWorkflows(context.Background(), &ListOptions{
		Namespace:     "prod",
		LabelSelector: "workflows.argoproj.io/phase=Running",
		FieldSelector: "metadata.name!=wf-1",
		Limit:         10,
		Offset:        5,
	})
	if err != nil {
		t.Fatalf("ListWorkflows returned error: %v", err)
	}
	if len(list.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(list.Items))
	}

	rec := fa.request()
	if rec.Path != "/api/v1/workflows/prod" {
		t.Errorf("expected prod namespace path, got %s", rec.Path)
	}
	for _, want := range []string{"labelSelector=", "fieldSelector=", "limit=10", "offset=5"} {
		if !strings.Contains(rec.Query, want) {
			t.Errorf("expected query to contain %q, got %q", want, rec.Query)
		}
	}
}

func TestListWorkflowsNilOptions(t *testing.T) {
	fa, client := newFakeArgo(t)
	fa.setResponse(http.StatusOK, sampleWorkflowListJSON())
	defer func() { _ = client.Close() }()

	if _, err := client.ListWorkflows(context.Background(), nil); err != nil {
		t.Fatalf("ListWorkflows returned error: %v", err)
	}
	rec := fa.request()
	if rec.Path != "/api/v1/workflows/default" || rec.Query != "" {
		t.Errorf("unexpected request %+v", rec)
	}
}

func TestDeleteWorkflowHTTP(t *testing.T) {
	fa, client := newFakeArgo(t)
	fa.setResponse(http.StatusOK, "{}")
	defer func() { _ = client.Close() }()

	if err := client.DeleteWorkflow(context.Background(), "wf-1", ""); err != nil {
		t.Fatalf("DeleteWorkflow returned error: %v", err)
	}
	if rec := fa.request(); rec.Method != http.MethodDelete ||
		rec.Path != "/api/v1/workflows/default/wf-1" {
		t.Errorf("unexpected request %+v", rec)
	}

	fa.setResponse(http.StatusNotFound, "not found")
	if err := client.DeleteWorkflow(context.Background(), "wf-1", ""); err == nil {
		t.Error("expected error on HTTP 404 delete")
	}
}

////////////////////////////////////////////////////////////////////////////////
/// Lifecycle operations
////////////////////////////////////////////////////////////////////////////////

func TestWorkflowLifecycleOperations(t *testing.T) {
	tests := []struct {
		name       string
		call       func(c *WorkflowClient) error
		wantMethod string
		wantSuffix string
	}{
		{
			name:       "suspend",
			call:       func(c *WorkflowClient) error { return c.SuspendWorkflow(context.Background(), "wf", "") },
			wantMethod: http.MethodPut,
			wantSuffix: "/suspend",
		},
		{
			name:       "resume",
			call:       func(c *WorkflowClient) error { return c.ResumeWorkflow(context.Background(), "wf", "") },
			wantMethod: http.MethodPut,
			wantSuffix: "/resume",
		},
		{
			name:       "terminate",
			call:       func(c *WorkflowClient) error { return c.TerminateWorkflow(context.Background(), "wf", "ns2") },
			wantMethod: http.MethodPut,
			wantSuffix: "/ns2/wf/terminate",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fa, client := newFakeArgo(t)
			fa.setResponse(http.StatusOK, "{}")
			defer func() { _ = client.Close() }()

			if err := tt.call(client); err != nil {
				t.Fatalf("%s returned error: %v", tt.name, err)
			}
			rec := fa.request()
			if rec.Method != tt.wantMethod {
				t.Errorf("expected %s, got %s", tt.wantMethod, rec.Method)
			}
			if !strings.HasSuffix(rec.Path, tt.wantSuffix) {
				t.Errorf("expected path suffix %s, got %s", tt.wantSuffix, rec.Path)
			}
		})
	}
}

func TestStopWorkflowWithMessage(t *testing.T) {
	fa, client := newFakeArgo(t)
	fa.setResponse(http.StatusOK, "{}")
	defer func() { _ = client.Close() }()

	if err := client.StopWorkflow(context.Background(), "wf", "", "shutting down"); err != nil {
		t.Fatalf("StopWorkflow returned error: %v", err)
	}
	rec := fa.request()
	if !strings.HasSuffix(rec.Path, "/stop") {
		t.Errorf("expected stop path, got %s", rec.Path)
	}
	if !strings.Contains(rec.Body, "shutting down") {
		t.Errorf("expected message in body, got %s", rec.Body)
	}
}

func TestResubmitAndRetryWorkflow(t *testing.T) {
	fa, client := newFakeArgo(t)
	fa.setResponse(http.StatusOK, sampleWorkflowJSON)
	defer func() { _ = client.Close() }()

	if _, err := client.ResubmitWorkflow(context.Background(), "wf", ""); err != nil {
		t.Fatalf("ResubmitWorkflow returned error: %v", err)
	}
	if rec := fa.request(); !strings.HasSuffix(rec.Path, "/resubmit") || rec.Method != http.MethodPut {
		t.Errorf("unexpected resubmit request %+v", rec)
	}

	if _, err := client.RetryWorkflow(context.Background(), "wf", ""); err != nil {
		t.Fatalf("RetryWorkflow returned error: %v", err)
	}
	if rec := fa.request(); !strings.HasSuffix(rec.Path, "/retry") || rec.Method != http.MethodPut {
		t.Errorf("unexpected retry request %+v", rec)
	}

	// Error path
	fa.setResponse(http.StatusBadRequest, "bad request")
	if _, err := client.RetryWorkflow(context.Background(), "wf", ""); err == nil {
		t.Error("expected error on HTTP 400 retry")
	}
}

////////////////////////////////////////////////////////////////////////////////
/// Logs
////////////////////////////////////////////////////////////////////////////////

func TestGetWorkflowLogsHTTP(t *testing.T) {
	fa, client := newFakeArgo(t)
	fa.setResponse(http.StatusOK, "log line 1\nlog line 2\n")
	defer func() { _ = client.Close() }()

	logs, err := client.GetWorkflowLogs(context.Background(), "wf", "", "main")
	if err != nil {
		t.Fatalf("GetWorkflowLogs returned error: %v", err)
	}
	if !strings.Contains(logs, "log line 2") {
		t.Errorf("expected log content, got %q", logs)
	}
	rec := fa.request()
	if !strings.HasSuffix(rec.Path, "/wf/log") || !strings.Contains(rec.Query, "podName=main") {
		t.Errorf("unexpected log request %+v", rec)
	}
}

func TestGetWorkflowLogsNoPodName(t *testing.T) {
	fa, client := newFakeArgo(t)
	fa.setResponse(http.StatusOK, "all pods")
	defer func() { _ = client.Close() }()

	logs, err := client.GetWorkflowLogs(context.Background(), "wf", "", "")
	if err != nil {
		t.Fatalf("GetWorkflowLogs returned error: %v", err)
	}
	if logs != "all pods" {
		t.Errorf("unexpected logs %q", logs)
	}
	if rec := fa.request(); rec.Query != "" {
		t.Errorf("expected no query for empty podName, got %s", rec.Query)
	}
}

func TestGetWorkflowLogsServerErrorIsNotReturnedAsLogs(t *testing.T) {
	fa, client := newFakeArgo(t)
	fa.setResponse(http.StatusInternalServerError, "internal error text")
	defer func() { _ = client.Close() }()

	logs, err := client.GetWorkflowLogs(context.Background(), "wf", "", "")
	if err == nil {
		t.Fatal("expected error on HTTP 500 logs request")
	}
	if logs != "" {
		t.Errorf("error response body must not be returned as logs, got %q", logs)
	}
	if !strings.Contains(err.Error(), "internal error text") {
		t.Errorf("expected error to include response body, got %v", err)
	}
}

////////////////////////////////////////////////////////////////////////////////
/// Response / transport error handling
////////////////////////////////////////////////////////////////////////////////

func TestGetWorkflowInvalidJSON(t *testing.T) {
	fa, client := newFakeArgo(t)
	fa.failJSON = true
	defer func() { _ = client.Close() }()

	_, err := client.GetWorkflow(context.Background(), "wf", "")
	if err == nil {
		t.Fatal("expected unmarshal error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "unmarshal response error") {
		t.Errorf("expected unmarshal error, got %v", err)
	}
}

func TestRequestConnectionError(t *testing.T) {
	// Server that is immediately closed: every request fails at transport level.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	client, err := NewClient(ClientOptions{ServerURL: srv.URL})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	defer func() { _ = client.Close() }()

	if _, err := client.GetWorkflow(context.Background(), "wf", ""); err == nil {
		t.Fatal("expected transport error for unreachable server")
	}
}

func TestNewRequestErrors(t *testing.T) {
	client, err := NewClient(ClientOptions{ServerURL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	defer func() { _ = client.Close() }()

	// Invalid HTTP method
	if _, err := client.newRequest(context.Background(), "bad method", "/x", nil); err == nil {
		t.Error("expected error for invalid HTTP method")
	}

	// Unmarshalable body
	if _, err := client.newRequest(context.Background(), http.MethodPost, "/x", make(chan int)); err == nil {
		t.Error("expected marshal error for unmarshalable body")
	}
}

func TestTokenHeader(t *testing.T) {
	fa, client := newFakeArgo(t)
	fa.setResponse(http.StatusOK, sampleWorkflowJSON)
	defer func() { _ = client.Close() }()

	// Rebuild the client with a token.
	authed, err := NewClient(ClientOptions{ServerURL: fa.srv.URL, Token: "secret-token"})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	defer func() { _ = authed.Close() }()

	if _, err := authed.GetWorkflow(context.Background(), "wf", ""); err != nil {
		t.Fatalf("GetWorkflow returned error: %v", err)
	}
	if rec := fa.request(); rec.Token != "Bearer secret-token" {
		t.Errorf("expected bearer token header, got %q", rec.Token)
	}
}

func TestCloseMarksClientStopped(t *testing.T) {
	client, err := NewClient(ClientOptions{})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	client.mu.RLock()
	running := client.running
	client.mu.RUnlock()
	if !running {
		t.Error("expected client to be running after NewClient")
	}
	if err := client.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	client.mu.RLock()
	running = client.running
	client.mu.RUnlock()
	if running {
		t.Error("expected client to be stopped after Close")
	}
}

func TestServerURLTrailingSlash(t *testing.T) {
	client, err := NewClient(ClientOptions{ServerURL: "http://127.0.0.1:2746/"})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	defer func() { _ = client.Close() }()
	if client.baseURL != "http://127.0.0.1:2746" {
		t.Errorf("expected trailing slash trimmed, got %s", client.baseURL)
	}
}

////////////////////////////////////////////////////////////////////////////////
/// Logging helpers
////////////////////////////////////////////////////////////////////////////////

// recordingLogger implements log.Logger and records emitted messages.
type recordingLogger struct {
	mu     sync.Mutex
	levels map[string][]string
	msgs   []string
}

func newRecordingLogger() *recordingLogger {
	return &recordingLogger{levels: make(map[string][]string)}
}

func (l *recordingLogger) record(level, msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.levels[level] = append(l.levels[level], msg)
	l.msgs = append(l.msgs, msg)
}

func (l *recordingLogger) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.msgs)
}

func (l *recordingLogger) Debug(_ context.Context, msg string, _ ...any) { l.record("debug", msg) }
func (l *recordingLogger) Info(_ context.Context, msg string, _ ...any)  { l.record("info", msg) }
func (l *recordingLogger) Warn(_ context.Context, msg string, _ ...any)  { l.record("warn", msg) }
func (l *recordingLogger) Error(_ context.Context, msg string, _ ...any) { l.record("error", msg) }

func (l *recordingLogger) Enabled(_ log.Level) bool { return true }
func (l *recordingLogger) With(_ ...any) log.Logger { return l }

func TestLogHelpers(t *testing.T) {
	lg := newRecordingLogger()
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

	if lg.count() != 10 {
		t.Errorf("expected 10 log records, got %d", lg.count())
	}
	for _, m := range lg.msgs {
		if !strings.HasPrefix(m, logKey+" ") {
			t.Errorf("expected message %q to be prefixed with %q", m, logKey)
		}
	}

	// A non-nil logger injection must survive a read.
	if getLogger() == nil {
		t.Error("expected injected logger to be returned by getLogger")
	}
}

func TestSetLoggerNilRestoresDefault(t *testing.T) {
	SetLogger(newRecordingLogger())
	SetLogger(nil)
	// Falls back to the framework logger; must not panic.
	LogInfo("after reset")
	if getLogger() == nil {
		t.Error("expected fallback logger from framework")
	}
}
