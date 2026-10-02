package rule_test

import (
	"context"
	"strings"
	"testing"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/entql"
	"entgo.io/ent/privacy"

	"github.com/tx7do/go-wind-plugins/crud/entgo/ent"
	"github.com/tx7do/go-wind-plugins/crud/entgo/ent/enttest"
	"github.com/tx7do/go-wind-plugins/crud/entgo/rule"
	"github.com/tx7do/go-wind-plugins/crud/viewer"
	_ "github.com/xiaoqidun/entps"
)

// ---------------------------------------------------------------------------
// Additional viewer stubs (stubQueryViewer from tenant_r1_test.go is reused).
// ---------------------------------------------------------------------------

type scopedViewer struct {
	tid      uint64
	uid      uint64
	platform bool
	system   bool
	allow    bool
	scopes   []viewer.DataScope
}

func (s *scopedViewer) UserID() uint64                 { return s.uid }
func (s *scopedViewer) TenantID() uint64               { return s.tid }
func (s *scopedViewer) OrgUnitID() uint64              { return 0 }
func (s *scopedViewer) Permissions() []string          { return nil }
func (s *scopedViewer) Roles() []string                { return nil }
func (s *scopedViewer) DataScope() []viewer.DataScope  { return s.scopes }
func (s *scopedViewer) TraceID() string                { return "" }
func (s *scopedViewer) HasPermission(_, _ string) bool { return s.allow }
func (s *scopedViewer) IsPlatformContext() bool        { return s.platform }
func (s *scopedViewer) IsTenantContext() bool          { return s.tid > 0 && !s.platform }
func (s *scopedViewer) IsSystemContext() bool          { return s.system }
func (s *scopedViewer) ShouldAudit() bool              { return false }

func scopedCtx(v *scopedViewer) context.Context {
	return viewer.WithContext(context.Background(), v)
}

type fakeFilter struct {
	preds []entql.P
}

func (f *fakeFilter) Where(p entql.P) { f.preds = append(f.preds, p) }

// stubScopedBuilder mimics a tenant-scoped update builder (implements
// viewer.ScopedModel via GetTenantID/SetTenantID) and records Where calls.
type evalStubBuilder struct {
	whereCalled bool
}

func (s *evalStubBuilder) Where(ps ...func(*sql.Selector)) *evalStubBuilder {
	if len(ps) > 0 {
		s.whereCalled = true
	}
	return s
}
func (s *evalStubBuilder) GetTenantID() *uint32 { return nil }
func (s *evalStubBuilder) SetTenantID(uint32)   {}

// Query fakes for the reflect-based where-injection failure branches.
type noWhereQuery struct{}

type nonVariadicWhereQuery struct{}

func (nonVariadicWhereQuery) Where(func(*sql.Selector)) {}

type wrongPredQuery struct{}

func (wrongPredQuery) Where(ps ...func(int, *sql.Selector)) {}

// Mutation fakes for rule.GetClientFromMutation. Embedding *ent.MenuCreate
// satisfies ent.Mutation; a same-named field or method shadows the promoted
// Client method.
type noClientMutation struct {
	*ent.MenuMutation
	Client int // field shadows the promoted Client method
}

type voidClientMutation struct{ *ent.MenuMutation }

func (voidClientMutation) Client() {}

type plainClientMutation struct{ *ent.MenuMutation }

func (plainClientMutation) Client() int { return 1 }

type clientErrMutation struct {
	*ent.MenuMutation
	err error
}

func (m clientErrMutation) Client() (int, error) { return 0, m.err }

type twoValueClientMutation struct{ *ent.MenuMutation }

func (twoValueClientMutation) Client() (int, string) { return 0, "not an error" }

type argfulClientMutation struct{ *ent.MenuMutation }

func (argfulClientMutation) Client(int) int { return 0 }

// ---------------------------------------------------------------------------
// rule.TenantPrivacy.EvalQuery / EvalMutation branches
// ---------------------------------------------------------------------------

