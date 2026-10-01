package mixin

import (
	"strings"
	"testing"

	"entgo.io/ent"
)

// fieldNames extracts the declared names of schema fields.
func fieldNames(t *testing.T, fields []ent.Field) []string {
	t.Helper()
	names := make([]string, 0, len(fields))
	for _, f := range fields {
		names = append(names, f.Descriptor().Name)
	}
	return names
}

// TestMixinFields walks every mixin's Fields() and asserts the produced
// column names. These are pure schema declarations with no I/O.
func TestMixinFields(t *testing.T) {
	cases := []struct {
		name  string
		mixin ent.Mixin
		want  []string
	}{
		{"Archived", Archived{}, []string{"archived", "archived_at"}},
		{"AuditStatus", AuditStatus{}, nil}, // names asserted below
		{"CreateBy", CreateBy{}, []string{"create_by"}},
		{"CreateBy64", CreateBy64{}, []string{"create_by"}},
		{"CreatedBy", CreatedBy{}, []string{"created_by"}},
		{"CreatedBy64", CreatedBy64{}, []string{"created_by"}},
		{"CreatorId", CreatorId{}, []string{"creator_id"}},
		{"CreatedAt", CreatedAt{}, []string{"created_at"}},
		{"CreateTime", CreateTime{}, []string{"create_time"}},
		{"CreateTimestamp", CreateTimestamp{}, []string{"create_time"}},
		{"CreatedAtTimestamp", CreatedAtTimestamp{}, []string{"created_at"}},
		{"UpdatedAt", UpdatedAt{}, []string{"updated_at"}},
		{"UpdateTime", UpdateTime{}, []string{"update_time"}},
		{"UpdateTimestamp", UpdateTimestamp{}, []string{"update_time"}},
		{"UpdatedAtTimestamp", UpdatedAtTimestamp{}, []string{"updated_at"}},
		{"DeletedAt", DeletedAt{}, []string{"deleted_at"}},
		{"TimeAt", TimeAt{}, []string{"created_at"}},
		{"DeletedAtTimestamp", DeletedAtTimestamp{}, []string{"deleted_at"}},
		{"TimestampAt", TimestampAt{}, []string{"created_at"}},
		{"Timestamp", Timestamp{}, nil},
		{"UpdateBy", UpdateBy{}, []string{"update_by"}},
		{"UpdateBy64", UpdateBy64{}, []string{"update_by"}},
		{"UpdatedBy", UpdatedBy{}, []string{"updated_by"}},
		{"UpdatedBy64", UpdatedBy64{}, []string{"updated_by"}},
		{"DeleteBy", DeleteBy{}, []string{"delete_by"}},
		{"DeleteBy64", DeleteBy64{}, []string{"delete_by"}},
		{"DeletedBy", DeletedBy{}, []string{"deleted_by"}},
		{"DeletedBy64", DeletedBy64{}, []string{"deleted_by"}},
		{"OperatorID", OperatorID{}, nil},
		{"OperatorID64", OperatorID64{}, nil},
		{"AuditorID", AuditorID{}, nil},
		{"AuditorID64", AuditorID64{}, nil},
		{"Description", Description{}, []string{"description"}},
		{"IpAddress", IpAddress{}, []string{"ip_address"}},
		{"IsEnabled", IsEnabled{}, []string{"is_enabled"}},
		{"Enabled", Enabled{}, []string{"enabled"}},
		{"Tag", Tag{}, []string{"tags"}},
		{"SortOrder", SortOrder{}, []string{"sort_order"}},
		{"Remark", Remark{}, []string{"remark"}},
		{"SnowflakeId", SnowflakeId{}, []string{"id"}},
		{"StringId", StringId{}, []string{"id"}},
		{"SoftDelete", SoftDelete{}, []string{"deleted_at"}},
		{"SoftDelete64", SoftDelete64{}, []string{"deleted_at"}},
		{"ParentID", ParentID{}, []string{"parent_id"}},
		{"Version", Version{}, []string{"version"}},
		{"TenantIDU32", TenantID[uint32]{}, []string{"tenant_id"}},
		{"TenantIDU64", TenantID[uint64]{}, []string{"tenant_id"}},
		{"TreePathIDs", TreePathIDs{}, nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fields := tc.mixin.Fields()
			names := fieldNames(t, fields)
			if len(names) == 0 {
				t.Log("mixin produces no direct fields (composed)")
				return
			}
			for _, want := range tc.want {
				found := false
				for _, n := range names {
					if n == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("mixin %s fields %v missing %q", tc.name, names, want)
				}
			}
		})
	}

	// composed mixins: verify all their member columns exist
	composed := []struct {
		name  string
		mixin ent.Mixin
		want  []string
	}{
		{"AuditStatus", AuditStatus{}, []string{"audit_status"}},
		{"Timestamp", Timestamp{}, []string{"create_time", "update_time"}},
		{"OperatorID", OperatorID{}, []string{"created_by", "updated_by"}},
		{"OperatorID64", OperatorID64{}, []string{"created_by", "updated_by"}},
		{"AuditorID", AuditorID{}, []string{"created_by", "updated_by", "deleted_by"}},
		{"AuditorID64", AuditorID64{}, []string{"created_by", "updated_by", "deleted_by"}},
		{"TreePathIDs", TreePathIDs{}, []string{"ancestor_ids"}},
	}
	for _, tc := range composed {
		t.Run(tc.name+"_composed", func(t *testing.T) {
			names := fieldNames(t, tc.mixin.Fields())
			if len(names) == 0 {
				t.Fatalf("mixin %s produced no fields", tc.name)
			}
			for _, want := range tc.want {
				found := false
				for _, n := range names {
					if strings.Contains(n, want) || n == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("mixin %s fields %v missing %q", tc.name, names, want)
				}
			}
		})
	}
}

