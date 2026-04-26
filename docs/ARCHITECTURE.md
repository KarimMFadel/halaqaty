# Halaqaty — Technical Architecture

> **Version:** 1.0 | **Status:** Planning Phase | **Last Updated:** 2026

**Related Documents:** [PRD.md](PRD.md) · [PLAN.md](PLAN.md) · [DEPLOYMENT.md](DEPLOYMENT.md)

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
║  │  │  (Gin / Echo)    │    │                          │    │     ║
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
Client connects → WS /ws?token=<jwt>
    ↓
Server authenticates JWT
    ↓
Server registers client in Hub (by userID)
    ↓
Client subscribes to rooms (circle IDs, session IDs)
    ↓
Bidirectional messages:
  Client → Server: chat message, queue action, typing indicator
  Server → Client: chat message, queue update, presence change, notification
    ↓
Client disconnects → Hub removes registration → Presence update broadcast
```

**WebSocket Message Format:**
```json
{
  "type": "queue.student_status_changed",
  "session_id": "sess_123",
  "data": {
    "entry_id": "entry_456",
    "student_id": "user_789",
    "status": "reciting",
    "position": 3
  },
  "timestamp": "2026-03-15T10:30:00Z"
}
```

**Message Types:**
| Type | Direction | Description |
|------|-----------|-------------|
| `chat.message` | S→C, C→S | New message in circle or DM |
| `chat.typing` | C→S, S→C | Typing indicator |
| `chat.read` | C→S | Mark messages as read |
| `queue.updated` | S→C | Full queue state refresh |
| `queue.student_status_changed` | S→C | Single student status update |
| `queue.round_started` | S→C | New round created |
| `queue.reset` | S→C | Queue was reset |
| `presence.online` | S→C | User came online |
| `presence.offline` | S→C | User went offline |
| `notification.in_app` | S→C | In-app notification |
| `session.started` | S→C | Teacher started session |
| `session.ended` | S→C | Session ended |

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
│     │  GET /sessions/{id}/token    │                      │      │
│     │─────────────────────────────►│                      │      │
│     │  GenerateJWT(roomName, uid)  │                      │      │
│     │                              │  (student has join   │      │
│     │                              │   permission only,   │      │
│     │                              │   not room admin)    │      │
│     │  { token }                   │                      │      │
│     │◄─────────────────────────────│                      │      │
│     │  room.connect(url, token)    │                      │      │
│     │───────────────────────────────────────────────────►│       │
│     │  Connected; receives audio streams                 │       │
│     │◄───────────────────────────────────────────────────│       │
│                                                                   │
│  STEP 4: Teacher controls (mute, remove)                         │
│  ════════════════════════════════════                            │
│                                                                   │
│  Flutter UI                    Go Backend              LiveKit   │
│     │  POST /sessions/{id}/       │                    Server   │
│     │    mute/{participant_id}    │                      │       │
│     │─────────────────────────────►│                      │      │
│     │                              │  MutePublishedTrack  │      │
│     │                              │  (LiveKit Admin API) │      │
│     │                              │─────────────────────►│      │
│     │                              │  Participant muted   │      │
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

---

## 4. Database Schema

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
│ fcm_token     │                               │ max_members   │
│ preferred_lang│     ┌───────────────────┐     │ privacy       │
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
| name | VARCHAR(100) | NOT NULL | Display name |
| email | VARCHAR(255) | UNIQUE | Email (nullable for phone-only) |
| phone | VARCHAR(20) | UNIQUE | Phone number with country code |
| avatar_url | TEXT | | MinIO object URL |
| preferred_lang | VARCHAR(10) | DEFAULT 'ar' | ISO 639-1 language code |
| fcm_token | TEXT | | Current device FCM token |
| created_at | TIMESTAMPTZ | DEFAULT NOW() | |
| updated_at | TIMESTAMPTZ | DEFAULT NOW() | |

#### `circles`
| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK | |
| name | VARCHAR(100) | NOT NULL | Circle display name |
| description | TEXT | | Circle description |
| rules | TEXT | | Circle rules/guidelines |
| teacher_id | UUID | FK → users.id NOT NULL | Circle owner |
| invite_code | VARCHAR(20) | UNIQUE NOT NULL | Join code (e.g., HLQ-7X2K) |
| max_members | INTEGER | DEFAULT 50 | Maximum student capacity |
| privacy | VARCHAR(20) | CHECK IN ('public','private') | |
| gender_spec | VARCHAR(20) | CHECK IN ('male','female','mixed','unspecified') | |
| language | VARCHAR(10) | DEFAULT 'ar' | Primary language |
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
| title | VARCHAR(200) | | Session title |
| scheduled_start | TIMESTAMPTZ | | Planned start time |
| scheduled_end | TIMESTAMPTZ | | Planned end time |
| actual_start | TIMESTAMPTZ | | When teacher actually started |
| actual_end | TIMESTAMPTZ | | When session actually ended |
| status | VARCHAR(20) | CHECK IN ('scheduled','live','completed','cancelled') | |
| media_mode | VARCHAR(20) | CHECK IN ('audio_only','audio_video'), DEFAULT 'audio_only' | Session media policy (MVP always audio_only) |
| livekit_room_name | VARCHAR(200) | UNIQUE | LiveKit room identifier |
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
| surah_name | VARCHAR(100) | | Arabic Surah name |
| from_ayah | INTEGER | | Starting Ayah number |
| to_ayah | INTEGER | | Ending Ayah number |
| is_active | BOOLEAN | DEFAULT TRUE | Only one active queue per session |
| created_at | TIMESTAMPTZ | DEFAULT NOW() | |

#### `recitation_queue_entries`
| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK | |
| queue_id | UUID | FK → recitation_queue.id NOT NULL | |
| student_id | UUID | FK → users.id NOT NULL | |
| position | INTEGER | NOT NULL | Order in queue (1, 2, 3...) |
| status | VARCHAR(20) | CHECK IN ('waiting','reciting','completed','skipped') | |
| grade | VARCHAR(30) | CHECK IN ('excellent','very_good','good','acceptable','needs_review','repeat') | |
| teacher_notes | TEXT | | Free-form notes from teacher |
| started_at | TIMESTAMPTZ | | When recitation began |
| completed_at | TIMESTAMPTZ | | When recitation ended / was graded |

#### `messages`
| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK | |
| circle_id | UUID | FK → circles.id NOT NULL | Null for DMs |
| dm_recipient_id | UUID | FK → users.id | For direct messages |
| sender_id | UUID | FK → users.id NOT NULL | |
| content | TEXT | | Text content (empty for voice/files) |
| type | VARCHAR(20) | CHECK IN ('text','voice','image','file') | |
| file_url | TEXT | | MinIO presigned URL |
| file_name | VARCHAR(255) | | Original filename |
| file_size_bytes | INTEGER | | |
| reply_to_id | UUID | FK → messages.id | Threaded reply |
| is_pinned | BOOLEAN | DEFAULT FALSE | |
| created_at | TIMESTAMPTZ | DEFAULT NOW() | |
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
| grade | VARCHAR(30) | | Same grade enum as queue entries |
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
| start_time | TIME | NOT NULL | Local time |
| end_time | TIME | NOT NULL | Local time |
| timezone | VARCHAR(50) | NOT NULL | IANA timezone (e.g., 'Asia/Riyadh') |
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

---

## 5. API Endpoint Planning

### Base URL: `https://api.halaqaty.app/api/v1`

