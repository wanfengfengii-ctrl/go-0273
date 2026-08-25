package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"strawberry-vitro-acclimation-gate/catalog"
	"strawberry-vitro-acclimation-gate/domain"
	"strawberry-vitro-acclimation-gate/measure"
	"strawberry-vitro-acclimation-gate/pathogen"
	"strawberry-vitro-acclimation-gate/sample"
	"strawberry-vitro-acclimation-gate/store"
	"strawberry-vitro-acclimation-gate/task"
)

// newTestServer builds a fully wired server backed by an in-memory SQLite DB
// and seeds a minimal but complete catalog for the happy-path flow. The shared
// logical clock is returned so callers can advance deterministic time.
func newTestServer(t *testing.T) (*Server, *domain.LogicalClock) {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	clock := domain.NewLogicalClock(0)
	catalogSvc := catalog.NewService(db)
	taskSvc := task.NewService(db, db, clock)
	sampleSvc := sample.NewService(db, clock)
	measureSvc := measure.NewService(db, clock)
	pathogenSvc := pathogen.NewService(db, clock)

	seedCatalog(t, catalogSvc)
	return NewServer(catalogSvc, taskSvc, sampleSvc, measureSvc, pathogenSvc, func() bool { return true }), clock
}

func seedCatalog(t *testing.T, c *catalog.Service) {
	t.Helper()
	must(t, c.SaveMother(context.Background(), catalog.MotherPlant{ID: "M1", Strain: "s", LineageID: "L1", Enabled: true, Version: 1}))
	must(t, c.SaveMedium(context.Background(), catalog.MediumRevision{FormulaID: "F1", Revision: 1, Summary: "V1", EffectiveAt: 1}))
	must(t, c.SaveRuleProfile(context.Background(), catalog.RuleProfile{
		ID: "R1", VitrificationMaxPct: 30, BrowningMaxPct: 20, RootViabilityMin: 50,
		VirusCtMax: 350, EndophyteCFUMax: 1000, FixedScale: 2,
	}))
	for _, id := range []string{"P1", "P2", "R1", "R2"} {
		must(t, c.SavePersonnel(context.Background(), catalog.Personnel{ID: id, Qualifications: []string{"subculture", "review"}, Enabled: true}))
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func doJSON(t *testing.T, srv *Server, method, path, op string, body any) (int, domain.Envelope) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	if op != "" {
		req.Header.Set("X-Operation-ID", op)
	}
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	var env domain.Envelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	return rec.Code, env
}

func createAndLock(t *testing.T, srv *Server) string {
	t.Helper()
	create := createReq()
	code, env := doJSON(t, srv, "POST", "/api/v1/tasks", "op-create", create)
	if code != http.StatusOK || env.Code != domain.CodeOK {
		t.Fatalf("create failed: %d %+v", code, env)
	}
	code, env = doJSON(t, srv, "POST", "/api/v1/tasks/T1/lock", "op-lock", map[string]any{})
	if code != http.StatusOK || env.Code != domain.CodeOK {
		t.Fatalf("lock failed: %d %+v", code, env)
	}
	return "T1"
}

func TestFullAcclimationFlow(t *testing.T) {
	srv, clock := newTestServer(t)
	id := createAndLock(t, srv)

	// Two distinct subculture confirmations.
	for _, p := range []string{"P1", "P2"} {
		code, env := doJSON(t, srv, "POST", "/api/v1/tasks/"+id+"/subculture-confirmations", "op-sc-"+p,
			task.ConfirmRequest{PersonnelID: p, Generation: 1})
		if code != http.StatusOK || env.Code != domain.CodeOK {
			t.Fatalf("confirm %s failed: %d %+v", p, code, env)
		}
	}

	// Seal samples.
	code, env := doJSON(t, srv, "POST", "/api/v1/tasks/"+id+"/samples/seal", "op-seal",
		sampleSealReq())
	if code != http.StatusOK || env.Code != domain.CodeOK {
		t.Fatalf("seal failed: %d %+v", code, env)
	}

	// Acquire resources.
	code, env = doJSON(t, srv, "POST", "/api/v1/tasks/"+id+"/resources/acquire", "op-acq",
		map[string]any{"task_generation": 1})
	if code != http.StatusOK || env.Code != domain.CodeOK {
		t.Fatalf("acquire failed: %d %+v", code, env)
	}

	// Morphology.
	code, env = doJSON(t, srv, "POST", "/api/v1/tasks/"+id+"/morphology", "op-morph",
		morphReq())
	if code != http.StatusOK || env.Code != domain.CodeOK {
		t.Fatalf("morphology failed: %d %+v", code, env)
	}

	// Root measurements advance to pathogen recheck.
	code, env = doJSON(t, srv, "POST", "/api/v1/tasks/"+id+"/measurements/root", "op-root",
		rootReq())
	if code != http.StatusOK || env.Code != domain.CodeOK {
		t.Fatalf("root failed: %d %+v", code, env)
	}

	// Pathogen evidence via device calls with deterministic retry.
	runDeviceSuccess(t, srv, clock, id, "call-w1", "BC1", "virus", "W1", "20.0", 1)
	runDeviceSuccess(t, srv, clock, id, "call-w2", "BC2", "virus", "W2", "22.0", 1)
	runDeviceSuccess(t, srv, clock, id, "call-ew1", "BC1", "endophyte", "EW1", "100", 0)
	runDeviceSuccess(t, srv, clock, id, "call-ew2", "BC2", "endophyte", "EW2", "120", 0)

	// Advance to independent review.
	code, env = doJSON(t, srv, "POST", "/api/v1/tasks/"+id+"/advance-review", "op-adv", map[string]any{})
	if code != http.StatusOK || env.Code != domain.CodeOK {
		t.Fatalf("advance-review failed: %d %+v", code, env)
	}

	// Two independent reviews.
	for _, p := range []string{"R1", "R2"} {
		code, env = doJSON(t, srv, "POST", "/api/v1/tasks/"+id+"/reviews", "op-rev-"+p,
			task.ReviewRequest{PersonnelID: p, Generation: 1, EvidenceDigest: "d", Conclusion: "acclimate"})
		if code != http.StatusOK || env.Code != domain.CodeOK {
			t.Fatalf("review %s failed: %d %+v", p, code, env)
		}
	}

	// Finalize acclimate issues the unique permit.
	code, env = doJSON(t, srv, "POST", "/api/v1/tasks/"+id+"/finalize", "op-fin",
		map[string]any{"task_generation": 1, "action": "acclimate"})
	if code != http.StatusOK || env.Code != domain.CodeOK {
		t.Fatalf("finalize failed: %d %+v", code, env)
	}

	// Permit is retrievable.
	code, _ = doJSON(t, srv, "GET", "/api/v1/tasks/"+id+"/permit", "", nil)
	if code != http.StatusOK {
		t.Fatalf("permit GET failed: %d", code)
	}

	// Confirm acclimation to acclimated.
	code, env = doJSON(t, srv, "POST", "/api/v1/tasks/"+id+"/acclimation-confirmation", "op-confirm", map[string]any{})
	if code != http.StatusOK || env.Code != domain.CodeOK {
		t.Fatalf("acclimation-confirmation failed: %d %+v", code, env)
	}
}

// runDeviceSuccess drives one device call to a successful evidence append by
// retrying through the deterministic fault script. Each retry must wait for
// the logical clock to reach the scheduled next retry time, so the helper
// advances the clock to next_retry_at before every attempt.
func runDeviceSuccess(t *testing.T, srv *Server, clock *domain.LogicalClock, id, callID, blindCode, detType, well, value string, scale int) {
	t.Helper()
	req := map[string]any{
		"task_generation": 1, "call_id": callID, "device_type": "rt_qpcr", "target": well,
		"blind_code": blindCode, "detection_type": detType, "well": well, "value": value, "scale": scale,
	}
	code, env := doJSON(t, srv, "POST", "/api/v1/tasks/"+id+"/pathogen-calls", "op-"+callID, req)
	if code != http.StatusOK || env.Code != domain.CodeOK {
		t.Fatalf("device call %s failed: %d %+v", callID, code, env)
	}
	// Retry until succeeded (fault steps 0..3 fail, step 4 succeeds). A retry is
	// rejected with RETRY_NOT_DUE until the clock reaches next_retry_at, so
	// advance deterministic time to the scheduled moment before each attempt.
	for i := 0; i < 5; i++ {
		clock.Set(nextRetryAt(env))
		code, env = doJSON(t, srv, "POST", "/api/v1/device-calls/"+callID+"/retry", "op-retry-"+callID+"-"+string(rune('a'+i)),
			map[string]any{"task_id": id})
		if code != http.StatusOK || env.Code != domain.CodeOK {
			t.Fatalf("retry %s failed: %d %+v", callID, code, env)
		}
		if d, ok := env.Data.(map[string]any); ok && d["status"] == "succeeded" {
			return
		}
	}
	t.Fatalf("device call %s never succeeded", callID)
}

// nextRetryAt extracts the scheduled next retry time from a device-call
// response envelope. The pathogen-calls and retry endpoints both return
// next_retry_at in their data payload.
func nextRetryAt(env domain.Envelope) domain.LogicalTime {
	d, ok := env.Data.(map[string]any)
	if !ok {
		return 0
	}
	v, _ := d["next_retry_at"]
	switch n := v.(type) {
	case float64:
		return domain.LogicalTime(int64(n))
	case int64:
		return domain.LogicalTime(n)
	default:
		return 0
	}
}

func sampleSealReq() map[string]any {
	return map[string]any{
		"task_generation": 1,
		"seals": []map[string]any{
			{"seal": "S1", "position": "POS1", "blind_code_digest": "BC1"},
			{"seal": "S2", "position": "POS2", "blind_code_digest": "BC2"},
		},
	}
}

func morphReq() map[string]any {
	return map[string]any{
		"task_generation": 1,
		"cells": []map[string]any{
			{"position": "POS1", "normal": 8, "vitrified": 1, "browned": 0, "contaminated": 1, "scoring_plate_pos": "VP1"},
			{"position": "POS2", "normal": 9, "vitrified": 0, "browned": 1, "contaminated": 0, "scoring_plate_pos": "VP1"},
		},
	}
}

func rootReq() map[string]any {
	return map[string]any{
		"task_generation": 1, "scale": 2,
		"readings": []map[string]any{
			{"position": "POS1", "point": "RP1", "metric": "root_length", "value": "5.00"},
			{"position": "POS1", "point": "RP1", "metric": "ttc_viability", "value": "80.00"},
			{"position": "POS2", "point": "RP1", "metric": "root_length", "value": "6.00"},
			{"position": "POS2", "point": "RP1", "metric": "ttc_viability", "value": "85.00"},
		},
	}
}
