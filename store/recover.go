package store

import (
	"context"

	"strawberry-vitro-acclimation-gate/domain"
)

// RecoveryReport summarizes what startup recovery re-loaded from the database.
// Recovery never auto-advances state, never auto-reveals blind codes, never
// fabricates a passing reading, and never issues a permit; it only re-reads
// persisted facts so the process continues deterministically after a restart.
type RecoveryReport struct {
	OpenTasks     int
	ActiveLeases  int
	PendingCalls  int
	RevealedCodes int
	Permits       int
}

// Recover re-establishes the in-memory readiness state from the database. All
// real state lives in SQLite, so every request already reads persisted data;
// this method exists to prove recovery and to surface a readiness summary.
func (db *DB) Recover(ctx context.Context, now domain.LogicalTime) (RecoveryReport, error) {
	var rep RecoveryReport
	tasks, err := db.OpenTasks(ctx)
	if err != nil {
		return rep, err
	}
	rep.OpenTasks = len(tasks)

	leases, err := db.QueryContext(ctx, `SELECT COUNT(*) FROM resource_leases WHERE active=1`)
	if err != nil {
		return rep, err
	}
	defer leases.Close()
	if leases.Next() {
		_ = leases.Scan(&rep.ActiveLeases)
	}

	calls, err := db.PendingRetries(ctx, now)
	if err != nil {
		return rep, err
	}
	rep.PendingCalls = len(calls)

	var revealed, permits int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM blind_codes WHERE revealed=1`).Scan(&revealed); err != nil {
		return rep, err
	}
	rep.RevealedCodes = revealed
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM acclimation_permits`).Scan(&permits); err != nil {
		return rep, err
	}
	rep.Permits = permits
	return rep, nil
}
