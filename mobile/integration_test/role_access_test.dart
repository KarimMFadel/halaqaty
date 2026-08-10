import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/auth/presentation/role_guard.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:integration_test/integration_test.dart';

class _IntegrationAuthNotifier extends StateNotifier<AuthState>
    implements AuthController {
  _IntegrationAuthNotifier({required AuthStatus status})
      : super(AuthState(status: status));

  @override
  Future<void> register({
    required String email,
    required String password,
    required String displayName,
    required String preferredLanguage,
  }) async {}

  @override
  Future<void> signIn({
    required String email,
    required String password,
  }) async {}

  @override
  Future<void> logout() async {
    state = const AuthState(status: AuthStatus.unauthenticated);
  }
}

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets('manager-only control is hidden from student role',
      (tester) async {
    final authNotifier =
        _IntegrationAuthNotifier(status: AuthStatus.authenticated);

    await tester.pumpWidget(
      ProviderScope(
        overrides: [authControllerProvider.overrideWith((_) => authNotifier)],
        child: MaterialApp(
          home: RoleGuard(
            allowedRoles: const {CircleRole.teacher, CircleRole.supervisor},
            currentRole: CircleRole.student,
            unauthorizedChild: const Text('no-access'),
            child: ElevatedButton(
              key: const Key('manageRoleButton'),
              onPressed: () {},
              child: const Text('manage-role'),
            ),
          ),
        ),
      ),
    );

    expect(find.byKey(const Key('manageRoleButton')), findsNothing);
    expect(find.text('no-access'), findsOneWidget);
  });

  testWidgets('manager-only control is visible for teacher role', (tester) async {
    final authNotifier =
        _IntegrationAuthNotifier(status: AuthStatus.authenticated);
    var tapped = false;

    await tester.pumpWidget(
      ProviderScope(
        overrides: [authControllerProvider.overrideWith((_) => authNotifier)],
        child: MaterialApp(
          home: RoleGuard(
            allowedRoles: const {CircleRole.teacher, CircleRole.supervisor},
            currentRole: CircleRole.teacher,
            child: ElevatedButton(
              key: const Key('manageRoleButton'),
              onPressed: () => tapped = true,
              child: const Text('manage-role'),
            ),
          ),
        ),
      ),
    );

    expect(find.byKey(const Key('manageRoleButton')), findsOneWidget);
    await tester.tap(find.byKey(const Key('manageRoleButton')));
    await tester.pumpAndSettle();
    expect(tapped, isTrue);
    expect(isCircleManagerRole(CircleRole.teacher), isTrue);
  });
}
