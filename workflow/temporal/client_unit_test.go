package temporal

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"

	enumspb "go.temporal.io/api/enums/v1"
	workflowservice "go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/converter"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"

	"github.com/tx7do/go-wind/log"
)

////////////////////////////////////////////////////////////////////////////////
/// Mock Temporal SDK client
////////////////////////////////////////////////////////////////////////////////

// stubWorkflowRun is a client.WorkflowRun implementation with canned results.
type stubWorkflowRun struct {
	runID   string
	wfID    string
	result  []byte
	getErr  error
	gotPtr  any
	getCall bool
}

func (r *stubWorkflowRun) GetID() string { return r.wfID }

func (r *stubWorkflowRun) GetRunID() string { return r.runID }

func (r *stubWorkflowRun) Get(ctx context.Context, valuePtr interface{}) error {
	r.getCall = true
	r.gotPtr = valuePtr
	if r.getErr != nil {
		return r.getErr
	}
	if p, ok := valuePtr.(*[]byte); ok && r.result != nil {
		*p = r.result
	}
	return nil
}

func (r *stubWorkflowRun) GetWithOptions(ctx context.Context, valuePtr interface{}, options client.WorkflowRunGetOptions) error {
	return r.Get(ctx, valuePtr)
}

// mockTemporalClient implements the parts of client.Client exercised by the
// wrapper; everything else is inherited (and panics if ever called).
type mockTemporalClient struct {
	client.Client

	mu              sync.Mutex
	calls           []string
	executeErr      error
	run             *stubWorkflowRun
	lastOptions     client.StartWorkflowOptions
	lastWorkflowFn  any
	signalErr       error
	queryValue      converter.EncodedValue
	queryErr        error
	cancelErr       error
	describeErr     error
	lastSignalName  string
	lastQueryType   string
	lastWorkflowIDs []string
	canceled        bool
}

func newMockTemporalClient() *mockTemporalClient {
	return &mockTemporalClient{
		run: &stubWorkflowRun{runID: "run-123", wfID: "wf-123", result: []byte("done")},
	}
}

func (m *mockTemporalClient) record(call string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, call)
}

func (m *mockTemporalClient) ExecuteWorkflow(ctx context.Context, options client.StartWorkflowOptions, workflowFunc interface{}, args ...interface{}) (client.WorkflowRun, error) {
	m.record("ExecuteWorkflow")
	m.mu.Lock()
	m.lastOptions = options
	m.lastWorkflowFn = workflowFunc
	m.mu.Unlock()
	if m.executeErr != nil {
		return nil, m.executeErr
	}
	return m.run, nil
}

func (m *mockTemporalClient) SignalWorkflow(ctx context.Context, workflowID string, runID string, signalName string, arg interface{}) error {
	m.record("SignalWorkflow")
	m.mu.Lock()
	m.lastSignalName = signalName
	m.lastWorkflowIDs = append(m.lastWorkflowIDs, workflowID)
	m.mu.Unlock()
	return m.signalErr
}

func (m *mockTemporalClient) QueryWorkflow(ctx context.Context, workflowID string, runID string, queryType string, args ...interface{}) (converter.EncodedValue, error) {
	m.record("QueryWorkflow")
	m.mu.Lock()
	m.lastQueryType = queryType
	m.mu.Unlock()
	return m.queryValue, m.queryErr
}

func (m *mockTemporalClient) CancelWorkflow(ctx context.Context, workflowID string, runID string) error {
	m.record("CancelWorkflow")
	m.mu.Lock()
	m.canceled = true
	m.mu.Unlock()
	return m.cancelErr
}

func (m *mockTemporalClient) DescribeWorkflowExecution(ctx context.Context, workflowID, runID string) (*workflowservice.DescribeWorkflowExecutionResponse, error) {
	m.record("DescribeWorkflowExecution")
	return &workflowservice.DescribeWorkflowExecutionResponse{}, m.describeErr
}

func newMockWorkflowClient(mc *mockTemporalClient) *WorkflowClient {
	return &WorkflowClient{client: mc}
}

////////////////////////////////////////////////////////////////////////////////
/// Nil-client guard paths (no server reachable in tests)
////////////////////////////////////////////////////////////////////////////////

