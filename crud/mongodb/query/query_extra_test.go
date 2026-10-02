package query

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	bsonV2 "go.mongodb.org/mongo-driver/v2/bson"
	optionsV2 "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// ---------------------------------------------------------------------------
// Where
// ---------------------------------------------------------------------------

func TestWhere_MergesAndOverrides(t *testing.T) {
	qb := NewQueryBuilder()
	qb.SetFilter(bsonV2.M{"name": "old", "keep": 1})

	qb.Where(bsonV2.M{"name": "new", "age": bsonV2.M{OperatorGt: 18}})

	assert.Equal(t, bsonV2.M{
		"name": "new", // overridden
		"keep": 1,     // preserved
		"age":  bsonV2.M{OperatorGt: 18},
	}, qb.filter)

	// Where returns the builder for chaining.
	assert.Same(t, qb, qb.Where(bsonV2.M{"x": 1}))
}

func TestWhere_NilFilterInitialized(t *testing.T) {
	qb := &Builder{} // filter nil

	qb.Where(bsonV2.M{"k": "v"})

	assert.Equal(t, bsonV2.M{"k": "v"}, qb.filter)
}

// ---------------------------------------------------------------------------
// Pagination setters
// ---------------------------------------------------------------------------

func TestSetSkipLimit(t *testing.T) {
	qb := NewQueryBuilder()
	qb.SetSkipLimit(20, 5)

	require.NotNil(t, qb.findOpts.Skip)
	require.NotNil(t, qb.findOpts.Limit)
	assert.Equal(t, int64(20), *qb.findOpts.Skip)
	assert.Equal(t, int64(5), *qb.findOpts.Limit)

	// Internal bookkeeping for BuildFind/BuildFindOne.
	require.NotNil(t, qb.skip)
	require.NotNil(t, qb.limit)
	assert.Equal(t, int64(20), *qb.skip)
	assert.Equal(t, int64(5), *qb.limit)
}

func TestSetPage_NormalizesInvalidInput(t *testing.T) {
	qb := NewQueryBuilder()
	qb.SetPage(0, 0) // page < 1, size <= 0 → normalized to 1 and 10

	require.NotNil(t, qb.findOpts.Skip)
	require.NotNil(t, qb.findOpts.Limit)
	assert.Equal(t, int64(0), *qb.findOpts.Skip) // (1-1)*10
	assert.Equal(t, int64(10), *qb.findOpts.Limit)

	qb.SetPage(-5, -3)
	assert.Equal(t, int64(0), *qb.findOpts.Skip)
	assert.Equal(t, int64(10), *qb.findOpts.Limit)
}

func TestSetTokenPagination(t *testing.T) {
	qb := NewQueryBuilder()
	qb.SetTokenPagination("cursor-abc", 25)

	require.NotNil(t, qb.token)
	require.NotNil(t, qb.pageSize)
	assert.Equal(t, "cursor-abc", *qb.token)
	assert.Equal(t, int64(25), *qb.pageSize)
}

// ---------------------------------------------------------------------------
// Pipeline
// ---------------------------------------------------------------------------

func TestBuildPipeline_NilReturnsNil(t *testing.T) {
	qb := NewQueryBuilder()
	assert.Nil(t, qb.BuildPipeline())
}

func TestBuildPipeline_ReturnsCopy(t *testing.T) {
	qb := NewQueryBuilder()
	stage := bsonV2.D{{Key: OperatorMatch, Value: bsonV2.M{"a": 1}}}
	qb.AddStage(stage)

	pipeline := qb.BuildPipeline()
	require.Len(t, pipeline, 1)

	// Mutating the returned slice must not affect the builder.
	pipeline[0] = bsonV2.D{{Key: OperatorLimit, Value: 1}}
	assert.Equal(t, stage, qb.pipeline[0])
}

// ---------------------------------------------------------------------------
// Build with nil findOpts
// ---------------------------------------------------------------------------

func TestBuild_NilFindOptsReturnsFreshOptions(t *testing.T) {
	qb := &Builder{filter: bsonV2.M{"k": "v"}}

	filter, opts := qb.Build()
	assert.Equal(t, bsonV2.M{"k": "v"}, filter)
	require.NotNil(t, opts)
	assert.Nil(t, opts.Limit)
	assert.Nil(t, opts.Skip)
}

// ---------------------------------------------------------------------------
// BuildFind
// ---------------------------------------------------------------------------

