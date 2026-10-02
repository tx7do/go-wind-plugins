package http3

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Server options and accessors
// ---------------------------------------------------------------------------

func TestNewServerDefaults(t *testing.T) {
	srv := NewServer()

	assert.Equal(t, KindHTTP3, srv.Name())
	assert.Equal(t, ":443", srv.Addr)
	assert.Equal(t, ":443", srv.Endpoint())
	assert.Equal(t, time.Second, srv.timeout)
	assert.True(t, srv.strictSlash)
	assert.NotNil(t, srv.dec)
	assert.NotNil(t, srv.enc)
	assert.NotNil(t, srv.ene)
	assert.NotNil(t, srv.router, "router must be built during init")
	assert.NotNil(t, srv.tlsConf, "a self-signed TLS config must be generated")
	assert.NotNil(t, srv.Handler, "the handler chain must be installed")
}

func TestServerOptions(t *testing.T) {
	tlsConf := &tls.Config{}
	dec := func(*http.Request, any) error { return nil }
	enc := func(http.ResponseWriter, *http.Request, any) error { return nil }
	ene := func(http.ResponseWriter, *http.Request, error) error { return nil }
	filter := func(next http.Handler) http.Handler { return next }
	hm := func(next Handler) Handler { return next }

	srv := NewServer(
		WithAddress("127.0.0.1:9500"),
		WithTLSConfig(tlsConf),
		WithTimeout(5*time.Second),
		WithStrictSlash(false),
		WithRequestDecoder(dec),
		WithResponseEncoder(enc),
		WithErrorEncoder(ene),
		WithFilter(filter),
		WithMiddleware(hm),
	)

	assert.Equal(t, "127.0.0.1:9500", srv.Addr)
	assert.Same(t, tlsConf, srv.tlsConf)
	assert.Equal(t, 5*time.Second, srv.timeout)
	assert.False(t, srv.strictSlash)
	assert.Len(t, srv.filters, 1)
	assert.Len(t, srv.hms, 1)
}

// ---------------------------------------------------------------------------
// Route registration served through ServeHTTP (no QUIC required)
// ---------------------------------------------------------------------------

