package activemq

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tx7do/go-wind-plugins/broker"
	"github.com/tx7do/go-wind-plugins/broker/stomp"
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

	s.Broker = stomp.NewBroker(s.brokerOpts...)
}

func (s *Server) Name() string {
	return KindActiveMQ
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

	// 先置位再注册订阅：与 Stop 的交接由 doRegisterSubscriber 的 started 复查处理。
	s.started.Store(true)


	if s.err = s.doRegisterSubscriberMap(); s.err != nil {
		return s.err
	}

	s.baseCtx = ctx

	return nil
}

func (s *Server) Stop(ctx context.Context) error {
	if !s.started.Load() {
		// 允许 Start 失败后重试：清掉残留的错误状态
		s.err = nil
		return nil
	}

	LogInfo("server stopping...")

	s.started.Store(false)

	// 持锁快照订阅表，避免与并发的 RegisterSubscriber 竞争
	s.Lock()
	subs := s.subscribers
	s.subscribers = make(broker.SubscriberMap)
	s.Unlock()

	for _, v := range subs {
		_ = v.Unsubscribe(false)
	}

	// 保留 subscriberOpts，下一次 Start 会通过 doRegisterSubscriberMap 重新注册

	err := s.Disconnect()
	s.err = nil


	LogInfo("server stopped.")

	return err
}

func (s *Server) RegisterSubscriber(ctx context.Context, topic string, handler broker.Handler, binder broker.Binder, opts ...broker.SubscribeOption) error {
	s.Lock()
	if s.baseCtx == nil {
		s.baseCtx = context.Background()
	}
	if ctx == nil {
		ctx = s.baseCtx
	}

	// context必须要插入到头部，否则后续传入的配置会被覆盖掉。
	opts = append([]broker.SubscribeOption{broker.WithSubscribeContext(ctx)}, opts...)
	started := s.started.Load()
	if !started {
		s.subscriberOpts[topic] = &transport.SubscribeOption{Handler: handler, Binder: binder, SubscribeOptions: opts}
		s.Unlock()
		return nil
	}
	s.Unlock()

	// 订阅动作放在锁外执行，避免 broker 阻塞拖住整个注册面
	return s.doRegisterSubscriber(topic, handler, binder, opts...)
}

func RegisterSubscriber[T any](srv *Server, ctx context.Context, topic string, handler func(context.Context, string, broker.Headers, *T) error, opts ...broker.SubscribeOption) error {
	return srv.RegisterSubscriber(ctx,
		topic,
		func(ctx context.Context, event broker.Event) error {
			if event == nil || event.Message() == nil || event.Message().Body == nil {
				return fmt.Errorf("event or message body is nil")
			}

			var zero T
			expectedType := fmt.Sprintf("%T", &zero)

			switch t := event.Message().Body.(type) {
			case *T:
				if err := handler(ctx, event.Topic(), event.Message().Headers, t); err != nil {
					return err
				}
			case T:
				if err := handler(ctx, event.Topic(), event.Message().Headers, &t); err != nil {
					return err
				}
			default:
				return fmt.Errorf("unsupported type: expected %s, got %T", expectedType, event.Message().Body)
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

	var old broker.Subscriber
	exists := false
	started := false

	s.Lock()
	started = s.started.Load()
	if started {
		old, exists = s.subscribers[topic]
		s.subscribers[topic] = sub
	}
	// 记录注册参数：Stop 后保留在缓存表中，Start→Stop→Start 时重新注册
	s.subscriberOpts[topic] = &transport.SubscribeOption{Handler: handler, Binder: binder, SubscribeOptions: opts}
	s.Unlock()

	if !started {
		// 与 Stop 竞态：服务已停止，新建的订阅立即退订，等待下次 Start 重新注册
		LogWarnf("server stopped, subscription for '%s' deferred to next start", topic)
		_ = sub.Unsubscribe(false)
		return nil
	}

	if exists {
		// 旧订阅先退订，避免旧订阅继续消费造成泄漏
		LogWarnf("subscriber for '%s' already exists, unsubscribing the old one", topic)
		if uerr := old.Unsubscribe(false); uerr != nil {
			LogErrorf("unsubscribe old subscriber for '%s' failed: %s", topic, uerr.Error())
		}
	}
	return nil
}

func (s *Server) doRegisterSubscriberMap() error {
	// 持锁取出并清空缓存表，避免与并发的 RegisterSubscriber 竞争
	s.Lock()
	optsMap := s.subscriberOpts
	s.subscriberOpts = make(transport.SubscribeOptionMap)
	s.Unlock()

	var errs []error
	for topic, opt := range optsMap {
		if err := s.doRegisterSubscriber(topic, opt.Handler, opt.Binder, opt.SubscribeOptions...); err != nil {
			LogErrorf("register subscriber failed, topic: %s, error: %s", topic, err.Error())
			errs = append(errs, err)
		}
	}
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
