package sse

import (
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
)

// StreamID uniquely identifies an SSE stream.
type StreamID string

// SubscriberFunction is invoked when a subscriber is added to or removed from a stream.
type SubscriberFunction func(streamID StreamID, sub *Subscriber)

// Stream holds subscribers and dispatches events for a single stream ID.
type Stream struct {
	id StreamID

	event    chan *Event
	quit     chan struct{}
	quitOnce sync.Once
	eventLog EventLog

	autoReplay bool
	autoStream bool

	register   chan *Subscriber
	deregister chan *Subscriber

	subscribers     []*Subscriber
	subscriberCount int32

	onSubscribe   SubscriberFunction
	onUnsubscribe SubscriberFunction
}

// newStream creates a stream instance with subscriber callbacks and buffering settings.
func newStream(id StreamID, buffSize int, replay, autoStream bool, onSubscribe, onUnsubscribe SubscriberFunction) *Stream {
	return &Stream{
		id:            id,
		autoStream:    autoStream,
		autoReplay:    replay,
		subscribers:   make([]*Subscriber, 0),
		register:      make(chan *Subscriber),
		deregister:    make(chan *Subscriber),
		event:         make(chan *Event, buffSize),
		quit:          make(chan struct{}),
		eventLog:      make(EventLog, 0),
		onSubscribe:   onSubscribe,
		onUnsubscribe: onUnsubscribe,
	}
}

// StreamID returns the stream identifier.
func (s *Stream) StreamID() StreamID {
	return s.id
}

// run starts the stream event loop.
func (s *Stream) run() {
	go func(stream *Stream) {
		for {
			select {
			case subscriber := <-stream.register:
				stream.subscribers = append(stream.subscribers, subscriber)
				if stream.autoReplay {
					stream.eventLog.Replay(subscriber)
				}

			case subscriber := <-stream.deregister:
				i := stream.getSubIndex(subscriber)
				if i != -1 {
					stream.removeSubscriber(i)
				}

				if stream.onUnsubscribe != nil {
					go stream.onUnsubscribe(stream.id, subscriber)
				}

			case event := <-stream.event:
				if stream.autoReplay {
					// EventLog.Add 会改写事件的 ID 与时间戳；直接把派发事件
					// 交给它会把时间戳刷新为当前时间，导致 ServeHTTP 的
					// eventTTL 检查永远看不到过期事件。日志用副本打戳，
					// 派发事件保持发布时的时间戳，仅同步日志分配的 ID
					// （供客户端携带 Last-Event-ID 重连续传）。
					journal := *event
					stream.eventLog.Add(&journal)
					event.ID = journal.ID
				}
				for i := range stream.subscribers {
					stream.subscribers[i].connection <- event
				}

			case <-stream.quit:
				stream.removeAllSubscribers()
				return
			}
		}
	}(s)
}

// close stops the stream event loop once.
func (s *Stream) close() {
	s.quitOnce.Do(func() {
		close(s.quit)
	})
}

func (s *Stream) getSubIndex(sub *Subscriber) int {
	for i := range s.subscribers {
		if s.subscribers[i] == sub {
			return i
		}
	}
	return -1
}

func (s *Stream) addSubscriber(eventId string, req *http.Request) *Subscriber {
	atomic.AddInt32(&s.subscriberCount, 1)

	var requestURL *url.URL
	var header http.Header
	if req != nil {
		if req.URL != nil {
			urlCopy := *req.URL
			requestURL = &urlCopy
		}
		header = req.Header.Clone()
	}

	sub := &Subscriber{
		eventId:    eventId,
		quit:       s.deregister,
		connection: make(chan *Event, 64),
		URL:        requestURL,
		Header:     header,
	}

	if s.autoStream {
		sub.removed = make(chan struct{}, 1)
	}

	s.register <- sub

	if s.onSubscribe != nil {
		go s.onSubscribe(s.id, sub)
	}

	return sub
}

func (s *Stream) removeSubscriber(i int) {
	atomic.AddInt32(&s.subscriberCount, -1)
	close(s.subscribers[i].connection)
	if s.subscribers[i].removed != nil {
		s.subscribers[i].removed <- struct{}{}
		close(s.subscribers[i].removed)
	}
	s.subscribers = append(s.subscribers[:i], s.subscribers[i+1:]...)
}

func (s *Stream) removeAllSubscribers() {
	for i := 0; i < len(s.subscribers); i++ {
		sub := s.subscribers[i]
		close(sub.connection)
		if sub.removed != nil {
			sub.removed <- struct{}{}
			close(sub.removed)
		}
		// 流级关闭（StreamManager.Remove/RemoveWithID/Clean）同样要触发
		// onUnsubscribe：与显式退订路径互斥（订阅者只会被其中一处移除），
		// 每个订阅者恰好触发一次。
		if s.onUnsubscribe != nil {
			go s.onUnsubscribe(s.id, sub)
		}
	}
	atomic.StoreInt32(&s.subscriberCount, 0)
	s.subscribers = s.subscribers[:0]
}

func (s *Stream) getSubscriberCount() int {
	return int(atomic.LoadInt32(&s.subscriberCount))
}
