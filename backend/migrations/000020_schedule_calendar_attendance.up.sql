-- 000020_schedule_calendar_attendance.up.sql
-- F-006 schedule, calendar and attendance persistence (ADR-025, ADR-026;
-- specs/006-schedule-calendar-attendance/data-model.md).
-- Additive only: extends profiles with an IANA timezone, adds the schedule/
-- revision/selected-date/exception/planned-detail/attendance/correction/replay
-- tables, and never alters F-005 enums or drops existing columns.

-- 1. Viewer timezone on profiles. Existing rows read the constant default
-- 'UTC'; new rows default to 'UTC' until the profile update flow sets an
-- IANA zone validated server-side.
ALTER TABLE profiles
    ADD COLUMN IF NOT EXISTS timezone TEXT NOT NULL DEFAULT 'UTC';

UPDATE profiles SET timezone = 'UTC' WHERE timezone IS NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM information_schema.table_constraints
        WHERE table_schema = current_schema()
          AND table_name = 'profiles'
          AND constraint_name = 'ck_profiles_timezone'
    ) THEN
        ALTER TABLE profiles
            ADD CONSTRAINT ck_profiles_timezone
            CHECK (timezone = btrim(timezone) AND timezone <> '');
    END IF;
END
$$;

-- 2. Stable schedule identity with a CAS counter for series edits.
CREATE TABLE IF NOT EXISTS schedules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    circle_id UUID NOT NULL,
    created_by UUID NOT NULL,
    current_version INTEGER NOT NULL,
    stopped_from_local_date DATE NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_schedules_circle_id FOREIGN KEY (circle_id) REFERENCES circles(id) ON DELETE RESTRICT,
    CONSTRAINT fk_schedules_created_by FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE RESTRICT,
    CONSTRAINT ck_schedules_current_version CHECK (current_version > 0)
);

CREATE INDEX IF NOT EXISTS idx_schedules_circle_stopped_from
    ON schedules (circle_id, stopped_from_local_date);

-- 3. Effective-dated recurrence revisions. Older revisions are retained so
-- past virtual and started history remain reproducible. The end clock equals
-- start plus duration by nominal 24-hour arithmetic; resolved UTC ends
-- preserve elapsed duration across DST.
CREATE OR REPLACE FUNCTION schedule_weekdays_unique(days SMALLINT[])
RETURNS BOOLEAN LANGUAGE SQL IMMUTABLE STRICT AS $$
    SELECT cardinality(days) = (SELECT COUNT(DISTINCT day) FROM unnest(days) AS selected(day))
$$;

