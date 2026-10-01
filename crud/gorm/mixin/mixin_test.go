package mixin_test

import (
	"reflect"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm/schema"

	gormmixin "github.com/tx7do/go-wind-plugins/crud/gorm/mixin"
)

// ---------------------------------------------------------------------------
// Test models: one struct per mixin (or mixin group) to avoid field-name
// collisions between mixins that share column names (e.g. Status vs
// SwitchStatus both expose a `status` column).
// ---------------------------------------------------------------------------

type autoIncModel struct{ gormmixin.AutoIncrementID }
type snowflakeModel struct{ gormmixin.SnowflakeID }
type uuidModel struct{ gormmixin.UuidID }
type stringIDModel struct{ gormmixin.StringID }
type tenantModel struct{ gormmixin.TenantID }
type creatorModel struct{ gormmixin.CreatorID }
type createByModel struct{ gormmixin.CreateBy }
type updateByModel struct{ gormmixin.UpdateBy }
type deleteByModel struct{ gormmixin.DeleteBy }
type operatorModel struct{ gormmixin.OperatorID }
type timeAtModel struct{ gormmixin.TimeAt }
type timeModel struct{ gormmixin.Time }
type timestampModel struct{ gormmixin.Timestamp }
type timestampAtModel struct{ gormmixin.TimestampAt }
type descriptionModel struct{ gormmixin.Description }
type isEnabledModel struct{ gormmixin.IsEnabled }
type metadataModel struct{ gormmixin.Metadata }
type remarkModel struct{ gormmixin.Remark }
type sortOrderModel struct{ gormmixin.SortOrder }
type statusModel struct{ gormmixin.Status }
type switchStatusModel struct{ gormmixin.SwitchStatus }
type tagModel struct{ gormmixin.Tag }
type parentModel struct{ gormmixin.ParentID }
type versionModel struct{ gormmixin.Version }

// parseModel parses a model struct into a gorm schema.
func parseModel(t *testing.T, model any) *schema.Schema {
	t.Helper()
	s, err := schema.Parse(model, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("schema.Parse(%T): %v", model, err)
	}
	return s
}

// fieldSetter is the fluent helper capturing expectations per column.
type fieldCheck func(t *testing.T, f *schema.Field)

func expectTagSetting(key, value string) fieldCheck {
	return func(t *testing.T, f *schema.Field) {
		t.Helper()
		if got, ok := f.TagSettings[key]; !ok || got != value {
			t.Errorf("field %s TagSettings[%q] = %q (present=%v), want %q", f.DBName, key, got, ok, value)
		}
	}
}

func expectDefaultValue(v string) fieldCheck {
	return func(t *testing.T, f *schema.Field) {
		t.Helper()
		if !f.HasDefaultValue {
			t.Errorf("field %s must have a default value", f.DBName)
			return
		}
		if f.DefaultValue != v {
			t.Errorf("field %s DefaultValue = %q, want %q", f.DBName, f.DefaultValue, v)
		}
	}
}

func expectNotNull(t *testing.T, f *schema.Field) {
	if !f.NotNull {
		t.Errorf("field %s must be NOT NULL", f.DBName)
	}
}

func expectNullable(t *testing.T, f *schema.Field) {
	if f.NotNull {
		t.Errorf("pointer field %s must be nullable (NotNull=false)", f.DBName)
	}
}

func expectPrimaryKey(t *testing.T, f *schema.Field) {
	if !f.PrimaryKey {
		t.Errorf("field %s must be the primary key", f.DBName)
	}
}

func expectCreateOnly(t *testing.T, f *schema.Field) {
	if !f.Creatable {
		t.Errorf("field %s must be creatable (<-:create)", f.DBName)
	}
	if f.Updatable {
		t.Errorf("field %s must not be updatable (<-:create)", f.DBName)
	}
	if !f.Readable {
		t.Errorf("field %s must be readable", f.DBName)
	}
}

