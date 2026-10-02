package sse

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/tx7do/go-wind-plugins/encoding/json"
)

// ---------------------------------------------------------------------------
// event stream reader / framing helpers
// ---------------------------------------------------------------------------

func TestMinPosInt(t *testing.T) {
	tests := []struct {
		a, b, want int
	}{
		{1, 2, 1},
		{2, 1, 1},
		{5, 5, 5},
		{-1, 4, 0},
		{4, -1, 0},
		{-3, -7, 0},
	}
	for _, tt := range tests {
		if got := minPosInt(tt.a, tt.b); got != tt.want {
			t.Errorf("minPosInt(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestContainsDoubleNewline(t *testing.T) {
	tests := []struct {
		name      string
		data      string
		wantIdx   int
		wantWidth int
	}{
		{"lf lf", "abc\n\ndef", 3, 2},
		{"crlf crlf", "abc\r\n\r\ndef", 3, 4},
		{"none", "abcndef", -1, 0},
		{"single newline", "a\nb", -1, 0},
		{"trailing incomplete crlfcrlf", "abc\r\n", -1, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx, width := containsDoubleNewline([]byte(tt.data))
			if idx != tt.wantIdx || width != tt.wantWidth {
				t.Errorf("containsDoubleNewline(%q) = (%d, %d), want (%d, %d)",
					tt.data, idx, width, tt.wantIdx, tt.wantWidth)
			}
		})
	}
}

func TestEventStreamReader(t *testing.T) {
	stream := strings.NewReader("event: one\ndata: 1\n\nevent: two\ndata: 2\n\n")
	r := NewEventStreamReader(stream, 4096)

	first, err := r.ReadEvent()
	if err != nil {
		t.Fatalf("ReadEvent() error = %v", err)
	}
	if !strings.Contains(string(first), "event: one") {
		t.Errorf("first event = %q, want the first block", first)
	}

	second, err := r.ReadEvent()
	if err != nil {
		t.Fatalf("ReadEvent() error = %v", err)
	}
	if !strings.Contains(string(second), "event: two") {
		t.Errorf("second event = %q, want the second block", second)
	}

	if _, err := r.ReadEvent(); err == nil {
		t.Error("expected io.EOF after the stream ends")
	}
}

func TestEventStreamReader_CRLF(t *testing.T) {
	stream := strings.NewReader("data: a\r\n\r\ndata: b\r\n\r\n")
	r := NewEventStreamReader(stream, 4096)

	for i, want := range []string{"data: a", "data: b"} {
		got, err := r.ReadEvent()
		if err != nil {
			t.Fatalf("event %d: ReadEvent() error = %v", i, err)
		}
		if string(got) != want {
			t.Errorf("event %d = %q, want %q", i, got, want)
		}
	}
}

func TestEventStreamReader_TrailingEventWithoutBlankLine(t *testing.T) {
	// atEOF final flush returns the remainder as an event.
	stream := strings.NewReader("data: tail")
	r := NewEventStreamReader(stream, 4096)

	got, err := r.ReadEvent()
	if err != nil {
		t.Fatalf("ReadEvent() error = %v", err)
	}
	if string(got) != "data: tail" {
		t.Errorf("event = %q, want the trailing block", got)
	}
}

func TestEventStreamReader_LineTooLong(t *testing.T) {
	big := strings.Repeat("x", 10000)
	stream := strings.NewReader("data: " + big + "\n\n")
	r := NewEventStreamReader(stream, 128)

	if _, err := r.ReadEvent(); err == nil {
		t.Error("expected an error for an event exceeding maxBufferSize")
	}
}

func TestWriteData(t *testing.T) {
	var b strings.Builder

	if _, err := writeData(&b, []byte("data"), []byte("value")); err != nil {
		t.Fatalf("writeData() error = %v", err)
	}
	if !strings.HasSuffix(b.String(), "data value\n") {
		t.Errorf("output = %q, want a field line", b.String())
	}

	b.Reset()
	if _, err := writeData(&b, nil, []byte("just-a-comment")); err != nil {
		t.Fatalf("writeData() error = %v", err)
	}
	if b.String() != "just-a-comment\n" {
		t.Errorf("output = %q, want the bare comment line", b.String())
	}

	// A field that already ends with a space must not get a second one.
	b.Reset()
	if _, err := writeData(&b, []byte("data "), []byte("v")); err != nil {
		t.Fatalf("writeData() error = %v", err)
	}
	if b.String() != "data v\n" {
		t.Errorf("output = %q, want a single separating space", b.String())
	}
}

// ---------------------------------------------------------------------------
// event log replay and eviction
// ---------------------------------------------------------------------------

func TestEventLog_ReplayOrder(t *testing.T) {
	log := EventLog{}
	log.Add(&Event{Data: []byte("e1")})
	log.Add(&Event{Data: []byte("e2")})
	log.Add(&Event{Data: []byte("e3")})

	sub := &Subscriber{
		eventId:    string(log[0].ID),
		connection: make(chan *Event, 8),
	}
	// Replay everything strictly newer than the first event.
	log.Replay(sub)
	close(sub.connection)

	count := 0
	for range sub.connection {
		count++
	}
	if count != 2 {
		t.Errorf("replayed %d events, want 2 (e2 and e3)", count)
	}
}

func TestEventLog_ReplayAllWhenUnknownLastID(t *testing.T) {
	log := EventLog{}
	log.Add(&Event{Data: []byte("e1")})

	sub := &Subscriber{eventId: "", connection: make(chan *Event, 8)}
	log.Replay(sub)
	close(sub.connection)

	count := 0
	for range sub.connection {
		count++
	}
	if count != 1 {
		t.Errorf("replayed %d events, want 1", count)
	}
}

func TestEventLog_EvictionAtMaxSize(t *testing.T) {
	log := EventLog{}
	for i := 0; i < maxEventLogSize+10; i++ {
		log.Add(&Event{Data: []byte("e")})
	}
	if len(log) != maxEventLogSize {
		t.Errorf("log length = %d, want capped at %d", len(log), maxEventLogSize)
	}
}

func TestNewEventID_Format(t *testing.T) {
	id := newEventID()
	if len(id) != 32 {
		t.Errorf("newEventID() length = %d, want 32 hex chars", len(id))
	}
	if strings.Contains(id, "-") {
		t.Errorf("newEventID() = %q, want no dashes", id)
	}
}

// ---------------------------------------------------------------------------
// stream manager
// ---------------------------------------------------------------------------

func TestStreamManager_Lifecycle(t *testing.T) {
	mgr := NewStreamManager()
	if mgr.Count() != 0 {
		t.Fatal("new manager should be empty")
	}

	// Add(nil) is a no-op.
	mgr.Add(nil)

	s1 := newStream("a", 4, false, false, nil, nil)
	s2 := newStream("b", 4, false, false, nil, nil)
	mgr.Add(s1)
	mgr.Add(s2)
	if mgr.Count() != 2 {
		t.Fatalf("Count() = %d, want 2", mgr.Count())
	}

	// Duplicate add is ignored.
	mgr.Add(newStream("a", 4, false, false, nil, nil))
	if mgr.Count() != 2 {
		t.Errorf("Count() after duplicate Add = %d, want 2", mgr.Count())
	}

	if mgr.Get("a") != s1 {
		t.Error("Get(a) returned the wrong stream")
	}
	if mgr.Get("zzz") != nil {
		t.Error("Get(zzz) should be nil")
	}
	if !mgr.Exist("a") || mgr.Exist("zzz") {
		t.Error("Exist() results are wrong")
	}

	var ranged int
	mgr.Range(func(*Stream) { ranged++ })
	if ranged != 2 {
		t.Errorf("Range visited %d streams, want 2", ranged)
	}

	mgr.Remove(s1)
	if mgr.Exist("a") {
		t.Error("Remove() should delete the stream")
	}

	mgr.RemoveWithID("b")
	mgr.RemoveWithID("nonexistent") // must not panic
	if mgr.Count() != 0 {
		t.Errorf("Count() = %d, want 0", mgr.Count())
	}
}

// ---------------------------------------------------------------------------
// stream loop: subscribers, dispatch, replay, teardown
// ---------------------------------------------------------------------------

func TestStream_DispatchAndTeardown(t *testing.T) {
	var unsubCalls atomic.Int64
	onUnsub := func(_ StreamID, _ *Subscriber) { unsubCalls.Add(1) }

	s := newStream("stream-1", 8, false, true, nil, onUnsub)
	s.run()
	defer s.close()

	sub := s.addSubscriber("", nil)
	if s.getSubscriberCount() != 1 {
		t.Fatalf("subscriber count = %d, want 1", s.getSubscriberCount())
	}

	// A comment-only event is dispatched to subscribers.
	s.event <- &Event{Comment: []byte("keepalive")}

	select {
	case ev := <-sub.connection:
		if string(ev.Comment) != "keepalive" {
			t.Errorf("dispatched comment = %q, want keepalive", ev.Comment)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("event not dispatched within 2s")
	}

	// Deregistration closes the connection channel and fires the callback.
	sub.quit <- sub
	// removeSubscriber signals the removed channel for autoStream sessions.
	<-sub.removed

	select {
	case _, ok := <-sub.connection:
		if ok {
			t.Error("connection channel should be closed after deregistration")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("connection channel not closed within 2s")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if unsubCalls.Load() > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("onUnsubscribe callback was never invoked")
}

func TestStream_ReplayToLateSubscriber(t *testing.T) {
	s := newStream("stream-2", 8, true /* autoReplay */, false, nil, nil)
	s.run()
	defer s.close()

	// Publish before anyone is listening: the event is journaled.
	s.event <- &Event{Data: []byte("historic")}

	sub := s.addSubscriber("", nil)
	got := 0
	deadline := time.Now().Add(2 * time.Second)
	for got < 1 && time.Now().Before(deadline) {
		select {
		case ev, ok := <-sub.connection:
			if !ok {
				return
			}
			if string(ev.Data) != "historic" {
				t.Errorf("replayed event = %q, want historic", ev.Data)
			}
			got++
		case <-time.After(100 * time.Millisecond):
		}
	}
	if got != 1 {
		t.Errorf("replayed %d events, want 1", got)
	}
}

func TestStream_CloseRemovesAllSubscribers(t *testing.T) {
	s := newStream("stream-3", 8, false, false, nil, nil)
	s.run()

	sub := s.addSubscriber("", nil)
	s.close()

	select {
	case _, ok := <-sub.connection:
		if ok {
			t.Error("connection should be closed when the stream stops")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stream close did not tear down the subscriber within 2s")
	}
}

// TestStream_JournalCopiesInsteadOfMutating is a regression test for the
// eventTTL/autoReplay interaction: the run loop must journal a copy of the
// event, keeping the dispatched event's publish-time timestamp intact (only
// the journaled ID is propagated back for Last-Event-ID continuity).
func TestStream_JournalCopiesInsteadOfMutating(t *testing.T) {
	s := newStream("stream-ttl", 8, true /* autoReplay */, false, nil, nil)
	s.run()
	defer s.close()

	sub := s.addSubscriber("", nil)

	ev := &Event{Data: []byte("old")}
	ev.timestamp = time.Now().Add(-2 * time.Hour)
	s.event <- ev

	select {
	case got := <-sub.connection:
		if !got.timestamp.Equal(ev.timestamp) {
			t.Errorf("dispatched timestamp = %v, want the untouched publish-time stamp %v", got.timestamp, ev.timestamp)
		}
		if len(got.ID) == 0 {
			t.Error("dispatched event must carry the journaled ID for Last-Event-ID resume")
		}
		if string(got.Data) != "old" {
			t.Errorf("dispatched data = %q, want old", got.Data)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("event not dispatched within 2s")
	}

	if len(s.eventLog) != 1 {
		t.Fatalf("event log size = %d, want 1", len(s.eventLog))
	}
	if s.eventLog[0] == ev {
		t.Error("event log must hold a copy, not the dispatched event")
	}
	if s.eventLog[0].timestamp.IsZero() || s.eventLog[0].timestamp.Equal(ev.timestamp) {
		t.Errorf("journal timestamp = %v, want a fresh stamp distinct from the publish time", s.eventLog[0].timestamp)
	}
}

// TestStream_CloseFiresUnsubscribeOncePerSubscriber pins the teardown callback
// contract: stream-level close fires onUnsubscribe exactly once for every
// subscriber still attached.
func TestStream_CloseFiresUnsubscribeOncePerSubscriber(t *testing.T) {
	var calls int32
	s := newStream("stream-unsub", 8, false, false, nil, func(StreamID, *Subscriber) {
		atomic.AddInt32(&calls, 1)
	})
	s.run()

	subs := make([]*Subscriber, 2)
	for i := range subs {
		subs[i] = s.addSubscriber("", nil)
	}
	s.close()

	for i, sub := range subs {
		select {
		case _, ok := <-sub.connection:
			if ok {
				t.Errorf("subscriber %d: connection should be closed when the stream stops", i)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("subscriber %d not torn down within 2s", i)
		}
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&calls) == 2 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("onUnsubscribe fired %d times, want exactly 2 (once per subscriber)", atomic.LoadInt32(&calls))
}

// TestStream_ExplicitDeregisterThenCloseDoesNotDoubleFire pins that a
// subscriber removed through the explicit deregister path is not counted
// again when the stream later tears down.
func TestStream_ExplicitDeregisterThenCloseDoesNotDoubleFire(t *testing.T) {
	var calls int32
	s := newStream("stream-unsub-once", 8, false, true, nil, func(StreamID, *Subscriber) {
		atomic.AddInt32(&calls, 1)
	})
	s.run()

	sub := s.addSubscriber("", nil)

	sub.quit <- sub
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&calls) == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("onUnsubscribe fired %d times after explicit deregister, want 1", got)
	}

	s.close()
	time.Sleep(100 * time.Millisecond)
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("onUnsubscribe fired %d times after close, want still 1 (no double fire)", got)
	}
}

// ---------------------------------------------------------------------------
// server: event processing and marshalling
// ---------------------------------------------------------------------------

func TestServer_Process_ClonesAndStamps(t *testing.T) {
	srv := NewServer(":0", WithEncodeBase64(true))
	src := &Event{Data: []byte("hello")}

	out := srv.process(src)

	if string(src.Data) != "hello" {
		t.Errorf("original event mutated: %q, want hello untouched", src.Data)
	}
	if string(out.Data) != "aGVsbG8=" {
		t.Errorf("processed data = %q, want base64 of hello", out.Data)
	}
	if out.timestamp.IsZero() {
		t.Error("process() should stamp a zero timestamp")
	}
}

func TestServer_Process_Base64NotDoubled(t *testing.T) {
	// The same event fanned out through two process calls must not encode twice.
	srv := NewServer(":0", WithEncodeBase64(true))
	src := &Event{Data: []byte("hello")}

	first := srv.process(src)
	second := srv.process(src)
	if string(first.Data) != string(second.Data) {
		t.Errorf("double encode detected: %q vs %q", first.Data, second.Data)
	}
}

func TestServer_MarshalEvent(t *testing.T) {
	srv := NewServer(":0")

	ev, err := srv.marshalEvent(nil)
	if err != nil {
		t.Fatalf("marshalEvent(nil) error = %v", err)
	}
	if len(ev.Data) != 0 {
		t.Errorf("marshalEvent(nil) data = %q, want empty", ev.Data)
	}

	ev, err = srv.marshalEvent(map[string]string{"k": "v"})
	if err != nil {
		t.Fatalf("marshalEvent() error = %v", err)
	}
	if string(ev.Data) != `{"k":"v"}` {
		t.Errorf("marshalEvent() data = %s, want JSON object", ev.Data)
	}

	// Unmarshalable payloads surface the codec error.
	if _, err := srv.marshalEvent(make(chan int)); err == nil {
		t.Error("marshalEvent() should fail for unmarshalable payloads")
	}
}

func TestServer_PublishData_MarshalError(t *testing.T) {
	srv := NewServer(":0")
	srv.CreateStream("s")
	if err := srv.PublishData(context.Background(), "s", make(chan int)); err == nil {
		t.Error("PublishData() should surface codec errors")
	}
	if err := srv.PublishData(context.Background(), "missing", "x"); err != nil {
		t.Errorf("PublishData() to a missing stream should be a silent no-op, got %v", err)
	}
}

func TestServer_NotifyAndMetaVariants(t *testing.T) {
	srv := NewServer(":0")
	s1 := srv.CreateStream("n1")
	s2 := srv.CreateStream("n2")
	defer srv.RemoveStream("n1")
	defer srv.RemoveStream("n2")

	// Subscribe raw subscribers so the run loop has somewhere to deliver.
	sub1 := s1.addSubscriber("", nil)
	sub2 := s2.addSubscriber("", nil)

	if err := srv.NotifyDataWithMeta(context.Background(), map[string]string{"k": "v"},
		WithEventName("bulk")); err != nil {
		t.Fatalf("NotifyDataWithMeta() error = %v", err)
	}

	for i, sub := range []*Subscriber{sub1, sub2} {
		select {
		case ev := <-sub.connection:
			if string(ev.Event) != "bulk" {
				t.Errorf("subscriber %d: event name = %q, want bulk", i, ev.Event)
			}
		case <-time.After(2 * time.Second):
			t.Errorf("subscriber %d: NotifyDataWithMeta did not arrive", i)
		}
	}
}

// ---------------------------------------------------------------------------
// server: HTTP surface additions
// ---------------------------------------------------------------------------

func TestServer_ServeHTTP_OptionsPreflight(t *testing.T) {
	srv := NewServer(":0", WithCORSAllowOrigin("https://ok.example.com"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/events?stream=s", nil)
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://ok.example.com" {
		t.Errorf("CORS origin = %q, want https://ok.example.com", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, "GET") {
		t.Errorf("allow methods = %q, want GET included", got)
	}
}

// noFlushWriter is an http.ResponseWriter that deliberately does not
// implement http.Flusher.
type noFlushWriter struct {
	header http.Header
	buf    strings.Builder
	code   int
}

func (w *noFlushWriter) Header() http.Header {
	if w.header == nil {
		w.header = http.Header{}
	}
	return w.header
}

func (w *noFlushWriter) WriteHeader(code int) { w.code = code }

func (w *noFlushWriter) Write(b []byte) (int, error) { return w.buf.Write(b) }

func TestServer_ServeHTTP_NonFlusher(t *testing.T) {
	srv := NewServer(":0")

	// A writer without http.Flusher must be rejected before any streaming.
	// (httptest.ResponseRecorder DOES implement Flusher, so a plain writer
	// is needed to reach this branch.)
	w := &noFlushWriter{}
	req := httptest.NewRequest(http.MethodGet, "/events?stream=s", nil)
	srv.ServeHTTP(w, req)

	if w.code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.code)
	}
	if !strings.Contains(w.buf.String(), "Streaming unsupported") {
		t.Errorf("body = %q, want the streaming-unsupported message", w.buf.String())
	}
}

// collectStreamBody subscribes over HTTP and returns everything streamed
// within a short window.
func collectStreamBody(t *testing.T, base, streamID string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/events?stream="+streamID, nil)
	if err != nil {
		t.Fatalf("request build failed: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}
	defer resp.Body.Close()

	buf := make([]byte, 0, 1024)
	tmp := make([]byte, 256)
	for {
		n, err := resp.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil || strings.Contains(string(buf), "event: done") {
			break
		}
	}
	return string(buf)
}

func TestServer_ServeHTTP_CustomHeadersAndTTL(t *testing.T) {
	// Flipped regression test: with autoReplay enabled, EventLog.Add used to
	// re-stamp the dispatched event's timestamp at journaling time, which
	// defeated the eventTTL filter entirely. The journal now gets a copy, so
	// the TTL check sees the publish-time stamp even on the replay path.
	srv := NewServer("127.0.0.1:0",
		WithHeaders(map[string]string{"X-Extra": "yes"}),
		WithEventTTL(time.Hour),
	)
	srv.CreateStream("ttl-stream")

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = srv.Start(ctx) }()
	time.Sleep(100 * time.Millisecond)
	base := "http://" + srv.listener.Addr().String()

	bodyCh := make(chan string, 1)
	go func() { bodyCh <- collectStreamBody(t, base, "ttl-stream") }()
	time.Sleep(150 * time.Millisecond)

	// A stale event (timestamp older than the TTL) must be skipped, while a
	// fresh one flows through; a named terminator stops the collector.
	stale := &Event{Data: []byte("stale"), Event: []byte("never")}
	stale.timestamp = time.Now().Add(-2 * time.Hour)
	srv.Publish(ctx, "ttl-stream", stale)
	srv.Publish(ctx, "ttl-stream", &Event{Data: []byte("fresh")})
	srv.Publish(ctx, "ttl-stream", &Event{Event: []byte("done")})

	select {
	case body := <-bodyCh:
		if strings.Contains(body, "stale") {
			t.Errorf("stale event leaked: %q", body)
		}
		if !strings.Contains(body, "fresh") {
			t.Errorf("fresh event missing, got %q", body)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("never received streamed events")
	}

	cancel()
}

func TestServer_ServeHTTP_HeadersApplied(t *testing.T) {
	srv := NewServer("127.0.0.1:0", WithHeaders(map[string]string{"X-Extra": "yes"}))
	srv.CreateStream("hdr-stream")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go func() { _ = srv.Start(ctx) }()
	time.Sleep(100 * time.Millisecond)
	base := "http://" + srv.listener.Addr().String()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/events?stream=hdr-stream", nil)
	if err != nil {
		t.Fatalf("request build failed: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("X-Extra"); got != "yes" {
		t.Errorf("X-Extra = %q, want yes", got)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", got)
	}
}

func TestServer_SplitDataAndComments(t *testing.T) {
	srv := NewServer("127.0.0.1:0", WithSplitData(true), WithAutoReplay(false))
	srv.CreateStream("split-stream")

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = srv.Start(ctx) }()
	time.Sleep(100 * time.Millisecond)
	base := "http://" + srv.listener.Addr().String()

	bodyCh := make(chan string, 1)
	go func() { bodyCh <- collectStreamBody(t, base, "split-stream") }()
	time.Sleep(150 * time.Millisecond)

	srv.Publish(ctx, "split-stream", &Event{Data: []byte("line1\nline2"), Comment: []byte("note")})
	srv.Publish(ctx, "split-stream", &Event{Event: []byte("done")})

	select {
	case body := <-bodyCh:
		if !strings.Contains(body, "data: line1\ndata: line2\n") {
			t.Errorf("split data missing, got %q", body)
		}
		if !strings.Contains(body, "note\n") {
			t.Errorf("comment missing, got %q", body)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("never received streamed events")
	}

	cancel()
}

func TestServer_SubscriberCallbacks(t *testing.T) {
	var mu sync.Mutex
	var added, removed int
	srv := NewServer("127.0.0.1:0",
		WithSubscriberFunction(func(StreamID, *Subscriber) {
			mu.Lock()
			added++
			mu.Unlock()
		}),
		WithUnSubscriberFunction(func(StreamID, *Subscriber) {
			mu.Lock()
			removed++
			mu.Unlock()
		}),
	)
	srv.CreateStream("cb-stream")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	go func() { _ = srv.Start(ctx) }()
	time.Sleep(100 * time.Millisecond)
	base := "http://" + srv.listener.Addr().String()

	go func() {
		collectStreamBody(t, base, "cb-stream")
	}()
	time.Sleep(150 * time.Millisecond)

	// A terminator event ends the collector, closing the response body so the
	// request context is cancelled and the subscriber deregisters via the
	// explicit deregister path.
	stream := srv.GetStream("cb-stream")
	if stream == nil {
		t.Fatal("stream missing")
	}
	stream.event <- &Event{Event: []byte("done")}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		a, r := added, removed
		mu.Unlock()
		if a >= 1 && r >= 1 {
			// Regression: stream-level teardown (RemoveWithID → Stream.close)
			// must fire onUnsubscribe for its remaining subscribers too.
			srv.CreateStream("cb2-stream")
			s2 := srv.GetStream("cb2-stream")
			if s2 == nil {
				t.Fatal("cb2 stream missing")
			}
			s2.addSubscriber("", nil)
			srv.RemoveStream("cb2-stream")

			teardownDeadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(teardownDeadline) {
				mu.Lock()
				a2, r2 := added, removed
				mu.Unlock()
				if a2 >= 2 && r2 >= 2 {
					cancel()
					return
				}
				time.Sleep(20 * time.Millisecond)
			}
			mu.Lock()
			a2, r2 := added, removed
			mu.Unlock()
			t.Fatalf("teardown callbacks added=%d removed=%d, want both >= 2", a2, r2)
		}
		time.Sleep(20 * time.Millisecond)
	}
	mu.Lock()
	a, r := added, removed
	mu.Unlock()
	t.Fatalf("callbacks added=%d removed=%d, want both >= 1", a, r)
}

func TestServer_StopRunning(t *testing.T) {
	srv := NewServer("127.0.0.1:0")

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start(ctx) }()
	time.Sleep(100 * time.Millisecond)

	if err := srv.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after Stop")
	}
}

func TestServer_Endpoint_NoPort(t *testing.T) {
	srv := NewServer("kettle")
	if got := srv.Endpoint(); got != "http://kettle" {
		t.Errorf("Endpoint() = %q, want http://kettle", got)
	}
}

// ---------------------------------------------------------------------------
// options extras
// ---------------------------------------------------------------------------

func TestOptions_Extra(t *testing.T) {
	certPath, keyPath := writeSSETestCert(t, t.TempDir())

	srv := NewServer(":0",
		WithTLS(certPath, keyPath),
		WithCodec("json"),
		WithStreamIdKey("sid"),
		WithBufferSize(64),
		WithAutoStream(true),
		WithTokenExtractor(func(*http.Request) string { return "custom" }),
		WithAuthorizeFunc(func(*http.Request, string) error { return nil }),
	)
	if srv.tlsConfig == nil {
		t.Error("WithTLS did not configure TLS")
	}
	if got := srv.Endpoint(); !strings.HasPrefix(got, "https://") {
		t.Errorf("Endpoint() = %q, want https scheme", got)
	}
	if srv.streamIdKey != "sid" {
		t.Errorf("streamIdKey = %q, want sid", srv.streamIdKey)
	}
	if srv.bufferSize != 64 {
		t.Errorf("bufferSize = %d, want 64", srv.bufferSize)
	}
	if !srv.autoStream {
		t.Error("autoStream not enabled")
	}
	if srv.codec == nil {
		t.Error("WithCodec did not set the codec")
	}
	if srv.tokenExtractor == nil || srv.authorizeFunc == nil {
		t.Error("auth options not stored")
	}

	// WithTLS with missing files panics.
	func() {
		defer func() {
			if recover() == nil {
				t.Error("expected a panic for missing TLS files")
			}
		}()
		NewServer(":0", WithTLS("/nonexistent/c.pem", "/nonexistent/k.pem"))
	}()
}

func TestSubscriber_TokenFallbacks(t *testing.T) {
	// A non-Bearer Authorization value is returned verbatim (the extractor
	// only strips the "Bearer " prefix) and wins over the query parameter.
	u, _ := url.Parse("/events?token=query-token")
	sub := &Subscriber{
		URL:    u,
		Header: http.Header{},
	}
	sub.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	if got := sub.Token(""); got != "Basic dXNlcjpwYXNz" {
		t.Errorf("Token() = %q, want the raw Authorization value", got)
	}

	// With no Authorization header at all, the query token is the fallback.
	sub2 := &Subscriber{URL: u, Header: http.Header{}}
	if got := sub2.Token(""); got != "query-token" {
		t.Errorf("Token() = %q, want query-token", got)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func writeSSETestCert(t *testing.T, dir string) (certPath, keyPath string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("key generation failed: %v", err)
	}
	tmpl := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "localhost"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("certificate creation failed: %v", err)
	}
	certPath = filepath.Join(dir, "cert.pem")
	keyPath = filepath.Join(dir, "key.pem")
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("key marshalling failed: %v", err)
	}
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatalf("writing cert failed: %v", err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatalf("writing key failed: %v", err)
	}
	return certPath, keyPath
}
