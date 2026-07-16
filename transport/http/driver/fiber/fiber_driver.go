package fiber

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/gofiber/fiber/v2"
	httpPlugin "github.com/tx7do/go-wind-plugins/transport/http"
)

// Driver 基于 fiber(fasthttp) 框架实现的 HTTP 服务器驱动。
//
// 它同时实现 httpPlugin.Driver 接口（Handle/Start/Stop，接收标准
// http.HandlerFunc，经 net/http↔fasthttp 适配层）和框架特有的
// HandleFiber（接收原生 fiber.Handler，直接用 *fiber.Ctx）。
//
// 选型说明：
//   - 用 Handle（继承自 Driver 接口）：handler 走标准 net/http 接口，
//     能复用本插件的全部中间件与 binding；但经适配层，无法用 *fiber.Ctx，
//     且有一份 net/http 转换开销。
//   - 用 HandleFiber：handler 直接拿原生 *fiber.Ctx，可用 fiber 全部能力
//     （Params/Query/JSON/QueryParser 等），无适配层开销（性能更好），
//     适合复用存量 fiber 业务代码或追求性能的场景；但这些路由不经过
//     httpPlugin.Server 注册的中间件链。
type Driver struct {
	app      *fiber.App
	cfg      fiber.Config // fiber.App 配置，在 app 创建前由选项填充
	stopOnce sync.Once
	stopErr  error
}

// 编译期确保 *Driver 实现 httpPlugin.Driver 接口。
var _ httpPlugin.Driver = (*Driver)(nil)

// Option 用于在构造 Driver 时注入配置：既包括 fiber.App 的配置
// （WithDisableStartupMessage 等），也包括框架原生的路由与中间件
// （WithRoute / WithMiddleware）。
//
// 与 httpPlugin.Option（作用于 Server）不同，本选项作用于 driver 本身，
// 让用户能在纯 options 风格下复用存量 fiber 业务代码。
//
// 示例：
//
//	d := fiber.New(
//	    fiber.WithDisableStartupMessage(),
//	    fiber.WithRoute("GET", "/users/:id", GetUser),
//	    fiber.WithMiddleware(middleware.Logger()),
//	)
//	srv := httpPlugin.NewServer(":8080", httpPlugin.WithDriver(d))
type Option func(*Driver)

// NewDriver 创建一个基于 fiber 的驱动实例，返回 httpPlugin.Driver 接口。
//
// 适用于只需要标准 http.HandlerFunc 接口、不需要框架特有能力的场景。
// 若需注册 fiber 原生 handler（复用存量 fiber 代码或追求性能），改用 New。
func NewDriver(opts ...Option) httpPlugin.Driver {
	return New(opts...)
}

