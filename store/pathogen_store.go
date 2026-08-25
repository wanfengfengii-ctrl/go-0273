package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"strawberry-vitro-acclimation-gate/domain"
	"strawberry-vitro-acclimation-gate/pathogen"
)

// SaveDeviceCall inserts or replaces a device call record.
func (db *DB) SaveDeviceCall(ctx context.Context, c pathogen.DeviceCall) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO device_calls (id, task_id, device_type, target, fault_step, attempts, status, next_retry_at, error_code, payload_digest,
		   blind_code, detection_type, well, evidence_value, evidence_scale)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET attempts=excluded.attempts, status=excluded.status,
		   next_retry_at=excluded.next_retry_at, error_code=excluded.error_code, payload_digest=excluded.payload_digest,
		   fault_step=excluded.fault_step`,
		c.ID, string(c.TaskID), string(c.DeviceType), c.Target, c.FaultStep, c.Attempts,
		string(c.Status), int64(c.NextRetryAt), string(c.ErrorCode), c.PayloadDigest,
		c.BlindCode, c.DetectionType, c.Well, c.EvidenceValue, c.EvidenceScale)
	return err
}

// UpdateDeviceCall updates only mutable retry fields.
func (db *DB) UpdateDeviceCall(ctx context.Context, c pathogen.DeviceCall) error {
	return db.SaveDeviceCall(ctx, c)
}

// DeviceCall returns a device call by id.
func (db *DB) DeviceCall(ctx context.Context, id string) (pathogen.DeviceCall, error) {
	var c pathogen.DeviceCall
	err := db.QueryRowContext(ctx,
		`SELECT id, task_id, device_type, target, fault_step, attempts, status, next_retry_at, error_code, payload_digest,
		   blind_code, detection_type, well, evidence_value, evidence_scale
		 FROM device_calls WHERE id=?`, id).
		Scan(&c.ID, &c.TaskID, &c.DeviceType, &c.Target, &c.FaultStep, &c.Attempts, &c.Status,
			&c.NextRetryAt, &c.ErrorCode, &c.PayloadDigest, &c.BlindCode, &c.DetectionType, &c.Well,
			&c.EvidenceValue, &c.EvidenceScale)
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

// PendingRetries returns device calls that failed and are due for retry.
func (db *DB) PendingRetries(ctx context.Context, now domain.LogicalTime) ([]pathogen.DeviceCall, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, task_id, device_type, target, fault_step, attempts, status, next_retry_at, error_code, payload_digest,
		   blind_code, detection_type, well, evidence_value, evidence_scale
		 FROM device_calls WHERE status IN ('failed_retry','pending') AND next_retry_at <= ? ORDER BY next_retry_at, id`, int64(now))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []pathogen.DeviceCall
	for rows.Next() {
		var c pathogen.DeviceCall
		if err := rows.Scan(&c.ID, &c.TaskID, &c.DeviceType, &c.Target, &c.FaultStep, &c.Attempts, &c.Status,
			&c.NextRetryAt, &c.ErrorCode, &c.PayloadDigest, &c.BlindCode, &c.DetectionType, &c.Well,
			&c.EvidenceValue, &c.EvidenceScale); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// AppendEvidence appends an immutable pathogen evidence version.
func (db *DB) AppendEvidence(ctx context.Context, e pathogen.PathogenEvidence) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO pathogen_evidence (task_id, blind_code, detection_type, well, value, scale, generation, recheck_round, version)
		 VALUES (?,?,?,?,?,?,?,?,?)`,
		string(e.TaskID), e.BlindCode, e.DetectionType, e.Well, e.Value.Value, e.Value.Scale,
		int64(e.Generation), e.RecheckRound, e.Version)
	return err
}

// Evidence returns the versioned evidence chain for a blind code.
func (db *DB) Evidence(ctx context.Context, blindCode string) ([]pathogen.PathogenEvidence, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT task_id, blind_code, detection_type, well, value, scale, generation, recheck_round, version
		 FROM pathogen_evidence WHERE blind_code=? ORDER BY version`, blindCode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []pathogen.PathogenEvidence
	for rows.Next() {
		var e pathogen.PathogenEvidence
		if err := rows.Scan(&e.TaskID, &e.BlindCode, &e.DetectionType, &e.Well, &e.Value.Value, &e.Value.Scale,
			&e.Generation, &e.RecheckRound, &e.Version); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// EvidenceByTask returns all evidence for a task ordered deterministically.
func (db *DB) EvidenceByTask(ctx context.Context, id domain.TaskID) ([]pathogen.PathogenEvidence, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT task_id, blind_code, detection_type, well, value, scale, generation, recheck_round, version
		 FROM pathogen_evidence WHERE task_id=? ORDER BY blind_code, detection_type, well, version`, string(id))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []pathogen.PathogenEvidence
	for rows.Next() {
		var e pathogen.PathogenEvidence
		if err := rows.Scan(&e.TaskID, &e.BlindCode, &e.DetectionType, &e.Well, &e.Value.Value, &e.Value.Scale,
			&e.Generation, &e.RecheckRound, &e.Version); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// SaveLateArrival records a stale-generation arrival for audit.
func (db *DB) SaveLateArrival(ctx context.Context, a pathogen.LateArrival) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO late_arrivals (task_id, blind_code, detection_type, generation, reason) VALUES (?,?,?,?,?)`,
		string(a.TaskID), a.BlindCode, a.DetectionType, int64(a.Generation), a.Reason)
	return err
}

// CreateRecheckRound inserts the single active recheck round for a generation.
func (db *DB) CreateRecheckRound(ctx context.Context, r pathogen.RecheckRound) error {
	ap, _ := json.Marshal(r.AffectedPositions)
	ab, _ := json.Marshal(r.AffectedBlindCodes)
	apt, _ := json.Marshal(r.AffectedPoints)
	aw, _ := json.Marshal(r.AffectedWells)
	_, err := db.ExecContext(ctx,
		`INSERT INTO recheck_rounds (task_id, generation, round, trigger, affected_positions, affected_blind_codes, affected_points, affected_wells, status, evidence_digest)
		 VALUES (?,?,?,?,?,?,?,?,?,?)`,
		string(r.TaskID), int64(r.Generation), r.Round, r.Trigger, string(ap), string(ab), string(apt), string(aw), r.Status, r.EvidenceDigest)
	return err
}

// ActiveRecheck returns the active recheck round for a generation.
func (db *DB) ActiveRecheck(ctx context.Context, gen domain.Generation) (pathogen.RecheckRound, error) {
	var r pathogen.RecheckRound
	var ap, ab, apt, aw string
	err := db.QueryRowContext(ctx,
		`SELECT task_id, generation, round, trigger, affected_positions, affected_blind_codes, affected_points, affected_wells, status, evidence_digest
		 FROM recheck_rounds WHERE generation=? AND status='active'`, int64(gen)).
		Scan(&r.TaskID, &r.Generation, &r.Round, &r.Trigger, &ap, &ab, &apt, &aw, &r.Status, &r.EvidenceDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	if err != nil {
		return r, err
	}
	_ = json.Unmarshal([]byte(ap), &r.AffectedPositions)
	_ = json.Unmarshal([]byte(ab), &r.AffectedBlindCodes)
	_ = json.Unmarshal([]byte(apt), &r.AffectedPoints)
	_ = json.Unmarshal([]byte(aw), &r.AffectedWells)
	return r, nil
}

// UpdateRecheck closes a recheck round.
func (db *DB) UpdateRecheck(ctx context.Context, r pathogen.RecheckRound) error {
	ap, _ := json.Marshal(r.AffectedPositions)
	ab, _ := json.Marshal(r.AffectedBlindCodes)
	apt, _ := json.Marshal(r.AffectedPoints)
	aw, _ := json.Marshal(r.AffectedWells)
	_, err := db.ExecContext(ctx,
		`UPDATE recheck_rounds SET status=?, evidence_digest=?, affected_positions=?, affected_blind_codes=?, affected_points=?, affected_wells=?
		 WHERE task_id=? AND round=?`,
		r.Status, r.EvidenceDigest, string(ap), string(ab), string(apt), string(aw), string(r.TaskID), r.Round)
	return err
}

// SavePermit persists the unique acclimation permit.
func (db *DB) SavePermit(ctx context.Context, p pathogen.AcclimationPermit) error {
	rev, _ := json.Marshal(p.Reviewers)
	_, err := db.ExecContext(ctx,
		`INSERT INTO acclimation_permits (serial, task_id, lock_digest, evidence_digest, reviewers, issued_at)
		 VALUES (?,?,?,?,?,?)`,
		p.Serial, string(p.TaskID), p.LockDigest, p.EvidenceDigest, string(rev), int64(p.IssuedAt))
	return err
}

// Permit returns the acclimation permit for a task.
func (db *DB) Permit(ctx context.Context, taskID domain.TaskID) (pathogen.AcclimationPermit, error) {
	var p pathogen.AcclimationPermit
	var rev string
	err := db.QueryRowContext(ctx,
		`SELECT serial, task_id, lock_digest, evidence_digest, reviewers, issued_at FROM acclimation_permits WHERE task_id=?`,
		string(taskID)).
		Scan(&p.Serial, &p.TaskID, &p.LockDigest, &p.EvidenceDigest, &rev, &p.IssuedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	if err != nil {
		return p, err
	}
	_ = json.Unmarshal([]byte(rev), &p.Reviewers)
	return p, nil
}