func TestServerServeHTTPRoutes(t *testing.T) {
	srv := NewServer()
	// Context-style handlers register through the Router; Server.HandleFunc
	// takes a plain http.HandlerFunc instead.
	router := srv.Route("")

	router.Handle(http.MethodGet, "/plain", func(ctx Context) error {
		return ctx.String(http.StatusOK, "plain")
	})

	router.Handle(http.MethodGet, "/json", func(ctx Context) error {
		return ctx.JSON(http.StatusCreated, map[string]string{"ok": "yes"})
	})

	router.Handle(http.MethodGet, "/xml", func(ctx Context) error {
		// encoding/xml rejects some anonymous struct literals, so use a named
		// payload type here.
		type xmlPayload struct {
			OK string `xml:"ok"`
		}
		return ctx.XML(http.StatusOK, xmlPayload{OK: "yes"})
	})

	router.Handle(http.MethodGet, "/blob", func(ctx Context) error {
		return ctx.Blob(http.StatusOK, "application/octet-stream", []byte{0x01, 0x02})
	})

	router.Handle(http.MethodGet, "/stream", func(ctx Context) error {
		return ctx.Stream(http.StatusOK, "text/plain", strings.NewReader("streamed"))
	})

	router.Handle(http.MethodGet, "/query", func(ctx Context) error {
		return ctx.String(http.StatusOK, "q="+ctx.Query().Get("q"))
	})

	router.Handle(http.MethodPost, "/echo", func(ctx Context) error {
		var in map[string]string
		if err := ctx.Bind(&in); err != nil {
			return err
		}
		return ctx.Returns(in, nil)
	})

	router.Handle(http.MethodGet, "/boom", func(ctx Context) error {
		return errors.New("kaboom")
	})

	do := func(method, target string, body io.Reader) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, body)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		return rec
	}

	t.Run("plain string", func(t *testing.T) {
		rec := do(http.MethodGet, "/plain", nil)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "plain", rec.Body.String())
	})

	t.Run("json", func(t *testing.T) {
		rec := do(http.MethodGet, "/json", nil)
		assert.Equal(t, http.StatusCreated, rec.Code)
		assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
		assert.Contains(t, rec.Body.String(), `"ok":"yes"`)
	})

	t.Run("xml", func(t *testing.T) {
		rec := do(http.MethodGet, "/xml", nil)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "application/xml", rec.Header().Get("Content-Type"))
		assert.Contains(t, rec.Body.String(), "<ok>yes</ok>")
	})

	t.Run("blob", func(t *testing.T) {
		rec := do(http.MethodGet, "/blob", nil)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "application/octet-stream", rec.Header().Get("Content-Type"))
		assert.Equal(t, []byte{0x01, 0x02}, rec.Body.Bytes())
	})

	t.Run("stream", func(t *testing.T) {
		rec := do(http.MethodGet, "/stream", nil)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "streamed", rec.Body.String())
	})

	t.Run("query binding", func(t *testing.T) {
		rec := do(http.MethodGet, "/query?q=hello", nil)
		assert.Equal(t, "q=hello", rec.Body.String())
	})

	t.Run("json bind and returns", func(t *testing.T) {
		rec := do(http.MethodPost, "/echo", strings.NewReader(`{"msg":"hi"}`))
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"msg":"hi"`)
	})

	t.Run("error goes through the error encoder", func(t *testing.T) {
		rec := do(http.MethodGet, "/boom", nil)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), `"error":"kaboom"`)
	})

	t.Run("unknown route 404", func(t *testing.T) {
		rec := do(http.MethodGet, "/missing", nil)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("wrong method 404", func(t *testing.T) {
		// MethodNotAllowedHandler is intentionally wired to NotFoundHandler.
		rec := do(http.MethodPost, "/plain", nil)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

// TestServerRoutePrefixAndVars exercises Router group prefixes, path
// variables, and the method helpers.
func TestServerRoutePrefixAndVars(t *testing.T) {
	srv := NewServer()
	router := srv.Route("/api")

	router.GET("/hello/{name}", func(ctx Context) error {
		return ctx.String(http.StatusOK, "hello "+ctx.Vars().Get("name"))
	})
	router.POST("/submit", func(ctx Context) error { return ctx.String(http.StatusOK, "posted") })
	router.HEAD("/head", func(ctx Context) error { return nil })
	router.PUT("/put", func(ctx Context) error { return ctx.String(http.StatusOK, "put") })
	router.PATCH("/patch", func(ctx Context) error { return ctx.String(http.StatusOK, "patch") })
	router.DELETE("/del", func(ctx Context) error { return ctx.String(http.StatusOK, "deleted") })
	router.OPTIONS("/opt", func(ctx Context) error { return ctx.String(http.StatusOK, "options") })
	router.TRACE("/trace", func(ctx Context) error { return ctx.String(http.StatusOK, "trace") })
	router.CONNECT("/conn", func(ctx Context) error { return ctx.String(http.StatusOK, "connect") })

	do := func(method, target string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		return rec
	}

	assert.Equal(t, "hello world", do(http.MethodGet, "/api/hello/world").Body.String())
	assert.Equal(t, "posted", do(http.MethodPost, "/api/submit").Body.String())
	assert.Equal(t, "put", do(http.MethodPut, "/api/put").Body.String())
	assert.Equal(t, "patch", do(http.MethodPatch, "/api/patch").Body.String())
	assert.Equal(t, "deleted", do(http.MethodDelete, "/api/del").Body.String())
	assert.Equal(t, "options", do(http.MethodOptions, "/api/opt").Body.String())
	assert.Equal(t, "trace", do(http.MethodTrace, "/api/trace").Body.String())
	assert.Equal(t, "connect", do(http.MethodConnect, "/api/conn").Body.String())
	assert.Equal(t, http.StatusOK, do(http.MethodHead, "/api/head").Code)
}

func TestServerRouteGroupInheritsPrefixAndFilters(t *testing.T) {
	var order []string
	traceFilter := func(tag string) FilterFunc {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, tag)
				next.ServeHTTP(w, r)
			})
		}
	}

	srv := NewServer()
	group := srv.Route("/api", traceFilter("api"))
	sub := group.Group("/v1", traceFilter("v1"))

	sub.GET("/ping", func(ctx Context) error { return ctx.String(http.StatusOK, "pong") })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "pong", rec.Body.String())
	// Route-level filter runs outermost, group filter innermost.
	assert.Equal(t, []string{"api", "v1"}, order)
}

func TestServerHandleVariants(t *testing.T) {
	srv := NewServer()

	srv.Handle("/exact", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("exact"))
	}))
	srv.HandleFunc("/rawfn", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("rawfn"))
	}))
	srv.HandlePrefix("/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("static"))
	}))
	srv.HandleHeader("X-Custom", "match", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("header-route"))
	})

	do := func(method, target string, hdr map[string]string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, nil)
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		return rec
	}

	assert.Equal(t, "exact", do(http.MethodGet, "/exact", nil).Body.String())
	assert.Equal(t, "rawfn", do(http.MethodGet, "/rawfn", nil).Body.String())
	assert.Equal(t, "static", do(http.MethodGet, "/static/file.js", nil).Body.String())
	assert.Equal(t, "header-route", do(http.MethodGet, "/anywhere", map[string]string{"X-Custom": "match"}).Body.String())
	assert.Equal(t, http.StatusNotFound, do(http.MethodGet, "/anywhere", nil).Code)
}

// TestServerUseMiddlewareOrder is the flipped regression test for the former
// defect: the HTTP middleware chain used to be built inside NewServer's init,
// so a post-construction Use() call never reached the live handler chain. Use()
// now rebuilds the chain, and the ordering contract is verified through the
// server's real ServeHTTP path: the first registered middleware runs outermost.
func TestServerUseMiddlewareOrder(t *testing.T) {
	var order []string
	srv := NewServer()
	srv.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "outer")
			next.ServeHTTP(w, r)
		})
	})
	srv.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "inner")
			next.ServeHTTP(w, r)
		})
	})
	srv.Route("").Handle(http.MethodGet, "/mw", func(ctx Context) error {
		return ctx.String(http.StatusOK, "done")
	})

	assert.Len(t, srv.middlewares, 2)

	req := httptest.NewRequest(http.MethodGet, "/mw", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, []string{"outer", "inner"}, order)
}

func TestServerRouteFilters(t *testing.T) {
	var order []string
	srv := NewServer()
	router := srv.Route("/api")
	router.GET("/filtered", func(ctx Context) error {
		order = append(order, "handler")
		return ctx.String(http.StatusOK, "ok")
	}, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "route-filter")
			next.ServeHTTP(w, r)
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/filtered", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, []string{"route-filter", "handler"}, order)
}

func TestServerErrorEncoderWithHTTPStatus(t *testing.T) {
	srv := NewServer()
	srv.Route("").Handle(http.MethodGet, "/status", func(ctx Context) error {
		return &statusError{code: http.StatusTeapot, msg: "brewing"}
	})

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusTeapot, rec.Code)
	assert.Contains(t, rec.Body.String(), "brewing")
}

type statusError struct {
	code int
	msg  string
}

func (e *statusError) Error() string   { return e.msg }
func (e *statusError) HTTPStatus() int { return e.code }

// TestServerRequestTimeout verifies the filter() timeout wrapper: the request
// context handed to handlers carries the server timeout as its deadline.
func TestServerRequestTimeout(t *testing.T) {
	srv := NewServer(WithTimeout(2 * time.Second))
	srv.Route("").Handle(http.MethodGet, "/deadline", func(ctx Context) error {
		deadline, ok := ctx.Deadline()
		if !ok {
			return errors.New("expected a request deadline")
		}
		if time.Until(deadline) > 2*time.Second {
			return errors.New("deadline exceeds the configured timeout")
		}
		return ctx.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/deadline", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

// ---------------------------------------------------------------------------
// Handler middlewares (WithMiddleware + ctx.Middleware)
// ---------------------------------------------------------------------------

func TestServerHandlerMiddleware(t *testing.T) {
	srv := NewServer(WithMiddleware(func(next Handler) Handler {
		return func(ctx context.Context, req any) (any, error) {
			return "wrapped:" + req.(string), nil
		}
	}))

	srv.Route("").Handle(http.MethodGet, "/hm", func(ctx Context) error {
		out, err := ctx.Middleware(func(ctx context.Context, req any) (any, error) {
			return req, nil
		})(context.Background(), "payload")
		if err != nil {
			return err
		}
		return ctx.String(http.StatusOK, out.(string))
	})

	req := httptest.NewRequest(http.MethodGet, "/hm", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	assert.Equal(t, "wrapped:payload", rec.Body.String())
}

// ---------------------------------------------------------------------------
// wrapper context helpers
// ---------------------------------------------------------------------------

func TestWrapperContextAccessors(t *testing.T) {
	w := &wrapper{}

	// With no request bound the context methods degrade gracefully.
	_, hasDeadline := w.Deadline()
	assert.False(t, hasDeadline)
	assert.Nil(t, w.Done())
	assert.ErrorIs(t, w.Err(), context.Canceled)
	assert.Nil(t, w.Value("k"))

	req := httptest.NewRequest(http.MethodGet, "http://x/y?a=1", nil)
	rec := httptest.NewRecorder()
	w.Reset(rec, req)

	assert.Same(t, req, w.Request())
	assert.Equal(t, rec, w.Response())
	assert.Equal(t, "1", w.Query().Get("a"))
	assert.NotNil(t, w.Header())
	assert.NoError(t, w.Err())
}

// ---------------------------------------------------------------------------
// Codec helpers (types.go)
// ---------------------------------------------------------------------------

func TestFilterChainOrder(t *testing.T) {
	var order []string
	mk := func(tag string) FilterFunc {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, tag)
				next.ServeHTTP(w, r)
			})
		}
	}

	handler := FilterChain(mk("first"), mk("second"))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	assert.Equal(t, []string{"first", "second"}, order)
}

func TestChainHandlerOrder(t *testing.T) {
	var order []string
	mk := func(tag string) HandlerMiddleware {
		return func(next Handler) Handler {
			return func(ctx context.Context, req any) (any, error) {
				order = append(order, tag)
				return next(ctx, req)
			}
		}
	}

	final := Handler(func(ctx context.Context, req any) (any, error) {
		order = append(order, "handler")
		return nil, nil
	})
	_, err := ChainHandler(mk("first"), mk("second"))(final)(context.Background(), nil)
	assert.NoError(t, err)
	assert.Equal(t, []string{"first", "second", "handler"}, order)
}

func TestDefaultRequestDecoder(t *testing.T) {
	t.Run("nil body is a no-op", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		var out map[string]string
		assert.NoError(t, DefaultRequestDecoder(req, &out))
	})

	t.Run("zero content length is a no-op", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
		req.ContentLength = 0
		var out map[string]string
		assert.NoError(t, DefaultRequestDecoder(req, &out))
	})

	t.Run("valid json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"a":"b"}`))
		req.ContentLength = 9
		var out map[string]string
		assert.NoError(t, DefaultRequestDecoder(req, &out))
		assert.Equal(t, "b", out["a"])
	})

	t.Run("invalid json errors", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{bad"))
		req.ContentLength = 5
		var out map[string]string
		assert.Error(t, DefaultRequestDecoder(req, &out))
	})
}