func TestWorkflowClientNilClientErrors(t *testing.T) {
	wc := &WorkflowClient{}

	_, err := wc.Execute(context.Background(), nil, ExecuteOptions{})
	assert.ErrorContains(t, err, "temporal client is not connected")

	_, err = wc.ExecuteSync(context.Background(), nil, ExecuteOptions{})
	assert.ErrorContains(t, err, "temporal client is not connected")

	assert.ErrorContains(t, wc.Signal(context.Background(), "wf", "run", "sig", nil), "temporal client is not connected")

	_, err = wc.Query(context.Background(), "wf", "run", "query", nil)
	assert.ErrorContains(t, err, "temporal client is not connected")

	assert.ErrorContains(t, wc.Cancel(context.Background(), "wf", "run"), "temporal client is not connected")
	assert.ErrorContains(t, wc.Describe(context.Background(), "wf", "run"), "temporal client is not connected")

	_, err = wc.NewWorker(WorkerOptions{})
	assert.ErrorContains(t, err, "temporal client is not connected")

	_, err = wc.StartSimpleWorker(context.Background(), "tq", func(ctx context.Context, body []byte) error { return nil })
	assert.ErrorContains(t, err, "temporal client is not connected")
}

////////////////////////////////////////////////////////////////////////////////
/// Execute / ExecuteSync
////////////////////////////////////////////////////////////////////////////////

func TestExecuteSuccess(t *testing.T) {
	mc := newMockTemporalClient()
	wc := newMockWorkflowClient(mc)

	runID, err := wc.Execute(context.Background(), map[string]any{"k": "v"}, ExecuteOptions{
		TaskQueue:   "tq",
		WorkflowID:  "wf-1",
		WorkflowFn:  BrokerMessageWorkflow,
		RunTimeout:  time.Minute,
		TaskTimeout: time.Second * 30,
	})
	require.NoError(t, err)
	assert.Equal(t, "run-123", runID)

	mc.mu.Lock()
	defer mc.mu.Unlock()
	assert.Equal(t, []string{"ExecuteWorkflow"}, mc.calls)
	assert.Equal(t, "tq", mc.lastOptions.TaskQueue)
	assert.Equal(t, "wf-1", mc.lastOptions.ID)
	// Custom workflow function must be passed through.
	assert.NotNil(t, mc.lastWorkflowFn)
}

func TestExecuteUsesDefaultWorkflowFn(t *testing.T) {
	mc := newMockTemporalClient()
	wc := newMockWorkflowClient(mc)

	_, err := wc.Execute(context.Background(), nil, ExecuteOptions{TaskQueue: "tq"})
	require.NoError(t, err)

	mc.mu.Lock()
	defer mc.mu.Unlock()
	// nil WorkflowFn must fall back to BrokerMessageWorkflow
	assert.NotNil(t, mc.lastWorkflowFn)
}

func TestExecuteError(t *testing.T) {
	mc := newMockTemporalClient()
	mc.executeErr = errors.New("server unavailable")
	wc := newMockWorkflowClient(mc)

	runID, err := wc.Execute(context.Background(), nil, ExecuteOptions{})
	assert.Empty(t, runID)
	assert.ErrorContains(t, err, "execute workflow error")
	assert.ErrorContains(t, err, "server unavailable")
}

func TestExecuteSyncSuccess(t *testing.T) {
	mc := newMockTemporalClient()
	wc := newMockWorkflowClient(mc)

	result, err := wc.ExecuteSync(context.Background(), "payload", ExecuteOptions{TaskQueue: "tq"})
	require.NoError(t, err)
	assert.Equal(t, []byte("done"), result)
	assert.True(t, mc.run.getCall, "expected workflow result to be fetched")
}

func TestExecuteSyncExecuteError(t *testing.T) {
	mc := newMockTemporalClient()
	mc.executeErr = errors.New("start failed")
	wc := newMockWorkflowClient(mc)

	result, err := wc.ExecuteSync(context.Background(), nil, ExecuteOptions{})
	assert.Nil(t, result)
	assert.ErrorContains(t, err, "execute workflow error")
}

func TestExecuteSyncGetError(t *testing.T) {
	mc := newMockTemporalClient()
	mc.run.getErr = errors.New("workflow failed")
	wc := newMockWorkflowClient(mc)

	result, err := wc.ExecuteSync(context.Background(), nil, ExecuteOptions{})
	assert.Nil(t, result)
	assert.ErrorContains(t, err, "get workflow result error")
}

