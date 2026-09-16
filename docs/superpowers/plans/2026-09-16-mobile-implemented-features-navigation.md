# Mobile Implemented Features Navigation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the Flutter counter demo with an authentication-aware entry point that opens already implemented Halaqaty screens using real runtime data.

**Architecture:** Wrap `MyApp` in Riverpod at startup and select a small root page from the existing `AuthStatus`. Keep the welcome and implemented-features UI in `mobile/lib/app/implemented_features_app.dart`, reuse the existing auth and feature screens, and use the existing imperative `Navigator` pattern.

**Tech Stack:** Flutter 3.47.4, Dart 3.13.3, Riverpod 2.x, Firebase Auth, Flutter `Navigator`.

**Spec:** `docs/superpowers/specs/2026-09-16-mobile-implemented-features-navigation-design.md`

## Global Constraints

- No new dependencies.
- No backend, database, REST contract, or WebSocket contract changes.
- Use existing `AuthController`, `LoginScreen`, `RegisterScreen`, `LogoutButton`, `ProfileScreen`, `CircleDiscoveryScreen`, and `CreateCircleScreen`.
- Do not invent circle, session, or peer identifiers.
- Do not add `go_router`, a dashboard, deep links, or speculative routing abstractions.
- Preserve Arabic and English labels and RTL-aware navigation icons.
- Preserve unrelated worktree changes.
- Follow TDD: obtain a focused RED result before production implementation, then GREEN evidence.

---

### Task 1: Replace the counter demo with the implemented-features path

**Files:**
- Create: `mobile/lib/app/implemented_features_app.dart`
- Modify: `mobile/lib/main.dart`
- Modify: `mobile/test/widget_test.dart`

**Interfaces:**
- Consumes: `authControllerProvider` and `AuthStatus` from `features/auth/application/auth_controller.dart`.
- Consumes: existing screen constructors `LoginScreen({VoidCallback? onSuccess})`, `RegisterScreen({VoidCallback? onSuccess})`, `LogoutButton({VoidCallback? onLoggedOut})`, `ProfileScreen()`, `CircleDiscoveryScreen()`, and `CreateCircleScreen()`.
- Produces: `ImplementedFeaturesRoot`, the auth-aware `MaterialApp.home` widget; `AuthWelcomeScreen`; and `ImplementedFeaturesScreen`.

- [ ] **Step 1: Replace the generated counter test with failing root-navigation tests**

  In `mobile/test/widget_test.dart`, create a local `StateNotifier<AuthState>` test controller implementing the existing `AuthController` methods. Pump `ProviderScope(overrides: [authControllerProvider.overrideWith((_) => controller)], child: const MyApp())` and add tests that assert:

  ```dart
  expect(find.byKey(const Key('authInitializing')), findsOneWidget);
  expect(find.byKey(const Key('openLogin')), findsOneWidget);
  expect(find.byKey(const Key('openRegister')), findsOneWidget);
  expect(find.byKey(const Key('openProfile')), findsOneWidget);
  expect(find.byKey(const Key('openCircleDiscovery')), findsOneWidget);
  expect(find.byKey(const Key('openCreateCircle')), findsOneWidget);
  expect(find.byKey(const Key('logoutButton')), findsOneWidget);
  ```

  Verify Sign in and Register taps show the existing screen types. Verify authenticated feature-entry taps show `ProfileScreen`, `CircleDiscoveryScreen`, and `CreateCircleScreen`. Verify tapping Logout causes the test controller to emit `AuthStatus.unauthenticated` and reveals `openLogin` again.

- [ ] **Step 2: Run the focused test and capture RED evidence**

  Run from `mobile/`:

  ```powershell
  flutter test test/widget_test.dart
  ```

  Expected: FAIL because `ImplementedFeaturesRoot` and the required navigation keys do not exist and `MyApp` still renders the counter demo.

- [ ] **Step 3: Create the minimal auth-aware root and feature menu**

  In `mobile/lib/app/implemented_features_app.dart`, implement:

  ```dart
  class ImplementedFeaturesRoot extends ConsumerWidget {
    const ImplementedFeaturesRoot({super.key});

    @override
    Widget build(BuildContext context, WidgetRef ref) {
      return switch (ref.watch(authControllerProvider).status) {
        AuthStatus.unknown => const Center(
            child: CircularProgressIndicator(key: Key('authInitializing')),
          ),
        AuthStatus.unauthenticated => const AuthWelcomeScreen(),
        AuthStatus.authenticated => const ImplementedFeaturesScreen(),
      };
    }
  }
  ```

  `AuthWelcomeScreen` is a `Scaffold` with Halaqaty title text and buttons keyed `openLogin` and `openRegister`. Push the existing auth screen with `MaterialPageRoute`; pass `onSuccess: () => Navigator.of(context).pop()`.

  `ImplementedFeaturesScreen` is a `Scaffold` with three `ListTile` entries keyed `openProfile`, `openCircleDiscovery`, and `openCreateCircle`. Each pushes the matching existing screen through `MaterialPageRoute<void>`. Include the existing `LogoutButton` without wrapping or duplicating logout logic.

  Use English/Arabic text based on `Directionality.of(context) == TextDirection.rtl`, and use left/right chevrons appropriate to the current direction.

- [ ] **Step 4: Replace the counter entry point**

  In `mobile/lib/main.dart`, preserve Firebase initialization and change startup to:

  ```dart
  runApp(const ProviderScope(child: MyApp()));
  ```

  Keep `MyApp` as the public testable root. Its `MaterialApp` must use title `Halaqaty`, preserve the current seed-color theme, and set:

  ```dart
  home: const ImplementedFeaturesRoot(),
  ```

  Delete `MyHomePage`, `_MyHomePageState`, the counter, and generated template comments.

- [ ] **Step 5: Run the focused test and capture GREEN evidence**

  Run from `mobile/`:

  ```powershell
  flutter test test/widget_test.dart
  ```

  Expected: all root-navigation tests pass with no warnings.

- [ ] **Step 6: Run Flutter verification gates**

  Run from `mobile/`:

  ```powershell
  flutter test test
  flutter analyze
  dart format --set-exit-if-changed lib test
  ```

  Expected: tests pass, analyzer reports no issues, formatter exits 0 without modifying files on its second run. Do not claim the device integration gate from these commands.

- [ ] **Step 7: Verify the real emulator path**

  With the local Go API healthy on port 8080 and `emulator-5554` connected, run from `mobile/`:

  ```powershell
  flutter run -d emulator-5554 --dart-define=API_BASE_URL=http://10.0.2.2:8080/api/v1
  ```

  Expected: the emulator shows the Halaqaty welcome screen rather than `Flutter Demo Home Page`; Sign in opens the existing login form; after successful authentication the Implemented Features page opens Profile, Discover circles, and Create circle. If the Codex sandbox cannot create Gradle's internal loopback socket, report the device launch as blocked and keep successful host/IDE evidence separate.

- [ ] **Step 8: Review the diff**

  Confirm `git diff --check` passes, the generated counter code is gone, no dependency changed, and unrelated modifications remain untouched.
