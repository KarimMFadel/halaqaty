-- 000017_recitation_queue_system.up.sql
-- F-003 recitation queue system: immutable Quran Surah reference seed, the six
-- sessions queue-policy columns with ADR-018 defaults/CHECKs, and the queue,
-- opt-out, idempotency, outbox, and progress tables. Every foreign key is a
-- plain FK with default NO ACTION (ADR-019 — no cascade on business/history
-- tables). Grades use the five canonical ADR-013 values.

-- Immutable Surah reference (1..114); migrations own all changes, the
-- application only reads it for ayah-range validation.
CREATE TABLE IF NOT EXISTS quran_surahs (
    id INTEGER PRIMARY KEY,
    name_arabic VARCHAR(100) NOT NULL,
    name_transliterated VARCHAR(100) NOT NULL,
    ayah_count INTEGER NOT NULL,
    CONSTRAINT ck_quran_surahs_ayah_count CHECK (ayah_count > 0)
);

INSERT INTO quran_surahs (id, name_arabic, name_transliterated, ayah_count) VALUES
    (1, 'الفاتحة', 'Al-Fatihah', 7),
    (2, 'البقرة', 'Al-Baqarah', 286),
    (3, 'آل عمران', 'Ali Imran', 200),
    (4, 'النساء', 'An-Nisa', 176),
    (5, 'المائدة', 'Al-Maidah', 120),
    (6, 'الأنعام', 'Al-Anam', 165),
    (7, 'الأعراف', 'Al-Araf', 206),
    (8, 'الأنفال', 'Al-Anfal', 75),
    (9, 'التوبة', 'At-Tawbah', 129),
    (10, 'يونس', 'Yunus', 109),
    (11, 'هود', 'Hud', 123),
    (12, 'يوسف', 'Yusuf', 111),
    (13, 'الرعد', 'Ar-Rad', 43),
    (14, 'إبراهيم', 'Ibrahim', 52),
    (15, 'الحجر', 'Al-Hijr', 99),
    (16, 'النحل', 'An-Nahl', 128),
    (17, 'الإسراء', 'Al-Isra', 111),
    (18, 'الكهف', 'Al-Kahf', 110),
    (19, 'مريم', 'Maryam', 98),
    (20, 'طه', 'Taha', 135),
    (21, 'الأنبياء', 'Al-Anbiya', 112),
    (22, 'الحج', 'Al-Hajj', 78),
    (23, 'المؤمنون', 'Al-Muminun', 118),
    (24, 'النور', 'An-Nur', 64),
    (25, 'الفرقان', 'Al-Furqan', 77),
    (26, 'الشعراء', 'Ash-Shuara', 227),
    (27, 'النمل', 'An-Naml', 93),
    (28, 'القصص', 'Al-Qasas', 88),
    (29, 'العنكبوت', 'Al-Ankabut', 69),
    (30, 'الروم', 'Ar-Rum', 60),
    (31, 'لقمان', 'Luqman', 34),
    (32, 'السجدة', 'As-Sajdah', 30),
    (33, 'الأحزاب', 'Al-Ahzab', 73),
    (34, 'سبأ', 'Saba', 54),
    (35, 'فاطر', 'Fatir', 45),
    (36, 'يس', 'Ya-Sin', 83),
    (37, 'الصافات', 'As-Saffat', 182),
    (38, 'ص', 'Sad', 88),
    (39, 'الزمر', 'Az-Zumar', 75),
    (40, 'غافر', 'Ghafir', 85),
    (41, 'فصلت', 'Fussilat', 54),
    (42, 'الشورى', 'Ash-Shura', 53),
    (43, 'الزخرف', 'Az-Zukhruf', 89),
    (44, 'الدخان', 'Ad-Dukhan', 59),
    (45, 'الجاثية', 'Al-Jathiyah', 37),
    (46, 'الأحقاف', 'Al-Ahqaf', 35),
    (47, 'محمد', 'Muhammad', 38),
    (48, 'الفتح', 'Al-Fath', 29),
    (49, 'الحجرات', 'Al-Hujurat', 18),
    (50, 'ق', 'Qaf', 45),
    (51, 'الذاريات', 'Adh-Dhariyat', 60),
    (52, 'الطور', 'At-Tur', 49),
    (53, 'النجم', 'An-Najm', 62),
    (54, 'القمر', 'Al-Qamar', 55),
    (55, 'الرحمن', 'Ar-Rahman', 78),
    (56, 'الواقعة', 'Al-Waqiah', 96),
    (57, 'الحديد', 'Al-Hadid', 29),
    (58, 'المجادلة', 'Al-Mujadila', 22),
    (59, 'الحشر', 'Al-Hashr', 24),
    (60, 'الممتحنة', 'Al-Mumtahanah', 13),
    (61, 'الصف', 'As-Saff', 14),
    (62, 'الجمعة', 'Al-Jumuah', 11),
    (63, 'المنافقون', 'Al-Munafiqun', 11),
    (64, 'التغابن', 'At-Taghabun', 18),
    (65, 'الطلاق', 'At-Talaq', 12),
    (66, 'التحريم', 'At-Tahrim', 12),
    (67, 'الملك', 'Al-Mulk', 30),
    (68, 'القلم', 'Al-Qalam', 52),
    (69, 'الحاقة', 'Al-Haqqah', 52),
    (70, 'المعارج', 'Al-Maarij', 44),
    (71, 'نوح', 'Nuh', 28),
    (72, 'الجن', 'Al-Jinn', 28),
    (73, 'المزمل', 'Al-Muzzammil', 20),
    (74, 'المدثر', 'Al-Muddaththir', 56),
    (75, 'القيامة', 'Al-Qiyamah', 40),
    (76, 'الإنسان', 'Al-Insan', 31),
    (77, 'المرسلات', 'Al-Mursalat', 50),
    (78, 'النبأ', 'An-Naba', 40),
    (79, 'النازعات', 'An-Naziat', 46),
    (80, 'عبس', 'Abasa', 42),
    (81, 'التكوير', 'At-Takwir', 29),
    (82, 'الانفطار', 'Al-Infitar', 19),
    (83, 'المطففين', 'Al-Mutaffifin', 36),
    (84, 'الانشقاق', 'Al-Inshiqaq', 25),
    (85, 'البروج', 'Al-Buruj', 22),
    (86, 'الطارق', 'At-Tariq', 17),
    (87, 'الأعلى', 'Al-Ala', 19),
    (88, 'الغاشية', 'Al-Ghashiyah', 26),
    (89, 'الفجر', 'Al-Fajr', 30),
    (90, 'البلد', 'Al-Balad', 20),
    (91, 'الشمس', 'Ash-Shams', 15),
    (92, 'الليل', 'Al-Layl', 21),
    (93, 'الضحى', 'Ad-Duhaa', 11),
    (94, 'الشرح', 'Ash-Sharh', 8),
    (95, 'التين', 'At-Tin', 8),
    (96, 'العلق', 'Al-Alaq', 19),
    (97, 'القدر', 'Al-Qadr', 5),
    (98, 'البينة', 'Al-Bayyinah', 8),
    (99, 'الزلزلة', 'Az-Zalzalah', 8),
    (100, 'العاديات', 'Al-Adiyat', 11),
    (101, 'القارعة', 'Al-Qariah', 11),
    (102, 'التكاثر', 'At-Takathur', 8),
    (103, 'العصر', 'Al-Asr', 3),
    (104, 'الهمزة', 'Al-Humazah', 9),
    (105, 'الفيل', 'Al-Fil', 5),
    (106, 'قريش', 'Quraysh', 4),
    (107, 'الماعون', 'Al-Maun', 7),
    (108, 'الكوثر', 'Al-Kawthar', 3),
    (109, 'الكافرون', 'Al-Kafirun', 6),
    (110, 'النصر', 'An-Nasr', 3),
    (111, 'المسد', 'Al-Masad', 5),
    (112, 'الإخلاص', 'Al-Ikhlas', 4),
    (113, 'الفلق', 'Al-Falaq', 5),
    (114, 'الناس', 'An-Nas', 6);

