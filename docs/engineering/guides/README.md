# Development Guides

How-to guides, troubleshooting, common workflows, and technical walkthroughs.

---

## Troubleshooting Runbooks

### RB-01: Go backend fails to start

**Symptom:** `go run ./backend/cmd/api` exits with an error immediately.

**Check 1 — Missing environment variables:**
```bash
# Required vars (copy from .env.example)
DATABASE_URL=postgres://halaqaty:password@localhost:5432/halaqaty?sslmode=disable
FIREBASE_PROJECT_ID=your-project-id
FIREBASE_SERVICE_ACCOUNT_JSON=<base64 encoded service account JSON>
LIVEKIT_API_KEY=...
LIVEKIT_API_SECRET=...
MINIO_ENDPOINT=localhost:9000
```

**Check 2 — PostgreSQL not running:**
```bash
docker compose ps           # verify postgres container is Up
docker compose logs postgres # check for init errors
```

**Check 3 — Migrations not applied:**
```bash
make migrate-up             # apply all pending migrations
# or manually:
cd backend
migrate -path ./migrations -database $DATABASE_URL up
```

**Check 4 — Port 8080 already in use:**
```bash
netstat -an | grep 8080     # find conflicting process
# Change PORT env var to another value
```

---

### RB-02: Firebase Auth token validation fails (401 on all requests)

**Symptom:** All authenticated requests return `401 ERR_UNAUTHORIZED` even with a fresh token.

**Check 1 — Service account misconfigured:**
- Verify `FIREBASE_PROJECT_ID` matches the project in Firebase Console
- Verify `FIREBASE_SERVICE_ACCOUNT_JSON` is valid base64-encoded JSON (not the file path)

```bash
# Decode and inspect the service account
echo $FIREBASE_SERVICE_ACCOUNT_JSON | base64 -d | python3 -m json.tool | head -5
# Expected: { "type": "service_account", "project_id": "your-project-id", ... }
```

**Check 2 — Clock skew:**
Firebase tokens include `iat` (issued at) and `exp` (expiry). If the server clock is >5 minutes off, all tokens will be rejected.
```bash
date -u                     # check server time vs actual UTC
ntpdate pool.ntp.org        # sync NTP if needed
```

**Check 3 — Wrong Firebase project:**
If the Flutter app is pointing to a different Firebase project than the backend, tokens won't validate.

---

### RB-03: WebSocket connection drops immediately

**Symptom:** Flutter client connects and disconnects in under 1 second. No events received.

**Check 1 — WS token expired:**
WS tokens are valid for 60 seconds from issuance. If the network round-trip takes longer, the connection is rejected.
```
Look for server log: "ws_token expired" or "token validation failed"
```

**Check 2 — Proposed max connection limit hit:**
The system-design notes propose a max of 3 simultaneous WS connections per user and close code `4001`, but this is not yet codified in `ws_events.md`. Treat this as a future-policy check only until the WS contract is updated.
```
Look for server log: "max_connections_exceeded user_id=..."
```

**Check 3 — Wrong WebSocket URL:**
```
Correct:   wss://api.halaqaty.app/ws?token=<token>
Incorrect: ws://  (non-TLS) or missing token parameter
```

**Check 4 — Nginx proxy not configured for WS:**
WebSocket upgrade requires specific Nginx directives:
```nginx
location /ws {
    proxy_pass http://backend;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_read_timeout 3600s;
}
```

---

### RB-04: LiveKit audio session — students can't hear each other

**Symptom:** Students join the LiveKit room but audio is silent.

**Check 1 — Student audio-publish permission:**
Authorized students join with `CanPublish: true`; F-003 queue order does not
change that permission. Verify the authorized join completed and that an
explicit F-005 moderator action has not muted or removed the participant.

**Check 2 — Flutter audio permissions not granted:**
```
iOS: NSMicrophoneUsageDescription must be in Info.plist
Android: <uses-permission android:name="android.permission.RECORD_AUDIO" />
```

**Check 3 — LiveKit server not reachable:**
```bash
curl https://livekit.halaqaty.app/health   # should return 200 {"status":"ok"}
```

**Check 4 — LiveKit token misconfigured:**
```bash
# Inspect token claims (JWT decode — no secret needed for payload inspection)
echo "<token>" | cut -d. -f2 | base64 -d 2>/dev/null | python3 -m json.tool
# Verify: "video" claims contain CanPublish, CanSubscribe, room name
```

**Check 5 — Audio config boundary:**
Room-level LiveKit settings and client-side Flutter audio capture settings are separate. Verify room creation and token issuance in the backend, then verify microphone capture, echo cancellation, noise suppression, and AGC behavior in the Flutter client or platform layer.

---

### RB-05: Database migration fails

**Symptom:** `make migrate-up` fails with a SQL error.

**Check 1 — Dirty migration state:**
```bash
make migrate-status   # check for "dirty: true"
# If dirty, the migration was partially applied. Fix manually:
cd backend
migrate -path ./migrations -database $DATABASE_URL force <version>
```

**Check 2 — Missing rollback file:**
Every `.up.sql` migration must have a corresponding `.down.sql`. Check:
```bash
ls backend/migrations/ | sort
# Each 000NNN_name.up.sql should have a 000NNN_name.down.sql
```

**Check 3 — Test rollback on fresh schema:**
```bash
make migrate-fresh    # drops all tables, applies all migrations from scratch
```

---

### RB-06: Flutter app fails to build

**Symptom:** `flutter build apk` fails.

**Check 1 — Flutter version mismatch:**
```bash
flutter --version     # use the version resolved for the project/toolchain
flutter upgrade       # upgrade if needed
```

