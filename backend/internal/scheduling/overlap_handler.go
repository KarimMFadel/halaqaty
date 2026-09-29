package scheduling

import (
	"context"
	"net/http"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

type OverlapPreviewCommands interface {
	Preview(context.Context, string, string, RevisionRecord) (WarningResult, error)
}

// OverlapHandler serves the non-mutating circle planning preview.
type OverlapHandler struct {
	service OverlapPreviewCommands
}

// NewOverlapHandler constructs the preview endpoint.
func NewOverlapHandler(service OverlapPreviewCommands) *OverlapHandler {
	return &OverlapHandler{service: service}
}

// Preview serves POST /api/v1/circles/{circleId}/planning-preview.
func (h *OverlapHandler) Preview(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.service == nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return
	}
	principal, ok := currentScheduleActor(w, r)
	if !ok {
		return
	}
	circleID, ok := pathUUID(w, r, "circleId", httpconst.FieldCircleID)
	if !ok {
		return
	}
	var req planInputRequest
	if !phttp.DecodeJSONBody(w, r, &req) {
		return
	}
	plan, ok := decodePlan(w, req)
	if !ok {
		return
	}
	warnings, err := h.service.Preview(r.Context(), principal, circleID, plan)
	if err != nil {
		writeScheduleError(w, err)
		return
	}
	phttp.WriteJSON(w, http.StatusOK, warnings)
}

func currentScheduleActor(w http.ResponseWriter, r *http.Request) (string, bool) {
	principal, ok := auth.CurrentPrincipal(r.Context())
	if !ok || principal.UserID == "" {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return "", false
	}
	return principal.UserID, true
}
