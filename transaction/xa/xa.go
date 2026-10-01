// Package xa coordinates a two-phase commit across multiple databases using
// the XA protocol (MySQL / MariaDB). Unlike transaction/outbox (eventual
// consistency) or transaction/tcc (business-level reservations), XA commits
// or aborts every resource atomically at the database level — strong
// consistency at the cost of blocking locks and lower throughput. Use it
// only when a short, small-fanout write absolutely must be atomic.
//
//	err := xa.Run(ctx,
//	    func(ctx context.Context, s *xa.Session) error {
//	        if _, err := s.Exec(ctx, "orders",
//	            "UPDATE orders SET paid = 1 WHERE id = ?", "o-1"); err != nil {
//	            return err
//	        }
//	        _, err := s.Exec(ctx, "stocks",
//	            "UPDATE stocks SET locked = locked + 1 WHERE sku = ?", "sku-1")
//	        return err
//	    },
//	    xa.Resource{Name: "orders", DB: orderDB},
//	    xa.Resource{Name: "stocks", DB: stockDB},
//	)
//
// Protocol flow per Run: XA START on every resource, business SQL, XA END,
// then XA PREPARE on every resource; only if all prepares succeed are the
// transactions XA COMMITted. Any earlier failure rolls back everything.
//
// A crash between PREPARE and COMMIT strands prepared transactions. They are
// visible with Pending and resolvable with CommitPending / RollbackPending;
// all XIDs minted by this package carry the XIDPrefix so operators can
// distinguish them from foreign transactions.
package xa

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// XIDPrefix marks every XID minted by this package (used by Pending).
const XIDPrefix = "windxa-"

// Resource is one participating database.
type Resource struct {
	// Name keys the resource inside Session; must be unique per Run.
	Name string
	// DB is the participating database handle.
	DB *sql.DB
}

// Run executes fn against every resource inside one XA transaction and
// commits atomically: all resources commit, or none do.
func Run(ctx context.Context, fn func(ctx context.Context, s *Session) error, resources ...Resource) error {
	if len(resources) == 0 {
		return errors.New("xa: no resources")
	}
	seen := make(map[string]bool, len(resources))
	for _, r := range resources {
		if r.Name == "" || r.DB == nil {
			return fmt.Errorf("xa: resource %q missing name or DB", r.Name)
		}
		if seen[r.Name] {
			return fmt.Errorf("xa: duplicate resource %q", r.Name)
		}
		seen[r.Name] = true
	}

	xid := newXID()
	state := make([]*xaRes, len(resources))
	defer func() {
		for _, r := range state {
			if r != nil && r.conn != nil {
				_ = r.conn.Close()
			}
		}
	}()

	// Phase 0: bind a connection per resource and start its XA branch.
	for i, res := range resources {
		conn, err := res.DB.Conn(ctx)
		if err != nil {
			return fmt.Errorf("xa: acquire %q: %w", res.Name, err)
		}
		r := &xaRes{Resource: res, conn: conn}
		state[i] = r
		if err := r.exec(ctx, "XA START", xid); err != nil {
			rollbackAll(ctx, state[:i+1], xid)
			return fmt.Errorf("xa: start %q: %w", res.Name, err)
		}
		r.phase = phaseStarted
	}

	// Business phase.
	s := &Session{byName: make(map[string]*xaRes, len(state))}
	for _, r := range state {
		s.byName[r.Name] = r
	}
	if err := fn(ctx, s); err != nil {
		rollbackAll(ctx, state, xid)
		return fmt.Errorf("xa: business: %w", err)
	}

	// Phase 1: end all branches.
	for _, r := range state {
		if err := r.exec(ctx, "XA END", xid); err != nil {
			rollbackAll(ctx, state, xid)
			return fmt.Errorf("xa: end %q: %w", r.Name, err)
		}
		r.phase = phaseEnded
	}

	// Phase 2: prepare all branches; any refusal rolls everything back.
	for _, r := range state {
		if err := r.exec(ctx, "XA PREPARE", xid); err != nil {
			rollbackAll(ctx, state, xid)
			return fmt.Errorf("xa: prepare %q: %w", r.Name, err)
		}
		r.phase = phasePrepared
	}

	// Phase 3: commit all branches. A failure here is the in-doubt case:
	// the branch stays prepared in the database and must be resolved via
	// Pending / CommitPending / RollbackPending.
	var errs []error
	for _, r := range state {
		if err := r.exec(ctx, "XA COMMIT", xid); err != nil {
			r.phase = phaseInDoubt
			errs = append(errs, fmt.Errorf("xa: commit %q: %w", r.Name, err))
			continue
		}
		r.phase = phaseCommitted
	}
	return errors.Join(errs...)
}

const (
	phaseNew = iota
	phaseStarted
	phaseEnded
	phasePrepared
	phaseCommitted
	phaseInDoubt
	phaseRolledBack
)

type xaRes struct {
	Resource
	conn  *sql.Conn
	phase int
}

func (r *xaRes) exec(ctx context.Context, verb, xid string) error {
	// The XID is minted by this package from [0-9a-f-] only, so
	// interpolation is safe; XA statements do not accept placeholders.
	_, err := r.conn.ExecContext(ctx, verb+" '"+xid+"'")
	return err
}

// rollbackAll ends and rolls back every branch that is still rollbackable.
// Started branches must be ended before rollback; prepared branches accept
// XA ROLLBACK directly.
func rollbackAll(ctx context.Context, state []*xaRes, xid string) {
	for _, r := range state {
		if r == nil || r.conn == nil {
			continue
		}
		switch r.phase {
		case phaseStarted:
			_ = r.exec(ctx, "XA END", xid)
			_ = r.exec(ctx, "XA ROLLBACK", xid)
		case phaseEnded, phasePrepared:
			_ = r.exec(ctx, "XA ROLLBACK", xid)
		}
		r.phase = phaseRolledBack
	}
}

// Session exposes the business phase: statements on a named resource run on
// the connection that owns its XA branch.
type Session struct {
	byName map[string]*xaRes
}

// Conn returns the connection bound to the named resource's XA branch, for
// queries and statements beyond Exec.
func (s *Session) Conn(name string) (*sql.Conn, error) {
	r, ok := s.byName[name]
	if !ok {
		return nil, fmt.Errorf("xa: unknown resource %q", name)
	}
	return r.conn, nil
}

// Exec runs a statement on the named resource's XA branch.
func (s *Session) Exec(ctx context.Context, name, query string, args ...any) (sql.Result, error) {
	conn, err := s.Conn(name)
	if err != nil {
		return nil, err
	}
	return conn.ExecContext(ctx, query, args...)
}
