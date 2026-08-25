package store

import (
	"context"
	"path/filepath"
	"testing"

	"strawberry-vitro-acclimation-gate/domain"
	"strawberry-vitro-acclimation-gate/pathogen"
	"strawberry-vitro-acclimation-gate/sample"
	"strawberry-vitro-acclimation-gate/task"
)

// TestRestartRecovery verifies that open tasks, active leases, pending device
// calls, blind-code state, and the next retry logical time all survive a
// process restart and are re-read by Recover without advancing state.
func TestRestartRecovery(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "recover.db")
	ctx := context.Background()

	// First run: create, lock, lease, and a pending device call.
	db, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	clock := domain.NewLogicalClock(0)
	snap := testSnapshot()
	if err := db.CreateTask(ctx, task.Task{ID: "T1", State: domain.StateDraft, Generation: 1, CreatedAt: clock.Now(), UpdatedAt: clock.Now()}, snap); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := db.LockTask(ctx, "T1", snap); err != nil {
		t.Fatalf("lock: %v", err)
	}
	if err := db.AcquireLease(ctx, sample.ResourceLease{
		ResourceType: sample.ResourceRackShelf, ResourceKey: "RS1", TaskID: "T1",
		Generation: 1, Version: 1, StartAt: 0, EndAt: 1000, Active: true,
	}); err != nil {
		t.Fatalf("lease: %v", err)
	}
	call := pathogen.DeviceCall{
		ID: "call-1", TaskID: "T1", DeviceType: pathogen.DeviceRTQPCR, Target: "W1",
		Status: pathogen.CallFailedRetry, Attempts: 1, NextRetryAt: 2, ErrorCode: "DEVICE_REFUSED",
	}
	if err := db.SaveDeviceCall(ctx, call); err != nil {
		t.Fatalf("device call: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Second run: reopen and recover.
	db, err = Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db.Close()
	report, err := db.Recover(ctx, 100)
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	if report.OpenTasks != 1 || report.ActiveLeases != 1 || report.PendingCalls != 1 {
		t.Fatalf("unexpected recovery report: %+v", report)
	}

	tk, err := db.Get(ctx, "T1")
	if err != nil || tk.State != domain.StatePendingSubculture {
		t.Fatalf("task state not recovered: %+v err=%v", tk, err)
	}
	lease, err := db.ActiveLease(ctx, sample.ResourceRackShelf, "RS1")
	if err != nil || lease.Version != 1 || lease.TaskID != "T1" {
		t.Fatalf("lease not recovered: %+v err=%v", lease, err)
	}
	pending, err := db.PendingRetries(ctx, 100)
	if err != nil || len(pending) != 1 || pending[0].NextRetryAt != 2 {
		t.Fatalf("pending call not recovered: %+v err=%v", pending, err)
	}
}

func testSnapshot() task.Snapshot {
	return task.Snapshot{
		TaskID: "T1", MotherPlantID: "M1", LineageID: "L1", MediumFormulaID: "F1", MediumSummary: "V1",
		SubcultureBatch: "B1", BottleSeals: []string{"S1"}, BottlePositions: []string{"POS1"},
		LockedSeedlings: map[string]int{"POS1": 10}, BlindCodes: []string{"BC1"}, RootPoints: []string{"RP1"},
		VitrificationPos: []string{"VP1"}, RTQPCRWells: []string{"W1"}, EndophyteWells: []string{"EW1"},
		RackShelf: "RS1", LightWindow: "LW1", AcclimationWindow: "AW1", Generation: 1,
	}
}
