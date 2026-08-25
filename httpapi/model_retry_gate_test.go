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

func TestModel_RetryWaitsForLogicalSchedule(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	clock := domain.NewLogicalClock(0)
	catalogSvc := catalog.NewService(db)
	seedCatalog(t, catalogSvc)
	srv := NewServer(
		catalogSvc,
		task.NewService(db, db, clock),
		sample.NewService(db, clock),
		measure.NewService(db, clock),
		pathogen.NewService(db, clock),
		func() bool { return true },
	)
	id := createAndLock(t, srv)

	type testCase struct {
		name         string
		action       string
		callID       string
		op           string
		setClock     bool
		at           domain.LogicalTime
		wantHTTP     int
		wantCode     domain.ErrorCode
		wantStatus   pathogen.CallStatus
		wantAttempts int
		wantFault    int
		wantNext     domain.LogicalTime
		wantError    domain.ErrorCode
		wantEvidence int
	}

	cases := []testCase{
		{name: "first failure schedules retry", action: "start", callID: "CALL1", op: "op-start", wantHTTP: http.StatusOK, wantCode: domain.CodeOK, wantStatus: pathogen.CallFailedRetry, wantAttempts: 1, wantFault: 1, wantNext: 1, wantError: pathogen.ErrDeviceRefused},
		{name: "immediate retry is rejected", action: "retry", callID: "CALL1", op: "op-early-1", wantHTTP: http.StatusBadRequest, wantCode: domain.CodeRetryNotDue, wantStatus: pathogen.CallFailedRetry, wantAttempts: 1, wantFault: 1, wantNext: 1, wantError: pathogen.ErrDeviceRefused},
		{name: "repeated immediate retry is stable", action: "retry", callID: "CALL1", op: "op-early-2", wantHTTP: http.StatusBadRequest, wantCode: domain.CodeRetryNotDue, wantStatus: pathogen.CallFailedRetry, wantAttempts: 1, wantFault: 1, wantNext: 1, wantError: pathogen.ErrDeviceRefused},
		{name: "first due retry advances one fault", action: "retry", callID: "CALL1", op: "op-due-1", setClock: true, at: 1, wantHTTP: http.StatusOK, wantCode: domain.CodeOK, wantStatus: pathogen.CallFailedRetry, wantAttempts: 2, wantFault: 2, wantNext: 3, wantError: pathogen.ErrDeviceDisconnected},
		{name: "retry before doubled backoff is rejected", action: "retry", callID: "CALL1", op: "op-early-3", wantHTTP: http.StatusBadRequest, wantCode: domain.CodeRetryNotDue, wantStatus: pathogen.CallFailedRetry, wantAttempts: 2, wantFault: 2, wantNext: 3, wantError: pathogen.ErrDeviceDisconnected},
		{name: "second due retry follows fault script", action: "retry", callID: "CALL1", op: "op-due-2", setClock: true, at: 3, wantHTTP: http.StatusOK, wantCode: domain.CodeOK, wantStatus: pathogen.CallFailedRetry, wantAttempts: 3, wantFault: 3, wantNext: 7, wantError: pathogen.ErrDeviceTimeout},
		{name: "third due retry follows fault script", action: "retry", callID: "CALL1", op: "op-due-3", setClock: true, at: 7, wantHTTP: http.StatusOK, wantCode: domain.CodeOK, wantStatus: pathogen.CallFailedRetry, wantAttempts: 4, wantFault: 4, wantNext: 15, wantError: pathogen.ErrDeviceMalformed},
		{name: "success waits for final due time", action: "retry", callID: "CALL1", op: "op-due-success", setClock: true, at: 15, wantHTTP: http.StatusOK, wantCode: domain.CodeOK, wantStatus: pathogen.CallSucceeded, wantAttempts: 5, wantFault: 4, wantNext: 15, wantEvidence: 1},
		{name: "successful retry is idempotent", action: "retry", callID: "CALL1", op: "op-due-success", wantHTTP: http.StatusOK, wantCode: domain.CodeOK, wantStatus: pathogen.CallSucceeded, wantAttempts: 5, wantFault: 4, wantNext: 15, wantEvidence: 1},
		{name: "resolved success rejects another retry", action: "retry", callID: "CALL1", op: "op-after-success", wantHTTP: http.StatusBadRequest, wantCode: domain.CodeInvalidInput, wantStatus: pathogen.CallSucceeded, wantAttempts: 5, wantFault: 4, wantNext: 15, wantEvidence: 1},
		{name: "permanent failure rejects retry", action: "permanent", callID: "CALL-PERM", op: "op-after-permanent", wantHTTP: http.StatusBadRequest, wantCode: domain.CodeInvalidInput, wantStatus: pathogen.CallPermanentFail, wantAttempts: 9, wantFault: 4, wantNext: 15, wantError: pathogen.ErrDeviceMalformed, wantEvidence: 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setClock {
				clock.Set(tc.at)
			}
			if tc.action == "permanent" {
				err := db.SaveDeviceCall(context.Background(), pathogen.DeviceCall{
					ID: tc.callID, TaskID: domain.TaskID(id), DeviceType: pathogen.DeviceRTQPCR,
					Target: "W2", BlindCode: "BC2", DetectionType: "virus", Well: "W2",
					Status: tc.wantStatus, Attempts: tc.wantAttempts, FaultStep: tc.wantFault,
					NextRetryAt: tc.wantNext, ErrorCode: tc.wantError,
				})
				if err != nil {
					t.Fatalf("seed permanent call: %v", err)
				}
			}

			var code int
			var env domain.Envelope
			if tc.action == "start" {
				code, env = doJSON(t, srv, http.MethodPost, "/api/v1/tasks/"+id+"/pathogen-calls", tc.op, map[string]any{
					"task_generation": 1, "call_id": tc.callID, "device_type": "rt_qpcr", "target": "W1",
					"blind_code": "BC1", "detection_type": "virus", "well": "W1", "value": "20.0", "scale": 1,
				})
			} else {
				code, env = doJSON(t, srv, http.MethodPost, "/api/v1/device-calls/"+tc.callID+"/retry", tc.op, map[string]any{"task_id": id})
			}
			if code != tc.wantHTTP || env.Code != tc.wantCode {
				t.Fatalf("response = HTTP %d, code %s; want HTTP %d, code %s", code, env.Code, tc.wantHTTP, tc.wantCode)
			}

			call, err := db.DeviceCall(context.Background(), tc.callID)
			if err != nil {
				t.Fatalf("load device call: %v", err)
			}
			if call.Status != tc.wantStatus || call.Attempts != tc.wantAttempts || call.FaultStep != tc.wantFault || call.NextRetryAt != tc.wantNext || call.ErrorCode != tc.wantError {
				t.Fatalf("persisted call = {status:%s attempts:%d fault:%d next:%d error:%s}; want {status:%s attempts:%d fault:%d next:%d error:%s}",
					call.Status, call.Attempts, call.FaultStep, call.NextRetryAt, call.ErrorCode,
					tc.wantStatus, tc.wantAttempts, tc.wantFault, tc.wantNext, tc.wantError)
			}
			evidence, err := db.EvidenceByTask(context.Background(), domain.TaskID(id))
			if err != nil {
				t.Fatalf("load evidence: %v", err)
			}
			if len(evidence) != tc.wantEvidence {
				t.Fatalf("evidence count = %d; want %d", len(evidence), tc.wantEvidence)
			}
		})
	}
}
