import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/auth/presentation/auth_screens.dart';

// ---------------------------------------------------------------------------
// Stub StateNotifier — avoids Firebase/Dio in widget tests
// ---------------------------------------------------------------------------

/// A minimal [StateNotifier] that stands in for [AuthController] in widget
/// tests. It records calls and immediately transitions state — no network
/// or platform-plugin calls are made.
class _StubAuthNotifier extends StateNotifier<AuthState>
    implements AuthController {
  _StubAuthNotifier()
      : super(const AuthState(status: AuthStatus.unauthenticated));

  bool registerCalled = false;
  String? lastDisplayName;
  String? lastPreferredLanguage;

  @override
  Future<void> register({
    required String email,
    required String password,
    required String displayName,
    required String preferredLanguage,
  }) async {
    registerCalled = true;
    lastDisplayName = displayName;
    lastPreferredLanguage = preferredLanguage;
    state = const AuthState(
      status: AuthStatus.authenticated,
      sessionId: 'stub-session',
    );
  }

  @override
  Future<void> signIn({
    required String email,
    required String password,
  }) async {
    state = const AuthState(
      status: AuthStatus.authenticated,
      sessionId: 'stub-session',
    );
  }

  @override
  Future<void> logout() async {
    state = const AuthState(status: AuthStatus.unauthenticated);
  }
}

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

Widget _buildRegisterScreen(_StubAuthNotifier stub) {
  return ProviderScope(
    overrides: [
      authControllerProvider.overrideWith((_) => stub),
    ],
    child: const MaterialApp(
      home: RegisterScreen(),
    ),
  );
}

// ---------------------------------------------------------------------------
// Tests: RegisterScreen form validation
// ---------------------------------------------------------------------------

void main() {
  group('RegisterScreen — form validation', () {
    testWidgets('empty display_name shows required-field error',
        (WidgetTester tester) async {
      final stub = _StubAuthNotifier();
      await tester.pumpWidget(_buildRegisterScreen(stub));

      // Leave display-name blank; fill other fields so they do not trigger
      // their own errors and obscure the assertion.
      await tester.enterText(find.byKey(const Key('emailField')), 'a@b.com');
      await tester.enterText(
          find.byKey(const Key('passwordField')), 'password123');

      await tester.tap(find.byKey(const Key('submitButton')));
      await tester.pump();

      expect(find.text('Display name is required'), findsOneWidget);
      expect(stub.registerCalled, isFalse);
    });

    testWidgets(
        'display_name with 1 character shows minimum-length error',
        (WidgetTester tester) async {
      final stub = _StubAuthNotifier();
      await tester.pumpWidget(_buildRegisterScreen(stub));

      await tester.enterText(find.byKey(const Key('displayNameField')), 'X');
      await tester.enterText(find.byKey(const Key('emailField')), 'a@b.com');
      await tester.enterText(
          find.byKey(const Key('passwordField')), 'password123');

      await tester.tap(find.byKey(const Key('submitButton')));
      await tester.pump();

      expect(
        find.text('Display name must be at least 2 characters'),
        findsOneWidget,
      );
      expect(stub.registerCalled, isFalse);
    });

    testWidgets(
        'valid display_name does not show any display-name validation error',
        (WidgetTester tester) async {
      final stub = _StubAuthNotifier();
      await tester.pumpWidget(_buildRegisterScreen(stub));

      await tester.enterText(
          find.byKey(const Key('displayNameField')), 'Ahmad');
      await tester.enterText(find.byKey(const Key('emailField')), 'a@b.com');
      await tester.enterText(
          find.byKey(const Key('passwordField')), 'password123');

      // Validate the form without triggering the async Firebase submit — tap
      // and pump a single frame so synchronous form-validation runs but async
      // network/platform calls have not yet executed.
      await tester.tap(find.byKey(const Key('submitButton')));
      await tester.pump();

      expect(find.text('Display name is required'), findsNothing);
      expect(
        find.text('Display name must be at least 2 characters'),
        findsNothing,
      );
    });
  });
}
