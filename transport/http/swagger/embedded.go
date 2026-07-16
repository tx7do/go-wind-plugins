package swagger

import (
	"github.com/swaggest/swgui/v5/static"
	"github.com/vearutop/statigz"
)

// staticServer 提供嵌入的 Swagger UI 静态资源（js/css/favicon 等）。
// 资源来自 swaggest/swgui 的 embed.FS，经 statigz 做嵌入式文件服务。
var staticServer = statigz.FileServer(static.FS)

// 模板中静态资源引用的 URL 前缀占位符。
// 渲染时由 Config.BasePath 替换，确保资源相对于文档基础路径加载。
const (
	assetsBase  = "{{ .BasePath }}"
	faviconBase = "{{ .BasePath }}"
)
