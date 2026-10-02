package pagination_test

import (
	"fmt"

	"github.com/tx7do/go-wind-plugins/crud/pagination"
)

// ExampleEncodeAndSign round-trips a signed pagination cursor token. In an
// application's data access layer keyset pagination encodes the last row
// identifier of a page into an opaque token handed to the client; on the next
// page request the token is verified and decoded so the scan resumes after
// that row.
func ExampleEncodeAndSign() {
	secret := []byte("placeholder-signing-secret-32-by")
	tok := pagination.EncodeAndSign(42, secret)

	lastID, ok := pagination.VerifyAndDecode(tok, secret)
	fmt.Println(lastID, ok)
	// Output: 42 true
}

type treeRow struct {
	Id       *uint32
	ParentId *uint32
	Children []*treeRow
}

// ExampleBuildTree reassembles a hierarchy from a flat list of records. In an
// application's data access layer, parent-child rows fetched as a flat result
// set are rebuilt into trees for hierarchical display; the accessor callbacks
// expose the identifier, parent identifier and children fields of the row
// type.
func ExampleBuildTree() {
	rootID := uint32(1)
	childID := uint32(2)
	rows := []*treeRow{
		{Id: &rootID},
		{Id: &childID, ParentId: &rootID},
	}

	_ = pagination.BuildTree(
		rows,
		func(row *treeRow) *uint32 { return row.Id },
		func(row *treeRow) *uint32 { return row.ParentId },
		func(row *treeRow) *[]*treeRow { return &row.Children },
	)
}
