package httpapi

import (
	"context"
	"net/http"
	"testing"

	"strawberry-vitro-acclimation-gate/catalog"
	"strawberry-vitro-acclimation-gate/domain"
	"strawberry-vitro-acclimation-gate/measure"
	"strawberry-vitro-acclimation-gate/pathogen"
	"strawberry-vitro-acclimation-gate/sample"
	"strawberry-vitro-acclimation-gate/store"
	"strawberry-vitro-acclimation-gate/task"
)

func TestModel_SealBatchAtomicity(t *testing.T) {
	tests := []struct {
		name      string
		seals     []map[string]any
		wantCode  domain.ErrorCode
		wantState domain.TaskState
		wantBound map[string]string
		wantSaved bool
	}{
		{
			name: "unknown second seal rolls back the first seal and both bindings",
			seals: []map[string]any{
				{"seal": "S1", "position": "POS1", "blind_code_digest": "BC1"},
				{"seal": "NOPE", "position": "POS2", "blind_code_digest": "BC2"},
			},
			wantCode:  domain.CodeInvalidInput,
			wantState: domain.StateSealingSamples,
			wantBound: map[string]string{"BC1": "", "BC2": ""},
		},
		{
			name: "unknown second position rolls back the first seal and both bindings",
			seals: []map[string]any{
				{"seal": "S1", "position": "POS1", "blind_code_digest": "BC1"},
				{"seal": "S2", "position": "NOPE", "blind_code_digest": "BC2"},
			},
			wantCode:  domain.CodeInvalidInput,
			wantState: domain.StateSealingSamples,
			wantBound: map[string]string{"BC1": "", "BC2": ""},
		},
		{
			name: "unknown second blind code rolls back the first binding and both seals",
			seals: []map[string]any{
				{"seal": "S1", "position": "POS1", "blind_code_digest": "BC1"},
				{"seal": "S2", "position": "POS2", "blind_code_digest": "NOPE"},
			},
			wantCode:  domain.CodeInvalidInput,
			wantState: domain.StateSealingSamples,
			wantBound: map[string]string{"BC1": "", "BC2": ""},
		},
		{
			name: "complete valid batch seals and binds everything before advancing",
			seals: []map[string]any{
				{"seal": "S1", "position": "POS1", "blind_code_digest": "BC1"},
				{"seal": "S2", "position": "POS2", "blind_code_digest": "BC2"},
			},
			wantCode:  domain.CodeOK,
			wantState: domain.StateOccupyingResources,
			wantBound: map[string]string{"BC1": "S1", "BC2": "S2"},
			wantSaved: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			db, err := store.Open(":memory:")
			if err != nil {
				t.Fatalf("open store: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			clock := domain.NewLogicalClock(0)
			catalogSvc := catalog.NewService(db)
			srv := NewServer(
				catalogSvc,
				task.NewService(db, db, clock),
				sample.NewService(db, clock),
				measure.NewService(db, clock),
				pathogen.NewService(db, clock),
				func() bool { return true },
			)
			seedCatalog(t, catalogSvc)

			code, env := doJSON(t, srv, http.MethodPost, "/api/v1/tasks", "model-create", createReq())
			if code != http.StatusOK || env.Code != domain.CodeOK {
				t.Fatalf("create task: status=%d envelope=%+v", code, env)
			}
			code, env = doJSON(t, srv, http.MethodPost, "/api/v1/tasks/T1/lock", "model-lock", map[string]any{})
			if code != http.StatusOK || env.Code != domain.CodeOK {
				t.Fatalf("lock task: status=%d envelope=%+v", code, env)
			}
			for _, personnel := range []string{"P1", "P2"} {
				code, env = doJSON(t, srv, http.MethodPost, "/api/v1/tasks/T1/subculture-confirmations", "model-confirm-"+personnel,
					task.ConfirmRequest{PersonnelID: personnel, Generation: 1})
				if code != http.StatusOK || env.Code != domain.CodeOK {
					t.Fatalf("confirm %s: status=%d envelope=%+v", personnel, code, env)
				}
			}

			const sealOperation = domain.OperationID("model-seal")
			code, env = doJSON(t, srv, http.MethodPost, "/api/v1/tasks/T1/samples/seal", string(sealOperation), map[string]any{
				"task_generation": 1,
				"seals":           tt.seals,
			})
			wantHTTP := http.StatusBadRequest
			if tt.wantCode == domain.CodeOK {
				wantHTTP = http.StatusOK
			}
			if code != wantHTTP || env.Code != tt.wantCode {
				t.Fatalf("seal response: status=%d code=%s, want status=%d code=%s", code, env.Code, wantHTTP, tt.wantCode)
			}

			gotTask, err := db.Get(ctx, domain.TaskID("T1"))
			if err != nil {
				t.Fatalf("get task: %v", err)
			}
			if gotTask.State != tt.wantState {
				t.Errorf("task state=%s, want %s", gotTask.State, tt.wantState)
			}

			samples, err := db.SamplesByTask(ctx, domain.TaskID("T1"))
			if err != nil {
				t.Fatalf("get samples: %v", err)
			}
			if len(samples) != 2 {
				t.Fatalf("sample count=%d, want 2", len(samples))
			}
			wantSealed := tt.wantCode == domain.CodeOK
			for _, got := range samples {
				if got.TripleSealed != wantSealed {
					t.Errorf("sample %s triple_sealed=%v, want %v", got.Seal, got.TripleSealed, wantSealed)
				}
			}

			codes, err := db.BlindCodesByTask(ctx, domain.TaskID("T1"))
			if err != nil {
				t.Fatalf("get blind codes: %v", err)
			}
			if len(codes) != 2 {
				t.Fatalf("blind-code count=%d, want 2", len(codes))
			}
			for _, got := range codes {
				if got.BoundSeal != tt.wantBound[got.Digest] {
					t.Errorf("blind code %s bound_seal=%q, want %q", got.Digest, got.BoundSeal, tt.wantBound[got.Digest])
				}
			}

			_, saved, err := db.FindOperation(ctx, domain.TaskID("T1"), sealOperation)
			if err != nil {
				t.Fatalf("find seal operation: %v", err)
			}
			if saved != tt.wantSaved {
				t.Errorf("seal idempotency record present=%v, want %v", saved, tt.wantSaved)
			}
		})
	}
}
