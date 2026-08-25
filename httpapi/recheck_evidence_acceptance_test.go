package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"strawberry-vitro-acclimation-gate/domain"
	"strawberry-vitro-acclimation-gate/pathogen"
	"strawberry-vitro-acclimation-gate/store"
	"strawberry-vitro-acclimation-gate/task"
)

func TestModel_RecheckEvidenceRequiresActiveRoundAndPreservesVersions(t *testing.T) {
	tests := []struct {
		name         string
		roundStatus  string
		requestRound int
		seedEvidence bool
		wantStatus   int
		wantCode     domain.ErrorCode
		wantVersions []int64
		wantRound    int
	}{
		{
			name:         "no active recheck cannot mint an arbitrary round",
			requestRound: 7,
			wantStatus:   http.StatusBadRequest,
			wantCode:     domain.CodeInvalidInput,
		},
		{
			name:         "body round must match the active round",
			roundStatus:  "active",
			requestRound: 7,
			wantStatus:   http.StatusBadRequest,
			wantCode:     domain.CodeInvalidInput,
		},
		{
			name:         "closed round cannot receive evidence",
			roundStatus:  "closed",
			requestRound: 1,
			wantStatus:   http.StatusBadRequest,
			wantCode:     domain.CodeInvalidInput,
		},
		{
			name:         "current active round appends a version without replacing history",
			roundStatus:  "active",
			requestRound: 1,
			seedEvidence: true,
			wantStatus:   http.StatusOK,
			wantCode:     domain.CodeOK,
			wantVersions: []int64{1, 2},
			wantRound:    1,
		},
	}

	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, err := store.Open(":memory:")
			if err != nil {
				t.Fatalf("open store: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			id := domain.TaskID(fmt.Sprintf("recheck-boundary-%d", i))
			taskService := task.NewService(db, db, domain.NewLogicalClock(0))
			if _, err := taskService.Create(context.Background(), task.CreateRequest{
				TaskID: string(id), MotherPlantID: "M1", Generation: 1,
				RTQPCRWells: []string{"W1"}, BlindCodes: []string{"BC1"},
			}); err != nil {
				t.Fatalf("create task: %v", err)
			}

			if tc.roundStatus != "" {
				if err := db.CreateRecheckRound(context.Background(), pathogen.RecheckRound{
					TaskID: id, Generation: 1, Round: 1, Status: tc.roundStatus,
					AffectedBlindCodes: []string{"BC1"}, AffectedWells: []string{"W1"},
				}); err != nil {
					t.Fatalf("create recheck round: %v", err)
				}
			}
			if tc.seedEvidence {
				if err := db.AppendEvidence(context.Background(), pathogen.PathogenEvidence{
					TaskID: id, BlindCode: "BC1", DetectionType: "virus", Well: "W1",
					Value: domain.FixedPoint{Value: 200, Scale: 1}, Generation: 1, Version: 1,
				}); err != nil {
					t.Fatalf("seed evidence: %v", err)
				}
			}

			pathogenService := pathogen.NewService(db, domain.NewLogicalClock(0))
			srv := NewServer(nil, nil, nil, nil, pathogenService, nil)
			status, env := doJSON(t, srv, http.MethodPost,
				fmt.Sprintf("/api/v1/tasks/%s/rechecks/%d/evidence", id, tc.requestRound),
				fmt.Sprintf("op-recheck-evidence-%d", i), pathogen.RecheckEvidenceRequest{
					Generation: 1, Round: tc.requestRound, BlindCode: "BC1",
					DetectionType: "virus", Well: "W1", Value: "21.0", Scale: 1,
				})
			if status != tc.wantStatus || env.Code != tc.wantCode {
				t.Fatalf("response = (%d, %s), want (%d, %s)", status, env.Code, tc.wantStatus, tc.wantCode)
			}

			evidence, err := db.EvidenceByTask(context.Background(), id)
			if err != nil {
				t.Fatalf("read evidence: %v", err)
			}
			if len(evidence) != len(tc.wantVersions) {
				t.Fatalf("evidence count = %d, want %d: %+v", len(evidence), len(tc.wantVersions), evidence)
			}
			for j, wantVersion := range tc.wantVersions {
				if evidence[j].Version != wantVersion {
					t.Fatalf("evidence[%d].version = %d, want %d", j, evidence[j].Version, wantVersion)
				}
			}
			if len(evidence) > 0 && evidence[len(evidence)-1].RecheckRound != tc.wantRound {
				t.Fatalf("latest recheck round = %d, want %d", evidence[len(evidence)-1].RecheckRound, tc.wantRound)
			}
		})
	}
}
