package queue

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

const idempotencyKeyHeader = "Idempotency-Key"

// Handler exposes the F-003 queue-management REST operations.
type Handler struct {
	repo     *Repository
	rounds   *RoundService
	turns    *TurnService
	policies *PolicyService
	optOuts  *OptOutService
}

// NewHandler constructs the queue REST handler over the queue services.
func NewHandler(repo *Repository, rounds *RoundService, turns *TurnService, policies *PolicyService, optOuts *OptOutService) *Handler {
	return &Handler{repo: repo, rounds: rounds, turns: turns, policies: policies, optOuts: optOuts}
}

type roundRequest struct {
	RoundType       RoundType `json:"round_type"`
	SurahID         int       `json:"surah_id"`
	FromAyah        int       `json:"from_ayah"`
	ToAyah          int       `json:"to_ayah"`
	GradingRequired bool      `json:"grading_required"`
	StudentOrder    []string  `json:"student_order"`
	ExpectedVersion int64     `json:"expected_version"`
}

type versionRequest struct {
	ExpectedVersion int64 `json:"expected_version"`
}

type reorderRequest struct {
	OrderedIDs      []string `json:"ordered_ids"`
	ExpectedVersion int64    `json:"expected_version"`
}

type moveRequest struct {
	NewPosition     int   `json:"new_position"`
	ExpectedVersion int64 `json:"expected_version"`
}

type entryStatusRequest struct {
	Status               EntryStatus  `json:"status"`
	ExpectedEntryVersion int64        `json:"expected_entry_version"`
	Grade                *Grade       `json:"grade"`
	Notes                optionalNote `json:"notes"`
}

// optionalNote distinguishes an absent notes field from an explicit JSON
// null: the contract makes null mean "clear the existing note" during an
// audited correction, while absence means "leave the note unchanged".
type optionalNote struct {
	set   bool
	value *string
}

// UnmarshalJSON records presence; absent fields never invoke this method.
func (n *optionalNote) UnmarshalJSON(data []byte) error {
	n.set = true
	if string(data) == "null" {
		n.value = nil
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return err
	}
	n.value = &text
	return nil
}

// correctionNotes maps the decoded note onto the Correct service contract:
// absent → nil (unchanged), explicit null → empty string (clear), text → text.
func (r entryStatusRequest) correctionNotes() *string {
	if !r.Notes.set {
		return nil
	}
	if r.Notes.value == nil {
		empty := ""
		return &empty
	}
	return r.Notes.value
}

type policyPatchRequest struct {
	Population      *PopulationPolicy   `json:"population"`
	Finalization    *FinalizationPolicy `json:"unfinished_finalization"`
	OptOut          *OptOutPolicy       `json:"opt_out"`
	GradeVisibility *GradeVisibility    `json:"grade_visibility"`
	GradeCorrection *GradeCorrection    `json:"grade_correction"`
	ExpectedVersion int64               `json:"expected_version"`
}

type optOutDecisionRequest struct {
	Decision             OptOutRequestStatus `json:"decision"`
	ExpectedEntryVersion int64               `json:"expected_entry_version"`
}

// GetQueue returns the current queue projection allowed for the caller.
func (h *Handler) GetQueue(w http.ResponseWriter, r *http.Request) {
	sessionID, actorID, manager, ok := h.authorize(w, r, false)
	if !ok {
		return
	}
	round, err := h.repo.CurrentRound(r.Context(), sessionID)
	if err != nil {
		writeQueueError(w, err)
		return
	}
	h.writeState(w, r, round.ID, Viewer{UserID: actorID, IsManager: manager}, http.StatusOK)
}

