package gcpubsub

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tx7do/go-wind-plugins/broker"
	"github.com/tx7do/go-wind-plugins/broker/gcpubsub"
	"github.com/tx7do/go-wind-plugins/metrics"

	"github.com/tx7do/go-wind-plugins/transport"
)

type Server struct {
	broker.Broker
	brokerOpts []broker.Option
	m          metrics.Metrics

	subscribers    broker.SubscriberMap
	subscriberOpts transport.SubscribeOptionMap

	sync.RWMutex
	started atomic.Bool

	baseCtx context.Context
	err     error
}

func NewServer(opts ...ServerOption) *Server {
	srv := &Server{
		baseCtx:        context.Background(),
		subscribers:    make(broker.SubscriberMap),
		subscriberOpts: make(transport.SubscribeOptionMap),
		brokerOpts:     []broker.Option{},
		started:        atomic.Bool{},
	}

	srv.init(opts...)

	return srv
}

func (s *Server) init(opts ...ServerOption) {
	for _, o := range opts {
		o(s)
	}

	s.Broker = gcpubsub.NewBroker(s.brokerOpts...)
}

func (s *Server) Name() string {
	return KindGCPPubSub
}

func (s *Server) Start(ctx context.Context) error {
	if s.err != nil {
		return s.err
	}

	if s.started.Load() {
		return nil
	}

	if s.err = s.Init(); s.err != nil {
		LogErrorf("init broker failed: [%s]", s.err.Error())
		return s.err
	}

	if s.err = s.Connect(); s.err != nil {
		LogErrorf("connect broker failed: [%s]", s.err.Error())
		return s.err
	}

	LogInfof("server listening on: %s", s.Address())

	if s.err = s.doRegisterSubscriberMap(); s.err != nil {
		return s.err
	}

	s.baseCtx = ctx
	s.started.Store(true)

	return nil
}

func (s *Server) Stop(ctx context.Context) error {
	if !s.started.Load() {
		return nil
	}

	LogInfo("server stopping...")

	for _, v := range s.subscribers {
		_ = v.Unsubscribe(false)
	}
	s.subscribers = make(broker.SubscriberMap)
	s.subscriberOpts = make(transport.SubscribeOptionMap)

	s.started.Store(false)
	err := s.Disconnect()
	s.err = nil

	LogInfo("server stopped.")

	return err
}

func (s *Server) RegisterSubscriber(topic string, handler broker.Handler, binder broker.Binder, opts ...broker.SubscribeOption) error {
	s.Lock()
	defer s.Unlock()

	if s.started.Load() {
		return s.doRegisterSubscriber(topic, handler, binder, opts...)
	} else {
		s.subscriberOpts[topic] = &transport.SubscribeOption{Handler: handler, Binder: binder, SubscribeOptions: opts}
	}
	return nil
}

func RegisterSubscriber[T any](srv *Server, topic string, handler func(context.Context, string, broker.Headers, *T) error, opts ...broker.SubscribeOption) error {
	return srv.RegisterSubscriber(topic,
		func(ctx context.Context, event broker.Event) error {
			switch t := event.Message().Body.(type) {
			case *T:
				if err := handler(ctx, event.Topic(), event.Message().Headers, t); err != nil {
					return err
				}
			default:
				return fmt.Errorf("unsupported type: %T", t)
			}
			return nil
		},
		func() any {
			var t T
			return &t
		},
		opts...,
	)
}

func (s *Server) doRegisterSubscriber(topic string, handler broker.Handler, binder broker.Binder, opts ...broker.SubscribeOption) error {
	handler = s.wrapHandler(topic, handler)

	sub, err := s.Subscribe(topic, handler, binder, opts...)
	if err != nil {
		return err
	}

	if _, exists := s.subscribers[topic]; exists {
		LogWarnf("subscriber for topic '%s' already exists, overwriting", topic)
	}
	s.subscribers[topic] = sub
	return nil
}

func (s *Server) doRegisterSubscriberMap() error {
	var errs []error
	for topic, opt := range s.subscriberOpts {
		if err := s.doRegisterSubscriber(topic, opt.Handler, opt.Binder, opt.SubscribeOptions...); err != nil {
			LogErrorf("register subscriber failed, topic: %s, error: %s", topic, err.Error())
			errs = append(errs, err)
		}
	}
	s.subscriberOpts = make(transport.SubscribeOptionMap)
	return errors.Join(errs...)
}

func (s *Server) Endpoint() string {
	return ""
}

func (s *Server) wrapHandler(topic string, handler broker.Handler) broker.Handler {
	if s.m == nil {
		return handler
	}
	return func(ctx context.Context, event broker.Event) error {
		startTime := time.Now()
		labels := map[string]string{
			"broker": s.Name(),
			"topic":  topic,
		}

		s.m.Counter(ctx, "broker.messages.received", 1, labels)

		err := handler(ctx, event)
		s.m.Histogram(ctx, "broker.message.duration", time.Since(startTime).Seconds(), labels)

		if err != nil {
			s.m.Counter(ctx, "broker.messages.errors", 1, map[string]string{
				"broker": s.Name(),
				"topic":  topic,
				"error":  "true",
			})
		}

		return err
	}
}
