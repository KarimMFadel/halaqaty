package chat

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

type discoveryHandlerMinimalService struct{}

func (discoveryHandlerMinimalService) History(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID, int) ([]Message, error) {
	return nil, nil
}

func (discoveryHandlerMinimalService) Search(context.Context, uuid.UUID, uuid.UUID, string, *uuid.UUID, int) ([]Message, error) {
	return nil, nil
}

func (discoveryHandlerMinimalService) SendText(context.Context, uuid.UUID, uuid.UUID, string, string) (Message, error) {
	return Message{}, nil
}

func TestDiscoveryHandler_FailsClosedWithoutPersistentCapabilities(t *testing.T) {
	h := NewDiscoveryHandler(discoveryHandlerMinimalService{})
	ctx := context.Background()
	actorID, circleID, messageID := uuid.New(), uuid.New(), uuid.New()
	if _, err := h.listPinned(ctx, actorID, circleID); err == nil {
		t.Fatal("list pinned without persistent capability must fail closed")
	}
	if _, err := h.pin(ctx, actorID, circleID, messageID); err == nil {
		t.Fatal("pin without persistent capability must fail closed")
	}
	if err := h.unpin(ctx, actorID, circleID, messageID); err == nil {
		t.Fatal("unpin without persistent capability must fail closed")
	}
}
