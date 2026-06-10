package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"

	natsGo "github.com/nats-io/nats.go"

	"github.com/stretchr/testify/assert"

	"github.com/tx7do/go-wind-plugins/broker"
	api "github.com/tx7do/go-wind-plugins/testing/api/manual"
)

///////////////////////////////////////////////////////////////////////////////
/// Basic JetStream Publish / Subscribe
///////////////////////////////////////////////////////////////////////////////

func TestJetStream_Publish_WithRawData(t *testing.T) {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	ctx := context.Background()

	b := NewJetStreamBroker(
		broker.WithAddress(localBroker),
	)

	_ = b.Init()

	if err := b.Connect(); err != nil {
		t.Logf("cant connect to broker, skip: %v", err)
		t.Skip()
	}
	defer b.Disconnect()

	// Ensure stream exists
	js := GetJetStreamContext(b)
	if js != nil {
		_, _ = js.AddStream(&natsGo.StreamConfig{
			Name:     "TEST",
			Subjects: []string{"test_topic"},
		})
	}

	var msg api.Hygrothermograph
	const count = 10
	for i := 0; i < count; i++ {
		startTime := time.Now()
		msg.Humidity = float64(rand.Intn(100))
		msg.Temperature = float64(rand.Intn(100))
		buf, _ := json.Marshal(&msg)
		err := b.Publish(ctx, testTopic, broker.NewMessage(buf))
		assert.Nil(t, err)
		elapsedTime := time.Since(startTime) / time.Millisecond
		fmt.Printf("JS Publish %d, elapsed time: %dms, Humidity: %.2f Temperature: %.2f\n",
			i, elapsedTime, msg.Humidity, msg.Temperature)
	}

	fmt.Printf("total send %d messages\n", count)

	<-interrupt
}

func TestJetStream_Subscribe_WithRawData(t *testing.T) {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	b := NewJetStreamBroker(
		broker.WithAddress(localBroker),
	)

	_ = b.Init()

	if err := b.Connect(); err != nil {
		t.Logf("cant connect to broker, skip: %v", err)
		t.Skip()
	}
	defer b.Disconnect()

	// Ensure stream exists
	js := GetJetStreamContext(b)
	if js != nil {
		_, _ = js.AddStream(&natsGo.StreamConfig{
			Name:     "TEST",
			Subjects: []string{"test_topic"},
		})
	}

	_, err := b.Subscribe(testTopic,
		RegisterHygrothermographHandler(handleHygrothermograph),
		nil,
		WithDeliverNew(),
		WithDurable("test-durable"),
	)
	assert.Nil(t, err)

	<-interrupt
}

func TestJetStream_Publish_WithJsonCodec(t *testing.T) {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	ctx := context.Background()

	b := NewJetStreamBroker(
		broker.WithAddress(localBroker),
		broker.WithCodec("json"),
	)

	_ = b.Init()

	if err := b.Connect(); err != nil {
		t.Logf("cant connect to broker, skip: %v", err)
		t.Skip()
	}
	defer b.Disconnect()

	js := GetJetStreamContext(b)
	if js != nil {
		_, _ = js.AddStream(&natsGo.StreamConfig{
			Name:     "TEST",
			Subjects: []string{"test_topic"},
		})
	}

	var msg api.Hygrothermograph
	const count = 10
	for i := 0; i < count; i++ {
		startTime := time.Now()
		msg.Humidity = float64(rand.Intn(100))
		msg.Temperature = float64(rand.Intn(100))
		err := b.Publish(ctx, testTopic, broker.NewMessage(msg))
		assert.Nil(t, err)
		elapsedTime := time.Since(startTime) / time.Millisecond
		fmt.Printf("JS Publish %d, elapsed time: %dms, Humidity: %.2f Temperature: %.2f\n",
			i, elapsedTime, msg.Humidity, msg.Temperature)
	}

	fmt.Printf("total send %d messages\n", count)

	<-interrupt
}

func TestJetStream_Subscribe_WithJsonCodec(t *testing.T) {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	b := NewJetStreamBroker(
		broker.WithAddress(localBroker),
		broker.WithCodec("json"),
	)

	_ = b.Init()

	if err := b.Connect(); err != nil {
		t.Logf("cant connect to broker, skip: %v", err)
		t.Skip()
	}
	defer b.Disconnect()

	js := GetJetStreamContext(b)
	if js != nil {
		_, _ = js.AddStream(&natsGo.StreamConfig{
			Name:     "TEST",
			Subjects: []string{"test_topic"},
		})
	}

	_, err := b.Subscribe(testTopic,
		RegisterHygrothermographHandler(handleHygrothermograph),
		api.HygrothermographCreator,
		WithDeliverNew(),
		WithDurable("test-json-durable"),
	)
	assert.Nil(t, err)

	<-interrupt
}

///////////////////////////////////////////////////////////////////////////////
/// Advanced JetStream Features
///////////////////////////////////////////////////////////////////////////////