func TestBuildFind_WithFilterAndOptions(t *testing.T) {
	qb := NewQueryBuilder()
	qb.SetFilter(bsonV2.M{"name": "alice"})
	qb.SetSkipLimit(10, 20)
	qb.SetSort(bsonV2.D{{Key: "name", Value: 1}})
	qb.SetProjection(bsonV2.M{"name": 1})

	filter, lister, err := qb.BuildFind()
	require.NoError(t, err)
	assert.Equal(t, bsonV2.M{"name": "alice"}, filter)
	require.NotNil(t, lister)

	// Apply the lister to fresh options and check every field is transferred.
	fresh := &optionsV2.FindOptions{}
	fns := lister.List()
	require.Len(t, fns, 4) // limit, skip, sort, projection
	for _, fn := range fns {
		require.NoError(t, fn(fresh))
	}
	require.NotNil(t, fresh.Limit)
	assert.Equal(t, int64(20), *fresh.Limit)
	require.NotNil(t, fresh.Skip)
	assert.Equal(t, int64(10), *fresh.Skip)
	assert.Equal(t, bsonV2.D{{Key: "name", Value: 1}}, fresh.Sort)
	assert.Equal(t, bsonV2.M{"name": 1}, fresh.Projection)
}

func TestBuildFind_NilFilterReturnsEmptyFilter(t *testing.T) {
	qb := NewQueryBuilder()
	qb.filter = nil

	filter, lister, err := qb.BuildFind()
	require.NoError(t, err)
	assert.Equal(t, bsonV2.M{}, filter)
	require.NotNil(t, lister)
}

func TestBuildFind_NilFindOptsInitialized(t *testing.T) {
	qb := &Builder{filter: bsonV2.M{"k": "v"}} // findOpts nil

	filter, lister, err := qb.BuildFind()
	require.NoError(t, err)
	assert.Equal(t, bsonV2.M{"k": "v"}, filter)

	// Empty options → empty (but non-nil) lister.
	assert.Empty(t, lister.List())
}

func TestBuildFind_SkipLimitBookkeepingApplied(t *testing.T) {
	qb := NewQueryBuilder()
	// Simulate state where findOpts.Skip/Limit were cleared but bookkeeping kept.
	qb.SetSkipLimit(7, 3)
	qb.findOpts.Skip = nil
	qb.findOpts.Limit = nil

	_, lister, err := qb.BuildFind()
	require.NoError(t, err)

	fresh := &optionsV2.FindOptions{}
	for _, fn := range lister.List() {
		require.NoError(t, fn(fresh))
	}
	require.NotNil(t, fresh.Skip)
	require.NotNil(t, fresh.Limit)
	assert.Equal(t, int64(7), *fresh.Skip)
	assert.Equal(t, int64(3), *fresh.Limit)
}

// ---------------------------------------------------------------------------
// BuildFindOne
// ---------------------------------------------------------------------------

func TestBuildFindOne_WithFilterAndOptions(t *testing.T) {
	qb := NewQueryBuilder()
	qb.SetFilter(bsonV2.M{"_id": 42})
	qb.SetSkip(5)
	qb.SetSort(bsonV2.D{{Key: "createdAt", Value: -1}})
	qb.SetProjection(bsonV2.M{"name": 1, "createdAt": 1})

	filter, lister, err := qb.BuildFindOne()
	require.NoError(t, err)
	assert.Equal(t, bsonV2.M{"_id": 42}, filter)
	require.NotNil(t, lister)

	fresh := &optionsV2.FindOneOptions{}
	fns := lister.List()
	// NOTE: only Skip is propagated to FindOne options. SetSort/SetProjection
	// stay on FindOptions and are silently dropped by BuildFindOne — see report.
	require.Len(t, fns, 1)
	for _, fn := range fns {
		require.NoError(t, fn(fresh))
	}
	require.NotNil(t, fresh.Skip)
	assert.Equal(t, int64(5), *fresh.Skip)
	assert.Nil(t, fresh.Sort)
	assert.Nil(t, fresh.Projection)
}

func TestBuildFindOne_NilFilterReturnsEmptyFilter(t *testing.T) {
	qb := NewQueryBuilder()
	qb.filter = nil

	filter, lister, err := qb.BuildFindOne()
	require.NoError(t, err)
	assert.Equal(t, bsonV2.M{}, filter)
	require.NotNil(t, lister)
	assert.Empty(t, lister.List())
}

func TestBuildFindOne_NilFindOptsInitialized(t *testing.T) {
	qb := &Builder{filter: bsonV2.M{"k": "v"}}

	filter, lister, err := qb.BuildFindOne()
	require.NoError(t, err)
	assert.Equal(t, bsonV2.M{"k": "v"}, filter)
	assert.Empty(t, lister.List())
}

// ---------------------------------------------------------------------------
// Lister edge cases
// ---------------------------------------------------------------------------

func TestFindOptsLister_NilReceiverAndNilOpts(t *testing.T) {
	var nilLister *findOptsLister
	assert.Nil(t, nilLister.List())

	empty := &findOptsLister{}
	assert.Nil(t, empty.List())
}

