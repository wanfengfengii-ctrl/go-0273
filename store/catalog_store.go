package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"strawberry-vitro-acclimation-gate/catalog"
)

// ErrNotFound is returned when a catalog entry or task is absent.
var ErrNotFound = errors.New("not found")

// UpsertMother inserts or replaces a mother plant in the catalog.
func (db *DB) UpsertMother(ctx context.Context, m catalog.MotherPlant) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO mother_plants (id, strain, lineage_id, enabled, version)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET strain=excluded.strain, lineage_id=excluded.lineage_id,
		   enabled=excluded.enabled, version=excluded.version`,
		m.ID, m.Strain, m.LineageID, boolInt(m.Enabled), m.Version)
	return err
}

// Mother returns a cataloged mother plant by id.
func (db *DB) Mother(ctx context.Context, id string) (catalog.MotherPlant, error) {
	var m catalog.MotherPlant
	var enabled int
	err := db.QueryRowContext(ctx,
		`SELECT id, strain, lineage_id, enabled, version FROM mother_plants WHERE id = ?`, id).
		Scan(&m.ID, &m.Strain, &m.LineageID, &enabled, &m.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return m, ErrNotFound
	}
	if err != nil {
		return m, err
	}
	m.Enabled = enabled != 0
	return m, nil
}

// UpsertMedium inserts or replaces a medium formula revision.
func (db *DB) UpsertMedium(ctx context.Context, m catalog.MediumRevision) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO medium_revisions (formula_id, revision, summary, effective_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(formula_id) DO UPDATE SET revision=excluded.revision,
		   summary=excluded.summary, effective_at=excluded.effective_at`,
		m.FormulaID, m.Revision, m.Summary, m.EffectiveAt)
	return err
}

// Medium returns the current medium revision for a formula.
func (db *DB) Medium(ctx context.Context, formulaID string) (catalog.MediumRevision, error) {
	var m catalog.MediumRevision
	err := db.QueryRowContext(ctx,
		`SELECT formula_id, revision, summary, effective_at FROM medium_revisions WHERE formula_id = ?`, formulaID).
		Scan(&m.FormulaID, &m.Revision, &m.Summary, &m.EffectiveAt)
	if errors.Is(err, sql.ErrNoRows) {
		return m, ErrNotFound
	}
	return m, err
}

// UpsertRuleProfile inserts or replaces a rule profile.
func (db *DB) UpsertRuleProfile(ctx context.Context, r catalog.RuleProfile) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO rule_profiles (id, vitrification_max_pct, browning_max_pct,
		   root_viability_min, virus_ct_max, endophyte_cfu_max, fixed_scale)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET vitrification_max_pct=excluded.vitrification_max_pct,
		   browning_max_pct=excluded.browning_max_pct, root_viability_min=excluded.root_viability_min,
		   virus_ct_max=excluded.virus_ct_max, endophyte_cfu_max=excluded.endophyte_cfu_max,
		   fixed_scale=excluded.fixed_scale`,
		r.ID, r.VitrificationMaxPct, r.BrowningMaxPct, r.RootViabilityMin,
		r.VirusCtMax, r.EndophyteCFUMax, r.FixedScale)
	return err
}

// RuleProfile returns a rule profile by id.
func (db *DB) RuleProfile(ctx context.Context, id string) (catalog.RuleProfile, error) {
	var r catalog.RuleProfile
	err := db.QueryRowContext(ctx,
		`SELECT id, vitrification_max_pct, browning_max_pct, root_viability_min,
		   virus_ct_max, endophyte_cfu_max, fixed_scale FROM rule_profiles WHERE id = ?`, id).
		Scan(&r.ID, &r.VitrificationMaxPct, &r.BrowningMaxPct, &r.RootViabilityMin,
			&r.VirusCtMax, &r.EndophyteCFUMax, &r.FixedScale)
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

// UpsertPersonnel inserts or replaces a personnel record.
func (db *DB) UpsertPersonnel(ctx context.Context, p catalog.Personnel) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO personnel (id, qualifications, enabled) VALUES (?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET qualifications=excluded.qualifications, enabled=excluded.enabled`,
		p.ID, strings.Join(p.Qualifications, ","), boolInt(p.Enabled))
	return err
}

// Personnel returns a personnel record by id.
func (db *DB) Personnel(ctx context.Context, id string) (catalog.Personnel, error) {
	var p catalog.Personnel
	var quals string
	var enabled int
	err := db.QueryRowContext(ctx,
		`SELECT id, qualifications, enabled FROM personnel WHERE id = ?`, id).
		Scan(&p.ID, &quals, &enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	if err != nil {
		return p, err
	}
	if quals != "" {
		p.Qualifications = strings.Split(quals, ",")
	}
	p.Enabled = enabled != 0
	return p, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