func TestJetStream_Publish_WithMsgId(t *testing.T) {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	ctx := context.Background()

	b := NewJetStreamBroker(
		broker.WithAddress(localBroker),
		broker.WithCodec("json"),
	)

	_ = b.Init()

	if err := b.Connect(); err != nil {
		t.Logf("cant connect to broker, skip: %v", err)
		t.Skip()
	}
	defer b.Disconnect()

	js := GetJetStreamContext(b)
	if js != nil {
		_, _ = js.AddStream(&natsGo.StreamConfig{
			Name:     "TEST",
			Subjects: []string{"test_topic"},
		})
	}

	var msg api.Hygrothermograph
	const count = 5
	for i := 0; i < count; i++ {
		msg.Humidity = float64(rand.Intn(100))
		msg.Temperature = float64(rand.Intn(100))

		// Publish with MsgId for deduplication
		err := b.Publish(ctx, testTopic, broker.NewMessage(msg),
			WithMsgId(fmt.Sprintf("msg-%d", i)),
		)
		assert.Nil(t, err)
		fmt.Printf("Published message %d with MsgId deduplication\n", i)
	}

	<-interrupt
}

func TestJetStream_Subscribe_WithManualAck(t *testing.T) {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	b := NewJetStreamBroker(
		broker.WithAddress(localBroker),
		broker.WithCodec("json"),
	)

	_ = b.Init()

	if err := b.Connect(); err != nil {
		t.Logf("cant connect to broker, skip: %v", err)
		t.Skip()
	}
	defer b.Disconnect()

	js := GetJetStreamContext(b)
	if js != nil {
		_, _ = js.AddStream(&natsGo.StreamConfig{
			Name:     "TEST",
			Subjects: []string{"test_topic"},
		})
	}

	// Subscribe with manual ack — handler must call event.Ack() itself
	_, err := b.Subscribe(testTopic,
		func(ctx context.Context, event broker.Event) error {
			switch t := event.Message().Body.(type) {
			case *api.Hygrothermograph:
				LogInfof("ManualAck received: Topic %s, Humidity: %.2f Temperature: %.2f",
					event.Topic(), t.Humidity, t.Temperature)
				// Manually acknowledge
				if msg, ok := JetStreamMsgFromEvent(event); ok {
					return msg.Ack()
				}
				return event.Ack()
			default:
				return fmt.Errorf("unsupported type: %T", t)
			}
		},
		api.HygrothermographCreator,
		WithDurable("manual-ack-durable"),
		WithDeliverNew(),
		WithManualAck(),
		WithSubscribeAckWait(30*time.Second),
		WithSubscribeMaxAckPending(100),
	)
	assert.Nil(t, err)

	<-interrupt
}

func TestJetStream_Subscribe_Pull(t *testing.T) {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	b := NewJetStreamBroker(
		broker.WithAddress(localBroker),
		broker.WithCodec("json"),
	)

	_ = b.Init()

	if err := b.Connect(); err != nil {
		t.Logf("cant connect to broker, skip: %v", err)
		t.Skip()
	}
	defer b.Disconnect()

	js := GetJetStreamContext(b)
	if js != nil {
		_, _ = js.AddStream(&natsGo.StreamConfig{
			Name:     "TEST",
			Subjects: []string{"test_topic"},
		})
	}

	// Pull-based subscription
	_, err := b.Subscribe(testTopic,
		RegisterHygrothermographHandler(handleHygrothermograph),
		api.HygrothermographCreator,
		WithPullSubscribe(),
		WithPullBatchSize(10),
		WithDurable("pull-durable"),
		WithDeliverAll(),
	)
	assert.Nil(t, err)

	<-interrupt
}

///////////////////////////////////////////////////////////////////////////////
/// Tracing
///////////////////////////////////////////////////////////////////////////////

func TestJetStream_Publish_WithTracer(t *testing.T) {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	ctx := context.Background()

	b := NewJetStreamBroker(
		broker.WithAddress(localBroker),
		broker.WithCodec("json"),
		createTracerProvider("js_publish_tracer_tester"),
	)

	_ = b.Init()

	if err := b.Connect(); err != nil {
		t.Logf("cant connect to broker, skip: %v", err)
		t.Skip()
	}
	defer b.Disconnect()

	js := GetJetStreamContext(b)
	if js != nil {
		_, _ = js.AddStream(&natsGo.StreamConfig{
			Name:     "TEST",
			Subjects: []string{"test_topic"},
		})
	}

	var msg api.Hygrothermograph
	const count = 10
	for i := 0; i < count; i++ {
		startTime := time.Now()
		msg.Humidity = float64(rand.Intn(100))
		msg.Temperature = float64(rand.Intn(100))
		err := b.Publish(ctx, testTopic, broker.NewMessage(msg))
		assert.Nil(t, err)
		elapsedTime := time.Since(startTime) / time.Millisecond
		fmt.Printf("JS Publish %d, elapsed time: %dms, Humidity: %.2f Temperature: %.2f\n",
			i, elapsedTime, msg.Humidity, msg.Temperature)
	}

	fmt.Printf("total send %d messages\n", count)

	<-interrupt
}

func TestJetStream_Subscribe_WithTracer(t *testing.T) {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	b := NewJetStreamBroker(
		broker.WithAddress(localBroker),
		broker.WithCodec("json"),
		createTracerProvider("js_subscribe_tracer_tester"),
	)

	_ = b.Init()

	if err := b.Connect(); err != nil {
		t.Logf("cant connect to broker, skip: %v", err)
		t.Skip()
	}
	defer b.Disconnect()

	js := GetJetStreamContext(b)
	if js != nil {
		_, _ = js.AddStream(&natsGo.StreamConfig{
			Name:     "TEST",
			Subjects: []string{"test_topic"},
		})
	}

	_, err := b.Subscribe(testTopic,
		RegisterHygrothermographHandler(handleHygrothermograph),
		api.HygrothermographCreator,
		WithDeliverNew(),
		WithDurable("tracer-durable"),
	)
	assert.Nil(t, err)

	<-interrupt
}
