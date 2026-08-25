package store

import (
	"context"
	"database/sql"
	"errors"

	"strawberry-vitro-acclimation-gate/domain"
	"strawberry-vitro-acclimation-gate/task"
)

// AddSubcultureConfirmation inserts a subculture confirmation; the primary key
// (task_id, personnel_id) prevents the same person confirming twice.
func (db *DB) AddSubcultureConfirmation(ctx context.Context, c task.SubcultureConfirmation) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO subculture_confirmations (task_id, generation, personnel_id, operation_id, confirmed_at)
		 VALUES (?,?,?,?,?)`,
		string(c.TaskID), int64(c.Generation), c.PersonnelID, string(c.OperationID), int64(c.ConfirmedAt))
	return err
}

// SubcultureConfirmations returns all confirmations for a task.
func (db *DB) SubcultureConfirmations(ctx context.Context, id domain.TaskID) ([]task.SubcultureConfirmation, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT task_id, generation, personnel_id, operation_id, confirmed_at
		 FROM subculture_confirmations WHERE task_id=? ORDER BY confirmed_at, personnel_id`, string(id))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []task.SubcultureConfirmation
	for rows.Next() {
		var c task.SubcultureConfirmation
		if err := rows.Scan(&c.TaskID, &c.Generation, &c.PersonnelID, &c.OperationID, &c.ConfirmedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// AddIndependentReview inserts an independent review.
func (db *DB) AddIndependentReview(ctx context.Context, r task.IndependentReview) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO independent_reviews (task_id, seq, personnel_id, evidence_digest, conclusion)
		 VALUES (?,?,?,?,?)`,
		string(r.TaskID), r.Seq, r.PersonnelID, r.EvidenceDigest, r.Conclusion)
	return err
}

// IndependentReviews returns all reviews for a task ordered by seq.
func (db *DB) IndependentReviews(ctx context.Context, id domain.TaskID) ([]task.IndependentReview, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT seq, personnel_id, evidence_digest, conclusion FROM independent_reviews WHERE task_id=? ORDER BY seq`, string(id))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []task.IndependentReview
	for rows.Next() {
		var r task.IndependentReview
		r.TaskID = id
		if err := rows.Scan(&r.Seq, &r.PersonnelID, &r.EvidenceDigest, &r.Conclusion); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SaveFinalDecision persists the single terminal decision.
func (db *DB) SaveFinalDecision(ctx context.Context, d task.FinalDecision) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO final_decisions (task_id, final_type, version, reason, permit_serial, issued_at)
		 VALUES (?,?,?,?,?,?)`,
		string(d.TaskID), string(d.FinalType), d.Version, d.Reason, d.PermitSerial, int64(d.IssuedAt))
	return err
}

// FinalDecision returns the terminal decision for a task.
func (db *DB) FinalDecision(ctx context.Context, id domain.TaskID) (task.FinalDecision, error) {
	var d task.FinalDecision
	err := db.QueryRowContext(ctx,
		`SELECT task_id, final_type, version, reason, permit_serial, issued_at FROM final_decisions WHERE task_id=?`, string(id)).
		Scan(&d.TaskID, &d.FinalType, &d.Version, &d.Reason, &d.PermitSerial, &d.IssuedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return d, ErrNotFound
	}
	return d, err
}

// SetFinal atomically writes the terminal state and final type using a
// conditional state_version update, forming the single-writer barrier: only one
// of several concurrent finalize requests can win.
func (db *DB) SetFinal(ctx context.Context, id domain.TaskID, from domain.TaskState, to domain.TaskState, final domain.FinalType) (bool, error) {
	var won bool
	err := db.Tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE tasks SET state=?, final_type=?, state_version=state_version+1, updated_at=updated_at+1
			 WHERE id=? AND state=?`,
			int(to), string(final), string(id), int(from))
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		won = n > 0
		return nil
	})
	return won, err
}

// OpenTasks returns all non-terminal tasks for startup recovery.
func (db *DB) OpenTasks(ctx context.Context) ([]task.Task, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, state, generation, state_version, created_at, updated_at, final_type
		 FROM tasks WHERE state < ? ORDER BY id`, terminalState)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []task.Task
	for rows.Next() {
		var t task.Task
		var state int
		var final string
		if err := rows.Scan(&t.ID, &state, &t.Generation, &t.StateVersion, &t.CreatedAt, &t.UpdatedAt, &final); err != nil {
			return nil, err
		}
		t.State = domain.TaskState(state)
		t.Final = domain.FinalType(final)
		out = append(out, t)
	}
	return out, rows.Err()
}