func expectUpdatable(t *testing.T, f *schema.Field) {
	if !f.Creatable || !f.Updatable {
		t.Errorf("field %s must be creatable and updatable, got create=%v update=%v", f.DBName, f.Creatable, f.Updatable)
	}
}

// TestMixinFieldRegistration verifies every mixin registers its column with
// the expected gorm tag settings when parsed into a schema.
func TestMixinFieldRegistration(t *testing.T) {
	tests := []struct {
		name   string
		model  any
		column string
		checks []fieldCheck
	}{
		{
			name:   "AutoIncrementID",
			model:  &autoIncModel{},
			column: "id",
			checks: []fieldCheck{
				expectPrimaryKey,
				expectTagSetting("AUTOINCREMENT", "AUTOINCREMENT"),
			},
		},
		{
			name:   "SnowflakeID",
			model:  &snowflakeModel{},
			column: "id",
			checks: []fieldCheck{
				expectPrimaryKey,
				expectTagSetting("TYPE", "bigint"),
			},
		},
		{
			name:   "UuidID",
			model:  &uuidModel{},
			column: "id",
			checks: []fieldCheck{
				expectPrimaryKey,
				expectTagSetting("TYPE", "char(36)"),
			},
		},
		{
			name:   "StringID",
			model:  &stringIDModel{},
			column: "id",
			checks: []fieldCheck{
				expectPrimaryKey,
				expectTagSetting("TYPE", "varchar(25)"),
			},
		},
		{
			name:   "TenantID",
			model:  &tenantModel{},
			column: "tenant_id",
			checks: []fieldCheck{
				expectNullable,
				expectTagSetting("TYPE", "int unsigned"),
				expectTagSetting("INDEX", "INDEX"),
			},
		},
		{
			name:   "CreatorID",
			model:  &creatorModel{},
			column: "creator_id",
			checks: []fieldCheck{
				expectNullable,
				expectCreateOnly,
				expectTagSetting("INDEX", "INDEX"),
			},
		},
		{
			name:   "CreateBy",
			model:  &createByModel{},
			column: "create_by",
			checks: []fieldCheck{expectNullable, expectCreateOnly},
		},
		{
			name:   "UpdateBy",
			model:  &updateByModel{},
			column: "update_by",
			checks: []fieldCheck{expectNullable, expectUpdatable},
		},
		{
			name:   "DeleteBy",
			model:  &deleteByModel{},
			column: "delete_by",
			checks: []fieldCheck{expectNullable, expectUpdatable},
		},
		{
			name:   "OperatorID.CreatedBy",
			model:  &operatorModel{},
			column: "created_by",
			checks: []fieldCheck{expectNullable, expectCreateOnly},
		},
		{
			name:   "OperatorID.UpdatedBy",
			model:  &operatorModel{},
			column: "updated_by",
			checks: []fieldCheck{expectNullable, expectUpdatable},
		},
		{
			name:   "OperatorID.DeletedBy",
			model:  &operatorModel{},
			column: "deleted_by",
			checks: []fieldCheck{expectNullable, expectUpdatable},
		},
		{
			name:   "TimeAt.CreatedAt",
			model:  &timeAtModel{},
			column: "created_at",
			checks: []fieldCheck{expectNullable},
		},
		{
			name:   "TimeAt.UpdatedAt",
			model:  &timeAtModel{},
			column: "updated_at",
			checks: []fieldCheck{expectNullable},
		},
		{
			name:   "TimeAt.DeletedAt",
			model:  &timeAtModel{},
			column: "deleted_at",
			checks: []fieldCheck{expectNullable, expectTagSetting("INDEX", "INDEX")},
		},
		{
			name:   "Time.CreateTime",
			model:  &timeModel{},
			column: "create_time",
			checks: []fieldCheck{expectNullable},
		},
		{
			name:   "Time.UpdateTime",
			model:  &timeModel{},
			column: "update_time",
			checks: []fieldCheck{expectNullable},
		},
		{
			name:   "Time.DeleteTime",
			model:  &timeModel{},
			column: "delete_time",
			checks: []fieldCheck{expectNullable, expectTagSetting("INDEX", "INDEX")},
		},
		{
			name:   "Timestamp.CreateTime",
			model:  &timestampModel{},
			column: "create_time",
			checks: []fieldCheck{expectNullable, expectTagSetting("TYPE", "bigint")},
		},
		{
			name:   "Timestamp.UpdateTime",
			model:  &timestampModel{},
			column: "update_time",
			checks: []fieldCheck{expectNullable, expectTagSetting("TYPE", "bigint")},
		},
		{
			name:   "Timestamp.DeleteTime",
			model:  &timestampModel{},
			column: "delete_time",
			checks: []fieldCheck{expectNullable, expectTagSetting("TYPE", "bigint"), expectTagSetting("INDEX", "INDEX")},
		},
		{
			name:   "TimestampAt.CreatedAt",
			model:  &timestampAtModel{},
			column: "created_at",
			checks: []fieldCheck{expectNullable, expectTagSetting("TYPE", "bigint")},
		},
		{
			name:   "TimestampAt.UpdatedAt",
			model:  &timestampAtModel{},
			column: "updated_at",
			checks: []fieldCheck{expectNullable, expectTagSetting("TYPE", "bigint")},
		},
		{
			name:   "TimestampAt.DeletedAt",
			model:  &timestampAtModel{},
			column: "deleted_at",
			checks: []fieldCheck{expectNullable, expectTagSetting("TYPE", "bigint"), expectTagSetting("INDEX", "INDEX")},
		},
		{
			name:   "Description",
			model:  &descriptionModel{},
			column: "description",
			checks: []fieldCheck{expectNullable, expectTagSetting("TYPE", "text")},
		},
		{
			name:   "IsEnabled",
			model:  &isEnabledModel{},
			column: "is_enabled",
			checks: []fieldCheck{
				expectNullable,
				expectDefaultValue("true"),
				expectTagSetting("TYPE", "boolean"),
				expectTagSetting("INDEX", "INDEX"),
			},
		},
		{
			name:   "Metadata",
			model:  &metadataModel{},
			column: "metadata",
			checks: []fieldCheck{expectTagSetting("TYPE", "json")},
		},
		{
			name:   "Remark",
			model:  &remarkModel{},
			column: "remark",
			checks: []fieldCheck{expectNullable, expectTagSetting("TYPE", "text")},
		},
		{
			name:   "SortOrder",
			model:  &sortOrderModel{},
			column: "sort_order",
			checks: []fieldCheck{
				expectNullable,
				expectDefaultValue("0"),
				expectTagSetting("INDEX", "INDEX"),
			},
		},
		{
			name:   "Status",
			model:  &statusModel{},
			column: "status",
			checks: []fieldCheck{
				expectNotNull,
				expectDefaultValue("1"),
				expectTagSetting("TYPE", "tinyint unsigned"),
				expectTagSetting("INDEX", "INDEX"),
			},
		},
		{
			name:   "SwitchStatus",
			model:  &switchStatusModel{},
			column: "status",
			checks: []fieldCheck{
				expectNullable,
				expectDefaultValue("ON"),
				expectTagSetting("TYPE", "varchar(10)"),
				expectTagSetting("INDEX", "INDEX"),
			},
		},
		{
			name:   "Tag",
			model:  &tagModel{},
			column: "tags",
			checks: []fieldCheck{expectTagSetting("TYPE", "json")},
		},
		{
			name:   "ParentID",
			model:  &parentModel{},
			column: "parent_id",
			checks: []fieldCheck{
				expectNullable,
				expectTagSetting("TYPE", "int unsigned"),
				expectTagSetting("INDEX", "INDEX"),
			},
		},
		{
			name:   "Version",
			model:  &versionModel{},
			column: "version",
			checks: []fieldCheck{
				expectNotNull,
				expectDefaultValue("1"),
				expectTagSetting("TYPE", "int unsigned"),
				expectTagSetting("INDEX", "INDEX"),
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			s := parseModel(t, tc.model)
			f, ok := s.FieldsByDBName[tc.column]
			if !ok {
				t.Fatalf("column %q not registered for %T (fields: %v)", tc.column, tc.model, s.FieldsByDBName)
			}
			for _, check := range tc.checks {
				check(t, f)
			}
		})
	}
}

