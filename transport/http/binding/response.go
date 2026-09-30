package binding

import (
	"encoding/json"
	"errors"
	"net/http"

	windErrors "github.com/tx7do/go-wind/errors"
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

// WriteError 将错误写入 HTTP 响应。
//
// 结构化错误（[windErrors.WindError]，含被 %w 包装进错误链的情况）按框架
// 错误模型写出：HTTP 状态码取自 [windErrors.CodeToHTTP] 对错误 Code 的
// 权威映射（gRPC code → HTTP 状态，与 Envoy / Google API transcoding 同表），
// 响应体为错误自身的 JSON 形状（reason / details 字段，字段标注见
// go-wind/errors）。消费端按 reason 驱动本地化文案、按状态码驱动路由分支，
// 二者均依赖本函数的映射。
//
// 非结构化错误不在上述模型内：命中哨兵表（[ErrBadRequest] 等）按表映射状态，
// 其余统一回退 500；响应体为通用错误文本形式（与既有行为一致，未做脱敏）。
func WriteError(w http.ResponseWriter, err error) {
	if wErr, ok := windErrors.FromError(err); ok {
		body, mErr := json.Marshal(wErr)
		if mErr == nil {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(windErrors.CodeToHTTP(wErr.Code))
			_, _ = w.Write(body)
			return
		}
	}
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