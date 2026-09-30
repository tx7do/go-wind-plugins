package http3

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gorilla/mux"
	"github.com/quic-go/quic-go/http3"
)

// Middleware 是标准 HTTP 中间件类型。
// 使用类型别名使得 transport/http/middleware 下的中间件可以直接复用。
type Middleware = func(http.Handler) http.Handler

type Server struct {
	*http3.Server

	tlsConf *tls.Config
	timeout time.Duration

	err error

	filters     []FilterFunc
	hms         []HandlerMiddleware
	middlewares []Middleware
	dec         DecodeRequestFunc
	enc         EncodeResponseFunc
	ene         EncodeErrorFunc

	router      *mux.Router
	strictSlash bool

	stopped     atomic.Bool
	everStarted atomic.Bool
}

func NewServer(opts ...ServerOption) *Server {
	srv := &Server{
		timeout:     1 * time.Second,
		dec:         DefaultRequestDecoder,
		enc:         DefaultResponseEncoder,
		ene:         DefaultErrorEncoder,
		strictSlash: true,
	}

	srv.init(opts...)

	return srv
}

func (s *Server) init(opts ...ServerOption) {
	s.Server = &http3.Server{
		Addr: ":443",
	}

	for _, o := range opts {
		o(s)
	}

	if s.tlsConf == nil {
		s.tlsConf = s.generateTLSConfig()
	}
	s.Server.TLSConfig = s.tlsConf

	s.router = mux.NewRouter().StrictSlash(s.strictSlash)
	// 未匹配请求显式 404：指向 DefaultServeMux 会把 /debug/pprof 等全局路由暴露出去
	s.router.NotFoundHandler = http.NotFoundHandler()
	s.router.MethodNotAllowedHandler = http.NotFoundHandler()

	// 应用中间件链：先应用标准 HTTP 中间件，再应用 FilterFunc
	h := http.Handler(s.router)
	for i := len(s.middlewares) - 1; i >= 0; i-- {
		h = s.middlewares[i](h)
	}
	handler := s.filter()(h)
	s.Server.Handler = FilterChain(s.filters...)(handler)
}

func (s *Server) Name() string {
	return KindHTTP3
}

func (s *Server) Endpoint() string {
	return s.Addr
}

func (s *Server) Start(ctx context.Context) error {
	if s.stopped.Load() {
		// quic-go 的 http3.Server 一旦 Close/Shutdown 便永久失效，
		// 静默重启只会得到“假启动”，这里显式报错
		return errors.New("http3 server cannot be restarted after Stop; create a new server instance")
	}

	s.everStarted.Store(true)

	LogInfof("server listening on: %s", s.Addr)

	if err := s.ListenAndServe(); err != nil {
		if !errors.Is(err, http.ErrServerClosed) {
			LogErrorf("start server failed: %s", err.Error())
			return err
		}
	}

	return nil
}

func (s *Server) Stop(ctx context.Context) error {
	if s.everStarted.Load() {
		s.stopped.Store(true)
	}

	LogInfo("server stopping...")

	// 优先优雅关闭（发 GOAWAY、等待在途请求），
	// ctx 取消或超时后降级为硬关闭
	err := s.Shutdown(ctx)
	if err != nil {
		LogWarnf("graceful shutdown failed, closing: %s", err.Error())
		err = s.Close()
	}
	s.err = nil

	LogInfo("server stopped.")

	return err
}

func (s *Server) Route(prefix string, filters ...FilterFunc) *Router {
	return newRouter(prefix, s, filters...)
}

func (s *Server) Handle(path string, h http.Handler) {
	s.router.Handle(path, h)
}

func (s *Server) HandlePrefix(prefix string, h http.Handler) {
	s.router.PathPrefix(prefix).Handler(h)
}

func (s *Server) HandleFunc(path string, h http.HandlerFunc) {
	s.router.HandleFunc(path, h)
}

func (s *Server) HandleHeader(key, val string, h http.HandlerFunc) {
	s.router.Headers(key, val).Handler(h)
}

// Use 注册全局标准 HTTP 中间件，对所有路由生效。
// 支持直接使用 transport/http/middleware 下的中间件，例如：
//
//	srv.Use(tracing.Middleware())
//	srv.Use(metrics.Middleware(myMetrics))
//
// 必须在 Start 之前调用。
func (s *Server) Use(middlewares ...Middleware) {
	s.middlewares = append(s.middlewares, middlewares...)
}

func (s *Server) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	s.Handler.ServeHTTP(res, req)
}

func (s *Server) filter() mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			var (
				ctx    context.Context
				cancel context.CancelFunc
			)
			if s.timeout > 0 {
				ctx, cancel = context.WithTimeout(req.Context(), s.timeout)
			} else {
				ctx, cancel = context.WithCancel(req.Context())
			}
			defer cancel()

			next.ServeHTTP(w, req.WithContext(ctx))
		})
	}
}

func (s *Server) generateTLSConfig() *tls.Config {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	template := x509.Certificate{SerialNumber: big.NewInt(1)}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		panic(err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"h3"},
	}
}
