package task

import (
	"context"

	"strawberry-vitro-acclimation-gate/catalog"
	"strawberry-vitro-acclimation-gate/domain"
)

// TaskStore is the persistence surface the task service needs. It is satisfied
// by the SQLite-backed store and, in tests, by lightweight fakes.
type TaskStore interface {
	Get(ctx context.Context, id domain.TaskID) (Task, error)
	Snapshot(ctx context.Context, id domain.TaskID) (Snapshot, error)
	CreateTask(ctx context.Context, t Task, snap Snapshot) error
	LockTask(ctx context.Context, id domain.TaskID, snap Snapshot) error
	SetState(ctx context.Context, id domain.TaskID, from, to domain.TaskState) error
	SetFinal(ctx context.Context, id domain.TaskID, from, to domain.TaskState, final domain.FinalType) (bool, error)
	AddSubcultureConfirmation(ctx context.Context, c SubcultureConfirmation) error
	SubcultureConfirmations(ctx context.Context, id domain.TaskID) ([]SubcultureConfirmation, error)
	AddIndependentReview(ctx context.Context, r IndependentReview) error
	IndependentReviews(ctx context.Context, id domain.TaskID) ([]IndependentReview, error)
	SaveFinalDecision(ctx context.Context, d FinalDecision) error
	FinalDecision(ctx context.Context, id domain.TaskID) (FinalDecision, error)
	FindOperation(ctx context.Context, id domain.TaskID, op domain.OperationID) (OperationRecord, bool, error)
	SaveOperation(ctx context.Context, rec OperationRecord) error
}

// CatalogStore is the catalog read surface needed for lock-time validation.
type CatalogStore interface {
	Mother(ctx context.Context, id string) (catalog.MotherPlant, error)
	Medium(ctx context.Context, formulaID string) (catalog.MediumRevision, error)
	Personnel(ctx context.Context, id string) (catalog.Personnel, error)
}

// Service implements the acclimation task aggregate business flows: the
// one-shot lock gate, dual-person subculture confirmation, independent review,
// and the single-writer terminal barrier.
type Service struct {
	tasks   TaskStore
	catalog CatalogStore
	clock   *domain.LogicalClock
}

// NewService wires the task service to its stores and logical clock.
func NewService(tasks TaskStore, catalog CatalogStore, clock *domain.LogicalClock) *Service {
	return &Service{tasks: tasks, catalog: catalog, clock: clock}
}

// BottleInput describes one bottle seal, position, and locked seedling count.
type BottleInput struct {
	Seal            string `json:"seal"`
	Position        string `json:"position"`
	LockedSeedlings int    `json:"locked_seedlings"`
}

// CreateRequest carries every field required to lock a task.
type CreateRequest struct {
	TaskID            string            `json:"task_id"`
	MotherPlantID     string            `json:"mother_plant_id"`
	LineageID         string            `json:"lineage_id"`
	MediumFormulaID   string            `json:"medium_formula_id"`
	MediumSummary     string            `json:"medium_summary"`
	SubcultureBatch   string            `json:"subculture_batch"`
	Bottles           []BottleInput     `json:"bottles"`
	BlindCodes        []string          `json:"blind_codes"`
	RootPoints        []string          `json:"root_points"`
	VitrificationPos  []string          `json:"vitrification_positions"`
	RTQPCRWells       []string          `json:"rt_pcr_wells"`
	EndophyteWells    []string          `json:"endophyte_wells"`
	RackShelf         string            `json:"rack_shelf"`
	LightWindow       string            `json:"light_window"`
	AcclimationWindow string            `json:"acclimation_window"`
	Thresholds        Thresholds        `json:"thresholds"`
	AllowedPersonnel  []string          `json:"allowed_personnel"`
	Generation        domain.Generation `json:"task_generation"`
}

