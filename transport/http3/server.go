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
	"time"

	"github.com/gorilla/mux"
	"github.com/quic-go/quic-go/http3"
)

type Server struct {
	*http3.Server

	tlsConf *tls.Config
	timeout time.Duration

	err error

	filters []FilterFunc
	ms      []Middleware
	dec     DecodeRequestFunc
	enc     EncodeResponseFunc
	ene     EncodeErrorFunc

	router      *mux.Router
	strictSlash bool
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
	s.router.NotFoundHandler = http.DefaultServeMux
	s.router.MethodNotAllowedHandler = http.DefaultServeMux

	handler := s.filter()(s.router)
	s.Server.Handler = FilterChain(s.filters...)(handler)
}

func (s *Server) Name() string {
	return KindHTTP3
}

func (s *Server) Endpoint() string {
	return s.Addr
}

func (s *Server) Start(ctx context.Context) error {
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
	LogInfo("server stopping...")

	err := s.Close()
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