// New 创建一个基于 fiber 的驱动实例，返回具体类型 *Driver。
//
// 选项应用分两阶段：
//  1. config 类选项（如 WithDisableStartupMessage）填充 d.cfg，在 fiber.New 前生效
//  2. 路由/中间件类选项（WithRoute / WithMiddleware）在 app 创建后应用
//
// 这是因为 fiber.Config 必须在 fiber.New() 时确定，而路由注册需要已创建的 app。
// New 内部按"先建 app、再应用所有选项"的顺序处理，选项函数内部自行判断阶段。
func New(opts ...Option) *Driver {
	d := &Driver{}
	// 第一遍：config 类选项填充 d.cfg（此时 d.app==nil，路由类选项 no-op）。
	for _, opt := range opts {
		opt(d)
	}
	// 用收集到的 config 创建 app。
	d.app = fiber.New(d.cfg)
	// 第二遍：路由/中间件类选项此时 d.app!=nil，真正注册。
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// WithDisableStartupMessage 关闭 fiber 启动时打印的 banner（默认开启）。
// 适用于 benchmark、测试等不希望 banner 污染输出的场景。
//
// 属于 config 类选项：仅在新 的第一遍（app 创建前）修改 cfg，
// 第二遍（app 已建）为 no-op，安全。
func WithDisableStartupMessage() Option {
	return func(d *Driver) {
		if d.app == nil {
			d.cfg.DisableStartupMessage = true
		}
	}
}

// WithRoute 声明一个 fiber 原生路由（method + path + fiber.Handler）。
//
// 用于在 New(...) 时复用存量 fiber handler，handler 拿到真正的 *fiber.Ctx，
// 可用 fiber 全部能力（Params/Query/JSON/QueryParser 等）。
//
// 注意：通过本选项注册的路由不经过 httpPlugin.Server.Use 注册的中间件链
// （那些中间件基于 http.HandlerFunc）。如需中间件，配合 WithMiddleware
// 注册 fiber 原生中间件。
func WithRoute(method, path string, handler fiber.Handler) Option {
	return func(d *Driver) {
		if d.app != nil {
			d.app.Add(method, path, handler)
		}
	}
}

// WithMiddleware 注册 fiber 原生中间件，作用于通过 WithRoute 注册的路由。
//
// 注意：这些中间件与 httpPlugin.Server.Use 的中间件是两套体系，互不作用。
// 框架原生路由用 fiber 中间件，标准 http.HandlerFunc 路由用 Server 中间件。
func WithMiddleware(middleware ...fiber.Handler) Option {
	return func(d *Driver) {
		if d.app != nil {
			// fiber.App.Use 收 ...interface{}，需转换切片类型。
			args := make([]interface{}, len(middleware))
			for i, m := range middleware {
				args[i] = m
			}
			d.app.Use(args...)
		}
	}
}

// Handle 注册路由处理器，将标准 http.HandlerFunc 适配为 fiber handler。
//
// 这是 httpPlugin.Driver 接口的实现：handler 走标准 net/http 接口，
// 经适配层（net/http ↔ fasthttp 转换），可复用本插件中间件与 binding，
// 但无法使用 *fiber.Ctx，且有一份转换开销。
func (d *Driver) Handle(method, path string, handler http.HandlerFunc) {
	d.app.Add(method, path, adaptFiber(handler))
}

// HandleFiber 注册 fiber 原生 handler（func(*fiber.Ctx) error）。
//
// 与 Handle 的区别：
//   - handler 直接接收 *fiber.Ctx，可用 fiber 全部能力
//     （c.Params / c.Query / c.JSON / c.QueryParser / 路径参数 :id 等）
//   - 不经适配层，性能更好（无 net/http 转换开销，见 benchmark）
//   - 适用于复用存量 fiber 业务代码，无需改写为 http.HandlerFunc
//
// 注意：通过 HandleFiber 注册的路由不经过 httpPlugin.Server.Use 注册的
// 中间件链（因为那些中间件基于 http.HandlerFunc）。如需对这类路由
// 应用 fiber 中间件，请用 App() 获取底层应用直接注册。
func (d *Driver) HandleFiber(method, path string, handler fiber.Handler) {
	d.app.Add(method, path, handler)
}

// HandlePrefix 在 prefix 下挂载 handler，匹配 prefix 及其所有子路径。
// 用 fiber 的通配符路由 "<prefix>/*" 实现，经 net/http↔fasthttp 适配层
// （复用 adaptFiber 的 sync.Pool 优化），handler 接收原始请求路径。
// 注册所有 HTTP 方法（Use），适用于静态资源、Swagger UI 等场景。
func (d *Driver) HandlePrefix(prefix string, h http.Handler) {
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	// adaptFiber 接收 http.HandlerFunc；将 http.Handler 适配为 http.HandlerFunc。
	adapter := adaptFiber(http.HandlerFunc(h.ServeHTTP))
	d.app.Use(prefix+"*", adapter)
}

// App 返回底层 *fiber.App，用于需要直接操作应用的高级场景
// （注册路由组、fiber 原生中间件、websocket 等）。
//
// 警告：直接操作 App 会绕过 Driver 抽象，用户代码将与 fiber 强耦合，
// 失去 driver 可替换性。仅在确有需要时使用。
func (d *Driver) App() *fiber.App {
	return d.app
}

// Start 启动服务器并阻塞，直到 ctx 被取消时执行优雅关闭。
// listener 由 Server 创建并传入（已处理 TLS 包装）。
func (d *Driver) Start(ctx context.Context, ln net.Listener) error {
	errChan := make(chan error, 1)
	go func() {
		if err := d.app.Listener(ln); err != nil {
			errChan <- err
			return
		}
		errChan <- nil
	}()
	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return d.Stop(context.Background())
	}
}

// Stop 优雅关闭服务器，保证只关闭一次。
func (d *Driver) Stop(_ context.Context) error {
	d.stopOnce.Do(func() {
		d.stopErr = d.app.Shutdown()
	})
	return d.stopErr
}

// adaptFiber 将标准 http.HandlerFunc 适配为 fiber.Handler。
// 由于 fiber 基于 fasthttp，与 net/http 不兼容，这里做必要的请求/响应转换。
//
// 性能优化：请求/响应路径上分配密集的对象（http.Header 的 map、bytes.Buffer、
// 请求体 reader）通过 sync.Pool 复用，避免每请求大量堆分配。
// *http.Request / *url.URL 因字段多且含 context 难以安全复用，仍每次构造。
func adaptFiber(h http.HandlerFunc) fiber.Handler {
	return func(c *fiber.Ctx) error {
		rw := acquireResponseWriter()
		req := newRequest(c)
		h(rw, req)
		err := rw.flush(c)
		// 归还池化对象：请求体 reader、请求 header map、响应 writer。
		// 注意顺序：body.Close 幂等，handler 可能已 Close。
		_ = req.Body.Close()
		releaseHeader(req.Header)
		releaseResponseWriter(rw)
		return err
	}
}