CREATE TABLE IF NOT EXISTS schedule_revisions (
    schedule_id UUID NOT NULL,
    version INTEGER NOT NULL,
    effective_local_date DATE NOT NULL,
    mode VARCHAR(20) NOT NULL,
    title VARCHAR(200) NOT NULL DEFAULT 'Circle Session',
    anchor_local_date DATE NOT NULL,
    start_local_time TIME NOT NULL,
    end_local_time TIME NOT NULL,
    duration_minutes INTEGER NOT NULL,
    timezone TEXT NOT NULL,
    end_local_date DATE NULL,
    week_cadence SMALLINT NULL,
    weekdays SMALLINT[] NULL,
    interval_count INTEGER NULL,
    interval_unit VARCHAR(20) NULL,
    CONSTRAINT pk_schedule_revisions PRIMARY KEY (schedule_id, version),
    CONSTRAINT fk_schedule_revisions_schedule_id
        FOREIGN KEY (schedule_id) REFERENCES schedules(id) ON DELETE RESTRICT,
    CONSTRAINT ck_schedule_revisions_version CHECK (version > 0),
    CONSTRAINT ck_schedule_revisions_mode CHECK (mode IN ('weekday_pattern', 'interval', 'selected_dates')),
    CONSTRAINT ck_schedule_revisions_title CHECK (title = btrim(title) AND title <> ''),
    CONSTRAINT ck_schedule_revisions_duration CHECK (duration_minutes BETWEEN 1 AND 44640),
    CONSTRAINT ck_schedule_revisions_end_clock
        CHECK (end_local_time = start_local_time + (duration_minutes * INTERVAL '1 minute')),
    CONSTRAINT ck_schedule_revisions_timezone CHECK (timezone = btrim(timezone) AND timezone <> ''),
    CONSTRAINT ck_schedule_revisions_end_date
        CHECK (end_local_date IS NULL OR end_local_date >= anchor_local_date),
    CONSTRAINT ck_schedule_revisions_weekday_pattern CHECK (
        mode <> 'weekday_pattern'
        OR (week_cadence IS NOT NULL
            AND week_cadence IN (1, 2)
            AND weekdays IS NOT NULL
            AND cardinality(weekdays) > 0
            AND weekdays <@ ARRAY[0, 1, 2, 3, 4, 5, 6]::SMALLINT[]
            AND schedule_weekdays_unique(weekdays)
            AND EXTRACT(DOW FROM anchor_local_date)::SMALLINT = ANY (weekdays)
            AND interval_count IS NULL
            AND interval_unit IS NULL)
    ),
    CONSTRAINT ck_schedule_revisions_interval CHECK (
        mode <> 'interval'
        OR (interval_count IS NOT NULL
            AND interval_count > 0
            AND interval_unit IS NOT NULL
            AND interval_unit IN ('day', 'week')
            AND week_cadence IS NULL
            AND weekdays IS NULL)
    ),
    CONSTRAINT ck_schedule_revisions_selected_dates CHECK (
        mode <> 'selected_dates'
        OR (week_cadence IS NULL
            AND weekdays IS NULL
            AND interval_count IS NULL
            AND interval_unit IS NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_schedule_revisions_schedule_effective
    ON schedule_revisions (schedule_id, effective_local_date DESC, version DESC);

-- 4. Explicit dates for the selected_dates mode. Distinctness is the primary
-- key; "at/after the anchor, first equals anchor, within end date" is
-- validated against the parent revision in the application transaction.
CREATE TABLE IF NOT EXISTS schedule_selected_dates (
    schedule_id UUID NOT NULL,
    revision INTEGER NOT NULL,
    local_date DATE NOT NULL,
    CONSTRAINT pk_schedule_selected_dates PRIMARY KEY (schedule_id, revision, local_date),
    CONSTRAINT fk_schedule_selected_dates_revision
        FOREIGN KEY (schedule_id, revision)
        REFERENCES schedule_revisions (schedule_id, version) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_schedule_selected_dates_date
    ON schedule_selected_dates (local_date);

-- 5. Per-occurrence exceptions. (schedule_id, original_local_date) is the
-- stable logical occurrence identity even after the occurrence is moved; a
-- later series edit invalidates unstarted exception values, while started
-- history lives in planned_session_details.
CREATE TABLE IF NOT EXISTS schedule_occurrence_exceptions (
    schedule_id UUID NOT NULL,
    original_local_date DATE NOT NULL,
    series_version INTEGER NOT NULL,
    version INTEGER NOT NULL,
    replacement_local_date DATE NULL,
    replacement_start_local_time TIME NULL,
    replacement_end_local_time TIME NULL,
    replacement_duration_minutes INTEGER NULL,
    replacement_title VARCHAR(200) NULL,
    cancelled_at TIMESTAMPTZ NULL,
    updated_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT pk_schedule_occurrence_exceptions PRIMARY KEY (schedule_id, original_local_date),
    CONSTRAINT fk_schedule_occurrence_exceptions_schedule_id
        FOREIGN KEY (schedule_id) REFERENCES schedules(id) ON DELETE RESTRICT,
    CONSTRAINT fk_schedule_occurrence_exceptions_updated_by
        FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE RESTRICT,
    CONSTRAINT ck_schedule_occurrence_exceptions_series_version CHECK (series_version > 0),
    CONSTRAINT ck_schedule_occurrence_exceptions_version CHECK (version > 0),
    CONSTRAINT ck_schedule_occurrence_exceptions_replacement_duration CHECK (
        replacement_duration_minutes IS NULL
        OR replacement_duration_minutes BETWEEN 1 AND 44640
    ),
    CONSTRAINT ck_schedule_occurrence_exceptions_replacement_timing CHECK (
        (replacement_start_local_time IS NULL) = (replacement_end_local_time IS NULL)
        AND (replacement_start_local_time IS NULL) = (replacement_duration_minutes IS NULL)
        AND (replacement_start_local_time IS NULL
             OR replacement_end_local_time =
                replacement_start_local_time + (replacement_duration_minutes * INTERVAL '1 minute'))
    ),
    CONSTRAINT ck_schedule_occurrence_exceptions_replacement_title CHECK (
        replacement_title IS NULL
        OR (replacement_title = btrim(replacement_title) AND replacement_title <> '')
    )
);

-- 6. Planned metadata for materialized F-005 sessions. One-off rows carry no
-- schedule key; recurring rows are unique per logical occurrence. sessions
-- remains the sole live-session state; cancellation lives only here.
CREATE TABLE IF NOT EXISTS planned_session_details (
    session_id UUID PRIMARY KEY,
    schedule_id UUID NULL,
    original_local_date DATE NULL,
    title VARCHAR(200) NULL,
    start_local_time TIME NOT NULL,
    end_local_time TIME NOT NULL,
    duration_minutes INTEGER NOT NULL,
    planned_end_at TIMESTAMPTZ NOT NULL,
    planning_timezone TEXT NOT NULL,
    cancelled_at TIMESTAMPTZ NULL,
    version INTEGER NOT NULL,
    CONSTRAINT fk_planned_session_details_session_id
        FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE RESTRICT,
    CONSTRAINT fk_planned_session_details_schedule_id
        FOREIGN KEY (schedule_id) REFERENCES schedules(id) ON DELETE RESTRICT,
    CONSTRAINT ck_planned_session_details_occurrence_key
        CHECK ((schedule_id IS NULL) = (original_local_date IS NULL)),
    CONSTRAINT ck_planned_session_details_title CHECK (
        title IS NULL OR (title = btrim(title) AND title <> '')
    ),
    CONSTRAINT ck_planned_session_details_duration CHECK (duration_minutes BETWEEN 1 AND 44640),
    CONSTRAINT ck_planned_session_details_end_clock
        CHECK (end_local_time = start_local_time + (duration_minutes * INTERVAL '1 minute')),
    CONSTRAINT ck_planned_session_details_planning_timezone
        CHECK (planning_timezone = btrim(planning_timezone) AND planning_timezone <> ''),
    CONSTRAINT ck_planned_session_details_version CHECK (version > 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_planned_session_details_occurrence
    ON planned_session_details (schedule_id, original_local_date)
    WHERE schedule_id IS NOT NULL;

-- 7. Eligible roster and derived/final attendance per session. Finalization
-- fills both statuses; corrections never rewrite this base derivation.
CREATE TABLE IF NOT EXISTS session_attendance (
    session_id UUID NOT NULL,
    user_id UUID NOT NULL,
    base_status VARCHAR(20) NULL,
    effective_status VARCHAR(20) NULL,
    first_presence_at TIMESTAMPTZ NULL,
    roster_source VARCHAR(20) NOT NULL,
    finalized_at TIMESTAMPTZ NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT pk_session_attendance PRIMARY KEY (session_id, user_id),
    CONSTRAINT fk_session_attendance_session_id
        FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE RESTRICT,
    CONSTRAINT fk_session_attendance_user_id
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT,
    CONSTRAINT ck_session_attendance_base_status CHECK (
        base_status IS NULL OR base_status IN ('present', 'late', 'absent')
    ),
    CONSTRAINT ck_session_attendance_effective_status CHECK (
        effective_status IS NULL OR effective_status IN ('present', 'late', 'absent', 'excused')
    ),
    CONSTRAINT ck_session_attendance_roster_source
        CHECK (roster_source IN ('start_snapshot', 'later_participant')),
    CONSTRAINT ck_session_attendance_finalized CHECK (
        finalized_at IS NULL OR (base_status IS NOT NULL AND effective_status IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_session_attendance_session_status
    ON session_attendance (session_id, effective_status);

-- 8. Append-only teacher corrections; the latest valid correction wins and
-- every row must explain the difference from the base derivation.
CREATE TABLE IF NOT EXISTS attendance_corrections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL,
    user_id UUID NOT NULL,
    actor_id UUID NOT NULL,
    at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reason TEXT NOT NULL,
    previous_status VARCHAR(20) NULL,
    new_status VARCHAR(20) NOT NULL,
    CONSTRAINT fk_attendance_corrections_attendance
        FOREIGN KEY (session_id, user_id)
        REFERENCES session_attendance (session_id, user_id) ON DELETE RESTRICT,
    CONSTRAINT fk_attendance_corrections_actor_id
        FOREIGN KEY (actor_id) REFERENCES users(id) ON DELETE RESTRICT,
    CONSTRAINT ck_attendance_corrections_reason CHECK (reason = btrim(reason) AND reason <> ''),
    CONSTRAINT ck_attendance_corrections_previous_status CHECK (
        previous_status IS NULL OR previous_status IN ('present', 'late', 'absent', 'excused')
    ),
    CONSTRAINT ck_attendance_corrections_new_status
        CHECK (new_status IN ('present', 'late', 'absent', 'excused'))
);

CREATE INDEX IF NOT EXISTS idx_attendance_corrections_session_user_at
    ON attendance_corrections (session_id, user_id, at, id);

-- 9. Retry-safe mutation replays. (actor_id, idempotency_key) is unique;
-- replay with a different command fingerprint is a conflict. Prune only after
-- the documented retry window; never to change occurrence identity.
CREATE TABLE IF NOT EXISTS schedule_request_replays (
    actor_id UUID NOT NULL,
    idempotency_key VARCHAR(128) NOT NULL,
    command_fingerprint TEXT NOT NULL,
    response_status INTEGER NOT NULL,
    response_resource_id UUID NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT pk_schedule_request_replays PRIMARY KEY (actor_id, idempotency_key),
    CONSTRAINT fk_schedule_request_replays_actor_id
        FOREIGN KEY (actor_id) REFERENCES users(id) ON DELETE RESTRICT,
    CONSTRAINT ck_schedule_request_replays_idempotency_key CHECK (
        idempotency_key = btrim(idempotency_key) AND idempotency_key <> ''
    ),
    CONSTRAINT ck_schedule_request_replays_command_fingerprint CHECK (
        command_fingerprint = btrim(command_fingerprint) AND command_fingerprint <> ''
    ),
    CONSTRAINT ck_schedule_request_replays_response_status
        CHECK (response_status BETWEEN 100 AND 599)
);
