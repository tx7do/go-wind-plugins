package binding

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/tx7do/go-wind-plugins/encoding"
	_ "github.com/tx7do/go-wind-plugins/encoding/json"
)

var bodyCodec encoding.Codec = encoding.GetCodec("json")

func SetCodec(c encoding.Codec) {
	if c != nil {
		bodyCodec = c
	}
}

func GetCodec() encoding.Codec {
	return bodyCodec
}

// Router is implemented by httpserver.Server.
type Router interface {
	Handle(method, path string, handler http.HandlerFunc)
}

var pathParamFunc = func(r *http.Request, name string) string {
	return r.PathValue(name)
}

func SetPathParamFunc(f func(*http.Request, string) string) {
	pathParamFunc = f
}

func PathParam(r *http.Request, name string) string {
	return pathParamFunc(r, name)
}

func BindPath(req interface{}, fieldPath string, value string) error {
	return setFieldByPath(req, fieldPath, value)
}

func BindAllPaths(req interface{}, r *http.Request, pathVars []string) error {
	for _, name := range pathVars {
		val := PathParam(r, name)
		if val != "" {
			if err := BindPath(req, name, val); err != nil {
				return err
			}
		}
	}
	return nil
}

// BindQuery 将 URL query 参数绑定到结构体 req。
//
// 采用反射直填（与 gin/echo 的 form binding 一致，与 BindPath 同源）：
//   - 单值 → 按字段类型转换（string/int/uint/float/bool）
//   - 多值 → []string 或 []T（T 为标量类型）
//   - 单值给 slice 字段 → 自动包成单元素 slice
//   - 字段匹配：json tag → camelCase 回退 → 大小写不敏感（与 BindPath 统一）
//
// 相比 JSON 中转方案，本实现同时解决了：性能（无序列化）、功能（支持 int/bool 等标量）、
// 一致性（与 BindPath 共用 findField/setScalar）三个问题。
func BindQuery(req interface{}, query url.Values) error {
	if len(query) == 0 {
		return nil
	}
	v := reflect.ValueOf(req)
	if v.Kind() != reflect.Ptr {
		return fmt.Errorf("binding: BindQuery requires a pointer, got %T", req)
	}
	v = v.Elem()
	for key, values := range query {
		if len(values) == 0 {
			continue
		}
		field, err := findField(v, key)
		if err != nil {
			continue // 未匹配的 key 静默忽略（与主流框架一致）
		}
		if field.Kind() == reflect.Slice {
			if err := setSliceField(field, values); err != nil {
				return err
			}
		} else {
			if err := setScalar(field, values[0]); err != nil {
				return err
			}
		}
	}
	return nil
}

// setSliceField 将多值填入 slice 字段。
// 支持 []string 和标量类型的 []T（T 为 int/uint/float/bool）。
// 单值场景（len(values)==1）会包成单元素 slice。
func setSliceField(field reflect.Value, values []string) error {
	// 解指针：slice 字段本身不可能是 ptr（slice 是引用类型），但保守处理。
	for field.Kind() == reflect.Ptr {
		if field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
		field = field.Elem()
	}
	if field.Kind() != reflect.Slice {
		// 非 slice（findField 命中但类型不符），降级用首个标量值。
		return setScalar(field, values[0])
	}

	elemType := field.Type().Elem()
	out := reflect.MakeSlice(field.Type(), len(values), len(values))
	for i, val := range values {
		if err := setScalar(out.Index(i), val); err != nil {
			return fmt.Errorf("binding: slice element %d: %w", i, err)
		}
		_ = elemType // setScalar 内部按 Kind 处理，elemType 仅作文档
	}
	field.Set(out)
	return nil
}

func BindBody(r *http.Request, req interface{}) error {
	if r.Body == nil {
		return nil
	}
	defer r.Body.Close()
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}
	return bodyCodec.Unmarshal(data, req)
}

