package stream

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tx7do/go-wind-plugins/broker"
	redisOption "github.com/tx7do/go-wind-plugins/broker/redis/option"
)

// newSelfSignedCert 生成覆盖 127.0.0.1/localhost 的自签服务器证书（仅测试用）。
func newSelfSignedCert(t *testing.T) (certPEM, keyPEM []byte) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)

	tmpl := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.IPv6loopback},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	require.NoError(t, err)

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return certPEM, keyPEM
}

// startTLSServer 启动进程内 TLS 监听，并在独立 goroutine 中读取首个命令后
// 回复 reply。返回监听地址、服务端证书（供客户端构建 RootCAs）与收到的命令 channel。
func startTLSServer(t *testing.T, reply string) (addr string, leaf *x509.Certificate, received <-chan string) {
	t.Helper()

	certPEM, keyPEM := newSelfSignedCert(t)
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)

	block, _ := pem.Decode(certPEM)
	require.NotNil(t, block)
	leaf, err = x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	ch := make(chan string, 1)
	go func() {
		conn, aerr := ln.Accept()
		if aerr != nil {
			return
		}
		defer conn.Close()

		// 读取客户端命令明文（TLS 握手完成后才可见）。
		buf := make([]byte, 512)
		var sb strings.Builder
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		for {
			n, rerr := conn.Read(buf)
			sb.Write(buf[:n])
			if rerr != nil || sb.Len() > 0 {
				break
			}
		}
		ch <- sb.String()

		_, _ = conn.Write([]byte(reply))
	}()

	return ln.Addr().String(), leaf, ch
}

// TestConnectWithTLSConfigDialsSecurely 断言 WithTLSConfig 后连接池确实以 TLS 拨号：
// 对端是自签证书的进程内 TLS 监听，明文拨号无法通过握手。
func TestConnectWithTLSConfigDialsSecurely(t *testing.T) {
	addr, leaf, received := startTLSServer(t, "+OK\r\n")

	rootPool := x509.NewCertPool()
	rootPool.AddCert(leaf)

	b := NewBroker(
		broker.WithAddress(addr),
		redisOption.WithReadTimeout(3*time.Second),
		redisOption.WithWriteTimeout(3*time.Second),
		broker.WithTLSConfig(&tls.Config{RootCAs: rootPool}),
	)
	require.NoError(t, b.Init())
	require.NoError(t, b.Connect())

	require.NoError(t, b.Publish(context.Background(), "topic.a", broker.NewMessage("hello")))

	select {
	case raw := <-received:
		assert.Contains(t, raw, "XADD", "the pooled connection must deliver XADD over TLS")
		assert.Contains(t, raw, "topic.a")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the XADD command over TLS")
	}

	require.NoError(t, b.Disconnect())
}

// TestSecureFlagWithoutTLSConfigStillDialsTLS 断言仅开启 Secure（无 TLSConfig）时
// 也走 TLS：对自签服务端会因证书校验失败报错，而不是以明文成功。
func TestSecureFlagWithoutTLSConfigStillDialsTLS(t *testing.T) {
	addr, _, _ := startTLSServer(t, "+OK\r\n")

	b := NewBroker(
		broker.WithAddress(addr),
		redisOption.WithReadTimeout(3*time.Second),
		redisOption.WithWriteTimeout(3*time.Second),
		broker.WithEnableSecure(true),
	)
	require.NoError(t, b.Init())
	require.NoError(t, b.Connect())

	err := b.Publish(context.Background(), "topic.a", broker.NewMessage("hello"))
	require.Error(t, err, "plaintext against a TLS endpoint must fail")
	assert.Contains(t, err.Error(), "certificate", "failure must come from TLS verification, not plaintext dialing")

	require.NoError(t, b.Disconnect())
}

// TestInitWhileConnectedIdempotentWithoutOptions 回归：先 Connect 再 Init 是公开 API
// 的合法顺序（如先 srv.Connect() 再 srv.Start()）。旧实现对已连接状态一律报
// "redis-stream: cannot init while connected"，会让 transport 的 Start 直接失败。
func TestInitWhileConnectedIdempotentWithoutOptions(t *testing.T) {
	b := NewBroker(broker.WithAddress("127.0.0.1:6379")).(*streamBroker)

	require.NoError(t, b.Init())
	require.NoError(t, b.Connect())
	require.NotNil(t, b.pool)

	// 已连接时空参 Init 幂等成功。
	require.NoError(t, b.Init())

	// 已连接时携带新选项的 Init 仍然拒绝，防止活跃连接池感知不到的配置漂移。
	err := b.Init(broker.WithAddress("127.0.0.1:9999"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot init while connected")

	// 断开后恢复正常。
	require.NoError(t, b.Disconnect())
	assert.NoError(t, b.Init())
}

// TestEnableTLSScheme 断言 scheme 改写规则。
func TestEnableTLSScheme(t *testing.T) {
	assert.Equal(t, "rediss://127.0.0.1:6379", enableTLSScheme("redis://127.0.0.1:6379"))
	assert.Equal(t, "rediss://127.0.0.1:6379", enableTLSScheme("rediss://127.0.0.1:6379"))
}