////////////////////////////////////////////////////////////////////////////////
/// Signal / Query / Cancel / Describe
////////////////////////////////////////////////////////////////////////////////

func TestSignalWorkflow(t *testing.T) {
	mc := newMockTemporalClient()
	wc := newMockWorkflowClient(mc)

	assert.NoError(t, wc.Signal(context.Background(), "wf-1", "run-1", "wake-up", "arg"))
	mc.mu.Lock()
	assert.Equal(t, "wake-up", mc.lastSignalName)
	mc.mu.Unlock()

	mc.signalErr = errors.New("not found")
	assert.ErrorContains(t, wc.Signal(context.Background(), "wf", "", "sig", nil), "not found")
}

func TestQueryWorkflow(t *testing.T) {
	mc := newMockTemporalClient()
	wc := newMockWorkflowClient(mc)

	res, err := wc.Query(context.Background(), "wf-1", "run-1", "state", nil)
	require.NoError(t, err)
	assert.Nil(t, res)
	mc.mu.Lock()
	assert.Equal(t, "state", mc.lastQueryType)
	mc.mu.Unlock()

	mc.queryErr = errors.New("query failed")
	_, err = wc.Query(context.Background(), "wf", "", "state", nil)
	assert.ErrorContains(t, err, "query failed")
}

func TestCancelWorkflow(t *testing.T) {
	mc := newMockTemporalClient()
	wc := newMockWorkflowClient(mc)

	assert.NoError(t, wc.Cancel(context.Background(), "wf-1", "run-1"))
	mc.mu.Lock()
	assert.True(t, mc.canceled)
	mc.mu.Unlock()

	mc.cancelErr = errors.New("cancel failed")
	assert.ErrorContains(t, wc.Cancel(context.Background(), "wf", ""), "cancel failed")
}

func TestDescribeWorkflowExecution(t *testing.T) {
	mc := newMockTemporalClient()
	wc := newMockWorkflowClient(mc)

	assert.NoError(t, wc.Describe(context.Background(), "wf-1", "run-1"))

	mc.describeErr = errors.New("describe failed")
	assert.ErrorContains(t, wc.Describe(context.Background(), "wf", ""), "describe failed")
}

func TestTemporalClientAccessor(t *testing.T) {
	mc := newMockTemporalClient()
	wc := newMockWorkflowClient(mc)
	assert.Equal(t, client.Client(mc), wc.TemporalClient())
}

func TestCloseNilClientIsSafe(t *testing.T) {
	wc := &WorkflowClient{}
	assert.NoError(t, wc.Close())
}

////////////////////////////////////////////////////////////////////////////////
/// Tracing helpers
////////////////////////////////////////////////////////////////////////////////

func TestWithTracing(t *testing.T) {
	wc := &WorkflowClient{}
	assert.Nil(t, wc.tracer)
	wc.WithTracing()
	assert.NotNil(t, wc.tracer)

	// With a tracer configured, spans are created and ended without panic.
	ctx, span := wc.startProducerSpan(context.Background(), "topic-1")
	assert.NotNil(t, span)
	wc.finishProducerSpan(span, nil)

	_, consumerSpan := wc.startConsumerSpan(ctx, "topic-1")
	assert.NotNil(t, consumerSpan)
	wc.finishConsumerSpan(consumerSpan, errors.New("boom"))
}

func TestTracingWithoutTracerReturnsNilSpan(t *testing.T) {
	wc := &WorkflowClient{}

	ctx, span := wc.startProducerSpan(context.Background(), "topic")
	assert.NotNil(t, ctx)
	assert.Nil(t, span)
	// finish helpers must tolerate nil spans.
	wc.finishProducerSpan(span, errors.New("ignored"))
	wc.finishConsumerSpan(span, nil)
}

////////////////////////////////////////////////////////////////////////////////
/// processMessageActivity
////////////////////////////////////////////////////////////////////////////////

