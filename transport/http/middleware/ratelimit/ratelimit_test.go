package ratelimit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tx7do/go-wind-plugins/ratelimit"
)

// fakeLimiter is a controllable test double for ratelimit.Limiter.
type fakeLimiter struct {
	allowOk    bool
	allowErr   error
	waitErr    error
	allowCalls int
	waitCalls  int
}

func (f *fakeLimiter) Allow() (bool, error) {
	f.allowCalls++
	return f.allowOk, f.allowErr
}

func (f *fakeLimiter) Wait(ctx context.Context) error {
	f.waitCalls++
	return f.waitErr
}

func (f *fakeLimiter) Close() error { return nil }

// ---------------------------------------------------------------------------

func handlerOK() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

// --- Middleware (shared limiter) ---

func TestMiddleware_Allow(t *testing.T) {
	lim := &fakeLimiter{allowOk: true}
	mw := Middleware(lim)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	mw(handlerOK()).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, lim.allowCalls)
}

func TestMiddleware_Reject(t *testing.T) {
	lim := &fakeLimiter{allowOk: false, allowErr: ratelimit.ErrLimited}
	mw := Middleware(lim)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	mw(handlerOK()).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Equal(t, 1, lim.allowCalls)
}

func TestMiddleware_WaitMode(t *testing.T) {
	lim := &fakeLimiter{waitErr: nil}
	mw := Middleware(lim, WithWait())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	mw(handlerOK()).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, lim.waitCalls)
	assert.Equal(t, 0, lim.allowCalls)
}

func TestMiddleware_WaitMode_Rejected(t *testing.T) {
	lim := &fakeLimiter{waitErr: ratelimit.ErrLimited}
	mw := Middleware(lim, WithWait())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	mw(handlerOK()).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Equal(t, 1, lim.waitCalls)
}

func TestMiddleware_SkipFunc(t *testing.T) {
	lim := &fakeLimiter{allowOk: false, allowErr: ratelimit.ErrLimited}
	mw := Middleware(lim, WithSkipFunc(func(r *http.Request) bool {
		return r.URL.Path == "/healthz"
	}))

	// Should bypass rate limiting.
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	mw(handlerOK()).ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 0, lim.allowCalls)

	// Should NOT bypass rate limiting.
	req2 := httptest.NewRequest(http.MethodGet, "/api", nil)
	rec2 := httptest.NewRecorder()
	mw(handlerOK()).ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusTooManyRequests, rec2.Code)
	assert.Equal(t, 1, lim.allowCalls)
}

func TestMiddleware_CustomErrorHandler(t *testing.T) {
	lim := &fakeLimiter{allowOk: false, allowErr: ratelimit.ErrLimited}
	mw := Middleware(lim, WithErrorHandler(func(w http.ResponseWriter, _ *http.Request, _ error) {
		http.Error(w, "rate limited", http.StatusServiceUnavailable)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mw(handlerOK()).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), "rate limited")
}

// --- MiddlewareKeyed ---

func TestMiddlewareKeyed_PerKey(t *testing.T) {
	var factories int
	mw := MiddlewareKeyed(
		func(r *http.Request) string { return r.Header.Get("X-Client-ID") },
		func(_ string) ratelimit.Limiter {
			factories++
			return &fakeLimiter{allowOk: true}
		},
	)

	// Two different clients — should create two limiters.
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Client-ID", "client-1")
		rec := httptest.NewRecorder()
		mw(handlerOK()).ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Client-ID", "client-2")
	rec := httptest.NewRecorder()
	mw(handlerOK()).ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	assert.Equal(t, 2, factories) // client-1 and client-2 each created once
}

func TestMiddlewareKeyed_RejectedClient(t *testing.T) {
	mw := MiddlewareKeyed(
		func(r *http.Request) string { return r.Header.Get("X-Client-ID") },
		func(_ string) ratelimit.Limiter {
			return &fakeLimiter{allowOk: false, allowErr: ratelimit.ErrLimited}
		},
	)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Client-ID", "blocked")
	rec := httptest.NewRecorder()
	mw(handlerOK()).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
}
