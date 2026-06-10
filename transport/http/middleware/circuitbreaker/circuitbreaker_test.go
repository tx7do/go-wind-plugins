package circuitbreaker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tx7do/go-wind-plugins/circuitbreaker"
)

// fakeBreaker is a controllable test double for circuitbreaker.CircuitBreaker.
type fakeBreaker struct {
	allowErr    error
	allowCalls  int
	markSuccess int
	markFailure int
	state       circuitbreaker.State
}

func (f *fakeBreaker) Allow() error {
	f.allowCalls++
	return f.allowErr
}

func (f *fakeBreaker) MarkSuccess()                                       { f.markSuccess++ }
func (f *fakeBreaker) MarkFailure()                                       { f.markFailure++ }
func (f *fakeBreaker) State() circuitbreaker.State                        { return f.state }
func (f *fakeBreaker) Execute(ctx context.Context, fn func() error) error { return fn() }
func (f *fakeBreaker) Close() error                                       { return nil }

// ---------------------------------------------------------------------------

func handlerWithStatus(code int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(code)
	})
}

// --- Middleware ---

func TestMiddleware_Success(t *testing.T) {
	cb := &fakeBreaker{}
	mw := Middleware(cb)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mw(handlerWithStatus(http.StatusOK)).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, cb.allowCalls)
	assert.Equal(t, 1, cb.markSuccess)
	assert.Equal(t, 0, cb.markFailure)
}

func TestMiddleware_Failure_5xx(t *testing.T) {
	cb := &fakeBreaker{}
	mw := Middleware(cb)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mw(handlerWithStatus(http.StatusInternalServerError)).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, 1, cb.markFailure)
	assert.Equal(t, 0, cb.markSuccess)
}

func TestMiddleware_CircuitOpen(t *testing.T) {
	cb := &fakeBreaker{allowErr: circuitbreaker.ErrCircuitOpen}
	mw := Middleware(cb)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mw(handlerWithStatus(http.StatusOK)).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, 1, cb.allowCalls)
	assert.Equal(t, 0, cb.markSuccess)
	assert.Equal(t, 0, cb.markFailure)
}

func TestMiddleware_CustomFailureStatus(t *testing.T) {
	cb := &fakeBreaker{}
	// Treat only 429 as failure.
	mw := Middleware(cb, WithFailureStatus(http.StatusTooManyRequests))

	// 500 is NOT a failure with custom config.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mw(handlerWithStatus(http.StatusInternalServerError)).ServeHTTP(rec, req)
	assert.Equal(t, 1, cb.markSuccess)
	assert.Equal(t, 0, cb.markFailure)

	// 429 IS a failure.
	cb.markSuccess = 0
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec2 := httptest.NewRecorder()
	mw(handlerWithStatus(http.StatusTooManyRequests)).ServeHTTP(rec2, req2)
	assert.Equal(t, 0, cb.markSuccess)
	assert.Equal(t, 1, cb.markFailure)
}

func TestMiddleware_SkipFunc(t *testing.T) {
	cb := &fakeBreaker{allowErr: circuitbreaker.ErrCircuitOpen}
	mw := Middleware(cb, WithSkipFunc(func(r *http.Request) bool {
		return r.URL.Path == "/healthz"
	}))

	// Bypassed.
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	mw(handlerWithStatus(http.StatusOK)).ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 0, cb.allowCalls)

	// Not bypassed.
	req2 := httptest.NewRequest(http.MethodGet, "/api", nil)
	rec2 := httptest.NewRecorder()
	mw(handlerWithStatus(http.StatusOK)).ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusServiceUnavailable, rec2.Code)
	assert.Equal(t, 1, cb.allowCalls)
}

func TestMiddleware_CustomErrorHandler(t *testing.T) {
	cb := &fakeBreaker{allowErr: circuitbreaker.ErrCircuitOpen}
	mw := Middleware(cb, WithErrorHandler(func(w http.ResponseWriter, _ *http.Request, _ error) {
		http.Error(w, "circuit open", http.StatusBadGateway)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mw(handlerWithStatus(http.StatusOK)).ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadGateway, rec.Code)
}

// --- MiddlewareKeyed ---

func TestMiddlewareKeyed_PerKey(t *testing.T) {
	var factories int
	mw := MiddlewareKeyed(
		func(r *http.Request) string { return r.URL.Path },
		func(_ string) circuitbreaker.CircuitBreaker {
			factories++
			return &fakeBreaker{}
		},
	)

	// Same path → same breaker.
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1", nil)
		rec := httptest.NewRecorder()
		mw(handlerWithStatus(http.StatusOK)).ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v2", nil)
	rec := httptest.NewRecorder()
	mw(handlerWithStatus(http.StatusOK)).ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	assert.Equal(t, 2, factories) // /api/v1 and /api/v2
}
