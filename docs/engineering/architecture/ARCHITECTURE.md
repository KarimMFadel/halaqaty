# Halaqaty — Technical Architecture

> **Version:** 1.0 | **Status:** Planning Phase | **Last Updated:** 2026

**Related Documents:** [product/PRD.md](../../management/product/PRD.md) · [planning/PROJECT_PLAN.md](../../management/planning/PROJECT_PLAN.md) · [deployment/DEPLOYMENT.md](../deployment/DEPLOYMENT.md) · [README.md (ADR index)](./README.md) · [../../../DEVELOPMENT.md](../../../DEVELOPMENT.md) · [collaboration/AGENT_COLLABORATION_GUIDE.md](../collaboration/AGENT_COLLABORATION_GUIDE.md)

> **Key architectural decisions** (framework choice, state management, auth boundary, migrations) are documented as ADRs in [`./adr/`](./adr/) — see [`./README.md`](./README.md) for the live index.

---

## Table of Contents

1. [System Overview Diagram](#1-system-overview-diagram)
2. [Communication Protocols](#2-communication-protocols)
3. [Session-Media Provider + Flutter Integration](#3-session-media-provider--flutter-integration)
4. [Database Schema](#4-database-schema)
5. [API Endpoint Planning](#5-api-endpoint-planning)
6. [Security Considerations](#6-security-considerations)

---

## 1. System Overview Diagram

```mermaid
graph TD
    subgraph CLIENT["📱 Client Layer"]
        Flutter["Flutter App\n(iOS · Android · Web)"]
        subgraph UI["UI Modules"]
            AuthUI["Auth UI"]
            CirclesUI["Circles UI"]
            ChatUI["Chat UI"]
            SessionUI["Session UI\n(Queue + Media)"]
        end
        Pkgs["livekit_client · firebase_auth\nfirebase_messaging · riverpod"]
    end

    subgraph BACKEND["⚙️ Backend Layer"]
        subgraph GoServer["Go Backend (Echo v4)"]
            REST["REST API\n/api/v1/*"]
            WSHub["WebSocket Hub\n(Chat · Queue · Presence)"]
            LKMgr["Media Adapter\n(LiveKit in MVP)"]
        end
        REST --> LKMgr
        WSHub --> LKMgr
    end

    subgraph DATA["🗄️ Data & Services Layer"]
        PG[("PostgreSQL 16\nPrimary DB\n(source of truth)")]
        MinIO[("MinIO\nFile Store\n(voice · images · files)")]
        LK["LiveKit SFU\n(WebRTC · audio-only MVP)"]
        FB["Firebase\nAuth (identity)\nFCM (push notifs)"]
        CF["Cloudflare\nDNS · TLS"]
    end

    Flutter -->|"HTTPS/REST\nAuth · CRUD"| REST
    Flutter -->|"WebSocket\nChat · Queue · Presence"| WSHub
    Flutter -->|"WebRTC\naudio via LiveKit"| LK

    REST <-->|"pgx queries"| PG
    WSHub <-->|"pgx queries"| PG
    LKMgr <-->|"LiveKit SDK"| LK
    REST -->|"FCM dispatch"| FB
    REST -->|"file upload/serve"| MinIO
```

---

## 2. Communication Protocols

### 2.1 HTTPS / REST API

**Used for:** All standard CRUD operations, authentication, file uploads, configuration.

- Stateless request-response model
- JWT authentication header: `Authorization: Bearer <token>`
- JSON request/response bodies
- Standard HTTP status codes
- Versioned: `/api/v1/...`

**When to use REST (not WebSocket):**
- Creating/reading/updating/deleting resources (circles, sessions, users, progress records)
- File upload (photos, voice notes — multipart to presigned MinIO URL)
- Authentication token exchange
- Long-form data retrieval (history, reports)

### 2.2 WebSocket (Real-time)

**Used for:** Real-time communication that requires low latency and server-push capability.

**WebSocket connection lifecycle:**

```mermaid
sequenceDiagram
    participant C as Flutter Client
    participant API as Go REST API
    participant WS as WebSocket Hub
    participant DB as PostgreSQL

    C->>API: POST /realtime/tickets
    API->>DB: verify current device session and eligible circle topics
    DB-->>API: authorized circle topics
    API-->>C: { token, expires_at } (60s TTL)

    C->>WS: WSS wss://api.halaqaty.app/ws?token=<ws_token>
    WS->>DB: validate ticket and revalidate subscriptions
    DB-->>WS: circle topics; session topics after authorized join
    WS-->>C: connection upgraded ✓
    Note over WS: client registered in Hub rooms<br/>(circle IDs, session IDs)

    loop Every 30 seconds
        C->>WS: { "type": "ping" }
        WS-->>C: { "type": "pong", "server_time": "..." }
    end

    Note over C,WS: On disconnect → Hub removes client<br/>Client reconnects + re-fetches state via REST

### 2.5 Real-Time Data Flow — End-to-End Example

How a teacher prepares a queue round, then automatic activation propagates when
the F-005 session becomes live:

```mermaid
sequenceDiagram
        participant T as Teacher (Flutter)
        participant API as Go REST API
        participant DB as PostgreSQL
        participant Hub as WebSocket Hub
        participant S as Student (Flutter)
        participant O as Other Students

        rect rgb(230,245,255)
            Note over T,O: Teacher prepares a new recitation round
            T->>+API: POST /sessions/{id}/queue/rounds\n{ surah_id:2, from_ayah:1, to_ayah:10 }
            API->>DB: INSERT prepared recitation_queue (round 1, surah_id=2)
            DB-->>API: prepared QueueState
            API-->>-T: 201 QueueState { lifecycle: prepared }
            Note over API,DB: F-005 session becomes live; no active round exists
            API->>DB: Activate lowest prepared round and materialize entries per policy
            API->>Hub: emit queue.round_started to session room
            Hub-->>T: queue.round_started { round_number:1, surah_id:2, ... }
            Hub-->>O: queue.round_started { round_number:1, surah_id:2, ... }
            Hub-->>S: queue.round_started { round_number:1, surah_id:2, ... }
        end

        rect rgb(230,255,230)
            Note over T,O: First student's turn begins
            API->>Hub: emit queue.your_turn to Student S (targeted)
            Hub-->>S: queue.your_turn { queue_entry_id, surah_id:2, from_ayah:1, to_ayah:10 }
            API->>Hub: emit queue.entry_updated to all (broadcast)
            Hub-->>T: queue.entry_updated { new_status: reciting, student_id: S }
            Hub-->>O: queue.entry_updated { new_status: reciting, student_id: S }
        end

        rect rgb(255,245,230)
            Note over T,O: Participant raises hand (WS command — no REST round-trip)
            S->>Hub: cmd.raise_hand { session_id }
            Hub->>DB: log raise_hand event
            Hub-->>T: session.hand_raised { student_id, student_name }
            Hub-->>O: session.hand_raised { student_id, student_name }
        end
```
```

**Event Envelope (all events):**
```json
{
  "type": "event.name",
  "payload": { "...": "..." },
  "timestamp": "2024-01-15T10:30:00Z",
  "request_id": "optional-client-correlation-id"
}
```

**Server → Client event types:**

| Type | Description |
|------|-------------|
| `queue.state` | Full queue snapshot — sent on join or queue reset |
| `queue.entry_updated` | Single queue entry status change |
| `queue.your_turn` | Targeted: notifies the student whose turn it is |
| `queue.next_soon` | Targeted: notifies the student who is next (position 2) |
| `queue.reordered` | Teacher manually reordered the queue |
| `queue.round_started` | New recitation round started |
| `queue.advanced` | Manager selected the next waiting entry without starting it |
| `queue.round_finalized` | Prior round became immutable after reset/session end |
| `queue.policy_changed` | Closed session queue policy changed prospectively |
| `queue.opt_out_requested` | Targeted to managers for a pending approval |
| `queue.grade_submitted` | Grade recorded for a completed turn; visibility follows session queue policy (default: managers and graded student) |
| `session.started` | Session went live; contains session metadata only (credentials come from authorized start/join REST responses) |
| `session.ended` | Session ended by teacher or supervisor, or automatically by duration/idle policy |
| `session.participant_joined` | A participant joined the session |
| `session.participant_left` | A participant left the session |
| `session.hand_raised` | A participant raised their hand |
| `chat.message` | New circle message delivered |
| `chat.message_read` | Recipient read a message (sent to sender) |
| `chat.typing` | Typing indicator |
| `error` | Server could not process a client command |

**Client → Server command types:**

| Type | Description |
|------|-------------|
| `cmd.raise_hand` | Participant raises hand in session |
| `cmd.lower_hand` | Participant lowers hand in session |
| `ping` | Heartbeat (every 30 s) |

> **Source of truth for all event schemas and payloads:** [`docs/contracts/ws_events.md`](../../contracts/ws_events.md)

> **Reconnection:** on reconnect, clients re-fetch state via REST (`GET /api/v1/sessions/{id}/queue`, etc.) rather than relying solely on buffered WebSocket events.

### 2.3 Session Media via Provider Boundary (MVP Audio-Only with LiveKit)

**Used for:** Audio streaming in live sessions for MVP (video remains post-MVP behind feature flag).

- Session and queue code depend on the provider-neutral boundaries defined in [ADR-015](adr/ADR-015-session-media-provider-boundary.md); they never import LiveKit SDK types
- LiveKit is the sole MVP adapter and SFU — it receives each participant's stream and forwards it to all others, without mixing
- This is more scalable than peer-to-peer WebRTC (which doesn't scale beyond ~4 participants)
- Flutter client uses `livekit_client` package (official LiveKit Flutter SDK)
- Go backend uses `livekit-server-sdk-go` for room management and token generation

**Quality levels supported:**
- Audio: Opus codec, 48–64 kbps, mono (sufficient for voice)
- Video (post-MVP feature flag): VP8/VP9, simulcast (low/medium/high), adaptive bitrate

### 2.4 Firebase Cloud Messaging (FCM)

**Used for:** Push notifications when the app is in the background or completely closed.

**Flow — WS online vs FCM offline decision:**

```mermaid
flowchart TD
    E["⚡ Event occurs\n(session starts · your turn · new message)"]
    E --> Q{Is user connected\nvia WebSocket?}
    Q -->|Yes – online / foreground| WS["📨 Deliver via WebSocket\n(zero latency, in-app)"]
    Q -->|No – offline / background| DB[("🗄️ Fetch FCM device\ntokens from PostgreSQL")]
    DB --> FCM["POST Firebase FCM API"]
    FCM --> OS{Platform}
    OS -->|iOS| APNs["🍎 Apple APNs → device"]
    OS -->|Android| AFCM["🤖 Android FCM → device"]
    APNs --> N["🔔 System notification\n(app closed or backgrounded)"]
    AFCM --> N
    WS --> Bell["🔔 In-app notification bell\n+ real-time UI update"]
```

---

## 3. Session-Media Provider + Flutter Integration

### 3.0 Provider Boundary

LiveKit is the only MVP implementation, but it is isolated behind feature-local
compile-time adapters:

- `backend/internal/sessions` owns `SessionMediaGateway` and neutral session/media
  types; `backend/internal/sessions/livekit` owns all LiveKit SDK, credential, room,
  track, and webhook details.
- F-003 imports no media-control or provider type; it records voluntary queue
  order and displayed turn state only. F-005 owns participant audio and explicit
  moderation.
- `mobile/lib/features/sessions/application` owns `MediaSession`; only
  `mobile/lib/features/sessions/data/livekit_media_session.dart` imports
  `livekit_client`.
- Start/join REST operations return an opaque participant-specific
  `media_connection`. WebSocket broadcasts never contain connection credentials.
- MVP constructs the LiveKit adapters directly. Provider identifiers, registries,
  driver switches, and selection flags are deferred until a second provider is
  approved and introduced through ADR-015's session-pinned rollout.

This is a targeted dependency-inversion seam, not a project-wide Clean/Onion
Architecture conversion.

The boundary name deliberately allows a future approved video-session feature,
but the F-005 gateway and mobile contract remain audio-only. Video, camera,
screen-share, recording, and generic capability maps are not added speculatively;
future video extends or composes the seam through its own specification and ADR.

### 3.1 Complete Integration Flow

```mermaid
sequenceDiagram
    participant T as Teacher (Flutter)
    participant API as Go Backend
    participant DB as PostgreSQL
    participant Media as SessionMediaGateway
    participant LK as LiveKit Server
    participant S as Student (Flutter)

    Note over T,S: Step 1 — Teacher starts session
    T->>API: POST /sessions/{id}/start
    API->>DB: Lock session; verify scheduled or active replay
    DB-->>API: Session remains non-joinable while provisioning
    API->>Media: EnsureRoom(session_id)
    Media->>LK: CreateRoom(adapter room ref)
    LK-->>Media: room created ✓
    Media-->>API: room ready
    API->>DB: CAS status='active', media_room_ref=ref
    Note right of API: Activation failure closes the orphan; reconciler repairs crash windows
    API-->>T: { session, media_connection [teacher permissions] }
    T->>LK: LiveKitMediaSession.connect(endpoint, credential)
    LK-->>T: Connected ✓ (WebRTC handshake)
    API--)S: WS session.started { session_id, circle_id } (notification only; no credentials)

    Note over T,S: Step 2 — Student joins
    S->>API: POST /sessions/{id}/join
    API->>DB: verify circle membership
    API->>Media: IssueConnection(participant, audio publish enabled)
    Media->>LK: GenerateToken(uid, CanPublish=true, CanPublishVideo=false)
    LK-->>Media: identity-scoped credential
    Media-->>API: MediaConnection
    API-->>S: { session, media_connection }
    S->>LK: LiveKitMediaSession.connect(endpoint, credential)
    LK-->>S: Connected ✓ (publishes/subscribes to audio streams)

    Note over T,S: Step 3 — Student's turn (manager advances then starts)
    T->>API: POST /sessions/{id}/queue/rounds\n{ round_type, surah_id, from_ayah, to_ayah, grading_required }
    API->>DB: INSERT recitation_queue + entries
    T->>API: POST /sessions/{id}/queue/advance
    T->>API: PUT /sessions/{id}/queue/entries/{id}/status\n{ status: "reciting", expected_entry_version }
    API--)S: WS queue.your_turn { queue_entry_id, surah_id, from_ayah, to_ayah }
    S->>LK: publishAudioTrack() [48kbps Opus]
    LK-->>T: receives student audio stream ✓

    Note over T,S: Step 4 — Turn ends and completion is recorded atomically
    T->>API: PUT /sessions/{id}/queue/entries/{id}/status\n{ status: "completed", grade: "excellent", expected_entry_version }
    API->>DB: UPDATE entry + INSERT one memorization_progress
    API--)S: WS queue.entry_updated { new_status: completed }
    API--)T: WS queue.grade_submitted { grade, student_id }
```

The diagram shows the required safety boundary, not a distributed transaction.
Room references are deterministic, opaque, and non-guessable; the adapter uses
a backend-keyed derivation rather than a literal session ID. Gateway operations are idempotent. A
session becomes joinable only after the room is ready and the `active` transition
commits. If the process crashes between provider and database operations, F-005's
sessions-owned reconciler closes orphan rooms or repairs missing rooms. Ending a
session commits `ended` first to block new joins, then closes the provider room;
failed close operations are retried idempotently.

### 3.2 LiveKit Token Security Flow

The Go backend is the **sole token issuer** — the Flutter client never calls LiveKit APIs directly (Constitution §IV.1). Tokens encode the minimum permission set for each role.

```mermaid
sequenceDiagram
    participant App as Flutter App
    participant Mobile as LiveKitMediaSession
    participant API as Go Backend
    participant DB as PostgreSQL
    participant Adapter as Backend LiveKit Adapter
    participant LK as LiveKit Server

    Note over App,LK: Teacher token — issued on session start
    App->>API: POST /sessions/{id}/start
    API->>DB: verify role = teacher in circle_members
    API->>Adapter: IssueConnection(teacher permissions)
    Adapter->>LK: GenerateToken(uid, RoomAdmin=true,\nCanPublish=true, CanPublishVideo=false)
    LK-->>Adapter: identity-specific token
    Adapter-->>API: MediaConnection
    API-->>App: { media_connection }
    App->>Mobile: connect(media_connection)
    Mobile->>LK: room.connect(endpoint, credential) — RoomAdmin=true ✓

    Note over App,LK: Student token — issued on session join
    App->>API: POST /sessions/{id}/join
    API->>DB: verify role = student/supervisor in circle_members
    API->>Adapter: IssueConnection(student audio permissions)
    Adapter->>LK: GenerateToken(uid, RoomAdmin=false,\nCanPublish=true, CanPublishVideo=false)
    LK-->>Adapter: identity-specific token
    Adapter-->>API: MediaConnection
    API-->>App: { media_connection }
    App->>Mobile: connect(media_connection)
    Mobile->>LK: room.connect(endpoint, credential) — audio publish enabled ✓

    Note over App,LK: F-003 queue states communicate voluntary order only;
    Note over App,LK: explicit F-005 moderator actions handle exceptional cases
```

> **Security invariant:** `CanPublishVideo` is **always** `false` in MVP. The token generation function enforces this unconditionally — no code path can produce a video-enabled token until `FEATURE_VIDEO_ENABLED=true` and an ADR approves the change.

### 3.3 Go Backend — Token Generation Code Pattern

```go
// Using github.com/livekit/server-sdk-go

func generateLiveKitToken(
    apiKey, apiSecret, roomName, identity, name string,
    isTeacher bool,
    canPublishAudio bool, // for students: true only when teacher grants current turn
) (string, error) {
    
    at := auth.NewAccessToken(apiKey, apiSecret)
    
    grant := &auth.VideoGrant{
        RoomJoin:       true,
        Room:           roomName,
        CanPublish:     canPublishAudio, // microphone publish only in MVP turns
        CanPublishVideo: false,          // MVP hard-stop; enable post-MVP via feature flag
        CanSubscribe:   true,
        CanPublishData: isTeacher, // only teachers send data messages
    }
    
    // Teachers get additional permissions
    if isTeacher {
        grant.RoomAdmin = true      // can mute others, remove participants
    }
    
    at.AddGrant(grant).
        SetIdentity(identity).  // userID
        SetName(name).          // display name
        SetValidFor(1 * time.Hour)
    
    return at.ToJWT()
}
```

MVP participant credentials are valid for at most one hour, independently of the
four-hour session maximum, and never beyond the usable session lifecycle. The
adapter derives `MediaConnection.expires_at` from the actual signed credential.
Before or after expiry, Flutter obtains a fresh connection only through an
authenticated, authorized start/join call; session end, removal, or revoked
membership remains terminal.

### 3.4 Flutter — Room Connection Pattern

```dart
// Using livekit_client Flutter package

// This adapter is the only mobile file that imports LiveKit SDK types.
final class LiveKitMediaSession implements MediaSession {
  Room? _room; // Private provider state; never exposed to controllers or UI.

  @override
  Future<void> connect(MediaConnection connection) async {
    final room = Room();
    final roomOptions = RoomOptions(
      defaultAudioPublishOptions: const AudioPublishOptions(
        name: 'recitation',
        audioBitrate: 48000,
      ),
      adaptiveStream: true,
    );

    await room.connect(
      connection.endpoint,
      connection.credential,
      roomOptions: roomOptions,
    );
    _room = room;
    // Map LiveKit events into provider-neutral MediaSession state here.
  }
}
```

Controllers and UI consume only `MediaSession` state. They retry provider-level
reconnect only while the current credential is usable. Near or after expiry they
call the authenticated, idempotent start/join REST operation for a fresh
`MediaConnection`; credentials stay in memory and are never written to storage.

### 3.5 Audio Configuration (Critical)

```dart
// Disable noise suppression and auto-gain for Quran recitation
// Must be set before connecting to the room

await Hardware.instance.setPreferSpeakerOutput(true);

// Platform-level audio processing
final audioConstraints = {
  'echoCancellation': false,   // OFF where the platform permits — preserves natural voice
  'noiseSuppression': false,   // OFF — preserves tajweed phonetics
  'autoGainControl': false,    // OFF — consistent recitation volume
};
```

### 3.6 Network Requirements

Halaqaty sessions are hosted on **Hetzner Nuremberg (EU)**. Target audience is primarily MENA region (Saudi Arabia, UAE, Egypt).

| Route | Approximate RTT | Assessment |
|-------|----------------|------------|
| Riyadh → Hetzner Nuremberg | ~60 ms | ✅ Acceptable for real-time audio |
| Cairo → Hetzner Nuremberg | ~55 ms | ✅ Acceptable |
| Dubai → Hetzner Nuremberg | ~65 ms | ✅ Acceptable |
| Jakarta → Hetzner Nuremberg | ~160 ms | ⚠️ Degraded; acceptable for V1, monitor |

**Bandwidth requirements per session:**
- Audio only (Opus 48 kbps per participant): ~50 kbps upstream per student
- 30-student circle: ~1.5 Mbps total downstream at the server
- LiveKit SFU handles fan-out; individual student only sends ~50 kbps and receives the teacher stream (~50 kbps)

**Decision:** Stay with Hetzner Nuremberg for Phase 1. 60 ms RTT is within LiveKit's acceptable threshold for voice (< 150 ms). Revisit geographic expansion (Hetzner Ashburn or Singapore) at 500+ concurrent users.

> **⚠️ Investigation pending — server location not yet validated:** No systematic latency benchmarking or region testing has been performed. The 60 ms figure above is an estimate from public ping data, not from actual Hetzner load tests. If pilot teachers in Egypt or Gulf report noticeable audio lag, the first corrective action is to evaluate alternative Hetzner regions (e.g., Helsinki, Singapore) or other providers (e.g., Fly.io, Railway, Vultr) for lower-latency MENA routing. This is a known open item for Phase 1 evaluation.

### 3.7 LiveKit SFU Audio Routing

LiveKit uses a **Selective Forwarding Unit (SFU)** architecture. Each participant uploads one audio stream; LiveKit fans it out to all others without mixing or re-encoding. This is why it scales beyond 4 participants (the ceiling for P2P WebRTC).

```mermaid
graph TD
    subgraph Reciter["🎙️ Current Reciter"]
        R["Active Student\n1× upload — 48kbps Opus"]
    end

    subgraph Teacher["👨‍🏫 Teacher"]
        T["Teacher\n1× upload + receives all"]
    end

    subgraph SFU["⚡ LiveKit SFU"]
        FWD["Selective Forwarding Unit\nno mixing · no re-encoding\n1 stream in → N copies out"]
    end

    subgraph Listeners["👨‍🎓 Listening Students"]
        S1["Student 1\n🎧 receive only"]
        S2["Student 2\n🎧 receive only"]
        SN["Student N...\n🎧 receive only"]
    end

    R -->|"48kbps"| SFU
    T -->|"48kbps"| SFU
    SFU -->|"48kbps"| S1
    SFU -->|"48kbps"| S2
    SFU -->|"48kbps"| SN
    SFU -->|"48kbps"| T
    SFU -->|"48kbps"| R

    note1["❌ P2P WebRTC: N² connections\n30 students = 870 streams\nimpractical above 4 participants"]
    note2["✅ SFU: each client sends 1 stream\nregardless of audience size\n30 students = 31 upstream streams total"]
```

> **Migration tool:** [golang-migrate v4](https://github.com/golang-migrate/migrate) — sequential SQL files in `backend/migrations/`. See [ADR-006](adr/ADR-006-db-migrations.md) for rationale.

## 4. Database Schema

### 4.0 Domain Enumerations

These are the canonical enum values used in PostgreSQL CHECK constraints and Go backend constants. Product-level human labels are in [FEATURES.md F-003](../../management/product/FEATURES.md#f-003-recitation-queue-system).

#### Recitation Grade (`grade` column)

> **Canonical 5-grade scale — locked 2026-06-30.** Replaces the previous 4-grade scale. See [ADR-013](adr/ADR-013-recitation-grade-scale.md); product labels are in [FEATURES.md F-003](../../management/product/FEATURES.md#f-003-recitation-queue-system).

| DB Value | English Label | Arabic Label | Meaning |
|----------|--------------|--------------|---------|
| `excellent` | Excellent | ممتاز | Perfect recitation, excellent tajweed |
| `good` | Good | جيد | Minor errors, good tajweed |
| `acceptable` | Acceptable | مقبول | Notable errors, basic tajweed |
| `needs_review` | Needs Review | يحتاج مراجعة | Significant errors; review required before advancing |
| `repeat` | Repeat | إعادة | Teacher indicates that a full repeat is needed; the value does not block manager controls |

Used in: `recitation_queue_entries.grade`, `memorization_progress.grade`

> **Migration note:** The current implemented schema does not yet contain these grade columns. The F-003/F-007 migrations must introduce the canonical constraint directly; no legacy-value migration is currently required.

#### Queue Entry State Machine

```mermaid
stateDiagram-v2
    direction LR
    [*] --> waiting : Student added to round

    waiting --> reciting : Teacher records student's current turn
    reciting --> completed : Grade submitted
    waiting --> skipped : Manager skips student
    reciting --> skipped : Manager skips student
    waiting --> opted_out : Student opts out\n(per session approval policy)
    reciting --> opted_out : Student opts out

    completed --> [*]
    skipped --> [*]
    opted_out --> [*]

    note right of reciting : Only ONE entry\ncan be 'reciting'\nper round at a time
    note right of completed : grade stored:\nexcellent · good\nacceptable · needs_review · repeat
```

#### Session Lifecycle State Machine

```mermaid
stateDiagram-v2
    direction LR
    [*] --> scheduled : POST /circles/{id}/sessions\n(teacher or supervisor; ad-hoc F-005 only)

    scheduled --> active : POST /sessions/{id}/start\n(teacher or supervisor)\nMedia room ready via adapter\nWS → session.started broadcast

    active --> ended : POST /sessions/{id}/end\n(teacher or supervisor, or automatic timeout)\nMedia room closed via adapter\nWS → session.ended broadcast

    ended --> [*]

    note right of scheduled : Session stored in DB\nNo ready media room yet\nQueue rounds not yet possible
    note right of active : Queue rounds active\nWS events flowing\nAttendance auto-tracked
```

### 4.1 Entity-Relationship Diagram

```mermaid
erDiagram
    users {
        uuid id PK
        varchar firebase_uid UK
        varchar display_name
        varchar email UK
        varchar timezone
        text avatar_url
        varchar preferred_lang
        timestamptz created_at
        timestamptz updated_at
    }
    user_sessions {
        uuid id PK
        uuid user_id FK
        varchar device_name
        timestamptz last_activity_at
        timestamptz expires_at
        timestamptz revoked_at
        timestamptz created_at
    }
    circles {
        uuid id PK
        varchar name
        uuid teacher_id FK
        varchar invite_code UK
        int max_capacity
        bool is_private
        varchar gender_restriction
        varchar grading_policy
        bool is_archived
        timestamptz created_at
    }
    circle_members {
        uuid id PK
        uuid circle_id FK
        uuid user_id FK
        varchar role
        timestamptz joined_at
    }
    sessions {
        uuid id PK
        uuid circle_id FK
        uuid created_by FK
        text notes
        timestamptz scheduled_at
        timestamptz actual_start
        timestamptz actual_end
        varchar end_reason
        varchar status
        varchar media_mode
        varchar media_room_ref UK
        boolean is_locked
        int participant_count
        timestamptz created_at
    }
    session_participant_presence {
        uuid id PK
        uuid session_id FK
        uuid user_id FK
        timestamptz first_joined_at
        timestamptz last_joined_at
        timestamptz last_left_at
        int reconnect_count
        boolean is_currently_present
        timestamptz removed_at
        timestamptz hand_raised_at
    }
    recitation_queue {
        uuid id PK
        uuid session_id FK
        int round_number
        varchar round_type
        int surah_id FK
        int from_ayah
        int to_ayah
        bool grading_required
        bool is_active
        timestamptz created_at
    }
    recitation_queue_entries {
        uuid id PK
        uuid queue_id FK
        uuid student_id FK
        int position
        varchar status
        varchar grade
        text teacher_notes
        timestamptz started_at
        timestamptz completed_at
    }
    messages {
        uuid id PK
        uuid circle_id FK
        uuid sender_id FK
        text content
        varchar message_type
        text media_url
        uuid reply_to_id FK
        bool is_pinned
        timestamptz sent_at
        timestamptz deleted_at
    }
    memorization_progress {
        uuid id PK
        uuid student_id FK
        uuid circle_id FK
        uuid session_id FK
        uuid queue_entry_id FK-UK
        int surah_id FK
        int from_ayah
        int to_ayah
        varchar type
        varchar grade
        varchar notes
        date date
        timestamptz created_at
        timestamptz updated_at
    }
    quran_divisions {
        int id PK
        int surah_id FK
        int from_ayah
        int to_ayah
        int juz_number
        int hizb_number
        int rub_number
    }
    schedules {
        uuid id PK
        uuid circle_id FK
        int day_of_week
        time start_time
        time end_time
        varchar timezone
        bool is_active
    }
    quran_surahs {
        int id PK
        varchar name_arabic
        varchar name_transliterated
        int ayah_count
        int juz_start
    }
    device_tokens {
        uuid id PK
        uuid user_id FK
        text token UK
        varchar platform
        timestamptz last_seen_at
    }
    notifications {
        uuid id PK
        uuid user_id FK
        varchar type
        varchar title
        bool is_read
        timestamptz created_at
    }

    users ||--o{ circle_members : "joins circles"
    circles ||--o{ circle_members : "has members"
    users ||--o{ circles : "teaches (teacher_id)"
    circles ||--o{ sessions : "hosts"
    circles ||--o{ messages : "has messages"
    circles ||--o{ schedules : "has schedules"
    sessions ||--o{ session_participant_presence : "tracks live presence"
    sessions ||--o{ recitation_queue : "has rounds"
    recitation_queue ||--o{ recitation_queue_entries : "has entries"
    users ||--o{ recitation_queue_entries : "is student"
    users ||--o{ memorization_progress : "records"
    sessions ||--o{ memorization_progress : "sources"
    recitation_queue_entries ||--|| memorization_progress : "generates (1:1)"
    quran_surahs ||--o{ memorization_progress : "referenced by"
    quran_surahs ||--o{ quran_divisions : "divided into"
    users ||--o{ device_tokens : "registers"
    users ||--o{ user_sessions : "has"
    users ||--o{ notifications : "receives"
    quran_surahs ||--o{ recitation_queue : "referenced by"
```

### 4.2 Table Definitions

#### `users`
| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK, DEFAULT gen_random_uuid() | Unique user identifier |
| firebase_uid | VARCHAR(128) | UNIQUE NOT NULL | Firebase Auth UID |
| display_name | VARCHAR(100) | NOT NULL | User's chosen display name |
| email | VARCHAR(255) | UNIQUE | Email (nullable for Apple relay used) |
| phone | VARCHAR(20) | UNIQUE | Phone number with country code |
| timezone | VARCHAR(50) | NOT NULL DEFAULT 'UTC' | IANA timezone string (e.g., Asia/Riyadh) |
| avatar_url | TEXT | | MinIO object URL |
| preferred_lang | VARCHAR(10) | NOT NULL DEFAULT 'ar' | ISO 639-1 language code |
| created_at | TIMESTAMPTZ | NOT NULL DEFAULT NOW() | |
| updated_at | TIMESTAMPTZ | NOT NULL DEFAULT NOW() | |

#### `device_tokens`
| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK, DEFAULT gen_random_uuid() | |
| user_id | UUID | FK → users.id NOT NULL ON DELETE CASCADE | Token owner |
| token | TEXT | NOT NULL | FCM device token |
| platform | VARCHAR(10) | CHECK IN ('ios','android','web') NOT NULL | Device platform |
| device_name | VARCHAR(100) | | Optional human-readable label |
| created_at | TIMESTAMPTZ | DEFAULT NOW() | |
| last_seen_at | TIMESTAMPTZ | DEFAULT NOW() | Updated on each successful FCM delivery |
| UNIQUE | (user_id, token) | | One entry per device per user |

#### `user_sessions`
| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK, server-generated | Opaque current-device session identifier; supplied as `X-Halaqaty-Session-ID` |
| user_id | UUID | FK → users.id NOT NULL ON DELETE CASCADE | Session owner |
| device_name | VARCHAR(100) | NULL | Optional user-visible device label; not a credential |
| last_activity_at | TIMESTAMPTZ | NOT NULL | Updated by authenticated requests; used for the 30-day inactivity rule |
| expires_at | TIMESTAMPTZ | NOT NULL | Absolute backend-session expiry |
| revoked_at | TIMESTAMPTZ | NULL | Set on current-device logout or account revocation |
| created_at | TIMESTAMPTZ | NOT NULL DEFAULT NOW() | |

> Firebase refresh tokens are never stored in this table. Firebase owns their rotation and reuse detection.

#### `circles`
| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK | |
| name | VARCHAR(100) | NOT NULL | Circle display name |
| description | TEXT | | Circle description |
| rules | TEXT | | Circle rules/guidelines |
| teacher_id | UUID | FK → users.id NOT NULL | Circle owner |
| invite_code | VARCHAR(20) | UNIQUE NOT NULL | Join code (e.g., HLQ-7X2K) |
| max_capacity | INTEGER | DEFAULT 50 | Maximum student capacity (min 2, max 200) |
| is_private | BOOLEAN | DEFAULT FALSE | Whether circle requires invite to join |
| gender_restriction | VARCHAR(20) | CHECK IN ('male','female','mixed','unspecified') DEFAULT 'unspecified' | Student-audience setting; independent of teacher gender |
| language | VARCHAR(10) | DEFAULT 'ar' | Primary language |
| grading_policy | VARCHAR(20) | CHECK IN ('required','optional') DEFAULT 'required' | Whether grading is required after each completed turn |
| is_archived | BOOLEAN | DEFAULT FALSE | |
| created_at | TIMESTAMPTZ | DEFAULT NOW() | |

#### `circle_members`
| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK | |
| circle_id | UUID | FK → circles.id NOT NULL | |
| user_id | UUID | FK → users.id NOT NULL | |
| role | VARCHAR(20) | CHECK IN ('teacher','student','supervisor') | Per-circle role |
| joined_at | TIMESTAMPTZ | DEFAULT NOW() | |
| UNIQUE | (circle_id, user_id) | | One membership per user per circle |

#### `sessions`

F-005 owns the complete base `sessions` table and its lifecycle/media invariants.
The F-005 migration creates the circle/creator keys, `scheduled_at` boundary,
`scheduled → active → ended` status, `actual_start`/`actual_end`, audio-only
`media_mode`, unique opaque `media_room_ref`, `is_locked`, `end_reason`,
`participant_count`, and audit timestamps. F-003 and F-006 extend this table
only through later paired migrations for queue and attendance concerns.

F-003 extends the session with the closed queue-policy values defined by
[ADR-018](adr/ADR-018-configurable-session-queue-policy.md) using five closed
policy columns and a monotonic policy version, as detailed in the F-003 table
section below. It does not alter F-005 lifecycle or make session end depend on
queue cleanup.
| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK | |
| circle_id | UUID | FK → circles.id NOT NULL | |
| notes | TEXT | maxLength 500 | Optional session notes (visible to members) |
| scheduled_at | TIMESTAMPTZ | | Planned start time; NULL for ad-hoc sessions |
| actual_start | TIMESTAMPTZ | | When teacher actually started |
| actual_end | TIMESTAMPTZ | | When session actually ended |
| end_reason | VARCHAR(20) | CHECK IN ('manual','duration_limit','idle_timeout') NULL | Why the session ended; NULL until ended |
| status | VARCHAR(20) | CHECK IN ('scheduled','active','ended') | |
| media_mode | VARCHAR(20) | CHECK IN ('audio_only','audio_video'), DEFAULT 'audio_only' | Session media policy (MVP always audio_only) |
| media_room_ref | VARCHAR(200) | UNIQUE | Opaque media-adapter room reference; LiveKit-backed in MVP |
| is_locked | BOOLEAN | NOT NULL DEFAULT false | Prevents new joins while true |
| created_by | UUID | FK → users.id NOT NULL | Teacher who created the session |
| participant_count | INTEGER | DEFAULT 0 | Running count updated on join/leave |
| created_at | TIMESTAMPTZ | DEFAULT NOW() | |

#### `session_participant_presence`
| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK | |
| session_id | UUID | FK → sessions.id NOT NULL | |
| user_id | UUID | FK → users.id NOT NULL | |
| first_joined_at | TIMESTAMPTZ | NULL | First observed authorized room join |
| last_joined_at | TIMESTAMPTZ | NULL | Most recent authorized room join or reconnect |
| last_left_at | TIMESTAMPTZ | NULL | Most recent room leave |
| reconnect_count | INTEGER | NOT NULL DEFAULT 0 | Reconnects after the first join |
| is_currently_present | BOOLEAN | NOT NULL DEFAULT false | Authoritative current-presence state |
| removed_at | TIMESTAMPTZ | NULL | Set when moderation removes the participant |
| hand_raised_at | TIMESTAMPTZ | NULL | Current standalone hand state; NULL when lowered |
| UNIQUE | (session_id, user_id) | | |

#### `session_attendance` (F-006)

F-006 owns attendance classification and manual overrides. It derives its policy
from F-005 participant-presence facts and must not alter them.

#### `recitation_queue`
| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK | |
| session_id | UUID | FK → sessions.id NOT NULL | |
| round_number | INTEGER | NOT NULL | Round 1, 2, 3... |
| round_type | VARCHAR(30) | CHECK IN ('new_memorization','revision','old_revision','test') | |
| surah_id | INTEGER | FK → quran_surahs.id NOT NULL | Surah number (1–114); name derived via JOIN |
| from_ayah | INTEGER | NOT NULL | Starting Ayah number (validated ≥ 1) |
| to_ayah | INTEGER | NOT NULL | Ending Ayah number (validated ≤ quran_surahs.ayah_count) |
| grading_required | BOOLEAN | NOT NULL | Required for this round |
| lifecycle | VARCHAR(16) | CHECK IN ('prepared','active','finalized') | Separate from queue-entry state |
| selected_entry_id | UUID | FK → recitation_queue_entries.id NULL | Manager selection; does not start recitation |
| version | BIGINT | NOT NULL DEFAULT 1 CHECK > 0 | Optimistic/realtime version |
| created_by | UUID | FK → users.id NOT NULL | Audit attribution |
| created_at | TIMESTAMPTZ | DEFAULT NOW() | |
| activated_at | TIMESTAMPTZ | NULL | |
| finalized_at | TIMESTAMPTZ | NULL | |
| UNIQUE | (session_id, round_number) | | Sequential numbering |
| PARTIAL UNIQUE | (session_id) WHERE lifecycle = 'active' | | One active round |

#### `recitation_queue_preorder`
| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| queue_id | UUID | FK → recitation_queue.id NOT NULL | Prepared round |
| student_id | UUID | FK → users.id NOT NULL | Active student candidate |
| position | INTEGER | NOT NULL CHECK > 0 | Durable pre-set relative order |
| added_by | UUID | FK → users.id NOT NULL | Manager attribution |
| created_at | TIMESTAMPTZ | DEFAULT NOW() | |
| UNIQUE | (queue_id, student_id), (queue_id, position) | | No duplicate candidate/order |

#### `recitation_queue_entries`
| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK | |
| queue_id | UUID | FK → recitation_queue.id NOT NULL | |
| student_id | UUID | FK → users.id NOT NULL | |
| position | INTEGER | NOT NULL | Order in queue (1, 2, 3...) |
| status | VARCHAR(20) | CHECK IN ('waiting','reciting','completed','skipped','opted_out') | |
| grade | VARCHAR(30) | CHECK IN ('excellent','good','acceptable','needs_review','repeat') | Nullable until teacher submits grade |
| teacher_notes | VARCHAR(500) | | Optional teacher note |
| version | BIGINT | NOT NULL DEFAULT 1 CHECK > 0 | Optimistic mutation version |
| started_at | TIMESTAMPTZ | | When recitation began |
| completed_at | TIMESTAMPTZ | | When recitation ended / was graded |
| resolved_by | UUID | FK → users.id NULL | Manager responsible for terminal transition |
| created_at, updated_at | TIMESTAMPTZ | NOT NULL | UTC timestamps |
| UNIQUE | (queue_id, student_id), (queue_id, position) | | One position/student and contiguous order target |
| PARTIAL UNIQUE | (queue_id) WHERE status = 'reciting' | | One active reciter |

#### `queue_opt_out_requests`

Durable request records use `pending`, `approved`, or `declined`; these are
request outcomes, not queue-entry states. Each row references one entry and
stores requester/decider attribution and UTC timestamps. A partial unique index
allows at most one pending request per entry.

#### `queue_command_receipts`

Optional client idempotency keys are stored by `(session_id, actor_id,
idempotency_key)` with a closed command name, resulting resource ID/version, and
timestamp. Receipts never store request bodies, grades, notes, media values, or
response secrets.

#### `queue_event_outbox`

Each committed queue mutation inserts a redacted-metadata event intent containing
a stable event ID, session/round/resource IDs, closed event type, round version,
non-sensitive transition/order facts, retry schedule, and delivery timestamp.
The worker reconstructs visibility-sensitive fields from PostgreSQL; the outbox
stores no grades, notes, names, media values, URLs, credentials, or provider
identifiers.

#### F-003 session queue policy

F-003 adds five CHECK-constrained columns to `sessions`:
`queue_population_policy`, `queue_finalization_policy`,
`queue_opt_out_policy`, `queue_grade_visibility`, and
`queue_grade_correction`, plus positive `queue_policy_version`. Defaults and
allowed values are defined by ADR-018.
Policy changes are prospective, do not rewrite history, and are authorized by
current active teacher/supervisor circle roles. Session creation is audit
attribution rather than a permanent authorization grant.

F-005 end commits independently. F-003 finalizes the round idempotently after
end; a queue failure cannot delay or roll back the
ended session.

#### `messages`
| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK | |
| circle_id | UUID | FK → circles.id | NULL for direct messages |
| CHECK | (circle_id IS NOT NULL OR dm_recipient_id IS NOT NULL) | | At least one must be set |
| dm_recipient_id | UUID | FK → users.id | For direct messages |
| sender_id | UUID | FK → users.id NOT NULL | |
| content | TEXT | | Text content (empty for voice/image/file) |
| message_type | VARCHAR(20) | CHECK IN ('text','voice','image','file') NOT NULL | |
| media_url | TEXT | | MinIO presigned URL (expires 7 days); NULL for text messages |
| file_name | VARCHAR(255) | | Original filename for file/image/voice attachments |
| file_size_bytes | INTEGER | | File size in bytes |
| reply_to_id | UUID | FK → messages.id | Threaded reply target |
| is_pinned | BOOLEAN | DEFAULT FALSE | |
| sent_at | TIMESTAMPTZ | NOT NULL DEFAULT NOW() | Message send timestamp; maps to API field `sent_at` |
| deleted_at | TIMESTAMPTZ | | Soft delete |

#### `message_reads`
| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK | |
| message_id | UUID | FK → messages.id NOT NULL | |
| user_id | UUID | FK → users.id NOT NULL | |
| read_at | TIMESTAMPTZ | DEFAULT NOW() | |
| UNIQUE | (message_id, user_id) | | |

#### `memorization_progress`
| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK | |
| student_id | UUID | FK → users.id NOT NULL | No cascade; progress history survives unless explicitly erased (ADR-019) |
| circle_id | UUID | FK → circles.id NOT NULL | |
| session_id | UUID | FK → sessions.id NOT NULL | Source session; always known — only the completion transaction inserts |
| queue_entry_id | UUID | FK → recitation_queue_entries.id NOT NULL UNIQUE | Source queue entry; NOT NULL UNIQUE enables idempotent re-grade upsert |
| surah_id | INTEGER | FK → quran_surahs.id NOT NULL | Normalized surah reference (replaces deprecated `surah_name`) |
| surah_name | VARCHAR(100) | DEPRECATED | Use `surah_id`; retained until v1.1 for in-flight client compat (F-007-SPEC OQ-032) |
| from_ayah | INTEGER | NOT NULL | Starting Ayah of the recited range |
| to_ayah | INTEGER | NOT NULL | Ending Ayah of the recited range |
| type | VARCHAR(30) | CHECK IN ('new_memorization','revision','old_revision','test') | Completed `test` records remain practice history but do not change Quran-map memorization/revision status |
| grade | VARCHAR(30) | CHECK IN ('excellent','good','acceptable','needs_review','repeat') NULLABLE | NULL when `grading_required = false` on the round |
| notes | VARCHAR(500) | | Teacher notes — same bound as `recitation_queue_entries.teacher_notes` |
| date | DATE | NOT NULL | Session date |
| created_at | TIMESTAMPTZ | DEFAULT NOW() | |
| updated_at | TIMESTAMPTZ | | Set on re-grade |

#### `quran_divisions` *(Reference — Static Seed Data)*
| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | SERIAL | PK | |
| surah_id | INTEGER | FK → quran_surahs.id NOT NULL | |
| from_ayah | INTEGER | NOT NULL CHECK ≥ 1 | Start of this division segment |
| to_ayah | INTEGER | NOT NULL CHECK ≥ from_ayah | End of this division segment |
| juz_number | INTEGER | NOT NULL CHECK 1–30 | Juz containing this segment |
| hizb_number | INTEGER | NOT NULL CHECK 1–60 | Hizb (half-juz) containing this segment |
| rub_number | INTEGER | NOT NULL CHECK 1–240 | Rub' (quarter-hizb) number — 240 total |
| UNIQUE | (surah_id, from_ayah) | | |

> **Usage:** Pre-populated with all 240 Medina Mushaf Rub' boundaries. Never modified by the application. Enables teacher grading UI to offer a Rub'/Hizb/Juz picker and powers Surah coverage segment bars for long Surahs (e.g., Al-Baqarah spans 3 Juz and 6 Hizb).

#### `notifications`
| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK | |
| user_id | UUID | FK → users.id NOT NULL | Recipient |
| type | VARCHAR(50) | NOT NULL | e.g., 'session_reminder', 'queue_turn' |
| title | VARCHAR(200) | NOT NULL | Notification title |
| body | TEXT | NOT NULL | Notification body |
| data_json | JSONB | | Extra data (session_id, etc.) |
| is_read | BOOLEAN | DEFAULT FALSE | |
| created_at | TIMESTAMPTZ | DEFAULT NOW() | |

#### `schedules`
| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK | |
| circle_id | UUID | FK → circles.id NOT NULL | |
| day_of_week | INTEGER | CHECK BETWEEN 0 AND 6 | 0=Sunday, 6=Saturday |
| start_time | TIME | NOT NULL | Stored in local time; use timezone column to convert to UTC |
| end_time | TIME | NOT NULL | Stored in local time; use timezone column to convert to UTC |
| timezone | VARCHAR(50) | NOT NULL | IANA timezone string; used to interpret start_time/end_time as local and convert to UTC |
| is_active | BOOLEAN | DEFAULT TRUE | |
| created_at | TIMESTAMPTZ | DEFAULT NOW() | |

#### `parent_links`
Removed from MVP scope. The product currently supports direct student and teacher accounts without parent-linked account management.

#### `institutions` *(Future)*
| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK | |
| name | VARCHAR(200) | NOT NULL | Institution name |
| description | TEXT | | |
| admin_user_id | UUID | FK → users.id NOT NULL | Primary admin |
| logo_url | TEXT | | MinIO URL |
| is_verified | BOOLEAN | DEFAULT FALSE | Admin approval |
| created_at | TIMESTAMPTZ | DEFAULT NOW() | |

#### `institution_members` *(Future)*
| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK | |
| institution_id | UUID | FK → institutions.id | |
| user_id | UUID | FK → users.id | |
| role | VARCHAR(20) | CHECK IN ('admin','teacher','student') | |
| created_at | TIMESTAMPTZ | DEFAULT NOW() | |

#### `quran_surahs` *(Reference — Static Seed Data)*
| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | INTEGER | PK | Surah number 1–114 |
| name_arabic | VARCHAR(100) | NOT NULL | Arabic name (e.g., البقرة) |
| name_transliterated | VARCHAR(100) | NOT NULL | Latin transliteration (e.g., Al-Baqarah) |
| ayah_count | INTEGER | NOT NULL | Total Ayahs in this Surah |
| juz_start | INTEGER | NOT NULL | Juz number where this Surah begins (1–30) |
| revelation_type | VARCHAR(10) | CHECK IN ('meccan','medinan') | |

> **Usage:** Pre-populated by seed migration (all 114 Surahs). Never modified by the application. Ayah range validation in queue API uses this table: `from_ayah >= 1 AND to_ayah <= surah.ayah_count AND from_ayah <= to_ayah`. Invalid ranges return HTTP 422.

---

## 5. API Endpoint Planning

> **Legend:** ✅ In `docs/contracts/openapi.yaml` (contracted, MVP) · 🔲 Post-MVP (not yet in contract — must be added to `openapi.yaml` via spec before implementation per Constitution §III)

### Base URL: `https://api.halaqaty.app/api/v1`

### Authentication: Firebase identity plus backend device session

Flutter Firebase Auth owns email/password validation, identity creation, sign-in, and
Firebase ID-token refresh. The Go backend never receives passwords and never issues
Firebase ID or refresh tokens. `POST /auth/register` provisions a just-created Firebase
identity locally; `POST /auth/sessions` creates a backend session after Firebase sign-in.
Both require `Authorization: Bearer <firebase-jwt>`. All other protected routes require
that bearer token plus `X-Halaqaty-Session-ID`, an opaque backend-issued current-device
session identifier. A revoked or inactive session is rejected even while its Firebase ID
token remains valid.

---

### `/auth`
| Method | Path | Status | Description |
|--------|------|--------|-------------|
| POST | `/auth/register` | ✅ | Create user profile after Firebase registration |
| POST | `/auth/sessions` | ✅ | Create backend session after Firebase sign-in |
| POST | `/auth/logout` | ✅ | Revoke the current backend device session |
| POST | `/auth/fcm-token` | ✅ | Register or update a device FCM token (upsert by user_id + token) |
| GET | `/auth/me` | ✅ | Get current user profile |
| PUT | `/auth/me` | ✅ | Update profile (name, avatar, language) |
| DELETE | `/auth/me` | ✅ | Delete account and all data |

### `/circles`
| Method | Path | Status | Description |
|--------|------|--------|-------------|
| GET | `/circles` | ✅ | List my circles (teacher + student) |
| POST | `/circles` | ✅ | Create a new circle |
| GET | `/circles/{id}` | ✅ | Get circle details |
| PUT | `/circles/{id}` | ✅ | Update circle settings (teacher only) |
| DELETE | `/circles/{id}` | ✅ | Archive circle (teacher only) |
| POST | `/circles/join` | ✅ | Join a circle by invite code `{ invite_code }` |
| GET | `/circles/{id}/members` | ✅ | List members with roles |
| DELETE | `/circles/{id}/members/{userId}` | ✅ | Remove member from circle (teacher only) |
| POST | `/circles/{id}/leave` | 🔲 | Leave a circle |
| PUT | `/circles/{id}/members/{userId}/role` | ✅ | Teacher or supervisor changes another member's circle role |
| POST | `/circles/{id}/invite-code/refresh` | 🔲 | Generate new invite code |

### `/sessions`
| Method | Path | Status | Description |
|--------|------|--------|-------------|
| GET | `/circles/{id}/sessions` | ✅ | List sessions for a circle |
| POST | `/circles/{id}/sessions` | ✅ | Create an ad-hoc F-005 session (teacher or supervisor) |
| POST | `/sessions/{id}/start` | ✅ | Teacher or supervisor starts session and receives their participant-specific media connection |
| POST | `/sessions/{id}/join` | ✅ | Join an active session and receive the caller's media connection (members only) |
| POST | `/sessions/{id}/end` | ✅ | Teacher or supervisor ends session; duration/idle endings are system-attributed |
| GET | `/sessions/{id}` | 🔲 | Get session details |
| POST | `/sessions/{id}/participants/{userId}/mute` | 🔲 | Mute a participant |
| POST | `/sessions/{id}/participants/{userId}/unmute` | 🔲 | Unmute a participant |
| POST | `/sessions/{id}/participants/{userId}/remove` | 🔲 | Remove participant from session |
| POST | `/sessions/{id}/lock` | 🔲 | Lock session (no new joiners) |

### `/realtime`
| Method | Path | Status | Description |
|--------|------|--------|-------------|
| POST | `/realtime/tickets` | 🔲 | Issue a short-lived authenticated ticket for authorized circle and session topics |

### `/sessions/{id}/attendance`
| Method | Path | Status | Description |
|--------|------|--------|-------------|
| GET | `/sessions/{id}/attendance` | 🔲 | List attendance for a session |
| PUT | `/sessions/{id}/attendance/{userId}` | 🔲 | Manual attendance override |

### `/queue`
| Method | Path | Status | Description |
|--------|------|--------|-------------|
| GET | `/sessions/{id}/queue` | ✅ | Get current queue state |
| POST | `/sessions/{id}/queue/rounds` | ✅ | Prepare or activate a round |
| POST | `/sessions/{id}/queue/reset` | ✅ | Finalize current history and create next round |
| POST | `/sessions/{id}/queue/advance` | ✅ | Select next waiting entry without starting it |
| PUT | `/sessions/{id}/queue/entries/{entryId}/status` | ✅ | Start, skip, or atomically complete |
| PUT | `/sessions/{id}/queue/order` | ✅ | Reorder waiting entries/pre-set candidates |
| POST | `/sessions/{id}/queue/entries/{entryId}/grade` | ✅ | Audited completed-entry correction |
| POST | `/sessions/{id}/queue/opt-out` | ✅ | Request or auto-approve student opt-out |
| POST | `/sessions/{id}/queue/opt-out-requests/{requestId}/decision` | ✅ | Approve/decline pending opt-out |
| PATCH | `/sessions/{id}/queue/policy` | ✅ | Change closed policy values prospectively |

Late-join append is driven by the committed F-005 participant join fact; F-003
does not expose separate add/delete entry controls.

### `/messages`
| Method | Path | Status | Description |
|--------|------|--------|-------------|
| GET | `/circles/{id}/messages` | ✅ | List circle messages (paginated) |
| POST | `/circles/{id}/messages` | ✅ | Send a message |
| DELETE | `/circles/{id}/messages/{msgId}` | ✅ | Delete a message |
| POST | `/circles/{id}/messages/{msgId}/pin` | 🔲 | Pin a message |
| DELETE | `/circles/{id}/messages/{msgId}/pin` | 🔲 | Unpin a message |
| POST | `/circles/{id}/messages/{msgId}/read` | 🔲 | Mark message as read |
| GET | `/dm/{userId}` | 🔲 | List DM conversation with a user |
| POST | `/dm/{userId}` | 🔲 | Send a direct message |

### `/progress`
| Method | Path | Status | Description |
|--------|------|--------|-------------|
| GET | `/students/me/circles/history` | 🔲 | My circle history — attendance + practice counts per circle |
| GET | `/students/me/progress` | 🔲 | My global Quran map — all 114 Surahs with memorization status (cross-circle) |
| GET | `/students/me/sessions/history` | 🔲 | My session timeline — attended vs practiced per session |
| GET | `/students/me/progress/stats` | 🔲 | My analytics — Ayahs/week, attendance %, practice % |
| GET | `/circles/{id}/progress` | 🔲 | All students' progress summary in a circle (teacher/supervisor only) |
| GET | `/circles/{id}/progress/{userId}` | 🔲 | One student's full profile incl. cross-circle Quran map (teacher only) |
| GET | `/circles/{id}/surah-insights` | 🔲 | Surahs ranked by weak grade frequency — last 30 days (teacher only) |

> **Note:** `POST /circles/{id}/progress` (manual student self-logging) was explicitly decided out of scope (OQ-020). All progress records are auto-generated from session-based recitations only.

### `/schedule`
| Method | Path | Status | Description |
|--------|------|--------|-------------|
| GET | `/circles/{id}/schedules` | ✅ | List schedules for a circle |
| POST | `/circles/{id}/schedules` | ✅ | Create a schedule (teacher only) |
| PUT | `/circles/{id}/schedules/{schedId}` | 🔲 | Update a schedule |
| DELETE | `/circles/{id}/schedules/{schedId}` | 🔲 | Delete a schedule |
| GET | `/schedule/me` | 🔲 | My unified calendar across all circles |

### `/config`
| Method | Path | Status | Description |
|--------|------|--------|-------------|
| GET | `/config/features` | ✅ | Get feature flags for the authenticated user |

### `/uploads`
| Method | Path | Status | Description |
|--------|------|--------|-------------|
| POST | `/uploads/avatar` | ✅ | Upload user avatar → returns presigned MinIO URL |
| POST | `/uploads/voice` | ✅ | Upload voice message → returns MinIO object key |
| POST | `/uploads/image` | ✅ | Upload image attachment → returns MinIO object key |
| POST | `/uploads/file` | ✅ | Upload file attachment (PDF) → returns MinIO object key |

### `/notifications`
| Method | Path | Status | Description |
|--------|------|--------|-------------|
| GET | `/notifications` | 🔲 | List my notifications (paginated) |
| PUT | `/notifications/{id}/read` | 🔲 | Mark notification as read |
| PUT | `/notifications/read-all` | 🔲 | Mark all as read |
| GET | `/notifications/preferences` | 🔲 | Get notification preferences |
| PUT | `/notifications/preferences` | 🔲 | Update preferences |

---

## 6. Security Considerations

### 6.1 Authentication & Authorization

- **Identity:** Firebase Auth issues JWTs; our Go backend validates them on every request
- **Authorization:** Role-based per circle. After JWT validation, Go backend checks `circle_members` table for user's role in the requested circle
- **Firebase token lifecycle:** Firebase ID tokens expire after 1 hour; the Flutter Firebase SDK refreshes them silently. Firebase owns refresh-token rotation and reuse detection.
- **Backend session lifecycle:** The backend creates one opaque session per device after a verified Firebase sign-in. Every protected request includes its session ID; session activity extends the 30-day inactivity window. Current-device logout revokes only that session. A future logout-all-devices operation must revoke every session for the authenticated user. Backend sessions do not mint access or refresh tokens.
- **Media credentials:** Generated exclusively by the Go backend through the LiveKit MVP adapter; never by the Flutter client, persisted client-side, logged, or broadcast. Authorized students receive non-admin audio-publish scope; video remains disabled. F-003 does not alter that scope.
- **Circle role lifecycle:** Roles are stored only in `circle_members`. On creation, the creator may assign existing registered users as one or more teachers and one optional backup supervisor; if no teacher is selected, the creator becomes teacher, otherwise the creator is supervisor. Invite acceptance creates `student`. Any teacher or supervisor may change another member's role among `student`, `supervisor`, and `teacher`, but cannot change their own role or leave the circle without a teacher. See ADR-010.

#### Circle permission matrix

All rows below require `Authorization: Bearer <firebase-jwt>` and `X-Halaqaty-Session-ID`, except `/auth/register` and `/auth/sessions`, which require only the Firebase bearer token.

| Action | Required role | Expected rejection |
|--------|---------------|--------------------|
| List/read own circles and joined circle details | active member | `401` invalid credentials, `403` non-member, `404` missing circle |
| Create circle | authenticated user | `401` invalid credentials, `400` invalid role assignment input |
| Join by invite | authenticated user | `401` invalid credentials, `404` invalid invite, `409` already member |
| Update circle settings, archive circle, remove member | teacher | `401` invalid credentials, `403` non-teacher, `404` missing circle/member |
| Create/start/end session, create schedules | teacher or supervisor for F-005 ad-hoc lifecycle; teacher for scheduling | `401` invalid credentials, `403` unauthorized role, `404` missing circle/session |
| Join live session, list members, read queue/chat | active member | `401` invalid credentials, `403` non-member, `404` missing resource |
| Grade recitation | teacher or supervisor | `401` invalid credentials, `403` student/non-member, `404` missing queue entry |
| Change another member role | teacher or supervisor | `401` invalid credentials, `403` self-change/final-teacher/student/non-member/cross-circle, `404` missing member |

### 6.2 Authentication & Authorization Flow

```mermaid
sequenceDiagram
    participant U as User
    participant App as Flutter App
    participant FB as Firebase Auth
    participant API as Go Backend
    participant DB as PostgreSQL circle_members

    Note over U,DB: Phase 1 — Identity (Authentication)
    U->>App: Sign in (Google / Apple / Email)
    App->>FB: signInWith(provider)
    FB-->>App: Firebase ID Token (JWT, 1hr expiry)
    App->>API: POST /auth/register { display_name }\n[Authorization: Bearer firebase_jwt]
    API->>FB: VerifyIDToken(jwt) → uid ✓
    API->>DB: INSERT users + user_sessions (firebase_uid, display_name, ...)
    API-->>App: { user, session_id, expires_at }

    Note over U,DB: Phase 2 — Authorization (per-circle role check)
    App->>API: GET /circles/{id}\n[Authorization: Bearer firebase_jwt; X-Halaqaty-Session-ID]
    API->>FB: VerifyIDToken(jwt) → uid ✓
    API->>DB: SELECT role FROM circle_members\nWHERE circle_id=$1 AND user_id=$2
    DB-->>API: role = "teacher"
    API-->>App: 200 Circle { ... }

    Note over API,DB: ⚠️ No Firebase custom claims for authz.\nAll roles are per-circle from circle_members.\nRole changes take effect immediately — no cache delay.
```

### 6.3 Media Room Security (MVP LiveKit)

- Each session generates a stable opaque, non-guessable `media_room_ref` mapped to a LiveKit room by the MVP adapter
- Room names are not publicly guessable
- Each participant needs a JWT from Go backend to join — no anonymous access
- Teacher's JWT includes `RoomAdmin: true` (can mute, remove)
- Student's JWT includes `CanPublish: true`, `CanPublishVideo: false`, and never includes `RoomAdmin`
- F-003 records voluntary recitation order only; teachers/supervisors retain explicit F-005 mute, mute-all, remove, lock, and end controls
- Room is deleted from LiveKit server when session ends

### Rate Limiting

- REST API: rate limited by IP and by user ID
- WebSocket: connections limited per user (max 3 active connections per user)
- Message sending: max 30 messages per minute per user per circle
- File uploads: max 10 uploads per hour per user

### 6.4.1 Database Indexing Strategy

**Canonical indexing policy:**

1. **Foreign key indexes (not automatic):** Create indexes on FK columns when they’re used in joins/filters or to avoid FK-related lock contention (PostgreSQL does not add these indexes for you).
2. **Search & filtering:** Columns in WHERE clauses must be indexed (e.g., `circle_id`, `user_id`, `session_id`, `surah_id`).
3. **Sorting:** Columns in ORDER BY clauses should have indexes (e.g., `created_at`, `sent_at`).
4. **Partial indexes:** Use for soft-deletes and status filters (e.g., WHERE `deleted_at IS NULL`).

**Index naming convention:**
```
idx_<table>_<column>                    -- simple
idx_<table>_<col1>_<col2>               -- composite
idx_<table>_<col>_partial_<condition>   -- partial (e.g., idx_messages_circle_id_partial_not_deleted)
```

**Index review checklist (before merge):**
- [ ] Query uses indexed columns in WHERE/JOIN/ORDER BY
- [ ] Composite indexes follow query predicate order
- [ ] Partial indexes used for soft-deletes and status filters
- [ ] No redundant indexes (e.g., don't index both `col` and `(col, col2)` unless both are used)
- [ ] Index size estimated (large indexes slow writes)

**Periodic audit:** Run `EXPLAIN ANALYZE` on top 10 queries monthly and recommend new indexes.

### 6.4.2 Column Versioning (`updated_at` Coverage)

**Policy:** All tables containing user-modifiable data should have an `updated_at TIMESTAMPTZ DEFAULT NOW()` column.

**Current coverage:**
- ✅ users, circles, circle_members, circle_invites, schedules, sessions, messages, memorization_progress
- ⚠️ **Audit required:** recitation_queue, recitation_queue_entries, session_attendance, device_tokens, message_reads

**Tables that should NOT have `updated_at`:**
- Reference data (quran_surahs, quran_divisions) — immutable
- Audit logs (session_participant_presence) — append-only
- Event logs — append-only

**Migration plan:** Create migration to add `updated_at` to missing tables; update repository methods to set `updated_at = NOW()` on all UPDATEs.

### 6.5 Input Validation

- All API inputs validated and sanitized server-side (never trust client)
- Ayah numbers validated against `quran_surahs` reference table (e.g., Al-Baqarah has 286 Ayahs; `to_ayah: 300` returns HTTP 422)
- File type validation (MIME type, not just extension)
- Max file sizes enforced server-side (not just client-side)
- SQL injection prevention via parameterized queries (Go `database/sql` with `pgx`)
- XSS prevention: message content stored as plain text; HTML escaped on display

### 6.6 Data Privacy

- Voice messages (chat voice notes) stored in MinIO with access-controlled bucket policies
- File URLs are pre-signed and expire after 7 days (renewable on access)
- Personal data (email, phone) not returned in group-visible APIs
- Live-session recording is disabled in MVP (no session audio/video storage)

### 6.7 Transport Security

- HTTPS enforced everywhere (TLS 1.2+, Cloudflare handles certificates)
- WebSocket connections over WSS (TLS)
- LiveKit WebRTC streams encrypted with DTLS/SRTP (built into WebRTC protocol)
- HSTS headers set

### 6.8 Future Security Improvements

- **End-to-end encryption** for direct messages (P3)
- **Two-factor authentication** for teacher accounts (P2)
- **Audit logging** for sensitive actions (remove member, delete messages)
- **GDPR data export** — allow users to download all their data

### 6.9 Privacy Risk Register for Recording (Post-MVP)

- Recording introduces high privacy sensitivity, especially for circles with minors.
- Any future recording rollout requires explicit participant consent UX, retention limits, and strict access controls.
- Recording feature flag must remain OFF until privacy/legal framework is approved.

### 6.10 Firebase Auth Availability & Degraded Mode

Firebase Auth has a 99.95% SLA but has experienced brief outages historically. Behavior during unavailability:

| Scenario | System Behavior |
|----------|----------------|
| Firebase down — active session in progress | Valid JWTs (< 1 hour old) continue to work; sessions are uninterrupted |
| Firebase down — new login attempt | Login fails gracefully: "Authentication temporarily unavailable, try again shortly" |
| Token refresh fails (Firebase down) | Go backend returns HTTP 401; Flutter SDK retries up to 3× before prompting re-login |
| Firebase down — token < 1 hour old | User continues uninterrupted (cached token still valid) |
| Extended outage (> 1 hour) | Users with expired tokens must wait for Firebase recovery before re-authenticating |

**Cached Token Policy:** Firebase tokens have a 1-hour TTL. The FlutterFire SDK caches and auto-refreshes silently. During a brief outage, users authenticated within the last hour are unaffected.

**Migration Path Away from Firebase Auth:** Firebase Auth is the identity layer only (ADR-001). Authorization lives in our PostgreSQL `users` table. Migrating to another provider (e.g., Auth0, Supabase Auth, custom JWT) requires:
1. New JWT validation middleware in Go (swap Firebase public key URL)
2. New Flutter auth package
3. One-time migration of `users.firebase_uid` values

The architecture isolates this change to two files: the Go middleware and the Flutter auth service.

---

## 7. Dependency Version Matrix

Pin versions here. Update this table when bumping a dependency.

### Flutter

| Package | Pinned Version | Notes |
|---------|---------------|-------|
| `livekit_client` | `^2.4.0` | Official LiveKit Flutter SDK; breaking changes between major versions |
| `firebase_auth` | `^5.3.0` | Firebase Auth (FlutterFire) |
| `firebase_messaging` | `^15.1.0` | FCM push notifications |
| `flutter_riverpod` | `^2.6.0` | State management (ADR-003) |
| `go_router` | `^14.6.0` | Navigation |

### Go Backend

| Module | Pinned Version | Notes |
|--------|---------------|-------|
| `labstack/echo/v4` | `v4.13.x` | HTTP framework (ADR-002) |
| `livekit/server-sdk-go` | `v1.7.x` | LiveKit room + token management |
| `golang-jwt/jwt/v5` | `v5.2.x` | JWT validation |
| `jackc/pgx/v5` | `v5.7.x` | PostgreSQL driver |
| `golang-migrate/migrate/v4` | `v4.18.x` | Schema migrations (ADR-006) |
| `firebase.google.com/go/v4` | `v4.14.x` | Firebase Admin SDK (FCM) |

### Infrastructure

| Component | Minimum Version | Notes |
|-----------|----------------|-------|
| LiveKit Server | `v1.8.x` | Must match `server-sdk-go` major version |
| PostgreSQL | `16.x` | Requires `gen_random_uuid()` (PG 13+) |
| Docker | `26.x` | Local dev and production |

> **Policy:** Use `^` (caret) pinning in `pubspec.yaml` and `go.mod`. Pin LiveKit Server version explicitly in `docker-compose.yml`. Version bumps require test run and PR description callout.

---

*This document is the source of truth for technical architecture.*

*See [DEPLOYMENT.md](../deployment/DEPLOYMENT.md) for infrastructure and deployment details.*