// newRequest 从 fiber.Ctx 构造标准 http.Request。
//
// 优化点：
//   - http.Header 复用池中的 map（reset 后保留容量），省掉每请求 make(map)。
//   - 请求体用池化的 bodyCloser 包装，避免 io.NopCloser 的每次分配。
//   - scheme 默认 http 时直接用常量，避免 []byte→string 拷贝（仅 HTTPS 时才转换）。
func newRequest(c *fiber.Ctx) *http.Request {
	u := &url.URL{}
	u.Scheme = "http"
	if s := c.Request().URI().Scheme(); len(s) > 0 && bytes.Equal(s, strHTTPS) {
		u.Scheme = "https"
	}
	host := string(c.Request().Header.Host())
	u.Host = host
	u.Path = string(c.Request().URI().Path())
	u.RawQuery = string(c.Request().URI().QueryString())

	header := acquireHeader()
	c.Request().Header.VisitAll(func(k, v []byte) {
		header.Add(string(k), string(v))
	})

	req := &http.Request{
		Method: c.Method(),
		URL:    u,
		Header: header,
		Body:   acquireBodyCloser(c.Body()),
		Host:   host,
	}
	return req
}

// fiberResponseWriter 实现 http.ResponseWriter，将写入缓存后统一 flush 到 fiber.Ctx。
type fiberResponseWriter struct {
	header http.Header
	status int
	body   *bytes.Buffer
}

func (w *fiberResponseWriter) Header() http.Header { return w.header }

func (w *fiberResponseWriter) Write(b []byte) (int, error) {
	return w.body.Write(b)
}

func (w *fiberResponseWriter) WriteHeader(statusCode int) {
	w.status = statusCode
}

// flush 将缓存的状态码、响应头和响应体写入 fiber.Ctx。
//   - 响应头用 SetCanonical（fiber 内部对 header 名做 []byte 处理）。
//   - 响应体用 SetBody（拷贝到 fasthttp 内部 buffer）：因 fasthttp 在
//     handler 返回后才序列化，池化 buffer 不能安全地通过 SetBodyRaw 交付。
func (w *fiberResponseWriter) flush(c *fiber.Ctx) error {
	c.Status(w.status)

	for k, vs := range w.header {
		kBytes := []byte(k)
		for _, v := range vs {
			c.Response().Header.SetCanonical(kBytes, []byte(v))
		}
	}

	body := w.body.Bytes()
	if len(body) == 0 {
		return nil
	}
	c.Response().SetBody(body)
	return nil
}

// --- sync.Pool 优化（每请求对象复用） ---

var (
	headerPool = sync.Pool{
		New: func() any { return make(http.Header, 16) },
	}

	responseWriterPool = sync.Pool{
		New: func() any {
			return &fiberResponseWriter{
				header: make(http.Header, 8),
				body:   &bytes.Buffer{},
			}
		},
	}

	bodyCloserPool = sync.Pool{
		New: func() any { return &bodyCloser{r: &bytes.Reader{}} },
	}
)

// acquireHeader 从池中取一个已 reset 的 http.Header（保留底层 map 容量）。
func acquireHeader() http.Header {
	h := headerPool.Get().(http.Header)
	clear(h)
	return h
}

// releaseHeader 归还请求 header map（重置后保留容量）。
func releaseHeader(h http.Header) {
	clear(h)
	headerPool.Put(h)
}

// acquireResponseWriter 从池中取一个已重置的 fiberResponseWriter。
func acquireResponseWriter() *fiberResponseWriter {
	w := responseWriterPool.Get().(*fiberResponseWriter)
	clear(w.header)
	w.status = http.StatusOK
	w.body.Reset()
	return w
}

// releaseResponseWriter 归还响应 writer。
// body.Buffer 与 header map 的重置在 acquireResponseWriter 中完成（取出时清空），
// 这里直接归还即可；归还后不可再被当前 goroutine 引用。
func releaseResponseWriter(w *fiberResponseWriter) {
	responseWriterPool.Put(w)
}

// bodyCloser 实现 io.ReadCloser，包装一个可 Reset 的 bytes.Reader。
// 池化以替代每请求的 io.NopCloser(bytes.NewReader(...))。
type bodyCloser struct {
	r      *bytes.Reader
	closed bool
}

func acquireBodyCloser(b []byte) *bodyCloser {
	bc := bodyCloserPool.Get().(*bodyCloser)
	bc.r.Reset(b)
	bc.closed = false
	return bc
}

func (bc *bodyCloser) Read(p []byte) (int, error) { return bc.r.Read(p) }

// WriteTo 透传内部 bytes.Reader 的 WriteTo，使 io.Copy 走零中间缓冲路径，
// 避免 io.copyBuffer 每次分配 32KB 临时 buffer。这是请求体读取场景的关键优化。
func (bc *bodyCloser) WriteTo(w io.Writer) (int64, error) { return bc.r.WriteTo(w) }

// Close 归还到 pool。幂等：重复 Close 不 panic、不重复 Put。
// 同时兼容 handler 内部主动调用 Close 的情况。
func (bc *bodyCloser) Close() error {
	if bc.closed {
		return nil
	}
	bc.closed = true
	bc.r.Reset(nil)
	bodyCloserPool.Put(bc)
	return nil
}

// --- 常量 ---

var (
	strHTTPS = []byte("https")
)
