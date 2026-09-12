package binding

import (
	"encoding/json"
	"errors"
	"net/http"
)

var contentType = "application/json"

func SetContentType(ct string) {
	if ct != "" {
		contentType = ct
	}
}

func WriteResponse(w http.ResponseWriter, r *http.Request, v interface{}) {
	if v == nil {
		w.WriteHeader(http.StatusOK)
		return
	}
	data, err := bodyCodec.Marshal(v)
	if err != nil {
		WriteError(w, err)
		return
	}
	w.Header().Set("Content-Type", contentType+"; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func WriteError(w http.ResponseWriter, err error) {
	code := MapError(err)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	body, _ := json.Marshal(map[string]string{"error": err.Error()})
	_, _ = w.Write(body)
}

// WriteStreamChunk 写出服务端流式响应（chunked transfer）的一块并立即
// flush 到客户端。contentType 非空且响应尚未设置 Content-Type 时先设置
// （HttpBody 首帧携带类型信息的约定）。仅用于 server-streaming 路由的
// 处理器；任何一帧写出后调用方应停止向响应写错误，只能中止流。
func WriteStreamChunk(w http.ResponseWriter, contentType string, data []byte) error {
	if contentType != "" && w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", contentType)
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		return errors.New("http: response writer is not flushable")
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}