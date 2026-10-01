package gorm

import (
	"context"
	"testing"
	"time"

	"github.com/tx7do/go-utils/mapper"
	paginationV1 "github.com/tx7do/go-wind-plugins/crud/api/gen/go/pagination/v1"
	"github.com/tx7do/go-wind-plugins/crud/pagination"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"gorm.io/gorm"
)

// newCrudRepo builds a repository over the shared in-memory sqlite fixture
// entities (testUserEntity / CacheTestUser live in the other test files).
func newCrudRepo(t *testing.T) *Repository[CacheTestUser, testUserEntity] {
	t.Helper()
	return NewRepository[CacheTestUser, testUserEntity](mapper.NewCopierMapper[CacheTestUser, testUserEntity]())
}

// whereAgeOver returns a where selector filtering on age.
func whereAgeOver(age int) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB { return db.Where("age > ?", age) }
}

// Small helpers for paging request pointer fields.
func uint32Ptr(v uint32) *uint32 { return &v }
func uint64Ptr(v uint64) *uint64 { return &v }
func boolPtr(v bool) *bool       { return &v }
func strPtr(v string) *string    { return &v }

func TestRepository_Count_ErrorsAndSelectors(t *testing.T) {
	db := openTestDBForRepository(t)
	ctx := context.Background()
	seedUsers(t, db,
		testUserEntity{Name: "alice", Age: 20},
		testUserEntity{Name: "bob", Age: 30},
		testUserEntity{Name: "carol", Age: 40},
	)
	r := newCrudRepo(t)

	if _, err := r.Count(ctx, nil, nil); err == nil {
		t.Fatal("Count with nil db must fail")
	}

	cnt, err := r.Count(ctx, db, nil)
	if err != nil {
		t.Fatalf("Count error: %v", err)
	}
	if cnt != 3 {
		t.Fatalf("expected count 3, got %d", cnt)
	}

	cnt, err = r.Count(ctx, db, []func(*gorm.DB) *gorm.DB{
		whereAgeOver(25),
		nil, // nil selectors must be skipped
	})
	if err != nil {
		t.Fatalf("Count with selector error: %v", err)
	}
	if cnt != 2 {
		t.Fatalf("expected filtered count 2, got %d", cnt)
	}
}

func TestRepository_CountWithOptions(t *testing.T) {
	db := openTestDBForRepository(t)
	ctx := context.Background()
	seedUsers(t, db,
		testUserEntity{Name: "alice", Age: 20},
		testUserEntity{Name: "bob", Age: 30},
		testUserEntity{Name: "carol", Age: 40},
		testUserEntity{Name: "alice", Age: 50},
	)
	r := newCrudRepo(t)

	if _, err := r.CountWithOptions(ctx, nil, nil, nil); err == nil {
		t.Fatal("CountWithOptions with nil db must fail")
	}

	// nil opts behaves like plain Count
	cnt, err := r.CountWithOptions(ctx, db, nil, nil)
	if err != nil {
		t.Fatalf("CountWithOptions(nil opts) error: %v", err)
	}
	if cnt != 4 {
		t.Fatalf("expected 4, got %d", cnt)
	}

	// timeout must not break the query
	cnt, err = r.CountWithOptions(ctx, db, nil, &CountOptions{Timeout: time.Second})
	if err != nil || cnt != 4 {
		t.Fatalf("CountWithOptions(timeout) = %d, %v", cnt, err)
	}

	// distinct counts unique names only
	cnt, err = r.CountWithOptions(ctx, db, nil, &CountOptions{Distinct: "name"})
	if err != nil {
		t.Fatalf("CountWithOptions(distinct) error: %v", err)
	}
	if cnt != 3 {
		t.Fatalf("expected distinct count 3, got %d", cnt)
	}

	// extra scopes apply after where selectors
	cnt, err = r.CountWithOptions(ctx, db, []func(*gorm.DB) *gorm.DB{whereAgeOver(25)},
		&CountOptions{Scopes: []func(*gorm.DB) *gorm.DB{
			func(db *gorm.DB) *gorm.DB { return db.Where("name = ?", "bob") },
			nil,
		}})
	if err != nil {
		t.Fatalf("CountWithOptions(scopes) error: %v", err)
	}
	if cnt != 1 {
		t.Fatalf("expected scoped count 1, got %d", cnt)
	}
}

