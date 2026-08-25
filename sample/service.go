package sample

import (
	"context"

	"strawberry-vitro-acclimation-gate/domain"
	"strawberry-vitro-acclimation-gate/task"
)

// Store is the persistence surface the sample service needs. The SQLite-backed
// store satisfies it; tests may substitute a fake.
type Store interface {
	Get(ctx context.Context, id domain.TaskID) (task.Task, error)
	Snapshot(ctx context.Context, id domain.TaskID) (task.Snapshot, error)
	SamplesByTask(ctx context.Context, id domain.TaskID) ([]BottleSample, error)
	SealSample(ctx context.Context, b BottleSample) error
	SealBatch(ctx context.Context, id domain.TaskID, seals []SealInput, from, to domain.TaskState) error
	BlindCodesByTask(ctx context.Context, id domain.TaskID) ([]BlindCode, error)
	BindBlindCode(ctx context.Context, c BlindCode) error
	BlindCode(ctx context.Context, digest string) (BlindCode, error)
	Reveal(ctx context.Context, digest string, ev RevealEvent) error
	AcquireLease(ctx context.Context, l ResourceLease) error
	ReplaceLease(ctx context.Context, l ResourceLease) error
	ReleaseLease(ctx context.Context, resourceType ResourceType, key string) error
	ActiveLease(ctx context.Context, resourceType ResourceType, key string) (ResourceLease, error)
	ActiveLeasesByTask(ctx context.Context, id domain.TaskID) ([]ResourceLease, error)
	SetState(ctx context.Context, id domain.TaskID, from, to domain.TaskState) error
	FindOperation(ctx context.Context, id domain.TaskID, op domain.OperationID) (task.OperationRecord, bool, error)
	SaveOperation(ctx context.Context, rec task.OperationRecord) error
}

// Service implements the bottle-sample and resource-occupancy ledger flows:
// triple-sample sealing, blind-code binding and reveal, and transactional
// acquisition, switching, and renewal of the five resource-lease kinds.
type Service struct {
	store Store
	clock *domain.LogicalClock
}

// NewService wires the sample service to its store and logical clock.
func NewService(store Store, clock *domain.LogicalClock) *Service {
	return &Service{store: store, clock: clock}
}

// SealInput binds one seal to a position and a blind-code digest.
type SealInput struct {
	Seal            string `json:"seal"`
	Position        string `json:"position"`
	BlindCodeDigest string `json:"blind_code_digest"`
}

// SealRequest is the triple-sample sealing batch.
type SealRequest struct {
	Generation domain.Generation `json:"task_generation"`
	Seals      []SealInput       `json:"seals"`
}

// SealSamples binds blind codes and seals all triple samples in one atomic
// step, then advances the task into resource occupation. A missing or
// duplicate position or blind code rejects the entire batch.
func (s *Service) SealSamples(ctx context.Context, id domain.TaskID, op domain.OperationID, digest string, req SealRequest) ([]byte, domain.ErrorCode, error) {
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
		if t.State != domain.StateSealingSamples {
			return domain.Envelope{}, domain.NewError(domain.CodeInvalidInput, "not in sample sealing state")
		}
		snap, err := s.store.Snapshot(ctx, id)
		if err != nil {
			return domain.Envelope{}, err
		}
		expected := len(snap.BottleSeals)
		if len(req.Seals) != expected {
			return domain.Envelope{}, domain.NewError(domain.CodeInvalidInput, "seal batch does not match snapshot")
		}
		seenSeal := map[string]bool{}
		seenPos := map[string]bool{}
		seenCode := map[string]bool{}
		for _, in := range req.Seals {
			if seenSeal[in.Seal] || seenPos[in.Position] || seenCode[in.BlindCodeDigest] {
				return domain.Envelope{}, domain.NewError(domain.CodeDuplicateValue, "duplicate seal, position, or blind code")
			}
			seenSeal[in.Seal] = true
			seenPos[in.Position] = true
			seenCode[in.BlindCodeDigest] = true
		}
		// Bind every blind code, seal every sample, and advance the state in a
		// single transaction so a failure on any one entry rolls back the whole
		// batch and leaves no partial binding or sealed sample behind.
		if err := s.store.SealBatch(ctx, id, req.Seals, domain.StateSealingSamples, domain.StateOccupyingResources); err != nil {
			return domain.Envelope{}, err
		}
		return domain.OKEnvelope(map[string]any{"sealed": expected}), nil
	})
}

// AcquireRequest names the resources to occupy in one transaction.
type AcquireRequest struct {
	Generation domain.Generation `json:"task_generation"`
}