// CreateRound prepares a round and returns its current visibility-filtered state.
func (h *Handler) CreateRound(w http.ResponseWriter, r *http.Request) {
	sessionID, actorID, _, ok := h.authorize(w, r, true)
	if !ok {
		return
	}
	var req roundRequest
	if !phttp.DecodeJSONBody(w, r, &req) || !validateRoundRequest(w, req) {
		return
	}
	if receipt, replay := h.receipt(w, r, sessionID, actorID, "queue.create_round"); replay {
		if receipt != nil && receipt.ResourceID != nil {
			round, err := h.repo.Round(r.Context(), *receipt.ResourceID)
			if err != nil {
				writeQueueError(w, err)
				return
			}
			if !sameRoundRequest(round, req) {
				phttp.WriteError(w, httpconst.ErrorCodeQueueDuplicateCommand, httpconst.ErrorMessageQueueDuplicateCommand, http.StatusConflict)
				return
			}
			h.writeState(w, r, *receipt.ResourceID, Viewer{UserID: actorID, IsManager: true}, http.StatusOK)
		}
		return
	}
	round, err := h.rounds.Prepare(r.Context(), prepareInput(sessionID, actorID, req))
	if err != nil {
		writeQueueError(w, err)
		return
	}
	if err := h.recordReceipt(r, sessionID, actorID, round.ID, round.Version); err != nil {
		writeQueueError(w, err)
		return
	}
	h.writeState(w, r, round.ID, Viewer{UserID: actorID, IsManager: true}, http.StatusCreated)
}

// ResetQueue finalizes the displayed active round and prepares its successor.
func (h *Handler) ResetQueue(w http.ResponseWriter, r *http.Request) {
	sessionID, actorID, _, ok := h.authorize(w, r, true)
	if !ok {
		return
	}
	var req roundRequest
	if !phttp.DecodeJSONBody(w, r, &req) || !validateRoundRequest(w, req) || !validateVersion(w, req.ExpectedVersion) {
		return
	}
	if receipt, replay := h.receipt(w, r, sessionID, actorID, "queue.reset"); replay {
		h.replayState(w, r, receipt, actorID)
		return
	}
	round, err := h.rounds.Reset(r.Context(), prepareInput(sessionID, actorID, req))
	if err != nil {
		writeQueueError(w, err)
		return
	}
	if err := h.recordReceipt(r, sessionID, actorID, round.ID, round.Version); err != nil {
		writeQueueError(w, err)
		return
	}
	h.writeState(w, r, round.ID, Viewer{UserID: actorID, IsManager: true}, http.StatusCreated)
}

// Advance selects the next waiting entry without changing audio permission.
func (h *Handler) Advance(w http.ResponseWriter, r *http.Request) {
	sessionID, actorID, _, ok := h.authorize(w, r, true)
	if !ok {
		return
	}
	var req versionRequest
	if !phttp.DecodeJSONBody(w, r, &req) || !validateVersion(w, req.ExpectedVersion) {
		return
	}
	if receipt, replay := h.receipt(w, r, sessionID, actorID, "queue.advance"); replay {
		h.replayState(w, r, receipt, actorID)
		return
	}
	round, err := h.repo.CurrentRound(r.Context(), sessionID)
	if err == nil {
		round, err = h.turns.Advance(r.Context(), round.ID, req.ExpectedVersion)
	}
	if err != nil {
		writeQueueError(w, err)
		return
	}
	if err := h.recordReceipt(r, sessionID, actorID, round.ID, round.Version); err != nil {
		writeQueueError(w, err)
		return
	}
	h.writeState(w, r, round.ID, Viewer{UserID: actorID, IsManager: true}, http.StatusOK)
}

// Reorder replaces the pre-activation candidate order.
func (h *Handler) Reorder(w http.ResponseWriter, r *http.Request) {
	sessionID, actorID, _, ok := h.authorize(w, r, true)
	if !ok {
		return
	}
	var req reorderRequest
	if !phttp.DecodeJSONBody(w, r, &req) || !validateVersion(w, req.ExpectedVersion) {
		return
	}
	if receipt, replay := h.receipt(w, r, sessionID, actorID, "queue.reorder"); replay {
		h.replayState(w, r, receipt, actorID)
		return
	}
	round, err := h.repo.LowestPreparedRound(r.Context(), sessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		err = &QueueError{Code: QueueErrorCodeInvalidTransition, Message: "queue order is no longer editable"}
	}
	if err == nil {
		round, err = h.rounds.Reorder(r.Context(), round.ID, actorID, req.ExpectedVersion, req.OrderedIDs)
	}
	if err != nil {
		writeQueueError(w, err)
		return
	}
	if err := h.recordReceipt(r, sessionID, actorID, round.ID, round.Version); err != nil {
		writeQueueError(w, err)
		return
	}
	h.writeState(w, r, round.ID, Viewer{UserID: actorID, IsManager: true}, http.StatusOK)
}

