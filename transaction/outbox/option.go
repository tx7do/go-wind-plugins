package outbox

import "time"

// Default table names. Override via WithTable / WithInboxTable when the
// defaults collide with an existing schema.
const (
	defaultOutboxTable = "outbox_events"
	defaultInboxTable  = "inbox_messages"
)

// Option configures table names, placeholders and the injectable clock
// shared by Enqueue, EnsureSchema, Store and the consumer middlewares.
type Option func(*config)

type config struct {
	outboxTable string
	inboxTable  string
	skipLocked  bool
	now         func() time.Time
}

func newConfig(opts ...Option) config {
	cfg := config{
		outboxTable: defaultOutboxTable,
		inboxTable:  defaultInboxTable,
		skipLocked:  true,
		now:         nowFunc,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return cfg
}

// WithTable overrides the outbox table name (default "outbox_events").
func WithTable(name string) Option {
	return func(c *config) {
		if name != "" {
			c.outboxTable = name
		}
	}
}

// WithInboxTable overrides the inbox table name (default "inbox_messages").
func WithInboxTable(name string) Option {
	return func(c *config) {
		if name != "" {
			c.inboxTable = name
		}
	}
}

// WithSkipLocked toggles the FOR UPDATE SKIP LOCKED hint used while claiming
// rows, which lets multiple relay instances share one outbox table without
// double delivery. It requires MySQL 8.0+ or PostgreSQL; disable it for
// SQLite or older MySQL releases.
func WithSkipLocked(v bool) Option {
	return func(c *config) { c.skipLocked = v }
}

// WithNow overrides the clock used for timestamps (tests only).
func WithNow(now func() time.Time) Option {
	return func(c *config) {
		if now != nil {
			c.now = now
		}
	}
}
