package pathogen

import (
	"context"
	"strconv"

	"strawberry-vitro-acclimation-gate/domain"
	"strawberry-vitro-acclimation-gate/measure"
	"strawberry-vitro-acclimation-gate/sample"
	"strawberry-vitro-acclimation-gate/task"
)

// Store is the persistence surface the pathogen service needs.
type Store interface {
	Get(ctx context.Context, id domain.TaskID) (task.Task, error)
	Snapshot(ctx context.Context, id domain.TaskID) (task.Snapshot, error)
	SetState(ctx context.Context, id domain.TaskID, from, to domain.TaskState) error
	SetFinal(ctx context.Context, id domain.TaskID, from, to domain.TaskState, final domain.FinalType) (bool, error)
	SaveDeviceCall(ctx context.Context, c DeviceCall) error
	DeviceCall(ctx context.Context, id string) (DeviceCall, error)
	PendingRetries(ctx context.Context, now domain.LogicalTime) ([]DeviceCall, error)
	AppendEvidence(ctx context.Context, e PathogenEvidence) error
	EvidenceByTask(ctx context.Context, id domain.TaskID) ([]PathogenEvidence, error)
	SaveLateArrival(ctx context.Context, a LateArrival) error
	CreateRecheckRound(ctx context.Context, r RecheckRound) error
	ActiveRecheck(ctx context.Context, gen domain.Generation) (RecheckRound, error)
	UpdateRecheck(ctx context.Context, r RecheckRound) error
	SavePermit(ctx context.Context, p AcclimationPermit) error
	Permit(ctx context.Context, taskID domain.TaskID) (AcclimationPermit, error)
	MorphologyCells(ctx context.Context, id domain.TaskID) ([]measure.MorphologyCell, error)
	MeasurementCells(ctx context.Context, id domain.TaskID) ([]measure.MeasurementCell, error)
	SubcultureConfirmations(ctx context.Context, id domain.TaskID) ([]task.SubcultureConfirmation, error)
	IndependentReviews(ctx context.Context, id domain.TaskID) ([]task.IndependentReview, error)
	ActiveLeasesByTask(ctx context.Context, id domain.TaskID) ([]sample.ResourceLease, error)
	ReleaseLease(ctx context.Context, resourceType sample.ResourceType, key string) error
	FindOperation(ctx context.Context, id domain.TaskID, op domain.OperationID) (task.OperationRecord, bool, error)
	SaveOperation(ctx context.Context, rec task.OperationRecord) error
}

// Service implements the pathogen recheck and terminal arbitration component:
// scriptable device calls with deterministic retry, append-only evidence
// chains, recheck rounds, evidence closure, and the unique acclimation permit.
type Service struct {
	store Store
	clock *domain.LogicalClock
}

// NewService wires the pathogen service to its store and logical clock.
func NewService(store Store, clock *domain.LogicalClock) *Service {
	return &Service{store: store, clock: clock}
}

// DeviceCallRequest drives one device invocation.
type DeviceCallRequest struct {
	CallID        string            `json:"call_id"`
	Generation    domain.Generation `json:"task_generation"`
	DeviceType    DeviceType        `json:"device_type"`
	Target        string            `json:"target"`
	BlindCode     string            `json:"blind_code"`
	DetectionType string            `json:"detection_type"`
	Well          string            `json:"well"`
	Value         string            `json:"value"`
	Scale         int               `json:"scale"`
}