### Authentication: All endpoints require `Authorization: Bearer <firebase-jwt>` except `/auth/*`

---

### `/auth`
| Method | Path | Description |
|--------|------|-------------|
| POST | `/auth/register` | Create user profile after Firebase registration |
| POST | `/auth/fcm-token` | Update device FCM token |
| GET | `/auth/me` | Get current user profile |
| PUT | `/auth/me` | Update profile (name, avatar, language) |
| DELETE | `/auth/me` | Delete account and all data |

### `/circles`
| Method | Path | Description |
|--------|------|-------------|
| GET | `/circles` | List my circles (teacher + student) |
| POST | `/circles` | Create a new circle |
| GET | `/circles/{id}` | Get circle details |
| PUT | `/circles/{id}` | Update circle settings |
| DELETE | `/circles/{id}` | Delete circle (teacher only) |
| POST | `/circles/{id}/archive` | Archive circle (teacher only) |
| POST | `/circles/join` | Join a circle by invite code `{ invite_code }` |
| POST | `/circles/{id}/leave` | Leave a circle |
| GET | `/circles/{id}/members` | List members with roles |
| PUT | `/circles/{id}/members/{userId}/role` | Update member role (assign/revoke supervisor) |
| DELETE | `/circles/{id}/members/{userId}` | Remove member from circle |
| POST | `/circles/{id}/invite-code/refresh` | Generate new invite code |

