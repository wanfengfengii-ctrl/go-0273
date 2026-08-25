package catalog

import (
	"context"
	"strings"

	"strawberry-vitro-acclimation-gate/domain"
)

// Store is the persistence surface the catalog service needs.
type Store interface {
	UpsertMother(ctx context.Context, m MotherPlant) error
	Mother(ctx context.Context, id string) (MotherPlant, error)
	UpsertMedium(ctx context.Context, m MediumRevision) error
	Medium(ctx context.Context, formulaID string) (MediumRevision, error)
	UpsertRuleProfile(ctx context.Context, r RuleProfile) error
	RuleProfile(ctx context.Context, id string) (RuleProfile, error)
	UpsertPersonnel(ctx context.Context, p Personnel) error
	Personnel(ctx context.Context, id string) (Personnel, error)
}

// Service implements the mother-plant and tissue-culture rule catalog flows:
// versioned entries for mother plants, medium revisions, rule profiles, and
// personnel, with input validation before any write. Locking a task snapshots
// this catalog; later edits never change an already-locked task.
type Service struct {
	store Store
}

// NewService wires the catalog service to its store.
func NewService(store Store) *Service { return &Service{store: store} }

// SaveMother validates and upserts a mother plant.
func (s *Service) SaveMother(ctx context.Context, m MotherPlant) error {
	if strings.TrimSpace(m.ID) == "" || strings.TrimSpace(m.LineageID) == "" {
		return domain.NewError(domain.CodeInvalidInput, "mother id and lineage id are required")
	}
	return s.store.UpsertMother(ctx, m)
}

// SaveMedium validates and upserts a medium revision.
func (s *Service) SaveMedium(ctx context.Context, m MediumRevision) error {
	if strings.TrimSpace(m.FormulaID) == "" || strings.TrimSpace(m.Summary) == "" {
		return domain.NewError(domain.CodeInvalidInput, "formula id and summary are required")
	}
	return s.store.UpsertMedium(ctx, m)
}

// SaveRuleProfile validates and upserts a rule profile.
func (s *Service) SaveRuleProfile(ctx context.Context, r RuleProfile) error {
	if strings.TrimSpace(r.ID) == "" {
		return domain.NewError(domain.CodeInvalidInput, "rule profile id is required")
	}
	if r.VitrificationMaxPct < 0 || r.BrowningMaxPct < 0 || r.FixedScale < 0 {
		return domain.NewError(domain.CodeInvalidInput, "thresholds must be non-negative")
	}
	return s.store.UpsertRuleProfile(ctx, r)
}

// SavePersonnel validates and upserts a personnel record.
func (s *Service) SavePersonnel(ctx context.Context, p Personnel) error {
	if strings.TrimSpace(p.ID) == "" {
		return domain.NewError(domain.CodeInvalidInput, "personnel id is required")
	}
	return s.store.UpsertPersonnel(ctx, p)
}

// Mother returns a cataloged mother plant by id.
func (s *Service) Mother(ctx context.Context, id string) (MotherPlant, error) {
	return s.store.Mother(ctx, id)
}

// Medium returns the current medium revision for a formula.
func (s *Service) Medium(ctx context.Context, formulaID string) (MediumRevision, error) {
	return s.store.Medium(ctx, formulaID)
}

// RuleProfile returns a rule profile by id.
func (s *Service) RuleProfile(ctx context.Context, id string) (RuleProfile, error) {
	return s.store.RuleProfile(ctx, id)
}

// Personnel returns a personnel record by id.
func (s *Service) Personnel(ctx context.Context, id string) (Personnel, error) {
	return s.store.Personnel(ctx, id)
}