// MoveEntry changes one waiting entry's durable position.
func (h *Handler) MoveEntry(w http.ResponseWriter, r *http.Request) {
	sessionID, actorID, _, ok := h.authorize(w, r, true)
	if !ok {
		return
	}
	entryID, ok := pathUUID(w, r, "entryId", httpconst.FieldUserID)
	if !ok {
		return
	}
	var req moveRequest
	if !phttp.DecodeJSONBody(w, r, &req) || !validateVersion(w, req.ExpectedVersion) || req.NewPosition < 1 {
		if req.NewPosition < 1 {
			writeQueueValidation(w, httpconst.ErrorCodeQueueInvalidRange, httpconst.ErrorMessageQueueInvalidRange, httpconst.FieldNewPosition)
		}
		return
	}
	roundID, err := h.repo.EntryQueueIDForSession(r.Context(), sessionID, entryID)
	if err != nil {
		writeQueueError(w, err)
		return
	}
	if receipt, replay := h.receipt(w, r, sessionID, actorID, "queue.move"); replay {
		h.replayState(w, r, receipt, actorID)
		return
	}
	_, err = h.rounds.Move(r.Context(), entryID, req.ExpectedVersion, req.NewPosition)
	if err != nil {
		writeQueueError(w, err)
		return
	}
	round, err := h.repo.Round(r.Context(), roundID)
	if err != nil {
		writeQueueError(w, err)
		return
	}
	if err := h.recordReceipt(r, sessionID, actorID, roundID, round.Version); err != nil {
		writeQueueError(w, err)
		return
	}
	h.writeState(w, r, roundID, Viewer{UserID: actorID, IsManager: true}, http.StatusOK)
}

// UpdateEntryStatus starts or skips an entry without changing media permission.
func (h *Handler) UpdateEntryStatus(w http.ResponseWriter, r *http.Request) {
	sessionID, actorID, _, ok := h.authorize(w, r, true)
	if !ok {
		return
	}
	entryID, ok := pathUUID(w, r, "entryId", httpconst.FieldUserID)
	if !ok {
		return
	}
	var req entryStatusRequest
	if !phttp.DecodeJSONBody(w, r, &req) || !validateVersion(w, req.ExpectedEntryVersion) {
		return
	}
	roundID, err := h.repo.EntryQueueIDForSession(r.Context(), sessionID, entryID)
	if err != nil {
		writeQueueError(w, err)
		return
	}
	switch req.Status {
	case EntryStatusReciting, EntryStatusSkipped:
		if req.Grade != nil || req.Notes.set {
			writeQueueValidation(w, httpconst.ErrorCodeQueueInvalidGrade, httpconst.ErrorMessageQueueInvalidGrade, httpconst.FieldGrade)
			return
		}
	case EntryStatusCompleted:
		if req.Notes.value != nil && len(*req.Notes.value) > 500 {
			writeQueueValidation(w, httpconst.ErrorCodeQueueInvalidRange, httpconst.ErrorMessageQueueInvalidRange, httpconst.FieldNote)
			return
		}
	default:
		writeQueueValidation(w, httpconst.ErrorCodeQueueInvalidEnum, httpconst.ErrorMessageQueueInvalidEnum, httpconst.FieldStatus)
		return
	}
	if receipt, replay := h.receipt(w, r, sessionID, actorID, "queue.update_entry_status"); replay {
		h.replayState(w, r, receipt, actorID)
		return
	}
	switch req.Status {
	case EntryStatusReciting:
		_, err = h.turns.Start(r.Context(), entryID, req.ExpectedEntryVersion)
	case EntryStatusSkipped:
		_, err = h.turns.Skip(r.Context(), entryID, req.ExpectedEntryVersion, actorID)
	case EntryStatusCompleted:
		_, err = h.turns.Complete(r.Context(), entryID, req.ExpectedEntryVersion, actorID, req.Grade, req.Notes.value)
	}
	if err != nil {
		writeQueueError(w, err)
		return
	}
	round, err := h.repo.Round(r.Context(), roundID)
	if err != nil {
		writeQueueError(w, err)
		return
	}
	if err := h.recordReceipt(r, sessionID, actorID, roundID, round.Version); err != nil {
		writeQueueError(w, err)
		return
	}
	h.writeState(w, r, roundID, Viewer{UserID: actorID, IsManager: true}, http.StatusOK)
}

