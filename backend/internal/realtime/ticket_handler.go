package realtime

import (
	"net/http"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

// Handler exposes the generic realtime ticket endpoint.
type Handler struct{ tickets *TicketService }

// NewHandler constructs a realtime HTTP handler.
func NewHandler(tickets *TicketService) *Handler { return &Handler{tickets: tickets} }

// CreateTicket issues a no-store ticket for the authenticated caller.
func (h *Handler) CreateTicket(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.CurrentPrincipal(r.Context())
	if !ok || principal.UserID == "" {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return
	}
	if h == nil || h.tickets == nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return
	}
	ticket, err := h.tickets.Issue(r.Context(), principal.UserID)
	if err != nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	phttp.WriteJSON(w, http.StatusOK, map[string]any{"token": ticket.Token, "expires_at": ticket.ExpiresAt.Format(time.RFC3339)})
}
