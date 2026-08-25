package httpapi

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"strawberry-vitro-acclimation-gate/domain"
	"strawberry-vitro-acclimation-gate/task"
)

func TestModel_PermitFreezesPathogenEvidence(t *testing.T) {
	srv := newTestServer(t)
	id := createAndLock(t, srv)

	for _, personnel := range []string{"P1", "P2"} {
		code, env := doJSON(t, srv, http.MethodPost, "/api/v1/tasks/"+id+"/subculture-confirmations", "setup-confirm-"+personnel,
			task.ConfirmRequest{PersonnelID: personnel, Generation: 1})
		if code != http.StatusOK || env.Code != domain.CodeOK {
			t.Fatalf("confirm %s: status=%d envelope=%+v", personnel, code, env)
		}
	}
	setupWrites := []struct {
		name string
		path string
		op   string
		body any
	}{
		{"seal", "/api/v1/tasks/" + id + "/samples/seal", "setup-seal", sampleSealReq()},
		{"acquire", "/api/v1/tasks/" + id + "/resources/acquire", "setup-acquire", map[string]any{"task_generation": 1}},
		{"morphology", "/api/v1/tasks/" + id + "/morphology", "setup-morphology", morphReq()},
		{"roots", "/api/v1/tasks/" + id + "/measurements/root", "setup-roots", rootReq()},
	}
	for _, write := range setupWrites {
		code, env := doJSON(t, srv, http.MethodPost, write.path, write.op, write.body)
		if code != http.StatusOK || env.Code != domain.CodeOK {
			t.Fatalf("%s: status=%d envelope=%+v", write.name, code, env)
		}
	}

	callBody := map[string]any{
		"task_generation": 1, "call_id": "normal-call", "device_type": "rt_qpcr", "target": "W1",
		"blind_code": "BC1", "detection_type": "virus", "well": "W1", "value": "20.0", "scale": 1,
	}
	prePermitCases := []struct {
		name string
		run  func(*testing.T)
	}{
		{
			name: "deterministic failure creates no evidence",
			run: func(t *testing.T) {
				code, env := doJSON(t, srv, http.MethodPost, "/api/v1/tasks/"+id+"/pathogen-calls", "normal-start", callBody)
				if code != http.StatusOK || env.Code != domain.CodeOK || env.Data.(map[string]any)["status"] != "failed_retry" {
					t.Fatalf("first deterministic failure: status=%d envelope=%+v", code, env)
				}
				evidence, err := srv.pathogen.Evidence(context.Background(), domain.TaskID(id))
				if err != nil || len(evidence) != 0 {
					t.Fatalf("failed call produced evidence: evidence=%+v err=%v", evidence, err)
				}
			},
		},
		{
			name: "pending call remains retryable",
			run: func(t *testing.T) {
				code, env := doJSON(t, srv, http.MethodPost, "/api/v1/device-calls/normal-call/retry", "normal-retry-1", map[string]any{"task_id": id})
				if code != http.StatusOK || env.Code != domain.CodeOK || env.Data.(map[string]any)["status"] != "failed_retry" {
					t.Fatalf("pending retry: status=%d envelope=%+v", code, env)
				}
			},
		},
		{
			name: "eventual success appends one version",
			run: func(t *testing.T) {
				for attempt := 2; attempt <= 4; attempt++ {
					code, env := doJSON(t, srv, http.MethodPost, "/api/v1/device-calls/normal-call/retry", "normal-retry-"+string(rune('0'+attempt)), map[string]any{"task_id": id})
					if code != http.StatusOK || env.Code != domain.CodeOK {
						t.Fatalf("retry %d: status=%d envelope=%+v", attempt, code, env)
					}
				}
				evidence, err := srv.pathogen.Evidence(context.Background(), domain.TaskID(id))
				if err != nil || len(evidence) != 1 || evidence[0].Version != 1 || evidence[0].Value.Value != 200 {
					t.Fatalf("successful retry did not append exactly one expected version: evidence=%+v err=%v", evidence, err)
				}
			},
		},
	}
	for _, tc := range prePermitCases {
		t.Run(tc.name, tc.run)
	}

	runDeviceSuccess(t, srv, id, "setup-w2", "BC2", "virus", "W2", "22.0", 1)
	runDeviceSuccess(t, srv, id, "setup-ew1", "BC1", "endophyte", "EW1", "100", 0)
	runDeviceSuccess(t, srv, id, "setup-ew2", "BC2", "endophyte", "EW2", "120", 0)

	lateRetryBody := map[string]any{
		"task_generation": 1, "call_id": "late-ready", "device_type": "rt_qpcr", "target": "W1",
		"blind_code": "BC1", "detection_type": "virus", "well": "W1", "value": "21.0", "scale": 1,
	}
	code, env := doJSON(t, srv, http.MethodPost, "/api/v1/tasks/"+id+"/pathogen-calls", "setup-late-start", lateRetryBody)
	if code != http.StatusOK || env.Code != domain.CodeOK {
		t.Fatalf("prepare late retry: status=%d envelope=%+v", code, env)
	}
	for attempt := 1; attempt <= 3; attempt++ {
		code, env = doJSON(t, srv, http.MethodPost, "/api/v1/device-calls/late-ready/retry", "setup-late-retry-"+string(rune('0'+attempt)), map[string]any{"task_id": id})
		if code != http.StatusOK || env.Code != domain.CodeOK {
			t.Fatalf("prepare late retry attempt %d: status=%d envelope=%+v", attempt, code, env)
		}
	}

	code, env = doJSON(t, srv, http.MethodPost, "/api/v1/tasks/"+id+"/advance-review", "setup-advance", map[string]any{})
	if code != http.StatusOK || env.Code != domain.CodeOK {
		t.Fatalf("advance review: status=%d envelope=%+v", code, env)
	}
	for _, personnel := range []string{"R1", "R2"} {
		code, env = doJSON(t, srv, http.MethodPost, "/api/v1/tasks/"+id+"/reviews", "setup-review-"+personnel,
			task.ReviewRequest{PersonnelID: personnel, Generation: 1, EvidenceDigest: "reviewed", Conclusion: "acclimate"})
		if code != http.StatusOK || env.Code != domain.CodeOK {
			t.Fatalf("review %s: status=%d envelope=%+v", personnel, code, env)
		}
	}
	code, env = doJSON(t, srv, http.MethodPost, "/api/v1/tasks/"+id+"/finalize", "setup-finalize",
		map[string]any{"task_generation": 1, "action": "acclimate"})
	if code != http.StatusOK || env.Code != domain.CodeOK {
		t.Fatalf("issue permit: status=%d envelope=%+v", code, env)
	}

	baselineEvidence, err := srv.pathogen.Evidence(context.Background(), domain.TaskID(id))
	if err != nil {
		t.Fatalf("read baseline evidence: %v", err)
	}
	baselinePermit, err := srv.pathogen.Permit(context.Background(), domain.TaskID(id))
	if err != nil {
		t.Fatalf("read permit: %v", err)
	}
	if got := domain.Digest(baselineEvidence); got != baselinePermit.EvidenceDigest {
		t.Fatalf("permit started with a stale evidence digest: permit=%s evidence=%s", baselinePermit.EvidenceDigest, got)
	}

	postPermitCases := []struct {
		name string
		path string
		op   string
		body any
	}{
		{
			name: "new device call is rejected",
			path: "/api/v1/tasks/" + id + "/pathogen-calls",
			op:   "late-new-call",
			body: map[string]any{
				"task_generation": 1, "call_id": "late-new", "device_type": "rt_qpcr", "target": "W1",
				"blind_code": "BC1", "detection_type": "virus", "well": "W1", "value": "23.0", "scale": 1,
			},
		},
		{
			name: "successful retry is rejected",
			path: "/api/v1/device-calls/late-ready/retry",
			op:   "late-successful-retry",
			body: map[string]any{"task_id": id},
		},
		{
			name: "recheck evidence is rejected",
			path: "/api/v1/tasks/" + id + "/rechecks/1/evidence",
			op:   "late-recheck-evidence",
			body: map[string]any{
				"task_generation": 1, "round": 1, "blind_code": "BC1", "detection_type": "virus",
				"well": "W1", "value": "24.0", "scale": 1,
			},
		},
	}
	for _, tc := range postPermitCases {
		t.Run(tc.name, func(t *testing.T) {
			code, env := doJSON(t, srv, http.MethodPost, tc.path, tc.op, tc.body)
			if code != http.StatusConflict || env.Code != domain.CodeTerminalState {
				t.Fatalf("post-permit write was not terminally rejected: status=%d envelope=%+v", code, env)
			}
			currentEvidence, evidenceErr := srv.pathogen.Evidence(context.Background(), domain.TaskID(id))
			currentPermit, permitErr := srv.pathogen.Permit(context.Background(), domain.TaskID(id))
			if evidenceErr != nil || permitErr != nil {
				t.Fatalf("read after rejection: evidence_err=%v permit_err=%v", evidenceErr, permitErr)
			}
			if !reflect.DeepEqual(currentEvidence, baselineEvidence) || !reflect.DeepEqual(currentPermit, baselinePermit) {
				t.Fatalf("post-permit write changed evidence or permit: before=%+v after=%+v permit_before=%+v permit_after=%+v",
					baselineEvidence, currentEvidence, baselinePermit, currentPermit)
			}
		})
	}
}
