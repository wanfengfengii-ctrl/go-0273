package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"strawberry-vitro-acclimation-gate/domain"
)

func TestWriteErrorSortedReasons(t *testing.T) {
	rec := httptest.NewRecorder()
	reasons := []domain.Reason{
		{MotherPlantID: "B"},
		{MotherPlantID: "A"},
	}
	WriteError(rec, http.StatusBadRequest, domain.CodeDuplicateValue, "dup", reasons)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Code != domain.CodeDuplicateValue {
		t.Fatalf("code = %s", resp.Code)
	}
	if resp.Reasons[0].MotherPlantID != "A" {
		t.Fatalf("reasons not sorted: %+v", resp.Reasons)
	}
}

func TestHealthz(t *testing.T) {
	s := NewServer(nil, nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestReadyzNotReady(t *testing.T) {
	s := NewServer(nil, nil, nil, nil, nil, func() bool { return false })
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rec.Code)
	}
}