// GradeEntry corrects the grade and/or note of a completed entry.
func (h *Handler) GradeEntry(w http.ResponseWriter, r *http.Request) {
	sessionID, actorID, _, ok := h.authorize(w, r, true)
	if !ok {
		return
	}
	entryID, ok := pathUUID(w, r, "entryId", httpconst.FieldUserID)
	if !ok {
		return
	}
	var req entryStatusRequest
	if !phttp.DecodeJSONBody(w, r, &req) || !validateVersion(w, req.ExpectedEntryVersion) {
		return
	}
	if req.Status != "" {
		writeQueueValidation(w, httpconst.ErrorCodeQueueInvalidEnum, httpconst.ErrorMessageQueueInvalidEnum, httpconst.FieldStatus)
		return
	}
	if req.Grade == nil && !req.Notes.set {
		// GradeRequest carries minProperties 2: expected_entry_version plus
		// at least one of grade or notes.
		writeQueueValidation(w, httpconst.ErrorCodeQueueInvalidGrade, httpconst.ErrorMessageQueueInvalidGrade, httpconst.FieldGrade)
		return
	}
	if req.Notes.value != nil && len(*req.Notes.value) > 500 {
		writeQueueValidation(w, httpconst.ErrorCodeQueueInvalidRange, httpconst.ErrorMessageQueueInvalidRange, httpconst.FieldNote)
		return
	}
	correctionNotes := req.correctionNotes()
	if _, err := h.repo.EntryQueueIDForSession(r.Context(), sessionID, entryID); err != nil {
		writeQueueError(w, err)
		return
	}
	if receipt, replay := h.receipt(w, r, sessionID, actorID, "queue.grade_entry"); replay {
		h.replayState(w, r, receipt, actorID)
		return
	}
	entry, err := h.turns.Correct(r.Context(), entryID, req.ExpectedEntryVersion, actorID, req.Grade, correctionNotes)
	if err != nil {
		writeQueueError(w, err)
		return
	}
	if err := h.recordReceipt(r, sessionID, actorID, entryID, entry.Version); err != nil {
		writeQueueError(w, err)
		return
	}
	h.writeQueueEntry(w, r, entry, http.StatusOK)
}

func (h *Handler) writeQueueEntry(w http.ResponseWriter, r *http.Request, entry QueueEntry, status int) {
	ctx := r.Context()
	names, err := h.repo.DisplayNames(ctx, []string{entry.StudentID})
	if err != nil {
		writeQueueError(w, err)
		return
	}
	phttp.WriteJSON(w, status, map[string]any{
		"id":           entry.ID,
		"student_id":   entry.StudentID,
		"student_name": names[entry.StudentID],
		"position":     entry.Position,
		"status":       entry.Status,
		"grade":        entry.Grade,
		"grade_notes":  entry.TeacherNotes,
		"version":      entry.Version,
	})
}

