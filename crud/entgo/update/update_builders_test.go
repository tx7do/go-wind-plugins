package update

import (
	"testing"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// ---------------------------------------------------------------------------
// A runtime-built message covering every scalar type the JSON-value extractor
// handles (the module ships no generated proto with numeric fields).
// ---------------------------------------------------------------------------

var scalarMessageType = func() protoreflect.MessageDescriptor {
	fdp := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("update_scalar_test.proto"),
		Package: proto.String("wind.plugins.update_test"),
		Syntax:  proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("Scalars"),
			Field: []*descriptorpb.FieldDescriptorProto{
				{Name: proto.String("int32_val"), Number: proto.Int32(1), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum()},
				{Name: proto.String("int64_val"), Number: proto.Int32(2), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum()},
				{Name: proto.String("uint32_val"), Number: proto.Int32(3), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_UINT32.Enum()},
				{Name: proto.String("uint64_val"), Number: proto.Int32(4), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_UINT64.Enum()},
				{Name: proto.String("float_val"), Number: proto.Int32(5), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_FLOAT.Enum()},
				{Name: proto.String("double_val"), Number: proto.Int32(6), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_DOUBLE.Enum()},
				{Name: proto.String("bool_val"), Number: proto.Int32(7), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_BOOL.Enum()},
				{Name: proto.String("string_val"), Number: proto.Int32(8), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum()},
				{Name: proto.String("empty_val"), Number: proto.Int32(9), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum()},
			},
		}},
	}
	fd, err := protodesc.NewFile(fdp, nil)
	if err != nil {
		panic(err)
	}
	return fd.Messages().Get(0)
}()

func newScalarMessage(t *testing.T) *dynamicpb.Message {
	t.Helper()
	m := dynamicpb.NewMessage(scalarMessageType)
	set := func(name string, v protoreflect.Value) {
		fd := m.Descriptor().Fields().ByName(protoreflect.Name(name))
		require.NotNil(t, fd)
		m.Set(fd, v)
	}
	set("int32_val", protoreflect.ValueOfInt32(-7))
	set("int64_val", protoreflect.ValueOfInt64(-9))
	set("uint32_val", protoreflect.ValueOfUint32(11))
	set("uint64_val", protoreflect.ValueOfUint64(13))
	set("float_val", protoreflect.ValueOfFloat32(1.5))
	set("double_val", protoreflect.ValueOfFloat64(2.25))
	set("bool_val", protoreflect.ValueOfBool(true))
	set("string_val", protoreflect.ValueOfString("it's"))
	return m
}

func TestBuildSetNullUpdater(t *testing.T) {
	require.Nil(t, BuildSetNullUpdater(nil))
	require.Nil(t, BuildSetNullUpdater([]string{}))

	s := sql.Dialect(dialect.Postgres).Update("users")
	updater := BuildSetNullUpdater([]string{"Name", `we"rd`})
	require.NotNil(t, updater)
	updater(s)
	query, args := s.Query()
	// ToSnakeCase normalizes before the whitelist check, so `we"rd` becomes
	// the valid identifier `we_rd` instead of being skipped
	require.Equal(t, `UPDATE "users" SET "name" = NULL, "we_rd" = NULL`, query)
	require.Empty(t, args)
}

func TestExtractJsonFieldKeyValues(t *testing.T) {
	msg := newScalarMessage(t)

	// paths are proto field names (snake_case)
	got := ExtractJsonFieldKeyValues(msg, []string{
		"int32_val", "int64_val", "uint32_val", "uint64_val",
		"float_val", "double_val", "bool_val", "string_val",
		"missing", "empty_val",
	}, true)

	// keys snake-cased, missing/unset fields skipped, string literals escaped
	require.Equal(t, []string{
		"'int32_val'", "-7",
		"'int64_val'", "-9",
		"'uint32_val'", "11",
		"'uint64_val'", "13",
		"'float_val'", "1.5",
		"'double_val'", "2.25",
		"'bool_val'", "true",
		"'string_val'", "'it''s'",
	}, got)

	// without snake-casing the original path is used as the key
	got = ExtractJsonFieldKeyValues(msg, []string{"bool_val"}, false)
	require.Equal(t, []string{"'bool_val'", "true"}, got)

	// nil (and typed-nil) messages extract nothing instead of panicking
	require.Nil(t, ExtractJsonFieldKeyValues(nil, []string{"string_val"}, true))
	require.Nil(t, ExtractJsonFieldKeyValues((*dynamicpb.Message)(nil), []string{"string_val"}, true))
}

func TestSetJsonFieldValueUpdateBuilder(t *testing.T) {
	msg := newScalarMessage(t)

	s := sql.Dialect(dialect.Postgres).Update("docs")
	mod := SetJsonFieldValueUpdateBuilder("meta", msg, []string{"string_val", "uint32_val"}, true)
	require.NotNil(t, mod)
	mod(s)
	query, args := s.Query()
	require.Contains(t, query, `"meta" || jsonb_build_object`)
	require.Contains(t, query, `'string_val'`)
	require.Contains(t, query, `'it''s'`)
	require.Contains(t, query, `'uint32_val'`)
	require.Empty(t, args)

	// no extractable values => no modifier
	require.Nil(t, SetJsonFieldValueUpdateBuilder("meta", msg, []string{"missing", "empty_val"}, true))
	// a nil proto message extracts nothing => no modifier (previously panicked)
	require.Nil(t, SetJsonFieldValueUpdateBuilder("meta", nil, []string{"string_val"}, true))
	require.Nil(t, SetJsonFieldValueUpdateBuilder("meta", (*dynamicpb.Message)(nil), []string{"string_val"}, true))
	// non-identifier column names are rejected outright
	require.Nil(t, SetJsonFieldValueUpdateBuilder(`me"ta`, msg, []string{"stringVal"}, true))
}

func TestSetJsonNullFieldUpdateBuilder(t *testing.T) {
	msg := dynamicpb.NewMessage(scalarMessageType) // everything unset

	s := sql.Dialect(dialect.Postgres).Update("docs")
	mod := SetJsonNullFieldUpdateBuilder("meta", msg, []string{"string_val", "uint32_val"})
	require.NotNil(t, mod)
	mod(s)
	query, args := s.Query()
	require.Contains(t, query, `"meta" - '{string_val,uint32_val}'::text[]`)
	require.Empty(t, args)

	// no nil paths => no modifier
	full := newScalarMessage(t)
	require.Nil(t, SetJsonNullFieldUpdateBuilder("meta", full, []string{"string_val", "uint32_val"}))
}

type maskUpdateBuilder struct {
	applied []func(*sql.UpdateBuilder)
}

func (b *maskUpdateBuilder) Modify(modifiers ...func(*sql.UpdateBuilder)) *maskUpdateBuilder {
	b.applied = append(b.applied, modifiers...)
	return b
}

func TestApplyNilFieldMask(t *testing.T) {
	msg := dynamicpb.NewMessage(scalarMessageType) // all fields nil

	// nil mask is a no-op
	b := &maskUpdateBuilder{}
	ApplyNilFieldMask(msg, nil, b)
	require.Empty(t, b.applied)

	b2 := &maskUpdateBuilder{}
	ApplyNilFieldMask(msg, &fieldmaskpb.FieldMask{Paths: []string{"string_val", "uint32_val"}}, b2)
	require.Len(t, b2.applied, 1)

	s := sql.Dialect(dialect.Postgres).Update("docs")
	b2.applied[0](s)
	query, _ := s.Query()
	require.Contains(t, query, `"string_val" = NULL`)
	require.Contains(t, query, `"uint32_val" = NULL`)
}
