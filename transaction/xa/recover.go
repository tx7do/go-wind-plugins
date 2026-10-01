package xa

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"strings"
)

// newXID mints a package-namespaced XID from [0-9a-f-] only, well under the
// 64-byte gtrid limit of MySQL.
func newXID() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // crypto/rand never fails on supported platforms
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%s%x-%x-%x-%x-%x", XIDPrefix, b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// Pending lists stranded XIDs minted by this package on one database: the
// in-doubt transactions left behind by a crash between XA PREPARE and
// XA COMMIT (or a commit that the coordinator could not confirm).
func Pending(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, "XA RECOVER")
	if err != nil {
		return nil, fmt.Errorf("xa: recover: %w", err)
	}
	defer rows.Close()

	var xids []string
	for rows.Next() {
		var (
			formatID         int
			gtridLen, bqualL int
			data             []byte
		)
		if err := rows.Scan(&formatID, &gtridLen, &bqualL, &data); err != nil {
			return nil, fmt.Errorf("xa: scan recover: %w", err)
		}
		xid := string(data)
		if strings.HasPrefix(xid, XIDPrefix) {
			xids = append(xids, xid)
		}
	}
	return xids, rows.Err()
}

// CommitPending commits a stranded transaction returned by Pending.
func CommitPending(ctx context.Context, db *sql.DB, xid string) error {
	return resolve(ctx, db, "XA COMMIT", xid)
}

// RollbackPending rolls back a stranded transaction returned by Pending.
func RollbackPending(ctx context.Context, db *sql.DB, xid string) error {
	return resolve(ctx, db, "XA ROLLBACK", xid)
}

func resolve(ctx context.Context, db *sql.DB, verb, xid string) error {
	if !validXID(xid) {
		return fmt.Errorf("xa: refusing foreign XID %q", xid)
	}
	if _, err := db.ExecContext(ctx, verb+" '"+xid+"'"); err != nil {
		return fmt.Errorf("xa: %s %s: %w", verb, xid, err)
	}
	return nil
}

// validXID accepts only package-minted XIDs: the prefix plus [0-9a-f-].
func validXID(xid string) bool {
	if !strings.HasPrefix(xid, XIDPrefix) {
		return false
	}
	for _, c := range xid[len(XIDPrefix):] {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c == '-':
		default:
			return false
		}
	}
	return len(xid) > len(XIDPrefix)
}
