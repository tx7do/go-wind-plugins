package swagger_test

import (
	windhttp "github.com/tx7do/go-wind-plugins/transport/http"
	"github.com/tx7do/go-wind-plugins/transport/http/swagger"
)

// ExampleRegister 演示如何把嵌入式 Swagger UI 挂到 HTTP 服务器的 /docs/ 前缀下，
// 文档从远程 openapi.json 拉取；本地文件模式改用 WithLocalFile。
//
// Register 内部调用 srv.HandlePrefix("/docs/", handler)——前缀路由不经过
// Use 注册的中间件链。实际运行需先通过 WithDriver 设置驱动
// （见 transport/http/driver/std 的示例）。
func ExampleRegister() {
	srv := windhttp.NewServer(":8080")

	swagger.Register(srv,
		swagger.WithTitle("GoWind Swagger Demo"),
		swagger.WithRemoteFileURL("https://petstore3.swagger.io/api/v3/openapi.json"),
		swagger.WithBasePath("/docs/"),
	)
}