// TestTreeMixinFieldsAndEdges covers the generic tree mixin declaration.
func TestTreeMixinFieldsAndEdges(t *testing.T) {
	tree := Tree[struct{ TableInterface }]{}
	fields := tree.Fields()
	if len(fields) == 0 {
		t.Fatal("Tree mixin must declare fields")
	}
	if got := fieldNames(t, fields); got[0] != "parent_id" {
		t.Fatalf("Tree fields = %v", got)
	}

	edges := tree.Edges()
	if len(edges) == 0 {
		t.Fatal("Tree mixin must declare edges")
	}
}

// TestTenantIDPolicy covers the privacy policy hook of the tenant mixin.
func TestTenantIDPolicy(t *testing.T) {
	var p ent.Policy = TenantID[uint32]{}.Policy()
	if p == nil {
		t.Fatal("TenantID must provide a privacy policy")
	}
}

// TestSoftDeleteInterceptors covers the interceptor wiring of soft delete.
func TestSoftDeleteInterceptors(t *testing.T) {
	interceptors := SoftDelete{}.Interceptors()
	if len(interceptors) == 0 {
		t.Fatal("SoftDelete must provide interceptors")
	}
	if interceptors[0] == nil {
		t.Fatal("interceptor must be non-nil")
	}
}

// ---------------------------------------------------------------------------
// Audit helpers not covered by audit_test.go
// ---------------------------------------------------------------------------

func TestBuildPostDataFromValue(t *testing.T) {
	// nil and primitives yield nil
	if buildPostDataFromValue(nil) != nil {
		t.Fatal("nil input must yield nil")
	}
	if buildPostDataFromValue(42) != nil {
		t.Fatal("primitive input must yield nil")
	}
	if buildPostDataFromValue("str") != nil {
		t.Fatal("string input must yield nil")
	}

	// structs are marshalled to maps and sensitive keys redacted
	type inner struct {
		Token string `json:"token"`
		Note  string `json:"note"`
	}
	type payload struct {
		ID        int    `json:"id"`
		SecretPUT string `json:"secretPut"`
		Inner     inner  `json:"inner"`
	}
	got := buildPostDataFromValue(payload{ID: 1, SecretPUT: "s", Inner: inner{Token: "t", Note: "n"}})
	if got == nil {
		t.Fatal("struct input must yield a map")
	}
	if got["id"] != float64(1) {
		t.Fatalf("id = %v", got["id"])
	}
	if got["secretPut"] != "********" {
		t.Fatalf("sensitive key not redacted: %v", got["secretPut"])
	}
	innerMap := got["inner"].(map[string]any)
	if innerMap["token"] != "********" || innerMap["note"] != "n" {
		t.Fatalf("nested redaction wrong: %v", innerMap)
	}

	// unmarshalable input yields nil
	if buildPostDataFromValue(make(chan int)) != nil {
		t.Fatal("unmarshalable input must yield nil")
	}
}

func TestExtractTargetID(t *testing.T) {
	if got := extractTargetID(nil); got != "" {
		t.Fatalf("extractTargetID(nil) = %q", got)
	}

	// struct field fallback
	type entity struct {
		ID   int
		Name string
	}
	if got := extractTargetID(entity{ID: 7, Name: "x"}); got != "7" {
		t.Fatalf("extractTargetID(struct) = %q, want 7", got)
	}
	if got := extractTargetID(&entity{ID: 9}); got != "9" {
		t.Fatalf("extractTargetID(ptr) = %q, want 9", got)
	}

	// arbitrary values fall back to fmt.Sprintf
	if got := extractTargetID("raw"); got != "raw" {
		t.Fatalf("extractTargetID(string) = %q", got)
	}
}
