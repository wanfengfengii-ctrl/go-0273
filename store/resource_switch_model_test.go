package store

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"strawberry-vitro-acclimation-gate/domain"
	"strawberry-vitro-acclimation-gate/sample"
)

func TestModel_SwitchReleasesOldResourceLease(t *testing.T) {
	tests := []struct {
		name          string
		oldKey        string
		newKey        string
		occupyNewKey  bool
		wantError     bool
		wantOldActive bool
	}{
		{name: "switch frees old key and keeps one active lease", oldKey: "RS1", newKey: "RS2"},
		{name: "same-key renewal replaces the active lease", oldKey: "RS3", newKey: "RS3", wantOldActive: true},
		{name: "conflicting target rolls the switch back", oldKey: "RS4", newKey: "RS5", occupyNewKey: true, wantError: true, wantOldActive: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			db, err := Open(":memory:")
			if err != nil {
				t.Fatalf("open store: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			initial := sample.ResourceLease{
				ResourceType: sample.ResourceRackShelf,
				ResourceKey:  tt.oldKey,
				TaskID:       domain.TaskID("T1"),
				Generation:   1,
				Version:      1,
				StartAt:      10,
				EndAt:        1010,
				Active:       true,
			}
			if err := db.AcquireLease(ctx, initial); err != nil {
				t.Fatalf("acquire initial lease: %v", err)
			}
			if tt.occupyNewKey {
				conflict := initial
				conflict.ResourceKey = tt.newKey
				conflict.TaskID = domain.TaskID("T2")
				if err := db.AcquireLease(ctx, conflict); err != nil {
					t.Fatalf("acquire conflicting lease: %v", err)
				}
			}

			replacement := initial
			replacement.ResourceKey = tt.newKey
			replacement.Version = 2
			replacement.StartAt = 20
			replace := reflect.ValueOf(db).MethodByName("ReplaceLease")
			args := []reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(replacement)}
			if replace.Type().NumIn() == 3 {
				args = []reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(tt.oldKey), reflect.ValueOf(replacement)}
			}
			result := replace.Call(args)
			err = nil
			if !result[0].IsNil() {
				err = result[0].Interface().(error)
			}
			if (err != nil) != tt.wantError {
				t.Fatalf("ReplaceLease() error = %v, wantError %v", err, tt.wantError)
			}

			oldLease, oldErr := db.ActiveLease(ctx, sample.ResourceRackShelf, tt.oldKey)
			if tt.wantOldActive {
				if oldErr != nil || oldLease.TaskID != domain.TaskID("T1") {
					t.Fatalf("old lease should remain active for T1: lease=%+v err=%v", oldLease, oldErr)
				}
			} else {
				if !errors.Is(oldErr, ErrNotFound) {
					t.Fatalf("old lease remained active after switch: lease=%+v err=%v", oldLease, oldErr)
				}
				freeLease := initial
				freeLease.TaskID = domain.TaskID("T2")
				if err := db.AcquireLease(ctx, freeLease); err != nil {
					t.Fatalf("old resource key was not reusable by another task: %v", err)
				}
			}

			active, err := db.ActiveLeasesByTask(ctx, domain.TaskID("T1"))
			if err != nil {
				t.Fatalf("list active leases: %v", err)
			}
			if len(active) != 1 {
				t.Fatalf("T1 active lease count = %d, want 1: %+v", len(active), active)
			}
			if !tt.wantError && (active[0].ResourceKey != tt.newKey || active[0].Version != 2) {
				t.Fatalf("replacement lease = %+v, want key %q at version 2", active[0], tt.newKey)
			}
		})
	}
}
