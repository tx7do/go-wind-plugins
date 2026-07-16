package swagger

import (
	"encoding/json"
	"html/template"
	"net/http"
	"strings"
)

// Handler 处理 Swagger UI 请求。
type Handler struct {
	*Config

	ConfigJson template.JS

	tpl          *template.Template
	staticServer http.Handler
}

// NewHandlerWithConfig 根据配置返回 Swagger UI 的 HTTP handler。
func NewHandlerWithConfig(config *Config, assetsBase, faviconBase string, staticServer http.Handler) *Handler {
	config.BasePath = strings.TrimSuffix(config.BasePath, "/") + "/"

	h := &Handler{
		Config: config,
	}

	j, err := json.Marshal(h.Config)
	if err != nil {
		panic(err)
	}

	h.ConfigJson = template.JS(j) //nolint:gosec // Data is well-formed.

	h.tpl, err = template.New("index").Parse(IndexTpl(assetsBase, faviconBase, config))
	if err != nil {
		panic(err)
	}

	if staticServer != nil {
		h.staticServer = http.StripPrefix(h.BasePath, staticServer)
	}

	return h
}

// ServeHTTP 实现 http.Handler 接口，处理 Swagger UI 请求。
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSuffix(r.URL.Path, "/") != strings.TrimSuffix(h.BasePath, "/") && h.staticServer != nil {
		h.staticServer.ServeHTTP(w, r)

		return
	}

	w.Header().Set("Content-Type", "text/html")

	if err := h.tpl.Execute(w, h); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
