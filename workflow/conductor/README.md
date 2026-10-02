# Conductor Workflow

Conductor workflow module for [go-wind-plugins](https://github.com/tx7do/go-wind-plugins), based on [conductor-oss/conductor](https://github.com/conductor-oss/conductor) (Netflix Conductor).

## Overview

[Conductor](https://conductor-oss.github.io/conductor/) is an event-driven orchestration platform providing durable and highly resilient execution for microservices.

This module provides a Go wrapper around the [Conductor Go SDK](https://github.com/conductor-oss/go-sdk), exposing native Conductor concepts (Workflow, Task, Worker) without any broker abstraction.

## Installation

```bash
go get github.com/tx7do/go-wind-plugins/workflow/conductor
```

## Quick Start

### 1. Create a Client

```go
import conductor "github.com/tx7do/go-wind-plugins/workflow/conductor"

// Connect to a local Conductor server
client, err := conductor.NewClient(conductor.ClientOptions{
    ServerURL: "http://localhost:8080/api",
})
if err != nil {
    log.Fatal(err)
}
defer func() { _ = client.Close() }()

// Or from environment variables: CONDUCTOR_SERVER_URL, CONDUCTOR_AUTH_KEY, CONDUCTOR_AUTH_SECRET
client, err = conductor.NewClientFromEnv()
```

### 2. Start a Workflow

```go
import "context"

// Async start
id, err := client.StartWorkflow(context.Background(), conductor.StartWorkflowOptions{
    Name: "my_workflow",
    Input: map[string]interface{}{
        "name": "Gopher",
    },
})

// Sync start (wait until a specific task completes)
run, err := client.StartWorkflowSync(context.Background(), conductor.StartWorkflowOptions{
    Name: "my_workflow",
    Input: map[string]interface{}{
        "name": "Gopher",
    },
}, "")
```

### 3. Define a Task Worker

```go
import "github.com/conductor-sdk/conductor-go/sdk/model"

// Define your task handler
func GreetTask(task *model.Task) (interface{}, error) {
    name := task.InputData["name"]
    return map[string]interface{}{
        "greeting": "Hello, " + name.(string),
    }, nil
}

// Start the worker
worker, err := client.StartWorker("greet", GreetTask, 1, 100*time.Millisecond)
```

### 4. Manage Workflows

```go
// Get workflow status
workflow, err := client.GetWorkflow(ctx, workflowID, true)

// Monitor execution asynchronously
ch, err := client.MonitorExecution(workflowID)
for run := range ch {
    fmt.Printf("status: %s\n", run.Status)
}

// Pause / Resume
client.Pause(ctx, workflowID)
client.Resume(ctx, workflowID)

// Terminate
client.Terminate(ctx, workflowID, "no longer needed")

// Retry failed workflow
client.Retry(ctx, workflowID, false)

// Restart completed workflow
client.Restart(ctx, workflowID, true)
```

### 5. Task Worker with Full Configuration

```go
worker, err := client.StartWorkerWithConfig(conductor.WorkerConfig{
    TaskType:     "greet",
    Concurrency:  3,
    PollInterval: 200 * time.Millisecond,
    Domain:       "production",
}, GreetTask)
if err != nil {
    log.Fatal(err)
}

fmt.Println(worker.TaskType()) // "greet"
fmt.Println(worker.IsRunning()) // true

// Note: The Conductor Go SDK TaskRunner does not expose a direct Stop method.
// worker.Stop() marks the worker as stopped, but the underlying poller
// will stop when the process exits.
worker.Stop()
```

### 6. Access Underlying SDK

```go
// Get the raw API client for advanced operations
apiClient := client.APIClient()

// Get the workflow executor for low-level control
wfExecutor := client.WorkflowExecutor()
```

## Configuration

### ClientOptions

| Field       | Description                              | Default                      |
|-------------|------------------------------------------|------------------------------|
| ServerURL   | Conductor server API URL                 | `http://localhost:8080/api`  |
| AuthKey     | Authentication key (for Orkes Cloud)     | -                            |
| AuthSecret  | Authentication secret (for Orkes Cloud)  | -                            |

### StartWorkflowOptions

| Field          | Description                              | Default |
|----------------|------------------------------------------|---------|
| Name           | Workflow definition name                 | -       |
| Version        | Workflow definition version              | latest  |
| Input          | Workflow input data (`map[string]any`)   | -       |
| CorrelationID  | Message correlation ID                   | -       |
| Priority       | Workflow priority                        | -       |

### WorkerConfig

| Field        | Description                        | Default    |
|--------------|------------------------------------|------------|
| TaskType     | Task definition name to poll for   | -          |
| Concurrency  | Number of concurrent worker threads| `1`        |
| PollInterval | Interval between poll requests     | `100ms`    |
| Domain       | Task domain for isolation           | -          |

## API Reference

### WorkflowClient Methods

| Method | Description |
|--------|-------------|
| `NewClient(opts)` | Create a client connected to Conductor server |
| `NewClientFromEnv(opts...)` | Create a client from environment variables; non-empty `ClientOptions` fields override the env values |
| `StartWorkflow(ctx, opts)` | Start a workflow asynchronously, returns instance ID |
| `StartWorkflowSync(ctx, opts, waitUntilTask)` | Start and block until a task completes |
| `MonitorExecution(workflowID)` | Get a channel for async workflow result monitoring |
| `GetWorkflow(ctx, id, includeTasks)` | Retrieve current workflow state |
| `Terminate(ctx, id, reason)` | Terminate a running workflow |
| `Pause(ctx, id)` | Pause an ongoing workflow |
| `Resume(ctx, id)` | Resume a paused workflow |
| `Retry(ctx, id, resumeSubwf)` | Retry from the last failed task |
| `Restart(ctx, id, useLatestDef)` | Restart from the beginning |
| `StartWorker(taskType, handler, concurrency, interval)` | Start a simple task worker |
| `StartWorkerWithConfig(config, handler)` | Start a task worker with full config |
| `APIClient()` | Get the underlying Conductor `APIClient` |
| `WorkflowExecutor()` | Get the underlying `WorkflowExecutor` |
| `Close()` | Close the client |

### TaskWorker Methods

| Method | Description |
|--------|-------------|
| `Stop()` | Mark the worker as stopped (see note below) |
| `TaskType()` | Get the task type this worker polls for |
| `IsRunning()` | Check if the worker is still active |

> **Note on Stop():** The Conductor Go SDK `TaskRunner` does not expose a direct stop method. `Stop()` marks the internal state as stopped, but the underlying poller goroutine will continue until the process exits. For graceful shutdown, consider running workers in a separate goroutine and using process-level signals.

## Architecture

Unlike message brokers (Kafka, RabbitMQ, etc.), Conductor is a **workflow orchestration engine**. It is not suitable for the `broker.Broker` interface. This module lives under `workflow/conductor` alongside `workflow/temporal`, `workflow/argo`, and `workflow/goworkflows`. All four implement the `workflow.Client` interface (`Close() error`).

## References

- [Conductor Documentation](https://conductor-oss.github.io/conductor/)
- [Conductor Go SDK](https://github.com/conductor-oss/go-sdk)
- [Conductor Go SDK Examples](https://github.com/conductor-oss/go-sdk/tree/main/examples)
