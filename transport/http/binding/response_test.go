package binding

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

type nonFlusher struct {
	written []byte
}

func (nf *nonFlusher) Header() http.Header { return http.Header{} }

func (nf *nonFlusher) WriteHeader(int) {}

func (nf *nonFlusher) Write(p []byte) (int, error) {
	nf.written = append(nf.written, p...)
	return len(p), nil
}

// WriteStreamChunk 行为契约：
//   - 每块字节顺序写入响应体
//   - 仅首个非空 contentType 生效（HttpBody 首帧约定），后续不再覆盖
//   - 每块之后 flush
//   - 不可 flush 的 writer 在写出任何字节前返回错误
func TestWriteStreamChunk(t *testing.T) {
	rec := httptest.NewRecorder()
	if err := WriteStreamChunk(rec, "text/plain", []byte("a")); err != nil {
		t.Fatalf("chunk 1: %v", err)
	}
	if err := WriteStreamChunk(rec, "", []byte("b")); err != nil {
		t.Fatalf("chunk 2: %v", err)
	}
	if got := rec.Body.String(); got != "ab" {
		t.Fatalf("body = %q, want %q", got, "ab")
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/plain" {
		t.Fatalf("Content-Type = %q, want text/plain", ct)
	}
	if !rec.Flushed {
		t.Fatal("expected flush after each chunk")
	}

	rec2 := httptest.NewRecorder()
	if err := WriteStreamChunk(rec2, "text/plain", []byte("a")); err != nil {
		t.Fatalf("chunk 1: %v", err)
	}
	if err := WriteStreamChunk(rec2, "application/json", []byte("b")); err != nil {
		t.Fatalf("chunk 2: %v", err)
	}
	if ct := rec2.Header().Get("Content-Type"); ct != "text/plain" {
		t.Fatalf("Content-Type = %q, want first-frame text/plain", ct)
	}

	nf := &nonFlusher{}
	if err := WriteStreamChunk(nf, "", []byte("x")); err == nil {
		t.Fatal("non-flusher: want error, got nil")
	}
	if len(nf.written) != 0 {
		t.Fatalf("non-flusher wrote %d bytes before error", len(nf.written))
	}
}