// Create persists a new draft task and its snapshot child rows. The snapshot is
// immutable from creation onward; the lock step only validates and transitions.
func (s *Service) Create(ctx context.Context, req CreateRequest) (Task, error) {
	if req.TaskID == "" || req.MotherPlantID == "" {
		return Task{}, domain.NewError(domain.CodeInvalidInput, "task_id and mother_plant_id are required")
	}
	snap := snapshotFromRequest(req)
	now := s.clock.Now()
	t := Task{
		ID:           domain.TaskID(req.TaskID),
		State:        domain.StateDraft,
		Generation:   req.Generation,
		StateVersion: 0,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.tasks.CreateTask(ctx, t, snap); err != nil {
		return Task{}, err
	}
	return t, nil
}

func snapshotFromRequest(req CreateRequest) Snapshot {
	snap := Snapshot{
		TaskID:            domain.TaskID(req.TaskID),
		MotherPlantID:     req.MotherPlantID,
		LineageID:         req.LineageID,
		MediumFormulaID:   req.MediumFormulaID,
		MediumSummary:     req.MediumSummary,
		SubcultureBatch:   req.SubcultureBatch,
		RackShelf:         req.RackShelf,
		LightWindow:       req.LightWindow,
		AcclimationWindow: req.AcclimationWindow,
		Thresholds:        req.Thresholds,
		AllowedPersonnel:  req.AllowedPersonnel,
		Generation:        req.Generation,
		LockedSeedlings:   map[string]int{},
	}
	for _, b := range req.Bottles {
		snap.BottleSeals = append(snap.BottleSeals, b.Seal)
		snap.BottlePositions = append(snap.BottlePositions, b.Position)
		snap.LockedSeedlings[b.Position] = b.LockedSeedlings
	}
	snap.BlindCodes = append(snap.BlindCodes, req.BlindCodes...)
	snap.RootPoints = append(snap.RootPoints, req.RootPoints...)
	snap.VitrificationPos = append(snap.VitrificationPos, req.VitrificationPos...)
	snap.RTQPCRWells = append(snap.RTQPCRWells, req.RTQPCRWells...)
	snap.EndophyteWells = append(snap.EndophyteWells, req.EndophyteWells...)
	snap.LockSummary = domain.Digest(snap)
	return snap
}

// Lock validates the catalog against the snapshot and atomically applies the
// one-shot lock gate. It fails whole if the lineage mismatches, the medium
// summary is stale, or any identifier collides with another open task.
func (s *Service) Lock(ctx context.Context, id domain.TaskID) (domain.Envelope, error) {
	t, err := s.tasks.Get(ctx, id)
	if err != nil {
		return domain.Envelope{}, err
	}
	if t.State != domain.StateDraft {
		return domain.Envelope{}, domain.NewError(domain.CodeInvalidInput, "task is not in draft state")
	}
	snap, err := s.tasks.Snapshot(ctx, id)
	if err != nil {
		return domain.Envelope{}, err
	}

	mother, err := s.catalog.Mother(ctx, snap.MotherPlantID)
	if err != nil || !catalog.LineageMatches(mother, snap.LineageID) {
		return domain.Envelope{}, domain.NewError(domain.CodeLineageMismatch, "lineage mismatch").
			WithReason(domain.Reason{MotherPlantID: snap.MotherPlantID, Field: "lineage_id", Detail: snap.LineageID})
	}
	medium, err := s.catalog.Medium(ctx, snap.MediumFormulaID)
	if err != nil || !catalog.MediumFresh(medium, snap.MediumSummary) {
		return domain.Envelope{}, domain.NewError(domain.CodeStaleMediumSummary, "stale medium summary").
			WithReason(domain.Reason{MotherPlantID: snap.MotherPlantID, Field: "medium_summary", Detail: snap.MediumSummary})
	}

	if err := s.tasks.LockTask(ctx, id, snap); err != nil {
		return domain.Envelope{}, err
	}
	return domain.OKEnvelope(map[string]any{"task_id": string(id), "state": domain.StatePendingSubculture.String(), "lock_summary": snap.LockSummary}), nil
}

// ConfirmSubculture records one of the two required subculture confirmations.
// It enforces idempotency, current-generation checks, and role separation.
func (s *Service) ConfirmSubculture(ctx context.Context, id domain.TaskID, op domain.OperationID, digest string, req ConfirmRequest) ([]byte, domain.ErrorCode, error) {
	return RunIdempotent(ctx, s.tasks, id, op, digest, func() (domain.Envelope, error) {
		t, err := s.tasks.Get(ctx, id)
		if err != nil {
			return domain.Envelope{}, err
		}
		if t.State.IsTerminal() {
			return domain.Envelope{}, domain.NewError(domain.CodeTerminalState, "task is terminal")
		}
		if t.Generation != req.Generation {
			return domain.Envelope{}, domain.NewError(domain.CodeInvalidInput, "stale generation")
		}
		if t.State != domain.StatePendingSubculture {
			return domain.Envelope{}, domain.NewError(domain.CodeInvalidInput, "not awaiting subculture confirmation")
		}
		// Validate qualification against the locked snapshot's allowed list.
		snap, err := s.tasks.Snapshot(ctx, id)
		if err != nil {
			return domain.Envelope{}, err
		}
		if !contains(snap.AllowedPersonnel, req.PersonnelID) {
			return domain.Envelope{}, domain.NewError(domain.CodeNotAuthorized, "person not allowed")
		}
		confs, _ := s.tasks.SubcultureConfirmations(ctx, id)
		for _, c := range confs {
			if c.PersonnelID == req.PersonnelID {
				return domain.Envelope{}, domain.NewError(domain.CodeRoleOverlap, "person already confirmed")
			}
		}
		if err := s.tasks.AddSubcultureConfirmation(ctx, SubcultureConfirmation{
			TaskID: id, Generation: t.Generation, PersonnelID: req.PersonnelID,
			OperationID: op, ConfirmedAt: s.clock.Now(),
		}); err != nil {
			return domain.Envelope{}, err
		}
		confs, _ = s.tasks.SubcultureConfirmations(ctx, id)
		if len(confs) >= 2 {
			if err := s.tasks.SetState(ctx, id, domain.StatePendingSubculture, domain.StateSealingSamples); err != nil {
				return domain.Envelope{}, err
			}
		}
		return domain.OKEnvelope(map[string]any{"confirmations": len(confs)}), nil
	})
}

// Review records one of the two independent reviews.
func (s *Service) Review(ctx context.Context, id domain.TaskID, op domain.OperationID, digest string, req ReviewRequest) ([]byte, domain.ErrorCode, error) {
	return RunIdempotent(ctx, s.tasks, id, op, digest, func() (domain.Envelope, error) {
		t, err := s.tasks.Get(ctx, id)
		if err != nil {
			return domain.Envelope{}, err
		}
		if t.State.IsTerminal() {
			return domain.Envelope{}, domain.NewError(domain.CodeTerminalState, "task is terminal")
		}
		if t.Generation != req.Generation {
			return domain.Envelope{}, domain.NewError(domain.CodeInvalidInput, "stale generation")
		}
		if t.State != domain.StatePendingReview {
			return domain.Envelope{}, domain.NewError(domain.CodeInvalidInput, "not awaiting independent review")
		}
		snap, err := s.tasks.Snapshot(ctx, id)
		if err != nil {
			return domain.Envelope{}, err
		}
		if !contains(snap.AllowedPersonnel, req.PersonnelID) {
			return domain.Envelope{}, domain.NewError(domain.CodeNotAuthorized, "person not allowed")
		}
		// The reviewer must not overlap with any subculture confirmer.
		confs, _ := s.tasks.SubcultureConfirmations(ctx, id)
		for _, c := range confs {
			if c.PersonnelID == req.PersonnelID {
				return domain.Envelope{}, domain.NewError(domain.CodeRoleOverlap, "reviewer overlaps with subculture confirmer")
			}
		}
		reviews, _ := s.tasks.IndependentReviews(ctx, id)
		for _, r := range reviews {
			if r.PersonnelID == req.PersonnelID {
				return domain.Envelope{}, domain.NewError(domain.CodeRoleOverlap, "duplicate reviewer")
			}
		}
		if err := s.tasks.AddIndependentReview(ctx, IndependentReview{
			TaskID: id, Seq: len(reviews) + 1, PersonnelID: req.PersonnelID,
			EvidenceDigest: req.EvidenceDigest, Conclusion: req.Conclusion,
		}); err != nil {
			return domain.Envelope{}, err
		}
		reviews, _ = s.tasks.IndependentReviews(ctx, id)
		return domain.OKEnvelope(map[string]any{"reviews": len(reviews)}), nil
	})
}

// ConfirmAcclimation advances an acclimatable task to acclimated.
func (s *Service) ConfirmAcclimation(ctx context.Context, id domain.TaskID, op domain.OperationID, digest string) ([]byte, domain.ErrorCode, error) {
	return RunIdempotent(ctx, s.tasks, id, op, digest, func() (domain.Envelope, error) {
		t, err := s.tasks.Get(ctx, id)
		if err != nil {
			return domain.Envelope{}, err
		}
		if t.State.IsTerminal() {
			return domain.Envelope{}, domain.NewError(domain.CodeTerminalState, "task is terminal")
		}
		if t.State != domain.StateAcclimatable {
			return domain.Envelope{}, domain.NewError(domain.CodeInvalidInput, "not acclimatable")
		}
		if err := s.tasks.SetState(ctx, id, domain.StateAcclimatable, domain.StateAcclimated); err != nil {
			return domain.Envelope{}, err
		}
		return domain.OKEnvelope(map[string]any{"state": domain.StateAcclimated.String()}), nil
	})
}

// View is the read projection of a task plus its closure progress.
type View struct {
	Task          Task           `json:"task"`
	Snapshot      Snapshot       `json:"snapshot"`
	Confirmations int            `json:"confirmations"`
	Reviews       int            `json:"reviews"`
	Final         *FinalDecision `json:"final,omitempty"`
}

// View returns the task, its snapshot, and closure progress for the GET API.
func (s *Service) View(ctx context.Context, id domain.TaskID) (View, error) {
	t, err := s.tasks.Get(ctx, id)
	if err != nil {
		return View{}, err
	}
	snap, err := s.tasks.Snapshot(ctx, id)
	if err != nil {
		return View{}, err
	}
	confs, _ := s.tasks.SubcultureConfirmations(ctx, id)
	reviews, _ := s.tasks.IndependentReviews(ctx, id)
	v := View{Task: t, Snapshot: snap, Confirmations: len(confs), Reviews: len(reviews)}
	if fd, err := s.tasks.FinalDecision(ctx, id); err == nil {
		v.Final = &fd
	}
	return v, nil
}

// ConfirmRequest is a subculture confirmation payload.
type ConfirmRequest struct {
	PersonnelID string            `json:"personnel_id"`
	Generation  domain.Generation `json:"task_generation"`
}

// ReviewRequest is an independent review payload.
type ReviewRequest struct {
	PersonnelID    string            `json:"personnel_id"`
	Generation     domain.Generation `json:"task_generation"`
	EvidenceDigest string            `json:"evidence_digest"`
	Conclusion     string            `json:"conclusion"`
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
