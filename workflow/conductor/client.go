package conductor

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/antihax/optional"
	"github.com/conductor-sdk/conductor-go/sdk/client"
	"github.com/conductor-sdk/conductor-go/sdk/model"
	"github.com/conductor-sdk/conductor-go/sdk/settings"
	"github.com/conductor-sdk/conductor-go/sdk/workflow/executor"
)

// WorkflowClient provides high-level access to Conductor workflow operations.
type WorkflowClient struct {
	mu               sync.RWMutex
	apiClient        *client.APIClient
	workflowExecutor *executor.WorkflowExecutor
	workflowClient   client.WorkflowClient
	options          ClientOptions
	running          bool
}

// NewClient creates a new WorkflowClient and connects to the Conductor server.
func NewClient(opts ClientOptions) (*WorkflowClient, error) {
	if opts.ServerURL == "" {
		opts.ServerURL = defaultServerURL
	}

	var authSettings *settings.AuthenticationSettings
	if opts.AuthKey != "" && opts.AuthSecret != "" {
		authSettings = settings.NewAuthenticationSettings(opts.AuthKey, opts.AuthSecret)
	}

	httpSettings := settings.NewHttpSettings(opts.ServerURL)

	var apiClient *client.APIClient
	if authSettings != nil {
		apiClient = client.NewAPIClient(authSettings, httpSettings)
	} else {
		apiClient = client.NewAPIClient(nil, httpSettings)
	}

	workflowExecutor := executor.NewWorkflowExecutor(apiClient)
	workflowClient := client.NewWorkflowClient(apiClient)

	LogInfof("connected to Conductor server at %s", opts.ServerURL)

	return &WorkflowClient{
		apiClient:        apiClient,
		workflowExecutor: workflowExecutor,
		workflowClient:   workflowClient,
		options:          opts,
		running:          true,
	}, nil
}

// NewClientFromEnv creates a WorkflowClient from environment variables:
// CONDUCTOR_SERVER_URL, CONDUCTOR_AUTH_KEY, CONDUCTOR_AUTH_SECRET.
//
// Convention: the environment provides the defaults, and any non-empty field
// in the given ClientOptions overrides the corresponding environment value.
// Called without options it behaves as the pure environment-driven
// constructor, and it routes through NewClient so the default-ServerURL
// fallback applies when neither env nor options supply a URL.
func NewClientFromEnv(opts ...ClientOptions) (*WorkflowClient, error) {
	merged := ClientOptions{
		ServerURL:  os.Getenv(settings.EnvServerURL),
		AuthKey:    os.Getenv(settings.EnvAuthKey),
		AuthSecret: os.Getenv(settings.EnvAuthSecret),
	}
	for _, o := range opts {
		if o.ServerURL != "" {
			merged.ServerURL = o.ServerURL
		}
		if o.AuthKey != "" {
			merged.AuthKey = o.AuthKey
		}
		if o.AuthSecret != "" {
			merged.AuthSecret = o.AuthSecret
		}
	}

	LogInfo("connected to Conductor server from environment")

	return NewClient(merged)
}

// APIClient returns the underlying Conductor API client for advanced operations.
func (wc *WorkflowClient) APIClient() *client.APIClient {
	return wc.apiClient
}

// WorkflowExecutor returns the underlying workflow executor.
func (wc *WorkflowClient) WorkflowExecutor() *executor.WorkflowExecutor {
	return wc.workflowExecutor
}

// Close cleans up resources.
func (wc *WorkflowClient) Close() error {
	wc.mu.Lock()
	defer wc.mu.Unlock()
	wc.running = false
	LogInfo("disconnected from Conductor server")
	return nil
}

// StartWorkflow starts a new workflow execution asynchronously.
// Returns the workflow instance ID.
func (wc *WorkflowClient) StartWorkflow(ctx context.Context, opts StartWorkflowOptions) (string, error) {
	if wc.workflowExecutor == nil {
		return "", fmt.Errorf("workflow executor is nil")
	}

	req := toStartWorkflowRequest(opts)
	id, err := wc.workflowExecutor.StartWorkflowWithContext(ctx, req)
	if err != nil {
		return "", fmt.Errorf("start workflow error: %w", err)
	}

	LogInfof("started workflow %s with id: %s", opts.Name, id)
	return id, nil
}

