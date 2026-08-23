package queue

import "context"

// PolicyAuditSink receives redacted policy-change facts after a successful
// database update. ADR-012 keeps the MVP audit sink operational/structured;
// durable queryable audit storage is intentionally deferred.
type PolicyAuditSink interface {
	PolicyChanged(ctx context.Context, actorID, sessionID string, changes map[string][2]string)
}

type noopPolicyAuditSink struct{}

func (noopPolicyAuditSink) PolicyChanged(context.Context, string, string, map[string][2]string) {}

// PolicyService owns manager-authorized, prospective queue policy changes.
type PolicyService struct {
	repo  *Repository
	audit PolicyAuditSink
}

// NewPolicyService constructs a policy service over the queue repository.
func NewPolicyService(repo *Repository) *PolicyService {
	return NewPolicyServiceWithAudit(repo, nil)
}

// NewPolicyServiceWithAudit constructs a policy service with a redacted audit sink.
func NewPolicyServiceWithAudit(repo *Repository, audit PolicyAuditSink) *PolicyService {
	if audit == nil {
		audit = noopPolicyAuditSink{}
	}
	return &PolicyService{repo: repo, audit: audit}
}

// Update validates and applies an effective policy patch. Workflow values are
// prospective; visibility values affect new projections immediately.
func (s *PolicyService) Update(ctx context.Context, sessionID, actorID string, expectedVersion int64, next QueuePolicy) (QueuePolicy, error) {
	if err := ValidateExpectedVersion(expectedVersion); err != nil {
		return QueuePolicy{}, err
	}
	if err := next.Population.Validate(); err != nil {
		return QueuePolicy{}, err
	}
	if err := next.Finalization.Validate(); err != nil {
		return QueuePolicy{}, err
	}
	if err := next.OptOut.Validate(); err != nil {
		return QueuePolicy{}, err
	}
	if err := next.GradeVisibility.Validate(); err != nil {
		return QueuePolicy{}, err
	}
	if err := next.GradeCorrection.Validate(); err != nil {
		return QueuePolicy{}, err
	}
	before, updated, err := s.repo.UpdateSessionPolicyForManager(ctx, sessionID, actorID, expectedVersion, next)
	if err != nil {
		return QueuePolicy{}, err
	}
	if updated.Version != before.Version {
		s.audit.PolicyChanged(ctx, actorID, sessionID, policyChanges(before, updated))
	}
	return updated, nil
}

func policyChanges(before, after QueuePolicy) map[string][2]string {
	changes := make(map[string][2]string)
	if before.Population != after.Population {
		changes["population"] = [2]string{string(before.Population), string(after.Population)}
	}
	if before.Finalization != after.Finalization {
		changes["finalization"] = [2]string{string(before.Finalization), string(after.Finalization)}
	}
	if before.OptOut != after.OptOut {
		changes["opt_out"] = [2]string{string(before.OptOut), string(after.OptOut)}
	}
	if before.GradeVisibility != after.GradeVisibility {
		changes["grade_visibility"] = [2]string{string(before.GradeVisibility), string(after.GradeVisibility)}
	}
	if before.GradeCorrection != after.GradeCorrection {
		changes["grade_correction"] = [2]string{string(before.GradeCorrection), string(after.GradeCorrection)}
	}
	return changes
}