// TestTagMixinUsesJSONSerializer verifies the tags column is registered with
// gorm's json serializer (values are stored as a JSON array) and that the
// Go field type is the *[]string pointer slice.
func TestTagMixinUsesJSONSerializer(t *testing.T) {
	s := parseModel(t, &tagModel{})
	f, ok := s.FieldsByDBName["tags"]
	if !ok {
		t.Fatal("tags column not registered")
	}
	if f.Serializer == nil {
		t.Error("tags field must register a serializer (serializer:json)")
	}
	if f.FieldType.Kind() != reflect.Ptr || f.FieldType.Elem().Kind() != reflect.Slice {
		t.Errorf("tags FieldType = %v, want *[]string", f.FieldType)
	}
}

// ---------------------------------------------------------------------------
// Hook behavior (pure: hooks ignore the *gorm.DB argument)
// ---------------------------------------------------------------------------

func TestCreatedAtBeforeCreate(t *testing.T) {
	m := &gormmixin.CreatedAt{}
	if err := m.BeforeCreate(nil); err != nil {
		t.Fatalf("BeforeCreate: %v", err)
	}
	if m.CreatedAt == nil {
		t.Fatal("BeforeCreate must populate CreatedAt when nil")
	}
	if m.CreatedAt.Before(time.Now().Add(-time.Minute)) {
		t.Errorf("CreatedAt = %v, want approximately now", m.CreatedAt)
	}

	existing := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	m2 := &gormmixin.CreatedAt{CreatedAt: &existing}
	if err := m2.BeforeCreate(nil); err != nil {
		t.Fatalf("BeforeCreate: %v", err)
	}
	if !m2.CreatedAt.Equal(existing) {
		t.Errorf("BeforeCreate must preserve an explicitly set CreatedAt, got %v", m2.CreatedAt)
	}
}

