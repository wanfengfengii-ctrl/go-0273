package httpapi

import (
	"net/http"

	"strawberry-vitro-acclimation-gate/catalog"
	"strawberry-vitro-acclimation-gate/domain"
	"strawberry-vitro-acclimation-gate/measure"
	"strawberry-vitro-acclimation-gate/pathogen"
	"strawberry-vitro-acclimation-gate/sample"
	"strawberry-vitro-acclimation-gate/task"
)

// Routes returns the configured HTTP handler.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /readyz", s.handleReadyz)

	mux.HandleFunc("POST /api/v1/catalog/mothers", s.handleUpsertMother)
	mux.HandleFunc("GET /api/v1/catalog/mothers/{id}", s.handleGetMother)
	mux.HandleFunc("POST /api/v1/catalog/media", s.handleUpsertMedium)
	mux.HandleFunc("GET /api/v1/catalog/media/{id}", s.handleGetMedium)
	mux.HandleFunc("POST /api/v1/catalog/rules", s.handleUpsertRule)
	mux.HandleFunc("GET /api/v1/catalog/rules/{id}", s.handleGetRule)
	mux.HandleFunc("POST /api/v1/catalog/personnel", s.handleUpsertPersonnel)
	mux.HandleFunc("GET /api/v1/catalog/personnel/{id}", s.handleGetPersonnel)

	mux.HandleFunc("POST /api/v1/tasks", s.handleCreateTask)
	mux.HandleFunc("POST /api/v1/tasks/{id}/lock", s.handleLockTask)
	mux.HandleFunc("GET /api/v1/tasks/{id}", s.handleGetTask)
	mux.HandleFunc("POST /api/v1/tasks/{id}/subculture-confirmations", s.handleConfirmSubculture)
	mux.HandleFunc("POST /api/v1/tasks/{id}/samples/seal", s.handleSealSamples)
	mux.HandleFunc("POST /api/v1/tasks/{id}/resources/acquire", s.handleAcquireResources)
	mux.HandleFunc("POST /api/v1/tasks/{id}/resources/switch", s.handleSwitchResource)
	mux.HandleFunc("POST /api/v1/tasks/{id}/resources/renew", s.handleRenewResource)
	mux.HandleFunc("POST /api/v1/tasks/{id}/morphology", s.handleMorphology)
	mux.HandleFunc("POST /api/v1/tasks/{id}/measurements/root", s.handleRootMeasurements)
	mux.HandleFunc("POST /api/v1/tasks/{id}/measurements/physicochemical", s.handlePhysicochemical)
	mux.HandleFunc("POST /api/v1/tasks/{id}/pathogen-calls", s.handleRunDeviceCall)
	mux.HandleFunc("POST /api/v1/device-calls/{call_id}/retry", s.handleRetryDeviceCall)
	mux.HandleFunc("POST /api/v1/tasks/{id}/blind-codes/reveal", s.handleRevealBlindCode)
	mux.HandleFunc("POST /api/v1/tasks/{id}/rechecks", s.handleCreateRecheck)
	mux.HandleFunc("POST /api/v1/tasks/{id}/rechecks/{round}/evidence", s.handleRecheckEvidence)
	mux.HandleFunc("POST /api/v1/tasks/{id}/reviews", s.handleReview)
	mux.HandleFunc("POST /api/v1/tasks/{id}/advance-review", s.handleAdvanceReview)
	mux.HandleFunc("POST /api/v1/tasks/{id}/finalize", s.handleFinalize)
	mux.HandleFunc("POST /api/v1/tasks/{id}/acclimation-confirmation", s.handleAcclimationConfirm)
	mux.HandleFunc("GET /api/v1/tasks/{id}/evidence", s.handleGetEvidence)
	mux.HandleFunc("GET /api/v1/tasks/{id}/permit", s.handleGetPermit)
	return mux
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, map[string]string{"status": "ok"})
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if s.ready == nil || s.ready() {
		WriteJSON(w, map[string]string{"status": "ready"})
		return
	}
	WriteError(w, http.StatusServiceUnavailable, domain.CodeInvalidInput, "not ready", nil)
}

func (s *Server) handleUpsertMother(w http.ResponseWriter, r *http.Request) {
	var m catalog.MotherPlant
	if _, ok := decode(w, r, &m); !ok {
		return
	}
	if err := s.catalog.SaveMother(r.Context(), m); err != nil {
		writeServiceError(w, err)
		return
	}
	WriteJSON(w, map[string]any{"id": m.ID, "version": m.Version})
}

