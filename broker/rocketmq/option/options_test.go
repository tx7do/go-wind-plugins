package rocketmqOption

import (
	"reflect"
	"testing"
	"time"

	rmqClient "github.com/apache/rocketmq-clients/golang/v5"
	"github.com/tx7do/go-wind/log"

	"github.com/tx7do/go-wind-plugins/broker"
)

// optsFromOptions applies broker options and returns the resulting Options
// so context values can be inspected.
func optsFromOptions(t *testing.T, opts ...broker.Option) *broker.Options {
	t.Helper()
	o := broker.NewOptionsAndApply(opts...)
	return &o
}

// optsFromPublish applies publish options and returns the PublishOptions.
func optsFromPublish(t *testing.T, opts ...broker.PublishOption) broker.PublishOptions {
	t.Helper()
	return broker.NewPublishOptions(opts...)
}

// optsFromSubscribe applies subscribe options and returns the SubscribeOptions.
func optsFromSubscribe(t *testing.T, opts ...broker.SubscribeOption) broker.SubscribeOptions {
	t.Helper()
	return broker.NewSubscribeOptions(opts...)
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

func TestDefaultAddr(t *testing.T) {
	if DefaultAddr != "127.0.0.1:9876" {
		t.Errorf("DefaultAddr = %q, want %q", DefaultAddr, "127.0.0.1:9876")
	}
}

func TestDriverTypeValues(t *testing.T) {
	tests := []struct {
		name string
		got  DriverType
		want string
	}{
		{"aliyun", DriverTypeAliyun, "aliyun"},
		{"v2", DriverTypeV2, "v2"},
		{"v5", DriverTypeV5, "v5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.got) != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestMessageModelValues(t *testing.T) {
	if string(MessageModelBroadCasting) != "BroadCasting" {
		t.Errorf("MessageModelBroadCasting = %q, want BroadCasting", MessageModelBroadCasting)
	}
	if string(MessageModelClustering) != "Clustering" {
		t.Errorf("MessageModelClustering = %q, want Clustering", MessageModelClustering)
	}
}

func TestSpanAttributeConstants(t *testing.T) {
	// Spot-check the tracing attribute names that other drivers rely on.
	tests := []struct{ name, got, want string }{
		{"operation key", SPAN_ATTRIBUTE_KEY_ROCKETMQ_OPERATION, "messaging.rocketmq.operation"},
		{"tag key", SPAN_ATTRIBUTE_KEY_ROCKETMQ_TAG, "messaging.rocketmq.message_tag"},
		{"keys key", SPAN_ATTRIBUTE_KEY_ROCKETMQ_KEYS, "messaging.rocketmq.message_keys"},
		{"messaging system key", SPAN_ATTRIBUTE_KEY_MESSAGING_SYSTEM, "messaging.system"},
		{"messaging system value", SPAN_ATTRIBUTE_VALUE_ROCKETMQ_MESSAGING_SYSTEM, "rocketmq"},
		{"send operation", SPAN_ATTRIBUTE_VALUE_ROCKETMQ_SEND_OPERATION, "send"},
		{"protocol version", SPAN_ATTRIBUTE_VALUE_MESSAGING_PROTOCOL_VERSION, "v1"},
		{"transaction resolution", SPAN_ATTRIBUTE_KEY_TRANSACTION_RESOLUTION, "commitAction"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Broker-level Option functions
// ---------------------------------------------------------------------------

func TestBrokerOptions(t *testing.T) {
	tests := []struct {
		name  string
		apply broker.Option
		key   any
		want  any
	}{
		{"WithLoggerLevel", WithLoggerLevel(log.LevelWarn), LoggerLevelKey{}, log.LevelWarn},
		{"WithEnableTrace", WithEnableTrace(), EnableTraceKey{}, true},
		{"WithNameServer", WithNameServer([]string{"a:9876", "b:9876"}), NameServersKey{}, []string{"a:9876", "b:9876"}},
		{"WithNameServerDomain", WithNameServerDomain("https://example.com"), NameServerUrlKey{}, "https://example.com"},
		{"WithAccessKey", WithAccessKey("ak"), AccessKey{}, "ak"},
		{"WithSecretKey", WithSecretKey("sk"), SecretKey{}, "sk"},
		{"WithSecurityToken", WithSecurityToken("tok"), SecurityTokenKey{}, "tok"},
		{"WithRetryCount", WithRetryCount(3), RetryCountKey{}, 3},
		{"WithNamespace", WithNamespace("ns"), NamespaceKey{}, "ns"},
		{"WithInstanceName", WithInstanceName("inst"), InstanceNameKey{}, "inst"},
		{"WithGroupName", WithGroupName("grp"), GroupNameKey{}, "grp"},
		{"WithAwaitDuration", WithAwaitDuration(5 * time.Second), AwaitDurationKey{}, 5 * time.Second},
		{"WithMaxMessageNumKey", WithMaxMessageNumKey(32), MaxMessageNumKey{}, int32(32)},
		{"WithInvisibleDuration", WithInvisibleDuration(time.Minute), InvisibleDurationKey{}, time.Minute},
		{"WithReceiveInterval", WithReceiveInterval(2 * time.Second), ReceiveIntervalKey{}, 2 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := optsFromOptions(t, tt.apply)
			got := o.Context.Value(tt.key)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("%s: context value = %v (%T), want %v", tt.name, got, got, tt.want)
			}
		})
	}
}

func TestWithCredentials(t *testing.T) {
	o := optsFromOptions(t, WithCredentials("ak", "sk", "tok"))

	got, ok := o.Context.Value(CredentialsKey{}).(*Credentials)
	if !ok || got == nil {
		t.Fatalf("CredentialsKey value = %v (%T), want *Credentials", o.Context.Value(CredentialsKey{}), o.Context.Value(CredentialsKey{}))
	}
	if got.AccessKey != "ak" || got.AccessSecret != "sk" || got.SecurityToken != "tok" {
		t.Errorf("Credentials = %+v, want {ak sk tok}", got)
	}
}

func TestWithSubscriptionExpressions(t *testing.T) {
	fe := &rmqClient.FilterExpression{}
	o := optsFromOptions(t, WithSubscriptionExpressions(map[string]*rmqClient.FilterExpression{
		"topic-a": fe,
	}))

	got, ok := o.Context.Value(SubscriptionExpressionsKey{}).(map[string]*rmqClient.FilterExpression)
	if !ok {
		t.Fatalf("SubscriptionExpressionsKey value type = %T", o.Context.Value(SubscriptionExpressionsKey{}))
	}
	if len(got) != 1 || got["topic-a"] != fe {
		t.Errorf("SubscriptionExpressions = %v, want map[topic-a:<fe>]", got)
	}
}

// ---------------------------------------------------------------------------
// PublishOption functions
// ---------------------------------------------------------------------------

func TestPublishOptions(t *testing.T) {
	ts := time.Unix(1700000000, 0)

	tests := []struct {
		name  string
		apply broker.PublishOption
		key   any
		want  any
	}{
		{"WithCompress", WithCompress(true), CompressKey{}, true},
		{"WithBatch", WithBatch(true), BatchKey{}, true},
		{"WithProperties", WithProperties(map[string]string{"k": "v"}), PropertiesKey{}, map[string]string{"k": "v"}},
		{"WithDelayTimeLevel", WithDelayTimeLevel(4), DelayTimeLevelKey{}, 4},
		{"WithTag", WithTag("tag-1"), TagsKey{}, "tag-1"},
		{"WithKeys", WithKeys([]string{"a", "b"}), KeysKey{}, []string{"a", "b"}},
		{"WithShardingKey", WithShardingKey("shard"), ShardingKeyKey{}, "shard"},
		{"WithDeliveryTimestamp", WithDeliveryTimestamp(ts), DeliveryTimestampKey{}, ts},
		{"WithMessageGroup", WithMessageGroup("grp"), MessageGroupKey{}, "grp"},
		{"WithSendAsync", WithSendAsync(true), SendAsyncKey{}, true},
		{"WithSendWithTransaction", WithSendWithTransaction(true), SendWithTransactionKey{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := optsFromPublish(t, tt.apply)
			got := o.Context.Value(tt.key)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("%s: context value = %v (%T), want %v", tt.name, got, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// SubscribeOption functions
// ---------------------------------------------------------------------------

func TestSubscribeOptions(t *testing.T) {
	fe := &rmqClient.FilterExpression{}
	o := optsFromSubscribe(t,
		WithSubscriptionFilterExpression(fe),
		WithConsumerModel(MessageModelBroadCasting),
	)

	if got := o.Context.Value(SubscriptionFilterExpressionKey{}); got != fe {
		t.Errorf("SubscriptionFilterExpression = %v, want the injected filter expression", got)
	}
	if got := o.Context.Value(ConsumerModelKey{}); got != MessageModelBroadCasting {
		t.Errorf("ConsumerModel = %v, want %v", got, MessageModelBroadCasting)
	}
}

// ---------------------------------------------------------------------------
// WarnUnsupportedKeysOnce / unsupported key lists
// ---------------------------------------------------------------------------

func TestWarnUnsupportedKeysOnce_NilContext(t *testing.T) {
	// Must not panic on a nil context.
	WarnUnsupportedKeysOnce("aliyun", nil, AliyunUnsupportedBrokerKeys())
}

func TestWarnUnsupportedKeysOnce_WarnsOncePerDriverAndKey(t *testing.T) {
	// Clean the warning registry for the driver under test.
	warnedKeys.Delete("testdriver/WithRetryCount")

	ctx := broker.NewOptionsAndApply(WithRetryCount(3)).Context

	WarnUnsupportedKeysOnce("testdriver", ctx, []keySupport{{RetryCountKey{}, "WithRetryCount"}})
	WarnUnsupportedKeysOnce("testdriver", ctx, []keySupport{{RetryCountKey{}, "WithRetryCount"}})

	// The registry must contain the driver/key tag after the first call.
	if _, ok := warnedKeys.Load("testdriver/WithRetryCount"); !ok {
		t.Error("warnedKeys missing \"testdriver/WithRetryCount\" after warning call")
	}

	// A different driver tag must be independent.
	warnedKeys.Delete("otherdriver/WithRetryCount")
	WarnUnsupportedKeysOnce("otherdriver", ctx, []keySupport{{RetryCountKey{}, "WithRetryCount"}})
	if _, ok := warnedKeys.Load("otherdriver/WithRetryCount"); !ok {
		t.Error("warnedKeys missing \"otherdriver/WithRetryCount\"")
	}
}

func TestWarnUnsupportedKeysOnce_AbsentKeyNotWarned(t *testing.T) {
	warnedKeys.Delete("absentdriver/WithRetryCount")

	// Empty context: none of the unsupported options are present, so no tag
	// must be registered.
	ctx := broker.NewOptionsAndApply().Context
	WarnUnsupportedKeysOnce("absentdriver", ctx, []keySupport{{RetryCountKey{}, "WithRetryCount"}})

	if _, ok := warnedKeys.Load("absentdriver/WithRetryCount"); ok {
		t.Error("warnedKeys registered a tag for an option that was never set")
	}
}

func TestUnsupportedKeyLists(t *testing.T) {
	tests := []struct {
		name    string
		list    []keySupport
		wantLen int
	}{
		{"AliyunUnsupportedBrokerKeys", AliyunUnsupportedBrokerKeys(), 8},
		{"AliyunUnsupportedPublishKeys", AliyunUnsupportedPublishKeys(), 6},
		{"V2UnsupportedBrokerKeys", V2UnsupportedBrokerKeys(), 5},
		{"V2UnsupportedPublishKeys", V2UnsupportedPublishKeys(), 4},
		{"V2UnsupportedSubscribeKeys", V2UnsupportedSubscribeKeys(), 1},
		{"V5UnsupportedBrokerKeys", V5UnsupportedBrokerKeys(), 1},
		{"V5UnsupportedPublishKeys", V5UnsupportedPublishKeys(), 4},
		{"V5UnsupportedSubscribeKeys", V5UnsupportedSubscribeKeys(), 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.list) != tt.wantLen {
				t.Errorf("%s length = %d, want %d", tt.name, len(tt.list), tt.wantLen)
			}
			// Every entry must have a non-empty option name.
			for _, k := range tt.list {
				if k.name == "" {
					t.Errorf("%s contains an entry without a name", tt.name)
				}
				if k.key == nil {
					t.Errorf("%s entry %q has a nil key", tt.name, k.name)
				}
			}
		})
	}
}
