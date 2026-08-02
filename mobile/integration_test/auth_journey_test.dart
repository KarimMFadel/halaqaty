import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';

import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/auth/presentation/auth_screens.dart';

class _IntegrationAuthNotifier extends StateNotifier<AuthState>
    implements AuthController {
  _IntegrationAuthNotifier()
      : super(const AuthState(status: AuthStatus.unauthenticated));

  bool registerCalled = false;
  bool logoutCalled = false;

  @override
  Future<void> register({
    required String email,
    required String password,
    required String displayName,
    required String preferredLanguage,
  }) async {
    registerCalled = true;
    state = const AuthState(
      status: AuthStatus.authenticated,
      sessionId: 'integration-session',
    );
  }

  @override
  Future<void> signIn({
    required String email,
    required String password,
  }) async {
    state = const AuthState(
      status: AuthStatus.authenticated,
      sessionId: 'integration-session',
    );
  }

  @override
  Future<void> logout() async {
    logoutCalled = true;
    state = const AuthState(status: AuthStatus.unauthenticated);
  }
}

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets('register then logout journey updates auth state',
      (WidgetTester tester) async {
    final notifier = _IntegrationAuthNotifier();
    var registered = false;
    var loggedOut = false;

    await tester.pumpWidget(
      ProviderScope(
        overrides: [authControllerProvider.overrideWith((_) => notifier)],
        child: MaterialApp(
          home: Directionality(
            textDirection: TextDirection.ltr,
            child: RegisterScreen(onSuccess: () => registered = true),
          ),
        ),
      ),
    );

    await tester.enterText(
      find.byKey(const Key('displayNameField')),
      'Ahmad',
    );
    await tester.enterText(find.byKey(const Key('emailField')), 'a@b.com');
    await tester.enterText(
      find.byKey(const Key('passwordField')),
      'password123',
    );

    await tester.tap(find.byKey(const Key('submitButton')));
    await tester.pumpAndSettle();

    expect(notifier.registerCalled, isTrue);
    expect(registered, isTrue);
    expect(notifier.state.isAuthenticated, isTrue);

    await tester.pumpWidget(
      ProviderScope(
        overrides: [authControllerProvider.overrideWith((_) => notifier)],
        child: MaterialApp(
          home: Directionality(
            textDirection: TextDirection.ltr,
            child: LogoutButton(onLoggedOut: () => loggedOut = true),
          ),
        ),
      ),
    );

    await tester.tap(find.byKey(const Key('logoutButton')));
    await tester.pumpAndSettle();

    expect(notifier.logoutCalled, isTrue);
    expect(loggedOut, isTrue);
    expect(notifier.state.status, AuthStatus.unauthenticated);
  });
}
