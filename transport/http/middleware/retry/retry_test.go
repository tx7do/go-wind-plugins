package retry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	coreRetry "github.com/tx7do/go-wind-plugins/retry"
)

// fastRetrier creates a Retrier with no backoff delay for fast tests.
func fastRetrier(attempts int) *coreRetry.Retrier {
	return coreRetry.New(
		coreRetry.WithMaxAttempts(attempts),
		coreRetry.WithBackoff(coreRetry.FixedBackoff(1*time.Millisecond)),
	)
}

func TestMiddleware_Get_SuccessFirstTry(t *testing.T) {
	var calls int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mw := Middleware(fastRetrier(3))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	mw(handler).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "ok", rec.Body.String())
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
}

func TestMiddleware_Get_RetriesUntilSuccess(t *testing.T) {
	var calls int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mw := Middleware(fastRetrier(3))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	mw(handler).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "ok", rec.Body.String())
	assert.Equal(t, int32(3), atomic.LoadInt32(&calls))
}

func TestMiddleware_Get_AllRetriesFail(t *testing.T) {
	var calls int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("unavailable"))
	})

	mw := Middleware(fastRetrier(3))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	mw(handler).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, "unavailable", rec.Body.String())
	assert.Equal(t, int32(3), atomic.LoadInt32(&calls))
}

func TestMiddleware_Post_NotRetried(t *testing.T) {
	var calls int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	mw := Middleware(fastRetrier(5))
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()

	mw(handler).ServeHTTP(rec, req)

	// POST is not idempotent → only one attempt.
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
}

func TestMiddleware_Post_RetryAllMethods(t *testing.T) {
	var calls int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 2 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	mw := Middleware(fastRetrier(3), WithRetryAllMethods())
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()

	mw(handler).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, int32(2), atomic.LoadInt32(&calls))
}

func TestMiddleware_NonRetryableStatus(t *testing.T) {
	var calls int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusNotFound) // 404 is not in the default retry set
	})

	mw := Middleware(fastRetrier(3))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	mw(handler).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
}

func TestMiddleware_CustomRetryStatus(t *testing.T) {
	var calls int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			w.WriteHeader(http.StatusConflict) // 409
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	mw := Middleware(fastRetrier(5), WithRetryStatus(http.StatusConflict))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	mw(handler).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, int32(3), atomic.LoadInt32(&calls))
}

func TestMiddleware_SkipFunc(t *testing.T) {
	var calls int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	mw := Middleware(fastRetrier(3), WithSkipFunc(func(r *http.Request) bool {
		return r.URL.Path == "/healthz"
	}))

	// Bypassed.
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	mw(handler).ServeHTTP(rec, req)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))

	// Not bypassed — should retry.
	atomic.StoreInt32(&calls, 0)
	req2 := httptest.NewRequest(http.MethodGet, "/api", nil)
	rec2 := httptest.NewRecorder()
	mw(handler).ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusServiceUnavailable, rec2.Code)
	assert.Equal(t, int32(3), atomic.LoadInt32(&calls))
}

func TestMiddleware_RetrierContextCancellation(t *testing.T) {
	var calls int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	mw := Middleware(coreRetry.New(
		coreRetry.WithMaxAttempts(100),
		coreRetry.WithBackoff(coreRetry.FixedBackoff(10*time.Millisecond)),
	))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	mw(handler).ServeHTTP(rec, req)

	// Should NOT have retried 100 times.
	require.Less(t, atomic.LoadInt32(&calls), int32(100))
}

func TestIdempotentMethods(t *testing.T) {
	// Verify the idempotent methods map.
	assert.True(t, idempotentMethods[http.MethodGet])
	assert.True(t, idempotentMethods[http.MethodHead])
	assert.True(t, idempotentMethods[http.MethodOptions])
	assert.True(t, idempotentMethods[http.MethodPut])
	assert.True(t, idempotentMethods[http.MethodDelete])
	assert.False(t, idempotentMethods[http.MethodPost])
	assert.False(t, idempotentMethods[http.MethodPatch])
}
