package redoc

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/mvrilo/go-redoc"
	httpPlugin "github.com/tx7do/go-wind-plugins/transport/http"
)

// Config 用于 ReDoc 配置。
type Config struct {
	Title       string // 页面标题。
	Description string // 页面描述。
	BasePath    string // 文档访问基础路径，默认 "/docs/"。
	SpecFile    string // 本地 openapi 文档文件路径。
	SpecPath    string // 规范访问 URL 路径（仅本地 / 内嵌模式），默认 BasePath + "openapi.json"。
	SpecURL     string // 远程 openapi URL。若设置则不从服务端读取文档。
}

// HandlerOption 配置 ReDoc handler。
type HandlerOption func(opt *Config)

// NewConfig 创建带默认值的 ReDoc 配置。
func NewConfig() *Config {
	return &Config{
		BasePath: "/docs/",
	}
}

// WithTitle 设置页面标题。
func WithTitle(title string) HandlerOption {
	return func(opt *Config) { opt.Title = title }
}

// WithDescription 设置页面描述。
func WithDescription(desc string) HandlerOption {
	return func(opt *Config) { opt.Description = desc }
}

// WithBasePath 设置文档访问基础路径。
func WithBasePath(path string) HandlerOption {
	return func(opt *Config) { opt.BasePath = path }
}

// WithLocalFile 设置本地 openapi 文档文件路径，由服务端读取并托管。
func WithLocalFile(filePath string) HandlerOption {
	return func(opt *Config) { opt.SpecFile = filePath }
}

// WithRemoteFileURL 设置远程 openapi 文档 URL，前端直接拉取。
func WithRemoteFileURL(url string) HandlerOption {
	return func(opt *Config) { opt.SpecURL = url }
}

// WithSpecPath 设置规范文档的 URL 访问路径（仅本地 / 内嵌模式有效）。
func WithSpecPath(path string) HandlerOption {
	return func(opt *Config) { opt.SpecPath = path }
}

// Register 把 ReDoc 挂载到 go-wind 的 HTTP 服务器上。
//
// 支持两种数据源：
//   - 本地文件（WithLocalFile）：服务端读取文件并在 SpecPath 托管，前端从同路径拉取。
//   - 远程 URL（WithRemoteFileURL）：渲染的 HTML 直接引用远程文档地址，前端直连。
//
// 用法：
//
//	redoc.Register(srv,
//	    redoc.WithTitle("Petstore"),
//	    redoc.WithLocalFile("./openapi.json"),
//	    redoc.WithBasePath("/docs/"),
//	)
func Register(srv *httpPlugin.Server, opts ...HandlerOption) {
	cfg := NewConfig()
	for _, o := range opts {
		o(cfg)
	}

	basePath := strings.TrimSuffix(cfg.BasePath, "/") + "/"

	if cfg.SpecURL != "" {
		// 远程 URL 模式：渲染内嵌 ReDoc JS 的 HTML，前端直接从远端拉取文档。
		h := newRemoteHandler(cfg.Title, cfg.Description, cfg.SpecURL)
		srv.HandlePrefix(basePath, h)
		return
	}

	// 本地文件模式：使用 go-redoc 原生支持。
	specPath := cfg.SpecPath
	if specPath == "" {
		specPath = strings.TrimRight(basePath, "/") + "/openapi.json"
	}

	doc := redoc.Redoc{
		Title:       cfg.Title,
		Description: cfg.Description,
		SpecFile:    cfg.SpecFile,
		SpecPath:    specPath,
		DocsPath:    "", // 空值 = 所有非 spec 路径都渲染 HTML
	}

	srv.HandlePrefix(basePath, doc.Handler())
}

// ---------------------------------------------------------------------------
// 远程 URL 模式：利用 go-redoc 导出的 ReDoc standalone JS，自行渲染 HTML。
// ---------------------------------------------------------------------------

// remoteDocHandler 为远程 URL 模式提供 ReDoc HTML 页面。
type remoteDocHandler struct {
	title       string
	description string
	specURL     string
}

func newRemoteHandler(title, description, specURL string) http.Handler {
	return &remoteDocHandler{
		title:       title,
		description: description,
		specURL:     specURL,
	}
}

func (h *remoteDocHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 用 go-redoc 导出的 ReDoc standalone JS，内嵌到 HTML 页面中。
	funcMap := template.FuncMap{
		"json": func(v string) string {
			b, _ := json.Marshal(v)
			return string(b)
		},
	}
	t, err := template.New("redoc").Funcs(funcMap).Parse(remoteHTMLTemplate)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.Execute(w, map[string]interface{}{
		"Title":       h.title,
		"Description": h.description,
		"SpecURL":     h.specURL,
		"JavaScript":  template.JS(redoc.JavaScript),
	}); err != nil {
		// 写头后 Execute 失败无法回退，只能 log
		fmt.Printf("redoc: template execute error: %v\n", err)
	}
}

// remoteHTMLTemplate 是 ReDoc HTML 模板，内嵌 standalone JS。
// ReDoc standalone JS 已由 go-redoc 通过 //go:embed 编译进二进制。
const remoteHTMLTemplate = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="utf-8"/>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>{{.Title}} - ReDoc</title>
    {{if .Description}}
    <meta name="description" content="{{.Description}}">
    {{end}}
</head>
<body>
    <div id="redoc-container"></div>
    <script>{{.JavaScript}}</script>
    <script>
    Redoc.init(
        {{.SpecURL | json}},
        {
            scrollYOffset: 0,
            hideDownloadButton: false,
            expandResponses: '',
        },
        document.getElementById('redoc-container')
    );
    </script>
</body>
</html>`
