// Package catalog implements the strawberry mother-plant and tissue-culture
// rule catalog: mother plants, fixed detoxification lineages, medium formula
// revisions, threshold profiles, and personnel qualifications. Locking a task
// snapshots this catalog; later catalog edits do not change locked tasks.
package catalog

// MotherPlant is a cataloged mother plant with a fixed detoxification lineage.
type MotherPlant struct {
	ID        string `json:"id"`
	Strain    string `json:"strain"`
	LineageID string `json:"lineage_id"`
	Enabled   bool   `json:"enabled"`
	Version   int64  `json:"version"`
}

// MediumRevision is one revision of a medium formula with a normalized summary.
type MediumRevision struct {
	FormulaID   string `json:"formula_id"`
	Revision    int64  `json:"revision"`
	Summary     string `json:"summary"`
	EffectiveAt int64  `json:"effective_at"`
}

// RuleProfile holds morphology, root, physicochemical and pathogen thresholds
// plus the fixed-point precision used for each reading.
type RuleProfile struct {
	ID                  string `json:"id"`
	VitrificationMaxPct int    `json:"vitrification_max_pct"`
	BrowningMaxPct      int    `json:"browning_max_pct"`
	RootViabilityMin    int64  `json:"root_viability_min"`
	VirusCtMax          int64  `json:"virus_ct_max"`
	EndophyteCFUMax     int64  `json:"endophyte_cfu_max"`
	FixedScale          int    `json:"fixed_scale"`
}

// Personnel is a qualified operator with an eligibility set.
type Personnel struct {
	ID             string   `json:"id"`
	Qualifications []string `json:"qualifications"`
	Enabled        bool     `json:"enabled"`
}
