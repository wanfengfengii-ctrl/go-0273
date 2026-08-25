// Package pathogen is the pathogen recheck and terminal arbitration component:
// scriptable device calls with deterministic retry, append-only virus and
// endophyte evidence chains, recheck rounds, late-arrival isolation, and the
// unique acclimation permit.
package pathogen

import (
	"strawberry-vitro-acclimation-gate/domain"
)

// DeviceType enumerates the four controllable devices.
type DeviceType string

const (
	DeviceRTQPCR         DeviceType = "rt_qpcr"
	DeviceIncubatorProbe DeviceType = "incubator_probe"
	DeviceLightLogger    DeviceType = "light_logger"
	DeviceColonyReader   DeviceType = "colony_reader"
)

// CallStatus is the lifecycle of a device call.
type CallStatus string

const (
	CallPending       CallStatus = "pending"
	CallFailedRetry   CallStatus = "failed_retry"
	CallSucceeded     CallStatus = "succeeded"
	CallPermanentFail CallStatus = "permanent_failed"
)

// DeviceCall is an auditable device invocation with a deterministic retry plan.
type DeviceCall struct {
	ID            string
	TaskID        domain.TaskID
	DeviceType    DeviceType
	Target        string
	FaultStep     int
	Attempts      int
	Status        CallStatus
	NextRetryAt   domain.LogicalTime
	ErrorCode     domain.ErrorCode
	PayloadDigest string
	// Evidence fields carried so a successful call can append evidence.
	BlindCode     string
	DetectionType string
	Well          string
	EvidenceValue int64
	EvidenceScale int
}

// PathogenEvidence is one immutable versioned evidence record.
type PathogenEvidence struct {
	TaskID        domain.TaskID     `json:"task_id"`
	BlindCode     string            `json:"blind_code"`
	DetectionType string            `json:"detection_type"`
	Well          string            `json:"well"`
	Value         domain.FixedPoint `json:"value"`
	Generation    domain.Generation `json:"generation"`
	RecheckRound  int               `json:"recheck_round"`
	Version       int64             `json:"version"`
}

// LateArrival preserves a stale-generation result for audit without touching
// the active evidence chain.
type LateArrival struct {
	TaskID        domain.TaskID
	BlindCode     string
	DetectionType string
	Generation    domain.Generation
	Reason        string
}

// RecheckRound is the single active recheck round for a generation.
type RecheckRound struct {
	TaskID             domain.TaskID
	Generation         domain.Generation
	Round              int
	Trigger            string
	AffectedPositions  []string
	AffectedBlindCodes []string
	AffectedPoints     []string
	AffectedWells      []string
	Status             string
	EvidenceDigest     string
}

// AcclimationPermit is the unique acclimation credential.
type AcclimationPermit struct {
	Serial         string             `json:"serial"`
	TaskID         domain.TaskID      `json:"task_id"`
	LockDigest     string             `json:"lock_digest"`
	EvidenceDigest string             `json:"evidence_digest"`
	Reviewers      []string           `json:"reviewers"`
	IssuedAt       domain.LogicalTime `json:"issued_at"`
}
