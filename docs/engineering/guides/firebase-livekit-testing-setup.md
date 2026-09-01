# Firebase and LiveKit Testing Setup

This guide explains the external setup needed to run the F-003 T048 mobile
integration journey.

## Short answer

- A Firebase project on the no-cost Spark plan is suitable for this test.
- Firebase must be configured once by the development team; ordinary users do
  not create Firebase projects or Firebase users manually.
- The current app flow supports email/password registration and sign-in
  through the Firebase Flutter SDK. The app then provisions the user in the
  Halaqaty database and creates a backend device session.
- A LiveKit Cloud account is not required. Halaqaty's MVP uses a self-hosted
  LiveKit server. For local development, LiveKit's development server can use
  `devkey` and `secret`; production must use private server-side values.

Firebase Authentication is the identity provider; it is not the application's
authorization system. Circle roles are stored in PostgreSQL, so a Firebase
account is not automatically a teacher or student.

## Why T048 needs two accounts

T048 verifies two different authenticated participants at the same time:

1. The teacher creates and starts the live session, receives manager-only
   queue events, and approves or declines an opt-out request.
2. The student joins late, sees the student's own queue position, requests an
   opt-out, reconnects, and verifies that manager-only events are not delivered
   to the student.

These are two Firebase identities because the backend must distinguish two
users. Their teacher/student roles are assigned by Halaqaty circle membership,
not by Firebase. The same Firebase identity could be a teacher in one circle
and a student in another, but two identities are required for this concurrent
teacher/student acceptance journey.

## What the application does for a normal user

The intended normal-user flow is:

```text
Register or sign in in the app
        |
        v
Firebase SDK creates/signs in the identity and refreshes the Firebase ID token
        |
        v
POST /api/v1/auth/register       (new user: provision local Halaqaty user)
or    /api/v1/auth/sessions      (returning user: create a device session)
        |
        v
Halaqaty returns an opaque backend session ID
        |
        v
Protected requests send both:
  Authorization: Bearer <Firebase ID token>
  X-Halaqaty-Session-ID: <backend session ID>
```

The Go backend never receives the user's password and never issues Firebase
tokens. It verifies the Firebase token, maps the Firebase UID to the local
Halaqaty user, and enforces circle roles from PostgreSQL.

The current implementation uses email/password in the Flutter auth screens.
Google and Apple sign-in are product targets documented by the project, but
they require their respective Firebase provider and mobile-platform
configuration before they can be used in a test run. “Username and password”
is not the current contract: email/password is the supported password flow.

## Create a Firebase project for testing

