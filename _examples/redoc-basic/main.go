// Package main 演示如何把嵌入式 ReDoc 挂载到 go-wind 的 HTTP 服务器上。
//
// 本例展示远程 URL 模式：HTML 页面内嵌 ReDoc 引擎（go-redoc 的 standalone JS），
// 从远端拉取 openapi.json 渲染文档。也支持本地文件模式（见注释）。
//
// Run:
//
//	go run ./_examples/redoc-basic
//
// Test:
//
//	curl -sI http://localhost:8080/docs/     # 200, text/html
//	curl -s http://localhost:8080/hello       # 业务路由
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	httpServer "github.com/tx7do/go-wind-plugins/transport/http"
	"github.com/tx7do/go-wind-plugins/transport/http/driver/std"
	"github.com/tx7do/go-wind-plugins/transport/http/redoc"
)

func main() {
	// 创建一个监听 :8080 的 HTTP 服务器，使用标准库 driver。
	srv := httpServer.NewServer(":8080", httpServer.WithDriver(std.NewDriver()))

	// 注册一个普通业务路由。
	srv.GET("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintln(w, "Hello, GoWind!")
	})

	// 远程 URL 模式：前端从远端拉取 openapi.json。
	redoc.Register(srv,
		redoc.WithTitle("GoWind ReDoc Demo"),
		redoc.WithDescription("A sample API powered by ReDoc"),
		redoc.WithRemoteFileURL("https://petstore3.swagger.io/api/v3/openapi.json"),
		redoc.WithBasePath("/docs/"),
	)

	// 本地文件模式（打开注释即可使用）：
	// redoc.Register(srv,
	//     redoc.WithTitle("Petstore"),
	//     redoc.WithLocalFile("./openapi.json"),
	//     redoc.WithBasePath("/docs/"),
	// )

	// 优雅关闭：收到 SIGINT / SIGTERM 时退出。
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Printf("HTTP server listening on %s\n", srv.Endpoint())
	fmt.Println("ReDoc UI at http://localhost:8080/docs/")
	if err := srv.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("server stopped")
}
