package swagger

// HandlerOption 配置 Swagger UI handler。
type HandlerOption func(opt *Config)

// Config 用于 Swagger UI handler 配置。
type Config struct {
	Title          string `json:"title"`          // 首页标题。
	SwaggerJsonUrl string `json:"swaggerJsonUrl"` // openapi.json/swagger.json 文档规范的 URL。
	BasePath       string `json:"basePath"`       // 文档访问的基础 URL。

	ShowTopBar         bool              `json:"showTopBar"`         // 是否显示顶部导航栏，默认隐藏。
	HideCurl           bool              `json:"hideCurl"`           // 是否隐藏 curl 代码片段。
	JsonEditor         bool              `json:"jsonEditor"`         // 启用可视化 JSON 编辑器（实验性，复杂 schema 可能出错）。
	PreAuthorizeApiKey map[string]string `json:"preAuthorizeApiKey"` // 安全名到 key 值的映射。

	// SettingsUI 包含 SwaggerUIBundle 配置的键和纯 javascript 值。
	// 覆盖默认值。
	// 可用选项见 https://swagger.io/docs/open-source-tools/swagger-ui/usage/configuration/。
	SettingsUI map[string]string `json:"-"`

	LocalOpenApiFile string `json:"-"` // 本地 openapi 文件路径。

	OpenApiData     []byte `json:"-"` // 内存中的 openapi 文档数据。
	OpenApiDataType string `json:"-"` // 内存数据的扩展名（如 json/yaml）。
}

// NewConfig 创建带默认值的 Swagger UI 配置。
func NewConfig() *Config {
	return &Config{
		BasePath: "/docs/",
	}
}

// WithTitle 设置首页标题。
func WithTitle(title string) HandlerOption {
	return func(opt *Config) {
		opt.Title = title
	}
}

// WithBasePath 设置文档访问的基础 URL。
func WithBasePath(path string) HandlerOption {
	return func(opt *Config) {
		opt.BasePath = path
	}
}

// WithShowTopBar 设置是否显示导航顶部栏，默认隐藏。
func WithShowTopBar(show bool) HandlerOption {
	return func(opt *Config) {
		opt.ShowTopBar = show
	}
}

// WithHideCurl 设置是否隐藏 curl 代码片段。
func WithHideCurl(hide bool) HandlerOption {
	return func(opt *Config) {
		opt.HideCurl = hide
	}
}

// WithJsonEditor 启用可视化 JSON 编辑器支持（实验性，复杂 schema 可能出错）。
func WithJsonEditor(enable bool) HandlerOption {
	return func(opt *Config) {
		opt.JsonEditor = enable
	}
}

// WithPreAuthorizeApiKey 设置安全名到 key 值的映射。
func WithPreAuthorizeApiKey(keys map[string]string) HandlerOption {
	return func(opt *Config) {
		opt.PreAuthorizeApiKey = keys
	}
}

// WithSettingsUI 设置 SwaggerUIBundle 配置项（键和纯 javascript 值），覆盖默认值。
// 可用选项见 https://swagger.io/docs/open-source-tools/swagger-ui/usage/configuration/。
func WithSettingsUI(settings map[string]string) HandlerOption {
	return func(opt *Config) {
		opt.SettingsUI = settings
	}
}

// WithLocalFile 设置本地 openapi 文档文件路径，由服务端读取并托管。
func WithLocalFile(filePath string) HandlerOption {
	return func(opt *Config) {
		opt.LocalOpenApiFile = filePath
	}
}

// WithMemoryData 设置内存中的 openapi 文档数据及扩展名（如 json/yaml），由服务端托管。
func WithMemoryData(content []byte, ext string) HandlerOption {
	return func(opt *Config) {
		opt.OpenApiData = content
		opt.OpenApiDataType = ext
	}
}

// WithRemoteFileURL 设置 openapi.json/swagger.json 文档规范的远程 URL。
func WithRemoteFileURL(url string) HandlerOption {
	return func(opt *Config) {
		opt.SwaggerJsonUrl = url
	}
}
