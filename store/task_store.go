package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"strawberry-vitro-acclimation-gate/domain"
	"strawberry-vitro-acclimation-gate/task"
)

// terminalState is the first terminal task state; states below it are open.
const terminalState = int(domain.StateAcclimated)

// CreateTask persists a new task in the draft state together with its full
// snapshot child rows (bottles, blind codes, root points, plate positions, and
// wells). Everything is written in one transaction so a draft is never partial.
func (db *DB) CreateTask(ctx context.Context, t task.Task, snap task.Snapshot) error {
	return db.Tx(ctx, func(tx *sql.Tx) error {
		th, err := json.Marshal(snap.Thresholds)
		if err != nil {
			return err
		}
		ap, err := json.Marshal(snap.AllowedPersonnel)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO tasks (id, state, generation, state_version, created_at, updated_at, final_type,
			   mother_plant_id, lineage_id, medium_formula_id, medium_summary, subculture_batch, rack_shelf, light_window,
			   acclimation_window, thresholds, allowed_personnel, lock_summary)
			 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			string(t.ID), int(t.State), int64(t.Generation), t.StateVersion, int64(t.CreatedAt), int64(t.UpdatedAt),
			"", snap.MotherPlantID, snap.LineageID, snap.MediumFormulaID, snap.MediumSummary, snap.SubcultureBatch,
			snap.RackShelf, snap.LightWindow, snap.AcclimationWindow, string(th), string(ap), snap.LockSummary); err != nil {
			return err
		}
		if err := writeSnapshotChildren(ctx, tx, t.ID, snap); err != nil {
			return err
		}
		return nil
	})
}

// writeSnapshotChildren inserts the snapshot's child rows. It is shared by
// CreateTask so that the draft snapshot and the locked snapshot are identical.
func writeSnapshotChildren(ctx context.Context, tx *sql.Tx, id domain.TaskID, snap task.Snapshot) error {
	for i, seal := range snap.BottleSeals {
		pos := snap.BottlePositions[i]
		count := snap.LockedSeedlings[pos]
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO task_bottles (task_id, seal, position, locked_seedlings) VALUES (?,?,?,?)`,
			string(id), seal, pos, count); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO bottle_samples (task_id, seal, position, locked_seedlings, triple_sealed) VALUES (?,?,?,?,0)`,
			string(id), seal, pos, count); err != nil {
			return err
		}
	}
	for _, code := range snap.BlindCodes {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO task_blind_codes (task_id, digest, bound_seal) VALUES (?,?,?)`, string(id), code, ""); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO blind_codes (task_id, digest, bound_seal, revealed, reveal_audit) VALUES (?,?,?,0,'[]')`,
			string(id), code, ""); err != nil {
			return err
		}
	}
	for _, p := range snap.RootPoints {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO task_root_points (task_id, point) VALUES (?,?)`, string(id), p); err != nil {
			return err
		}
	}
	for _, p := range snap.VitrificationPos {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO task_vitrif_pos (task_id, pos) VALUES (?,?)`, string(id), p); err != nil {
			return err
		}
	}
	for _, w := range snap.RTQPCRWells {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO task_rtqpcr_wells (task_id, well) VALUES (?,?)`, string(id), w); err != nil {
			return err
		}
	}
	for _, w := range snap.EndophyteWells {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO task_endophyte_wells (task_id, well) VALUES (?,?)`, string(id), w); err != nil {
			return err
		}
	}
	return nil
}