func TestDefaultEncoders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	t.Run("response encoder", func(t *testing.T) {
		rec := httptest.NewRecorder()
		assert.NoError(t, DefaultResponseEncoder(rec, req, map[string]int{"n": 1}))
		assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
		assert.Contains(t, rec.Body.String(), `"n":1`)
	})

	t.Run("error encoder default status", func(t *testing.T) {
		rec := httptest.NewRecorder()
		assert.NoError(t, DefaultErrorEncoder(rec, req, errors.New("nope")))
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), `"error":"nope"`)
	})

	t.Run("error encoder honors HTTPStatus", func(t *testing.T) {
		rec := httptest.NewRecorder()
		assert.NoError(t, DefaultErrorEncoder(rec, req, &statusError{code: http.StatusBadGateway, msg: "upstream"}))
		assert.Equal(t, http.StatusBadGateway, rec.Code)
	})
}

// ---------------------------------------------------------------------------
// Binding (binding.go)
// ---------------------------------------------------------------------------

type bindTarget struct {
	Name    string  `json:"name"`
	Age     int     `json:"age"`
	Height  float64 `json:"height"`
	Active  bool    `json:"active"`
	Count   uint    `json:"count"`
	Ignored string  `json:"-"`
	Tagged  string  `form:"tagged"`
	Quered  string  `query:"quered"`
	Opts    *string `json:"opts"`
	Trimmed string  `json:"trimmed,omitempty"`
}