-- F-003 queue policy on sessions (ADR-018 defaults; prospective changes only).
-- NOT NULL DEFAULT backfills pre-existing rows on re-up after a down.
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS queue_population_policy VARCHAR(32) NOT NULL DEFAULT 'present_at_activation';
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS queue_finalization_policy VARCHAR(32) NOT NULL DEFAULT 'mark_unfinished_skipped';
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS queue_opt_out_policy VARCHAR(24) NOT NULL DEFAULT 'approval_required';
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS queue_grade_visibility VARCHAR(32) NOT NULL DEFAULT 'managers_and_student';
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS queue_grade_correction VARCHAR(32) NOT NULL DEFAULT 'audited_any_time';
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS queue_policy_version BIGINT NOT NULL DEFAULT 1;
ALTER TABLE sessions ADD CONSTRAINT ck_sessions_queue_population_policy CHECK (queue_population_policy IN ('present_at_activation', 'all_active_students'));
ALTER TABLE sessions ADD CONSTRAINT ck_sessions_queue_finalization_policy CHECK (queue_finalization_policy IN ('mark_unfinished_skipped', 'preserve_last_state'));
ALTER TABLE sessions ADD CONSTRAINT ck_sessions_queue_opt_out_policy CHECK (queue_opt_out_policy IN ('approval_required', 'auto_approve'));
ALTER TABLE sessions ADD CONSTRAINT ck_sessions_queue_grade_visibility CHECK (queue_grade_visibility IN ('managers_and_student', 'managers_only', 'all_participants'));
ALTER TABLE sessions ADD CONSTRAINT ck_sessions_queue_grade_correction CHECK (queue_grade_correction IN ('audited_any_time', 'before_round_finalization', 'immutable'));
ALTER TABLE sessions ADD CONSTRAINT ck_sessions_queue_policy_version CHECK (queue_policy_version > 0);

