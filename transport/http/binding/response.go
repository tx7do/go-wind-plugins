package binding

import (
	"encoding/json"
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