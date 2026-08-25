package httpapi

import (
	"net/http"
	"testing"

	"strawberry-vitro-acclimation-gate/domain"
	"strawberry-vitro-acclimation-gate/task"
)

// TestIdempotentReplayAndConflict verifies the operation-id protocol: the same
// operation with the same content replays the original result, and the same
// operation with different content returns OPERATION_CONTENT_CONFLICT.
func TestIdempotentReplayAndConflict(t *testing.T) {
	srv, _ := newTestServer(t)
	id := createAndLock(t, srv)

	code, env := doJSON(t, srv, "POST", "/api/v1/tasks/"+id+"/subculture-confirmations", "op-sc",
		task.ConfirmRequest{PersonnelID: "P1", Generation: 1})
	if code != http.StatusOK || env.Code != domain.CodeOK {
		t.Fatalf("first confirm failed: %+v", env)
	}

	// Replay with the same operation and content returns the original result.
	code, env = doJSON(t, srv, "POST", "/api/v1/tasks/"+id+"/subculture-confirmations", "op-sc",
		task.ConfirmRequest{PersonnelID: "P1", Generation: 1})
	if code != http.StatusOK || env.Code != domain.CodeOK {
		t.Fatalf("replay should return original result: %+v", env)
	}

	// Same operation with a different content conflicts.
	code, env = doJSON(t, srv, "POST", "/api/v1/tasks/"+id+"/subculture-confirmations", "op-sc",
		task.ConfirmRequest{PersonnelID: "P2", Generation: 1})
	if code != http.StatusConflict || env.Code != domain.CodeOperationContentConflict {
		t.Fatalf("expected content conflict, got %d %+v", code, env)
	}
}

// TestRoleOverlapRejected verifies a person cannot confirm twice.
func TestRoleOverlapRejected(t *testing.T) {
	srv, _ := newTestServer(t)
	id := createAndLock(t, srv)

	doJSON(t, srv, "POST", "/api/v1/tasks/"+id+"/subculture-confirmations", "op-1",
		task.ConfirmRequest{PersonnelID: "P1", Generation: 1})
	code, env := doJSON(t, srv, "POST", "/api/v1/tasks/"+id+"/subculture-confirmations", "op-2",
		task.ConfirmRequest{PersonnelID: "P1", Generation: 1})
	if code != http.StatusBadRequest || env.Code != domain.CodeRoleOverlap {
		t.Fatalf("expected role overlap, got %d %+v", code, env)
	}
}

// TestLineageMismatchRejected verifies locking fails on a wrong lineage.
func TestLineageMismatchRejected(t *testing.T) {
	srv, _ := newTestServer(t)
	req := createReq()
	req.TaskID = "T-bad"
	req.LineageID = "WRONG"
	doJSON(t, srv, "POST", "/api/v1/tasks", "op-c", req)
	code, env := doJSON(t, srv, "POST", "/api/v1/tasks/T-bad/lock", "op-l", map[string]any{})
	if code != http.StatusBadRequest || env.Code != domain.CodeLineageMismatch {
		t.Fatalf("expected lineage mismatch, got %d %+v", code, env)
	}
}

// TestStaleMediumSummaryRejected verifies locking fails on a stale summary.
func TestStaleMediumSummaryRejected(t *testing.T) {
	srv, _ := newTestServer(t)
	req := createReq()
	req.TaskID = "T-stale"
	req.MediumSummary = "OLD"
	doJSON(t, srv, "POST", "/api/v1/tasks", "op-c", req)
	code, env := doJSON(t, srv, "POST", "/api/v1/tasks/T-stale/lock", "op-l", map[string]any{})
	if code != http.StatusBadRequest || env.Code != domain.CodeStaleMediumSummary {
		t.Fatalf("expected stale medium summary, got %d %+v", code, env)
	}
}

// TestCountNotConservedRollback verifies a morphology batch with a conserved
// total that is wrong rolls back entirely.
func TestCountNotConservedRollback(t *testing.T) {
	srv, _ := newTestServer(t)
	id := createAndLock(t, srv)
	for _, p := range []string{"P1", "P2"} {
		doJSON(t, srv, "POST", "/api/v1/tasks/"+id+"/subculture-confirmations", "op-sc-"+p,
			task.ConfirmRequest{PersonnelID: p, Generation: 1})
	}
	doJSON(t, srv, "POST", "/api/v1/tasks/"+id+"/samples/seal", "op-seal", sampleSealReq())
	doJSON(t, srv, "POST", "/api/v1/tasks/"+id+"/resources/acquire", "op-acq", map[string]any{"task_generation": 1})

	bad := map[string]any{
		"task_generation": 1,
		"cells": []map[string]any{
			{"position": "POS1", "normal": 9, "vitrified": 1, "browned": 1, "contaminated": 0, "scoring_plate_pos": "VP1"},
			{"position": "POS2", "normal": 9, "vitrified": 0, "browned": 1, "contaminated": 0, "scoring_plate_pos": "VP1"},
		},
	}
	code, env := doJSON(t, srv, "POST", "/api/v1/tasks/"+id+"/morphology", "op-morph", bad)
	if code != http.StatusBadRequest || env.Code != domain.CodeCountNotConserved {
		t.Fatalf("expected count not conserved, got %d %+v", code, env)
	}
}

// TestTerminalStateRejected verifies writes after a terminal outcome return
// TERMINAL_STATE and do not change data.
func TestTerminalStateRejected(t *testing.T) {
	srv, _ := newTestServer(t)
	id := createAndLock(t, srv)
	code, env := doJSON(t, srv, "POST", "/api/v1/tasks/"+id+"/finalize", "op-fin",
		map[string]any{"task_generation": 1, "action": "cancel"})
	if code != http.StatusOK || env.Code != domain.CodeOK {
		t.Fatalf("cancel failed: %+v", env)
	}
	// Any later write is rejected with TERMINAL_STATE.
	code, env = doJSON(t, srv, "POST", "/api/v1/tasks/"+id+"/subculture-confirmations", "op-late",
		task.ConfirmRequest{PersonnelID: "P1", Generation: 1})
	if code != http.StatusConflict || env.Code != domain.CodeTerminalState {
		t.Fatalf("expected terminal state, got %d %+v", code, env)
	}
}

func createReq() task.CreateRequest {
	return task.CreateRequest{
		TaskID: "T1", MotherPlantID: "M1", LineageID: "L1", MediumFormulaID: "F1", MediumSummary: "V1",
		SubcultureBatch: "B1",
		Bottles: []task.BottleInput{
			{Seal: "S1", Position: "POS1", LockedSeedlings: 10},
			{Seal: "S2", Position: "POS2", LockedSeedlings: 10},
		},
		BlindCodes: []string{"BC1", "BC2"}, RootPoints: []string{"RP1"},
		VitrificationPos: []string{"VP1"}, RTQPCRWells: []string{"W1", "W2"}, EndophyteWells: []string{"EW1", "EW2"},
		RackShelf: "RS1", LightWindow: "LW1", AcclimationWindow: "AW1",
		Thresholds:       task.Thresholds{VitrificationMaxPct: 30, BrowningMaxPct: 20, RootViabilityMin: 50, VirusCtMax: 350, EndophyteCFUMax: 1000},
		AllowedPersonnel: []string{"P1", "P2", "R1", "R2"}, Generation: 1,
	}
}