// RunDeviceCall creates a device call and executes its first attempt. A failure
// records a retry schedule and produces no evidence; only a successful call
// appends an evidence version.
func (s *Service) RunDeviceCall(ctx context.Context, id domain.TaskID, op domain.OperationID, digest string, req DeviceCallRequest) ([]byte, domain.ErrorCode, error) {
	return task.RunIdempotent(ctx, s.store, id, op, digest, func() (domain.Envelope, error) {
		t, err := s.store.Get(ctx, id)
		if err != nil {
			return domain.Envelope{}, err
		}
		if t.State.IsTerminal() {
			return domain.Envelope{}, domain.NewError(domain.CodeTerminalState, "task is terminal")
		}
		if t.Generation != req.Generation {
			return domain.Envelope{}, domain.NewError(domain.CodeInvalidInput, "stale generation")
		}
		fp, err := domain.ParseFixedPoint(req.Value, 24, req.Scale, req.DetectionType == "endophyte")
		if err != nil {
			return domain.Envelope{}, domain.NewError(domain.ErrorCodeOf(err), "invalid fixed point")
		}
		call := DeviceCall{
			ID: req.CallID, TaskID: id, DeviceType: req.DeviceType, Target: req.Target,
			BlindCode: req.BlindCode, DetectionType: req.DetectionType, Well: req.Well,
			EvidenceValue: fp.Value, EvidenceScale: fp.Scale,
			Status: CallPending, PayloadDigest: domain.Digest(map[string]any{"target": req.Target, "value": req.Value}),
		}
		if err := s.execute(ctx, t, &call); err != nil {
			return domain.Envelope{}, err
		}
		if err := s.store.SaveDeviceCall(ctx, call); err != nil {
			return domain.Envelope{}, err
		}
		return domain.OKEnvelope(map[string]any{"call_id": call.ID, "status": string(call.Status), "attempts": call.Attempts, "next_retry_at": call.NextRetryAt}), nil
	})
}

// RetryDeviceCall advances a pending or failed device call one deterministic
// step. A retry is rejected with RETRY_NOT_DUE until the logical clock reaches
// the call's scheduled next retry time. Failures never produce evidence or
// release leases.
func (s *Service) RetryDeviceCall(ctx context.Context, id domain.TaskID, op domain.OperationID, digest string, callID string) ([]byte, domain.ErrorCode, error) {
	return task.RunIdempotent(ctx, s.store, id, op, digest, func() (domain.Envelope, error) {
		call, err := s.store.DeviceCall(ctx, callID)
		if err != nil {
			return domain.Envelope{}, domain.NewError(domain.CodeInvalidInput, "unknown device call")
		}
		if call.TaskID != id {
			return domain.Envelope{}, domain.NewError(domain.CodeInvalidInput, "device call belongs to another task")
		}
		t, err := s.store.Get(ctx, id)
		if err != nil {
			return domain.Envelope{}, err
		}
		if t.State.IsTerminal() {
			return domain.Envelope{}, domain.NewError(domain.CodeTerminalState, "task is terminal")
		}
		if call.Status == CallSucceeded || call.Status == CallPermanentFail {
			return domain.Envelope{}, domain.NewError(domain.CodeInvalidInput, "call already resolved")
		}
		// A scheduled retry may only run once the logical clock has reached the
		// next retry time. Without this gate a caller could fire retries back to
		// back and run the fault script ahead of its deterministic schedule.
		if call.NextRetryAt > 0 && s.clock.Now() < call.NextRetryAt {
			return domain.Envelope{}, domain.NewError(domain.CodeRetryNotDue, "retry not yet due")
		}
		if err := s.execute(ctx, t, &call); err != nil {
			return domain.Envelope{}, err
		}
		if err := s.store.SaveDeviceCall(ctx, call); err != nil {
			return domain.Envelope{}, err
		}
		return domain.OKEnvelope(map[string]any{"call_id": call.ID, "status": string(call.Status), "attempts": call.Attempts, "next_retry_at": call.NextRetryAt}), nil
	})
}

// execute runs one deterministic step of a device call: on success it appends
// an evidence version; on failure it advances the retry schedule.
func (s *Service) execute(ctx context.Context, t task.Task, call *DeviceCall) error {
	status, errCode := stepOutcome(call.FaultStep)
	if status == CallSucceeded {
		call.Status = CallSucceeded
		call.Attempts++
		call.ErrorCode = ""
		ev, err := s.store.EvidenceByTask(ctx, t.ID)
		if err != nil {
			return err
		}
		version := int64(1)
		for _, e := range ev {
			if e.BlindCode == call.BlindCode && e.DetectionType == call.DetectionType && e.Well == call.Well && e.Version >= version {
				version = e.Version + 1
			}
		}
		return s.store.AppendEvidence(ctx, PathogenEvidence{
			TaskID: t.ID, BlindCode: call.BlindCode, DetectionType: call.DetectionType, Well: call.Well,
			Value:      domain.FixedPoint{Value: call.EvidenceValue, Scale: call.EvidenceScale},
			Generation: t.Generation, RecheckRound: 0, Version: version,
		})
	}
	call.Status = CallFailedRetry
	call.Attempts++
	call.ErrorCode = errCode
	call.FaultStep++
	call.NextRetryAt = s.clock.Now() + backoff(call.Attempts)
	return nil
}

