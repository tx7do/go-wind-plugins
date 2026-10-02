package stream

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/tx7do/go-wind-plugins/broker"
	redisOption "github.com/tx7do/go-wind-plugins/broker/redis/option"
)

const (
	defaultBroker = "redis://127.0.0.1:6379"
)

type streamBroker struct {
	addr       string
	pool       *redis.Pool
	options    broker.Options
	commonOpts *redisOption.CommonOptions

	subscribers *broker.SubscriberSyncMap
}

func NewBroker(opts ...broker.Option) broker.Broker {
	commonOpts := &redisOption.CommonOptions{
		MaxIdle:        redisOption.DefaultMaxIdle,
		MaxActive:      redisOption.DefaultMaxActive,
		IdleTimeout:    redisOption.DefaultIdleTimeout,
		ConnectTimeout: redisOption.DefaultConnectTimeout,
		ReadTimeout:    redisOption.DefaultReadTimeout,
		WriteTimeout:   redisOption.DefaultWriteTimeout,
	}

	options := broker.NewOptionsAndApply(opts...)

	if l := broker.LoggerFromOptions(&options); l != nil {
		redisOption.SetLogger(l)
	}

	return &streamBroker{
		options:     options,
		commonOpts:  commonOpts,
		subscribers: broker.NewSubscriberSyncMap(),
	}
}

func (b *streamBroker) Name() string {
	return "redis-stream"
}

func (b *streamBroker) Options() broker.Options {
	return b.options
}

func (b *streamBroker) Address() string {
	return b.addr
}

func (b *streamBroker) Init(opts ...broker.Option) error {
	if b.pool != nil {
		// Connect 先于 Init 是公开 API 的合法顺序（如先 srv.Connect() 再 srv.Start()），
		// 此时无新选项的 Init 幂等成功；带新选项的 Init 会改写活跃连接池感知不到的配置，仍然拒绝。
		if len(opts) == 0 {
			return nil
		}
		return errors.New("redis-stream: cannot init while connected")
	}

	b.options.Apply(opts...)

	if v, ok := b.options.Context.Value(redisOption.OptionsKey).(*redisOption.CommonOptions); ok && v != nil {
		b.commonOpts = v
	}

	b.addr = normalizeAddr(b.options.Addrs)

	return nil
}

func normalizeAddr(addressList []string) string {
	if len(addressList) == 0 || addressList[0] == "" {
		return defaultBroker
	}

	addr := addressList[0]
	if !strings.HasPrefix(addr, "redis://") {
		addr = "redis://" + addr
	}

	return addr
}

// enableTLSScheme 把 redis:// scheme 改写为 rediss://，使 DialURL 以 TLS 拨号。
// 已经是 rediss:// 的地址原样返回。
func enableTLSScheme(addr string) string {
	return strings.Replace(addr, "redis://", "rediss://", 1)
}

func (b *streamBroker) Connect() error {
	if b.pool != nil {
		return nil
	}

	if b.addr == "" {
		b.addr = normalizeAddr(b.options.Addrs)
	}

	// TLS 透传：redigo 的 DialURL 只按 URL scheme（rediss://）决定是否启用 TLS
	// （DialUseTLS 会被 DialURL 忽略），因此这里改写 scheme 而不是传 DialUseTLS。
	addr := b.addr
	if b.options.Secure || b.options.TLSConfig != nil {
		addr = enableTLSScheme(addr)
	}

	b.pool = &redis.Pool{
		MaxIdle:     b.commonOpts.MaxIdle,
		MaxActive:   b.commonOpts.MaxActive,
		IdleTimeout: b.commonOpts.IdleTimeout,
		Dial: func() (redis.Conn, error) {
			dialOpts := []redis.DialOption{
				redis.DialConnectTimeout(b.commonOpts.ConnectTimeout),
				redis.DialReadTimeout(redisOption.DefaultHealthCheckPeriod + b.commonOpts.ReadTimeout),
				redis.DialWriteTimeout(b.commonOpts.WriteTimeout),
				redis.DialPassword(b.commonOpts.Password),
			}
			if b.options.TLSConfig != nil {
				dialOpts = append(dialOpts, redis.DialTLSConfig(b.options.TLSConfig))
			}
			return redis.DialURL(addr, dialOpts...)
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			if nil != err {
				redisOption.LogError("ping error:" + err.Error())
			}
			return err
		},
	}

	return nil
}

func (b *streamBroker) Disconnect() error {
	if b.pool == nil {
		return nil
	}
	err := b.pool.Close()
	b.pool = nil
	b.addr = ""

	b.subscribers.Clear()

	return err
}