// UpdatePolicy applies the requested policy dimensions and returns QueuePolicy.
func (h *Handler) UpdatePolicy(w http.ResponseWriter, r *http.Request) {
	sessionID, actorID, _, ok := h.authorize(w, r, true)
	if !ok {
		return
	}
	var req policyPatchRequest
	if !phttp.DecodeJSONBody(w, r, &req) || !validateVersion(w, req.ExpectedVersion) {
		return
	}
	if receipt, replay := h.receipt(w, r, sessionID, actorID, "queue.update_policy"); replay {
		if receipt != nil && receipt.ResourceID != nil {
			policy, err := h.repo.SessionPolicy(r.Context(), *receipt.ResourceID)
			if err != nil {
				writeQueueError(w, err)
				return
			}
			phttp.WriteJSON(w, http.StatusOK, policyResponse(policy.Policy))
		}
		return
	}
	current, err := h.repo.SessionPolicy(r.Context(), sessionID)
	if err != nil {
		writeQueueError(w, err)
		return
	}
	next := current.Policy
	if req.Population != nil {
		next.Population = *req.Population
	}
	if req.Finalization != nil {
		next.Finalization = *req.Finalization
	}
	if req.OptOut != nil {
		next.OptOut = *req.OptOut
	}
	if req.GradeVisibility != nil {
		next.GradeVisibility = *req.GradeVisibility
	}
	if req.GradeCorrection != nil {
		next.GradeCorrection = *req.GradeCorrection
	}
	updated, err := h.policies.Update(r.Context(), sessionID, actorID, req.ExpectedVersion, next)
	if err != nil {
		writeQueueError(w, err)
		return
	}
	if err := h.recordReceipt(r, sessionID, actorID, sessionID, updated.Version); err != nil {
		writeQueueError(w, err)
		return
	}
	phttp.WriteJSON(w, http.StatusOK, policyResponse(updated))
}

// RequestOptOut creates or replays a student opt-out request under the
// session's opt-out policy. Managers are forbidden; the caller resolves their
// own entry by session and student identity.
func (h *Handler) RequestOptOut(w http.ResponseWriter, r *http.Request) {
	sessionID, actorID, manager, ok := h.authorize(w, r, false)
	if !ok {
		return
	}
	if manager {
		phttp.WriteError(w, httpconst.ErrorCodeForbidden, httpconst.ErrorMessageForbidden, http.StatusForbidden)
		return
	}

	entry, err := h.repo.FindOwnEntryForSession(r.Context(), sessionID, actorID)
	if err != nil {
		writeQueueError(w, err)
		return
	}
	status := http.StatusCreated
	if _, err := h.repo.FindOptOutRequestByEntryAndRequester(r.Context(), entry.ID, actorID); err == nil {
		status = http.StatusOK
	}

	if _, replay := h.receipt(w, r, sessionID, actorID, "queue.request_opt_out"); replay {
		result, err := h.optOuts.Request(r.Context(), sessionID, actorID)
		if err != nil {
			writeQueueError(w, err)
			return
		}
		h.writeOptOutResult(w, r, result, http.StatusOK)
		return
	}

	result, err := h.optOuts.Request(r.Context(), sessionID, actorID)
	if err != nil {
		writeQueueError(w, err)
		return
	}
	if err := h.recordReceipt(r, sessionID, actorID, result.Request.ID, result.Entry.Version); err != nil {
		writeQueueError(w, err)
		return
	}
	h.writeOptOutResult(w, r, result, status)
}

// DecideOptOutRequest allows a teacher or supervisor to approve or decline one
// pending opt-out request.
func (h *Handler) DecideOptOutRequest(w http.ResponseWriter, r *http.Request) {
	sessionID, actorID, _, ok := h.authorize(w, r, true)
	if !ok {
		return
	}
	requestID, ok := pathUUID(w, r, "requestId", "request_id")
	if !ok {
		return
	}
	var req optOutDecisionRequest
	if !phttp.DecodeJSONBody(w, r, &req) || !validateVersion(w, req.ExpectedEntryVersion) {
		return
	}
	if req.Decision != OptOutRequestStatusApproved && req.Decision != OptOutRequestStatusDeclined {
		writeQueueValidation(w, httpconst.ErrorCodeQueueInvalidEnum, httpconst.ErrorMessageQueueInvalidEnum, httpconst.FieldDecision)
		return
	}

	result, err := h.optOuts.Decide(r.Context(), sessionID, requestID, actorID, req.Decision, req.ExpectedEntryVersion)
	if err != nil {
		writeQueueError(w, err)
		return
	}
	h.writeOptOutResult(w, r, result, http.StatusOK)
}

