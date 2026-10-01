package binding

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Codec accessors
// ---------------------------------------------------------------------------

type mockCodec struct{ name string }

func (m mockCodec) Marshal(v any) ([]byte, error) { return []byte("mock"), nil }
func (m mockCodec) Unmarshal([]byte, any) error   { return nil }
func (m mockCodec) Name() string                  { return m.name }

func TestSetAndGetCodec(t *testing.T) {
	orig := GetCodec()
	defer SetCodec(orig)

	c := mockCodec{name: "mock-codec"}
	SetCodec(c)
	if GetCodec().Name() != "mock-codec" {
		t.Errorf("GetCodec().Name() = %q, want mock-codec", GetCodec().Name())
	}

	// Setting nil must be ignored.
	SetCodec(nil)
	if GetCodec().Name() != "mock-codec" {
		t.Error("SetCodec(nil) should keep the previous codec")
	}
}

// ---------------------------------------------------------------------------
// Path params
// ---------------------------------------------------------------------------

func TestSetPathParamFuncAndPathParam(t *testing.T) {
	orig := pathParamFunc
	defer func() { pathParamFunc = orig }()

	SetPathParamFunc(func(r *http.Request, name string) string {
		if name == "id" {
			return "42"
		}
		return ""
	})

	req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	if got := PathParam(req, "id"); got != "42" {
		t.Errorf("PathParam(id) = %q, want 42", got)
	}
	if got := PathParam(req, "missing"); got != "" {
		t.Errorf("PathParam(missing) = %q, want empty", got)
	}
}

type bindTarget struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Inner *struct {
		City string `json:"city"`
	} `json:"inner"`
}

func TestBindPath(t *testing.T) {
	cases := []struct {
		name      string
		fieldPath string
		value     string
		check     func(*testing.T, *bindTarget)
		wantErr   string
	}{
		{
			name:      "int field",
			fieldPath: "id",
			value:     "7",
			check: func(t *testing.T, b *bindTarget) {
				if b.ID != 7 {
					t.Errorf("ID = %d, want 7", b.ID)
				}
			},
		},
		{
			name:      "string field",
			fieldPath: "name",
			value:     "alice",
			check: func(t *testing.T, b *bindTarget) {
				if b.Name != "alice" {
					t.Errorf("Name = %q", b.Name)
				}
			},
		},
		{
			name:      "nested through nil pointer",
			fieldPath: "inner.city",
			value:     "berlin",
			check: func(t *testing.T, b *bindTarget) {
				if b.Inner == nil {
					t.Fatal("Inner should be allocated automatically")
				}
				if b.Inner.City != "berlin" {
					t.Errorf("City = %q, want berlin", b.Inner.City)
				}
			},
		},
		{
			name:      "unknown field",
			fieldPath: "nope",
			value:     "x",
			wantErr:   `field "nope" not found`,
		},
		{
			name:      "bad int",
			fieldPath: "id",
			value:     "not-an-int",
			wantErr:   "cannot parse",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := &bindTarget{}
			err := BindPath(req, tc.fieldPath, tc.value)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("BindPath error = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("BindPath: %v", err)
			}
			tc.check(t, req)
		})
	}
}

