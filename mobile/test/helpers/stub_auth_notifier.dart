import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';

/// A minimal [StateNotifier] that stands in for [AuthController] in widget
/// tests: no Firebase, Dio, or platform-plugin calls are made. Widgets that
/// fall back to `authControllerProvider` for the current user can be tested
/// without initializing Firebase.
class StubAuthNotifier extends StateNotifier<AuthState>
    implements AuthController {
  StubAuthNotifier({AuthState? initialState})
      : super(initialState ?? const AuthState(status: AuthStatus.unauthenticated));

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
  Future<void> logout() async {}
}
