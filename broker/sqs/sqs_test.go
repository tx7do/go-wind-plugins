package sqs

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

	"github.com/stretchr/testify/assert"

	"github.com/tx7do/go-wind/log"

	"github.com/tx7do/go-wind-plugins/broker"
	api "github.com/tx7do/go-wind-plugins/testing/api/manual"
)

const (
	localEndpoint = "http://127.0.0.1:9324"
	localRegion   = "elasticmq"
	testQueueName = "test-queue"
)

func handleHygrothermograph(_ context.Context, topic string, headers broker.Headers, msg *api.Hygrothermograph) error {
	LogInfof("Topic %s, Headers: %+v, Payload: %+v\n", topic, headers, msg)
	return nil
}

func Test_Publish_WithRawData(t *testing.T) {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	ctx := context.Background()

	b := NewBroker(
		broker.WithOptionContext(ctx),
		broker.WithAddress(localEndpoint),
		WithRegion(localRegion),
		WithEndpoint(localEndpoint),
		WithQueueUrl(localEndpoint+"/queue/"+testQueueName),
	)

	_ = b.Init()

	if err := b.Connect(); err != nil {
		t.Logf("cant connect to broker, skip: %v", err)
		t.Skip()
	}
	defer b.Disconnect()

	var msg api.Hygrothermograph
	const count = 10
	for i := 0; i < count; i++ {
		startTime := time.Now()
		msg.Humidity = float64(rand.Intn(100))
		msg.Temperature = float64(rand.Intn(100))
		buf, _ := json.Marshal(&msg)
		err := b.Publish(ctx, testQueueName, broker.NewMessage(buf))
		assert.Nil(t, err)
		elapsedTime := time.Since(startTime) / time.Millisecond
		fmt.Printf("Publish %d, elapsed time: %dms, Humidity: %.2f Temperature: %.2f\n",
			i, elapsedTime, msg.Humidity, msg.Temperature)
	}

	fmt.Printf("total send %d messages\n", count)

	<-interrupt
}

func Test_Subscribe_WithRawData(t *testing.T) {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	b := NewBroker(
		broker.WithAddress(localEndpoint),
		WithRegion(localRegion),
		WithEndpoint(localEndpoint),
		WithQueueUrl(localEndpoint+"/queue/"+testQueueName),
	)
	defer b.Disconnect()

	_ = b.Connect()

	_, err := b.Subscribe(testQueueName,
		RegisterHygrothermographRawHandler(handleHygrothermograph),
		nil,
	)
	assert.Nil(t, err)

	<-interrupt
}

func Test_Publish_WithJsonCodec(t *testing.T) {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	ctx := context.Background()

	b := NewBroker(
		broker.WithOptionContext(ctx),
		broker.WithAddress(localEndpoint),
		broker.WithCodec("json"),
		WithRegion(localRegion),
		WithEndpoint(localEndpoint),
		WithQueueUrl(localEndpoint+"/queue/"+testQueueName),
	)

	_ = b.Init()

	if err := b.Connect(); err != nil {
		t.Logf("cant connect to broker, skip: %v", err)
		t.Skip()
	}
	defer b.Disconnect()

	var msg api.Hygrothermograph
	const count = 10
	for i := 0; i < count; i++ {
		startTime := time.Now()
		msg.Humidity = float64(rand.Intn(100))
		msg.Temperature = float64(rand.Intn(100))
		err := b.Publish(ctx, testQueueName, broker.NewMessage(msg))
		assert.Nil(t, err)
		elapsedTime := time.Since(startTime) / time.Millisecond
		fmt.Printf("Publish %d, elapsed time: %dms, Humidity: %.2f Temperature: %.2f\n",
			i, elapsedTime, msg.Humidity, msg.Temperature)
	}

	fmt.Printf("total send %d messages\n", count)

	<-interrupt
}

func Test_Subscribe_WithJsonCodec(t *testing.T) {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	b := NewBroker(
		broker.WithAddress(localEndpoint),
		broker.WithCodec("json"),
		WithRegion(localRegion),
		WithEndpoint(localEndpoint),
		WithQueueUrl(localEndpoint+"/queue/"+testQueueName),
	)
	defer b.Disconnect()

	_ = b.Connect()

	_, err := b.Subscribe(testQueueName,
		RegisterHygrothermographRawHandler(handleHygrothermograph),
		api.HygrothermographCreator,
	)
	assert.Nil(t, err)

	<-interrupt
}

type HygrothermographHandler func(_ context.Context, topic string, headers broker.Headers, msg *api.Hygrothermograph) error

func RegisterHygrothermographRawHandler(fnc HygrothermographHandler) broker.Handler {
	return func(ctx context.Context, event broker.Event) error {
		var msg api.Hygrothermograph

		switch t := event.Message().Body.(type) {
		case []byte:
			if err := json.Unmarshal(t, &msg); err != nil {
				log.GetLogger().Error(context.Background(), "json Unmarshal failed: ", "error", err.Error())
				return err
			}
		case string:
			if err := json.Unmarshal([]byte(t), &msg); err != nil {
				log.GetLogger().Error(context.Background(), "json Unmarshal failed: ", "error", err.Error())
				return err
			}
		default:
			log.GetLogger().Error(context.Background(), "unknown type Unmarshal failed: ", "type", fmt.Sprintf("%T", t))
			return fmt.Errorf("unsupported type: %T", t)
		}

		if err := fnc(ctx, event.Topic(), event.Message().Headers, &msg); err != nil {
			return err
		}

		return nil
	}
}
