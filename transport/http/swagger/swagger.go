// Package swagger 提供嵌入式 Swagger UI，可挂载到 go-wind 的 HTTP 服务器上。
//
// 静态资源（swagger-ui 的 js/css/favicon）通过 swaggest/swgui 的 embed.FS 嵌入，
// 文档数据源支持三种：
//   - 远程 URL（WithRemoteFileURL）：前端直接从远端拉取 openapi.json
//   - 本地文件（WithLocalFile）：服务端读取文件并托管为子路由
//   - 内存数据（WithMemoryData）：服务端托管传入的 []byte 为子路由
//
// 用法：
//
//	srv := httpServer.NewServer(":8080", httpServer.WithDriver(std.NewDriver()))
//	swagger.Register(srv,
//	    swagger.WithTitle("Petstore"),
//	    swagger.WithRemoteFileURL("https://petstore3.swagger.io/api/v3/openapi.json"),
//	    swagger.WithBasePath("/docs/"),
//	)
//	// 访问 http://localhost:8080/docs/
package swagger

import (
	"fmt"
	"net/http"
	"path"
	"strings"

	httpPlugin "github.com/tx7do/go-wind-plugins/transport/http"
)

// New 创建 Swagger UI 的 HTTP handler。
//
// 参数：
//   - title: 首页标题
//   - swaggerJSONPath: openapi.json/swagger.json 文档规范的 URL
//   - basePath: 文档访问基础路径（如 "/docs/"）
func New(title, swaggerJSONPath string, basePath string) http.Handler {
	return newHandler(title, swaggerJSONPath, basePath)
}

// NewWithOption 根据可选项创建 Swagger UI 的 HTTP handler。
func NewWithOption(handlerOpts ...HandlerOption) http.Handler {
	opts := NewConfig()

	for _, o := range handlerOpts {
		o(opts)
	}

	return newHandlerWithConfig(opts)
}

// newHandlerWithConfig 根据配置创建 Swagger UI handler。
func newHandlerWithConfig(config *Config) *Handler {
	return NewHandlerWithConfig(config, assetsBase, faviconBase, staticServer)
}

// newHandler 创建 Swagger UI handler（便捷构造）。
func newHandler(title, swaggerJSONPath string, basePath string) *Handler {
	return newHandlerWithConfig(&Config{
		Title:          title,
		SwaggerJsonUrl: swaggerJSONPath,
		BasePath:       basePath,
	})
}

// Register 把 Swagger UI 挂载到 go-wind 的 HTTP 服务器上。
//
// 等价于 srv.HandlePrefix(basePath, handler)，并按数据源类型自动注册
// openapi 文档子路由（仅本地文件 / 内存数据两种服务端托管模式需要）。
// 远程 URL 模式由前端直接拉取，无需注册子路由。
//
// 返回创建的 *Handler，便于调用方进一步控制。
func Register(srv *httpPlugin.Server, opts ...HandlerOption) *Handler {
	cfg := NewConfig()

	for _, o := range opts {
		o(cfg)
	}

	// 按数据源注册 openapi 文档子路由（服务端托管模式）。
	if cfg.LocalOpenApiFile != "" {
		registerOpenApiLocalFileRouter(srv, cfg)
	} else if len(cfg.OpenApiData) != 0 {
		registerOpenApiMemoryDataRouter(srv, cfg)
	}

	swaggerHandler := newHandlerWithConfig(cfg)
	srv.HandlePrefix(swaggerHandler.BasePath, swaggerHandler)

	return swaggerHandler
}

// registerOpenApiLocalFileRouter 加载本地 openapi 文件并注册托管子路由。
// 同时把该子路由 URL 写回 cfg.SwaggerJsonUrl，供前端拉取。
func registerOpenApiLocalFileRouter(srv *httpPlugin.Server, cfg *Config) {
	fileHandler := &openApiFileHandler{}
	if err := fileHandler.LoadFile(cfg.LocalOpenApiFile); err != nil {
		fmt.Println("load openapi file failed: ", err)
		return
	}
	pattern := strings.TrimRight(cfg.BasePath, "/") + "/openapi" + path.Ext(cfg.LocalOpenApiFile)
	cfg.SwaggerJsonUrl = pattern
	srv.HandlePrefix(pattern, fileHandler)
}

// registerOpenApiMemoryDataRouter 注册内存数据托管的 openapi 子路由。
// 注册后清空 cfg.OpenApiData，避免被序列化进前端配置。
func registerOpenApiMemoryDataRouter(srv *httpPlugin.Server, cfg *Config) {
	fileHandler := &openApiFileHandler{Content: cfg.OpenApiData}
	pattern := strings.TrimRight(cfg.BasePath, "/") + "/openapi." + cfg.OpenApiDataType
	cfg.SwaggerJsonUrl = pattern
	srv.HandlePrefix(pattern, fileHandler)
	cfg.OpenApiData = nil
}
