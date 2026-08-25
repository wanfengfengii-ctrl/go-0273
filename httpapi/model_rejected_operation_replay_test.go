package httpapi

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"strawberry-vitro-acclimation-gate/domain"
	"strawberry-vitro-acclimation-gate/task"
)

func TestModel_RejectedOperationReplayAfterLock(t *testing.T) {
	srv := newTestServer(t)
	code, env := doJSON(t, srv, "POST", "/api/v1/tasks", "model-create", createReq())
	if code != http.StatusOK || env.Code != domain.CodeOK {
		t.Fatalf("create failed: status=%d response=%+v", code, env)
	}

	var first domain.Envelope
	cases := []struct {
		name              string
		lockBefore        bool
		request           task.ConfirmRequest
		wantStatus        int
		wantCode          domain.ErrorCode
		wantMessage       string
		wantFirstResponse bool
	}{
		{
			name:        "first business rejection is recorded",
			request:     task.ConfirmRequest{PersonnelID: "P1", Generation: 1},
			wantStatus:  http.StatusBadRequest,
			wantCode:    domain.CodeInvalidInput,
			wantMessage: "not awaiting subculture confirmation",
		},
		{
			name:              "same digest replays rejection after lock",
			lockBefore:        true,
			request:           task.ConfirmRequest{PersonnelID: "P1", Generation: 1},
			wantStatus:        http.StatusBadRequest,
			wantCode:          domain.CodeInvalidInput,
			wantMessage:       "not awaiting subculture confirmation",
			wantFirstResponse: true,
		},
		{
			name:        "different digest still conflicts after lock",
			request:     task.ConfirmRequest{PersonnelID: "P2", Generation: 1},
			wantStatus:  http.StatusConflict,
			wantCode:    domain.CodeOperationContentConflict,
			wantMessage: "operation content conflict",
		},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.lockBefore {
				status, lockEnv := doJSON(t, srv, "POST", "/api/v1/tasks/T1/lock", "model-lock", map[string]any{})
				if status != http.StatusOK || lockEnv.Code != domain.CodeOK {
					t.Fatalf("lock failed: status=%d response=%+v", status, lockEnv)
				}
			}

			status, got := doJSON(t, srv, "POST", "/api/v1/tasks/T1/subculture-confirmations", "model-early-confirm", tc.request)
			if status != tc.wantStatus || got.Code != tc.wantCode || got.Message != tc.wantMessage {
				t.Fatalf("unexpected response: status=%d response=%+v; want status=%d code=%s message=%q", status, got, tc.wantStatus, tc.wantCode, tc.wantMessage)
			}
			if i == 0 {
				first = got
			}
			if tc.wantFirstResponse && !reflect.DeepEqual(got, first) {
				t.Fatalf("replay changed first response: first=%+v replay=%+v", first, got)
			}

			view, err := srv.task.View(context.Background(), "T1")
			if err != nil {
				t.Fatalf("view task: %v", err)
			}
			if view.Confirmations != 0 {
				t.Fatalf("rejected operation produced a confirmation: got %d", view.Confirmations)
			}
		})
	}
}