func TestBindQuery(t *testing.T) {
	values := url.Values{
		"name":    {"alice"},
		"age":     {"30"},
		"height":  {"1.75"},
		"active":  {"true"},
		"count":   {"9"},
		"tagged":  {"form-tag"},
		"quered":  {"query-tag"},
		"opts":    {"opt-value"},
		"skipped": {"not-bound"},
	}

	var out bindTarget
	require.NoError(t, BindQuery(values, &out))

	assert.Equal(t, "alice", out.Name)
	assert.Equal(t, 30, out.Age)
	assert.Equal(t, 1.75, out.Height)
	assert.True(t, out.Active)
	assert.Equal(t, uint(9), out.Count)
	assert.Equal(t, "", out.Ignored, "json:'-' fields must be skipped")
	assert.Equal(t, "form-tag", out.Tagged)
	assert.Equal(t, "query-tag", out.Quered)
	require.NotNil(t, out.Opts)
	assert.Equal(t, "opt-value", *out.Opts)
}

func TestBindQueryEmptyAndMissingValues(t *testing.T) {
	values := url.Values{"name": {""}, "age": {"12"}}
	var out bindTarget
	require.NoError(t, BindQuery(values, &out))
	assert.Equal(t, "", out.Name, "empty values are skipped")
	assert.Equal(t, 12, out.Age)
}