// AcquireResources occupies the rack shelf, light window, acclimation window,
// RT-PCR wells, and endophyte wells atomically. Any overlap with another open
// task fails the whole acquisition via the unique active-lease index.
func (s *Service) AcquireResources(ctx context.Context, id domain.TaskID, op domain.OperationID, digest string, req AcquireRequest) ([]byte, domain.ErrorCode, error) {
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
		if t.State != domain.StateOccupyingResources {
			return domain.Envelope{}, domain.NewError(domain.CodeInvalidInput, "not in resource occupation state")
		}
		snap, err := s.store.Snapshot(ctx, id)
		if err != nil {
			return domain.Envelope{}, err
		}
		now := s.clock.Now()
		leases := []ResourceLease{
			{ResourceType: ResourceRackShelf, ResourceKey: snap.RackShelf},
			{ResourceType: ResourceLightWindow, ResourceKey: snap.LightWindow},
			{ResourceType: ResourceAcclimationWindow, ResourceKey: snap.AcclimationWindow},
		}
		for _, w := range snap.RTQPCRWells {
			leases = append(leases, ResourceLease{ResourceType: ResourceRTQPCRWell, ResourceKey: w})
		}
		for _, w := range snap.EndophyteWells {
			leases = append(leases, ResourceLease{ResourceType: ResourceEndophyteWell, ResourceKey: w})
		}
		for i := range leases {
			leases[i].TaskID = id
			leases[i].Generation = t.Generation
			leases[i].Version = 1
			leases[i].StartAt = now
			leases[i].EndAt = now + 1000
			leases[i].Active = true
			if err := s.store.AcquireLease(ctx, leases[i]); err != nil {
				return domain.Envelope{}, err
			}
		}
		if err := s.store.SetState(ctx, id, domain.StateOccupyingResources, domain.StateCollectingMorphology); err != nil {
			return domain.Envelope{}, err
		}
		return domain.OKEnvelope(map[string]any{"leases": len(leases)}), nil
	})
}

// SwitchRequest replaces one lease with a new resource key.
type SwitchRequest struct {
	Generation   domain.Generation `json:"task_generation"`
	ResourceType ResourceType      `json:"resource_type"`
	OldKey       string            `json:"old_key"`
	NewKey       string            `json:"new_key"`
}

// SwitchResource compares the task generation and lease version, then replaces
// the lease atomically. It fails on a stale generation or version mismatch.
func (s *Service) SwitchResource(ctx context.Context, id domain.TaskID, op domain.OperationID, digest string, req SwitchRequest) ([]byte, domain.ErrorCode, error) {
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
		cur, err := s.store.ActiveLease(ctx, req.ResourceType, req.OldKey)
		if err != nil || cur.TaskID != id {
			return domain.Envelope{}, domain.NewError(domain.CodeInvalidInput, "lease not held by task")
		}
		newLease := cur
		newLease.ResourceKey = req.NewKey
		newLease.Version = cur.Version + 1
		newLease.StartAt = s.clock.Now()
		if err := s.store.ReplaceLease(ctx, newLease); err != nil {
			return domain.Envelope{}, err
		}
		return domain.OKEnvelope(map[string]any{"version": newLease.Version}), nil
	})
}

// RenewRequest extends a lease.
type RenewRequest struct {
	Generation   domain.Generation  `json:"task_generation"`
	ResourceType ResourceType       `json:"resource_type"`
	ResourceKey  string             `json:"resource_key"`
	Extension    domain.LogicalTime `json:"extension"`
}

// RenewResource extends the logical end time of a held lease.
func (s *Service) RenewResource(ctx context.Context, id domain.TaskID, op domain.OperationID, digest string, req RenewRequest) ([]byte, domain.ErrorCode, error) {
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
		cur, err := s.store.ActiveLease(ctx, req.ResourceType, req.ResourceKey)
		if err != nil || cur.TaskID != id {
			return domain.Envelope{}, domain.NewError(domain.CodeInvalidInput, "lease not held by task")
		}
		cur.EndAt += req.Extension
		cur.Version++
		if err := s.store.ReplaceLease(ctx, cur); err != nil {
			return domain.Envelope{}, err
		}
		return domain.OKEnvelope(map[string]any{"version": cur.Version}), nil
	})
}

// RevealRequest is a controlled blind-code reveal.
type RevealRequest struct {
	Generation domain.Generation `json:"task_generation"`
	Digest     string            `json:"digest"`
}

// RevealBlindCode reveals a blind code only at or after the authorized
// pathogen-recheck point. Early reveal is rejected whole with PRE_BLIND_REVEAL.
func (s *Service) RevealBlindCode(ctx context.Context, id domain.TaskID, op domain.OperationID, digest string, req RevealRequest) ([]byte, domain.ErrorCode, error) {
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
		if t.State < domain.StateRecheckingPathogen {
			return domain.Envelope{}, domain.NewError(domain.CodePreBlindReveal, "blind code reveal not yet authorized")
		}
		code, err := s.store.BlindCode(ctx, req.Digest)
		if err != nil {
			return domain.Envelope{}, domain.NewError(domain.CodeInvalidInput, "unknown blind code")
		}
		if code.TaskID != id {
			return domain.Envelope{}, domain.NewError(domain.CodeInvalidInput, "blind code bound to another task")
		}
		if err := s.store.Reveal(ctx, req.Digest, RevealEvent{At: s.clock.Now(), By: "operator"}); err != nil {
			return domain.Envelope{}, err
		}
		return domain.OKEnvelope(map[string]any{"digest": req.Digest, "bound_seal": code.BoundSeal}), nil
	})
}