func TestUpdatedAtHooks(t *testing.T) {
	m := &gormmixin.UpdatedAt{}
	if err := m.BeforeCreate(nil); err != nil {
		t.Fatalf("BeforeCreate: %v", err)
	}
	if m.UpdatedAt == nil {
		t.Fatal("BeforeCreate must populate UpdatedAt when nil")
	}

	// BeforeSave always refreshes UpdatedAt, even when already set.
	existing := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	m2 := &gormmixin.UpdatedAt{UpdatedAt: &existing}
	if err := m2.BeforeSave(nil); err != nil {
		t.Fatalf("BeforeSave: %v", err)
	}
	if m2.UpdatedAt.Equal(existing) {
		t.Error("BeforeSave must refresh UpdatedAt")
	}
}

func TestDeletedAtBeforeDelete(t *testing.T) {
	m := &gormmixin.DeletedAt{}
	if err := m.BeforeDelete(nil); err != nil {
		t.Fatalf("BeforeDelete: %v", err)
	}
	if m.DeletedAt == nil {
		t.Fatal("BeforeDelete must populate DeletedAt when nil")
	}

	existing := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	m2 := &gormmixin.DeletedAt{DeletedAt: &existing}
	if err := m2.BeforeDelete(nil); err != nil {
		t.Fatalf("BeforeDelete: %v", err)
	}
	if !m2.DeletedAt.Equal(existing) {
		t.Errorf("BeforeDelete must preserve an existing DeletedAt, got %v", m2.DeletedAt)
	}
}

