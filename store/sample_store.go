package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"strawberry-vitro-acclimation-gate/domain"
	"strawberry-vitro-acclimation-gate/sample"
)

// SealSample marks a bottle sample's triple seal as complete.
func (db *DB) SealSample(ctx context.Context, b sample.BottleSample) error {
	res, err := db.ExecContext(ctx,
		`UPDATE bottle_samples SET triple_sealed=1 WHERE task_id=? AND seal=? AND position=?`,
		b.TaskID, b.Seal, b.Position)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Sample returns a bottle sample row.
func (db *DB) Sample(ctx context.Context, seal string) (sample.BottleSample, error) {
	var b sample.BottleSample
	var sealed int
	err := db.QueryRowContext(ctx,
		`SELECT task_id, seal, position, locked_seedlings, triple_sealed FROM bottle_samples WHERE seal=?`, seal).
		Scan(&b.TaskID, &b.Seal, &b.Position, &b.LockedSeedlings, &sealed)
	if errors.Is(err, sql.ErrNoRows) {
		return b, ErrNotFound
	}
	if err != nil {
		return b, err
	}
	b.TripleSealed = sealed != 0
	return b, nil
}

// SamplesByTask returns all bottle samples for a task, ordered by seal.
func (db *DB) SamplesByTask(ctx context.Context, id domain.TaskID) ([]sample.BottleSample, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT task_id, seal, position, locked_seedlings, triple_sealed FROM bottle_samples WHERE task_id=? ORDER BY seal`, string(id))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []sample.BottleSample
	for rows.Next() {
		var b sample.BottleSample
		var sealed int
		if err := rows.Scan(&b.TaskID, &b.Seal, &b.Position, &b.LockedSeedlings, &sealed); err != nil {
			return nil, err
		}
		b.TripleSealed = sealed != 0
		out = append(out, b)
	}
	return out, rows.Err()
}

// BindBlindCode binds a blind code digest to a sample.
func (db *DB) BindBlindCode(ctx context.Context, c sample.BlindCode) error {
	_, err := db.ExecContext(ctx,
		`UPDATE blind_codes SET bound_seal=? WHERE task_id=? AND digest=?`,
		c.BoundSeal, c.TaskID, c.Digest)
	return err
}

// BlindCode returns a bound blind code by digest.
func (db *DB) BlindCode(ctx context.Context, digest string) (sample.BlindCode, error) {
	var c sample.BlindCode
	var revealed int
	var audit string
	err := db.QueryRowContext(ctx,
		`SELECT task_id, digest, bound_seal, revealed, reveal_audit FROM blind_codes WHERE digest=?`, digest).
		Scan(&c.TaskID, &c.Digest, &c.BoundSeal, &revealed, &audit)
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	if err != nil {
		return c, err
	}
	c.Revealed = revealed != 0
	if audit != "" {
		_ = json.Unmarshal([]byte(audit), &c.RevealAudit)
	}
	return c, nil
}

// BlindCodesByTask returns all blind codes for a task, ordered by digest.
func (db *DB) BlindCodesByTask(ctx context.Context, id domain.TaskID) ([]sample.BlindCode, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT task_id, digest, bound_seal, revealed, reveal_audit FROM blind_codes WHERE task_id=? ORDER BY digest`, string(id))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []sample.BlindCode
	for rows.Next() {
		var c sample.BlindCode
		var revealed int
		var audit string
		if err := rows.Scan(&c.TaskID, &c.Digest, &c.BoundSeal, &revealed, &audit); err != nil {
			return nil, err
		}
		c.Revealed = revealed != 0
		if audit != "" {
			_ = json.Unmarshal([]byte(audit), &c.RevealAudit)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Reveal marks a blind code revealed and appends an audit event.
func (db *DB) Reveal(ctx context.Context, digest string, ev sample.RevealEvent) error {
	c, err := db.BlindCode(ctx, digest)
	if err != nil {
		return err
	}
	if c.Revealed {
		return nil
	}
	audit := append(c.RevealAudit, ev)
	b, _ := json.Marshal(audit)
	_, err = db.ExecContext(ctx,
		`UPDATE blind_codes SET revealed=1, reveal_audit=? WHERE digest=?`, string(b), digest)
	return err
}

// AcquireLease inserts an active lease; the partial unique index rejects a
// second active lease on the same key.
func (db *DB) AcquireLease(ctx context.Context, l sample.ResourceLease) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO resource_leases (resource_type, resource_key, task_id, generation, version, start_at, end_at, active)
		 VALUES (?,?,?,?,?,?,?,1)`,
		string(l.ResourceType), l.ResourceKey, string(l.TaskID), int64(l.Generation), l.Version,
		int64(l.StartAt), int64(l.EndAt))
	if err != nil && strings.Contains(err.Error(), "UNIQUE") {
		return &domain.BusinessError{Code: domain.CodeDuplicateValue, Message: "resource already leased",
			Reasons: []domain.Reason{{Field: string(l.ResourceType), Detail: l.ResourceKey}}}
	}
	return err
}

// ReplaceLease deactivates any active lease on the key and inserts a new one
// with the incremented version, all within the caller's transaction.
func (db *DB) ReplaceLease(ctx context.Context, l sample.ResourceLease) error {
	return db.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`UPDATE resource_leases SET active=0 WHERE resource_type=? AND resource_key=? AND active=1`,
			string(l.ResourceType), l.ResourceKey); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO resource_leases (resource_type, resource_key, task_id, generation, version, start_at, end_at, active)
			 VALUES (?,?,?,?,?,?,?,1)`,
			string(l.ResourceType), l.ResourceKey, string(l.TaskID), int64(l.Generation), l.Version,
			int64(l.StartAt), int64(l.EndAt))
		return err
	})
}