func TestFindOneOptsLister_NilReceiverAndNilOpts(t *testing.T) {
	var nilLister *findOneOptsLister
	assert.Nil(t, nilLister.List())

	empty := &findOneOptsLister{}
	assert.Nil(t, empty.List())
}

// ---------------------------------------------------------------------------
// utils.go
// ---------------------------------------------------------------------------

func TestIsValidIdentifier(t *testing.T) {
	tests := []struct {
		name       string
		identifier string
		want       bool
	}{
		{"empty", "", false},
		{"simple", "name", true},
		{"with underscore", "user_name", true},
		{"leading underscore", "_id", true},
		{"with digits", "field1", true},
		{"digits then letter", "1field", false},
		{"with dash", "user-name", false},
		{"with dot", "user.name", false},
		{"with space", "user name", false},
		{"whitespace only", "   ", false},
		{"quoted simple", "`name`", true},
		{"quoted underscore", "`user_name`", true},
		{"quoted invalid inner", "`user-name`", false},
		{"quoted digit first", "`1bad`", false},
		{"only backticks", "``", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isValidIdentifier(tt.identifier))
		})
	}
}

func TestIsValidCondition(t *testing.T) {
	assert.True(t, isValidCondition("age > 18 AND status = 'active'"))
	assert.True(t, isValidCondition(""))

	// Statement separators and SQL comment markers are rejected.
	assert.False(t, isValidCondition("a = 1; DROP TABLE users"))
	assert.False(t, isValidCondition("a = 1 -- comment"))
	assert.False(t, isValidCondition(";"))
	assert.False(t, isValidCondition("--"))
}

// ---------------------------------------------------------------------------
// Nil findOpts defensive branches on every setter
// ---------------------------------------------------------------------------

func TestSetters_InitializeNilFindOpts(t *testing.T) {
	qb := &Builder{filter: bsonV2.M{}}

	qb.SetLimit(1)
	require.NotNil(t, qb.findOpts)
	assert.Equal(t, int64(1), *qb.findOpts.Limit)

	qb2 := &Builder{}
	qb2.SetSort(bsonV2.D{{Key: "a", Value: 1}})
	require.NotNil(t, qb2.findOpts)
	assert.Equal(t, bsonV2.D{{Key: "a", Value: 1}}, qb2.findOpts.Sort)

	qb3 := &Builder{}
	qb3.SetSortWithPriority([]bsonV2.E{{Key: "p", Value: -1}})
	require.NotNil(t, qb3.findOpts)
	assert.Equal(t, bsonV2.D{{Key: "p", Value: -1}}, qb3.findOpts.Sort)

	qb4 := &Builder{}
	qb4.SetProjection(bsonV2.M{"a": 1})
	require.NotNil(t, qb4.findOpts)
	assert.Equal(t, bsonV2.M{"a": 1}, qb4.findOpts.Projection)

	qb5 := &Builder{}
	qb5.SetSkip(9)
	require.NotNil(t, qb5.findOpts)
	require.NotNil(t, qb5.findOpts.Skip)
	assert.Equal(t, int64(9), *qb5.findOpts.Skip)

	qb6 := &Builder{}
	qb6.SetSkipLimit(1, 2)
	require.NotNil(t, qb6.findOpts)
	require.NotNil(t, qb6.findOpts.Skip)
	require.NotNil(t, qb6.findOpts.Limit)
	assert.Equal(t, int64(1), *qb6.findOpts.Skip)
	assert.Equal(t, int64(2), *qb6.findOpts.Limit)

	qb7 := &Builder{}
	qb7.SetPage(3, 4)
	require.NotNil(t, qb7.findOpts)
	require.NotNil(t, qb7.findOpts.Skip)
	require.NotNil(t, qb7.findOpts.Limit)
	assert.Equal(t, int64(8), *qb7.findOpts.Skip)
	assert.Equal(t, int64(4), *qb7.findOpts.Limit)
}

// ---------------------------------------------------------------------------
// findOneOptsLister — sort/projection branches
// ---------------------------------------------------------------------------

func TestFindOneOptsLister_SortAndProjection(t *testing.T) {
	sortD := bsonV2.D{{Key: "b", Value: 1}}
	l := &findOneOptsLister{opts: &optionsV2.FindOneOptions{
		Sort:       sortD,
		Projection: bsonV2.M{"b": 1},
	}}

	fresh := &optionsV2.FindOneOptions{}
	fns := l.List()
	require.Len(t, fns, 2)
	for _, fn := range fns {
		require.NoError(t, fn(fresh))
	}
	assert.Equal(t, sortD, fresh.Sort)
	assert.Equal(t, bsonV2.M{"b": 1}, fresh.Projection)
}