// LockTask atomically checks uniqueness of the snapshot's identifiers among
// other open tasks and transitions the task from draft to pending-subculture.
// Any conflict rolls back the entire lock with no partial state.
func (db *DB) LockTask(ctx context.Context, id domain.TaskID, snap task.Snapshot) error {
	return db.Tx(ctx, func(tx *sql.Tx) error {
		if err := uniqueScalar(ctx, tx, "tasks", "subculture_batch", snap.SubcultureBatch, "subculture_batch", string(id)); err != nil {
			return err
		}
		for _, seal := range snap.BottleSeals {
			if err := uniqueScalar(ctx, tx, "task_bottles", "seal", seal, "bottle_seal", string(id)); err != nil {
				return err
			}
		}
		for _, code := range snap.BlindCodes {
			if err := uniqueScalar(ctx, tx, "task_blind_codes", "digest", code, "blind_code", string(id)); err != nil {
				return err
			}
		}
		if err := uniqueResource(ctx, tx, "rack_shelf", snap.RackShelf, "rack_shelf", string(id)); err != nil {
			return err
		}
		if err := uniqueResource(ctx, tx, "light_window", snap.LightWindow, "light_window", string(id)); err != nil {
			return err
		}
		if err := uniqueResource(ctx, tx, "acclimation_window", snap.AcclimationWindow, "acclimation_window", string(id)); err != nil {
			return err
		}
		for _, w := range snap.RTQPCRWells {
			if err := uniqueScalar(ctx, tx, "task_rtqpcr_wells", "well", w, "test_well", string(id)); err != nil {
				return err
			}
		}
		for _, w := range snap.EndophyteWells {
			if err := uniqueScalar(ctx, tx, "task_endophyte_wells", "well", w, "test_well", string(id)); err != nil {
				return err
			}
		}

		res, err := tx.ExecContext(ctx,
			`UPDATE tasks SET state=?, state_version=state_version+1, updated_at=updated_at+1 WHERE id=? AND state=?`,
			int(domain.StatePendingSubculture), string(id), int(domain.StateDraft))
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("lock: task not in draft state")
		}
		return nil
	})
}

func uniqueScalar(ctx context.Context, tx *sql.Tx, table, column, value, field, exclude string) error {
	if value == "" {
		return nil
	}
	var count int
	if table == "tasks" {
		err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM tasks WHERE `+column+`=? AND state < ? AND id <> ?`, value, terminalState, exclude).Scan(&count)
		if err != nil {
			return err
		}
	} else {
		err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM `+table+` c JOIN tasks t ON t.id = c.task_id
			 WHERE c.`+column+`=? AND t.state < ? AND t.id <> ?`, value, terminalState, exclude).Scan(&count)
		if err != nil {
			return err
		}
	}
	if count > 0 {
		return &domain.BusinessError{Code: domain.CodeDuplicateValue, Message: "duplicate " + field,
			Reasons: []domain.Reason{{Field: field, Detail: value}}}
	}
	return nil
}

