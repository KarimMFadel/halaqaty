# F-008 Source Handoff Map

| Source | Approved event seam | Recipient and eligibility owner | F-008 behavior |
|---|---|---|---|
| F-003 queue | Stable `queue_event_outbox.event_id`, event type `queue.next_soon` / `queue.your_turn` | F-003 selected/waiting queue entry | Idempotent recipient history and push intent; never claim queue outbox ownership. |
| F-004 chat | Durable committed message ID via a narrow post-commit notification projection | F-004 conversation membership and direct/group recipient rules | Generic push/notice projection only; no duplicate message storage and no replacement of F-004 WebSocket delivery. |
| F-005 sessions | Existing `session.started` / `session.ended` lifecycle event | F-005 authorized subscribers/participants | Metadata-only history/FCM projection; no reconnect/snapshot duplicate and no media fields. |
| F-006 scheduling | Current occurrence/revision and `EligibleReminders` boundary | F-006 reminder eligibility plus circle/source gates | Create at approved offsets; recheck current eligibility immediately before dispatch; suppress stale/cancelled/superseded. |
| F-002 circles/invitations | Successful join/archive transitions and the approved role-bound invitation send to its named recipient (OQ-036/ADR-010) | F-002 membership, role, recipient, and invitation state | F-008 creates recipient history and delivery when the authorized invitation send commits. T023 integrates the send transition under existing F-002 authorization; do not map reusable code sharing or membership acceptance to it. Manager-deletion archive notice uses mandatory preference override. |
| F-007 grades | Posted grade identifier | F-007 visibility and recitation-log authorization | Generic notice deep-linking to authorized recitation log; no grade details in lock-screen payload. |
| F-001 identity/device | Authenticated token registration and account closure | F-001 identity lifecycle | Persist/refresh/remove device tokens; account closure removes actor's tokens under auth boundary. |

Source handoffs are designed in the owning feature package and same database transaction where its current transition permits it. If an existing durable source outbox is the only safe seam, F-008 consumes that source-owned outbox through its documented API; it never claims, updates, or repurposes its records. Any cross-package dependency that cannot preserve atomicity or avoid a new source event requires a surfaced design decision before code.
