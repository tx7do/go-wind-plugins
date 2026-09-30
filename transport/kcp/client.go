package kcp

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/tx7do/go-wind-plugins/encoding"
	"github.com/tx7do/go-wind-plugins/metrics"

	"go.opentelemetry.io/otel/trace"

	"github.com/xtaci/kcp-go/v5"
)

type ClientMessageHandler func(NetMessagePayload) error

type ClientRawMessageHandler func([]byte) error

type ClientHandlerData struct {
	Handler ClientMessageHandler
	Creator Creator
}
type ClientMessageHandlerMap map[NetMessageType]ClientHandlerData

type Client struct {
	connMu    sync.RWMutex
	writeMu   sync.Mutex
	handlerMu sync.RWMutex
	conn      *kcp.UDPSession

	url      string
	endpoint *url.URL

	codec             encoding.Codec
	messageHandlers   ClientMessageHandlerMap
	rawMessageHandler ClientRawMessageHandler

	tracer trace.Tracer
	m      metrics.Metrics

	timeout time.Duration

	blockCryptPassword, blockCryptSalt string
	dataShards, parityShards           int
}

func NewClient(opts ...ClientOption) *Client {
	cli := &Client{
		url:             "",
		timeout:         1 * time.Second,
		codec:           encoding.GetCodec("json"),
		messageHandlers: make(ClientMessageHandlerMap),
		dataShards:      10,
		parityShards:    3,
	}

	cli.init(opts...)

	return cli
}

func (c *Client) init(opts ...ClientOption) {
	for _, o := range opts {
		o(c)
	}

	addr := c.url

	// 仅在缺失时补 udp:// 前缀（原先两分支相同，已带前缀时会拼出 udp://udp://…）
	if !strings.HasPrefix(addr, "udp://") {
		addr = "udp://" + addr
	}

	c.endpoint, _ = url.Parse(addr)
}

func (c *Client) Connect() error {
	if c.endpoint == nil {
		return errors.New("endpoint is nil")
	}

	LogInfof("connecting to %s", c.endpoint.String())

	block := NewBlockCryptFromPassword(c.blockCryptPassword, c.blockCryptSalt)

	// kcp-go 不接受 scheme 前缀；用户传 udp://host:port 时剥离后再拨
	dialAddr := strings.TrimPrefix(c.url, "udp://")
	conn, err := kcp.DialWithOptions(dialAddr, block, c.dataShards, c.parityShards)
	if err != nil {
		LogErrorf("cant connect to server: %s", err)
		return err
	}

	c.connMu.Lock()
	c.conn = conn
	c.connMu.Unlock()

	go c.run()

	return nil
}

func (c *Client) Disconnect() {
	c.connMu.Lock()
	conn := c.conn
	c.conn = nil
	c.connMu.Unlock()

	if conn != nil {
		if err := conn.Close(); err != nil {
			LogErrorf("disconnect error: %s", err)
		}
	}
}

func (c *Client) RegisterMessageHandler(messageType NetMessageType, handler ClientMessageHandler, binder Creator) {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()

	if _, ok := c.messageHandlers[messageType]; ok {
		return
	}

	c.messageHandlers[messageType] = ClientHandlerData{handler, binder}
}

func RegisterClientMessageHandler[T any](cli *Client, messageType NetMessageType, handler func(*T) error) {
	cli.RegisterMessageHandler(messageType,
		func(payload NetMessagePayload) error {
			switch t := payload.(type) {
			case *T:
				return handler(t)
			default:
				LogError("invalid payload struct type:", t)
				return errors.New("invalid payload struct type")
			}
		},
		func() any {
			var t T
			return &t
		},
	)
}

func (c *Client) DeregisterMessageHandler(messageType NetMessageType) {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()

	delete(c.messageHandlers, messageType)
}

func (c *Client) SendRawData(message []byte) error {
	c.connMu.RLock()
	conn := c.conn
	c.connMu.RUnlock()

	if conn == nil {
		return errors.New("client is not connected")
	}

	startTime := time.Now()
	labels := map[string]string{
		"rpc.system": "kcp",
	}

	if c.m != nil {
		c.m.Counter(context.Background(), "kcp.client.messages.sent", 1, labels)
	}

	// 写锁串行化：并发发送时单帧 Write 交错会破坏帧流
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	// 写入带长度前缀的帧，与服务端分包逻辑对应
	if err := WriteFrame(conn, message); err != nil {
		if c.m != nil {
			c.m.Counter(context.Background(), "kcp.client.messages.errors", 1, map[string]string{
				"rpc.system": "kcp",
				"error":      "true",
			})
		}
		return err
	}

	if c.m != nil {
		c.m.Histogram(context.Background(), "kcp.client.message.send_duration", time.Since(startTime).Seconds(), labels)
	}

	return nil
}

func (c *Client) SendMessage(messageType int, message any) error {
	var msg NetPacket
	msg.Type = NetMessageType(messageType)
	var err error
	msg.Payload, err = c.codec.Marshal(message)
	if err != nil {
		return err
	}

	var buff []byte
	if buff, err = msg.Marshal(); err != nil {
		return err
	}

	return c.SendRawData(buff)
}

func (c *Client) run() {
	// 只关闭本次连接：旧读循环收敛时不应误杀用户新建的连接
	c.connMu.RLock()
	conn := c.conn
	c.connMu.RUnlock()
	defer func() {
		c.connMu.Lock()
		if c.conn == conn {
			c.conn = nil
		}
		c.connMu.Unlock()
		if conn != nil {
			if err := conn.Close(); err != nil {
				LogErrorf("disconnect error: %s", err)
			}
		}
	}()

	for {
		if conn == nil {
			return
		}

		// 按帧读取，与服务端分包逻辑对应
		frame, err := ReadFrame(conn)
		if err != nil {
			LogErrorf("read message error: %v", err)
			return
		}

		if c.rawMessageHandler != nil {
			if err := c.rawMessageHandler(frame); err != nil {
				LogErrorf("raw data handler exception: %s", err)
				continue
			}
			continue
		}

		if err = c.messageHandler(frame); err != nil {
			LogErrorf("process message error: %v", err)
		}

		if c.m != nil {
			c.m.Counter(context.Background(), "kcp.client.messages.received", 1, map[string]string{
				"rpc.system": "kcp",
			})
		}
	}
}

func (c *Client) messageHandler(buf []byte) error {
	var msg NetPacket
	if err := msg.Unmarshal(buf); err != nil {
		LogErrorf("decode message exception: %s", err)
		return err
	}

	handlerData, ok := c.messageHandlers[msg.Type]
	if !ok {
		LogError("message type not found:", msg.Type)
		return errors.New("message handler not found")
	}

	var payload NetMessagePayload

	if handlerData.Creator != nil {
		payload = handlerData.Creator()

		if err := c.codec.Unmarshal(msg.Payload, payload); err != nil {
			LogErrorf("unmarshal message exception: %s", err)
			return err
		}
	} else {
		payload = msg.Payload
	}

	if err := handlerData.Handler(payload); err != nil {
		LogErrorf("message handler exception: %s", err)
		return err
	}

	return nil
}