func TestTimeMixinsHooks(t *testing.T) {
	t.Run("CreateTime", func(t *testing.T) {
		m := &gormmixin.CreateTime{}
		if err := m.BeforeCreate(nil); err != nil {
			t.Fatalf("BeforeCreate: %v", err)
		}
		if m.CreateTime == nil {
			t.Error("BeforeCreate must populate CreateTime when nil")
		}
	})

	t.Run("UpdateTime", func(t *testing.T) {
		m := &gormmixin.UpdateTime{}
		if err := m.BeforeCreate(nil); err != nil {
			t.Fatalf("BeforeCreate: %v", err)
		}
		if m.UpdateTime == nil {
			t.Error("BeforeCreate must populate UpdateTime when nil")
		}
		// BeforeSave always refreshes.
		old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		m2 := &gormmixin.UpdateTime{UpdateTime: &old}
		if err := m2.BeforeSave(nil); err != nil {
			t.Fatalf("BeforeSave: %v", err)
		}
		if m2.UpdateTime.Equal(old) {
			t.Error("BeforeSave must refresh UpdateTime")
		}
	})

	t.Run("DeleteTime", func(t *testing.T) {
		m := &gormmixin.DeleteTime{}
		if err := m.BeforeDelete(nil); err != nil {
			t.Fatalf("BeforeDelete: %v", err)
		}
		if m.DeleteTime == nil {
			t.Error("BeforeDelete must populate DeleteTime when nil")
		}
	})
}

func TestTimestampMixinsHooks(t *testing.T) {
	start := time.Now().UnixMilli() - 1000

	t.Run("CreateTimestamp", func(t *testing.T) {
		m := &gormmixin.CreateTimestamp{}
		if err := m.BeforeCreate(nil); err != nil {
			t.Fatalf("BeforeCreate: %v", err)
		}
		if m.CreateTime == nil || *m.CreateTime < start {
			t.Errorf("BeforeCreate must populate CreateTime with millis, got %v", m.CreateTime)
		}
	})

	t.Run("UpdateTimestamp", func(t *testing.T) {
		m := &gormmixin.UpdateTimestamp{}
		if err := m.BeforeCreate(nil); err != nil {
			t.Fatalf("BeforeCreate: %v", err)
		}
		if m.UpdateTime == nil || *m.UpdateTime < start {
			t.Errorf("BeforeCreate must populate UpdateTime with millis, got %v", m.UpdateTime)
		}
		// BeforeSave always refreshes.
		m2 := &gormmixin.UpdateTimestamp{UpdateTime: &start}
		if err := m2.BeforeSave(nil); err != nil {
			t.Fatalf("BeforeSave: %v", err)
		}
		if m2.UpdateTime == nil || *m2.UpdateTime <= start {
			t.Error("BeforeSave must refresh UpdateTime")
		}
	})

	t.Run("DeleteTimestamp", func(t *testing.T) {
		m := &gormmixin.DeleteTimestamp{}
		if err := m.BeforeDelete(nil); err != nil {
			t.Fatalf("BeforeDelete: %v", err)
		}
		if m.DeleteTime == nil || *m.DeleteTime < start {
			t.Errorf("BeforeDelete must populate DeleteTime with millis, got %v", m.DeleteTime)
		}
	})
}

func TestTimestampAtMixinsHooks(t *testing.T) {
	start := time.Now().UnixMilli() - 1000

	t.Run("CreatedAtTimestamp", func(t *testing.T) {
		m := &gormmixin.CreatedAtTimestamp{}
		if err := m.BeforeCreate(nil); err != nil {
			t.Fatalf("BeforeCreate: %v", err)
		}
		if m.CreatedAt == nil || *m.CreatedAt < start {
			t.Errorf("BeforeCreate must populate CreatedAt with millis, got %v", m.CreatedAt)
		}
	})

	t.Run("UpdatedAtTimestamp", func(t *testing.T) {
		m := &gormmixin.UpdatedAtTimestamp{}
		if err := m.BeforeCreate(nil); err != nil {
			t.Fatalf("BeforeCreate: %v", err)
		}
		if m.UpdatedAt == nil || *m.UpdatedAt < start {
			t.Errorf("BeforeCreate must populate UpdatedAt with millis, got %v", m.UpdatedAt)
		}
		m2 := &gormmixin.UpdatedAtTimestamp{UpdatedAt: &start}
		if err := m2.BeforeSave(nil); err != nil {
			t.Fatalf("BeforeSave: %v", err)
		}
		if m2.UpdatedAt == nil || *m2.UpdatedAt <= start {
			t.Error("BeforeSave must refresh UpdatedAt")
		}
	})

	t.Run("DeletedAtTimestamp", func(t *testing.T) {
		m := &gormmixin.DeletedAtTimestamp{}
		if err := m.BeforeDelete(nil); err != nil {
			t.Fatalf("BeforeDelete: %v", err)
		}
		if m.DeletedAt == nil || *m.DeletedAt < start {
			t.Errorf("BeforeDelete must populate DeletedAt with millis, got %v", m.DeletedAt)
		}
	})
}

