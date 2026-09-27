import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';
import 'package:halaqaty_mobile/app/app_locale_controller.dart';
import 'package:halaqaty_mobile/core/theme/halaqaty_theme.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/profile/application/profile_controller.dart';
import 'package:halaqaty_mobile/features/profile/data/profile_api_client.dart';
import 'package:halaqaty_mobile/features/profile/presentation/profile_screen.dart';

class _ProfileController extends StateNotifier<ProfileState>
    implements ProfileController {
  _ProfileController() : super(const ProfileState());

  @override
  Future<void> loadProfile() async {
    state = ProfileState(
      profile: ProfileUser(
        id: 'visual-user',
        firebaseUid: 'visual-firebase',
        displayName: 'كريم',
        preferredLanguage: 'ar',
        createdAt: DateTime.utc(2026, 1, 1),
      ),
    );
  }

  @override
  Future<bool> updateProfile({required UpdateProfileRequest request}) async =>
      true;

  @override
  Future<AccountDeletionOutcome?> deleteAccount(
          {required String password}) async =>
      null;
}

class _AuthController extends StateNotifier<AuthState>
    implements AuthController {
  _AuthController()
      : super(const AuthState(
          status: AuthStatus.authenticated,
          sessionId: 'visual-session',
        ));

  @override
  Future<void> logout() async {}

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
}

void main() {
  final binding = IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets('captures the Arabic account settings and deletion action',
      (tester) async {
    await binding.convertFlutterSurfaceToImage();
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          platformLocaleProvider.overrideWithValue(const Locale('ar')),
          authControllerProvider.overrideWith((_) => _AuthController()),
          profileControllerProvider.overrideWith((_) => _ProfileController()),
        ],
        child: MaterialApp(
          theme: halaqatyLightTheme(),
          home: const Directionality(
            textDirection: TextDirection.rtl,
            child: ProfileScreen(),
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();
    final deleteAction = find.byKey(const Key('deleteAccountTile'));
    await tester.ensureVisible(deleteAction);
    await tester.pumpAndSettle();

    expect(find.text('حذف الحساب'), findsOneWidget);
    await binding.takeScreenshot('f001_account_deletion_rtl');
    await tester.tap(deleteAction);
    await tester.pumpAndSettle();
    expect(find.text('حذف الحساب نهائياً؟'), findsOneWidget);
    await binding.takeScreenshot('f001_account_deletion_confirm_rtl');
  });
}