// ReleaseLease deactivates the active lease on a key.
func (db *DB) ReleaseLease(ctx context.Context, resourceType sample.ResourceType, key string) error {
	_, err := db.ExecContext(ctx,
		`UPDATE resource_leases SET active=0 WHERE resource_type=? AND resource_key=? AND active=1`,
		string(resourceType), key)
	return err
}

// ActiveLease returns the active lease for a key, if any.
func (db *DB) ActiveLease(ctx context.Context, resourceType sample.ResourceType, key string) (sample.ResourceLease, error) {
	var l sample.ResourceLease
	var active int
	err := db.QueryRowContext(ctx,
		`SELECT resource_type, resource_key, task_id, generation, version, start_at, end_at, active
		 FROM resource_leases WHERE resource_type=? AND resource_key=? AND active=1`, string(resourceType), key).
		Scan(&l.ResourceType, &l.ResourceKey, &l.TaskID, &l.Generation, &l.Version, &l.StartAt, &l.EndAt, &active)
	if errors.Is(err, sql.ErrNoRows) {
		return l, ErrNotFound
	}
	if err != nil {
		return l, err
	}
	l.Active = active != 0
	return l, nil
}

// ActiveLeasesByTask returns all active leases held by a task.
func (db *DB) ActiveLeasesByTask(ctx context.Context, id domain.TaskID) ([]sample.ResourceLease, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT resource_type, resource_key, task_id, generation, version, start_at, end_at, active
		 FROM resource_leases WHERE task_id=? AND active=1 ORDER BY resource_type, resource_key`, string(id))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []sample.ResourceLease
	for rows.Next() {
		var l sample.ResourceLease
		var active int
		if err := rows.Scan(&l.ResourceType, &l.ResourceKey, &l.TaskID, &l.Generation, &l.Version, &l.StartAt, &l.EndAt, &active); err != nil {
			return nil, err
		}
		l.Active = active != 0
		out = append(out, l)
	}
	return out, rows.Err()
}