### `/sessions`
| Method | Path | Description |
|--------|------|-------------|
| GET | `/circles/{id}/sessions` | List sessions for a circle |
| POST | `/circles/{id}/sessions` | Create a session |
| GET | `/sessions/{id}` | Get session details |
| POST | `/sessions/{id}/start` | Teacher starts session (creates LiveKit room) |
| POST | `/sessions/{id}/end` | Teacher ends session |
| GET | `/sessions/{id}/token` | Get LiveKit JWT token for this session |
| POST | `/sessions/{id}/participants/{userId}/mute` | Mute a participant |
| POST | `/sessions/{id}/participants/{userId}/unmute` | Unmute a participant |
| POST | `/sessions/{id}/participants/{userId}/remove` | Remove participant from session |
| POST | `/sessions/{id}/lock` | Lock session (no new joiners) |

### `/sessions/{id}/attendance`
| Method | Path | Description |
|--------|------|-------------|
| GET | `/sessions/{id}/attendance` | List attendance for a session |
| PUT | `/sessions/{id}/attendance/{userId}` | Manual attendance override |

### `/queue`
| Method | Path | Description |
|--------|------|-------------|
| GET | `/sessions/{id}/queue` | Get current queue state |
| POST | `/sessions/{id}/queue/rounds` | Start a new round (Surah, Ayah range, type) |
| POST | `/sessions/{id}/queue/reset` | Reset queue (creates new round) |
| PUT | `/sessions/{id}/queue/entries/{entryId}/status` | Update student status in queue |
| PUT | `/sessions/{id}/queue/entries/{entryId}/grade` | Grade a student's recitation |
| PUT | `/sessions/{id}/queue/order` | Reorder queue `{ ordered_entry_ids: [...] }` |
| POST | `/sessions/{id}/queue/entries` | Add a student to queue (late-joiner) |
| DELETE | `/sessions/{id}/queue/entries/{entryId}` | Remove student from queue |

### `/messages`
| Method | Path | Description |
|--------|------|-------------|
| GET | `/circles/{id}/messages` | List circle messages (paginated) |
| POST | `/circles/{id}/messages` | Send a message |
| DELETE | `/circles/{id}/messages/{msgId}` | Delete a message |
| POST | `/circles/{id}/messages/{msgId}/pin` | Pin a message |
| DELETE | `/circles/{id}/messages/{msgId}/pin` | Unpin a message |
| POST | `/circles/{id}/messages/{msgId}/read` | Mark message as read |
| GET | `/dm/{userId}` | List DM conversation with a user |
| POST | `/dm/{userId}` | Send a direct message |

### `/progress`
| Method | Path | Description |
|--------|------|-------------|
| GET | `/circles/{id}/progress` | All students' progress in a circle |
| GET | `/circles/{id}/progress/{userId}` | One student's detailed progress |
| POST | `/circles/{id}/progress` | Manual progress entry (outside session) |
| GET | `/progress/me` | My own progress across all circles |

### `/schedule`
| Method | Path | Description |
|--------|------|-------------|
| GET | `/circles/{id}/schedules` | List schedules for a circle |
| POST | `/circles/{id}/schedules` | Create a schedule |
| PUT | `/circles/{id}/schedules/{schedId}` | Update a schedule |
| DELETE | `/circles/{id}/schedules/{schedId}` | Delete a schedule |
| GET | `/schedule/me` | My unified calendar across all circles |

### `/notifications`
| Method | Path | Description |
|--------|------|-------------|
| GET | `/notifications` | List my notifications (paginated) |
| PUT | `/notifications/{id}/read` | Mark notification as read |
| PUT | `/notifications/read-all` | Mark all as read |
| GET | `/notifications/preferences` | Get notification preferences |
| PUT | `/notifications/preferences` | Update preferences |

### `/uploads`
| Method | Path | Description |
|--------|------|-------------|
| POST | `/uploads/avatar` | Upload user avatar (multipart/form-data) |
| POST | `/uploads/voice` | Upload voice message (returns presigned URL pattern) |
| POST | `/uploads/image` | Upload image attachment |
| POST | `/uploads/file` | Upload file attachment (PDF) |

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
- Ayah numbers validated against known Surah lengths
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

---

*This document is the source of truth for technical architecture.*

*See [DEPLOYMENT.md](DEPLOYMENT.md) for infrastructure and deployment details.*
