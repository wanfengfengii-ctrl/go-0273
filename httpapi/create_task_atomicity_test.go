package httpapi

import (
	"net/http"
	"testing"

	"strawberry-vitro-acclimation-gate/domain"
	"strawberry-vitro-acclimation-gate/task"
)

func TestModel_CreateTaskRollsBackEverySnapshotConflict(t *testing.T) {
	tests := []struct {
		name   string
		breaks func(*task.CreateRequest)
	}{
		{
			name: "duplicate bottle seal",
			breaks: func(req *task.CreateRequest) {
				req.Bottles = append(req.Bottles, task.BottleInput{Seal: req.Bottles[0].Seal, Position: "POS3", LockedSeedlings: 4})
			},
		},
		{
			name: "duplicate blind code",
			breaks: func(req *task.CreateRequest) {
				req.BlindCodes = append(req.BlindCodes, req.BlindCodes[0])
			},
		},
		{
			name: "duplicate root point",
			breaks: func(req *task.CreateRequest) {
				req.RootPoints = append(req.RootPoints, req.RootPoints[0])
			},
		},
		{
			name: "duplicate vitrification position",
			breaks: func(req *task.CreateRequest) {
				req.VitrificationPos = append(req.VitrificationPos, req.VitrificationPos[0])
			},
		},
		{
			name: "duplicate RT-qPCR well",
			breaks: func(req *task.CreateRequest) {
				req.RTQPCRWells = append(req.RTQPCRWells, req.RTQPCRWells[0])
			},
		},
		{
			name: "duplicate endophyte well",
			breaks: func(req *task.CreateRequest) {
				req.EndophyteWells = append(req.EndophyteWells, req.EndophyteWells[0])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(t)
			invalid := createReq()
			tt.breaks(&invalid)

			status, _ := doJSON(t, srv, http.MethodPost, "/api/v1/tasks", "", invalid)
			if status == http.StatusOK {
				t.Fatal("create with conflicting snapshot child unexpectedly succeeded")
			}

			status, _ = doJSON(t, srv, http.MethodGet, "/api/v1/tasks/"+invalid.TaskID, "", nil)
			if status == http.StatusOK {
				t.Fatal("failed create left a queryable draft task")
			}

			valid := createReq()
			status, env := doJSON(t, srv, http.MethodPost, "/api/v1/tasks", "", valid)
			if status != http.StatusOK || env.Code != domain.CodeOK {
				t.Fatalf("task_id remained occupied after failed create: status=%d response=%+v", status, env)
			}

			status, env = doJSON(t, srv, http.MethodGet, "/api/v1/tasks/"+valid.TaskID, "", nil)
			if status != http.StatusOK {
				t.Fatalf("successfully recreated task is not queryable: status=%d response=%+v", status, env)
			}

			status, env = doJSON(t, srv, http.MethodPost, "/api/v1/tasks/"+valid.TaskID+"/lock", "", map[string]any{})
			if status != http.StatusOK || env.Code != domain.CodeOK {
				t.Fatalf("successfully recreated task cannot be locked: status=%d response=%+v", status, env)
			}
		})
	}
}
