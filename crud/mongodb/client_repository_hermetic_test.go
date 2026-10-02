package mongodb

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tx7do/go-utils/mapper"

	"github.com/tx7do/go-wind/log"
	bson "go.mongodb.org/mongo-driver/v2/bson"

	mongoV2 "go.mongodb.org/mongo-driver/v2/mongo"
	optionsV2 "go.mongodb.org/mongo-driver/v2/mongo/options"

	paginationV1 "github.com/tx7do/go-wind-plugins/crud/api/gen/go/pagination/v1"
	"github.com/tx7do/go-wind-plugins/crud/mongodb/query"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// ---------------------------------------------------------------------------
// Hermetic tests: everything below runs without a live mongodb server. The
// Client wrappers guard on an nil inner *mongoV2.Client, so a zero-value
// Client exercises every "not initialized" branch, and NewClient/Close only
// build (never dial) the lazy mongo driver client.
// ---------------------------------------------------------------------------

type hermeticUser struct {
	Id   string
	Name string
	Age  int
}

type nopLogger struct{}

func (n nopLogger) With(args ...any) log.Logger         { return n }
func (nopLogger) Enabled(log.Level) bool                { return false }
func (nopLogger) Debug(context.Context, string, ...any) {}
func (nopLogger) Info(context.Context, string, ...any)  {}
func (nopLogger) Warn(context.Context, string, ...any)  {}
func (nopLogger) Error(context.Context, string, ...any) {}

func TestClient_NotInitialized_Guards(t *testing.T) {
	c := &Client{}
	ctx := context.Background()

	_, err := c.InsertOne(ctx, "col", &hermeticUser{})
	assert.ErrorIs(t, err, mongoV2.ErrClientDisconnected)
	_, err = c.InsertMany(ctx, "col", []any{&hermeticUser{}})
	assert.ErrorIs(t, err, mongoV2.ErrClientDisconnected)
	_, err = c.UpdateOne(ctx, "col", bson.M{}, bson.M{})
	assert.ErrorIs(t, err, mongoV2.ErrClientDisconnected)
	_, err = c.UpdateMany(ctx, "col", bson.M{}, bson.M{})
	assert.ErrorIs(t, err, mongoV2.ErrClientDisconnected)
	_, err = c.DeleteOne(ctx, "col", bson.M{})
	assert.ErrorIs(t, err, mongoV2.ErrClientDisconnected)
	_, err = c.DeleteMany(ctx, "col", bson.M{})
	assert.ErrorIs(t, err, mongoV2.ErrClientDisconnected)

	assert.ErrorIs(t, c.FindOne(ctx, "col", bson.M{}, &hermeticUser{}), mongoV2.ErrClientDisconnected)
	assert.ErrorIs(t, c.Find(ctx, "col", bson.M{}, &[]*hermeticUser{}), mongoV2.ErrClientDisconnected)
	assert.ErrorIs(t, c.FindOneAndUpdate(ctx, "col", bson.M{}, bson.M{}, &hermeticUser{}), mongoV2.ErrClientDisconnected)

	n, err2 := c.Count(ctx, "col", bson.M{})
	assert.Zero(t, n)
	assert.ErrorIs(t, err2, mongoV2.ErrClientDisconnected)

	exists, err3 := c.Exist(ctx, "col", bson.M{})
	assert.False(t, exists)
	assert.ErrorIs(t, err3, mongoV2.ErrClientDisconnected)

	// CheckConnect reports false for an uninitialized client
	assert.False(t, c.CheckConnect())

	// Close on a nil inner client logs and returns instead of panicking
	c.Close()
}