// RecheckRequest triggers a unique recheck round for the current generation.
type RecheckRequest struct {
	Generation domain.Generation `json:"task_generation"`
	Trigger    Trigger           `json:"trigger"`
}

// CreateRecheck computes the arbitration reasons, ensures a single active
// recheck round for the generation, and records the affected object sets.
func (s *Service) CreateRecheck(ctx context.Context, id domain.TaskID, op domain.OperationID, digest string, req RecheckRequest) ([]byte, domain.ErrorCode, error) {
	return task.RunIdempotent(ctx, s.store, id, op, digest, func() (domain.Envelope, error) {
		t, err := s.store.Get(ctx, id)
		if err != nil {
			return domain.Envelope{}, err
		}
		if t.State.IsTerminal() {
			return domain.Envelope{}, domain.NewError(domain.CodeTerminalState, "task is terminal")
		}
		if t.Generation != req.Generation {
			return domain.Envelope{}, domain.NewError(domain.CodeInvalidInput, "stale generation")
		}
		snap, err := s.store.Snapshot(ctx, id)
		if err != nil {
			return domain.Envelope{}, err
		}
		mcells, _ := s.store.MorphologyCells(ctx, id)
		xcells, _ := s.store.MeasurementCells(ctx, id)
		ev, _ := s.store.EvidenceByTask(ctx, id)
		reasons := Arbitrate(mcells, xcells, ev, snap.Thresholds)
		if len(reasons) == 0 {
			return domain.Envelope{}, domain.NewError(domain.CodeInvalidInput, "no anomaly to recheck")
		}
		if _, err := s.store.ActiveRecheck(ctx, t.Generation); err == nil {
			return domain.Envelope{}, domain.NewError(domain.CodeDuplicateValue, "recheck round already active")
		}
		round := RecheckRound{
			TaskID: id, Generation: t.Generation, Round: 1, Trigger: string(req.Trigger),
			AffectedPositions:  uniqueSorted(reasonPositions(reasons, snap.BottlePositions)),
			AffectedBlindCodes: snap.BlindCodes,
			AffectedPoints:     snap.RootPoints,
			AffectedWells:      uniqueSorted(reasonWells(reasons, append(append([]string{}, snap.RTQPCRWells...), snap.EndophyteWells...))),
			Status:             "active",
		}
		if err := s.store.CreateRecheckRound(ctx, round); err != nil {
			return domain.Envelope{}, err
		}
		return domain.OKEnvelope(map[string]any{"round": round.Round, "affected_positions": round.AffectedPositions}), nil
	})
}

// RecheckEvidenceRequest appends evidence under a specific recheck round.
type RecheckEvidenceRequest struct {
	Generation    domain.Generation `json:"task_generation"`
	Round         int               `json:"round"`
	BlindCode     string            `json:"blind_code"`
	DetectionType string            `json:"detection_type"`
	Well          string            `json:"well"`
	Value         string            `json:"value"`
	Scale         int               `json:"scale"`
}

// AppendRecheckEvidence appends a new evidence version under a recheck round
// without overwriting history.
func (s *Service) AppendRecheckEvidence(ctx context.Context, id domain.TaskID, op domain.OperationID, digest string, req RecheckEvidenceRequest) ([]byte, domain.ErrorCode, error) {
	return task.RunIdempotent(ctx, s.store, id, op, digest, func() (domain.Envelope, error) {
		t, err := s.store.Get(ctx, id)
		if err != nil {
			return domain.Envelope{}, err
		}
		if t.State.IsTerminal() {
			return domain.Envelope{}, domain.NewError(domain.CodeTerminalState, "task is terminal")
		}
		if t.Generation != req.Generation {
			return domain.Envelope{}, domain.NewError(domain.CodeInvalidInput, "stale generation")
		}
		fp, err := domain.ParseFixedPoint(req.Value, 24, req.Scale, req.DetectionType == "endophyte")
		if err != nil {
			return domain.Envelope{}, domain.NewError(domain.ErrorCodeOf(err), "invalid fixed point")
		}
		ev, _ := s.store.EvidenceByTask(ctx, id)
		version := int64(1)
		for _, e := range ev {
			if e.BlindCode == req.BlindCode && e.DetectionType == req.DetectionType && e.Well == req.Well && e.Version >= version {
				version = e.Version + 1
			}
		}
		if err := s.store.AppendEvidence(ctx, PathogenEvidence{
			TaskID: id, BlindCode: req.BlindCode, DetectionType: req.DetectionType, Well: req.Well,
			Value:      domain.FixedPoint{Value: fp.Value, Scale: fp.Scale},
			Generation: t.Generation, RecheckRound: req.Round, Version: version,
		}); err != nil {
			return domain.Envelope{}, err
		}
		return domain.OKEnvelope(map[string]any{"version": version}), nil
	})
}

