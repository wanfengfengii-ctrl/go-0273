// Package httpapi exposes the JSON HTTP API: request validation, stable error
// codes, deterministic reason ordering, and health checks. It composes the
// catalog, task, sample, measure, and pathogen services with the SQLite
// persistence layer without duplicating business rules.
package httpapi

import (
	"encoding/json"
	"io"
	"net/http"

	"strawberry-vitro-acclimation-gate/catalog"
	"strawberry-vitro-acclimation-gate/domain"
	"strawberry-vitro-acclimation-gate/measure"
	"strawberry-vitro-acclimation-gate/pathogen"
	"strawberry-vitro-acclimation-gate/sample"
	"strawberry-vitro-acclimation-gate/task"
)

// ErrorResponse is the fixed error envelope.
type ErrorResponse struct {
	Code    domain.ErrorCode `json:"code"`
	Message string           `json:"message"`
	Reasons []domain.Reason  `json:"reasons"`
}

// Server wires the five business services and the readiness probe.
type Server struct {
	catalog  *catalog.Service
	task     *task.Service
	sample   *sample.Service
	measure  *measure.Service
	pathogen *pathogen.Service
	ready    func() bool
}

// NewServer returns a fully wired HTTP server.
func NewServer(c *catalog.Service, t *task.Service, sm *sample.Service, m *measure.Service, p *pathogen.Service, ready func() bool) *Server {
	return &Server{catalog: c, task: t, sample: sm, measure: m, pathogen: p, ready: ready}
}

// WriteError writes a stable, canonically sorted error response.
func WriteError(w http.ResponseWriter, status int, code domain.ErrorCode, message string, reasons []domain.Reason) {
	resp := ErrorResponse{Code: code, Message: message, Reasons: domain.SortReasons(reasons)}
	writeJSON(w, status, resp)
}

// WriteJSON writes a 200 JSON body.
func WriteJSON(w http.ResponseWriter, v any) {
	writeJSON(w, http.StatusOK, v)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeEnvelope writes a service-produced envelope body with a status derived
// from its stable business code.
func writeEnvelope(w http.ResponseWriter, body []byte, code domain.ErrorCode) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus(code))
	_, _ = w.Write(body)
}

// httpStatus maps a stable business code to a protocol-level status.
func httpStatus(code domain.ErrorCode) int {
	switch code {
	case domain.CodeOK:
		return http.StatusOK
	case domain.CodeOperationContentConflict, domain.CodeTerminalState, domain.CodeDuplicateValue:
		return http.StatusConflict
	default:
		return http.StatusBadRequest
	}
}

// decode reads a JSON request body into dst and returns its canonical digest.
// It writes a stable 400 error on any parse failure.
func decode(w http.ResponseWriter, r *http.Request, dst any) (string, bool) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		WriteError(w, http.StatusBadRequest, domain.CodeInvalidInput, "read body", nil)
		return "", false
	}
	var generic any
	if err := json.Unmarshal(body, &generic); err != nil {
		WriteError(w, http.StatusBadRequest, domain.CodeInvalidInput, "invalid JSON", nil)
		return "", false
	}
	if err := json.Unmarshal(body, dst); err != nil {
		WriteError(w, http.StatusBadRequest, domain.CodeInvalidInput, "invalid JSON", nil)
		return "", false
	}
	return domain.Digest(generic), true
}

// operationID returns the X-Operation-ID header value.
func operationID(r *http.Request) domain.OperationID {
	return domain.OperationID(r.Header.Get("X-Operation-ID"))
}
