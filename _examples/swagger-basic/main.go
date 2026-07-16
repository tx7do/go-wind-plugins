// Package main 演示如何把嵌入式 Swagger UI 挂载到 go-wind 的 HTTP 服务器上。
//
// 本例用最简单的组合：std driver + 远程 openapi.json。
// 启动后访问 http://localhost:8080/docs/ 即可看到 Swagger UI，
// 它会从 https://petstore3.swagger.io/api/v3/openapi.json 拉取文档。
//
// Run:
//
//	go run ./_examples/swagger-basic
//
// Test:
//
//	curl -sI http://localhost:8080/docs/                                  # 200, text/html
//	curl -sI http://localhost:8080/docs/swagger-ui-bundle.js              # 200, application/js
//	curl http://localhost:8080/hello                                      # 业务路由
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
	"github.com/tx7do/go-wind-plugins/transport/http/swagger"
)

func main() {
	// 创建一个监听 :8080 的 HTTP 服务器，使用标准库 driver。
	srv := httpServer.NewServer(":8080", httpServer.WithDriver(std.NewDriver()))

	// 注册一个普通业务路由。
	srv.GET("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintln(w, "Hello, GoWind!")
	})

	// 把 Swagger UI 挂到 /docs/ 下，文档从远程 openapi.json 拉取。
	// Register 内部调用 srv.HandlePrefix("/docs/", handler)，匹配 /docs/ 及其所有子路径。
	swagger.Register(srv,
		swagger.WithTitle("GoWind Swagger Demo"),
		swagger.WithRemoteFileURL("https://petstore3.swagger.io/api/v3/openapi.json"),
		swagger.WithBasePath("/docs/"),
	)

	// 优雅关闭：收到 SIGINT / SIGTERM 时退出。
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Printf("HTTP server listening on %s\n", srv.Endpoint())
	fmt.Println("Swagger UI at http://localhost:8080/docs/")
	if err := srv.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("server stopped")
}
