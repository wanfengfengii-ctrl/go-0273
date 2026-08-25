package store

import "fmt"

// schema is the full DDL for the acclimation-gate database. It is applied
// idempotently on startup. Unique indexes enforce the domain invariants that
// cannot be checked safely without a transaction: one active lease per
// resource key, one permit per task, and one idempotency result per operation.
var schema = []string{
	`CREATE TABLE IF NOT EXISTS mother_plants (
		id TEXT PRIMARY KEY,
		strain TEXT NOT NULL,
		lineage_id TEXT NOT NULL,
		enabled INTEGER NOT NULL,
		version INTEGER NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS medium_revisions (
		formula_id TEXT PRIMARY KEY,
		revision INTEGER NOT NULL,
		summary TEXT NOT NULL,
		effective_at INTEGER NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS rule_profiles (
		id TEXT PRIMARY KEY,
		vitrification_max_pct INTEGER NOT NULL,
		browning_max_pct INTEGER NOT NULL,
		root_viability_min INTEGER NOT NULL,
		virus_ct_max INTEGER NOT NULL,
		endophyte_cfu_max INTEGER NOT NULL,
		fixed_scale INTEGER NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS personnel (
		id TEXT PRIMARY KEY,
		qualifications TEXT NOT NULL,
		enabled INTEGER NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		state INTEGER NOT NULL,
		generation INTEGER NOT NULL,
		state_version INTEGER NOT NULL,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		final_type TEXT NOT NULL DEFAULT '',
		mother_plant_id TEXT NOT NULL DEFAULT '',
		lineage_id TEXT NOT NULL DEFAULT '',
		medium_formula_id TEXT NOT NULL DEFAULT '',
		medium_summary TEXT NOT NULL DEFAULT '',
		subculture_batch TEXT NOT NULL DEFAULT '',
		rack_shelf TEXT NOT NULL DEFAULT '',
		light_window TEXT NOT NULL DEFAULT '',
		acclimation_window TEXT NOT NULL DEFAULT '',
		thresholds TEXT NOT NULL DEFAULT '{}',
		allowed_personnel TEXT NOT NULL DEFAULT '[]',
		lock_summary TEXT NOT NULL DEFAULT ''
	)`,
	`CREATE TABLE IF NOT EXISTS task_bottles (
		task_id TEXT NOT NULL,
		seal TEXT NOT NULL,
		position TEXT NOT NULL,
		locked_seedlings INTEGER NOT NULL,
		PRIMARY KEY (task_id, seal)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_task_bottles_seal ON task_bottles(seal)`,
	`CREATE TABLE IF NOT EXISTS task_blind_codes (
		task_id TEXT NOT NULL,
		digest TEXT NOT NULL,
		bound_seal TEXT NOT NULL,
		PRIMARY KEY (task_id, digest)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_task_blind_digest ON task_blind_codes(digest)`,
	`CREATE TABLE IF NOT EXISTS task_root_points (
		task_id TEXT NOT NULL,
		point TEXT NOT NULL,
		PRIMARY KEY (task_id, point)
	)`,
	`CREATE TABLE IF NOT EXISTS task_vitrif_pos (
		task_id TEXT NOT NULL,
		pos TEXT NOT NULL,
		PRIMARY KEY (task_id, pos)
	)`,
	`CREATE TABLE IF NOT EXISTS task_rtqpcr_wells (
		task_id TEXT NOT NULL,
		well TEXT NOT NULL,
		PRIMARY KEY (task_id, well)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_task_rtqpcr_well ON task_rtqpcr_wells(well)`,
	`CREATE TABLE IF NOT EXISTS task_endophyte_wells (
		task_id TEXT NOT NULL,
		well TEXT NOT NULL,
		PRIMARY KEY (task_id, well)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_task_endo_well ON task_endophyte_wells(well)`,
	`CREATE TABLE IF NOT EXISTS operation_records (
		task_id TEXT NOT NULL,
		operation_id TEXT NOT NULL,
		request_digest TEXT NOT NULL,
		result_code TEXT NOT NULL,
		response BLOB NOT NULL,
		PRIMARY KEY (task_id, operation_id)
	)`,
	`CREATE TABLE IF NOT EXISTS subculture_confirmations (
		task_id TEXT NOT NULL,
		generation INTEGER NOT NULL,
		personnel_id TEXT NOT NULL,
		operation_id TEXT NOT NULL,
		confirmed_at INTEGER NOT NULL,
		PRIMARY KEY (task_id, personnel_id)
	)`,
	`CREATE TABLE IF NOT EXISTS independent_reviews (
		task_id TEXT NOT NULL,
		seq INTEGER NOT NULL,
		personnel_id TEXT NOT NULL,
		evidence_digest TEXT NOT NULL,
		conclusion TEXT NOT NULL,
		PRIMARY KEY (task_id, seq)
	)`,
	`CREATE TABLE IF NOT EXISTS final_decisions (
		task_id TEXT PRIMARY KEY,
		final_type TEXT NOT NULL,
		version INTEGER NOT NULL,
		reason TEXT NOT NULL,
		permit_serial TEXT NOT NULL,
		issued_at INTEGER NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS bottle_samples (
		task_id TEXT NOT NULL,
		seal TEXT NOT NULL,
		position TEXT NOT NULL,
		locked_seedlings INTEGER NOT NULL,
		triple_sealed INTEGER NOT NULL,
		PRIMARY KEY (task_id, seal)
	)`,
	`CREATE TABLE IF NOT EXISTS blind_codes (
		task_id TEXT NOT NULL,
		digest TEXT NOT NULL,
		bound_seal TEXT NOT NULL,
		revealed INTEGER NOT NULL,
		reveal_audit TEXT NOT NULL DEFAULT '[]',
		PRIMARY KEY (task_id, digest)
	)`,
	`CREATE TABLE IF NOT EXISTS resource_leases (
		resource_type TEXT NOT NULL,
		resource_key TEXT NOT NULL,
		task_id TEXT NOT NULL,
		generation INTEGER NOT NULL,
		version INTEGER NOT NULL,
		start_at INTEGER NOT NULL,
		end_at INTEGER NOT NULL,
		active INTEGER NOT NULL
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_lease_active ON resource_leases(resource_type, resource_key) WHERE active = 1`,
	`CREATE TABLE IF NOT EXISTS morphology_cells (
		task_id TEXT NOT NULL,
		position TEXT NOT NULL,
		normal INTEGER NOT NULL,
		vitrified INTEGER NOT NULL,
		browned INTEGER NOT NULL,
		contaminated INTEGER NOT NULL,
		scoring_plate_pos TEXT NOT NULL DEFAULT '',
		supplement INTEGER NOT NULL DEFAULT 0,
		version INTEGER NOT NULL,
		PRIMARY KEY (task_id, position, version)
	)`,
	`CREATE TABLE IF NOT EXISTS measurement_cells (
		task_id TEXT NOT NULL,
		position TEXT NOT NULL,
		point TEXT NOT NULL,
		metric TEXT NOT NULL,
		value INTEGER NOT NULL,
		scale INTEGER NOT NULL,
		version INTEGER NOT NULL,
		PRIMARY KEY (task_id, position, point, metric, version)
	)`,
	`CREATE TABLE IF NOT EXISTS device_calls (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		device_type TEXT NOT NULL,
		target TEXT NOT NULL,
		fault_step INTEGER NOT NULL,
		attempts INTEGER NOT NULL,
		status TEXT NOT NULL,
		next_retry_at INTEGER NOT NULL,
		error_code TEXT NOT NULL DEFAULT '',
		payload_digest TEXT NOT NULL DEFAULT '',
		blind_code TEXT NOT NULL DEFAULT '',
		detection_type TEXT NOT NULL DEFAULT '',
		well TEXT NOT NULL DEFAULT '',
		evidence_value INTEGER NOT NULL DEFAULT 0,
		evidence_scale INTEGER NOT NULL DEFAULT 0
	)`,
	`CREATE TABLE IF NOT EXISTS pathogen_evidence (
		task_id TEXT NOT NULL,
		blind_code TEXT NOT NULL,
		detection_type TEXT NOT NULL,
		well TEXT NOT NULL,
		value INTEGER NOT NULL,
		scale INTEGER NOT NULL,
		generation INTEGER NOT NULL,
		recheck_round INTEGER NOT NULL,
		version INTEGER NOT NULL,
		PRIMARY KEY (task_id, blind_code, detection_type, well, version)
	)`,
	`CREATE TABLE IF NOT EXISTS late_arrivals (
		task_id TEXT NOT NULL,
		blind_code TEXT NOT NULL,
		detection_type TEXT NOT NULL,
		generation INTEGER NOT NULL,
		reason TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS recheck_rounds (
		task_id TEXT NOT NULL,
		generation INTEGER NOT NULL,
		round INTEGER NOT NULL,
		trigger TEXT NOT NULL,
		affected_positions TEXT NOT NULL DEFAULT '[]',
		affected_blind_codes TEXT NOT NULL DEFAULT '[]',
		affected_points TEXT NOT NULL DEFAULT '[]',
		affected_wells TEXT NOT NULL DEFAULT '[]',
		status TEXT NOT NULL,
		evidence_digest TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (task_id, round)
	)`,
	`CREATE TABLE IF NOT EXISTS acclimation_permits (
		serial TEXT PRIMARY KEY,
		task_id TEXT NOT NULL UNIQUE,
		lock_digest TEXT NOT NULL,
		evidence_digest TEXT NOT NULL,
		reviewers TEXT NOT NULL DEFAULT '[]',
		issued_at INTEGER NOT NULL
	)`,
}

func (db *DB) migrate() error {
	for _, stmt := range schema {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
}