func TestBindQueryInvalidNumbers(t *testing.T) {
	tests := []struct {
		name   string
		values url.Values
	}{
		{"invalid int", url.Values{"age": {"not-a-number"}}},
		{"invalid uint", url.Values{"count": {"-1"}}},
		{"invalid float", url.Values{"height": {"abc"}}},
		{"invalid bool", url.Values{"active": {"maybe"}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out bindTarget
			assert.Error(t, BindQuery(tc.values, &out))
		})
	}
}

func TestBindQueryIgnoresNonStructTargets(t *testing.T) {
	var nilPtr *bindTarget
	assert.NoError(t, BindQuery(url.Values{"name": {"x"}}, nil))
	assert.NoError(t, BindQuery(url.Values{"name": {"x"}}, nilPtr))
	assert.NoError(t, BindQuery(url.Values{"name": {"x"}}, "not a struct pointer"))
}

func TestBindForm(t *testing.T) {
	body := strings.NewReader("name=bob&age=22&trimmed=kept")
	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var out bindTarget
	require.NoError(t, BindForm(req, &out))
	assert.Equal(t, "bob", out.Name)
	assert.Equal(t, 22, out.Age)
	assert.Equal(t, "kept", out.Trimmed)
}

// ---------------------------------------------------------------------------
// Lifecycle (hermetic: QUIC listener on a random local port)
// ---------------------------------------------------------------------------

func TestStartStopRestart(t *testing.T) {
	srv := NewServer(WithAddress("127.0.0.1:0"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- srv.Start(ctx) }()

	// Give the QUIC listener a moment to come up, then shut it down.
	time.Sleep(200 * time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	assert.NoError(t, srv.Stop(stopCtx))

	select {
	case err := <-done:
		// Start swallows http.ErrServerClosed and returns nil.
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return within 5s after Stop")
	}

	// A stopped http3.Server is permanently closed; the restart attempt must
	// fail fast instead of silently fake-starting.
	assert.EqualError(t, srv.Start(ctx),
		"http3 server cannot be restarted after Stop; create a new server instance")
}

func TestStopWithoutStart(t *testing.T) {
	srv := NewServer(WithAddress("127.0.0.1:0"))

	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Stop before Start must not flip the stopped latch (everStarted false).
	assert.NoError(t, srv.Stop(stopCtx))
	assert.NoError(t, srv.Start(context.Background()))
}

func TestJSONEncodingHelpers(t *testing.T) {
	// Sanity-check the error payload shape used by DefaultErrorEncoder.
	data, err := json.Marshal(map[string]any{"error": "x"})
	require.NoError(t, err)
	assert.Equal(t, `{"error":"x"}`, strings.TrimSpace(string(data)))
}