func TestRepository_ListWithPaging_Errors(t *testing.T) {
	db := openTestDBForRepository(t)
	ctx := context.Background()
	r := newCrudRepo(t)

	if _, err := r.ListWithPaging(ctx, db, nil); err == nil {
		t.Fatal("nil request must fail")
	}
	if _, err := r.ListWithPaging(ctx, nil, &paginationV1.PagingRequest{}); err == nil {
		t.Fatal("nil db must fail")
	}
}

func TestRepository_ListWithPaging_Modes(t *testing.T) {
	db := openTestDBForRepository(t)
	ctx := context.Background()
	seedUsers(t, db,
		testUserEntity{Name: "alice", Age: 20},
		testUserEntity{Name: "bob", Age: 30},
		testUserEntity{Name: "carol", Age: 40},
		testUserEntity{Name: "dave", Age: 50},
	)
	r := newCrudRepo(t)

	cases := []struct {
		name      string
		req       *paginationV1.PagingRequest
		wantItems int
		wantTotal int
	}{
		{
			name:      "empty request returns all",
			req:       &paginationV1.PagingRequest{},
			wantItems: 4,
			wantTotal: 4,
		},
		{
			name:      "page based",
			req:       &paginationV1.PagingRequest{Page: uint32Ptr(2), PageSize: uint32Ptr(2)},
			wantItems: 2,
			wantTotal: 4,
		},
		{
			name:      "offset based",
			req:       &paginationV1.PagingRequest{Offset: uint64Ptr(1), Limit: uint32Ptr(2)},
			wantItems: 2,
			wantTotal: 4,
		},
		{
			name:      "no paging caps at bound",
			req:       &paginationV1.PagingRequest{NoPaging: boolPtr(true)},
			wantItems: 4,
			wantTotal: 4,
		},
		{
			name: "field mask limits columns",
			req: &paginationV1.PagingRequest{
				FieldMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
			},
			wantItems: 4,
			wantTotal: 4,
		},
		{
			name: "sorting by age desc",
			req: &paginationV1.PagingRequest{
				Sorting: []*paginationV1.Sorting{{Field: "age", Direction: paginationV1.Sorting_DESC}},
			},
			wantItems: 4,
			wantTotal: 4,
		},
		{
			name: "order by string",
			req: &paginationV1.PagingRequest{
				OrderBy: strPtr("age desc"),
			},
			wantItems: 4,
			wantTotal: 4,
		},
		{
			name: "where filter via filter expr",
			req: &paginationV1.PagingRequest{
				FilteringType: &paginationV1.PagingRequest_FilterExpr{
					FilterExpr: &paginationV1.FilterExpr{
						Type: paginationV1.ExprType_AND,
						Conditions: []*paginationV1.FilterCondition{{
							Field:      "name",
							Op:         paginationV1.Operator_EQ,
							ValueOneof: &paginationV1.FilterCondition_Value{Value: "bob"},
						}},
					},
				},
			},
			wantItems: 1,
			wantTotal: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := r.ListWithPaging(ctx, db, tc.req)
			if err != nil {
				t.Fatalf("ListWithPaging(%s) error: %v", tc.name, err)
			}
			if res == nil {
				t.Fatalf("ListWithPaging(%s) returned nil", tc.name)
			}
			if len(res.Items) != tc.wantItems {
				t.Fatalf("ListWithPaging(%s) items = %d, want %d", tc.name, len(res.Items), tc.wantItems)
			}
			if int(res.Total) != tc.wantTotal {
				t.Fatalf("ListWithPaging(%s) total = %d, want %d", tc.name, res.Total, tc.wantTotal)
			}
		})
	}

	// token based paging needs a token plus offset; verify the cursor filters rows
	pagination.SetTokenSecret([]byte("unit-test-secret"))
	defer pagination.SetTokenSecret(nil)
	tok := pagination.EncodeAndSign(2, pagination.TokenSecret())
	res, err := r.ListWithPaging(ctx, db, &paginationV1.PagingRequest{
		Token:  strPtr(tok),
		Offset: uint64Ptr(2),
	})
	if err != nil {
		t.Fatalf("ListWithPaging(token) error: %v", err)
	}
	if len(res.Items) != 2 {
		t.Fatalf("expected 2 items after cursor id=2, got %d", len(res.Items))
	}

	// malformed JSON order-by string must fail the request
	if _, err := r.ListWithPaging(ctx, db, &paginationV1.PagingRequest{OrderBy: strPtr("[")}); err == nil {
		t.Fatal("expected malformed order-by to fail")
	}
}

