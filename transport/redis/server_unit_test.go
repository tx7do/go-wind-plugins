package redis

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKind(t *testing.T) {
	assert.Equal(t, "redis", KindRedis)
}

func TestNewServer(t *testing.T) {
	srv := NewServer(
		WithAddress("redis://127.0.0.1:6379"), WithCodec("json"),
	)
	assert.NotNil(t, srv)
	assert.Equal(t, "redis", srv.Name())
	assert.False(t, srv.started.Load())
}

func TestEndpoint(t *testing.T) {
	srv := NewServer(
		WithAddress("redis://127.0.0.1:6379"), WithCodec("json"),
	)
	assert.Equal(t, "", srv.Endpoint())
}

func TestStopBeforeStart(t *testing.T) {
	srv := NewServer(
		WithAddress("redis://127.0.0.1:6379"), WithCodec("json"),
	)
	err := srv.Stop(context.Background())
	assert.Nil(t, err)
}

// TestServerConnectBeforeStartOnClosedPort 回归 gated TestServer 的 panic：
// 旧实现中先 srv.Connect() 再 srv.Start() 时，Start 内部的 Init 会因连接池
// 已存在而报 “redis: cannot init while connected”。修复后该顺序合法——对
// 必然拒连的关闭端口，Start 返回普通拨号错误（可重试、不 panic）。
func TestServerConnectBeforeStartOnClosedPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	closedAddr := ln.Addr().String()
	_ = ln.Close() // 只借一个必然拒连的端口

	srv := NewServer(
		WithAddress(closedAddr),
		WithReadTimeout(2*time.Second),
		WithWriteTimeout(2*time.Second),
	)
	require.NoError(t, srv.RegisterSubscriber("topic.closed", fakeNopHandler, nil))

	// 模拟 gated TestServer：先手动 Connect（创建连接池，不拨号），再 Start。
	require.NoError(t, srv.Connect())

	err = srv.Start(context.Background())
	require.Error(t, err, "Start against a closed port must fail at subscriber registration")
	assert.NotContains(t, err.Error(), "cannot init while connected",
		"the init guard must not trip when Connect was called before Start")

	// 失败的 Start 已回滚（doRegisterSubscriberMap 消费了延迟注册表，
	// 与其余 transport 驱动一致的既有语义，故重试前重新注册）。
	// 重试仍是普通拨号错误而非 init 冲突或 panic。
	require.NoError(t, srv.RegisterSubscriber("topic.closed", fakeNopHandler, nil))
	err = srv.Start(context.Background())
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "cannot init while connected")

	assert.False(t, srv.started.Load())
	require.NoError(t, srv.Stop(context.Background()))
}

// TestServerStartFailureStopStartCycle 回归：Start 失败 → Stop → Start 循环
// 必须每次返回错误且状态干净（不 panic、不把错误粘死）。
// 注：doRegisterSubscriberMap 在启动时即消费延迟注册表（五个 transport 驱动
// 共有的既有语义），因此每轮循环重新注册订阅。
func TestServerStartFailureStopStartCycle(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	closedAddr := ln.Addr().String()
	_ = ln.Close()

	srv := NewServer(
		WithAddress(closedAddr),
		WithReadTimeout(2*time.Second),
		WithWriteTimeout(2*time.Second),
	)

	for i := 0; i < 3; i++ {
		require.NoError(t, srv.RegisterSubscriber("topic.closed", fakeNopHandler, nil))

		err := srv.Start(context.Background())
		require.Error(t, err, "cycle %d: Start against a closed port must fail", i)
		assert.NotContains(t, err.Error(), "cannot init while connected", "cycle %d", i)
		assert.False(t, srv.started.Load(), "cycle %d: failed Start must not leave the server started", i)
		assert.NoError(t, srv.err, "cycle %d: failed Start must not leave a sticky error", i)
	}

	require.NoError(t, srv.Stop(context.Background()))
}