// EvidenceClosure reports whether every locked well has a virus or endophyte
// evidence version, returning the sorted list of gaps.
func (s *Service) EvidenceClosure(ctx context.Context, id domain.TaskID) (bool, []domain.Reason, error) {
	snap, err := s.store.Snapshot(ctx, id)
	if err != nil {
		return false, nil, err
	}
	ev, _ := s.store.EvidenceByTask(ctx, id)
	virusWells := map[string]bool{}
	endoWells := map[string]bool{}
	for _, e := range ev {
		if e.DetectionType == "virus" {
			virusWells[e.Well] = true
		}
		if e.DetectionType == "endophyte" {
			endoWells[e.Well] = true
		}
	}
	var missing []domain.Reason
	for _, w := range snap.RTQPCRWells {
		if !virusWells[w] {
			missing = append(missing, domain.Reason{TestWell: w, Field: "virus"})
		}
	}
	for _, w := range snap.EndophyteWells {
		if !endoWells[w] {
			missing = append(missing, domain.Reason{TestWell: w, Field: "endophyte"})
		}
	}
	return len(missing) == 0, domain.SortReasons(missing), nil
}

// AdvanceToReview transitions a task whose evidence is closed from pathogen
// recheck to pending independent review.
func (s *Service) AdvanceToReview(ctx context.Context, id domain.TaskID, op domain.OperationID, digest string) ([]byte, domain.ErrorCode, error) {
	return task.RunIdempotent(ctx, s.store, id, op, digest, func() (domain.Envelope, error) {
		t, err := s.store.Get(ctx, id)
		if err != nil {
			return domain.Envelope{}, err
		}
		if t.State != domain.StateRecheckingPathogen {
			return domain.Envelope{}, domain.NewError(domain.CodeInvalidInput, "not in pathogen recheck state")
		}
		closed, missing, err := s.EvidenceClosure(ctx, id)
		if err != nil {
			return domain.Envelope{}, err
		}
		if !closed {
			return domain.Envelope{}, domain.NewError(domain.CodeEvidenceIncomplete, "evidence not closed").WithReasons(missing...)
		}
		if err := s.store.SetState(ctx, id, domain.StateRecheckingPathogen, domain.StatePendingReview); err != nil {
			return domain.Envelope{}, err
		}
		return domain.OKEnvelope(map[string]any{"state": domain.StatePendingReview.String()}), nil
	})
}

// FinalizeRequest is the terminal arbitration payload.
type FinalizeRequest struct {
	Generation domain.Generation `json:"task_generation"`
	Action     domain.FinalType  `json:"action"`
}

