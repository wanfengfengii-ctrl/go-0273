// Package sample is the bottle-sample and resource-occupancy ledger: bottle
// seals, positions, triple samples, blind codes, and versioned resource leases
// over rack shelves, light windows, acclimation windows, and detection wells.
package sample

import (
	"strawberry-vitro-acclimation-gate/domain"
)

// ResourceType enumerates the five leaseable resource kinds.
type ResourceType string

const (
	ResourceRackShelf         ResourceType = "rack_shelf"
	ResourceLightWindow       ResourceType = "light_window"
	ResourceAcclimationWindow ResourceType = "acclimation_window"
	ResourceRTQPCRWell        ResourceType = "rt_pcr_well"
	ResourceEndophyteWell     ResourceType = "endophyte_well"
)

// BottleSample ties one bottle seal to a position and its locked seedling count.
type BottleSample struct {
	TaskID          domain.TaskID
	Seal            string
	Position        string
	LockedSeedlings int
	TripleSealed    bool
}

// BlindCode binds a blinded code digest to a sample; the plaintext is never
// exposed in an unauthorized response.
type BlindCode struct {
	TaskID      domain.TaskID
	Digest      string
	BoundSeal   string
	Revealed    bool
	RevealAudit []RevealEvent
}

// RevealEvent records a controlled reveal for audit.
type RevealEvent struct {
	At domain.LogicalTime
	By string
}

// ResourceLease is a versioned lease with logical start/end times.
type ResourceLease struct {
	ResourceType ResourceType
	ResourceKey  string
	TaskID       domain.TaskID
	Generation   domain.Generation
	Version      int64
	StartAt      domain.LogicalTime
	EndAt        domain.LogicalTime
	Active       bool
}