func TestRepository_ListWithPagination_Modes(t *testing.T) {
	db := openTestDBForRepository(t)
	ctx := context.Background()
	seedUsers(t, db,
		testUserEntity{Name: "alice", Age: 20},
		testUserEntity{Name: "bob", Age: 30},
		testUserEntity{Name: "carol", Age: 40},
	)
	r := newCrudRepo(t)

	if _, err := r.ListWithPagination(ctx, db, nil); err == nil {
		t.Fatal("nil request must fail")
	}
	if _, err := r.ListWithPagination(ctx, nil, &paginationV1.PaginationRequest{}); err == nil {
		t.Fatal("nil db must fail")
	}

	cases := []struct {
		name      string
		req       *paginationV1.PaginationRequest
		wantItems int
		wantTotal int
	}{
		{
			name:      "unspecified pagination falls back to max limit",
			req:       &paginationV1.PaginationRequest{},
			wantItems: 3,
			wantTotal: 3,
		},
		{
			name: "offset based",
			req: &paginationV1.PaginationRequest{
				PaginationType: &paginationV1.PaginationRequest_OffsetBased{
					OffsetBased: &paginationV1.OffsetBasedPagination{Offset: 1, Limit: 1},
				},
			},
			wantItems: 1,
			wantTotal: 3,
		},
		{
			name: "page based",
			req: &paginationV1.PaginationRequest{
				PaginationType: &paginationV1.PaginationRequest_PageBased{
					PageBased: &paginationV1.PageBasedPagination{Page: 2, PageSize: 2},
				},
			},
			wantItems: 1,
			wantTotal: 3,
		},
		{
			name: "field mask",
			req: &paginationV1.PaginationRequest{
				PaginationType: &paginationV1.PaginationRequest_NoPaging{},
				FieldMask:      &fieldmaskpb.FieldMask{Paths: []string{"name"}},
			},
			wantItems: 3,
			wantTotal: 3,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := r.ListWithPagination(ctx, db, tc.req)
			if err != nil {
				t.Fatalf("ListWithPagination(%s) error: %v", tc.name, err)
			}
			if len(res.Items) != tc.wantItems {
				t.Fatalf("ListWithPagination(%s) items = %d, want %d", tc.name, len(res.Items), tc.wantItems)
			}
			if int(res.Total) != tc.wantTotal {
				t.Fatalf("ListWithPagination(%s) total = %d, want %d", tc.name, res.Total, tc.wantTotal)
			}
		})
	}

	// token based pagination returns rows after the decoded cursor
	pagination.SetTokenSecret([]byte("unit-test-secret"))
	defer pagination.SetTokenSecret(nil)
	tok := pagination.EncodeAndSign(1, pagination.TokenSecret())
	res, err := r.ListWithPagination(ctx, db, &paginationV1.PaginationRequest{
		PaginationType: &paginationV1.PaginationRequest_TokenBased{
			TokenBased: &paginationV1.TokenBasedPagination{Token: tok, PageSize: 1},
		},
	})
	if err != nil {
		t.Fatalf("ListWithPagination(token) error: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("expected 1 item after cursor id=1, got %d", len(res.Items))
	}

	// malformed JSON order-by string fails
	if _, err := r.ListWithPagination(ctx, db, &paginationV1.PaginationRequest{
		OrderBy: strPtr("["),
	}); err == nil {
		t.Fatal("expected malformed order-by to fail")
	}
}