func TestNewClient_WithOptions(t *testing.T) {
	c, err := NewClient(
		WithURI("mongodb://127.0.0.1:27017"),
		WithDatabase("hermetic"),
		WithTLSConfig(nil),
		WithTimeout(1<<20),
		WithConnectTimeout(1<<20),
		WithServerSelectionTimeout(1<<20),
		WithHeartbeatInterval(time.Second),
		WithLocalThreshold(1),
		WithMaxConnIdleTime(1),
		WithCredentials("", ""), // empty credentials are skipped
		WithCredentials("user", "pass"),
		WithBSONOptions(&optionsV2.BSONOptions{}),
		WithLogger(nopLogger{}),
	)
	require.NoError(t, err)
	require.NotNil(t, c)

	// the driver client is created lazily, no connection has been attempted
	assert.NotNil(t, c.cli)
	assert.Equal(t, "hermetic", c.database)

	// closing disconnects the lazy client and clears it
	c.Close()
	assert.Nil(t, c.cli)

	// double close hits the "already closed" guard
	c.Close()
}

func TestNewClient_InvalidURI(t *testing.T) {
	c, err := NewClient(WithURI("://not-a-mongo-uri"))
	assert.Error(t, err)
	assert.Nil(t, c)
}

// ---------------------------------------------------------------------------
// Repository over a disconnected client: full pipeline runs, only the final
// driver call reports ErrClientDisconnected.
// ---------------------------------------------------------------------------

func newHermeticRepo(t *testing.T, client *Client, collection string) *Repository[hermeticUser, hermeticUser] {
	t.Helper()
	return NewRepository[hermeticUser, hermeticUser](client, collection,
		mapper.NewCopierMapper[hermeticUser, hermeticUser](), nil)
}

func TestRepository_ValidationErrors(t *testing.T) {
	ctx := context.Background()

	// nil client validation on every method
	repo := newHermeticRepo(t, nil, "users")
	_, _, err := repo.ListWithPaging(ctx, &paginationV1.PagingRequest{})
	assert.EqualError(t, err, "mongodb database is nil")
	_, _, err = repo.ListWithPagination(ctx, &paginationV1.PaginationRequest{})
	assert.EqualError(t, err, "mongodb database is nil")
	_, err = repo.Get(ctx, nil, nil)
	assert.EqualError(t, err, "mongodb database is nil")
	_, err = repo.Create(ctx, &hermeticUser{})
	assert.EqualError(t, err, "mongodb database is nil")
	_, err = repo.BatchCreate(ctx, []*hermeticUser{{}})
	assert.EqualError(t, err, "mongodb database is nil")
	_, err = repo.Update(ctx, query.NewQueryBuilder(), bson.M{})
	assert.EqualError(t, err, "mongodb database is nil")
	_, err = repo.Delete(ctx, query.NewQueryBuilder())
	assert.EqualError(t, err, "mongodb database is nil")
	_, err = repo.Count(ctx, nil)
	assert.EqualError(t, err, "mongodb database is nil")
	_, err = repo.Exists(ctx, nil)
	assert.EqualError(t, err, "mongodb database is nil")

	// empty collection validation
	repo = newHermeticRepo(t, &Client{}, "")
	_, _, err = repo.ListWithPaging(ctx, &paginationV1.PagingRequest{})
	assert.EqualError(t, err, "collection is empty")
	_, _, err = repo.ListWithPagination(ctx, &paginationV1.PaginationRequest{})
	assert.EqualError(t, err, "collection is empty")
	_, err = repo.Get(ctx, nil, nil)
	assert.EqualError(t, err, "collection is empty")
	_, err = repo.Create(ctx, &hermeticUser{})
	assert.EqualError(t, err, "collection is empty")
	_, err = repo.BatchCreate(ctx, []*hermeticUser{{}})
	assert.EqualError(t, err, "collection is empty")
	_, err = repo.Update(ctx, query.NewQueryBuilder(), bson.M{})
	assert.EqualError(t, err, "collection is empty")
	_, err = repo.Delete(ctx, query.NewQueryBuilder())
	assert.EqualError(t, err, "collection is empty")
	_, err = repo.Count(ctx, nil)
	assert.EqualError(t, err, "collection is empty")
	_, err = repo.Exists(ctx, nil)
	assert.EqualError(t, err, "collection is empty")

	// nil dto validation
	repo = newHermeticRepo(t, &Client{}, "users")
	_, err = repo.Create(ctx, nil)
	assert.EqualError(t, err, "dto is nil")

	// nil qb validation
	_, err = repo.Update(ctx, nil, bson.M{})
	assert.EqualError(t, err, "query builder is nil for update")
	_, err = repo.Delete(ctx, nil)
	assert.EqualError(t, err, "query builder is nil for delete")

	// empty batch is a no-op
	out, err := repo.BatchCreate(ctx, nil)
	assert.NoError(t, err)
	assert.Empty(t, out)
}