func TestTenantPrivacyEvalQuery_Branches(t *testing.T) {
	tp := rule.TenantPrivacy[uint32]{}

	// missing viewer fails closed
	if err := tp.EvalQuery(context.Background(), noWhereQuery{}); err == nil {
		t.Error("missing viewer must fail-closed")
	}

	// platform / system views pass without injection
	if err := tp.EvalQuery(scopedCtx(&scopedViewer{platform: true}), noWhereQuery{}); err != nil {
		t.Errorf("platform view must pass: %v", err)
	}
	if err := tp.EvalQuery(scopedCtx(&scopedViewer{system: true}), noWhereQuery{}); err != nil {
		t.Errorf("system view must pass: %v", err)
	}

	// query without a Where method fails closed
	if err := tp.EvalQuery(scopedCtx(&scopedViewer{tid: 3}), noWhereQuery{}); err == nil {
		t.Error("query without Where must fail-closed")
	} else if !strings.Contains(err.Error(), "no usable Where method") {
		t.Errorf("unexpected error: %v", err)
	}

	// non-variadic Where signature fails closed
	if err := tp.EvalQuery(scopedCtx(&scopedViewer{tid: 3}), nonVariadicWhereQuery{}); err == nil {
		t.Error("non-variadic Where must fail-closed")
	}

	// predicate element signature mismatch fails closed
	if err := tp.EvalQuery(scopedCtx(&scopedViewer{tid: 3}), wrongPredQuery{}); err == nil {
		t.Error("wrong predicate signature must fail-closed")
	}
}

func newEvalClient(t *testing.T) *ent.Client {
	t.Helper()
	cli := enttest.Open(t, "sqlite3",
		"file:rule_eval_"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = cli.Close() })
	return cli
}

func TestTenantPrivacyEvalMutation_Create(t *testing.T) {
	cli := newEvalClient(t)
	tp := rule.TenantPrivacy[uint32]{}

	// missing viewer fails closed
	if err := tp.EvalMutation(context.Background(), cli.User.Create().Mutation()); err == nil {
		t.Error("missing viewer must fail-closed")
	}

	// normal tenant user: tenant_id is force-overwritten
	create := cli.User.Create()
	if err := tp.EvalMutation(scopedCtx(&scopedViewer{tid: 5}), create.Mutation()); err != nil {
		t.Fatalf("tenant create must pass: %v", err)
	}
	if v, ok := create.Mutation().Field("tenant_id"); !ok || v.(uint32) != 5 {
		t.Fatalf("tenant_id must be forced to the viewer tenant, got %v", v)
	}

	// platform with explicit tenant_id is respected
	platformSet := cli.User.Create().SetTenantID(101)
	if err := tp.EvalMutation(scopedCtx(&scopedViewer{platform: true}), platformSet.Mutation()); err != nil {
		t.Fatalf("platform create with tenant_id must pass: %v", err)
	}
	// platform without explicit tenant_id also passes
	if err := tp.EvalMutation(scopedCtx(&scopedViewer{platform: true}), cli.User.Create().Mutation()); err != nil {
		t.Fatalf("platform create must pass: %v", err)
	}
}