func TestProcessMessageActivity(t *testing.T) {
	var received [][]byte
	called := false
	handler := func(ctx context.Context, body []byte) error {
		called = true
		received = append(received, body)
		return nil
	}

	wc := newMockWorkflowClient(newMockTemporalClient())
	act := &processMessageActivity{handler: handler, client: wc, topic: "queue-1"}

	assert.NoError(t, act.ProcessMessage(context.Background(), []byte("hello")))
	assert.True(t, called)
	assert.Equal(t, [][]byte{[]byte("hello")}, received)
}

func TestProcessMessageActivityPropagatesError(t *testing.T) {
	handlerErr := errors.New("handler failed")
	handler := func(ctx context.Context, body []byte) error { return handlerErr }

	wc := &WorkflowClient{tracer: otel.Tracer("test-tracer")}
	act := &processMessageActivity{handler: handler, client: wc, topic: "queue-2"}

	assert.ErrorIs(t, act.ProcessMessage(context.Background(), nil), handlerErr)
}

func TestProcessMessageActivityNilClientTracing(t *testing.T) {
	// A zero-value WorkflowClient (no tracer) must not break the activity.
	handler := func(ctx context.Context, body []byte) error { return nil }
	act := &processMessageActivity{handler: handler, client: &WorkflowClient{}, topic: "q"}
	assert.NoError(t, act.ProcessMessage(context.Background(), []byte("x")))
}

////////////////////////////////////////////////////////////////////////////////
/// BrokerMessageWorkflow (in-memory test environment, no server)
////////////////////////////////////////////////////////////////////////////////

func TestBrokerMessageWorkflowDelegatesToProcessMessageActivity(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	var received []byte
	env.RegisterActivityWithOptions(
		func(ctx context.Context, body []byte) error {
			received = body
			return nil
		},
		activity.RegisterOptions{Name: defaultActivityName},
	)

	env.ExecuteWorkflow(BrokerMessageWorkflow, []byte("message-body"))

	// BrokerMessageWorkflow returns an error result, which serializes to no
	// payload on success; only the workflow error itself is asserted.
	require.NoError(t, env.GetWorkflowError())
	assert.Equal(t, []byte("message-body"), received)
}

func TestBrokerMessageWorkflowPropagatesActivityError(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	activityErr := errors.New("activity boom")
	env.RegisterActivityWithOptions(
		func(ctx context.Context, body []byte) error { return activityErr },
		activity.RegisterOptions{Name: defaultActivityName},
	)

	env.ExecuteWorkflow(BrokerMessageWorkflow, []byte("x"))

	assert.Error(t, env.GetWorkflowError())
}

////////////////////////////////////////////////////////////////////////////////
/// Worker lifecycle (mocked worker.Worker)
////////////////////////////////////////////////////////////////////////////////

// mockSDKWorker implements worker.Worker by embedding the interface; only the
// methods the wrapper calls are overridden.
type mockSDKWorker struct {
	worker.Worker

	mu         sync.Mutex
	started    bool
	stopped    bool
	startErr   error
	workflows  []any
	activities []any
}

func (m *mockSDKWorker) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.startErr != nil {
		return m.startErr
	}
	m.started = true
	return nil
}

func (m *mockSDKWorker) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopped = true
	m.started = false
}

func (m *mockSDKWorker) RegisterWorkflow(fn interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.workflows = append(m.workflows, fn)
}

func (m *mockSDKWorker) RegisterActivity(fn interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.activities = append(m.activities, fn)
}

func newMockWorkflowWorker() (*WorkflowWorker, *mockSDKWorker) {
	mw := &mockSDKWorker{}
	ww := &WorkflowWorker{
		client: &WorkflowClient{},
		worker: mw,
		opts:   WorkerOptions{TaskQueue: "task-queue-1"},
	}
	return ww, mw
}

func TestWorkflowWorkerStartStop(t *testing.T) {
	ww, mw := newMockWorkflowWorker()

	assert.NoError(t, ww.Start())
	assert.True(t, ww.IsRunning())
	mw.mu.Lock()
	assert.True(t, mw.started)
	mw.mu.Unlock()

	// Second start must fail.
	err := ww.Start()
	assert.ErrorContains(t, err, "worker is already started")

	ww.Stop()
	assert.False(t, ww.IsRunning())
	mw.mu.Lock()
	assert.True(t, mw.stopped)
	mw.mu.Unlock()

	// Start after stop must fail.
	assert.ErrorContains(t, ww.Start(), "worker is already stopped")
}

