package httpapi

import (
	"context"
	"net/http"
	"testing"

	"strawberry-vitro-acclimation-gate/domain"
	"strawberry-vitro-acclimation-gate/measure"
	"strawberry-vitro-acclimation-gate/task"
)

func TestModel_RootMeasurementsRequireLockedSnapshotCoverage(t *testing.T) {
	complete := []measure.MeasurementInput{
		{Position: "POS1", Point: "RP1", Metric: "root_length", Value: "5.00"},
		{Position: "POS1", Point: "RP1", Metric: "ttc_viability", Value: "80.00"},
		{Position: "POS1", Point: "RP2", Metric: "root_length", Value: "5.50"},
		{Position: "POS1", Point: "RP2", Metric: "ttc_viability", Value: "81.00"},
		{Position: "POS2", Point: "RP1", Metric: "root_length", Value: "6.00"},
		{Position: "POS2", Point: "RP1", Metric: "ttc_viability", Value: "85.00"},
		{Position: "POS2", Point: "RP2", Metric: "root_length", Value: "6.50"},
		{Position: "POS2", Point: "RP2", Metric: "ttc_viability", Value: "86.00"},
	}
	withExtra := func(extra measure.MeasurementInput) []measure.MeasurementInput {
		readings := append([]measure.MeasurementInput(nil), complete...)
		return append(readings, extra)
	}
	withReplacement := func(index int, replacement measure.MeasurementInput) []measure.MeasurementInput {
		readings := append([]measure.MeasurementInput(nil), complete...)
		readings[index] = replacement
		return readings
	}

	cases := []struct {
		name     string
		readings []measure.MeasurementInput
		wantCode domain.ErrorCode
	}{
		{
			name: "only one TTC cell leaves positions points and root lengths missing",
			readings: []measure.MeasurementInput{
				{Position: "POS1", Point: "RP1", Metric: "ttc_viability", Value: "80.00"},
			},
			wantCode: domain.CodeEvidenceIncomplete,
		},
		{
			name:     "duplicate locked cell",
			readings: withExtra(complete[0]),
			wantCode: domain.CodeEvidenceIncomplete,
		},
		{
			name:     "unknown bottle position",
			readings: withExtra(measure.MeasurementInput{Position: "POS3", Point: "RP1", Metric: "root_length", Value: "7.00"}),
			wantCode: domain.CodeEvidenceIncomplete,
		},
		{
			name:     "unknown root point",
			readings: withExtra(measure.MeasurementInput{Position: "POS1", Point: "RP3", Metric: "root_length", Value: "7.00"}),
			wantCode: domain.CodeEvidenceIncomplete,
		},
		{
			name:     "unknown root metric",
			readings: withExtra(measure.MeasurementInput{Position: "POS1", Point: "RP1", Metric: "spad", Value: "40.00"}),
			wantCode: domain.CodeEvidenceIncomplete,
		},
		{
			name:     "fixed point format remains rejected",
			readings: withReplacement(0, measure.MeasurementInput{Position: "POS1", Point: "RP1", Metric: "root_length", Value: "5.000"}),
			wantCode: domain.CodeInvalidFixedPoint,
		},
		{
			name:     "fixed point overflow remains rejected",
			readings: withReplacement(0, measure.MeasurementInput{Position: "POS1", Point: "RP1", Metric: "root_length", Value: "92233720368547758.08"}),
			wantCode: domain.CodeArithmeticOverflow,
		},
		{
			name:     "complete locked matrix advances",
			readings: complete,
			wantCode: domain.CodeOK,
		},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newTestServer(t)
			create := createReq()
			create.RootPoints = []string{"RP1", "RP2"}
			code, env := doJSON(t, srv, http.MethodPost, "/api/v1/tasks", "op-create", create)
			if code != http.StatusOK || env.Code != domain.CodeOK {
				t.Fatalf("create failed: %d %+v", code, env)
			}
			code, env = doJSON(t, srv, http.MethodPost, "/api/v1/tasks/T1/lock", "op-lock", map[string]any{})
			if code != http.StatusOK || env.Code != domain.CodeOK {
				t.Fatalf("lock failed: %d %+v", code, env)
			}
			for _, personnel := range []string{"P1", "P2"} {
				code, env = doJSON(t, srv, http.MethodPost, "/api/v1/tasks/T1/subculture-confirmations", "op-sc-"+personnel,
					task.ConfirmRequest{PersonnelID: personnel, Generation: 1})
				if code != http.StatusOK || env.Code != domain.CodeOK {
					t.Fatalf("confirmation failed: %d %+v", code, env)
				}
			}
			code, env = doJSON(t, srv, http.MethodPost, "/api/v1/tasks/T1/samples/seal", "op-seal", sampleSealReq())
			if code != http.StatusOK || env.Code != domain.CodeOK {
				t.Fatalf("seal failed: %d %+v", code, env)
			}
			code, env = doJSON(t, srv, http.MethodPost, "/api/v1/tasks/T1/resources/acquire", "op-acquire", map[string]any{"task_generation": 1})
			if code != http.StatusOK || env.Code != domain.CodeOK {
				t.Fatalf("acquire failed: %d %+v", code, env)
			}
			code, env = doJSON(t, srv, http.MethodPost, "/api/v1/tasks/T1/morphology", "op-morphology", morphReq())
			if code != http.StatusOK || env.Code != domain.CodeOK {
				t.Fatalf("morphology failed: %d %+v", code, env)
			}

			code, env = doJSON(t, srv, http.MethodPost, "/api/v1/tasks/T1/measurements/root", "op-root", measure.MeasurementRequest{
				Generation: 1,
				Scale:      2,
				Readings:   tc.readings,
			})
			if env.Code != tc.wantCode {
				t.Fatalf("case %d: code = %s (HTTP %d), want %s; response=%+v", i, env.Code, code, tc.wantCode, env)
			}

			view, err := srv.task.View(context.Background(), domain.TaskID("T1"))
			if err != nil {
				t.Fatalf("view task: %v", err)
			}
			if tc.wantCode == domain.CodeOK {
				if code != http.StatusOK || view.Task.State != domain.StateRecheckingPathogen {
					t.Fatalf("complete batch must advance: HTTP %d state=%s", code, view.Task.State.String())
				}
				return
			}
			if code == http.StatusOK || view.Task.State != domain.StateVerifyingRoots {
				t.Fatalf("rejected batch must stay verifying_roots: HTTP %d state=%s", code, view.Task.State.String())
			}

			code, env = doJSON(t, srv, http.MethodPost, "/api/v1/tasks/T1/measurements/root", "op-root-recovery", measure.MeasurementRequest{
				Generation: 1,
				Scale:      2,
				Readings:   complete,
			})
			if code != http.StatusOK || env.Code != domain.CodeOK {
				t.Fatalf("complete batch after rejection failed, indicating partial evidence was retained: %d %+v", code, env)
			}
		})
	}
}
