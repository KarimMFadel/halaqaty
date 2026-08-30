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

For T048, create two ordinary test users through the app's registration screen
or Firebase Authentication's user-management page:

```text
teacher.t048@example.test
student.t048@example.test
```

Use addresses in a domain you control or two real test addresses. Use strong,
unique test passwords and store them in a local password manager. Do not put
the passwords in the repository or in the T048 environment variables.

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