func TestRepository_GetAndOnly(t *testing.T) {
	db := openTestDBForRepository(t)
	ctx := context.Background()
	seedUsers(t, db, testUserEntity{Name: "alice", Age: 20})
	r := newCrudRepo(t)

	if _, err := r.Get(ctx, nil, nil); err == nil {
		t.Fatal("Get with nil db must fail")
	}

	dto, err := r.Get(ctx, db, nil)
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if dto == nil || dto.Name != "alice" {
		t.Fatalf("unexpected dto: %+v", dto)
	}

	// Only is an alias of Get
	dto2, err := r.Only(ctx, db, nil)
	if err != nil || dto2 == nil || dto2.Name != "alice" {
		t.Fatalf("Only error: %v, dto: %+v", err, dto2)
	}

	// view mask selects a subset of columns
	dto3, err := r.Get(ctx, db, &fieldmaskpb.FieldMask{Paths: []string{"name"}})
	if err != nil || dto3 == nil || dto3.Name != "alice" {
		t.Fatalf("Get with mask error: %v, dto: %+v", err, dto3)
	}

	// GetWithFilters applies the given selectors
	dto4, err := r.GetWithFilters(ctx, db, []func(*gorm.DB) *gorm.DB{whereAgeOver(100)}, nil)
	if err == nil {
		t.Fatal("GetWithFilters with unsatisfying filter must fail with record not found")
	}
	if dto4 != nil {
		t.Fatalf("expected nil dto, got %+v", dto4)
	}

	dto5, err := r.GetWithFilters(ctx, db, []func(*gorm.DB) *gorm.DB{nil, whereAgeOver(10)}, nil)
	if err != nil || dto5 == nil || dto5.Name != "alice" {
		t.Fatalf("GetWithFilters error: %v, dto: %+v", err, dto5)
	}
}

func TestRepository_CreateVariants(t *testing.T) {
	db := openTestDBForRepository(t)
	ctx := context.Background()
	r := newCrudRepo(t)

	// argument validation
	if _, err := r.Create(ctx, nil, &CacheTestUser{Name: "x"}, nil); err == nil {
		t.Fatal("Create with nil db must fail")
	}
	if _, err := r.Create(ctx, db, nil, nil); err == nil {
		t.Fatal("Create with nil dto must fail")
	}
	if _, err := r.CreateX(ctx, nil, &CacheTestUser{Name: "x"}, nil); err == nil {
		t.Fatal("CreateX with nil db must fail")
	}
	if _, err := r.CreateX(ctx, db, nil, nil); err == nil {
		t.Fatal("CreateX with nil dto must fail")
	}
	if _, err := r.CreateXWithFilters(ctx, nil, nil, &CacheTestUser{Name: "x"}, nil); err == nil {
		t.Fatal("CreateXWithFilters with nil db must fail")
	}
	if _, err := r.CreateXWithFilters(ctx, db, nil, nil, nil); err == nil {
		t.Fatal("CreateXWithFilters with nil dto must fail")
	}

	// Create returns the persisted DTO with the generated ID
	created, err := r.Create(ctx, db, &CacheTestUser{Name: "alice", Age: 20}, nil)
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if created == nil || created.Id == 0 {
		t.Fatalf("Create returned invalid dto: %+v", created)
	}

	// CreateX returns affected rows and honours the view mask
	rows, err := r.CreateX(ctx, db, &CacheTestUser{Name: "bob", Age: 30}, &fieldmaskpb.FieldMask{Paths: []string{"name"}})
	if err != nil || rows != 1 {
		t.Fatalf("CreateX = %d, %v", rows, err)
	}

	// CreateXWithFilters applies selectors (harmless for INSERT) and creates
	rows, err = r.CreateXWithFilters(ctx, db, []func(*gorm.DB) *gorm.DB{nil}, &CacheTestUser{Name: "carol", Age: 40}, nil)
	if err != nil || rows != 1 {
		t.Fatalf("CreateXWithFilters = %d, %v", rows, err)
	}

	// BatchCreate
	if _, err := r.BatchCreate(ctx, nil, []*CacheTestUser{{Name: "x"}}, nil); err == nil {
		t.Fatal("BatchCreate with nil db must fail")
	}
	got, err := r.BatchCreate(ctx, db, nil, nil)
	if err != nil || got != nil {
		t.Fatalf("BatchCreate(empty) = %v, %v", got, err)
	}
	got, err = r.BatchCreate(ctx, db, []*CacheTestUser{
		{Name: "dave", Age: 50},
		nil, // nil entries are skipped
		{Name: "erin", Age: 60},
	}, nil)
	if err != nil {
		t.Fatalf("BatchCreate error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 created dtos, got %d", len(got))
	}

	// total seeded rows: alice, bob, carol, dave, erin
	cnt, err := r.Count(ctx, db, nil)
	if err != nil || cnt != 5 {
		t.Fatalf("count after creates = %d, %v", cnt, err)
	}
}