func TestIsEnabledBeforeCreate(t *testing.T) {
	m := &gormmixin.IsEnabled{}
	if err := m.BeforeCreate(nil); err != nil {
		t.Fatalf("BeforeCreate: %v", err)
	}
	if m.IsEnabled == nil || !*m.IsEnabled {
		t.Error("BeforeCreate must default IsEnabled to true")
	}

	disabled := false
	m2 := &gormmixin.IsEnabled{IsEnabled: &disabled}
	if err := m2.BeforeCreate(nil); err != nil {
		t.Fatalf("BeforeCreate: %v", err)
	}
	if *m2.IsEnabled {
		t.Error("explicitly disabled value must be preserved (not overwritten with true)")
	}
}

func TestSortOrderBeforeCreate(t *testing.T) {
	m := &gormmixin.SortOrder{}
	if err := m.BeforeCreate(nil); err != nil {
		t.Fatalf("BeforeCreate: %v", err)
	}
	if m.SortOrder == nil || *m.SortOrder != 0 {
		t.Error("BeforeCreate must default SortOrder to 0")
	}

	existing := int32(5)
	m2 := &gormmixin.SortOrder{SortOrder: &existing}
	if err := m2.BeforeCreate(nil); err != nil {
		t.Fatalf("BeforeCreate: %v", err)
	}
	if *m2.SortOrder != 5 {
		t.Errorf("explicit SortOrder must be preserved, got %d", *m2.SortOrder)
	}
}

func TestStatusBeforeCreate(t *testing.T) {
	m := &gormmixin.Status{}
	if err := m.BeforeCreate(nil); err != nil {
		t.Fatalf("BeforeCreate: %v", err)
	}
	if m.Status != 1 {
		t.Errorf("BeforeCreate must default Status to 1, got %d", m.Status)
	}

	// NOTE: documents current behavior — an explicit zero cannot be stored
	// on create because the hook treats 0 as "unset" and rewrites it to 1.
	m2 := &gormmixin.Status{Status: 2}
	if err := m2.BeforeCreate(nil); err != nil {
		t.Fatalf("BeforeCreate: %v", err)
	}
	if m2.Status != 2 {
		t.Errorf("explicit non-zero Status must be preserved, got %d", m2.Status)
	}
}

func TestVersionBeforeCreate(t *testing.T) {
	m := &gormmixin.Version{}
	if err := m.BeforeCreate(nil); err != nil {
		t.Fatalf("BeforeCreate: %v", err)
	}
	if m.Version != 1 {
		t.Errorf("BeforeCreate must default Version to 1, got %d", m.Version)
	}

	m2 := &gormmixin.Version{Version: 7}
	if err := m2.BeforeCreate(nil); err != nil {
		t.Fatalf("BeforeCreate: %v", err)
	}
	if m2.Version != 7 {
		t.Errorf("explicit Version must be preserved, got %d", m2.Version)
	}
}

