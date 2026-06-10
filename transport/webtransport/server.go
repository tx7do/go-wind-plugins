package webtransport

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go"

	"github.com/tx7do/go-wind-plugins/broker"
	"github.com/tx7do/go-wind-plugins/encoding"
	"github.com/tx7do/go-wind/transport"

	"github.com/quic-go/quic-go/http3"
)

var (
	_ transport.Server = (*Server)(nil)
)

type Server struct {
	*http3.Server

	tlsConf  *tls.Config
	endpoint *url.URL
	timeout  time.Duration

	mux         *http.ServeMux
	path        string
	strictSlash bool

	ctx       context.Context // is closed when Close is called
	ctxCancel context.CancelFunc
	refCount  sync.WaitGroup

	messageHandlers MessageHandlerMap
	connectHandler  ConnectHandler
	codec           encoding.Codec

	sessionCount atomic.Int64
}

func NewServer(opts ...ServerOption) *Server {
	ctx, ctxCancel := context.WithCancel(context.Background())
	srv := &Server{
		ctx:       ctx,
		ctxCancel: ctxCancel,
		mux:       http.NewServeMux(),

		messageHandlers: make(MessageHandlerMap),
		codec:           encoding.GetCodec("json"),
	}
	srv.init(opts...)
	return srv
}

func (s *Server) init(opts ...ServerOption) {
	const idleTimeout = 30 * time.Second

	s.Server = &http3.Server{
		Addr: ":443",
		QUICConfig: &quic.Config{
			MaxIdleTimeout:  idleTimeout,
			KeepAlivePeriod: idleTimeout / 2,
		},
	}

	for _, o := range opts {
		o(s)
	}

	if s.tlsConf == nil {
		s.tlsConf = generateTLSConfig(alpnQuicTransport)
	}
	s.Server.TLSConfig = s.tlsConf

	if s.timeout == 0 {
		s.timeout = 5 * time.Second
	}

	// Advertise WebTransport support via HTTP/3 SETTINGS
	if s.Server.AdditionalSettings == nil {
		s.Server.AdditionalSettings = make(map[uint64]uint64)
	}
	s.Server.AdditionalSettings[settingsEnableWebtransport] = 1

	s.mux.HandleFunc(s.path, s.addHandler)
	s.Server.Handler = s.mux
}

func (s *Server) Endpoint() string {
	if err := s.listenAndEndpoint(); err != nil {
		return ""
	}
	if s.endpoint == nil {
		return ""
	}
	return s.endpoint.String()
}

func (s *Server) listenAndEndpoint() error {
	if s.endpoint == nil {
		host, port, err := net.SplitHostPort(s.Addr)
		if err != nil {
			return err
		}

		if host == "" || host == "0.0.0.0" {
			host = "localhost"
		}

		addr := host + ":" + fmt.Sprint(port)
		s.endpoint = &url.URL{Scheme: "https", Host: addr}
	}

	return nil
}

func (s *Server) Start(_ context.Context) error {
	if err := s.listenAndEndpoint(); err != nil {
		return err
	}

	LogInfof("server listening on: %s", s.Addr)

	if err := s.ListenAndServe(); err != nil {
		if !errors.Is(err, http.ErrServerClosed) {
			LogErrorf("start server failed: %s", err.Error())
			return err
		}
	}

	return nil
}

func (s *Server) Stop(_ context.Context) error {
	LogInfo("server stopping...")

	if s.ctxCancel != nil {
		s.ctxCancel()
	}

	err := s.Server.Close()
	s.refCount.Wait()

	LogInfo("server stopped.")

	return err
}

func (s *Server) RegisterMessageHandler(messageType MessageType, handler MessageHandler, binder Binder) {
	if _, ok := s.messageHandlers[messageType]; ok {
		return
	}

	s.messageHandlers[messageType] = HandlerData{
		handler, binder,
	}
}

func (s *Server) DeregisterMessageHandler(messageType MessageType) {
	delete(s.messageHandlers, messageType)
}

