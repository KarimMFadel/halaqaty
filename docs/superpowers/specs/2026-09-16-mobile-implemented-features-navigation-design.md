# Mobile Implemented Features Navigation Design

## Goal

Replace Flutter's counter demo with the smallest production path that lets a developer authenticate and open the Halaqaty screens that already have valid runtime inputs.

## Scope

The root application watches the existing `authControllerProvider`:

- `AuthStatus.unknown` shows a centered progress indicator.
- `AuthStatus.unauthenticated` shows a small Halaqaty welcome page with Sign in and Register actions.
- `AuthStatus.authenticated` shows an Implemented Features page with Profile, Discover circles, Create circle, and Logout actions.

Sign in and Register reuse the existing `LoginScreen` and `RegisterScreen`. Their success callbacks pop the form route; the root auth watcher then displays the authenticated page. Logout reuses `LogoutButton`; the auth watcher returns the user to the welcome page.

The authenticated page opens only screens that can obtain their own real data:

- `ProfileScreen`
- `CircleDiscoveryScreen`
- `CreateCircleScreen`

Circle discovery already leads to circle detail, members, management, retirement, invite joining, and group chat using real circle IDs and current authorization. Session rooms and direct chats are not linked from the preview page because they require a real session ID or peer ID supplied by their owning flows.

## Structure

Keep `main.dart` responsible only for Firebase initialization, `ProviderScope`, `MaterialApp`, and selecting the auth-aware root. Put the welcome and implemented-features pages in one focused presentation file under `mobile/lib/app/` so the entry point does not become another large UI file.

Use Flutter's existing `Navigator` and `MaterialPageRoute`. Do not add `go_router`, routing tables, a dashboard, deep links, fake identifiers, or new backend behavior.

## Error and State Behavior

Existing feature screens continue to render their existing loading and error states. The root handles only authentication state. Firebase/backend errors remain owned by the existing auth screens and controllers.

## Verification

Replace the generated counter smoke test with root-navigation widget tests using an overridden auth controller:

- unknown auth state shows startup progress;
- unauthenticated state shows Sign in and Register;
- authenticated state shows the three implemented feature entries and Logout;
- Sign in and Register open the existing form screens;
- the three feature entries open their real screen types;
- logout transitions the root back to the unauthenticated page.

Run the focused root test, the full Flutter unit/widget suite, analyzer, formatter, and then launch on `emulator-5554` against `http://10.0.2.2:8080/api/v1`.

## Constraints

- No new dependencies.
- No backend or contract changes.
- No fake circle, session, or peer identifiers.
- Preserve Arabic and English labels and RTL behavior.
- Preserve unrelated worktree changes.
- This is a developer-accessible bridge to implemented functionality, not the future F-010 dashboard.
