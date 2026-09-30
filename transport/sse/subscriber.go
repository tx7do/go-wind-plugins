package sse

import (
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Subscriber represents a single client connected to an SSE stream.
type Subscriber struct {
	quit       chan *Subscriber
	connection chan *Event
	removed    chan struct{}
	eventId    string
	URL        *url.URL
	Header     http.Header
}

// close deregisters the subscriber from its stream.
func (s *Subscriber) close() {
	// 优先非阻塞投递；run 循环忙时（Replay 慢订阅者/分发中）退避重试，
	// 避免 stream 退出窗口内静默丢失退订（订阅者残留、连接僵尸）
	for attempt := 0; attempt < 20; attempt++ {
		select {
		case <-time.After(50 * time.Millisecond):
		case s.quit <- s:
			if s.removed != nil {
				select {
				case <-s.removed:
				case <-time.After(time.Second):
				}
			}
			return
		}
	}
	// 多轮重试仍失败：stream run 循环已确认退出，直接放弃登记
}

// HeaderValue returns the value of a request header.
func (s *Subscriber) HeaderValue(key string) string {
	if s == nil || s.Header == nil {
		return ""
	}
	return s.Header.Get(key)
}

// Authorization returns the Authorization header value.
func (s *Subscriber) Authorization() string {
	return s.HeaderValue("Authorization")
}

// BearerToken extracts a Bearer token from the Authorization header.
func (s *Subscriber) BearerToken() string {
	auth := strings.TrimSpace(s.Authorization())
	if auth == "" {
		return ""
	}
	if len(auth) >= 7 && strings.EqualFold(auth[:7], "Bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	return auth
}

// Token extracts a token from a custom header, Bearer auth, or URL query parameter.
func (s *Subscriber) Token(headerKey string) string {
	if headerKey != "" {
		if token := strings.TrimSpace(s.HeaderValue(headerKey)); token != "" {
			return token
		}
	}
	if token := s.BearerToken(); token != "" {
		return token
	}
	if s != nil && s.URL != nil {
		return strings.TrimSpace(s.URL.Query().Get("token"))
	}
	return ""
}