func TestTenantPrivacyEvalMutation_Update(t *testing.T) {
	cli := newEvalClient(t)
	tp := rule.TenantPrivacy[uint32]{}
	tenantCtx := scopedCtx(&scopedViewer{tid: 5})

	// seed an own-tenant row through the create rule
	seed := cli.User.Create().SetName("seed-user")
	if err := tp.EvalMutation(tenantCtx, seed.Mutation()); err != nil {
		t.Fatalf("seed create: %v", err)
	}
	seeded, err := seed.Save(tenantCtx) // policy hook requires the viewer ctx
	if err != nil {
		t.Fatalf("seed save: %v", err)
	}

	// platform context skips update entirely
	if err := tp.EvalMutation(scopedCtx(&scopedViewer{platform: true}), cli.User.Update().Mutation()); err != nil {
		t.Fatalf("platform update must pass: %v", err)
	}

	// plain tenant update gets the row predicate injected
	if err := tp.EvalMutation(tenantCtx, cli.User.Update().Mutation()); err != nil {
		t.Fatalf("tenant update must pass: %v", err)
	}

	// touching tenant_id without a verifiable old value fails closed
	upd := cli.User.Update().Mutation()
	upd.SetTenantID(9)
	if err := tp.EvalMutation(tenantCtx, upd); err == nil {
		t.Error("unverifiable tenant_id change must fail-closed")
	} else if !strings.Contains(err.Error(), "cannot verify") {
		t.Errorf("unexpected error: %v", err)
	}

	// redundant same-tenant tenant_id set is allowed
	updOne := cli.User.UpdateOneID(seeded.ID).Mutation()
	updOne.SetTenantID(5)
	if err := tp.EvalMutation(tenantCtx, updOne); err != nil {
		t.Fatalf("redundant tenant_id set must pass: %v", err)
	}

	// moving a row to another tenant is denied
	updOther := cli.User.UpdateOneID(seeded.ID).Mutation()
	updOther.SetTenantID(9)
	if err := tp.EvalMutation(tenantCtx, updOther); err == nil {
		t.Error("cross-tenant tenant_id change must be denied")
	}
}

// The SetField reflection fallback covers mutations without the typed
// SetTenantID helper (menu has no tenant field at all).
func TestTenantPrivacyEvalMutation_SetFieldFallback(t *testing.T) {
	cli := newEvalClient(t)
	tp := rule.TenantPrivacy[uint32]{}

	if err := tp.EvalMutation(scopedCtx(&scopedViewer{tid: 5}), cli.Menu.Create().Mutation()); err != nil {
		t.Fatalf("SetField fallback must not fail: %v", err)
	}
}

// ---------------------------------------------------------------------------
// rule.InjectTenantWhereIntoBuilder: nil builder passes through
// ---------------------------------------------------------------------------

