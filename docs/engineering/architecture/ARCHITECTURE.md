# Halaqaty — Technical Architecture

> **Version:** 1.0 | **Status:** Planning Phase | **Last Updated:** 2026

**Related Documents:** [product/PRD.md](../../management/product/PRD.md) · [planning/PROJECT_PLAN.md](../../management/planning/PROJECT_PLAN.md) · [deployment/DEPLOYMENT.md](../deployment/DEPLOYMENT.md) · [adr/README.md](./adr/README.md) · [../../../DEVELOPMENT.md](../../../DEVELOPMENT.md) · [collaboration/AGENT_COLLABORATION_GUIDE.md](../collaboration/AGENT_COLLABORATION_GUIDE.md)

> **Key architectural decisions** (framework choice, state management, auth boundary, migrations) are documented as ADRs in [`./adr/`](./adr/README.md).

---

## Table of Contents

1. [System Overview Diagram](#1-system-overview-diagram)
2. [Communication Protocols](#2-communication-protocols)
3. [LiveKit + Flutter Integration](#3-livekit--flutter-integration)
4. [Database Schema](#4-database-schema)
5. [API Endpoint Planning](#5-api-endpoint-planning)
6. [Security Considerations](#6-security-considerations)

---

## 1. System Overview Diagram

```
╔═══════════════════════════════════════════════════════════════════╗
║                        CLIENT LAYER                               ║
║                                                                   ║
║   ┌─────────────────────────────────────────────────────────┐    ║
║   │          Flutter App (iOS / Android / Web)               │    ║
║   │                                                          │    ║
║   │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌────────┐  │    ║
║   │  │  Auth UI  │  │ Circles  │  │  Chat UI │  │Session │  │    ║
║   │  │           │  │   UI     │  │          │  │  UI    │  │    ║
║   │  └──────────┘  └──────────┘  └──────────┘  └────────┘  │    ║
║   │                                                          │    ║
║   │  Packages: livekit_client · firebase_auth · firebase_    │    ║
║   │            messaging · flutter_local_notifications       │    ║
║   └─────────────────────────────────────────────────────────┘    ║
╚══════════╦═══════════════════╦═══════════════════╦═══════════════╝
           ║                   ║                   ║
     HTTPS/REST           WebSocket            WebRTC
     (Auth, CRUD)      (Chat, Queue,        (Audio in MVP
                        Presence,            via LiveKit)
                        Notifications)
           ║                   ║                   ║
╔══════════╩═══════════════════╩═══════════════════╩═══════════════╗
║                       BACKEND LAYER                               ║
║                                                                   ║
║  ┌─────────────────────────────────────────────────────────┐     ║
║  │                    Go Backend Server                      │     ║
║  │                                                          │     ║
║  │  ┌─────────────────┐    ┌──────────────────────────┐    │     ║
║  │  │   REST API       │    │    WebSocket Hub          │    │     ║
║  │  │  (Echo v4)        │    │                          │    │     ║
║  │  │                 │    │  ┌──────────────────┐    │    │     ║
║  │  │  /api/v1/auth   │    │  │ Chat Handler     │    │    │     ║
║  │  │  /api/v1/circles│    │  ├──────────────────┤    │    │     ║
║  │  │  /api/v1/sessions│   │  │ Queue Handler    │    │    │     ║
║  │  │  /api/v1/queue  │    │  ├──────────────────┤    │    │     ║
║  │  │  /api/v1/messages│   │  │ Presence Handler │    │    │     ║
║  │  │  /api/v1/progress│   │  ├──────────────────┤    │    │     ║
║  │  │  /api/v1/schedule│   │  │ Notification     │    │    │     ║
║  │  │  /api/v1/notifs  │   │  │ Broadcaster      │    │    │     ║
║  │  └────────┬─────────┘   └──────────┬───────────┘    │    │     ║
║  │           │                        │                 │     ║
║  │  ┌────────▼────────────────────────▼───────┐        │     ║
║  │  │           LiveKit Manager                │        │     ║
║  │  │   (room creation, token generation)      │        │     ║
║  │  └────────┬────────────────────────────────┘        │     ║
║  └───────────┼─────────────────────────────────────────┘     ║
╚═════════════╦╩══════════════════════════════════════════════════╝
              ║
     ┌────────╩────────────────────────────────────────────────┐
     │                    DATA & SERVICES LAYER                  │
     │                                                           │
     │  ┌─────────────┐  ┌─────────────┐  ┌────────────────┐   │
     │  │  PostgreSQL  │  │    MinIO     │  │  LiveKit SFU   │   │
     │  │  (Primary DB)│  │ (File Store) │  │ (WebRTC Server)│   │
     │  └─────────────┘  └─────────────┘  └────────────────┘   │
     │                                                           │
     │  ┌─────────────────────────┐  ┌──────────────────────┐   │
     │  │      Firebase           │  │     Cloudflare       │   │
     │  │  Auth (Identity)        │  │  (DNS + SSL/TLS)     │   │
     │  │  FCM (Push Notifs)      │  │                      │   │
     │  └─────────────────────────┘  └──────────────────────┘   │
     └───────────────────────────────────────────────────────────┘
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
```
1. Client calls POST /api/v1/sessions/{id}/ws-token → receives short-lived token (60s TTL)
    ↓
2. Client connects: wss://api.halaqaty.app/ws?token=<ws_token>
    ↓
3. Server validates token, upgrades connection, registers client in Hub (by userID)
    ↓
4. Server adds client to relevant circle/session rooms
    ↓
5. Bidirectional messages:
   Client → Server: cmd.raise_hand, cmd.lower_hand, chat.typing, ping
   Server → Client: queue.*, session.*, chat.*, error, pong
    ↓
6. Client disconnects → Hub removes registration
```

**Heartbeat:** client sends `{"type":"ping"}` every 30 seconds; server replies `{"type":"pong","server_time":"<ISO8601>"}`.

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
| `queue.grade_submitted` | Grade recorded for a completed turn (teacher/supervisor only) |
| `session.started` | Session went live; includes `livekit_url` and `livekit_token` |
| `session.ended` | Session ended by teacher |
| `session.participant_joined` | A participant joined the session |
| `session.participant_left` | A participant left the session |
| `session.hand_raised` | A student raised their hand |
| `chat.message` | New circle message delivered |
| `chat.message_read` | Recipient read a message (sent to sender) |
| `chat.typing` | Typing indicator |
| `error` | Server could not process a client command |

**Client → Server command types:**

| Type | Description |
|------|-------------|
| `cmd.raise_hand` | Student raises hand in session |
| `cmd.lower_hand` | Student lowers hand in session |
| `ping` | Heartbeat (every 30 s) |

> **Source of truth for all event schemas and payloads:** [`docs/contracts/ws_events.md`](../../contracts/ws_events.md)

> **Reconnection:** on reconnect, clients re-fetch state via REST (`GET /api/v1/sessions/{id}/queue`, etc.) rather than relying solely on buffered WebSocket events.

### 2.3 WebRTC via LiveKit

**Used for:** Audio streaming in live sessions for MVP (video remains post-MVP behind feature flag).

- LiveKit is a Selective Forwarding Unit (SFU) — it receives each participant's stream and forwards it to all others, without mixing
- This is more scalable than peer-to-peer WebRTC (which doesn't scale beyond ~4 participants)
- Flutter client uses `livekit_client` package (official LiveKit Flutter SDK)
- Go backend uses `livekit-server-sdk-go` for room management and token generation

**Quality levels supported:**
- Audio: Opus codec, 48–64 kbps, mono (sufficient for voice)
- Video (post-MVP feature flag): VP8/VP9, simulcast (low/medium/high), adaptive bitrate

### 2.4 Firebase Cloud Messaging (FCM)

**Used for:** Push notifications when the app is in the background or completely closed.

**Flow:**
```
Event occurs (e.g., teacher starts session)
       ↓
Go Backend detects event
       ↓
Go Backend retrieves user's FCM device tokens from DB
       ↓
Go Backend → Firebase FCM API (HTTP POST)
       ↓
FCM → User's device (iOS APNs or Android FCM)
       ↓
Device shows notification even if app is closed
```

---

## 3. LiveKit + Flutter Integration

### 3.1 Complete Integration Flow

```
┌──────────────────────────────────────────────────────────────────┐
│                    LIVEKIT INTEGRATION FLOW                       │
│                                                                   │
│  STEP 1: Teacher starts session                                   │
│  ════════════════════════════                                     │
│                                                                   │
│  Flutter UI                    Go Backend              LiveKit    │
│     │                              │                    Server   │
│     │  POST /sessions/{id}/start   │                      │      │
│     │─────────────────────────────►│                      │      │
│     │                              │  CreateRoom(name)    │      │
│     │                              │─────────────────────►│      │
│     │                              │  Room created ✓      │      │
│     │                              │◄─────────────────────│      │
│     │  { token, livekit_url }      │                      │      │
│     │◄─────────────────────────────│                      │      │
│                                                                   │
│  STEP 2: Flutter connects to LiveKit                              │
│  ════════════════════════════════                                 │
│                                                                   │
│  Flutter (livekit_client)                          LiveKit SFU   │
│     │                                                    │        │
│     │  room.connect(url, token, RoomOptions{...})        │        │
│     │───────────────────────────────────────────────────►│        │
│     │  WebRTC handshake (ICE, DTLS, SRTP)               │        │
│     │◄──────────────────────────────────────────────────►│        │
│     │  Connected ✓                                       │        │
│     │◄───────────────────────────────────────────────────│        │
│                                                                   │
│  STEP 3: Other participants join                                  │
│  ══════════════════════════════                                   │
│                                                                   │
│  Student Flutter              Go Backend              LiveKit    │
│     │                              │                    Server   │
│     │  POST /sessions/{id}/join    │                      │      │
│     │─────────────────────────────►│                      │      │
│     │  GenerateJWT(roomName, uid)  │                      │      │
│     │                              │  (student has join   │      │
│     │                              │   permission only,   │      │
│     │                              │   not room admin)    │      │
│     │  { session, livekit_url,     │                      │      │
│     │    livekit_token }           │                      │      │
│     │◄─────────────────────────────│                      │      │
│     │  room.connect(url, token)    │                      │      │
│     │───────────────────────────────────────────────────►│       │
│     │  Connected; receives audio streams                 │       │
│     │◄───────────────────────────────────────────────────│       │
│                                                                   │
│  STEP 4: Teacher controls (mute, remove)                         │
│  ════════════════════════════════════                            │
│                                                                   │
│  Note: Teacher mute/remove actions are performed via the         │
│  LiveKit Admin SDK on the Go backend — triggered by future       │
│  endpoints to be specced post-MVP. In MVP, the teacher manages   │
│  turns through the queue system (grant/revoke CanPublish) rather │
│  than explicit mute calls.                                        │
└──────────────────────────────────────────────────────────────────┘
```

### 3.2 Go Backend — Token Generation Code Pattern

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

### 3.3 Flutter — Room Connection Pattern

```dart
// Using livekit_client Flutter package

Future<Room> connectToSession({
  required String livekitUrl,
  required String token,
}) async {
  final room = Room();
  
  // Quran recitation optimized audio settings
  final roomOptions = RoomOptions(
    defaultAudioPublishOptions: const AudioPublishOptions(
      name: 'recitation',
      audioBitrate: 48000,     // 48kbps minimum
      // Note: noise suppression and auto-gain are handled
      // at the platform level via AudioProcessingOptions
    ),
    // MVP intentionally omits video publish options.
    // Post-MVP video can be enabled behind a feature flag without re-architecting.
    adaptiveStream: true,      // auto-adjust quality to bandwidth
  );
  
  await room.connect(livekitUrl, token, roomOptions: roomOptions);
  
  return room;
}
```

### 3.4 Audio Configuration (Critical)

```dart
// Disable noise suppression and auto-gain for Quran recitation
// Must be set before connecting to the room

await Hardware.instance.setPreferSpeakerOutput(true);

// Platform-level audio processing
final audioConstraints = {
  'echoCancellation': true,    // Keep ON — prevents feedback
  'noiseSuppression': false,   // OFF — preserves tajweed phonetics
  'autoGainControl': false,    // OFF — consistent recitation volume
};
```

### 3.5 Network Requirements

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

---

## 4. Database Schema

> **Migration tool:** [golang-migrate v4](https://github.com/golang-migrate/migrate) — sequential SQL files in `backend/migrations/`. See [ADR-006](adr/ADR-006-db-migrations.md) for rationale.

### 4.0 Domain Enumerations

These are the canonical enum values used in PostgreSQL CHECK constraints and Go backend constants. Product-level human labels are in [FEATURES.md F-003](../../management/product/FEATURES.md#f-003-recitation-queue-system).

#### Recitation Grade (`grade` column)

| DB Value | English Label | Arabic Label | Meaning |
|----------|--------------|--------------|---------|
| `excellent` | Excellent | ممتاز | Perfect recitation, excellent tajweed |
| `good` | Good | جيد | Minor to moderate errors, acceptable tajweed |
| `needs_improvement` | Needs Improvement | يحتاج تحسين | Notable errors; requires more practice |
| `repeat` | Repeat | إعادة | Must fully repeat before advancing |

Used in: `recitation_queue_entries.grade`, `memorization_progress.grade`

### 4.1 Entity-Relationship Diagram (ASCII)

```
┌───────────────┐     ┌───────────────────┐     ┌───────────────┐
│    users      │     │   circle_members  │     │    circles    │
│───────────────│     │───────────────────│     │───────────────│
│ id (PK)       │────►│ user_id (FK)      │◄────│ id (PK)       │
│ name          │     │ circle_id (FK)    │     │ name          │
│ email         │     │ role              │     │ description   │
│ phone         │     │ joined_at         │     │ teacher_id(FK)│
│ avatar_url    │     └───────────────────┘     │ invite_code   │
│ preferred_lang│     ┌───────────────────┐     │ max_members   │
│ created_at    │                               │ gender_spec   │
│ updated_at    │                               │ created_at    │
└──────┬────────┘                               └───────┬───────┘
       │                                        ┌───────▼───────┐
       │                                        │   schedules   │
       │                                        │───────────────│
       │              ┌───────────────────┐     │ id (PK)       │
       │              │   notifications   │     │ circle_id (FK)│
       │◄─────────────│───────────────────│     │ day_of_week   │
       │              │ id (PK)           │     │ start_time    │
       │              │ user_id (FK)      │     │ end_time      │
       │              │ type              │     │ timezone      │
       │              │ title             │     │ is_active     │
       │              │ body              │     └───────────────┘
       │              │ data_json         │
       │              │ is_read           │     ┌───────────────┐
       │              │ created_at        │     │   sessions    │
       │              └───────────────────┘     │───────────────│
       │                                        │ id (PK)       │
       │              ┌───────────────────┐     │ circle_id (FK)│
       └─────────────►│    messages       │     │ title         │
                      │───────────────────│◄────│ scheduled_at  │
                      │ id (PK)           │     │ actual_start  │
                      │ circle_id (FK)    │     │ actual_end    │
                      │ sender_id (FK)    │     │ status        │
                      │ content           │     │ lk_room_name  │
                      │ type              │     └───────┬───────┘
                      │ reply_to_id (FK)  │             │
                      │ is_pinned         │     ┌───────▼───────┐
                      │ created_at        │     │  session_     │
                      └───────────────────┘     │  attendance   │
                                                │───────────────│
                      ┌───────────────────┐     │ session_id(FK)│
                      │  message_reads    │     │ user_id (FK)  │
                      │───────────────────│     │ joined_at     │
                      │ message_id (FK)   │     │ left_at       │
                      │ user_id (FK)      │     │ status        │
                      │ read_at           │     └───────────────┘
                      └───────────────────┘
                                                ┌───────────────┐
                      ┌───────────────────┐     │recitation_    │
                      │ memorization_     │     │queue          │
                      │ progress          │     │───────────────│
                      │───────────────────│     │ id (PK)       │
                      │ id (PK)           │     │ session_id(FK)│
                      │ student_id (FK)   │     │ round_number  │
                      │ circle_id (FK)    │     │ round_type    │
                      │ surah_name        │     │ surah_name    │
                      │ from_ayah         │     │ from_ayah     │
                      │ to_ayah           │     │ to_ayah       │
                      │ type              │     │ created_at    │
                      │ grade             │     └───────┬───────┘
                      │ notes             │             │
                      │ session_id (FK)   │     ┌───────▼───────┐
                      │ date              │     │recitation_    │
                      └───────────────────┘     │queue_entries  │
                                                │───────────────│
                                                │ id (PK)       │
                                                │ queue_id (FK) │
                                                │ student_id(FK)│
                                                │ position      │
                                                │ status        │
                                                │ grade         │
                                                │ teacher_notes │
                                                │ started_at    │
                                                │ completed_at  │
                                                └───────────────┘
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
| gender_restriction | VARCHAR(20) | CHECK IN ('male','female','mixed') DEFAULT 'mixed' | Audience restriction |
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
| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK | |
| circle_id | UUID | FK → circles.id NOT NULL | |
| notes | TEXT | maxLength 500 | Optional session notes (visible to members) |
| scheduled_at | TIMESTAMPTZ | | Planned start time; NULL for ad-hoc sessions |
| actual_start | TIMESTAMPTZ | | When teacher actually started |
| actual_end | TIMESTAMPTZ | | When session actually ended |
| status | VARCHAR(20) | CHECK IN ('scheduled','active','ended') | |
| media_mode | VARCHAR(20) | CHECK IN ('audio_only','audio_video'), DEFAULT 'audio_only' | Session media policy (MVP always audio_only) |
| livekit_room_name | VARCHAR(200) | UNIQUE | LiveKit room identifier |
| created_by | UUID | FK → users.id NOT NULL | Teacher who created the session |
| participant_count | INTEGER | DEFAULT 0 | Running count updated on join/leave |
| created_at | TIMESTAMPTZ | DEFAULT NOW() | |

#### `session_attendance`
| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK | |
| session_id | UUID | FK → sessions.id NOT NULL | |
| user_id | UUID | FK → users.id NOT NULL | |
| joined_at | TIMESTAMPTZ | | When student joined LiveKit room |
| left_at | TIMESTAMPTZ | | When student left |
| status | VARCHAR(20) | CHECK IN ('present','absent','late','excused') | |
| overridden_by | UUID | FK → users.id | Teacher who manually overrode |
| UNIQUE | (session_id, user_id) | | |

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
| grading_required | BOOLEAN | NOT NULL | Overrides circle grading_policy for this round |
| is_active | BOOLEAN | DEFAULT TRUE | Only one active queue per session |
| created_at | TIMESTAMPTZ | DEFAULT NOW() | |

#### `recitation_queue_entries`
| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK | |
| queue_id | UUID | FK → recitation_queue.id NOT NULL | |
| student_id | UUID | FK → users.id NOT NULL | |
| position | INTEGER | NOT NULL | Order in queue (1, 2, 3...) |
| status | VARCHAR(20) | CHECK IN ('waiting','reciting','completed','skipped','opted_out') | |
| grade | VARCHAR(30) | CHECK IN ('excellent','good','needs_improvement','repeat') | |
| teacher_notes | TEXT | | Free-form notes from teacher |
| started_at | TIMESTAMPTZ | | When recitation began |
| completed_at | TIMESTAMPTZ | | When recitation ended / was graded |

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
| student_id | UUID | FK → users.id NOT NULL | |
| circle_id | UUID | FK → circles.id NOT NULL | |
| session_id | UUID | FK → sessions.id | Source session |
| queue_entry_id | UUID | FK → recitation_queue_entries.id | Source queue entry |
| surah_name | VARCHAR(100) | NOT NULL | |
| from_ayah | INTEGER | NOT NULL | |
| to_ayah | INTEGER | NOT NULL | |
| type | VARCHAR(30) | CHECK IN ('new_memorization','revision','old_revision') | |
| grade | VARCHAR(30) | CHECK IN ('excellent','good','needs_improvement','repeat') | Same grade enum as queue entries |
| notes | TEXT | | Teacher notes |
| date | DATE | NOT NULL | Session date |
| created_at | TIMESTAMPTZ | DEFAULT NOW() | |

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

### Authentication: All endpoints require `Authorization: Bearer <firebase-jwt>` except `/auth/*`

---

### `/auth`
| Method | Path | Status | Description |
|--------|------|--------|-------------|
| POST | `/auth/register` | ✅ | Create user profile after Firebase registration |
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
| PUT | `/circles/{id}/members/{userId}/role` | 🔲 | Update member role (assign/revoke supervisor) |
| POST | `/circles/{id}/invite-code/refresh` | 🔲 | Generate new invite code |

### `/sessions`
| Method | Path | Status | Description |
|--------|------|--------|-------------|
| GET | `/circles/{id}/sessions` | ✅ | List sessions for a circle |
| POST | `/circles/{id}/sessions` | ✅ | Create a session (teacher only) |
| POST | `/sessions/{id}/start` | ✅ | Teacher starts session (creates LiveKit room, returns token) |
| POST | `/sessions/{id}/join` | ✅ | Join an active session — returns LiveKit token (members only) |
| POST | `/sessions/{id}/end` | ✅ | Teacher ends session |
| POST | `/sessions/{id}/ws-token` | ✅ | Issue short-lived WebSocket connection token |
| GET | `/sessions/{id}` | 🔲 | Get session details |
| POST | `/sessions/{id}/participants/{userId}/mute` | 🔲 | Mute a participant |
| POST | `/sessions/{id}/participants/{userId}/unmute` | 🔲 | Unmute a participant |
| POST | `/sessions/{id}/participants/{userId}/remove` | 🔲 | Remove participant from session |
| POST | `/sessions/{id}/lock` | 🔲 | Lock session (no new joiners) |

### `/sessions/{id}/attendance`
| Method | Path | Status | Description |
|--------|------|--------|-------------|
| GET | `/sessions/{id}/attendance` | 🔲 | List attendance for a session |
| PUT | `/sessions/{id}/attendance/{userId}` | 🔲 | Manual attendance override |

### `/queue`
| Method | Path | Status | Description |
|--------|------|--------|-------------|
| GET | `/sessions/{id}/queue` | ✅ | Get current queue state |
| POST | `/sessions/{id}/queue/rounds` | ✅ | Start a new round (Surah, Ayah range) |
| POST | `/sessions/{id}/queue/entries/{entryId}/grade` | ✅ | Grade a student's recitation (teacher/supervisor only) |
| POST | `/sessions/{id}/queue/opt-out` | ✅ | Student opts out of current round |
| POST | `/sessions/{id}/queue/reset` | 🔲 | Reset queue (creates new round) |
| PUT | `/sessions/{id}/queue/entries/{entryId}/status` | 🔲 | Update student status in queue |
| PUT | `/sessions/{id}/queue/order` | 🔲 | Reorder queue `{ ordered_entry_ids: [...] }` |
| POST | `/sessions/{id}/queue/entries` | 🔲 | Add a student to queue (late-joiner) |
| DELETE | `/sessions/{id}/queue/entries/{entryId}` | 🔲 | Remove student from queue |

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
| GET | `/circles/{id}/progress` | 🔲 | All students' progress in a circle |
| GET | `/circles/{id}/progress/{userId}` | 🔲 | One student's detailed progress |
| POST | `/circles/{id}/progress` | 🔲 | Manual progress entry (outside session) |
| GET | `/progress/me` | 🔲 | My own progress across all circles |

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
- **Token lifecycle:** Firebase tokens expire after 1 hour; `firebase_client` SDK auto-refreshes silently
- **LiveKit tokens:** Generated exclusively by Go backend; never by the Flutter client. Student publish scope is turn-based and non-admin.

### 6.2 LiveKit Room Security

- Each session generates a unique LiveKit room name (UUID-based)
- Room names are not publicly guessable
- Each participant needs a JWT from Go backend to join — no anonymous access
- Teacher's JWT includes `RoomAdmin: true` (can mute, remove)
- Student's JWT defaults to `CanPublish: false`, `CanPublishVideo: false`, and never includes `RoomAdmin`
- Backend grants `CanPublish: true` only for the active reciter turn, then revokes after the turn (audio-only in MVP)
- Room is deleted from LiveKit server when session ends

### 6.3 Rate Limiting

- REST API: rate limited by IP and by user ID
- WebSocket: connections limited per user (max 3 active connections per user)
- Message sending: max 30 messages per minute per user per circle
- File uploads: max 10 uploads per hour per user

### 6.4 Input Validation

- All API inputs validated and sanitized server-side (never trust client)
- Ayah numbers validated against `quran_surahs` reference table (e.g., Al-Baqarah has 286 Ayahs; `to_ayah: 300` returns HTTP 422)
- File type validation (MIME type, not just extension)
- Max file sizes enforced server-side (not just client-side)
- SQL injection prevention via parameterized queries (Go `database/sql` with `pgx`)
- XSS prevention: message content stored as plain text; HTML escaped on display

### 6.5 Data Privacy

- Voice messages (chat voice notes) stored in MinIO with access-controlled bucket policies
- File URLs are pre-signed and expire after 7 days (renewable on access)
- Personal data (email, phone) not returned in group-visible APIs
- Live-session recording is disabled in MVP (no session audio/video storage)

### 6.6 Transport Security

- HTTPS enforced everywhere (TLS 1.2+, Cloudflare handles certificates)
- WebSocket connections over WSS (TLS)
- LiveKit WebRTC streams encrypted with DTLS/SRTP (built into WebRTC protocol)
- HSTS headers set

### 6.7 Future Security Improvements

- **End-to-end encryption** for direct messages (P3)
- **Two-factor authentication** for teacher accounts (P2)
- **Audit logging** for sensitive actions (remove member, delete messages)
- **GDPR data export** — allow users to download all their data

### 6.8 Privacy Risk Register for Recording (Post-MVP)

- Recording introduces high privacy sensitivity, especially for circles with minors.
- Any future recording rollout requires explicit participant consent UX, retention limits, and strict access controls.
- Recording feature flag must remain OFF until privacy/legal framework is approved.

### 6.9 Firebase Auth Availability & Degraded Mode

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

