# Data Model: Circle Management

## Existing tables to extend

### `circles`

Reuse the table created by migration `000013_create_circles` and add the approved F-002 fields in one new additive migration:

| Column | Type | Constraints/default | Purpose |
|---|---|---|---|
| `id` | UUID | PK, existing | Circle identity |
| `name` | VARCHAR(100) | NOT NULL, existing | Display name |
| `description` | TEXT | nullable | Circle description; max 500 at API boundary |
| `rules` | TEXT | nullable | Circle rules; max 1000 at API boundary |
| `teacher_id` | UUID | FK users.id, existing | Compatibility owner/reference; must remain a valid teacher |
| `invite_code` | VARCHAR(20) | UNIQUE NOT NULL, existing | Current 8-character join code |
| `max_capacity` | INTEGER | DEFAULT 50, CHECK 2..200 | Maximum students |
| `is_private` | BOOLEAN | DEFAULT FALSE | Public discovery/join versus invite-only |
| `gender_restriction` | VARCHAR(20) | CHECK male/female/mixed/unspecified, default unspecified | Student-audience setting; independent of teacher gender |
| `language` | VARCHAR(10) | DEFAULT 'ar' | Primary circle language |
| `grading_policy` | VARCHAR(20) | existing architecture value | Preserved for later session features |
| `is_archived` | BOOLEAN | DEFAULT FALSE | Retirement/soft state; no hard deletion |
| `created_at` | TIMESTAMPTZ | DEFAULT NOW() | Creation timestamp |
| `updated_at` | TIMESTAMPTZ | DEFAULT NOW() | Mutation timestamp |

The migration must preserve existing rows and fill safe defaults. It must not drop data or rewrite applied migrations.

### `circle_members`

Reuse the table created by Feature 001 and migration `000014`:

- Composite primary key `(circle_id, user_id)` prevents duplicate membership.
- Foreign keys reference `circles.id` and `users.id` with existing cascade behavior only where approved by the architecture.
- `role` is constrained to `teacher`, `supervisor`, or `student`.
- Add/update indexes for circle listing and active-user membership counts if query evidence requires them.

## Invariants and transactions

1. Circle creation, initial teacher/supervisor assignment, invite-code generation, and compatibility `teacher_id` assignment occur in one transaction.
2. The creator and selected initial users must be distinct valid registered users; a backup supervisor cannot also be a selected teacher.
3. Public direct join and invite join lock the circle/member-count decision and insert exactly one student membership or return a conflict.
4. A user may have at most five active memberships; circle capacity counts student memberships and excludes manager roles unless the architecture confirms otherwise during implementation.
5. Role changes lock the target circle membership set; actor and target are distinct active members, actor is teacher/supervisor, and the result retains at least one teacher.
6. Invite refresh updates the code atomically and invalidates the previous code before returning success.
7. Archive sets `is_archived = TRUE`; archived circles remain queryable for authorized reads but reject joins, settings mutations, membership mutations, and future activity.
8. No circle hard-delete repository method, SQL statement, route, or cascade may be introduced.

## Conceptual entities

- **Circle**: Persistent Quran group and retirement state.
- **CircleMember**: Per-circle user membership and role.
- **InviteCode**: The current code stored on `circles.invite_code`; no separate table is required for MVP. Rotation history belongs in audit events, not a speculative new table.
- **AuditEvent**: Durable audit persistence is defined by the required `ADR-012-audit-logging-persistence.md`; do not create a table or external sink until that ADR fixes schema, retention, redaction, transaction, and access rules.

## Migration approach

- Add the next sequential migration after the current repository head; use a paired down migration.
- Do not edit `000013` or `000014`.
- Validate existing values before adding constraints/defaults.
- Existing rows without a circle gender may use the documented `unspecified` default; this does not infer any member's personal gender.
- Test upgrade from the current schema, fresh migration, rollback, and rerun/idempotency behavior.
