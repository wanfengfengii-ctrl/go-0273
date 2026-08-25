package task

import "strawberry-vitro-acclimation-gate/domain"

// CanTransition reports whether the state machine allows moving from s to next.
// Isolate and cancel are reachable only from the documented risk/cancel entry
// points; acclimatable requires evidence closure and dual independent review.
func CanTransition(s, next domain.TaskState) bool {
	switch s {
	case domain.StateDraft:
		return next == domain.StatePendingSubculture || next == domain.StateCancelled
	case domain.StatePendingSubculture:
		return next == domain.StateSealingSamples || next == domain.StateCancelled
	case domain.StateSealingSamples:
		return next == domain.StateOccupyingResources || next == domain.StateCancelled
	case domain.StateOccupyingResources:
		return next == domain.StateCollectingMorphology || next == domain.StateCancelled
	case domain.StateCollectingMorphology:
		return next == domain.StateVerifyingRoots || next == domain.StateCancelled
	case domain.StateVerifyingRoots:
		return next == domain.StateRecheckingPathogen || next == domain.StateCancelled
	case domain.StateRecheckingPathogen:
		return next == domain.StatePendingReview || next == domain.StateIsolated || next == domain.StateCancelled
	case domain.StatePendingReview:
		return next == domain.StateAcclimatable || next == domain.StateIsolated || next == domain.StateCancelled
	case domain.StateAcclimatable:
		return next == domain.StateAcclimated || next == domain.StateCancelled
	case domain.StateAcclimated, domain.StateIsolated, domain.StateCancelled:
		return false
	default:
		return false
	}
}
