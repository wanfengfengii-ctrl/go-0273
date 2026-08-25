package store

import (
	"context"
	"database/sql"

	"strawberry-vitro-acclimation-gate/domain"
	"strawberry-vitro-acclimation-gate/measure"
)

// SaveMorphologyBatch atomically inserts a batch of morphology cells.
func (db *DB) SaveMorphologyBatch(ctx context.Context, cells []measure.MorphologyCell) error {
	return db.Tx(ctx, func(tx *sql.Tx) error {
		for _, c := range cells {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO morphology_cells (task_id, position, normal, vitrified, browned, contaminated, scoring_plate_pos, supplement, version)
				 VALUES (?,?,?,?,?,?,?,?,?)`,
				string(c.TaskID), c.Position, c.Normal, c.Vitrified, c.Browned, c.Contaminated,
				c.ScoringPlatePos, boolInt(c.Supplement), c.Version); err != nil {
				return err
			}
		}
		return nil
	})
}

// MorphologyCells returns all morphology cells for a task ordered by position.
func (db *DB) MorphologyCells(ctx context.Context, id domain.TaskID) ([]measure.MorphologyCell, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT task_id, position, normal, vitrified, browned, contaminated, scoring_plate_pos, supplement, version
		 FROM morphology_cells WHERE task_id=? ORDER BY position, version`, string(id))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []measure.MorphologyCell
	for rows.Next() {
		var c measure.MorphologyCell
		var sup int
		if err := rows.Scan(&c.TaskID, &c.Position, &c.Normal, &c.Vitrified, &c.Browned, &c.Contaminated,
			&c.ScoringPlatePos, &sup, &c.Version); err != nil {
			return nil, err
		}
		c.Supplement = sup != 0
		out = append(out, c)
	}
	return out, rows.Err()
}

// SaveMeasurementBatch atomically inserts a batch of measurement cells.
func (db *DB) SaveMeasurementBatch(ctx context.Context, cells []measure.MeasurementCell) error {
	return db.Tx(ctx, func(tx *sql.Tx) error {
		for _, c := range cells {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO measurement_cells (task_id, position, point, metric, value, scale, version)
				 VALUES (?,?,?,?,?,?,?)`,
				string(c.TaskID), c.Position, c.Point, string(c.Metric), c.Value.Value, c.Value.Scale, c.Version); err != nil {
				return err
			}
		}
		return nil
	})
}

// MeasurementCells returns all measurement cells for a task.
func (db *DB) MeasurementCells(ctx context.Context, id domain.TaskID) ([]measure.MeasurementCell, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT task_id, position, point, metric, value, scale, version
		 FROM measurement_cells WHERE task_id=? ORDER BY position, point, metric, version`, string(id))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []measure.MeasurementCell
	for rows.Next() {
		var c measure.MeasurementCell
		if err := rows.Scan(&c.TaskID, &c.Position, &c.Point, &c.Metric, &c.Value.Value, &c.Value.Scale, &c.Version); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