func (s *Server) handleGetMother(w http.ResponseWriter, r *http.Request) {
	m, err := s.catalog.Mother(r.Context(), r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	WriteJSON(w, m)
}

func (s *Server) handleUpsertMedium(w http.ResponseWriter, r *http.Request) {
	var m catalog.MediumRevision
	if _, ok := decode(w, r, &m); !ok {
		return
	}
	if err := s.catalog.SaveMedium(r.Context(), m); err != nil {
		writeServiceError(w, err)
		return
	}
	WriteJSON(w, map[string]any{"formula_id": m.FormulaID, "revision": m.Revision})
}

func (s *Server) handleGetMedium(w http.ResponseWriter, r *http.Request) {
	m, err := s.catalog.Medium(r.Context(), r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	WriteJSON(w, m)
}

func (s *Server) handleUpsertRule(w http.ResponseWriter, r *http.Request) {
	var p catalog.RuleProfile
	if _, ok := decode(w, r, &p); !ok {
		return
	}
	if err := s.catalog.SaveRuleProfile(r.Context(), p); err != nil {
		writeServiceError(w, err)
		return
	}
	WriteJSON(w, map[string]any{"id": p.ID})
}

func (s *Server) handleGetRule(w http.ResponseWriter, r *http.Request) {
	p, err := s.catalog.RuleProfile(r.Context(), r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	WriteJSON(w, p)
}

func (s *Server) handleUpsertPersonnel(w http.ResponseWriter, r *http.Request) {
	var p catalog.Personnel
	if _, ok := decode(w, r, &p); !ok {
		return
	}
	if err := s.catalog.SavePersonnel(r.Context(), p); err != nil {
		writeServiceError(w, err)
		return
	}
	WriteJSON(w, map[string]any{"id": p.ID})
}

func (s *Server) handleGetPersonnel(w http.ResponseWriter, r *http.Request) {
	p, err := s.catalog.Personnel(r.Context(), r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	WriteJSON(w, p)
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var req task.CreateRequest
	if _, ok := decode(w, r, &req); !ok {
		return
	}
	t, err := s.task.Create(r.Context(), req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeServiceEnvelope(w, domain.OKEnvelope(map[string]any{"task_id": string(t.ID), "state": t.State.String()}))
}

func (s *Server) handleLockTask(w http.ResponseWriter, r *http.Request) {
	env, err := s.task.Lock(r.Context(), domain.TaskID(r.PathValue("id")))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeServiceEnvelope(w, env)
}

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	v, err := s.task.View(r.Context(), domain.TaskID(r.PathValue("id")))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	WriteJSON(w, v)
}

func (s *Server) handleConfirmSubculture(w http.ResponseWriter, r *http.Request) {
	var req task.ConfirmRequest
	digest, ok := decode(w, r, &req)
	if !ok {
		return
	}
	body, code, err := s.task.ConfirmSubculture(r.Context(), domain.TaskID(r.PathValue("id")), operationID(r), digest, req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeEnvelope(w, body, code)
}

func (s *Server) handleSealSamples(w http.ResponseWriter, r *http.Request) {
	var req sample.SealRequest
	digest, ok := decode(w, r, &req)
	if !ok {
		return
	}
	body, code, err := s.sample.SealSamples(r.Context(), domain.TaskID(r.PathValue("id")), operationID(r), digest, req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeEnvelope(w, body, code)
}

func (s *Server) handleAcquireResources(w http.ResponseWriter, r *http.Request) {
	var req sample.AcquireRequest
	digest, ok := decode(w, r, &req)
	if !ok {
		return
	}
	body, code, err := s.sample.AcquireResources(r.Context(), domain.TaskID(r.PathValue("id")), operationID(r), digest, req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeEnvelope(w, body, code)
}

func (s *Server) handleSwitchResource(w http.ResponseWriter, r *http.Request) {
	var req sample.SwitchRequest
	digest, ok := decode(w, r, &req)
	if !ok {
		return
	}
	body, code, err := s.sample.SwitchResource(r.Context(), domain.TaskID(r.PathValue("id")), operationID(r), digest, req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeEnvelope(w, body, code)
}

func (s *Server) handleRenewResource(w http.ResponseWriter, r *http.Request) {
	var req sample.RenewRequest
	digest, ok := decode(w, r, &req)
	if !ok {
		return
	}
	body, code, err := s.sample.RenewResource(r.Context(), domain.TaskID(r.PathValue("id")), operationID(r), digest, req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeEnvelope(w, body, code)
}

func (s *Server) handleMorphology(w http.ResponseWriter, r *http.Request) {
	var req measure.MorphologyRequest
	digest, ok := decode(w, r, &req)
	if !ok {
		return
	}
	body, code, err := s.measure.SubmitMorphology(r.Context(), domain.TaskID(r.PathValue("id")), operationID(r), digest, req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeEnvelope(w, body, code)
}

func (s *Server) handleRootMeasurements(w http.ResponseWriter, r *http.Request) {
	var req measure.MeasurementRequest
	digest, ok := decode(w, r, &req)
	if !ok {
		return
	}
	body, code, err := s.measure.SubmitMeasurements(r.Context(), domain.TaskID(r.PathValue("id")), operationID(r), digest,
		domain.StateVerifyingRoots, domain.StateRecheckingPathogen, true, req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeEnvelope(w, body, code)
}

func (s *Server) handlePhysicochemical(w http.ResponseWriter, r *http.Request) {
	var req measure.MeasurementRequest
	digest, ok := decode(w, r, &req)
	if !ok {
		return
	}
	body, code, err := s.measure.SubmitMeasurements(r.Context(), domain.TaskID(r.PathValue("id")), operationID(r), digest,
		domain.StateVerifyingRoots, domain.StateVerifyingRoots, false, req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeEnvelope(w, body, code)
}

func (s *Server) handleRunDeviceCall(w http.ResponseWriter, r *http.Request) {
	var req pathogen.DeviceCallRequest
	digest, ok := decode(w, r, &req)
	if !ok {
		return
	}
	body, code, err := s.pathogen.RunDeviceCall(r.Context(), domain.TaskID(r.PathValue("id")), operationID(r), digest, req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeEnvelope(w, body, code)
}

func (s *Server) handleRetryDeviceCall(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TaskID string `json:"task_id"`
	}
	digest, ok := decode(w, r, &req)
	if !ok {
		return
	}
	body, code, err := s.pathogen.RetryDeviceCall(r.Context(), domain.TaskID(req.TaskID), operationID(r), digest, r.PathValue("call_id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeEnvelope(w, body, code)
}

func (s *Server) handleRevealBlindCode(w http.ResponseWriter, r *http.Request) {
	var req sample.RevealRequest
	digest, ok := decode(w, r, &req)
	if !ok {
		return
	}
	body, code, err := s.sample.RevealBlindCode(r.Context(), domain.TaskID(r.PathValue("id")), operationID(r), digest, req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeEnvelope(w, body, code)
}

func (s *Server) handleCreateRecheck(w http.ResponseWriter, r *http.Request) {
	var req pathogen.RecheckRequest
	digest, ok := decode(w, r, &req)
	if !ok {
		return
	}
	body, code, err := s.pathogen.CreateRecheck(r.Context(), domain.TaskID(r.PathValue("id")), operationID(r), digest, req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeEnvelope(w, body, code)
}

func (s *Server) handleRecheckEvidence(w http.ResponseWriter, r *http.Request) {
	var req pathogen.RecheckEvidenceRequest
	digest, ok := decode(w, r, &req)
	if !ok {
		return
	}
	body, code, err := s.pathogen.AppendRecheckEvidence(r.Context(), domain.TaskID(r.PathValue("id")), operationID(r), digest, req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeEnvelope(w, body, code)
}

func (s *Server) handleReview(w http.ResponseWriter, r *http.Request) {
	var req task.ReviewRequest
	digest, ok := decode(w, r, &req)
	if !ok {
		return
	}
	body, code, err := s.task.Review(r.Context(), domain.TaskID(r.PathValue("id")), operationID(r), digest, req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeEnvelope(w, body, code)
}

func (s *Server) handleAdvanceReview(w http.ResponseWriter, r *http.Request) {
	digest, ok := decode(w, r, &struct{}{})
	if !ok {
		return
	}
	body, code, err := s.pathogen.AdvanceToReview(r.Context(), domain.TaskID(r.PathValue("id")), operationID(r), digest)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeEnvelope(w, body, code)
}

func (s *Server) handleFinalize(w http.ResponseWriter, r *http.Request) {
	var req pathogen.FinalizeRequest
	digest, ok := decode(w, r, &req)
	if !ok {
		return
	}
	body, code, err := s.pathogen.Finalize(r.Context(), domain.TaskID(r.PathValue("id")), operationID(r), digest, req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeEnvelope(w, body, code)
}

func (s *Server) handleAcclimationConfirm(w http.ResponseWriter, r *http.Request) {
	digest, ok := decode(w, r, &struct{}{})
	if !ok {
		return
	}
	body, code, err := s.task.ConfirmAcclimation(r.Context(), domain.TaskID(r.PathValue("id")), operationID(r), digest)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeEnvelope(w, body, code)
}

func (s *Server) handleGetEvidence(w http.ResponseWriter, r *http.Request) {
	ev, err := s.pathogen.Evidence(r.Context(), domain.TaskID(r.PathValue("id")))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	WriteJSON(w, map[string]any{"evidence": ev})
}

func (s *Server) handleGetPermit(w http.ResponseWriter, r *http.Request) {
	p, err := s.pathogen.Permit(r.Context(), domain.TaskID(r.PathValue("id")))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	WriteJSON(w, p)
}

// writeServiceError maps an error into the fixed error envelope.
func writeServiceError(w http.ResponseWriter, err error) {
	be := domain.AsBusinessError(err)
	WriteError(w, httpStatus(be.Code), be.Code, be.Message, be.Reasons)
}

// writeServiceEnvelope writes a service envelope (already serialized).
func writeServiceEnvelope(w http.ResponseWriter, env domain.Envelope) {
	body, _ := env.Bytes()
	writeEnvelope(w, body, env.Code)
}