1. Open the [Firebase Console](https://console.firebase.google.com/) with a
   Google account owned by the development team.
2. Create a project named something like `halaqaty-test`.
3. Keep the project on the **Spark (no-cost)** plan. Firebase documents that
   most Authentication options are no-cost and that the Spark plan does not
   require payment information. Review current limits before using phone
   authentication or other paid-tier products.
4. In **Build → Authentication → Sign-in method**, enable **Email/Password**.
5. Register the Android/iOS application in the Firebase project and generate
   the platform configuration files required by FlutterFire (`google-services.json`
   for Android and `GoogleService-Info.plist` for iOS). Do not commit secrets
   or private service-account keys.
6. Configure the backend Firebase Admin credentials through the repository's
   existing server configuration mechanism. Never put the Admin SDK service
   account JSON in the mobile app or in source control.

For the verified local T048 setup, the Firebase project is `halaqaty-test` and
these two test identities were created in Firebase Authentication:

| Test identity | Firebase email | Firebase UID |
|---|---|---|
| Teacher | `teacher.test@example.com` | `InJDagN7cHTGYHWWfOUsq6DGFkA3` |
| Student | `student.test@example.com` | `zIYyvPqiMLPOKkxBwEa4zIPZGPi1` |

The passwords are intentionally not documented. They remain local test
credentials in the ignored `.env` file and should be rotated if they are ever
shared. Firebase Authentication accounts are test identities only; teacher and
student authorization is assigned by Halaqaty circle membership in PostgreSQL.

## Create the Halaqaty backend sessions

After each test user signs in and obtains a Firebase ID token:

1. Call `POST /api/v1/auth/register` for a new Firebase identity. This
   provisions the local Halaqaty user and returns a backend session.
2. For an already-provisioned user, call `POST /api/v1/auth/sessions` with the
   Firebase bearer token. This creates a new current-device backend session.
3. Save the response's local `user.id` and backend `session.id` for the test.

The backend session is an opaque PostgreSQL record used for revocation,
expiry, device ownership, and activity tracking. It is different from:

- the Firebase ID token;
- the Firebase UID;
- the LiveKit room name or media credential; and
- the F-005 live-session ID created by the teacher during T048.

## Verified Docker setup and run process

The following is the process used for the successful T048 run on 2026-09-01:

1. Copy `.env.example` to `.env` and fill local Firebase, database, LiveKit,
   and T048 values. Keep `.env` untracked.
2. Build and start the repository tools and infrastructure:

   ```powershell
   docker compose build flutter-tools
   docker compose up -d postgres flutter-tools
   docker compose --profile media up -d livekit
   ```

3. Apply migrations and start the Go API from `backend/`, using the Firebase
   Admin service account at `.firebase/service-account.json` through
   `GOOGLE_APPLICATION_CREDENTIALS`. This file is ignored and must never be
   copied into the mobile image or committed.
4. Sign in each Firebase test identity to obtain a fresh Firebase ID token,
   then create a Halaqaty backend session with
   `POST /api/v1/auth/sessions`. Put the resulting token, session ID, and local
   user ID into the local `T048_*` environment variables. Tokens expire; do
   not paste them into documentation, logs, or shell history.
5. Run the real-backend Linux integration test inside the tools container:

   ```powershell
   docker compose exec flutter-tools `
     xvfb-run -a flutter test integration_test/queue_late_join_opt_out_test.dart -d linux
   ```

6. The expected result is `All tests passed!`. The test creates and cleans up
   its own circle/session data. It verifies late joining, manager-only events,
   opt-out approval and decline, duplicate-event handling, reconnect snapshot
   recovery, policy changes, round reset, and UI updates.

The Flutter container uses the local image `halaqaty-flutter-tools:local`, built
from `docker/flutter-tools.Dockerfile`. That image is based on the repository's
Flutter CI image and includes Flutter/Dart, Xvfb, Node.js, Firebase CLI, and
FlutterFire CLI. The Compose services are:

| Service | Image | Container | Purpose |
|---|---|---|---|
| `postgres` | `postgres:16-alpine` | `halaqaty-postgres` | Authoritative test database |
| `livekit` | `livekit/livekit-server:v1.8.3` | `halaqaty-livekit` | Local self-hosted media server |
| `flutter-tools` | `halaqaty-flutter-tools:local` | `halaqaty-dev-tools` | Flutter tests and Firebase CLI |

The Flutter tools container mounts the repository at `/workspace`, persists
the Pub cache, and uses `host.docker.internal` to reach the API running on the
Windows host. For a fresh machine, the fallback image
`ghcr.io/cirruslabs/flutter:stable` can run ordinary Flutter commands, but the
project Compose image is preferred for T048 because it also provides Xvfb and
the Firebase tooling.

## Firebase files and CLI configuration

Firebase CLI login authenticates the developer for project configuration; it
does not create an account for each application user. FlutterFire configuration
generates `mobile/lib/firebase_options.dart`. Native Firebase files, when the
platform folders are present, belong at `mobile/android/app/google-services.json`
and `mobile/ios/Runner/GoogleService-Info.plist`.

T048 was run as a Linux integration test with pre-provisioned Firebase ID
tokens, so Android/iOS native files were not required for that verification.
The temporary `mobile/linux/` platform scaffold exists only to provide the
Linux test target and can be regenerated with Flutter in the Docker container.
The Firebase Admin service-account JSON is different from these client config
files: it is server-only, stays under `.firebase/`, and is always ignored.

## Verification record

The T048 closure was checked with fresh Firebase tokens and fresh backend
sessions. The recorded evidence was:

| Check | Result |
|---|---|
| T048 real-backend Linux journey | Passed: `All tests passed!` |
| Flutter unit/widget tests | Passed: 126 tests |
| Linux integration journeys | Passed individually: auth, circles, roles, profile, queue, and T048 |
| Flutter analyze/format for T048 changes | Passed; unrelated informational diagnostics remain elsewhere |
| Go short tests | Passed |
| Go integration tests with PostgreSQL | Passed |
| Go vet and golangci-lint | Passed |
| Secret/diff checks | No secrets in the current diff; `.env`, Firebase config, and service-account files are ignored |

The implementation process followed the feature task order: backend realtime
delivery and queue-round fixes, Flutter event decoding and refresh ordering,
the real-backend T048 journey, then focused quality checks and task evidence.
The completion checkbox for T048 is recorded in
`specs/003-recitation-queue-system/tasks.md`.

## LiveKit for local testing

No LiveKit Cloud account is needed when running the repository's self-hosted
MVP stack. Start the local LiveKit server through the repository's Docker
workflow, or run LiveKit in development mode with the documented development
credentials:

```text
LIVEKIT_ENDPOINT=wss://<reachable-livekit-host>
LIVEKIT_API_KEY=devkey
LIVEKIT_API_SECRET=secret
```

The API key and secret belong only in the Go backend environment. The backend
creates short-lived participant credentials; the Flutter app receives only
the participant connection returned by the authorized backend endpoint.

For a phone or emulator outside the Docker network, the LiveKit and API
endpoints must be reachable from that device and must use the appropriate TLS
configuration. A local `localhost` URL inside a container is not the host
machine.

## T048 environment variables

The integration test reads these values from the environment passed into the
Flutter Docker container:

```text
T048_API_BASE_URL
T048_TEACHER_TOKEN
T048_TEACHER_SESSION
T048_TEACHER_USER_ID
T048_STUDENT_TOKEN
T048_STUDENT_SESSION
T048_STUDENT_USER_ID
```

`T048_TEACHER_TOKEN` and `T048_STUDENT_TOKEN` are fresh Firebase ID tokens
obtained after sign-in. They expire and must be refreshed when necessary.
`T048_*_SESSION` values are the Halaqaty backend session IDs. `T048_*_USER_ID`
values are the local Halaqaty user UUIDs.

Do not commit these values. Pass them only to the local Docker command or a
secret-managed CI job. The test defaults `T048_API_BASE_URL` to
`http://host.docker.internal:8080/api/v1` when running inside Docker.

## Is the free setup valid?

Yes, for this acceptance test, with these boundaries:

- Firebase Spark is sufficient for email/password identity testing.
- Self-hosted LiveKit is sufficient; its cost is the machine/network running
  it, not a LiveKit Cloud subscription.
- PostgreSQL, the Go backend, and the realtime WebSocket must be running.
- A Flutter target capable of launching the integration test is still needed.
- Phone authentication, paid Firebase services, production TLS, and production
  scale are outside this local test setup.

Official references:

- [Firebase Authentication](https://firebase.google.com/docs/auth)
- [Firebase pricing plans](https://firebase.google.com/docs/projects/billing/firebase-pricing-plans)
- [Running LiveKit locally](https://docs.livekit.io/transport/self-hosting/local/)
