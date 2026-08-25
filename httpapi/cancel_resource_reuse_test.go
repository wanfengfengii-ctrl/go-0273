package httpapi

import (
	"net/http"
	"testing"

	"strawberry-vitro-acclimation-gate/domain"
	"strawberry-vitro-acclimation-gate/task"
)

func TestModel_CancelReleasesResourcesForReuse(t *testing.T) {
	cases := []struct {
		name              string
		advanceAfterLease bool
	}{
		{name: "cancel immediately after resource acquisition"},
		{name: "cancel after work has begun on leased resources", advanceAfterLease: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newTestServer(t)

			requestFor := func(id string) task.CreateRequest {
				req := createReq()
				req.TaskID = id
				req.SubcultureBatch = "batch-" + id
				req.Bottles = []task.BottleInput{
					{Seal: "seal-1-" + id, Position: "position-1-" + id, LockedSeedlings: 10},
					{Seal: "seal-2-" + id, Position: "position-2-" + id, LockedSeedlings: 10},
				}
				req.BlindCodes = []string{"blind-1-" + id, "blind-2-" + id}
				req.RootPoints = []string{"root-" + id}
				req.VitrificationPos = []string{"plate-" + id}
				return req
			}

			prepareAndAcquire := func(id string) task.CreateRequest {
				req := requestFor(id)
				code, env := doJSON(t, srv, http.MethodPost, "/api/v1/tasks", "create-"+id, req)
				if code != http.StatusOK || env.Code != domain.CodeOK {
					t.Fatalf("create %s: status=%d response=%+v", id, code, env)
				}
				code, env = doJSON(t, srv, http.MethodPost, "/api/v1/tasks/"+id+"/lock", "lock-"+id, map[string]any{})
				if code != http.StatusOK || env.Code != domain.CodeOK {
					t.Fatalf("lock %s: status=%d response=%+v", id, code, env)
				}
				for _, personnel := range []string{"P1", "P2"} {
					code, env = doJSON(t, srv, http.MethodPost, "/api/v1/tasks/"+id+"/subculture-confirmations", "confirm-"+id+"-"+personnel,
						task.ConfirmRequest{PersonnelID: personnel, Generation: 1})
					if code != http.StatusOK || env.Code != domain.CodeOK {
						t.Fatalf("confirm %s for %s: status=%d response=%+v", personnel, id, code, env)
					}
				}
				sealRequest := map[string]any{
					"task_generation": 1,
					"seals": []map[string]any{
						{"seal": req.Bottles[0].Seal, "position": req.Bottles[0].Position, "blind_code_digest": req.BlindCodes[0]},
						{"seal": req.Bottles[1].Seal, "position": req.Bottles[1].Position, "blind_code_digest": req.BlindCodes[1]},
					},
				}
				code, env = doJSON(t, srv, http.MethodPost, "/api/v1/tasks/"+id+"/samples/seal", "seal-"+id, sealRequest)
				if code != http.StatusOK || env.Code != domain.CodeOK {
					t.Fatalf("seal %s: status=%d response=%+v", id, code, env)
				}
				code, env = doJSON(t, srv, http.MethodPost, "/api/v1/tasks/"+id+"/resources/acquire", "acquire-"+id,
					map[string]any{"task_generation": 1})
				if code != http.StatusOK || env.Code != domain.CodeOK {
					t.Fatalf("acquire %s: status=%d response=%+v", id, code, env)
				}
				return req
			}

			first := prepareAndAcquire("cancelled")
			if tc.advanceAfterLease {
				morphology := map[string]any{
					"task_generation": 1,
					"cells": []map[string]any{
						{"position": first.Bottles[0].Position, "normal": 8, "vitrified": 1, "browned": 0, "contaminated": 1, "scoring_plate_pos": first.VitrificationPos[0]},
						{"position": first.Bottles[1].Position, "normal": 9, "vitrified": 0, "browned": 1, "contaminated": 0, "scoring_plate_pos": first.VitrificationPos[0]},
					},
				}
				code, env := doJSON(t, srv, http.MethodPost, "/api/v1/tasks/cancelled/morphology", "morphology-cancelled", morphology)
				if code != http.StatusOK || env.Code != domain.CodeOK {
					t.Fatalf("advance leased task: status=%d response=%+v", code, env)
				}
			}

			code, env := doJSON(t, srv, http.MethodPost, "/api/v1/tasks/cancelled/finalize", "finalize-cancelled",
				map[string]any{"task_generation": 1, "action": "cancel"})
			if code != http.StatusOK || env.Code != domain.CodeOK {
				t.Fatalf("cancel leased task: status=%d response=%+v", code, env)
			}
			data, ok := env.Data.(map[string]any)
			if !ok || data["state"] != domain.StateCancelled.String() {
				t.Fatalf("cancel did not report cancelled terminal state: %+v", env)
			}

			code, env = doJSON(t, srv, http.MethodPost, "/api/v1/tasks/cancelled/subculture-confirmations", "write-after-cancel",
				task.ConfirmRequest{PersonnelID: "P1", Generation: 1})
			if code != http.StatusConflict || env.Code != domain.CodeTerminalState {
				t.Fatalf("cancelled task accepted a later write: status=%d response=%+v", code, env)
			}

			prepareAndAcquire("replacement")
		})
	}
}