func TestRepository_UpdateVariants(t *testing.T) {
	db := openTestDBForRepository(t)
	ctx := context.Background()
	seedUsers(t, db, testUserEntity{Name: "alice", Age: 20})
	r := newCrudRepo(t)

	if _, err := r.Update(ctx, nil, &CacheTestUser{Name: "x"}, nil); err == nil {
		t.Fatal("Update with nil db must fail")
	}
	if _, err := r.Update(ctx, db, nil, nil); err == nil {
		t.Fatal("Update with nil dto must fail")
	}
	if _, err := r.UpdateX(ctx, nil, &CacheTestUser{Name: "x"}, nil); err == nil {
		t.Fatal("UpdateX with nil db must fail")
	}
	if _, err := r.UpdateX(ctx, db, nil, nil); err == nil {
		t.Fatal("UpdateX with nil dto must fail")
	}

	// Update applies the change and returns the refreshed DTO.
	// Note: the WHERE clause must not reference the columns being changed,
	// because the read-back query reuses the same clause.
	updated, err := r.Update(ctx, db.Where("id = ?", 1), &CacheTestUser{Id: 1, Name: "alice2", Age: 21}, nil)
	if err != nil {
		t.Fatalf("Update error: %v", err)
	}
	if updated == nil || updated.Name != "alice2" || updated.Age != 21 {
		t.Fatalf("unexpected updated dto: %+v", updated)
	}

	// Update with mask only touches the selected column
	updated, err = r.Update(ctx, db.Where("id = ?", 1),
		&CacheTestUser{Id: 1, Name: "alice3", Age: 99},
		&fieldmaskpb.FieldMask{Paths: []string{"name"}})
	if err != nil || updated == nil || updated.Name != "alice3" {
		t.Fatalf("Update(mask) = %+v, %v", updated, err)
	}
	got, _ := r.Get(ctx, db.Where("name = ?", "alice3"), nil)
	if got == nil || got.Age != 21 {
		t.Fatalf("mask update must not change age, got %+v", got)
	}

	// UpdateX returns affected rows
	rows, err := r.UpdateX(ctx, db.Where("id = ?", 1), &CacheTestUser{Id: 1, Age: 22}, nil)
	if err != nil || rows != 1 {
		t.Fatalf("UpdateX = %d, %v", rows, err)
	}

	// UpdateWithFilters / UpdateXWithFilters with selectors
	updated, err = r.UpdateWithFilters(ctx, db, []func(*gorm.DB) *gorm.DB{whereAgeOver(0)}, &CacheTestUser{Id: 1, Age: 23}, nil)
	if err != nil || updated == nil || updated.Age != 23 {
		t.Fatalf("UpdateWithFilters = %+v, %v", updated, err)
	}
	rows, err = r.UpdateXWithFilters(ctx, db, []func(*gorm.DB) *gorm.DB{whereAgeOver(0)}, &CacheTestUser{Id: 1, Age: 24}, nil)
	if err != nil || rows != 1 {
		t.Fatalf("UpdateXWithFilters = %d, %v", rows, err)
	}

	// updating nothing must not error and report zero rows
	rows, err = r.UpdateX(ctx, db.Where("name = ?", "missing"), &CacheTestUser{Id: 42, Age: 1}, nil)
	if err != nil || rows != 0 {
		t.Fatalf("UpdateX(missing) = %d, %v", rows, err)
	}
}