func (h *Handler) writeOptOutResult(w http.ResponseWriter, r *http.Request, result OptOutResult, status int) {
	response, err := optOutResultResponse(r.Context(), h.repo, result)
	if err != nil {
		writeQueueError(w, err)
		return
	}
	phttp.WriteJSON(w, status, response)
}

func optOutResultResponse(ctx context.Context, repo *Repository, result OptOutResult) (map[string]any, error) {
	names, err := repo.DisplayNames(ctx, []string{result.Entry.StudentID})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"request": map[string]any{
			"id":             result.Request.ID,
			"queue_entry_id": result.Request.QueueEntryID,
			"status":         result.Request.Status,
			"requested_at":   result.Request.RequestedAt.UTC().Format(time.RFC3339Nano),
		},
		"entry": map[string]any{
			"id":           result.Entry.ID,
			"student_id":   result.Entry.StudentID,
			"student_name": names[result.Entry.StudentID],
			"position":     result.Entry.Position,
			"status":       result.Entry.Status,
			"version":      result.Entry.Version,
		},
	}, nil
}

func (h *Handler) replayState(w http.ResponseWriter, r *http.Request, receipt *CommandReceipt, actorID string) {
	if receipt == nil || receipt.ResourceID == nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return
	}
	h.writeState(w, r, *receipt.ResourceID, Viewer{UserID: actorID, IsManager: true}, http.StatusOK)
}

func (h *Handler) authorize(w http.ResponseWriter, r *http.Request, managerRequired bool) (string, string, bool, bool) {
	sessionID, ok := pathUUID(w, r, "sessionId", "session_id")
	if !ok {
		return "", "", false, false
	}
	principal, ok := auth.CurrentPrincipal(r.Context())
	if !ok || principal.UserID == "" {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return "", "", false, false
	}
	role, err := h.repo.SessionRole(r.Context(), sessionID, principal.UserID)
	if err != nil {
		writeQueueError(w, err)
		return "", "", false, false
	}
	manager := role == "teacher" || role == "supervisor"
	if role == "" || (managerRequired && !manager) {
		phttp.WriteError(w, httpconst.ErrorCodeForbidden, httpconst.ErrorMessageForbidden, http.StatusForbidden)
		return "", "", false, false
	}
	return sessionID, principal.UserID, manager, true
}

func (h *Handler) writeState(w http.ResponseWriter, r *http.Request, roundID string, viewer Viewer, status int) {
	state, err := h.repo.LoadQueueState(r.Context(), roundID, viewer)
	if err != nil {
		writeQueueError(w, err)
		return
	}
	response, err := queueStateResponse(r.Context(), h.repo, state)
	if err != nil {
		writeQueueError(w, err)
		return
	}
	phttp.WriteJSON(w, status, response)
}

func queueStateResponse(ctx context.Context, repo *Repository, state QueueState) (map[string]any, error) {
	ids := make([]string, 0, len(state.Entries)+len(state.Preorder))
	for _, entry := range state.Entries {
		ids = append(ids, entry.StudentID)
	}
	for _, candidate := range state.Preorder {
		ids = append(ids, candidate.StudentID)
	}
	names, err := repo.DisplayNames(ctx, ids)
	if err != nil {
		return nil, err
	}
	policy, err := repo.SessionPolicy(ctx, state.Round.SessionID)
	if err != nil {
		return nil, err
	}
	entries := make([]map[string]any, 0, len(state.Entries))
	for _, entry := range state.Entries {
		entries = append(entries, map[string]any{"id": entry.ID, "student_id": entry.StudentID, "student_name": names[entry.StudentID], "position": entry.Position, "status": entry.Status, "grade": entry.Grade, "grade_notes": entry.TeacherNotes, "version": entry.Version})
	}
	preorder := make([]map[string]any, 0, len(state.Preorder))
	for _, candidate := range state.Preorder {
		preorder = append(preorder, map[string]any{"student_id": candidate.StudentID, "student_name": names[candidate.StudentID], "position": candidate.Position})
	}
	return map[string]any{"session_id": state.Round.SessionID, "round_id": state.Round.ID, "round_number": state.Round.RoundNumber, "round_type": state.Round.Type, "lifecycle": state.Round.Lifecycle, "surah_id": state.Round.SurahID, "from_ayah": state.Round.FromAyah, "to_ayah": state.Round.ToAyah, "grading_required": state.Round.GradingRequired, "selected_entry_id": state.Round.SelectedEntryID, "version": state.Round.Version, "policy": policyResponse(policy.Policy), "preorder": preorder, "entries": entries}, nil
}

