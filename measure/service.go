package measure

import (
	"context"

	"strawberry-vitro-acclimation-gate/domain"
	"strawberry-vitro-acclimation-gate/task"
)

// Store is the persistence surface the measure service needs.
type Store interface {
	Get(ctx context.Context, id domain.TaskID) (task.Task, error)
	Snapshot(ctx context.Context, id domain.TaskID) (task.Snapshot, error)
	SetState(ctx context.Context, id domain.TaskID, from, to domain.TaskState) error
	SaveMorphologyBatch(ctx context.Context, cells []MorphologyCell) error
	MorphologyCells(ctx context.Context, id domain.TaskID) ([]MorphologyCell, error)
	SaveMeasurementBatch(ctx context.Context, cells []MeasurementCell) error
	MeasurementCells(ctx context.Context, id domain.TaskID) ([]MeasurementCell, error)
	FindOperation(ctx context.Context, id domain.TaskID, op domain.OperationID) (task.OperationRecord, bool, error)
	SaveOperation(ctx context.Context, rec task.OperationRecord) error
}

// Service implements the morphology, root, and physicochemical collection
// ledger: the coverage matrix, count conservation, and bounded fixed-point
// integer evidence with pre-write arithmetic validation.
type Service struct {
	store Store
	clock *domain.LogicalClock
}

// NewService wires the measure service to its store and logical clock.
func NewService(store Store, clock *domain.LogicalClock) *Service {
	return &Service{store: store, clock: clock}
}

// MorphologyInput is one per-position morphology cell in a batch.
type MorphologyInput struct {
	Position        string `json:"position"`
	Normal          int    `json:"normal"`
	Vitrified       int    `json:"vitrified"`
	Browned         int    `json:"browned"`
	Contaminated    int    `json:"contaminated"`
	ScoringPlatePos string `json:"scoring_plate_pos"`
	Supplement      bool   `json:"supplement"`
}

// MorphologyRequest is a batch morphology submission.
type MorphologyRequest struct {
	Generation domain.Generation `json:"task_generation"`
	Cells      []MorphologyInput `json:"cells"`
}

// SubmitMorphology validates count conservation and full coverage, then writes
// the whole batch atomically. Any invalid cell rolls back the entire batch and
// advances no state.
func (s *Service) SubmitMorphology(ctx context.Context, id domain.TaskID, op domain.OperationID, digest string, req MorphologyRequest) ([]byte, domain.ErrorCode, error) {
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
		if t.State != domain.StateCollectingMorphology {
			return domain.Envelope{}, domain.NewError(domain.CodeInvalidInput, "not in morphology collection state")
		}
		snap, err := s.store.Snapshot(ctx, id)
		if err != nil {
			return domain.Envelope{}, err
		}
		if len(req.Cells) != len(snap.BottlePositions) {
			return domain.Envelope{}, domain.NewError(domain.CodeEvidenceIncomplete, "morphology does not cover every bottle position")
		}
		seen := map[string]bool{}
		cells := make([]MorphologyCell, 0, len(req.Cells))
		for _, in := range req.Cells {
			if seen[in.Position] {
				return domain.Envelope{}, domain.NewError(domain.CodeDuplicateValue, "duplicate position")
			}
			seen[in.Position] = true
			locked, ok := snap.LockedSeedlings[in.Position]
			if !ok {
				return domain.Envelope{}, domain.NewError(domain.CodeInvalidInput, "unknown bottle position")
			}
			cell := MorphologyCell{
				TaskID: id, Position: in.Position, Normal: in.Normal, Vitrified: in.Vitrified,
				Browned: in.Browned, Contaminated: in.Contaminated,
				ScoringPlatePos: in.ScoringPlatePos, Supplement: in.Supplement, Version: 1,
			}
			if !ValidateCounts(cell, locked) {
				return domain.Envelope{}, domain.NewError(domain.CodeCountNotConserved, "counts not conserved").
					WithReason(domain.Reason{BottlePosition: in.Position, Field: "counts"})
			}
			cells = append(cells, cell)
		}
		if err := s.store.SaveMorphologyBatch(ctx, cells); err != nil {
			return domain.Envelope{}, err
		}
		if err := s.store.SetState(ctx, id, domain.StateCollectingMorphology, domain.StateVerifyingRoots); err != nil {
			return domain.Envelope{}, err
		}
		return domain.OKEnvelope(map[string]any{"cells": len(cells)}), nil
	})
}

// MeasurementInput is one raw fixed-point reading at a position and point.
type MeasurementInput struct {
	Position string `json:"position"`
	Point    string `json:"point"`
	Metric   string `json:"metric"`
	Value    string `json:"value"`
}

// MeasurementRequest is a batch measurement submission.
type MeasurementRequest struct {
	Generation domain.Generation  `json:"task_generation"`
	Scale      int                `json:"scale"`
	Readings   []MeasurementInput `json:"readings"`
}

// SubmitMeasurements parses raw fixed-point readings with pre-write length,
// sign, decimal-place, overflow, and divide-by-zero checks, then writes the
// whole batch atomically. Any invalid reading leaves no evidence behind. When
// advance is true and the task is in the "from" state, it advances to "to".
func (s *Service) SubmitMeasurements(ctx context.Context, id domain.TaskID, op domain.OperationID, digest string, from, to domain.TaskState, advance bool, req MeasurementRequest) ([]byte, domain.ErrorCode, error) {
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
		if t.State != from {
			return domain.Envelope{}, domain.NewError(domain.CodeInvalidInput, "not in expected measurement state")
		}
		// A measurement batch that advances the task must close the locked
		// coverage matrix before any state moves: the root step requires every
		// locked bottle position and root point to carry both root_length and
		// ttc_viability, with no duplicate, unknown, or extra cells. A partial
		// batch is rejected whole so the downstream pathogen recheck never
		// treats a missing reading as already closed.
		var snap task.Snapshot
		if advance {
			snap, err = s.store.Snapshot(ctx, id)
			if err != nil {
				return domain.Envelope{}, err
			}
		}
		cells := make([]MeasurementCell, 0, len(req.Readings))
		for _, in := range req.Readings {
			fp, err := domain.ParseFixedPoint(in.Value, 24, req.Scale, in.Metric == string(MetricTemperature))
			if err != nil {
				return domain.Envelope{}, domain.NewError(domain.ErrorCodeOf(err), "invalid fixed point").
					WithReason(domain.Reason{BottlePosition: in.Position, Field: in.Metric})
			}
			cells = append(cells, MeasurementCell{
				TaskID: id, Position: in.Position, Point: in.Point, Metric: MetricType(in.Metric),
				Value: fp, Version: 1,
			})
		}
		if advance {
			if missing := ValidateRootCoverage(cells, snap.BottlePositions, snap.RootPoints); len(missing) > 0 {
				return domain.Envelope{}, domain.NewError(domain.CodeEvidenceIncomplete, "root measurements do not cover every locked position and point").
					WithReasons(missing...)
			}
		}
		if err := s.store.SaveMeasurementBatch(ctx, cells); err != nil {
			return domain.Envelope{}, err
		}
		if advance {
			if err := s.store.SetState(ctx, id, from, to); err != nil {
				return domain.Envelope{}, err
			}
		}
		return domain.OKEnvelope(map[string]any{"readings": len(cells)}), nil
	})
}
