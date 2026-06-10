package http3

import (
	"crypto/tls"
	"time"
)

type ServerOption func(*Server)

func WithTLSConfig(c *tls.Config) ServerOption {
	return func(o *Server) {
		o.tlsConf = c
	}
}

func WithAddress(addr string) ServerOption {
	return func(s *Server) {
		s.Addr = addr
	}
}

func WithTimeout(timeout time.Duration) ServerOption {
	return func(s *Server) {
		s.timeout = timeout
	}
}

func WithMiddleware(m ...Middleware) ServerOption {
	return func(o *Server) {
		o.ms = m
	}
}

func WithFilter(filters ...FilterFunc) ServerOption {
	return func(o *Server) {
		o.filters = filters
	}
}

func WithRequestDecoder(dec DecodeRequestFunc) ServerOption {
	return func(o *Server) {
		o.dec = dec
	}
}

func WithResponseEncoder(en EncodeResponseFunc) ServerOption {
	return func(o *Server) {
		o.enc = en
	}
}

func WithErrorEncoder(en EncodeErrorFunc) ServerOption {
	return func(o *Server) {
		o.ene = en
	}
}

func WithStrictSlash(strictSlash bool) ServerOption {
	return func(o *Server) {
		o.strictSlash = strictSlash
	}
}