func TestBindAllPaths(t *testing.T) {
	orig := pathParamFunc
	defer func() { pathParamFunc = orig }()

	SetPathParamFunc(func(r *http.Request, name string) string {
		switch name {
		case "id":
			return "9"
		case "name":
			return "bob"
		default:
			return "" // unmatched vars are skipped
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	got := &bindTarget{}
	if err := BindAllPaths(got, req, []string{"id", "name", "missing"}); err != nil {
		t.Fatalf("BindAllPaths: %v", err)
	}
	if got.ID != 9 {
		t.Errorf("ID = %d, want 9", got.ID)
	}
	if got.Name != "bob" {
		t.Errorf("Name = %q, want bob", got.Name)
	}

	// An empty path-param value is skipped without touching the struct.
	if err := BindAllPaths(&bindTarget{}, req, []string{"unknown"}); err != nil {
		t.Errorf("empty path value should be skipped, got %v", err)
	}

	// A non-empty value whose field does not exist propagates the error.
	SetPathParamFunc(func(r *http.Request, name string) string { return "x" })
	defer func() { pathParamFunc = orig }()
	if err := BindAllPaths(&bindTarget{}, req, []string{"unknown"}); err == nil {
		t.Error("expected error for unknown path var field")
	}
}

// ---------------------------------------------------------------------------
// Body binding
// ---------------------------------------------------------------------------

func TestBindBody(t *testing.T) {
	// Nil body is a no-op.
	if err := BindBody(&http.Request{}, &struct{}{}); err != nil {
		t.Errorf("BindBody with nil body = %v, want nil", err)
	}

	// Empty body is a no-op.
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	var out bindTarget
	if err := BindBody(req, &out); err != nil {
		t.Errorf("BindBody with empty body: %v", err)
	}

	// Valid JSON unmarshals into the target.
	req2 := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"id":3,"name":"carol"}`))
	if err := BindBody(req2, &out); err != nil {
		t.Fatalf("BindBody: %v", err)
	}
	if out.ID != 3 || out.Name != "carol" {
		t.Errorf("bound = %+v", out)
	}

	// Invalid JSON errors.
	req3 := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{not-json"))
	if err := BindBody(req3, &bindTarget{}); err == nil {
		t.Error("expected error for invalid JSON body")
	}
}

func TestBindBodyField(t *testing.T) {
	type wrapper struct {
		User struct {
			Name string `json:"name"`
		} `json:"user"`
	}

	// Nil body is a no-op.
	if err := BindBodyField(&http.Request{}, &wrapper{}, "user.name"); err != nil {
		t.Errorf("nil body: %v", err)
	}

	// The body IS the value of the addressed field: a struct leaf unmarshals
	// from an object body.
	req := httptest.NewRequest(http.MethodPost, "/",
		strings.NewReader(`{"name":"dave"}`))
	got := &wrapper{}
	if err := BindBodyField(req, got, "user"); err != nil {
		t.Fatalf("BindBodyField: %v", err)
	}
	if got.User.Name != "dave" {
		t.Errorf("User.Name = %q, want dave", got.User.Name)
	}

	// A scalar leaf unmarshals from the JSON-encoded scalar.
	req2 := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`"erin"`))
	got2 := &wrapper{}
	if err := BindBodyField(req2, got2, "user.name"); err != nil {
		t.Fatalf("BindBodyField scalar: %v", err)
	}
	if got2.User.Name != "erin" {
		t.Errorf("User.Name = %q, want erin", got2.User.Name)
	}

	// Empty body leaves the field untouched.
	req3 := httptest.NewRequest(http.MethodPost, "/", nil)
	got3 := &wrapper{}
	if err := BindBodyField(req3, got3, "user.name"); err != nil {
		t.Errorf("empty body: %v", err)
	}
	if got3.User.Name != "" {
		t.Errorf("empty body should not set anything, got %q", got3.User.Name)
	}

	// Unknown field path errors.
	req4 := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
	if err := BindBodyField(req4, &wrapper{}, "no.such.field"); err == nil {
		t.Error("expected error for unknown field path")
	}

	// Invalid JSON errors.
	req5 := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("%%%"))
	if err := BindBodyField(req5, &wrapper{}, "user.name"); err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// ---------------------------------------------------------------------------
// WriteResponse / content type
// ---------------------------------------------------------------------------

func TestWriteResponse(t *testing.T) {
	// Nil value: bare 200, no body.
	rec := httptest.NewRecorder()
	WriteResponse(rec, httptest.NewRequest(http.MethodGet, "/", nil), nil)
	if rec.Code != http.StatusOK {
		t.Errorf("nil value status = %d, want 200", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("nil value body = %q, want empty", rec.Body.String())
	}

	// Value: JSON body with the configured content type.
	rec2 := httptest.NewRecorder()
	WriteResponse(rec2, httptest.NewRequest(http.MethodGet, "/", nil), map[string]string{"ok": "yes"})
	if rec2.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec2.Code)
	}
	if ct := rec2.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
	if got := rec2.Body.String(); !strings.Contains(got, `"ok":"yes"`) {
		t.Errorf("body = %q", got)
	}

	// Unmarshalable value: falls through to WriteError (500).
	rec3 := httptest.NewRecorder()
	WriteResponse(rec3, httptest.NewRequest(http.MethodGet, "/", nil), make(chan int))
	if rec3.Code != http.StatusInternalServerError {
		t.Errorf("marshal-failure status = %d, want 500", rec3.Code)
	}
}

func TestSetContentType(t *testing.T) {
	orig := contentType
	defer func() { contentType = orig }()

	SetContentType("text/plain")
	if contentType != "text/plain" {
		t.Errorf("contentType = %q, want text/plain", contentType)
	}

	// Empty values are ignored.
	SetContentType("")
	if contentType != "text/plain" {
		t.Error("SetContentType(\"\") should be ignored")
	}

	// WriteResponse picks the new type up.
	rec := httptest.NewRecorder()
	WriteResponse(rec, httptest.NewRequest(http.MethodGet, "/", nil), map[string]int{"n": 1})
	if ct := rec.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/plain; charset=utf-8", ct)
	}
}

// ---------------------------------------------------------------------------
// Error mapping registry
// ---------------------------------------------------------------------------

func TestRegisterErrorAndMapError(t *testing.T) {
	custom := fmt.Errorf("teapot")
	RegisterError(custom, http.StatusTeapot)

	if got := MapError(custom); got != http.StatusTeapot {
		t.Errorf("MapError(custom) = %d, want 418", got)
	}

	// Wrapped occurrences of the registered error are found via errors.Is.
	wrapped := fmt.Errorf("handler: %w", custom)
	if got := MapError(wrapped); got != http.StatusTeapot {
		t.Errorf("MapError(wrapped) = %d, want 418", got)
	}
}

// ---------------------------------------------------------------------------
// BindQuery edge cases not covered elsewhere
// ---------------------------------------------------------------------------

func TestBindQuery_PtrToSliceField(t *testing.T) {
	type target struct {
		Tags *[]string `json:"tags"`
		Nums *[]int    `json:"nums"`
	}
	got := &target{}

	// BindQuery dereferences pointer fields before kind dispatch, so a *[]T
	// field routes to setSliceField (and the nil pointer is allocated).
	q := url.Values{"tags": []string{"a", "b"}}
	if err := BindQuery(got, q); err != nil {
		t.Fatalf("BindQuery(*[]string): %v", err)
	}
	if got.Tags == nil {
		t.Fatal("Tags pointer should be allocated automatically")
	}
	if len(*got.Tags) != 2 || (*got.Tags)[0] != "a" || (*got.Tags)[1] != "b" {
		t.Errorf("Tags = %v, want [a b]", *got.Tags)
	}

	// Pointer to a slice of a non-string scalar type.
	q2 := url.Values{"nums": []string{"1", "2", "3"}}
	if err := BindQuery(got, q2); err != nil {
		t.Fatalf("BindQuery(*[]int): %v", err)
	}
	if got.Nums == nil || len(*got.Nums) != 3 || (*got.Nums)[2] != 3 {
		t.Errorf("Nums = %v, want [1 2 3]", *got.Nums)
	}

	// A single value still wraps into a one-element slice.
	got2 := &target{}
	q3 := url.Values{"tags": []string{"only"}}
	if err := BindQuery(got2, q3); err != nil {
		t.Fatalf("BindQuery single value: %v", err)
	}
	if got2.Tags == nil || len(*got2.Tags) != 1 || (*got2.Tags)[0] != "only" {
		t.Errorf("Tags = %v, want [only]", *got2.Tags)
	}

	// A bad element fails with the index in the message.
	q4 := url.Values{"nums": []string{"1", "x"}}
	if err := BindQuery(&target{}, q4); err == nil || !strings.Contains(err.Error(), "slice element 1") {
		t.Errorf("BindQuery error = %v, want slice element 1 failure", err)
	}
}

func TestBindQuery_PtrScalarField(t *testing.T) {
	type target struct {
		ID   *int    `json:"id"`
		Name *string `json:"name"`
	}
	got := &target{}

	q := url.Values{"id": []string{"42"}, "name": []string{"alice"}}
	if err := BindQuery(got, q); err != nil {
		t.Fatalf("BindQuery: %v", err)
	}
	if got.ID == nil || *got.ID != 42 {
		t.Errorf("ID = %v, want 42", got.ID)
	}
	if got.Name == nil || *got.Name != "alice" {
		t.Errorf("Name = %v, want alice", got.Name)
	}

	// A bad scalar still surfaces the parse error.
	q2 := url.Values{"id": []string{"not-an-int"}}
	if err := BindQuery(&target{}, q2); err == nil || !strings.Contains(err.Error(), "cannot parse") {
		t.Errorf("BindQuery error = %v, want cannot parse", err)
	}
}

func TestBindQuery_NonStructTargetRejected(t *testing.T) {
	// A pointer to a non-struct must return an error instead of panicking in
	// lookupFieldMeta (reflect NumField on a non-struct type).
	n := 42
	err := BindQuery(&n, url.Values{"x": []string{"1"}})
	if err == nil || !strings.Contains(err.Error(), "requires a pointer to struct") {
		t.Errorf("BindQuery(*int) error = %v, want 'requires a pointer to struct'", err)
	}

	s := "str"
	if err := BindQuery(&s, url.Values{"x": []string{"1"}}); err == nil {
		t.Error("BindQuery(*string) should error")
	}

	// A map target is likewise rejected.
	m := map[string]string{}
	if err := BindQuery(&m, url.Values{"x": []string{"1"}}); err == nil || !strings.Contains(err.Error(), "requires a pointer to struct") {
		t.Errorf("BindQuery(*map) error = %v, want 'requires a pointer to struct'", err)
	}
}

func TestBindQuery_IntSlice(t *testing.T) {
	type target struct {
		Nums []int `json:"nums"`
	}

	q := url.Values{"nums": []string{"1", "2", "3"}}
	got := &target{}
	if err := BindQuery(got, q); err != nil {
		t.Fatalf("BindQuery: %v", err)
	}
	if len(got.Nums) != 3 || got.Nums[0] != 1 || got.Nums[2] != 3 {
		t.Errorf("Nums = %v, want [1 2 3]", got.Nums)
	}

	// A bad element fails with the index in the message.
	q2 := url.Values{"nums": []string{"1", "x"}}
	if err := BindQuery(&target{}, q2); err == nil || !strings.Contains(err.Error(), "slice element 1") {
		t.Errorf("BindQuery error = %v, want slice element 1 failure", err)
	}
}

func TestBindQuery_MultiValuesOnScalarField(t *testing.T) {
	type target struct {
		Name string `json:"name"`
	}
	q := url.Values{"name": []string{"first", "second"}}

	got := &target{}
	if err := BindQuery(got, q); err != nil {
		t.Fatalf("BindQuery: %v", err)
	}
	if got.Name != "first" {
		t.Errorf("Name = %q, want first (first value wins on scalar fields)", got.Name)
	}
}

func TestBindQuery_UnsupportedFieldKind(t *testing.T) {
	type target struct {
		Meta map[string]string `json:"meta"`
	}
	q := url.Values{"meta": []string{"x"}}

	err := BindQuery(&target{}, q)
	if err == nil || !strings.Contains(err.Error(), "unsupported field kind") {
		t.Errorf("BindQuery error = %v, want unsupported field kind", err)
	}
}

func TestBindQuery_UnexportedFieldNotSettable(t *testing.T) {
	type target struct {
		hidden string // intentionally unexported; found via the lowercase fallback
	}
	q := url.Values{"hidden": []string{"x"}}

	err := BindQuery(&target{}, q)
	if err == nil || !strings.Contains(err.Error(), "not settable") {
		t.Errorf("BindQuery error = %v, want 'not settable'", err)
	}
}

// ---------------------------------------------------------------------------
// Field lookup internals
// ---------------------------------------------------------------------------

func TestFindField_InvalidValue(t *testing.T) {
	_, err := findField(reflect.Value{}, "x")
	if err == nil || !strings.Contains(err.Error(), "invalid value") {
		t.Errorf("findField error = %v, want invalid value", err)
	}
}

func TestFindField_NotFound(t *testing.T) {
	v := reflect.ValueOf(&struct{ A int }{}).Elem()
	if _, err := findField(v, "zzz"); err == nil {
		t.Error("expected error for unknown field")
	}
}

func TestToCamelCase(t *testing.T) {
	cases := map[string]string{
		"":              "",
		"_":             "",
		"__a__":         "A",
		"user_name":     "UserName",
		"ID":            "ID",
		"single":        "Single",
		"a_b_c":         "ABC",
		"trailing_":     "Trailing",
		"_leading":      "Leading",
		"with_response": "WithResponse",
	}
	for in, want := range cases {
		if got := toCamelCase(in); got != want {
			t.Errorf("toCamelCase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildFieldMeta_SkipsIgnoredAndDedupesLower(t *testing.T) {
	type target struct {
		A int `json:"-"`
		B int `json:",omitempty"`
		C int
	}
	meta := lookupFieldMeta(reflect.TypeOf(target{}))

	// "-" tags must not register a JSON name.
	if _, ok := meta.byJSON["-"]; ok {
		t.Error(`json tag "-" must be skipped`)
	}
	// A nameless tag (options only) registers nothing: the field falls back
	// to the lowercase table.
	if _, ok := meta.byJSON["B"]; ok {
		t.Error(`json tag ",omitempty" has an empty name and must not register`)
	}
	// Lowercase lookup covers field names.
	if idx, ok := meta.byLower["b"]; !ok || idx != 1 {
		t.Errorf("byLower[b] = %d,%v, want index 1", idx, ok)
	}
	if idx, ok := meta.byLower["c"]; !ok || idx != 2 {
		t.Errorf("byLower[c] = %d,%v, want index 2", idx, ok)
	}
}
