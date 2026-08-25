// Package task implements the acclimation task aggregate: the twelve-state
// machine, task generations, the one-shot lock gate, dual-person subculture
// confirmation, independent review, and the single-writer terminal barrier.
package task

import (
	"strawberry-vitro-acclimation-gate/domain"
)

// Snapshot is the immutable, locked view of a task captured at lock time.
type Snapshot struct {
	TaskID            domain.TaskID
	MotherPlantID     string
	LineageID         string
	MediumFormulaID   string
	MediumSummary     string
	SubcultureBatch   string
	BottleSeals       []string
	BottlePositions   []string
	LockedSeedlings   map[string]int
	BlindCodes        []string
	RootPoints        []string
	VitrificationPos  []string
	RTQPCRWells       []string
	EndophyteWells    []string
	RackShelf         string
	LightWindow       string
	AcclimationWindow string
	Thresholds        Thresholds
	AllowedPersonnel  []string
	Generation        domain.Generation
	LockSummary       string
}

// Thresholds is the fixed threshold set bound into a snapshot.
type Thresholds struct {
	VitrificationMaxPct int   `json:"vitrification_max_pct"`
	BrowningMaxPct      int   `json:"browning_max_pct"`
	RootViabilityMin    int64 `json:"root_viability_min"`
	VirusCtMax          int64 `json:"virus_ct_max"`
	EndophyteCFUMax     int64 `json:"endophyte_cfu_max"`
}

// Task is the live aggregate state of an acclimation task.
type Task struct {
	ID           domain.TaskID      `json:"id"`
	State        domain.TaskState   `json:"state"`
	Generation   domain.Generation  `json:"generation"`
	StateVersion int64              `json:"state_version"`
	CreatedAt    domain.LogicalTime `json:"created_at"`
	UpdatedAt    domain.LogicalTime `json:"updated_at"`
	Final        domain.FinalType   `json:"final,omitempty"`
}

// OperationRecord is the persisted idempotency result of a business write.
type OperationRecord struct {
	TaskID        domain.TaskID
	OperationID   domain.OperationID
	RequestDigest string
	ResultCode    domain.ErrorCode
	Response      []byte
}

// SubcultureConfirmation records one of the two required confirmations.
type SubcultureConfirmation struct {
	TaskID      domain.TaskID
	Generation  domain.Generation
	PersonnelID string
	OperationID domain.OperationID
	ConfirmedAt domain.LogicalTime
}

// IndependentReview records one of the two required independent reviews.
type IndependentReview struct {
	TaskID         domain.TaskID
	Seq            int
	PersonnelID    string
	EvidenceDigest string
	Conclusion     string
}

// FinalDecision is the single-writer terminal outcome.
type FinalDecision struct {
	TaskID       domain.TaskID
	FinalType    domain.FinalType
	Version      int64
	Reason       string
	PermitSerial string
	IssuedAt     domain.LogicalTime
}