func TestRepository_UpsertVariants(t *testing.T) {
	db := openTestDBForRepository(t)
	ctx := context.Background()
	r := newCrudRepo(t)

	if _, err := r.Upsert(ctx, nil, &CacheTestUser{Name: "x"}, nil); err == nil {
		t.Fatal("Upsert with nil db must fail")
	}
	if _, err := r.Upsert(ctx, db, nil, nil); err == nil {
		t.Fatal("Upsert with nil dto must fail")
	}
	if _, err := r.UpsertX(ctx, nil, &CacheTestUser{Name: "x"}, nil); err == nil {
		t.Fatal("UpsertX with nil db must fail")
	}
	if _, err := r.UpsertX(ctx, db, nil, nil); err == nil {
		t.Fatal("UpsertX with nil dto must fail")
	}

	// Insert path (no conflicting row yet)
	dto, err := r.Upsert(ctx, db, &CacheTestUser{Id: 1, Name: "alice", Age: 20}, nil)
	if err != nil {
		t.Fatalf("Upsert(insert) error: %v", err)
	}
	if dto == nil || dto.Name != "alice" {
		t.Fatalf("Upsert(insert) unexpected dto: %+v", dto)
	}

	// Upsert with mask performs the conflict update on the chosen columns
	dto, err = r.Upsert(ctx, db, &CacheTestUser{Id: 1, Name: "alice2", Age: 21},
		&fieldmaskpb.FieldMask{Paths: []string{"name"}})
	if err != nil {
		t.Fatalf("Upsert(conflict, mask) error: %v", err)
	}
	if dto == nil || dto.Name != "alice2" {
		t.Fatalf("Upsert(conflict, mask) unexpected dto: %+v", dto)
	}

	// X variants report affected rows
	rows, err := r.UpsertX(ctx, db, &CacheTestUser{Id: 2, Name: "bob", Age: 30}, nil)
	if err != nil || rows == 0 {
		t.Fatalf("UpsertX(insert) = %d, %v", rows, err)
	}
	rows, err = r.UpsertXWithFilters(ctx, db, nil, &CacheTestUser{Id: 2, Name: "bob2", Age: 31},
		&fieldmaskpb.FieldMask{Paths: []string{"name"}})
	if err != nil || rows == 0 {
		t.Fatalf("UpsertXWithFilters(conflict) = %d, %v", rows, err)
	}

	// UpsertWithFilters insert path
	dto, err = r.UpsertWithFilters(ctx, db, []func(*gorm.DB) *gorm.DB{nil}, &CacheTestUser{Id: 3, Name: "carol", Age: 40}, nil)
	if err != nil || dto == nil || dto.Name != "carol" {
		t.Fatalf("UpsertWithFilters(insert) = %+v, %v", dto, err)
	}
}

func TestRepository_DeleteExistsSoftDelete(t *testing.T) {
	db := openTestDBForRepository(t)
	ctx := context.Background()
	seedUsers(t, db,
		testUserEntity{Name: "alice", Age: 20},
		testUserEntity{Name: "bob", Age: 30},
	)
	r := newCrudRepo(t)

	if _, err := r.Delete(ctx, nil, false); err == nil {
		t.Fatal("Delete with nil db must fail")
	}
	if _, err := r.Exists(ctx, nil); err == nil {
		t.Fatal("Exists with nil db must fail")
	}

	// Exists true / false
	ok, err := r.Exists(ctx, db.Where("name = ?", "alice"))
	if err != nil || !ok {
		t.Fatalf("Exists(alice) = %v, %v", ok, err)
	}
	ok, err = r.Exists(ctx, db.Where("name = ?", "ghost"))
	if err != nil || ok {
		t.Fatalf("Exists(ghost) = %v, %v", ok, err)
	}
	ok, err = r.ExistsWithFilters(ctx, db, []func(*gorm.DB) *gorm.DB{nil, whereAgeOver(25)})
	if err != nil || !ok {
		t.Fatalf("ExistsWithFilters = %v, %v", ok, err)
	}
	ok, err = r.ExistsWithFilters(ctx, db, []func(*gorm.DB) *gorm.DB{whereAgeOver(100)})
	if err != nil || ok {
		t.Fatalf("ExistsWithFilters(none) = %v, %v", ok, err)
	}

	// SoftDelete delegates to Delete with soft-delete behaviour
	rows, err := r.SoftDelete(ctx, db.Where("name = ?", "bob"))
	if err != nil || rows != 1 {
		t.Fatalf("SoftDelete = %d, %v", rows, err)
	}
	// soft-deleted rows vanish from ordinary queries
	cnt, err := r.Count(ctx, db, nil)
	if err != nil || cnt != 1 {
		t.Fatalf("count after soft delete = %d, %v", cnt, err)
	}

	// hard delete path
	rows, err = r.Delete(ctx, db.Where("name = ?", "alice"), true)
	if err != nil || rows != 1 {
		t.Fatalf("hard Delete = %d, %v", rows, err)
	}

	// DeleteWithFilters
	seedUsers(t, db, testUserEntity{Name: "c1", Age: 1}, testUserEntity{Name: "c2", Age: 2})
	rows, err = r.DeleteWithFilters(ctx, db, []func(*gorm.DB) *gorm.DB{whereAgeOver(0)})
	if err != nil || rows != 2 {
		t.Fatalf("DeleteWithFilters = %d, %v", rows, err)
	}
}
