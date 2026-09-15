package chat

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/metrics"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
)

// authorizeRetainedCircleMember authorizes a read operation against the
// caller's current membership period. Archived circles remain readable for
// retained members; removal and unknown circles produce the same error so the
// API cannot be used to enumerate circles.
func authorizeRetainedCircleMember(ctx context.Context, membership MembershipReader, viewerID, circleID uuid.UUID, deny denialRecorder) error {
	_, err := membership.FindCircleByID(ctx, circleID.String())
	if errorsIsCircleNotFound(err) {
		deny(metrics.ChatDenialIneligible)
		return ErrCircleNotVisible
	}
	if err != nil {
		return fmt.Errorf("load chat circle for read: %w", err)
	}
	member, err := membership.IsMember(ctx, circleID.String(), viewerID.String())
	if err != nil {
		return fmt.Errorf("authorize chat read membership: %w", err)
	}
	if !member {
		deny(metrics.ChatDenialIneligible)
		return ErrCircleNotVisible
	}
	return nil
}

func errorsIsCircleNotFound(err error) bool {
	return errors.Is(err, rbac.ErrCircleNotFound)
}
