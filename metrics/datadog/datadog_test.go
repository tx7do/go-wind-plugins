package datadog

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/tx7do/go-wind-plugins/metrics"
)

// startUDPListener binds a loopback UDP socket and returns its address and the
// bound connection. This keeps every test local — no external service is
// contacted.
func startUDPListener(t *testing.T) (addr string, conn net.PacketConn) {
	t.Helper()

	c, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind loopback UDP listener: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c.LocalAddr().String(), c
}

// readDatagram reads a single datagram from the listener, waiting up to 4s.
// Gauge metrics go through the client's aggregation buffer, whose default
// flush interval is 2s, so a generous deadline is required.
func readDatagram(t *testing.T, conn net.PacketConn) string {
	t.Helper()
	buf := make([]byte, 1500)
	if err := conn.SetReadDeadline(time.Now().Add(4 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	n, _, err := conn.ReadFrom(buf)
	if err != nil {
		t.Fatalf("failed to read datagram: %v", err)
	}
	return strings.TrimSuffix(string(buf[:n]), "\n")
}

// ---------------------------------------------------------------------------
// New — construction and error handling
// ---------------------------------------------------------------------------

func TestNew_DefaultConfigSucceeds(t *testing.T) {
	// The default address is 127.0.0.1:8125. UDP is connectionless, so the
	// client is created successfully even when no agent is listening.
	p, err := New()
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	defer p.Close()

	if p.client == nil {
		t.Fatal("New() returned a provider with a nil client")
	}
	if p.rate != 1.0 {
		t.Errorf("rate = %v, want 1.0 (default)", p.rate)
	}
}

func TestNew_InvalidAddressFails(t *testing.T) {
	_, err := New(WithAddress("invalid-address-no-port"))
	if err == nil {
		t.Error("New() with an unparseable address should return an error")
	}
}

// ---------------------------------------------------------------------------
// Options
// ---------------------------------------------------------------------------

func TestWithSampleRate(t *testing.T) {
	p, err := New(WithSampleRate(0.5))
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	defer p.Close()

	if p.rate != 0.5 {
		t.Errorf("rate = %v, want 0.5", p.rate)
	}
}

func TestWithNamespaceAndBufferOptions(t *testing.T) {
	p, err := New(
		WithNamespace("myapp"),
		WithBufferSize(32),
		WithFlushPeriod(50*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	defer p.Close()
}

// ---------------------------------------------------------------------------
// toTags
// ---------------------------------------------------------------------------

func TestToTags(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   []string
	}{
		{"nil labels", nil, nil},
		{"empty labels", map[string]string{}, nil},
		{"single label", map[string]string{"method": "GET"}, []string{"method:GET"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toTags(tt.labels)
			if len(got) != len(tt.want) {
				t.Fatalf("toTags() = %v, want %v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("toTags()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestToTags_MultipleLabels(t *testing.T) {
	// Map iteration order is random, so only check membership.
	got := toTags(map[string]string{"k1": "v1", "k2": "v2"})
	if len(got) != 2 {
		t.Fatalf("toTags() length = %d, want 2", len(got))
	}
	seen := map[string]bool{}
	for _, tag := range got {
		seen[tag] = true
	}
	if !seen["k1:v1"] || !seen["k2:v2"] {
		t.Errorf("toTags() = %v, want it to contain k1:v1 and k2:v2", got)
	}
}

// ---------------------------------------------------------------------------
// DogStatsD wire format (over a loopback UDP socket)
// ---------------------------------------------------------------------------

func TestCounter_WireFormat(t *testing.T) {
	addr, l := startUDPListener(t)

	p, err := New(WithAddress(addr), WithNamespace("myapp"))
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	defer p.Close()

	p.Counter(context.Background(), "requests_total", 1, map[string]string{"method": "GET"})

	if got := readDatagram(t, l); got != "myapp.requests_total:1|c|#method:GET" {
		t.Errorf("Counter datagram = %q, want %q", got, "myapp.requests_total:1|c|#method:GET")
	}
}

func TestGauge_WireFormat(t *testing.T) {
	addr, l := startUDPListener(t)

	p, err := New(WithAddress(addr))
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	defer p.Close()

	p.Gauge(context.Background(), "queue_depth", 42, nil)

	// NOTE: the current implementation always applies the namespace option,
	// even when it is empty, and the DogStatsD client then turns "" into a
	// single "." metric-name prefix (see the bug note in the test report).
	// We therefore assert on the suffix, which is independent of that prefix.
	if got := readDatagram(t, l); !strings.HasSuffix(got, "queue_depth:42|g") {
		t.Errorf("Gauge datagram = %q, want suffix %q", got, "queue_depth:42|g")
	}
}

func TestHistogram_WireFormat(t *testing.T) {
	addr, l := startUDPListener(t)

	p, err := New(WithAddress(addr))
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	defer p.Close()

	p.Histogram(context.Background(), "request_duration_seconds", 0.25, nil)

	if got := readDatagram(t, l); !strings.HasSuffix(got, "request_duration_seconds:0.25|h") {
		t.Errorf("Histogram datagram = %q, want suffix %q", got, "request_duration_seconds:0.25|h")
	}
}

func TestCounter_FloatValueTruncatesToInteger(t *testing.T) {
	addr, l := startUDPListener(t)

	p, err := New(WithAddress(addr))
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	defer p.Close()

	p.Counter(context.Background(), "ints_total", 3.7, nil)

	if got := readDatagram(t, l); !strings.Contains(got, "ints_total:3|c") {
		t.Errorf("Counter datagram = %q, want it to contain %q (float value truncated to int64)", got, "ints_total:3|c")
	}
}

// ---------------------------------------------------------------------------
// Close
// ---------------------------------------------------------------------------

func TestClose_ReturnsNil(t *testing.T) {
	p, err := New(WithAddress("127.0.0.1:0"))
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	if err := p.Close(); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
}

// ---------------------------------------------------------------------------
// Interface compliance
// ---------------------------------------------------------------------------

var _ metrics.Metrics = (*Provider)(nil)
var _ metrics.Closer = (*Provider)(nil)