func policyResponse(policy QueuePolicy) map[string]any {
	return map[string]any{"population": policy.Population, "unfinished_finalization": policy.Finalization, "opt_out": policy.OptOut, "grade_visibility": policy.GradeVisibility, "grade_correction": policy.GradeCorrection, "version": policy.Version}
}

func (h *Handler) receipt(w http.ResponseWriter, r *http.Request, sessionID, actorID, command string) (*CommandReceipt, bool) {
	key := r.Header.Get(idempotencyKeyHeader)
	if key == "" {
		return nil, false
	}
	if err := ValidateIdempotencyKey(key); err != nil {
		writeQueueValidation(w, httpconst.ErrorCodeQueueInvalidRange, httpconst.ErrorMessageQueueInvalidRange, httpconst.FieldIdempotencyKey)
		return nil, true
	}
	var receipt *CommandReceipt
	var inserted bool
	err := h.repo.WithTx(r.Context(), func(tx *Tx) error {
		var err error
		receipt, inserted, err = tx.ReserveCommandReceipt(r.Context(), sessionID, actorID, key, command)
		return err
	})
	if err != nil {
		writeQueueError(w, err)
		return nil, true
	}
	if receipt != nil && receipt.ResourceID == nil {
		return nil, false
	}
	return receipt, !inserted
}

func (h *Handler) recordReceipt(r *http.Request, sessionID, actorID, resourceID string, version int64) error {
	key := r.Header.Get(idempotencyKeyHeader)
	if key == "" {
		return nil
	}
	return h.repo.WithTx(r.Context(), func(tx *Tx) error {
		return tx.UpdateCommandReceiptResult(r.Context(), sessionID, actorID, key, &resourceID, &version)
	})
}

func sameRoundRequest(round Round, req roundRequest) bool {
	return round.Type == req.RoundType && round.SurahID == req.SurahID &&
		round.FromAyah == req.FromAyah && round.ToAyah == req.ToAyah &&
		round.GradingRequired == req.GradingRequired
}

func prepareInput(sessionID, actorID string, req roundRequest) PrepareRoundInput {
	return PrepareRoundInput{SessionID: sessionID, Type: req.RoundType, SurahID: req.SurahID, FromAyah: req.FromAyah, ToAyah: req.ToAyah, SurahAyahCount: ayahCount(req.SurahID), GradingRequired: req.GradingRequired, CreatedBy: actorID, Preorder: req.StudentOrder}
}

func validateRoundRequest(w http.ResponseWriter, req roundRequest) bool {
	switch req.RoundType {
	case RoundTypeNewMemorization, RoundTypeRevision, RoundTypeOldRevision, RoundTypeTest:
	default:
		writeQueueValidation(w, httpconst.ErrorCodeQueueInvalidEnum, httpconst.ErrorMessageQueueInvalidEnum, httpconst.FieldRoundType)
		return false
	}
	if req.SurahID < 1 || req.SurahID > MaxSurahID || req.FromAyah < 1 || req.FromAyah > req.ToAyah || req.ToAyah > ayahCount(req.SurahID) {
		writeQueueValidation(w, httpconst.ErrorCodeQueueInvalidRange, httpconst.ErrorMessageQueueInvalidRange, httpconst.FieldSurahID)
		return false
	}
	return true
}

