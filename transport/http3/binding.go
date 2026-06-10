package http3

import (
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
)

// BindQuery binds URL query parameters to a struct using struct tags.
// Supports "json" and "form" tags.
func BindQuery(values url.Values, obj any) error {
	return bindURLValues(values, obj)
}

// BindForm binds form values to a struct.
func BindForm(req *http.Request, obj any) error {
	if err := req.ParseForm(); err != nil {
		return err
	}
	return bindURLValues(req.Form, obj)
}

// bindURLValues maps url.Values to a struct using reflection.
func bindURLValues(values url.Values, obj any) error {
	v := reflect.ValueOf(obj)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return nil
	}
	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return nil
	}

	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldValue := v.Field(i)

		// Get tag name
		name := field.Tag.Get("json")
		if name == "" {
			name = field.Tag.Get("form")
		}
		if name == "" {
			name = field.Tag.Get("query")
		}
		if name == "" {
			continue
		}
		// Trim options like "omitempty"
		if idx := strings.Index(name, ","); idx >= 0 {
			name = name[:idx]
		}
		if name == "-" {
			continue
		}

		val, ok := values[name]
		if !ok || len(val) == 0 || val[0] == "" {
			continue
		}

		if err := setField(fieldValue, val[0]); err != nil {
			return err
		}
	}
	return nil
}

func setField(field reflect.Value, value string) error {
	if !field.CanSet() {
		return nil
	}

	switch field.Kind() {
	case reflect.String:
		field.SetString(value)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return err
		}
		field.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return err
		}
		field.SetUint(n)
	case reflect.Float32, reflect.Float64:
		n, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return err
		}
		field.SetFloat(n)
	case reflect.Bool:
		b, err := strconv.ParseBool(value)
		if err != nil {
			return err
		}
		field.SetBool(b)
	case reflect.Ptr:
		if field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
		return setField(field.Elem(), value)
	default:
		// For complex types, try JSON unmarshal
		if field.CanAddr() {
			return json.Unmarshal([]byte(`"`+value+`"`), field.Addr().Interface())
		}
	}
	return nil
}