func (b *streamBroker) Request(ctx context.Context, topic string, msg *broker.Message, opts ...broker.RequestOption) (*broker.Message, error) {
	return broker.GenericRequest(ctx, b, topic, msg, opts...)
}

func (b *streamBroker) Publish(ctx context.Context, topic string, msg *broker.Message, opts ...broker.PublishOption) error {
	var finalTask = b.internalPublish

	if len(b.options.PublishMiddlewares) > 0 {
		finalTask = broker.ChainPublishMiddleware(finalTask, b.options.PublishMiddlewares)
	}

	return finalTask(ctx, topic, msg, opts...)
}

func (b *streamBroker) internalPublish(ctx context.Context, topic string, msg *broker.Message, opts ...broker.PublishOption) error {
	buf, err := broker.Marshal(b.options.Codec, msg.Body)
	if err != nil {
		return err
	}

	sendMsg := msg.Clone()
	sendMsg.Body = buf

	return b.publish(ctx, topic, sendMsg, opts...)
}

// publish 使用 XADD 命令将消息写入 Redis Stream
func (b *streamBroker) publish(_ context.Context, stream string, msg *broker.Message, opts ...broker.PublishOption) error {
	if b.pool == nil {
		return errors.New("redis-stream: not connected")
	}

	publishOpts := broker.PublishOptions{
		Context: context.Background(),
	}
	for _, o := range opts {
		o(&publishOpts)
	}

	conn := b.pool.Get()
	defer conn.Close()

	args := []any{stream}

	// MAXLEN 限制
	var maxLen int64
	if v, ok := publishOpts.Context.Value(redisOption.StreamMaxLenKey{}).(int64); ok {
		maxLen = v
	}
	if maxLen > 0 {
		args = append(args, "MAXLEN", "~", maxLen)
	}

	args = append(args, "*")
	args = append(args, "body", msg.BodyBytes())

	// 附加消息头（值转为 string）
	for k, v := range msg.Headers {
		args = append(args, k, fmt.Sprintf("%v", v))
	}

	_, err := conn.Do("XADD", args...)
	return err
}

func (b *streamBroker) Subscribe(topic string, handler broker.Handler, binder broker.Binder, opts ...broker.SubscribeOption) (broker.Subscriber, error) {
	if b.pool == nil {
		return nil, errors.New("redis-stream: not connected")
	}

	subOpts := broker.NewSubscribeOptions(opts...)

	if len(b.options.SubscriberMiddlewares) > 0 {
		handler = broker.ChainSubscriberMiddleware(handler, b.options.SubscriberMiddlewares)
	}

	// 提取 Stream 专属配置
	group := redisOption.DefaultStreamGroup
	if v, ok := subOpts.Context.Value(redisOption.StreamGroupKey{}).(string); ok && v != "" {
		group = v
	}

	consumer := redisOption.DefaultStreamConsumer
	if v, ok := subOpts.Context.Value(redisOption.StreamConsumerKey{}).(string); ok && v != "" {
		consumer = v
	}

	blockTime := redisOption.DefaultStreamBlockTime
	if v, ok := subOpts.Context.Value(redisOption.StreamBlockTimeKey{}).(time.Duration); ok && v > 0 {
		blockTime = v
	}

	count := redisOption.DefaultStreamCount
	if v, ok := subOpts.Context.Value(redisOption.StreamCountKey{}).(int); ok && v > 0 {
		count = v
	}

	// 确保消费组存在
	if err := b.ensureGroup(topic, group); err != nil {
		return nil, err
	}

	sub := &subscriber{
		b:         b,
		topic:     topic,
		group:     group,
		consumer:  consumer,
		blockTime: blockTime,
		count:     count,
		handler:   handler,
		binder:    binder,
		options:   subOpts,
	}

	if old := b.subscribers.Get(topic); old != nil {
		// 同主题重复订阅：先退订旧订阅，避免旧订阅继续消费（泄漏 + 重复消费）
		if uerr := old.Unsubscribe(false); uerr != nil {
			redisOption.LogWarnf("unsubscribe old subscriber for topic %q failed: %v", topic, uerr)
		}
	}
	b.subscribers.Add(topic, sub)

	go sub.recv()

	return sub, nil
}

// ensureGroup 确保消费组存在，不存在则创建
func (b *streamBroker) ensureGroup(stream, group string) error {
	conn := b.pool.Get()
	defer conn.Close()

	// XGROUP CREATE stream group $ MKSTREAM
	_, err := conn.Do("XGROUP", "CREATE", stream, group, "$", "MKSTREAM")
	if err != nil {
		// BUSYGROUP: Consumer Group name already exists 是正常的
		if strings.Contains(err.Error(), "BUSYGROUP") {
			return nil
		}
		return err
	}
	return nil
}