func TestSwitchStatusHooks(t *testing.T) {
	t.Run("BeforeCreate defaults to ON", func(t *testing.T) {
		m := &gormmixin.SwitchStatus{}
		if err := m.BeforeCreate(nil); err != nil {
			t.Fatalf("BeforeCreate: %v", err)
		}
		if m.Status == nil || *m.Status != gormmixin.SwitchStatusOn {
			t.Errorf("BeforeCreate must default Status to ON, got %v", m.Status)
		}
	})

	t.Run("BeforeCreate preserves OFF", func(t *testing.T) {
		off := gormmixin.SwitchStatusOff
		m := &gormmixin.SwitchStatus{Status: &off}
		if err := m.BeforeCreate(nil); err != nil {
			t.Fatalf("BeforeCreate: %v", err)
		}
		if *m.Status != gormmixin.SwitchStatusOff {
			t.Errorf("explicit OFF must be preserved, got %s", *m.Status)
		}
	})

	t.Run("BeforeCreate rejects invalid value", func(t *testing.T) {
		bad := "MAYBE"
		m := &gormmixin.SwitchStatus{Status: &bad}
		if err := m.BeforeCreate(nil); err == nil {
			t.Error("BeforeCreate must reject a value outside ON/OFF")
		}
	})

	t.Run("BeforeSave rejects invalid value", func(t *testing.T) {
		bad := "maybe"
		m := &gormmixin.SwitchStatus{Status: &bad}
		if err := m.BeforeSave(nil); err == nil {
			t.Error("BeforeSave must reject a value outside ON/OFF")
		}
	})

	t.Run("BeforeSave allows nil", func(t *testing.T) {
		m := &gormmixin.SwitchStatus{}
		if err := m.BeforeSave(nil); err != nil {
			t.Errorf("nil Status must stay valid (nullable), got %v", err)
		}
	})
}

func TestStringIDBeforeCreate(t *testing.T) {
	t.Run("generates id when empty", func(t *testing.T) {
		m := &gormmixin.StringID{}
		if err := m.BeforeCreate(nil); err != nil {
			t.Fatalf("BeforeCreate: %v", err)
		}
		if m.ID == "" {
			t.Fatal("BeforeCreate must generate an ID when empty")
		}
		if len(m.ID) > 25 {
			t.Errorf("generated ID %q must be at most 25 characters", m.ID)
		}
	})

	t.Run("preserves valid explicit id", func(t *testing.T) {
		m := &gormmixin.StringID{ID: "abc-123_XYZ"}
		if err := m.BeforeCreate(nil); err != nil {
			t.Fatalf("BeforeCreate: %v", err)
		}
		if m.ID != "abc-123_XYZ" {
			t.Errorf("valid explicit ID must be preserved, got %q", m.ID)
		}
	})

	t.Run("rejects id longer than 25", func(t *testing.T) {
		m := &gormmixin.StringID{ID: "abcdefghijklmnopqrstuvwxyz"} // 26 chars
		if err := m.BeforeCreate(nil); err == nil {
			t.Error("ID longer than 25 characters must be rejected")
		}
	})

	t.Run("rejects invalid characters", func(t *testing.T) {
		for _, bad := range []string{"has space", "slash/sep", "dot.name", "plus+id", "中文id"} {
			m := &gormmixin.StringID{ID: bad}
			if err := m.BeforeCreate(nil); err == nil {
				t.Errorf("ID %q must be rejected by the format check", bad)
			}
		}
	})
}

func TestSnowflakeIDBeforeCreate(t *testing.T) {
	m := &gormmixin.SnowflakeID{}
	if err := m.BeforeCreate(nil); err != nil {
		t.Fatalf("BeforeCreate: %v", err)
	}
	if m.ID == 0 {
		t.Error("BeforeCreate must generate a snowflake ID when 0")
	}

	m2 := &gormmixin.SnowflakeID{ID: 12345}
	if err := m2.BeforeCreate(nil); err != nil {
		t.Fatalf("BeforeCreate: %v", err)
	}
	if m2.ID != 12345 {
		t.Errorf("explicit ID must be preserved, got %d", m2.ID)
	}
}

