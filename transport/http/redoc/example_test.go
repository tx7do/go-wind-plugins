package redoc_test

import (
	windhttp "github.com/tx7do/go-wind-plugins/transport/http"
	"github.com/tx7do/go-wind-plugins/transport/http/redoc"
)

// ExampleRegister 演示如何把嵌入式 ReDoc 挂到 HTTP 服务器的 /docs/ 前缀下。
// ReDoc 引擎（standalone JS）从远端拉取 openapi.json 渲染文档；本地文件模式
// 改用 WithLocalFile。
//
// Register 内部调用 srv.HandlePrefix("/docs/", handler)——前缀路由不经过
// Use 注册的中间件链。实际运行需先通过 WithDriver 设置驱动
// （见 transport/http/driver/std 的示例）。
func ExampleRegister() {
	srv := windhttp.NewServer(":8080")

	redoc.Register(srv,
		redoc.WithTitle("GoWind ReDoc Demo"),
		redoc.WithDescription("A sample API powered by ReDoc"),
		redoc.WithRemoteFileURL("https://petstore3.swagger.io/api/v3/openapi.json"),
		redoc.WithBasePath("/docs/"),
	)
}
