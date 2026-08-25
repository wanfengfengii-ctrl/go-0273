// Package domain holds the stable cross-cutting types shared by all business
// packages: task state, identifiers, error codes, structured reasons, and the
// logical clock.
package domain

import (
	"encoding/json"
	"sort"
)

// TaskID identifies an acclimation task aggregate.
type TaskID string

// Generation is the task generation number; every business write carries one.
type Generation int64

// OperationID is the idempotency key carried by every business write request.
type OperationID string

// TaskState enumerates the twelve legal phases of an acclimation task.
type TaskState int

const (
	StateDraft                TaskState = iota // 待锁定
	StatePendingSubculture                     // 待继代确认
	StateSealingSamples                        // 样本封存中
	StateOccupyingResources                    // 资源占用中
	StateCollectingMorphology                  // 形态采集中
	StateVerifyingRoots                        // 根系核验中
	StateRecheckingPathogen                    // 病原复测中
	StatePendingReview                         // 待独立复核
	StateAcclimatable                          // 可驯化
	StateAcclimated                            // 已驯化
	StateIsolated                              // 无菌隔离
	StateCancelled                             // 已取消
)

var stateNames = [...]string{
	"draft",
	"pending_subculture",
	"sealing_samples",
	"occupying_resources",
	"collecting_morphology",
	"verifying_roots",
	"rechecking_pathogen",
	"pending_review",
	"acclimatable",
	"acclimated",
	"isolated",
	"cancelled",
}

func (s TaskState) String() string {
	if s < 0 || int(s) >= len(stateNames) {
		return "unknown"
	}
	return stateNames[s]
}

// MarshalJSON serializes a task state as its stable string name so the HTTP
// API exposes "pending_subculture" rather than a raw integer.
func (s TaskState) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

// IsTerminal reports whether the state is final and rejects further writes.
func (s TaskState) IsTerminal() bool {
	return s == StateAcclimated || s == StateIsolated || s == StateCancelled
}

// FinalType is the single-writer terminal outcome of a task.
type FinalType string

const (
	FinalAcclimate FinalType = "acclimate"
	FinalIsolate   FinalType = "isolate"
	FinalCancel    FinalType = "cancel"
)

// ErrorCode is a stable, business-level error code. HTTP status only conveys
// the protocol category; tests assert on these codes.
type ErrorCode string

const (
	CodeOK                       ErrorCode = "OK"
	CodeInvalidInput             ErrorCode = "INVALID_INPUT"
	CodeOperationContentConflict ErrorCode = "OPERATION_CONTENT_CONFLICT"
	CodeTerminalState            ErrorCode = "TERMINAL_STATE"
	CodeLineageMismatch          ErrorCode = "LINEAGE_MISMATCH"
	CodeStaleMediumSummary       ErrorCode = "STALE_MEDIUM_SUMMARY"
	CodeDuplicateValue           ErrorCode = "DUPLICATE_VALUE"
	CodeRoleOverlap              ErrorCode = "ROLE_OVERLAP"
	CodeNotAuthorized            ErrorCode = "NOT_AUTHORIZED"
	CodePreBlindReveal           ErrorCode = "PRE_BLIND_REVEAL"
	CodeCountNotConserved        ErrorCode = "COUNT_NOT_CONSERVED"
	CodeEvidenceIncomplete       ErrorCode = "EVIDENCE_INCOMPLETE"
	CodeInvalidFixedPoint        ErrorCode = "INVALID_FIXED_POINT"
	CodeArithmeticOverflow       ErrorCode = "ARITHMETIC_OVERFLOW"
	CodeDivideByZero             ErrorCode = "DIVIDE_BY_ZERO"
	CodeRetryNotDue              ErrorCode = "RETRY_NOT_DUE"
)

// Reason is one structured cause in a rejection response.
type Reason struct {
	MotherPlantID   string `json:"mother_plant_id"`
	SubcultureBatch string `json:"subculture_batch"`
	BottleSeal      string `json:"bottle_seal"`
	BottlePosition  string `json:"bottle_position"`
	TestWell        string `json:"test_well"`
	Field           string `json:"field"`
	Detail          string `json:"detail"`
}

// Reasons implements deterministic canonical ordering by mother_plant_id,
// subculture_batch, bottle_seal, bottle_position, then test_well. Missing
// fields sort as the empty string.
type Reasons []Reason

func (r Reasons) Len() int      { return len(r) }
func (r Reasons) Swap(i, j int) { r[i], r[j] = r[j], r[i] }
func (r Reasons) Less(i, j int) bool {
	a, b := r[i], r[j]
	if a.MotherPlantID != b.MotherPlantID {
		return a.MotherPlantID < b.MotherPlantID
	}
	if a.SubcultureBatch != b.SubcultureBatch {
		return a.SubcultureBatch < b.SubcultureBatch
	}
	if a.BottleSeal != b.BottleSeal {
		return a.BottleSeal < b.BottleSeal
	}
	if a.BottlePosition != b.BottlePosition {
		return a.BottlePosition < b.BottlePosition
	}
	return a.TestWell < b.TestWell
}

// SortReasons returns a canonically sorted copy of in.
func SortReasons(in []Reason) []Reason {
	out := append([]Reason(nil), in...)
	sort.Stable(Reasons(out))
	return out
}

// LogicalTime is a monotonic logical timestamp, decoupled from wall clocks.
type LogicalTime int64

// LogicalClock is a deterministic, manually advanced clock so no assertion or
// retry schedule depends on real time.
type LogicalClock struct {
	now LogicalTime
}

// NewLogicalClock returns a clock starting at start.
func NewLogicalClock(start LogicalTime) *LogicalClock { return &LogicalClock{now: start} }

// Now returns the current logical time.
func (c *LogicalClock) Now() LogicalTime { return c.now }

// Advance increments and returns the logical time.
func (c *LogicalClock) Advance() LogicalTime {
	c.now++
	return c.now
}

// Set overwrites the logical time.
func (c *LogicalClock) Set(t LogicalTime) { c.now = t }
