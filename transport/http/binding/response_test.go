package binding

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	windErrors "github.com/tx7do/go-wind/errors"
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

// WriteError 结构化错误行为契约：
//   - WindError：HTTP 状态 = CodeToHTTP(Code)，响应体为错误自带 JSON 形状
//     （含 reason 字段，消费端以此驱动本地化文案）
//   - 被 %w 包装进错误链的 WindError 同样命中（FromError 沿链取值）
//   - 哨兵错误保留既有映射与响应体形状
//   - 未知错误回退 500
func TestWriteError_StructuredWindError(t *testing.T) {
	wErr := windErrors.BadRequest("TEST_BAD_REQUEST")
	rec := httptest.NewRecorder()
	WriteError(rec, wErr)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: want 400, got %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body["reason"] != "TEST_BAD_REQUEST" {
		t.Fatalf("reason: want TEST_BAD_REQUEST, got %v", body["reason"])
	}
	if _, present := body["code"]; present {
		t.Fatalf("code field should be excluded from the wire shape")
	}
}

func TestWriteError_WrappedWindError(t *testing.T) {
	wErr := windErrors.NotFound("TEST_NOT_FOUND")
	rec := httptest.NewRecorder()
	WriteError(rec, fmt.Errorf("wrapped: %w", wErr))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: want 404, got %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body["reason"] != "TEST_NOT_FOUND" {
		t.Fatalf("reason: want TEST_NOT_FOUND, got %v", body["reason"])
	}
}

func TestWriteError_SentinelFallback(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, ErrBadRequest)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: want 400, got %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body["reason"] != nil {
		t.Fatalf("sentinel path must not emit a reason field")
	}
}

func TestWriteError_UnknownFallback(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, errors.New("boom"))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status: want 500, got %d", rec.Code)
	}
}
