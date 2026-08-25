package task

import (
	"testing"

	"strawberry-vitro-acclimation-gate/domain"
)

func TestForwardChain(t *testing.T) {
	chain := []domain.TaskState{
		domain.StateDraft, domain.StatePendingSubculture, domain.StateSealingSamples,
		domain.StateOccupyingResources, domain.StateCollectingMorphology,
		domain.StateVerifyingRoots, domain.StateRecheckingPathogen,
		domain.StatePendingReview, domain.StateAcclimatable, domain.StateAcclimated,
	}
	for i := 0; i < len(chain)-1; i++ {
		if !CanTransition(chain[i], chain[i+1]) {
			t.Fatalf("expected %v -> %v", chain[i], chain[i+1])
		}
	}
}

func TestAcclimatableOnlyAfterReview(t *testing.T) {
	if CanTransition(domain.StateRecheckingPathogen, domain.StateAcclimatable) {
		t.Fatal("must not acclimatize before independent review")
	}
	if !CanTransition(domain.StatePendingReview, domain.StateAcclimatable) {
		t.Fatal("acclimatable must follow pending review")
	}
}

func TestIsolateRiskEntry(t *testing.T) {
	if !CanTransition(domain.StateRecheckingPathogen, domain.StateIsolated) {
		t.Fatal("isolate must be reachable from pathogen recheck")
	}
	if !CanTransition(domain.StatePendingReview, domain.StateIsolated) {
		t.Fatal("isolate must be reachable from pending review")
	}
	if CanTransition(domain.StateDraft, domain.StateIsolated) {
		t.Fatal("isolate must not be reachable from draft")
	}
}

func TestCancelFromOpenStates(t *testing.T) {
	if !CanTransition(domain.StateDraft, domain.StateCancelled) {
		t.Fatal("cancel must be reachable from draft")
	}
	if CanTransition(domain.StateAcclimated, domain.StateCancelled) {
		t.Fatal("terminal state must not transition")
	}
	if CanTransition(domain.StateCancelled, domain.StateDraft) {
		t.Fatal("cancelled is terminal")
	}
}
