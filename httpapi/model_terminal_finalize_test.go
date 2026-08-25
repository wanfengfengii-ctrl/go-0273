package httpapi

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"strawberry-vitro-acclimation-gate/catalog"
	"strawberry-vitro-acclimation-gate/domain"
	"strawberry-vitro-acclimation-gate/measure"
	"strawberry-vitro-acclimation-gate/pathogen"
	"strawberry-vitro-acclimation-gate/sample"
	"strawberry-vitro-acclimation-gate/store"
	"strawberry-vitro-acclimation-gate/task"
)

func TestModel_FinalizeIsSingleWriterAfterPermit(t *testing.T) {
	cases := []struct {
		name          string
		action        string
		confirmFirst  bool
		retainedState domain.TaskState
	}{
		{name: "cancel cannot overwrite permit", action: "cancel", retainedState: domain.StateAcclimatable},
		{name: "isolate cannot overwrite permit", action: "isolate", retainedState: domain.StateAcclimatable},
		{name: "repeated acclimate is rejected after confirmation", action: "acclimate", confirmFirst: true, retainedState: domain.StateAcclimated},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, db, id := modelAcclimatableTask(t)

			if tc.confirmFirst {
				code, env := doJSON(t, srv, "POST", "/api/v1/tasks/"+id+"/acclimation-confirmation", "op-confirm", map[string]any{})
				if code != http.StatusOK || env.Code != domain.CodeOK {
					t.Fatalf("acclimation confirmation failed: status=%d envelope=%+v", code, env)
				}
			}

			ctx := context.Background()
			beforeTask, err := db.Get(ctx, domain.TaskID(id))
			if err != nil {
				t.Fatalf("read task before late finalize: %v", err)
			}
			beforePermit, err := db.Permit(ctx, domain.TaskID(id))
			if err != nil {
				t.Fatalf("read permit before late finalize: %v", err)
			}
			beforeLeases, err := db.ActiveLeasesByTask(ctx, domain.TaskID(id))
			if err != nil {
				t.Fatalf("read leases before late finalize: %v", err)
			}
			if len(beforeLeases) == 0 {
				t.Fatal("fixture has no active leases to protect")
			}

			code, env := doJSON(t, srv, "POST", "/api/v1/tasks/"+id+"/finalize", "op-late-"+tc.action,
				map[string]any{"task_generation": 1, "action": tc.action})
			if code != http.StatusConflict || env.Code != domain.CodeTerminalState {
				t.Fatalf("late finalize %q: want conflict/TERMINAL_STATE, got status=%d envelope=%+v", tc.action, code, env)
			}

			afterTask, err := db.Get(ctx, domain.TaskID(id))
			if err != nil {
				t.Fatalf("read task after late finalize: %v", err)
			}
			if afterTask.State != tc.retainedState || afterTask.Final != domain.FinalAcclimate {
				t.Fatalf("late finalize changed decision: got state=%s final=%s", afterTask.State, afterTask.Final)
			}
			if !reflect.DeepEqual(afterTask, beforeTask) {
				t.Fatalf("late finalize changed task: before=%+v after=%+v", beforeTask, afterTask)
			}

			afterPermit, err := db.Permit(ctx, domain.TaskID(id))
			if err != nil {
				t.Fatalf("read permit after late finalize: %v", err)
			}
			if !reflect.DeepEqual(afterPermit, beforePermit) {
				t.Fatalf("late finalize changed permit: before=%+v after=%+v", beforePermit, afterPermit)
			}

			afterLeases, err := db.ActiveLeasesByTask(ctx, domain.TaskID(id))
			if err != nil {
				t.Fatalf("read leases after late finalize: %v", err)
			}
			if !reflect.DeepEqual(afterLeases, beforeLeases) {
				t.Fatalf("late finalize changed lease release result: before=%+v after=%+v", beforeLeases, afterLeases)
			}
		})
	}
}

func modelAcclimatableTask(t *testing.T) (*Server, *store.DB, string) {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	clock := domain.NewLogicalClock(0)
	catalogSvc := catalog.NewService(db)
	taskSvc := task.NewService(db, db, clock)
	srv := NewServer(
		catalogSvc,
		taskSvc,
		sample.NewService(db, clock),
		measure.NewService(db, clock),
		pathogen.NewService(db, clock),
		func() bool { return true },
	)
	seedCatalog(t, catalogSvc)
	id := createAndLock(t, srv)

	for _, personnel := range []string{"P1", "P2"} {
		code, env := doJSON(t, srv, "POST", "/api/v1/tasks/"+id+"/subculture-confirmations", "op-sc-"+personnel,
			task.ConfirmRequest{PersonnelID: personnel, Generation: 1})
		if code != http.StatusOK || env.Code != domain.CodeOK {
			t.Fatalf("subculture confirmation %s failed: status=%d envelope=%+v", personnel, code, env)
		}
	}

	steps := []struct {
		path string
		op   string
		body any
	}{
		{path: "/api/v1/tasks/" + id + "/samples/seal", op: "op-seal", body: sampleSealReq()},
		{path: "/api/v1/tasks/" + id + "/resources/acquire", op: "op-acquire", body: map[string]any{"task_generation": 1}},
		{path: "/api/v1/tasks/" + id + "/morphology", op: "op-morphology", body: morphReq()},
		{path: "/api/v1/tasks/" + id + "/measurements/root", op: "op-root", body: rootReq()},
	}
	for _, step := range steps {
		code, env := doJSON(t, srv, "POST", step.path, step.op, step.body)
		if code != http.StatusOK || env.Code != domain.CodeOK {
			t.Fatalf("setup step %s failed: status=%d envelope=%+v", step.path, code, env)
		}
	}

	runDeviceSuccess(t, srv, id, "call-w1", "BC1", "virus", "W1", "20.0", 1)
	runDeviceSuccess(t, srv, id, "call-w2", "BC2", "virus", "W2", "22.0", 1)
	runDeviceSuccess(t, srv, id, "call-ew1", "BC1", "endophyte", "EW1", "100", 0)
	runDeviceSuccess(t, srv, id, "call-ew2", "BC2", "endophyte", "EW2", "120", 0)

	code, env := doJSON(t, srv, "POST", "/api/v1/tasks/"+id+"/advance-review", "op-advance", map[string]any{})
	if code != http.StatusOK || env.Code != domain.CodeOK {
		t.Fatalf("advance to review failed: status=%d envelope=%+v", code, env)
	}
	for _, personnel := range []string{"R1", "R2"} {
		code, env = doJSON(t, srv, "POST", "/api/v1/tasks/"+id+"/reviews", "op-review-"+personnel,
			task.ReviewRequest{PersonnelID: personnel, Generation: 1, EvidenceDigest: "d", Conclusion: "acclimate"})
		if code != http.StatusOK || env.Code != domain.CodeOK {
			t.Fatalf("review %s failed: status=%d envelope=%+v", personnel, code, env)
		}
	}

	code, env = doJSON(t, srv, "POST", "/api/v1/tasks/"+id+"/finalize", "op-acclimate",
		map[string]any{"task_generation": 1, "action": "acclimate"})
	if code != http.StatusOK || env.Code != domain.CodeOK {
		t.Fatalf("acclimate finalize failed: status=%d envelope=%+v", code, env)
	}
	return srv, db, id
}