// Finalize applies the single-writer terminal barrier for acclimate, isolate,
// or cancel. Acclimate requires evidence closure and two independent reviews,
// then issues the unique permit.
func (s *Service) Finalize(ctx context.Context, id domain.TaskID, op domain.OperationID, digest string, req FinalizeRequest) ([]byte, domain.ErrorCode, error) {
	return task.RunIdempotent(ctx, s.store, id, op, digest, func() (domain.Envelope, error) {
		t, err := s.store.Get(ctx, id)
		if err != nil {
			return domain.Envelope{}, err
		}
		if t.State.IsTerminal() {
			return domain.Envelope{}, domain.NewError(domain.CodeTerminalState, "task is terminal")
		}
		if t.Generation != req.Generation {
			return domain.Envelope{}, domain.NewError(domain.CodeInvalidInput, "stale generation")
		}
		switch req.Action {
		case domain.FinalAcclimate:
			if t.State != domain.StatePendingReview {
				return domain.Envelope{}, domain.NewError(domain.CodeInvalidInput, "cannot acclimate before independent review")
			}
			reviews, _ := s.store.IndependentReviews(ctx, id)
			if len(reviews) < 2 {
				return domain.Envelope{}, domain.NewError(domain.CodeEvidenceIncomplete, "two independent reviews required")
			}
			snap, _ := s.store.Snapshot(ctx, id)
			mcells, _ := s.store.MorphologyCells(ctx, id)
			xcells, _ := s.store.MeasurementCells(ctx, id)
			ev, _ := s.store.EvidenceByTask(ctx, id)
			if reasons := Arbitrate(mcells, xcells, ev, snap.Thresholds); len(reasons) > 0 {
				return domain.Envelope{}, domain.NewError(domain.CodeEvidenceIncomplete, "arbitration does not pass").WithReasons(reasons...)
			}
			won, err := s.store.SetFinal(ctx, id, domain.StatePendingReview, domain.StateAcclimatable, domain.FinalAcclimate)
			if err != nil {
				return domain.Envelope{}, err
			}
			if !won {
				return domain.Envelope{}, domain.NewError(domain.CodeTerminalState, "concurrent finalize lost")
			}
			serial := permitSerial(id, reviews)
			if err := s.store.SavePermit(ctx, AcclimationPermit{
				Serial: serial, TaskID: id, LockDigest: snap.LockSummary,
				EvidenceDigest: domain.Digest(ev), Reviewers: reviewerIDs(reviews), IssuedAt: s.clock.Now(),
			}); err != nil {
				return domain.Envelope{}, err
			}
			return domain.OKEnvelope(map[string]any{"state": domain.StateAcclimatable.String(), "permit_serial": serial}), nil
		case domain.FinalIsolate:
			won, err := s.store.SetFinal(ctx, id, t.State, domain.StateIsolated, domain.FinalIsolate)
			if err != nil {
				return domain.Envelope{}, err
			}
			if !won {
				return domain.Envelope{}, domain.NewError(domain.CodeTerminalState, "concurrent finalize lost")
			}
			s.releaseLeases(ctx, id)
			return domain.OKEnvelope(map[string]any{"state": domain.StateIsolated.String()}), nil
		case domain.FinalCancel:
			won, err := s.store.SetFinal(ctx, id, t.State, domain.StateCancelled, domain.FinalCancel)
			if err != nil {
				return domain.Envelope{}, err
			}
			if !won {
				return domain.Envelope{}, domain.NewError(domain.CodeTerminalState, "concurrent finalize lost")
			}
			s.releaseLeases(ctx, id)
			return domain.OKEnvelope(map[string]any{"state": domain.StateCancelled.String()}), nil
		default:
			return domain.Envelope{}, domain.NewError(domain.CodeInvalidInput, "unknown finalize action")
		}
	})
}

// Evidence returns the versioned evidence chain for a task.
func (s *Service) Evidence(ctx context.Context, id domain.TaskID) ([]PathogenEvidence, error) {
	return s.store.EvidenceByTask(ctx, id)
}

// Permit returns the acclimation permit for a task.
func (s *Service) Permit(ctx context.Context, id domain.TaskID) (AcclimationPermit, error) {
	return s.store.Permit(ctx, id)
}

// releaseLeases closes every active lease held by a task on cancel or isolate.
func (s *Service) releaseLeases(ctx context.Context, id domain.TaskID) {
	leases, err := s.store.ActiveLeasesByTask(ctx, id)
	if err != nil {
		return
	}
	for _, l := range leases {
		_ = s.store.ReleaseLease(ctx, l.ResourceType, l.ResourceKey)
	}
}

func permitSerial(id domain.TaskID, reviews []task.IndependentReview) string {
	return "PERMIT-" + string(id) + "-" + strconv.Itoa(len(reviews))
}

func reviewerIDs(reviews []task.IndependentReview) []string {
	ids := make([]string, 0, len(reviews))
	for _, r := range reviews {
		ids = append(ids, r.PersonnelID)
	}
	return ids
}

func reasonPositions(reasons []domain.Reason, all []string) []string {
	var out []string
	for _, r := range reasons {
		if r.BottlePosition != "" {
			out = append(out, r.BottlePosition)
		}
	}
	if len(out) == 0 {
		return all
	}
	return out
}

func reasonWells(reasons []domain.Reason, all []string) []string {
	var out []string
	for _, r := range reasons {
		if r.TestWell != "" {
			out = append(out, r.TestWell)
		}
	}
	if len(out) == 0 {
		return all
	}
	return out
}

func uniqueSorted(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return domain.SortStrings(out)
}
