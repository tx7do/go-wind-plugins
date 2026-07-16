package swagger

import (
	"io"
	"net/http"
	"os"
)

// openApiFileHandler 托管 openapi 文档内容（来自本地文件或内存数据），
// 供 Swagger UI 前端通过 SwaggerJsonUrl 拉取。
type openApiFileHandler struct {
	Content []byte
}

// ServeHTTP 返回托管的 openapi 文档内容。
func (h *openApiFileHandler) ServeHTTP(writer http.ResponseWriter, _ *http.Request) {
	_, _ = writer.Write(h.Content)
}

// loadOpenApiFile 从磁盘读取 openapi 文件内容。
func (h *openApiFileHandler) loadOpenApiFile(filePath string) ([]byte, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	return content, err
}

// LoadFile 从磁盘加载 openapi 文件到内存，供后续托管。
func (h *openApiFileHandler) LoadFile(filePath string) error {
	content, err := h.loadOpenApiFile(filePath)
	if err != nil {
		return err
	}

	h.Content = content
	return nil
}