func TestWorkflowWorkerStartError(t *testing.T) {
	ww, mw := newMockWorkflowWorker()
	mw.startErr = errors.New("poller failure")

	err := ww.Start()
	assert.ErrorContains(t, err, "failed to start temporal worker")
	assert.ErrorContains(t, err, "poller failure")
	assert.False(t, ww.IsRunning())
}

func TestWorkflowWorkerRegistrations(t *testing.T) {
	ww, mw := newMockWorkflowWorker()

	someWorkflow := func(ctx workflow.Context) error { return nil }
	someActivity := func(ctx context.Context, body []byte) error { return nil }

	ww.RegisterWorkflow(someWorkflow)
	ww.RegisterActivity(someActivity)

	mw.mu.Lock()
	defer mw.mu.Unlock()
	// Function values are not reflect.DeepEqual-comparable; compare pointers.
	require.Len(t, mw.workflows, 1)
	require.Len(t, mw.activities, 1)
	assert.Equal(t, reflect.ValueOf(someWorkflow).Pointer(), reflect.ValueOf(mw.workflows[0]).Pointer())
	assert.Equal(t, reflect.ValueOf(someActivity).Pointer(), reflect.ValueOf(mw.activities[0]).Pointer())
}

func TestWorkflowWorkerTaskQueue(t *testing.T) {
	ww, _ := newMockWorkflowWorker()
	assert.Equal(t, "task-queue-1", ww.TaskQueue())
}

func TestWorkflowWorkerStopWithoutWorker(t *testing.T) {
	ww := &WorkflowWorker{}
	ww.Stop() // must not panic
	assert.False(t, ww.IsRunning())
}

////////////////////////////////////////////////////////////////////////////////
/// toStartWorkflowOptions edge cases
////////////////////////////////////////////////////////////////////////////////

func TestToStartWorkflowOptionsMinimal(t *testing.T) {
	swo := toStartWorkflowOptions(ExecuteOptions{})
	assert.Empty(t, swo.TaskQueue)
	assert.Empty(t, swo.ID)
	assert.Zero(t, swo.WorkflowRunTimeout)
	assert.Zero(t, swo.WorkflowExecutionTimeout)
	assert.Zero(t, swo.WorkflowTaskTimeout)
	assert.Nil(t, swo.RetryPolicy)
	assert.Empty(t, swo.CronSchedule)
}

func TestToStartWorkflowOptionsIDReusePolicy(t *testing.T) {
	policy := enumspb.WORKFLOW_ID_REUSE_POLICY_ALLOW_DUPLICATE
	swo := toStartWorkflowOptions(ExecuteOptions{IDReusePolicy: policy})
	assert.Equal(t, policy, swo.WorkflowIDReusePolicy)
}

func TestToStartWorkflowOptionsTaskTimeout(t *testing.T) {
	swo := toStartWorkflowOptions(ExecuteOptions{
		TaskTimeout:      time.Second * 42,
		ExecutionTimeout: time.Hour * 2,
	})
	assert.Equal(t, time.Second*42, swo.WorkflowTaskTimeout)
	assert.Equal(t, time.Hour*2, swo.WorkflowExecutionTimeout)
}

////////////////////////////////////////////////////////////////////////////////
/// Logging helpers
////////////////////////////////////////////////////////////////////////////////

type recordingLogger struct {
	mu   sync.Mutex
	msgs []string
}

func (l *recordingLogger) record(level, msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.msgs = append(l.msgs, msg)
}

func (l *recordingLogger) Debug(_ context.Context, msg string, _ ...any) { l.record("debug", msg) }
func (l *recordingLogger) Info(_ context.Context, msg string, _ ...any)  { l.record("info", msg) }
func (l *recordingLogger) Warn(_ context.Context, msg string, _ ...any)  { l.record("warn", msg) }
func (l *recordingLogger) Error(_ context.Context, msg string, _ ...any) { l.record("error", msg) }
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
	assert.Len(t, lg.msgs, 10)
	for _, m := range lg.msgs {
		assert.Contains(t, m, logKey)
	}
}

func TestSetLoggerNilRestoresDefault(t *testing.T) {
	SetLogger(&recordingLogger{})
	SetLogger(nil)
	LogInfo("after reset") // must not panic
	assert.NotNil(t, getLogger())
}
