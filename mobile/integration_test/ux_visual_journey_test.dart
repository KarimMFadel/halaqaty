import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';
import 'package:halaqaty_mobile/app/home_screen.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_discovery_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_discovery_screen.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_load_error.dart';
import 'package:halaqaty_mobile/features/profile/application/profile_controller.dart';
import 'package:halaqaty_mobile/features/profile/data/profile_api_client.dart';
import 'package:halaqaty_mobile/features/profile/presentation/profile_screen.dart';

class _VisualCircleApi extends CircleApiClient {
  _VisualCircleApi() : super(Dio());

  static final circle = CircleSummary(
    id: 'visual-circle',
    name: 'حلقة طويلة لاختبار عرض الاسم في الواجهات المختلفة',
    description: 'Visual regression fixture',
    maxCapacity: 20,
    genderRestriction: 'mixed',
    language: 'ar',
    createdAt: DateTime.utc(2026, 1, 1),
  );

  @override
  Future<List<CircleSummary>> listCircles({
    required String firebaseIdToken,
    required String sessionId,
  }) async =>
      [circle];

  @override
  Future<CircleDiscoveryPage> discoverCircles({
    required String firebaseIdToken,
    required String sessionId,
    String? query,
    String? cursor,
  }) async =>
      CircleDiscoveryPage(circles: [circle]);
}

class _VisualProfileController extends StateNotifier<ProfileState>
    implements ProfileController {
  _VisualProfileController() : super(const ProfileState());

  @override
  Future<void> loadProfile() async {
    state = ProfileState(
      profile: ProfileUser(
        id: 'visual-user',
        firebaseUid: 'visual-firebase',
        fullName: 'Ali Mahmoud',
        displayName: 'Ali',
        bio: null,
        country: 'EG',
        preferredLanguage: 'ar',
        avatarUrl: null,
        phone: null,
        createdAt: DateTime.utc(2026, 1, 1),
      ),
    );
  }

  @override
  Future<bool> updateProfile({required UpdateProfileRequest request}) async =>
      true;
}

class _VisualAuthController extends StateNotifier<AuthState>
    implements AuthController {
  _VisualAuthController()
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

Widget _scope(TextDirection direction, Widget child) {
  final circleController = CircleDiscoveryController(
    apiClient: _VisualCircleApi(),
    loadFirebaseIdToken: () async => 'visual-token',
    readAuthState: () => const AuthState(sessionId: 'visual-session'),
    logout: () async {},
  );
  return ProviderScope(
    overrides: [
      authControllerProvider.overrideWith((_) => _VisualAuthController()),
      profileControllerProvider.overrideWith((_) => _VisualProfileController()),
      circleDiscoveryControllerProvider.overrideWith((_) => circleController),
    ],
    child: MaterialApp(
      home: Directionality(textDirection: direction, child: child),
    ),
  );
}

void main() {
  final binding = IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets('captures RTL and LTR UX journeys', (tester) async {
    for (final direction in TextDirection.values) {
      final suffix = direction == TextDirection.rtl ? 'rtl' : 'ltr';

      await tester.pumpWidget(_scope(direction, const HomeScreen()));
      await tester.pumpAndSettle();
      await binding.takeScreenshot('ux_home_$suffix');

      await tester.pumpWidget(
        _scope(direction, const CircleDiscoveryScreen()),
      );
      await tester.pumpAndSettle();
      await binding.takeScreenshot('ux_circles_$suffix');

      await tester.pumpWidget(_scope(direction, const ProfileScreen()));
      await tester.pumpAndSettle();
      await binding.takeScreenshot('ux_profile_$suffix');

      await tester.pumpWidget(
        _scope(
          direction,
          CircleLoadError(
            failure: CircleJoinFailure.network,
            onRetry: () {},
          ),
        ),
      );
      await tester.pumpAndSettle();
      await binding.takeScreenshot('ux_error_$suffix');
    }
  });
}
