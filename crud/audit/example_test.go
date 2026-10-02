package audit_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/crud/audit"
)

// ExampleMustFromContext retrieves the auditor that the platform middleware
// bound to the request context and records an audit entry from the data
// access layer. SetPostValue captures the post-image of the changed row so
// the audit trail reflects what was written, and Flush commits any buffered
// entries when the request completes.
func ExampleMustFromContext() {
	ctx := audit.WithAuditor(context.Background(), audit.NewNoopAuditor())

	a := audit.MustFromContext(ctx)

	entry := &audit.Entry{
		TraceID:   "trace-123",
		Service:   "order-service",
		Module:    "order",
		Action:    "Update",
		Resource:  "order_123",
		Operation: audit.OpUpdate,
		TargetID:  "123",
		Status:    audit.StatusOK,
	}
	if err := entry.SetPostValue(map[string]string{"order_status": "shipped"}); err != nil {
		return
	}

	if err := a.Record(ctx, entry); err != nil {
		return
	}
	if err := a.Flush(ctx); err != nil {
		return
	}
}