-- Recitation rounds. Partial unique (session_id) WHERE lifecycle = 'active'
-- keeps at most one active round while any number of prepared rounds stack.
CREATE TABLE IF NOT EXISTS recitation_queue (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL,
    round_number INTEGER NOT NULL,
    round_type VARCHAR(30) NOT NULL,
    surah_id INTEGER NOT NULL,
    from_ayah INTEGER NOT NULL,
    to_ayah INTEGER NOT NULL,
    grading_required BOOLEAN NOT NULL,
    lifecycle VARCHAR(16) NOT NULL,
    selected_entry_id UUID NULL,
    version BIGINT NOT NULL DEFAULT 1,
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    activated_at TIMESTAMPTZ NULL,
    finalized_at TIMESTAMPTZ NULL,
    CONSTRAINT fk_recitation_queue_session_id FOREIGN KEY (session_id) REFERENCES sessions(id),
    CONSTRAINT fk_recitation_queue_surah_id FOREIGN KEY (surah_id) REFERENCES quran_surahs(id),
    CONSTRAINT fk_recitation_queue_created_by FOREIGN KEY (created_by) REFERENCES users(id),
    CONSTRAINT ck_recitation_queue_round_number CHECK (round_number > 0),
    CONSTRAINT ck_recitation_queue_round_type CHECK (round_type IN ('new_memorization', 'revision', 'old_revision', 'test')),
    CONSTRAINT ck_recitation_queue_ayah_range CHECK (from_ayah > 0 AND to_ayah >= from_ayah),
    CONSTRAINT ck_recitation_queue_lifecycle CHECK (lifecycle IN ('prepared', 'active', 'finalized')),
    CONSTRAINT ck_recitation_queue_version CHECK (version > 0),
    CONSTRAINT uq_recitation_queue_session_round UNIQUE (session_id, round_number)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_recitation_queue_one_active
    ON recitation_queue (session_id)
    WHERE lifecycle = 'active';

-- Pre-activation candidate list (manager surface only).
CREATE TABLE IF NOT EXISTS recitation_queue_preorder (
    queue_id UUID NOT NULL,
    student_id UUID NOT NULL,
    position INTEGER NOT NULL,
    added_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (queue_id, student_id),
    CONSTRAINT fk_queue_preorder_queue_id FOREIGN KEY (queue_id) REFERENCES recitation_queue(id),
    CONSTRAINT fk_queue_preorder_student_id FOREIGN KEY (student_id) REFERENCES users(id),
    CONSTRAINT fk_queue_preorder_added_by FOREIGN KEY (added_by) REFERENCES users(id),
    CONSTRAINT ck_queue_preorder_position CHECK (position > 0),
    CONSTRAINT uq_queue_preorder_position UNIQUE (queue_id, position)
);

-- Materialized queue entries. Partial unique (queue_id) WHERE status =
-- 'reciting' is the final one-reciter race guard.
CREATE TABLE IF NOT EXISTS recitation_queue_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    queue_id UUID NOT NULL,
    student_id UUID NOT NULL,
    position INTEGER NOT NULL,
    status VARCHAR(20) NOT NULL,
    grade VARCHAR(30) NULL,
    teacher_notes VARCHAR(500) NULL,
    version BIGINT NOT NULL DEFAULT 1,
    started_at TIMESTAMPTZ NULL,
    completed_at TIMESTAMPTZ NULL,
    resolved_by UUID NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_queue_entries_queue_id FOREIGN KEY (queue_id) REFERENCES recitation_queue(id),
    CONSTRAINT fk_queue_entries_student_id FOREIGN KEY (student_id) REFERENCES users(id),
    CONSTRAINT fk_queue_entries_resolved_by FOREIGN KEY (resolved_by) REFERENCES users(id),
    CONSTRAINT ck_queue_entries_position CHECK (position > 0),
    CONSTRAINT ck_queue_entries_status CHECK (status IN ('waiting', 'reciting', 'completed', 'skipped', 'opted_out')),
    CONSTRAINT ck_queue_entries_grade CHECK (grade IS NULL OR grade IN ('excellent', 'good', 'acceptable', 'needs_review', 'repeat')),
    CONSTRAINT ck_queue_entries_version CHECK (version > 0),
    CONSTRAINT uq_queue_entries_round_student UNIQUE (queue_id, student_id),
    CONSTRAINT uq_queue_entries_round_position UNIQUE (queue_id, position)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_queue_entries_one_reciter
    ON recitation_queue_entries (queue_id)
    WHERE status = 'reciting';

-- Selection FK added after entries exist (queue and entries reference each other).
ALTER TABLE recitation_queue
    ADD CONSTRAINT fk_recitation_queue_selected_entry_id
    FOREIGN KEY (selected_entry_id) REFERENCES recitation_queue_entries(id);

-- Opt-out requests; at most one pending per entry (decided rows retained for audit).
CREATE TABLE IF NOT EXISTS queue_opt_out_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    queue_entry_id UUID NOT NULL,
    requested_by UUID NOT NULL,
    status VARCHAR(16) NOT NULL,
    decided_by UUID NULL,
    requested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    decided_at TIMESTAMPTZ NULL,
    idempotency_key VARCHAR(128) NULL,
    CONSTRAINT fk_opt_out_requests_queue_entry_id FOREIGN KEY (queue_entry_id) REFERENCES recitation_queue_entries(id),
    CONSTRAINT ck_opt_out_requests_status CHECK (status IN ('pending', 'approved', 'declined')),
    CONSTRAINT uq_opt_out_requests_idempotency UNIQUE (requested_by, idempotency_key)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_opt_out_requests_one_pending
    ON queue_opt_out_requests (queue_entry_id)
    WHERE status = 'pending';

-- Idempotency receipts keyed by (session, actor, key); no request/response payloads.
CREATE TABLE IF NOT EXISTS queue_command_receipts (
    session_id UUID NOT NULL,
    actor_id UUID NOT NULL,
    idempotency_key VARCHAR(128) NOT NULL,
    command VARCHAR(48) NOT NULL,
    resource_id UUID NULL,
    result_version BIGINT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (session_id, actor_id, idempotency_key)
);

-- Transactional outbox; attempt_count non-negative, parked when retries exhaust.
CREATE TABLE IF NOT EXISTS queue_event_outbox (
    event_id UUID PRIMARY KEY,
    session_id UUID NOT NULL,
    round_id UUID NOT NULL,
    event_type VARCHAR(48) NOT NULL,
    resource_id UUID NULL,
    round_version BIGINT NOT NULL,
    event_metadata JSONB NOT NULL,
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    delivered_at TIMESTAMPTZ NULL,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    parked_at TIMESTAMPTZ NULL,
    CONSTRAINT fk_event_outbox_session_id FOREIGN KEY (session_id) REFERENCES sessions(id),
    CONSTRAINT fk_event_outbox_round_id FOREIGN KEY (round_id) REFERENCES recitation_queue(id),
    CONSTRAINT ck_event_outbox_attempt_count CHECK (attempt_count >= 0)
);

-- Dispatcher scan support (Gate A note): only undelivered, unparked rows by
-- due order. Drops with the table in the down migration.
CREATE INDEX IF NOT EXISTS ix_event_outbox_dispatch
    ON queue_event_outbox (available_at)
    WHERE delivered_at IS NULL AND parked_at IS NULL;

-- Completed-turn practice history (ADR-019: all five FKs plain NO ACTION;
-- queue_entry_id NOT NULL UNIQUE is the idempotent completion/re-grade target).
CREATE TABLE IF NOT EXISTS memorization_progress (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    student_id UUID NOT NULL,
    circle_id UUID NOT NULL,
    session_id UUID NOT NULL,
    queue_entry_id UUID NOT NULL,
    surah_id INTEGER NOT NULL,
    surah_name VARCHAR(100) NOT NULL,
    from_ayah INTEGER NOT NULL,
    to_ayah INTEGER NOT NULL,
    type VARCHAR(30) NOT NULL,
    grade VARCHAR(30) NULL,
    notes VARCHAR(500) NULL,
    date DATE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_progress_student_id FOREIGN KEY (student_id) REFERENCES users(id),
    CONSTRAINT fk_progress_circle_id FOREIGN KEY (circle_id) REFERENCES circles(id),
    CONSTRAINT fk_progress_session_id FOREIGN KEY (session_id) REFERENCES sessions(id),
    CONSTRAINT fk_progress_queue_entry_id FOREIGN KEY (queue_entry_id) REFERENCES recitation_queue_entries(id),
    CONSTRAINT fk_progress_surah_id FOREIGN KEY (surah_id) REFERENCES quran_surahs(id),
    CONSTRAINT ck_progress_type CHECK (type IN ('new_memorization', 'revision', 'old_revision', 'test')),
    CONSTRAINT ck_progress_grade CHECK (grade IS NULL OR grade IN ('excellent', 'good', 'acceptable', 'needs_review', 'repeat')),
    CONSTRAINT uq_progress_queue_entry UNIQUE (queue_entry_id)
);