func validateVersion(w http.ResponseWriter, version int64) bool {
	if version < 1 {
		writeQueueValidation(w, httpconst.ErrorCodeQueueInvalidRange, httpconst.ErrorMessageQueueInvalidRange, httpconst.FieldExpectedVersion)
		return false
	}
	return true
}

func pathUUID(w http.ResponseWriter, r *http.Request, name, field string) (string, bool) {
	value := r.PathValue(name)
	if _, err := uuid.Parse(value); err != nil {
		writeQueueValidation(w, httpconst.ErrorCodeValidationFailed, httpconst.ErrorMessageValidationFailed, field)
		return "", false
	}
	return value, true
}

func writeQueueValidation(w http.ResponseWriter, code, message, field string) {
	phttp.WriteJSON(w, http.StatusUnprocessableEntity, phttp.ErrorEnvelope{Error: phttp.ErrorBody{Code: code, Message: message, Fields: map[string]string{field: message}}})
}

func writeQueueError(w http.ResponseWriter, err error) {
	var queueErr *QueueError
	if errors.As(err, &queueErr) {
		code, message := queueErrorResponse(queueErr.Code)
		status := http.StatusConflict
		if queueErr.Code == QueueErrorCodeValidation {
			status = http.StatusUnprocessableEntity
		}
		phttp.WriteError(w, code, message, status)
		return
	}
	if errors.Is(err, pgx.ErrNoRows) {
		phttp.WriteError(w, httpconst.ErrorCodeNotFound, httpconst.ErrorCodeNotFound, http.StatusNotFound)
		return
	}
	phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
}

func queueErrorResponse(code QueueErrorCode) (string, string) {
	switch code {
	case QueueErrorCodeStaleVersion:
		return httpconst.ErrorCodeQueueVersionConflict, httpconst.ErrorMessageQueueVersionConflict
	case QueueErrorCodeInvalidTransition:
		return httpconst.ErrorCodeQueueInvalidTransition, httpconst.ErrorMessageQueueInvalidTransition
	case QueueErrorCodeNoWaitingEntry:
		return httpconst.ErrorCodeQueueNoWaitingEntry, httpconst.ErrorMessageQueueNoWaitingEntry
	case QueueErrorCodeEntryReciting:
		return httpconst.ErrorCodeQueueEntryReciting, httpconst.ErrorMessageQueueEntryReciting
	case QueueErrorCodeEntryTerminal:
		return httpconst.ErrorCodeQueueEntryTerminal, httpconst.ErrorMessageQueueEntryTerminal
	case QueueErrorCodeRoundFinalized:
		return httpconst.ErrorCodeQueueRoundFinalized, httpconst.ErrorMessageQueueRoundFinalized
	case QueueErrorCodeDuplicateCommand:
		return httpconst.ErrorCodeQueueDuplicateCommand, httpconst.ErrorMessageQueueDuplicateCommand
	default:
		return httpconst.ErrorCodeQueueInvalidRange, strings.TrimSpace(httpconst.ErrorMessageQueueInvalidRange)
	}
}

func ayahCount(surahID int) int {
	counts := [...]int{0, 7, 286, 200, 176, 120, 165, 206, 75, 129, 109, 123, 111, 43, 52, 99, 128, 111, 110, 98, 135, 112, 78, 118, 64, 77, 227, 93, 88, 69, 60, 34, 30, 73, 54, 45, 83, 182, 88, 75, 85, 54, 53, 89, 59, 37, 35, 38, 29, 18, 45, 60, 49, 62, 55, 78, 96, 29, 22, 24, 13, 14, 11, 11, 18, 12, 12, 30, 52, 52, 44, 28, 28, 20, 56, 40, 31, 50, 40, 46, 42, 29, 19, 36, 25, 22, 17, 19, 26, 30, 20, 15, 21, 11, 8, 8, 19, 5, 8, 8, 11, 11, 8, 3, 9, 5, 4, 7, 3, 6, 3, 5, 4, 5, 6}
	if surahID < 1 || surahID >= len(counts) {
		return 0
	}
	return counts[surahID]
}