func TestRepository_DisconnectedPipeline(t *testing.T) {
	ctx := context.Background()
	repo := newHermeticRepo(t, &Client{}, "users")

	// ListWithPaging: filters, field mask, sorting and pagination clauses are
	// built before the (disconnected) count call fails
	req := &paginationV1.PagingRequest{
		Page:     ptrUint32(1),
		PageSize: ptrUint32(10),
		Sorting: []*paginationV1.Sorting{
			{Field: "name", Direction: paginationV1.Sorting_ASC},
		},
		FieldMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
	}
	_, _, err := repo.ListWithPaging(ctx, req)
	assert.ErrorIs(t, err, mongoV2.ErrClientDisconnected)

	// offset/token variant plus order-by string
	req = &paginationV1.PagingRequest{
		Offset:  ptrUint64(5),
		Limit:   ptrUint32(5),
		OrderBy: ptrString("name desc"),
	}
	_, _, err = repo.ListWithPaging(ctx, req)
	assert.ErrorIs(t, err, mongoV2.ErrClientDisconnected)

	// ListWithPagination: offset-based variant
	preq := &paginationV1.PaginationRequest{
		PaginationType: &paginationV1.PaginationRequest_OffsetBased{
			OffsetBased: &paginationV1.OffsetBasedPagination{Offset: 1, Limit: 2},
		},
	}
	_, _, err = repo.ListWithPagination(ctx, preq)
	assert.ErrorIs(t, err, mongoV2.ErrClientDisconnected)

	// page-based and token-based variants
	preq = &paginationV1.PaginationRequest{
		PaginationType: &paginationV1.PaginationRequest_PageBased{
			PageBased: &paginationV1.PageBasedPagination{Page: 2, PageSize: 2},
		},
	}
	_, _, err = repo.ListWithPagination(ctx, preq)
	assert.ErrorIs(t, err, mongoV2.ErrClientDisconnected)

	preq = &paginationV1.PaginationRequest{
		PaginationType: &paginationV1.PaginationRequest_TokenBased{
			TokenBased: &paginationV1.TokenBasedPagination{Token: "garbage", PageSize: 2},
		},
	}
	_, _, err = repo.ListWithPagination(ctx, preq)
	assert.ErrorIs(t, err, mongoV2.ErrClientDisconnected)

	// Get with view mask
	_, err = repo.Get(ctx, query.NewQueryBuilder(), &fieldmaskpb.FieldMask{Paths: []string{"name"}})
	assert.ErrorIs(t, err, mongoV2.ErrClientDisconnected)

	// Create / BatchCreate
	_, err = repo.Create(ctx, &hermeticUser{Id: "1", Name: "a"})
	assert.ErrorIs(t, err, mongoV2.ErrClientDisconnected)
	_, err = repo.BatchCreate(ctx, []*hermeticUser{{Id: "1"}, {Id: "2"}})
	assert.ErrorIs(t, err, mongoV2.ErrClientDisconnected)

	// Update / Delete / Count / Exists
	_, err = repo.Update(ctx, query.NewQueryBuilder(), bson.M{})
	assert.ErrorIs(t, err, mongoV2.ErrClientDisconnected)
	_, err = repo.Delete(ctx, query.NewQueryBuilder())
	assert.ErrorIs(t, err, mongoV2.ErrClientDisconnected)
	_, err = repo.Count(ctx, query.NewQueryBuilder())
	assert.ErrorIs(t, err, mongoV2.ErrClientDisconnected)
	_, err = repo.Exists(ctx, query.NewQueryBuilder())
	assert.ErrorIs(t, err, mongoV2.ErrClientDisconnected)
}

func ptrUint32(v uint32) *uint32 { return &v }
func ptrUint64(v uint64) *uint64 { return &v }
func ptrString(v string) *string { return &v }