func (s *Server) marshalMessage(messageType MessageType, message MessagePayload) ([]byte, error) {
	var err error
	var msg Message
	msg.Type = messageType
	msg.Body, err = broker.Marshal(s.codec, message)
	if err != nil {
		return nil, err
	}

	buff, err := msg.Marshal()
	if err != nil {
		return nil, err
	}

	return buff, nil
}

func (s *Server) messageHandler(sessionId SessionID, buf []byte) error {
	var msg Message
	if err := msg.Unmarshal(buf); err != nil {
		LogErrorf("decode message exception: %s", err)
		return err
	}

	handlerData, ok := s.messageHandlers[msg.Type]
	if !ok {
		LogError("message type not found:", msg.Type)
		return errors.New("message handler not found")
	}

	var payload MessagePayload

	if handlerData.Binder != nil {
		payload = handlerData.Binder()

		if err := broker.Unmarshal(s.codec, msg.Body, &payload); err != nil {
			LogErrorf("unmarshal message exception: %s", err)
			return err
		}
	} else {
		payload = msg.Body
	}

	if err := handlerData.Handler(sessionId, payload); err != nil {
		LogErrorf("message handler exception: %s", err)
		return err
	}

	return nil
}

// addHandler handles WebTransport CONNECT requests.
// It validates the request, hijacks the HTTP/3 stream, and enters a read loop
// to process incoming messages.
func (s *Server) addHandler(w http.ResponseWriter, r *http.Request) {
	// Validate WebTransport CONNECT request
	if r.Method != http.MethodConnect {
		http.Error(w, "expected CONNECT request", http.StatusMethodNotAllowed)
		return
	}

	if r.Proto != protocolHeader {
		http.Error(w, "invalid protocol", http.StatusBadRequest)
		return
	}

	// Accept the session by sending 200 OK
	w.WriteHeader(http.StatusOK)
	w.(http.Flusher).Flush()

	// Hijack the HTTP/3 stream for bidirectional communication
	hijacker, ok := w.(http3.HTTPStreamer)
	if !ok {
		LogError("response writer does not support HTTPStreamer")
		http.Error(w, "stream hijacking not supported", http.StatusInternalServerError)
		return
	}

	stream := hijacker.HTTPStream()

	// Generate session ID from stream ID
	sessionId := SessionID(stream.StreamID())

	s.sessionCount.Add(1)

	// Notify connect handler
	if s.connectHandler != nil {
		s.connectHandler(sessionId, true)
	}

	s.refCount.Add(1)
	go func() {
		defer s.refCount.Done()
		defer func() {
			s.sessionCount.Add(-1)
			if s.connectHandler != nil {
				s.connectHandler(sessionId, false)
			}
		}()

		s.serveSession(sessionId, stream)
	}()
}

// serveSession reads messages from the hijacked stream and dispatches them
// to the registered message handlers.
func (s *Server) serveSession(sessionId SessionID, stream *http3.Stream) {
	buf := make([]byte, 4096)

	for {
		// Read message length (4 bytes, little-endian uint32)
		var sizeBuf [4]byte
		if _, err := io.ReadFull(stream, sizeBuf[:]); err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
				LogErrorf("session %d: read size error: %s", sessionId, err)
			}
			return
		}

		// Decode message length
		msgLen := uint(sizeBuf[0]) | uint(sizeBuf[1])<<8 | uint(sizeBuf[2])<<16 | uint(sizeBuf[3])<<24
		if msgLen == 0 {
			continue
		}

		// Grow buffer if needed
		if uint(cap(buf)) < msgLen {
			buf = make([]byte, msgLen)
		}
		data := buf[:msgLen]

		if _, err := io.ReadFull(stream, data); err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
				LogErrorf("session %d: read data error: %s", sessionId, err)
			}
			return
		}

		// Dispatch to message handler
		if err := s.messageHandler(sessionId, data); err != nil {
			LogErrorf("session %d: message handler error: %s", sessionId, err)
		}
	}
}