**Check 2 — Pub cache issues:**
```bash
cd mobile
flutter clean
flutter pub get
flutter build apk --debug
```

**Check 3 — Firebase config missing:**
```
android/app/google-services.json    must exist
ios/Runner/GoogleService-Info.plist  must exist
```

---

## Offline & Low-Bandwidth Support (E-008)

Halaqaty must support users on unstable 3G/EDGE connections and brief offline periods.

### Mobile-Side Strategy

**Connection states (Flutter app):**
1. **Online:** Full API + WebSocket connectivity
2. **Degraded (3G/EDGE):** Slow API, high latency, frequent disconnects
3. **Offline:** No connectivity; use local cache

**Dio HTTP client configuration (in `mobile/lib/features/core/data/dio_client.dart`):**

```dart
final options = BaseOptions(
  connectTimeout: Duration(seconds: 10),  // 3G tolerance
  receiveTimeout: Duration(seconds: 20),  // wait for slow responses
  sendTimeout: Duration(seconds: 20),
);

dio.interceptors.add(
  RetryInterceptor(
    dio: dio,
    logPrint: print,
    retries: 3,
    retryDelays: [
      const Duration(seconds: 1),   // retry after 1s, 2s, 4s
      const Duration(seconds: 2),
      const Duration(seconds: 4),
    ],
  ),
);
```

**Cache TTLs (Riverpod + Local SQLite):**
- User profile: 24 hours (invalidate on login/logout)
- Circle data: 6 hours (invalidate on refresh action)
- Session list: 1 hour (invalidate on session end)
- Messages: 3 hours (fetch newer on connect)
- Quran data (surahs/ayahs): indefinite (never changes)

**Offline mode triggers:**
- No network connectivity (WiFi + cellular both offline)
- HTTP request timeout after 3 retries
- WebSocket disconnect lasting > 10 seconds

**Offline capabilities (read-only):**
- View cached user profile, circles, session history
- View cached messages (up to 24 hours old)
- View memorization progress (cached from last sync)
- Read Quran (always cached locally)

**Offline restrictions (unsupported):**
- Cannot send messages (queue locally, send on reconnect — Phase 2)
- Cannot update profile
- Cannot join live sessions (real-time)
- Cannot grade recitations

**Reconnection backoff (exponential):**
- 1st attempt: immediate
- 2nd attempt: after 2 seconds
- 3rd attempt: after 5 seconds
- 4th+ attempts: every 30 seconds

**WebSocket reconnection (in `mobile/lib/features/*/data/websocket_gateway.dart`):**

```dart
Future<void> connect() async {
  int retries = 0;
  while (true) {
    try {
      _socket = await WebSocket.connect(_url);
      retries = 0;  // reset on success
      _onConnected();
      break;
    } catch (e) {
      retries++;
      if (retries > 10) {
        // give up; rely on manual refresh
        return;
      }
      await Future.delayed(
        Duration(seconds: [1, 2, 5, 30, 60][min(retries - 1, 4)]),
      );
    }
  }
}
```

### Backend-Side Strategy

**Idempotency & conflict resolution:**
- All state-changing endpoints must be idempotent (POST should recheck if already done)
- Message sends and progress updates include client-generated UUIDs to prevent duplicates
- Last-write-wins for profile updates; timestamps guide conflict resolution

**Message queuing (Riverpod/Drift on mobile; no backend queue in MVP):**
- User composes message offline
- Message stored locally with `sent_at: null`
- On reconnect, app retries send
- If send succeeds, update local `sent_at` and sync view

**Session graceful exit:**
- If WebSocket drops during active session, client shows "reconnecting..." UI
- Backend keeps participant alive for 30 seconds (grace period)
- After grace period, participant marked as "dropped" (can rejoin)
- Teacher can manually remove participant or wait 5 minutes for auto-cleanup

---

## Code Snippets

Download from Firebase Console → Project Settings → Your Apps.

**Check 4 — Dart analysis errors blocking build:**
```bash
flutter analyze   # zero issues required before building
```

---

## Common Development Tasks

### Reset the local database

```bash
make migrate-fresh    # drop all tables and re-apply all migrations
```

### Add a new DB migration

```bash
migrate create -ext sql -dir backend/migrations -seq <migration_name>
# Creates: backend/migrations/000NNN_<migration_name>.up.sql
#          backend/migrations/000NNN_<migration_name>.down.sql
# Edit both files, then: make migrate-up
```

### Run only Go tests

```bash
cd backend
go test ./...                              # unit tests
go test -tags=integration ./...            # integration tests (requires Docker)
go test -run TestQueueHandler ./internal/queue/...  # specific test
```

### Run only Flutter tests

```bash
cd mobile
flutter test                               # all tests
flutter test test/queue_notifier_test.dart # specific test
```

### Inspect WebSocket events locally

Use [websocat](https://github.com/vi/websocat):
```bash
# Get a realtime ticket first (requires a valid current device session)
REALTIME_TICKET=$(curl -s -X POST http://localhost:8080/api/v1/realtime/tickets \
  -H "Authorization: Bearer <jwt>" \
  -H "X-Halaqaty-Session-ID: <session-id>" | jq -r .token)

# Connect and watch events
websocat "ws://localhost:8080/ws?token=$REALTIME_TICKET"
```

---

*For architecture context, see [`ARCHITECTURE.md`](../architecture/ARCHITECTURE.md). For deployment issues, see [`DEPLOYMENT.md`](../deployment/DEPLOYMENT.md).*

