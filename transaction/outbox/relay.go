package outbox

import (
	"context"
	"encoding/json"
	"time"

	"github.com/tx7do/go-wind-plugins/broker"
)

// Publisher is the minimal broker surface the relay needs. Every broker
// implementation (kafka, rabbitmq, nats, ...) satisfies it through the
// broker.Broker interface.
type Publisher interface {
	Publish(ctx context.Context, topic string, msg *broker.Message, opts ...broker.PublishOption) error
}

// Relay publishes committed outbox rows to a broker. Run it as a background
// goroutine; one or more relay instances may share the same outbox table.
type Relay struct {
	store Store
	pub   Publisher
	cfg   relayConfig
}

type relayConfig struct {
	pollInterval time.Duration
	batchSize    int
	maxAttempts  int
	lease        time.Duration
	sweepAfter   time.Duration
	retryBackoff func(attempts int) time.Duration
	autoMigrate  bool
	now          func() time.Time
	onError      func(stage string, row Row, err error)
}

// RelayOption configures the Relay.
type RelayOption func(*relayConfig)

func defaultRelayConfig() relayConfig {
	return relayConfig{
		pollInterval: time.Second,
		batchSize:    100,
		maxAttempts:  16,
		lease:        5 * time.Minute,
		retryBackoff: func(attempts int) time.Duration {
			return time.Duration(attempts) * time.Second
		},
		autoMigrate: true,
		now:         nowFunc,
	}
}

// NewRelay returns a relay publishing claimed rows through pub.
func NewRelay(store Store, pub Publisher, opts ...RelayOption) *Relay {
	cfg := defaultRelayConfig()
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return &Relay{store: store, pub: pub, cfg: cfg}
}

// WithPollInterval sets how often the relay polls the outbox table.
func WithPollInterval(d time.Duration) RelayOption {
	return func(c *relayConfig) {
		if d > 0 {
			c.pollInterval = d
		}
	}
}

// WithBatchSize sets the maximum number of rows claimed per poll.
func WithBatchSize(n int) RelayOption {
	return func(c *relayConfig) {
		if n > 0 {
			c.batchSize = n
		}
	}
}

// WithMaxAttempts sets how many delivery attempts precede the dead state.
func WithMaxAttempts(n int) RelayOption {
	return func(c *relayConfig) {
		if n > 0 {
			c.maxAttempts = n
		}
	}
}

// WithLease sets how long a claim may survive without completion before the
// row is handed to another relay. Zero disables crash recovery.
func WithLease(d time.Duration) RelayOption {
	return func(c *relayConfig) { c.lease = d }
}

// WithSweepAfter deletes completed rows older than the duration on every
// poll. Zero (default) keeps completed rows for manual cleanup.
func WithSweepAfter(d time.Duration) RelayOption {
	return func(c *relayConfig) { c.sweepAfter = d }
}

// WithRetryBackoff overrides the delay before the next attempt after a
// failed publish; attempts starts at 1.
func WithRetryBackoff(f func(attempts int) time.Duration) RelayOption {
	return func(c *relayConfig) {
		if f != nil {
			c.retryBackoff = f
		}
	}
}

// WithAutoMigrate toggles schema creation on Run (default true).
func WithAutoMigrate(v bool) RelayOption {
	return func(c *relayConfig) { c.autoMigrate = v }
}

// WithNow overrides the relay clock (tests only).
func WithRelayNow(now func() time.Time) RelayOption {
	return func(c *relayConfig) {
		if now != nil {
			c.now = now
		}
	}
}

// WithErrorHandler installs a hook invoked for every delivery failure and
// store error, for logging or metrics.
func WithErrorHandler(f func(stage string, row Row, err error)) RelayOption {
	return func(c *relayConfig) {
		if f != nil {
			c.onError = f
		}
	}
}

// Run polls the outbox until ctx is canceled. The schema is ensured first
// when WithAutoMigrate is enabled (default).
func (r *Relay) Run(ctx context.Context) error {
	if r.cfg.autoMigrate {
		if err := r.store.EnsureSchema(ctx); err != nil {
			return err
		}
	}
	ticker := time.NewTicker(r.cfg.pollInterval)
	defer ticker.Stop()
	for {
		if _, err := r.Once(ctx); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			r.report("relay", Row{}, err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// Once performs a single claim-publish-complete cycle and returns the number
// of successfully published rows. Schema is not ensured; call EnsureSchema or
// Run once beforehand.
func (r *Relay) Once(ctx context.Context) (int, error) {
	if r.cfg.lease > 0 {
		if err := r.store.Reclaim(ctx, r.cfg.now().Add(-r.cfg.lease).UTC()); err != nil {
			r.report("reclaim", Row{}, err)
		}
	}
	if r.cfg.sweepAfter > 0 {
		if _, err := r.store.Sweep(ctx, r.cfg.now().Add(-r.cfg.sweepAfter).UTC()); err != nil {
			r.report("sweep", Row{}, err)
		}
	}

	rows, err := r.store.Claim(ctx, r.cfg.batchSize)
	if err != nil {
		return 0, err
	}

	published := 0
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return published, err
		}
		if err := r.pub.Publish(ctx, row.Topic, messageFromRow(row)); err != nil {
			nextVisible := r.cfg.now().Add(r.cfg.retryBackoff(row.Attempts))
			dead := row.Attempts >= r.cfg.maxAttempts
			if ferr := r.store.Fail(ctx, row.ID, err.Error(), nextVisible, dead); ferr != nil {
				r.report("fail", row, ferr)
			}
			r.report("publish", row, err)
			continue
		}
		if err := r.store.Complete(ctx, row.ID); err != nil {
			// Published but not completed: the lease will hand the row to
			// another claim, and the broker-side dedupe absorbs the repeat.
			r.report("complete", row, err)
			continue
		}
		published++
	}
	return published, nil
}

func (r *Relay) report(stage string, row Row, err error) {
	if r.cfg.onError != nil && err != nil {
		r.cfg.onError(stage, row, err)
	}
}

// messageFromRow builds the broker message for a claimed row. The payload is
// published raw (JSON bytes); consumers decode it via Inbox/Barrier.
func messageFromRow(row Row) *broker.Message {
	msg := &broker.Message{
		ID:      row.ID,
		Key:     row.Key,
		Headers: broker.Headers{HeaderEventID: row.ID, HeaderTopic: row.Topic},
		Body:    row.Payload,
	}
	if len(row.Metadata) > 0 {
		var meta map[string]string
		if err := json.Unmarshal(row.Metadata, &meta); err == nil {
			for k, v := range meta {
				msg.SetHeader(k, v)
			}
		}
	}
	return msg
}