func TestInjectTenantWhereIntoBuilder_NilBuilder(t *testing.T) {
	ctx := scopedCtx(&scopedViewer{tid: 7})
	if err := rule.InjectTenantWhereIntoBuilder[evalStubBuilder](ctx, nil); err != nil {
		t.Fatalf("nil builder must pass, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// system.go rules
// ---------------------------------------------------------------------------

func TestOwnerOnlyRule(t *testing.T) {
	f := &fakeFilter{}
	if err := rule.OwnerOnlyRule(context.Background(), f); err == nil {
		t.Error("missing viewer must fail-closed")
	}

	if err := rule.OwnerOnlyRule(scopedCtx(&scopedViewer{platform: true}), f); err != nil {
		t.Errorf("platform view must pass: %v", err)
	}
	if err := rule.OwnerOnlyRule(scopedCtx(&scopedViewer{system: true}), f); err != nil {
		t.Errorf("system view must pass: %v", err)
	}

	f2 := &fakeFilter{}
	if err := rule.OwnerOnlyRule(scopedCtx(&scopedViewer{tid: 1, uid: 9}), f2); err != nil {
		t.Fatalf("owner rule: %v", err)
	}
	if len(f2.preds) != 1 {
		t.Fatalf("owner rule must inject one predicate, got %d", len(f2.preds))
	}
}

func TestPermissionRule(t *testing.T) {
	if err := rule.PermissionRule(context.Background(), &fakeFilter{}); err == nil {
		t.Error("missing viewer must fail-closed")
	}
	if err := rule.PermissionRule(scopedCtx(&scopedViewer{platform: true}), &fakeFilter{}); err != nil {
		t.Errorf("platform view must pass: %v", err)
	}
	if err := rule.PermissionRule(scopedCtx(&scopedViewer{tid: 1}), &fakeFilter{}); err == nil {
		t.Error("no scopes must fail-closed")
	}

	// ALL scope short-circuits without predicates
	f := &fakeFilter{}
	if err := rule.PermissionRule(scopedCtx(&scopedViewer{
		tid:    1,
		scopes: []viewer.DataScope{{ScopeType: viewer.ScopeTypeAll}},
	}), f); err != nil {
		t.Fatalf("all-scope must pass: %v", err)
	}
	if len(f.preds) != 0 {
		t.Errorf("all-scope must not inject predicates, got %d", len(f.preds))
	}

	// self scope injects the created_by predicate
	f = &fakeFilter{}
	if err := rule.PermissionRule(scopedCtx(&scopedViewer{
		tid: 1, uid: 9,
		scopes: []viewer.DataScope{{ScopeType: viewer.ScopeTypeSelf}},
	}), f); err != nil {
		t.Fatalf("self-scope: %v", err)
	}
	if len(f.preds) != 1 {
		t.Fatalf("self-scope must inject one predicate, got %d", len(f.preds))
	}

	// unit + user scopes OR-combine per target
	f = &fakeFilter{}
	if err := rule.PermissionRule(scopedCtx(&scopedViewer{
		tid: 1,
		scopes: []viewer.DataScope{
			{ScopeType: viewer.ScopeTypeUnit, TargetIDs: []uint64{3, 4}},
			{ScopeType: viewer.ScopeTypeUser, TargetIDs: []uint64{7}},
		},
	}), f); err != nil {
		t.Fatalf("unit+user scopes: %v", err)
	}
	// all appended predicates are OR-combined into a single Where call
	if len(f.preds) != 1 {
		t.Fatalf("unit+user scopes must combine into one Where call, got %d", len(f.preds))
	}

	// explicit NONE denies
	if err := rule.PermissionRule(scopedCtx(&scopedViewer{
		tid:    1,
		scopes: []viewer.DataScope{{ScopeType: viewer.ScopeTypeNone}},
	}), &fakeFilter{}); err == nil {
		t.Error("none-scope must deny")
	}

	// unknown scope types are skipped; scopes without resolvable predicates
	// deny defensively
	if err := rule.PermissionRule(scopedCtx(&scopedViewer{
		tid:    1,
		scopes: []viewer.DataScope{{ScopeType: viewer.ScopeType("MYSTERY")}},
	}), &fakeFilter{}); err == nil {
		t.Error("scopes without predicates must deny")
	}
}

func TestSoftDeleteRule(t *testing.T) {
	if err := rule.SoftDeleteRule(context.Background(), &fakeFilter{}); err == nil {
		t.Error("missing viewer must fail-closed")
	}
	if err := rule.SoftDeleteRule(scopedCtx(&scopedViewer{platform: true}), &fakeFilter{}); err != nil {
		t.Errorf("platform view must pass: %v", err)
	}
	f := &fakeFilter{}
	if err := rule.SoftDeleteRule(scopedCtx(&scopedViewer{tid: 1}), f); err != nil {
		t.Fatalf("soft delete rule: %v", err)
	}
	if len(f.preds) != 1 {
		t.Fatalf("soft delete rule must inject one predicate, got %d", len(f.preds))
	}
}

func TestDenyFieldsMutationRule(t *testing.T) {
	cli := newEvalClient(t)

	if err := rule.DenyFieldsMutationRule(context.Background(), cli.Menu.Create().Mutation(), "name"); err == nil {
		t.Error("missing viewer must fail-closed")
	}
	if err := rule.DenyFieldsMutationRule(scopedCtx(&scopedViewer{platform: true}), cli.Menu.Create().SetName("x").Mutation(), "name"); err != nil {
		t.Errorf("platform view must pass: %v", err)
	}

	// a set protected field is rejected
	if err := rule.DenyFieldsMutationRule(scopedCtx(&scopedViewer{tid: 1}), cli.Menu.Create().SetName("x").Mutation(), "name"); err == nil {
		t.Error("set protected field must be denied")
	}
	// a cleared protected field is rejected
	cleared := cli.Menu.Create().Mutation()
	cleared.ClearPath()
	if err := rule.DenyFieldsMutationRule(scopedCtx(&scopedViewer{tid: 1}), cleared, "path"); err == nil {
		t.Error("cleared protected field must be denied")
	}
	// untouched fields pass
	if err := rule.DenyFieldsMutationRule(scopedCtx(&scopedViewer{tid: 1}), cli.Menu.Create().Mutation(), "name", "path"); err != nil {
		t.Fatalf("untouched fields must pass: %v", err)
	}
}

func TestLimitFieldAccessRule(t *testing.T) {
	cli := newEvalClient(t)

	if err := rule.LimitFieldAccessRule(context.Background(), cli.Menu.Create().Mutation(), "name", "menu.write"); err == nil {
		t.Error("missing viewer must fail-closed")
	}
	if err := rule.LimitFieldAccessRule(scopedCtx(&scopedViewer{platform: true}), cli.Menu.Create().SetName("x").Mutation(), "name", "p"); err != nil {
		t.Errorf("platform view must pass: %v", err)
	}

	// without the permission, a set field is rejected
	if err := rule.LimitFieldAccessRule(scopedCtx(&scopedViewer{tid: 1}), cli.Menu.Create().SetName("x").Mutation(), "name", "menu.write"); err == nil {
		t.Error("set field without permission must be denied")
	}
	// a cleared field is rejected as well
	cleared2 := cli.Menu.Create().Mutation()
	cleared2.ClearPath()
	if err := rule.LimitFieldAccessRule(scopedCtx(&scopedViewer{tid: 1}), cleared2, "path", "menu.write"); err == nil {
		t.Error("cleared field without permission must be denied")
	}
	// untouched field without permission passes
	if err := rule.LimitFieldAccessRule(scopedCtx(&scopedViewer{tid: 1}), cli.Menu.Create().Mutation(), "name", "menu.write"); err != nil {
		t.Fatalf("untouched field must pass: %v", err)
	}
	// with the permission everything passes
	if err := rule.LimitFieldAccessRule(scopedCtx(&scopedViewer{tid: 1, allow: true}), cli.Menu.Create().SetName("x").Mutation(), "name", "menu.write"); err != nil {
		t.Fatalf("permitted viewer must pass: %v", err)
	}
}

func TestDenyIfNoViewer(t *testing.T) {
	r := rule.DenyIfNoViewer()

	if err := r.EvalQuery(context.Background(), nil); err == nil {
		t.Error("missing viewer must deny")
	}
	if err := r.EvalMutation(context.Background(), nil); err == nil {
		t.Error("missing viewer must deny")
	}
	if err := r.EvalQuery(scopedCtx(&scopedViewer{tid: 1}), nil); err != privacy.Skip {
		t.Errorf("present viewer must skip, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// rule.GetClientFromMutation reflection helpers
// ---------------------------------------------------------------------------

func TestGetClientFromMutation(t *testing.T) {
	if _, ok := rule.GetClientFromMutation(nil); ok {
		t.Error("nil mutation must fail")
	}

	cli := newEvalClient(t)
	if c, ok := rule.GetClientFromMutation(cli.Menu.Create().Mutation()); !ok || c == nil {
		t.Errorf("generated mutation must expose its client, ok=%v", ok)
	}

	if _, ok := rule.GetClientFromMutation(noClientMutation{}); ok {
		t.Error("mutation without a Client method must fail")
	}
	if _, ok := rule.GetClientFromMutation(voidClientMutation{}); ok {
		t.Error("Client without results must fail")
	}
	if _, ok := rule.GetClientFromMutation(voidClientMutation{}); ok {
		t.Error("Client without results must fail")
	}
	if _, ok := rule.GetClientFromMutation(argfulClientMutation{}); ok {
		t.Error("Client with arguments must fail")
	}
	if _, ok := rule.GetClientFromMutation(twoValueClientMutation{}); ok {
		t.Error("Client whose second result is not an error must fail")
	}
	if _, ok := rule.GetClientFromMutation(clientErrMutation{err: context.DeadlineExceeded}); ok {
		t.Error("Client returning an error must fail")
	}
	if v, ok := rule.GetClientFromMutation(clientErrMutation{}); !ok || v == nil {
		t.Error("Client returning nil error must succeed")
	}
	if v, ok := rule.GetClientFromMutation(plainClientMutation{}); !ok || v == nil {
		t.Error("single-result Client must succeed")
	}
}
