package task

import (
	"context"

	"strawberry-vitro-acclimation-gate/domain"
)

// OpStore is the minimal persistence surface required by the idempotency
// protocol: look up and persist deterministic operation results.
type OpStore interface {
	FindOperation(ctx context.Context, id domain.TaskID, op domain.OperationID) (OperationRecord, bool, error)
	SaveOperation(ctx context.Context, rec OperationRecord) error
}

// RunIdempotent executes a business write under the operation-id protocol. The
// first success or business rejection is persisted deterministically; replaying
// the same (operation, digest) returns the original result, while a different
// digest under the same operation returns OPERATION_CONTENT_CONFLICT. A nil
// returned error indicates a domain-level result (carried in the envelope);
// only unexpected internal failures surface as a non-nil error.
func RunIdempotent(ctx context.Context, s OpStore, id domain.TaskID, op domain.OperationID, digest string, run func() (domain.Envelope, error)) ([]byte, domain.ErrorCode, error) {
	if rec, ok, err := s.FindOperation(ctx, id, op); err != nil {
		return nil, domain.CodeInvalidInput, err
	} else if ok {
		if rec.RequestDigest == digest {
			return rec.Response, rec.ResultCode, nil
		}
		body, _ := domain.ErrorEnvelope(domain.CodeOperationContentConflict, "operation content conflict", nil).Bytes()
		return body, domain.CodeOperationContentConflict, nil
	}
	env, err := run()
	if err != nil {
		return nil, domain.CodeInvalidInput, err
	}
	body, err := env.Bytes()
	if err != nil {
		return nil, domain.CodeInvalidInput, err
	}
	_ = s.SaveOperation(ctx, OperationRecord{
		TaskID:        id,
		OperationID:   op,
		RequestDigest: digest,
		ResultCode:    env.Code,
		Response:      body,
	})
	return body, env.Code, nil
}