func BindBodyField(r *http.Request, req interface{}, fieldPath string) error {
	if r.Body == nil {
		return nil
	}
	ptr := getFieldPtr(reflect.ValueOf(req), fieldPath)
	if !ptr.IsValid() {
		return fmt.Errorf("binding: field %q not found", fieldPath)
	}
	defer r.Body.Close()
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}
	return bodyCodec.Unmarshal(data, ptr.Interface())
}

func setFieldByPath(req interface{}, fieldPath string, value string) error {
	parts := strings.Split(fieldPath, ".")
	v := reflect.ValueOf(req)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	for i, part := range parts {
		field, err := findField(v, part)
		if err != nil {
			return err
		}
		if i == len(parts)-1 {
			return setScalar(field, value)
		}
		if field.Kind() == reflect.Ptr {
			if field.IsNil() {
				field.Set(reflect.New(field.Type().Elem()))
			}
			v = field.Elem()
		} else {
			v = field
		}
	}
	return nil
}

func getFieldPtr(v reflect.Value, fieldPath string) reflect.Value {
	parts := strings.Split(fieldPath, ".")
	for _, part := range parts {
		if v.Kind() == reflect.Ptr {
			if v.IsNil() {
				v.Set(reflect.New(v.Type().Elem()))
			}
			v = v.Elem()
		}
		field, err := findField(v, part)
		if err != nil {
			return reflect.Value{}
		}
		v = field
	}
	if v.CanAddr() {
		return v.Addr()
	}
	return reflect.Value{}
}

// findField 在 struct v 中按 name 查找字段，三段优先级：
//  1. json tag 名（经缓存预计算，消除 strings.Split）
//  2. toCamelCase(name) 精确匹配字段名
//  3. 大小写不敏感（经缓存预计算字段名小写）
//
// 字段元数据按 reflect.Type 缓存（sync.Map），避免每次调用重复解析 tag。
func findField(v reflect.Value, name string) (reflect.Value, error) {
	if !v.IsValid() {
		return reflect.Value{}, fmt.Errorf("binding: invalid value for field %q", name)
	}

	meta := lookupFieldMeta(v.Type())

	// 1. json tag 名（最高优先级，命中即返回）。
	if idx, ok := meta.byJSON[name]; ok {
		return v.Field(idx), nil
	}

	// 2. camelCase 精确匹配字段名。
	if camel := toCamelCase(name); camel != "" {
		if field := v.FieldByName(camel); field.IsValid() {
			return field, nil
		}
	}

	// 3. 大小写不敏感（按字段名小写预计算表查找）。
	if idx, ok := meta.byLower[strings.ToLower(name)]; ok {
		return v.Field(idx), nil
	}

	return reflect.Value{}, fmt.Errorf("binding: field %q not found", name)
}

func setScalar(field reflect.Value, value string) error {
	for field.Kind() == reflect.Ptr {
		if field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
		field = field.Elem()
	}
	if !field.CanSet() {
		return fmt.Errorf("binding: field is not settable")
	}
	switch field.Kind() {
	case reflect.String:
		field.SetString(value)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("binding: cannot parse %q as int: %w", value, err)
		}
		field.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return fmt.Errorf("binding: cannot parse %q as uint: %w", value, err)
		}
		field.SetUint(n)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("binding: cannot parse %q as float: %w", value, err)
		}
		field.SetFloat(f)
	case reflect.Bool:
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("binding: cannot parse %q as bool: %w", value, err)
		}
		field.SetBool(b)
	default:
		return fmt.Errorf("binding: unsupported field kind %s for value %q", field.Kind(), value)
	}
	return nil
}

func toCamelCase(s string) string {
	if s == "" {
		return ""
	}
	parts := strings.Split(s, "_")
	var sb strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		sb.WriteString(strings.ToUpper(part[:1]))
		if len(part) > 1 {
			sb.WriteString(part[1:])
		}
	}
	return sb.String()
}