// StartWorkflowSync starts a workflow and blocks until it completes or the specified task finishes.
// Returns the workflow execution result.
func (wc *WorkflowClient) StartWorkflowSync(ctx context.Context, opts StartWorkflowOptions, waitUntilTask string) (*model.WorkflowRun, error) {
	if wc.workflowExecutor == nil {
		return nil, fmt.Errorf("workflow executor is nil")
	}

	req := toStartWorkflowRequest(opts)
	run, err := wc.workflowExecutor.ExecuteWorkflowWithContext(ctx, req, waitUntilTask)
	if err != nil {
		return nil, fmt.Errorf("execute workflow error: %w", err)
	}

	return run, nil
}

// MonitorExecution returns a channel that receives the workflow result when it completes.
func (wc *WorkflowClient) MonitorExecution(workflowID string) (executor.WorkflowExecutionChannel, error) {
	if wc.workflowExecutor == nil {
		return nil, fmt.Errorf("workflow executor is nil")
	}

	return wc.workflowExecutor.MonitorExecution(workflowID)
}

// GetWorkflow retrieves the current state of a workflow execution.
func (wc *WorkflowClient) GetWorkflow(ctx context.Context, workflowID string, includeTasks bool) (*model.Workflow, error) {
	if wc.workflowClient == nil {
		return nil, fmt.Errorf("workflow client is nil")
	}

	opts := &client.WorkflowResourceApiGetExecutionStatusOpts{
		IncludeTasks: optional.NewBool(includeTasks),
	}
	workflow, _, err := wc.workflowClient.GetExecutionStatus(ctx, workflowID, opts)
	if err != nil {
		return nil, fmt.Errorf("get workflow error: %w", err)
	}

	return &workflow, nil
}

// Terminate terminates a running workflow.
func (wc *WorkflowClient) Terminate(ctx context.Context, workflowID, reason string) error {
	if wc.workflowClient == nil {
		return fmt.Errorf("workflow client is nil")
	}

	terminateOpts := &client.WorkflowResourceApiTerminateOpts{
		Reason:                 optional.NewString(reason),
		TriggerFailureWorkflow: optional.NewBool(false),
	}
	_, err := wc.workflowClient.Terminate(ctx, workflowID, terminateOpts)
	if err != nil {
		return fmt.Errorf("terminate workflow error: %w", err)
	}

	LogInfof("terminated workflow %s", workflowID)
	return nil
}

// Pause pauses an ongoing workflow execution.
func (wc *WorkflowClient) Pause(ctx context.Context, workflowID string) error {
	if wc.workflowClient == nil {
		return fmt.Errorf("workflow client is nil")
	}

	_, err := wc.workflowClient.Pause(ctx, workflowID)
	if err != nil {
		return fmt.Errorf("pause workflow error: %w", err)
	}

	LogInfof("paused workflow %s", workflowID)
	return nil
}

// Resume resumes a paused workflow execution.
func (wc *WorkflowClient) Resume(ctx context.Context, workflowID string) error {
	if wc.workflowClient == nil {
		return fmt.Errorf("workflow client is nil")
	}

	_, err := wc.workflowClient.Resume(ctx, workflowID)
	if err != nil {
		return fmt.Errorf("resume workflow error: %w", err)
	}

	LogInfof("resumed workflow %s", workflowID)
	return nil
}

// Restart restarts a terminal workflow execution from the beginning.
func (wc *WorkflowClient) Restart(ctx context.Context, workflowID string, useLatestDef bool) error {
	if wc.workflowClient == nil {
		return fmt.Errorf("workflow client is nil")
	}

	restartOpts := &client.WorkflowResourceApiRestartOpts{
		UseLatestDefinitions: optional.NewBool(useLatestDef),
	}
	_, err := wc.workflowClient.Restart(ctx, workflowID, restartOpts)
	if err != nil {
		return fmt.Errorf("restart workflow error: %w", err)
	}

	LogInfof("restarted workflow %s", workflowID)
	return nil
}

// Retry retries a failed workflow from the last failed task.
func (wc *WorkflowClient) Retry(ctx context.Context, workflowID string, resumeSubworkflowTasks bool) error {
	if wc.workflowClient == nil {
		return fmt.Errorf("workflow client is nil")
	}

	retryOpts := &client.WorkflowResourceApiRetryOpts{
		ResumeSubworkflowTasks: optional.NewBool(resumeSubworkflowTasks),
	}
	_, err := wc.workflowClient.Retry(ctx, workflowID, retryOpts)
	if err != nil {
		return fmt.Errorf("retry workflow error: %w", err)
	}

	LogInfof("retried workflow %s", workflowID)
	return nil
}