func uniqueResource(ctx context.Context, tx *sql.Tx, column, value, field, exclude string) error {
	if value == "" {
		return nil
	}
	var count int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tasks WHERE `+column+`=? AND state < ? AND id <> ?`, value, terminalState, exclude).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return &domain.BusinessError{Code: domain.CodeDuplicateValue, Message: "resource conflict",
			Reasons: []domain.Reason{{Field: field, Detail: value}}}
	}
	return nil
}

// Get returns the live task aggregate.
func (db *DB) Get(ctx context.Context, id domain.TaskID) (task.Task, error) {
	var t task.Task
	var state int
	var final string
	err := db.QueryRowContext(ctx,
		`SELECT id, state, generation, state_version, created_at, updated_at, final_type
		 FROM tasks WHERE id = ?`, string(id)).
		Scan(&t.ID, &state, &t.Generation, &t.StateVersion, &t.CreatedAt, &t.UpdatedAt, &final)
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrNotFound
	}
	if err != nil {
		return t, err
	}
	t.State = domain.TaskState(state)
	t.Final = domain.FinalType(final)
	return t, nil
}

// Snapshot reconstructs the immutable snapshot from the locked child rows.
func (db *DB) Snapshot(ctx context.Context, id domain.TaskID) (task.Snapshot, error) {
	var s task.Snapshot
	s.TaskID = id
	var th, ap string
	err := db.QueryRowContext(ctx,
		`SELECT mother_plant_id, lineage_id, medium_formula_id, medium_summary, subculture_batch,
		   rack_shelf, light_window, acclimation_window, thresholds, allowed_personnel,
		   lock_summary, generation FROM tasks WHERE id = ?`, string(id)).
		Scan(&s.MotherPlantID, &s.LineageID, &s.MediumFormulaID, &s.MediumSummary, &s.SubcultureBatch,
			&s.RackShelf, &s.LightWindow, &s.AcclimationWindow, &th, &ap, &s.LockSummary, &s.Generation)
	if errors.Is(err, sql.ErrNoRows) {
		return s, ErrNotFound
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal([]byte(th), &s.Thresholds); err != nil {
		return s, err
	}
	if err := json.Unmarshal([]byte(ap), &s.AllowedPersonnel); err != nil {
		return s, err
	}
	s.LockedSeedlings = map[string]int{}
	type sealPos struct {
		seal, pos string
		count     int
	}
	var sps []sealPos
	rows, err := db.QueryContext(ctx, `SELECT seal, position, locked_seedlings FROM task_bottles WHERE task_id=? ORDER BY seal`, string(id))
	if err != nil {
		return s, err
	}
	for rows.Next() {
		var sp sealPos
		if err := rows.Scan(&sp.seal, &sp.pos, &sp.count); err != nil {
			rows.Close()
			return s, err
		}
		sps = append(sps, sp)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return s, err
	}
	for _, sp := range sps {
		s.BottleSeals = append(s.BottleSeals, sp.seal)
		s.BottlePositions = append(s.BottlePositions, sp.pos)
		s.LockedSeedlings[sp.pos] = sp.count
	}
	if s.BlindCodes, err = db.stringColumn(ctx, `SELECT digest FROM task_blind_codes WHERE task_id=? ORDER BY digest`, string(id)); err != nil {
		return s, err
	}
	if s.RootPoints, err = db.stringColumn(ctx, `SELECT point FROM task_root_points WHERE task_id=? ORDER BY point`, string(id)); err != nil {
		return s, err
	}
	if s.VitrificationPos, err = db.stringColumn(ctx, `SELECT pos FROM task_vitrif_pos WHERE task_id=? ORDER BY pos`, string(id)); err != nil {
		return s, err
	}
	if s.RTQPCRWells, err = db.stringColumn(ctx, `SELECT well FROM task_rtqpcr_wells WHERE task_id=? ORDER BY well`, string(id)); err != nil {
		return s, err
	}
	if s.EndophyteWells, err = db.stringColumn(ctx, `SELECT well FROM task_endophyte_wells WHERE task_id=? ORDER BY well`, string(id)); err != nil {
		return s, err
	}
	return s, nil
}

func (db *DB) stringColumn(ctx context.Context, q, arg string) ([]string, error) {
	rows, err := db.QueryContext(ctx, q, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// SetState conditionally advances the task state using a state_version check so
// that concurrent writers cannot both win; only one write succeeds.
func (db *DB) SetState(ctx context.Context, id domain.TaskID, from, to domain.TaskState) error {
	return db.Tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE tasks SET state=?, state_version=state_version+1, updated_at=updated_at+1 WHERE id=? AND state=?`,
			int(to), string(id), int(from))
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("state transition failed: task not in expected state")
		}
		return nil
	})
}

// FindOperation returns the persisted idempotency result for an operation.
func (db *DB) FindOperation(ctx context.Context, id domain.TaskID, op domain.OperationID) (task.OperationRecord, bool, error) {
	var rec task.OperationRecord
	var code string
	err := db.QueryRowContext(ctx,
		`SELECT task_id, operation_id, request_digest, result_code, response
		 FROM operation_records WHERE task_id=? AND operation_id=?`, string(id), string(op)).
		Scan(&rec.TaskID, &rec.OperationID, &rec.RequestDigest, &code, &rec.Response)
	if errors.Is(err, sql.ErrNoRows) {
		return rec, false, nil
	}
	if err != nil {
		return rec, false, err
	}
	rec.ResultCode = domain.ErrorCode(code)
	return rec, true, nil
}

// SaveOperation persists an idempotency result.
func (db *DB) SaveOperation(ctx context.Context, rec task.OperationRecord) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO operation_records (task_id, operation_id, request_digest, result_code, response)
		 VALUES (?,?,?,?,?)
		 ON CONFLICT(task_id, operation_id) DO NOTHING`,
		string(rec.TaskID), string(rec.OperationID), rec.RequestDigest, string(rec.ResultCode), rec.Response)
	return err
}