func TestUuidIDHooks(t *testing.T) {
	t.Run("BeforeCreate fills nil uuid", func(t *testing.T) {
		m := &gormmixin.UuidID{}
		if err := m.BeforeCreate(nil); err != nil {
			t.Fatalf("BeforeCreate: %v", err)
		}
		if m.ID == [16]byte{} {
			t.Error("BeforeCreate must generate a UUID when the field is nil")
		}
	})

	t.Run("BeforeCreate preserves existing uuid", func(t *testing.T) {
		existing := [16]byte{1, 2, 3}
		m := &gormmixin.UuidID{ID: existing}
		if err := m.BeforeCreate(nil); err != nil {
			t.Fatalf("BeforeCreate: %v", err)
		}
		if m.ID != existing {
			t.Error("existing UUID must be preserved by BeforeCreate")
		}
	})

	t.Run("BeforeSave fills nil uuid", func(t *testing.T) {
		m := &gormmixin.UuidID{}
		if err := m.BeforeSave(nil); err != nil {
			t.Fatalf("BeforeSave: %v", err)
		}
		if m.ID == [16]byte{} {
			t.Error("BeforeSave must generate a UUID when the field is nil")
		}
	})
}

func TestTagHooks(t *testing.T) {
	t.Run("BeforeCreate defaults to empty slice", func(t *testing.T) {
		m := &gormmixin.Tag{}
		if err := m.BeforeCreate(nil); err != nil {
			t.Fatalf("BeforeCreate: %v", err)
		}
		if m.Tags == nil {
			t.Error("BeforeCreate must default Tags to an empty (non-nil) slice")
		}
	})

	t.Run("BeforeSave defaults to empty slice", func(t *testing.T) {
		m := &gormmixin.Tag{}
		if err := m.BeforeSave(nil); err != nil {
			t.Fatalf("BeforeSave: %v", err)
		}
		if m.Tags == nil {
			t.Error("BeforeSave must default Tags to an empty (non-nil) slice")
		}
	})

	t.Run("preserves existing tags", func(t *testing.T) {
		existing := []string{"a", "b"}
		m := &gormmixin.Tag{Tags: &existing}
		if err := m.BeforeSave(nil); err != nil {
			t.Fatalf("BeforeSave: %v", err)
		}
		if len(*m.Tags) != 2 {
			t.Errorf("existing tags must be preserved, got %v", *m.Tags)
		}
	})
}

func TestNoOpHooks(t *testing.T) {
	// Metadata.BeforeSave, ParentID.BeforeCreate/BeforeSave are explicit
	// no-ops and must return nil without panicking.
	meta := &gormmixin.Metadata{}
	if err := meta.BeforeSave(nil); err != nil {
		t.Errorf("Metadata.BeforeSave must be a no-op, got %v", err)
	}

	pid := &gormmixin.ParentID{}
	if err := pid.BeforeCreate(nil); err != nil {
		t.Errorf("ParentID.BeforeCreate must be a no-op, got %v", err)
	}
	if err := pid.BeforeSave(nil); err != nil {
		t.Errorf("ParentID.BeforeSave must be a no-op, got %v", err)
	}
}

func TestSoftDeleteEmbedsDeletedAt(t *testing.T) {
	// SoftDelete only re-exposes DeletedAt (deleted_at marker column); the
	// deleted_by column belongs to OperatorID instead.
	s := parseModel(t, &struct{ gormmixin.SoftDelete }{})
	if _, ok := s.FieldsByDBName["deleted_at"]; !ok {
		t.Error("SoftDelete must register the deleted_at column")
	}
	if _, ok := s.FieldsByDBName["deleted_by"]; ok {
		t.Error("SoftDelete must not register a deleted_by column")
	}
}

func TestAutoIncrementIDGormDBDataType(t *testing.T) {
	// GormDBDataType returns an empty string (dialect-agnostic default).
	m := gormmixin.AutoIncrementID{}
	if got := m.GormDBDataType(nil, nil); got != "" {
		t.Errorf("GormDBDataType = %q, want an empty string", got)
	}
}
