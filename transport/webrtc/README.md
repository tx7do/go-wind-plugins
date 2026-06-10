# WebRTC

WebRTC（Web Real‑Time Communication）是 Google 开源的实时通信技术栈，支持浏览器与设备之间无需插件即可进行低延迟音视频通话与数据传输。

本模块提供了一个基于 WebRTC 的 SFU（Selective Forwarding Unit）服务器，实现了 `go-wind/transport.Server` 接口，支持音视频流转发、P2P 数据通道、会话管理，可以与 `go-wind` 应用框架无缝集成。底层使用 [Pion WebRTC](https://pion.ly/) v4。

## 核心特性

- **SFU 架构**：选择性转发单元，高效媒体流路由
- **音视频通话**：支持 H.264/VP8 视频和 Opus 音频编解码
- **数据通道**：低延迟二进制/文本数据传输，消息类型路由
- **会话管理**：每个客户端对应一个 Session，支持广播/定向发送
- **自定义编解码**：基于 `encoding.Codec` 支持 JSON / Proto / MsgPack 等格式
- **CORS 支持**：可配置跨域策略
- **鉴权支持**：内置 Token 提取
- **阻塞式生命周期**：`Start` 阻塞直到 context 取消，兼容 `go-wind` App

## 安装

```bash
go get github.com/tx7do/go-wind-plugins/transport/webrtc
```

## 快速开始

### 服务端

```go
package main

import (
    "context"
    "log"

    webrtc "github.com/tx7do/go-wind-plugins/transport/webrtc"
)

type ChatMessage struct {
    Sender  string `json:"sender"`
    Message string `json:"message"`
}

func main() {
    srv := webrtc.NewServer(
        webrtc.WithAddress(":9999"),
        webrtc.WithPath("/signal"),
        webrtc.WithCodec("json"),
    )

    // 注册消息处理器
    webrtc.RegisterServerMessageHandler(srv, 1, func(sid webrtc.SessionID, msg *ChatMessage) error {
        log.Printf("[%s] %s: %s", sid, msg.Sender, msg.Message)
        srv.Broadcast(1, *msg)
        return nil
    })

    // 启动服务器（阻塞）
    ctx := context.Background()
    if err := srv.Start(ctx); err != nil {
        panic(err)
    }
}
```

### 客户端

```go
cli := webrtc.NewClient(
    webrtc.WithSignalURL("http://127.0.0.1:9999/signal"),
    webrtc.WithClientCodec("json"),
)
defer cli.Disconnect()

webrtc.RegisterClientMessageHandler(cli, 1, func(msg *ChatMessage) error {
    log.Printf("received: %+v", msg)
    return nil
})

if err := cli.Connect(); err != nil {
    panic(err)
}

_ = cli.SendMessage(1, &ChatMessage{Sender: "alice", Message: "hello"})
```

## 配置选项

### 服务端选项

| 选项                                   | 说明                       | 默认值                 |
|--------------------------------------|--------------------------|---------------------|
| `WithAddress(addr)`                  | 监听地址                     | `:0`                |
| `WithPath(path)`                     | 信令端点路径                   | `/signal`           |
| `WithNetwork(network)`               | 网络类型                     | `tcp`               |
| `WithCodec(name)`                    | 编解码器名称                   | `json`              |
| `WithTLSConfig(cfg)`                 | TLS 配置                   | -                   |
| `WithListener(lis)`                  | 自定义 Listener             | -                   |
| `WithPayloadType(t)`                 | 数据包格式（Binary/Text）       | `PayloadTypeBinary` |
| `WithWebRTCConfiguration(cfg)`       | WebRTC ICE 配置            | -                   |
| `WithWebRTCAPI(api)`                 | 自定义 WebRTC API           | -                   |
| `WithMediaEnabled(true)`             | 启用媒体编解码器（H.264/VP8/Opus） | `false`             |
| `WithDataChannelLabel(label)`        | DataChannel 标签           | `wind`              |
| `WithAllowAnyDataChannelLabel(bool)` | 允许任意 DC 标签               | `true`              |
| `WithEnableCORS(bool)`               | 启用 CORS                  | `true`              |
| `WithCORS(origin, methods, headers)` | CORS 策略                  | `*`                 |
| `WithCheckOrigin(domain)`            | Origin 检查                | 允许所有                |
| `WithInjectTokenToQuery(bool, key)`  | 注入 Token 到查询参数           | `true, "token"`     |
| `WithSocketConnectHandler(fn)`       | 连接/断开回调                  | -                   |
| `WithSocketRawDataHandler(fn)`       | 原始数据处理回调                 | -                   |
| `WithMessageMarshaler(fn)`           | 自定义封包函数                  | -                   |
| `WithMessageUnmarshaler(fn)`         | 自定义拆包函数                  | -                   |

### 客户端选项

| 选项                                   | 说明                | 默认值                 |
|--------------------------------------|-------------------|---------------------|
| `WithSignalURL(uri)`                 | 信令服务器地址           | -                   |
| `WithAuthorization(token)`           | Authorization 请求头 | -                   |
| `WithClientCodec(name)`              | 编解码器名称            | `json`              |
| `WithClientPayloadType(t)`           | 数据包格式             | `PayloadTypeBinary` |
| `WithClientDataChannelLabel(label)`  | DataChannel 标签    | `wind`              |
| `WithClientWebRTCConfiguration(cfg)` | WebRTC ICE 配置     | -                   |
| `WithClientConnectTimeout(d)`        | 连接超时              | `10s`               |
| `WithClientSignalTimeout(d)`         | 信令请求超时            | `10s`               |

## 媒体流支持

### 启用媒体

```go
srv := webrtc.NewServer(
    webrtc.WithAddress(":9999"),
    webrtc.WithMediaEnabled(true), // 自动注册 H.264/VP8/Opus
)
```

或自定义 WebRTC API：

```go
import "github.com/pion/webrtc/v4"

settingEngine := webrtc.SettingEngine{}
mediaEngine := &webrtc.MediaEngine{}

mediaEngine.RegisterCodec(webrtc.RTPCodecParameters{
    RTPCodecCapability: webrtc.RTPCodecCapability{
        MimeType:  webrtc.MimeTypeH264,
        ClockRate: 90000,
    },
    PayloadType: 96,
}, webrtc.RTPCodecTypeVideo)

mediaEngine.RegisterCodec(webrtc.RTPCodecParameters{
    RTPCodecCapability: webrtc.RTPCodecCapability{
        MimeType:  webrtc.MimeTypeOpus,
        ClockRate: 48000,
        Channels:  2,
    },
    PayloadType: 111,
}, webrtc.RTPCodecTypeAudio)

api := webrtc.NewAPI(
    webrtc.WithMediaEngine(mediaEngine),
    webrtc.WithSettingEngine(settingEngine),
)

srv := webrtc.NewServer(
    webrtc.WithWebRTCAPI(api),
)
```

### 订阅媒体流

```go
// 服务端：订阅发布者的媒体流
err := srv.SubscribeToPublisher(subscriberID, publisherID)

// 服务端：取消订阅
srv.UnsubscribeFromPublisher(subscriberID, publisherID)

// 客户端：添加本地媒体轨道
localTrack, _ := webrtc.NewTrackLocalStaticRTP(
    webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2},
    "audio-track", "client-id",
)
cli.AddLocalTrack(localTrack)

// 客户端：获取远程轨道
tracks := cli.GetRemoteTracks()
```

## 协议格式

数据通道支持两种应用层包格式：

### Binary 模式（默认）

```
+-------------------+-------------------+
| Type (uint32, 4B) | Payload (变长)     |
+-------------------+-------------------+
```

- 小端序
- `Type`：消息类型标识，用于路由到对应的 handler
- `Payload`：由 `encoding.Codec` 序列化

### Text 模式

```json
{"type": 1, "payload": "{\"sender\":\"alice\"}"}
```

## API 参考

### Server 方法

| 方法                                              | 说明        |
|-------------------------------------------------|-----------|
| `Start(ctx)`                                    | 启动服务器（阻塞） |
| `Stop(ctx)`                                     | 停止服务器     |
| `Endpoint()`                                    | 获取服务端点地址  |
| `RegisterMessageHandler(type, handler, binder)` | 注册消息处理器   |
| `DeregisterMessageHandler(type)`                | 注销消息处理器   |
| `SendMessage(sessionID, type, msg)`             | 定向发送消息    |
| `Broadcast(type, msg)`                          | 广播消息      |
| `SubscribeToPublisher(sub, pub)`                | 订阅发布者媒体流  |
| `UnsubscribeFromPublisher(sub, pub)`            | 取消订阅      |

### Client 方法

| 方法                                              | 说明         |
|-------------------------------------------------|------------|
| `Connect()`                                     | 连接到信令服务器   |
| `Disconnect()`                                  | 断开连接       |
| `SendMessage(type, msg)`                        | 发送消息       |
| `RegisterMessageHandler(type, handler, binder)` | 注册消息处理器    |
| `AddLocalTrack(track)`                          | 添加本地媒体轨道   |
| `RemoveLocalTrack(trackID)`                     | 移除本地媒体轨道   |
| `GetRemoteTracks()`                             | 获取所有远程媒体轨道 |

## 测试客户端

### Go 客户端

```powershell
Push-Location "go-wind-plugins\transport\webrtc"
go run ./cmd/test_client -signal "http://127.0.0.1:9999/signal" -type 1 -mode binary -payload '{"type":1,"sender":"go","message":"hello"}'
Pop-Location
```

常用参数：

- `-signal`：信令地址（默认 `http://127.0.0.1:9999/signal`）
- `-auth`：`Authorization` 请求头
- `-label`：DataChannel 标签（默认 `wind`）
- `-mode`：`binary` 或 `text`
- `-type`：消息类型（`NetMessageType`）
- `-payload`：消息体（JSON 或普通字符串）

### HTML 客户端

浏览器打开 `test_client.html` 即可进行音视频和数据通道联调，详见 [HTML_CLIENT_GUIDE.md](./HTML_CLIENT_GUIDE.md)。

## 架构说明

```
                ┌──────────────┐
                │   SFU Server │
                │  (Wind)      │
                └──────┬───────┘
                       │
          ┌────────────┼────────────┐
          │            │            │
     ┌────▼────┐  ┌───▼────┐  ┌───▼────┐
     │Client A │  │Client B│  │Client C│
     └─────────┘  └────────┘  └────────┘
          │            │            │
          └────────────┴────────────┘
               媒体流通过 SFU 转发
```

**工作流程**：
1. Client 发布音视频流 → SFU Server
2. SFU Server 接收并转发 → 其他 Client
3. 数据通道用于信令交换（订阅通知、聊天等）

## 性能指标

- **延迟**: < 100ms（局域网），< 300ms（广域网）
- **并发**: 支持 100+ 客户端（取决于服务器配置）
- **带宽**: 每个视频流约 1-5 Mbps（取决于分辨率）
- **CPU**: SFU 转发开销较低，主要消耗在编解码

## 参考资料

- [WebRTC API - MDN](https://developer.mozilla.org/en-US/docs/Web/API/WebRTC_API)
- [Pion WebRTC](https://pion.ly/)
- [WebRTC 架构详解](https://webrtc.org/getting-started/server-side)
