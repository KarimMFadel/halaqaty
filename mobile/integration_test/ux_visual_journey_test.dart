import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';
import 'package:halaqaty_mobile/app/app_locale_controller.dart';
import 'package:halaqaty_mobile/app/home_screen.dart';
import 'package:halaqaty_mobile/core/theme/halaqaty_theme.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_discovery_controller.dart';
import 'package:halaqaty_mobile/features/circles/application/create_circle_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_discovery_screen.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_load_error.dart';
import 'package:halaqaty_mobile/features/profile/application/profile_controller.dart';
import 'package:halaqaty_mobile/features/profile/data/profile_api_client.dart';
import 'package:halaqaty_mobile/features/profile/presentation/profile_screen.dart';
import 'package:halaqaty_mobile/main.dart';

/// Wave 5 (T043) on-device UX visual journey: the Screenshot Acceptance
/// Matrix direction/theme ready-state variants for the shell-owned surfaces.
///
/// Every capture runs in Arabic RTL and English LTR, light and dark:
/// standalone home/discovery/profile/error surfaces, the authenticated shell
/// (bottom navigation visible), and the shell's Circles and Chats
/// destinations. Width (320dp/600dp) and 200% text-scale captures are owned
/// by the off-device harness `mobile/tool/wave5_visual_capture_test.dart`
/// while the emulator is shared.

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

CircleDiscoveryController _visualCircleController() =>
    CircleDiscoveryController(
      apiClient: _VisualCircleApi(),
      loadFirebaseIdToken: () async => 'visual-token',
      readAuthState: () => const AuthState(sessionId: 'visual-session'),
      logout: () async {},
    );

List<Override> _visualOverrides() => [
      authControllerProvider.overrideWith((_) => _VisualAuthController()),
      profileControllerProvider.overrideWith((_) => _VisualProfileController()),
      circleDiscoveryControllerProvider
          .overrideWith((_) => _visualCircleController()),
    ];

/// Standalone surface scope. When [brightness] is given the approved theme is
/// applied explicitly; otherwise the surface renders with default themes.
Widget _scope(TextDirection direction, Widget child, {Brightness? brightness}) {
  return ProviderScope(
    overrides: _visualOverrides(),
    child: MaterialApp(
      theme: brightness == null ? null : halaqatyLightTheme(),
      darkTheme: brightness == null ? null : halaqatyDarkTheme(),
      themeMode: switch (brightness) {
        Brightness.dark => ThemeMode.dark,
        Brightness.light => ThemeMode.light,
        null => null,
      },
      home: Directionality(textDirection: direction, child: child),
    ),
  );
}

/// Authenticated shell scope: the real [MyApp] router so the bottom
/// navigation bar is visible. Direction follows the overridden platform
/// locale; brightness follows the platform dispatcher (system theme mode).
Widget _shellScope(Locale locale) {
  final circleApi = _VisualCircleApi();
  return ProviderScope(
    overrides: [
      platformLocaleProvider.overrideWithValue(locale),
      authControllerProvider.overrideWith((_) => _VisualAuthController()),
      profileControllerProvider.overrideWith((_) => _VisualProfileController()),
      circleDiscoveryControllerProvider
          .overrideWith((_) => _visualCircleController()),
      createCircleControllerProvider.overrideWith(
        (_) => CreateCircleController(
          apiClient: circleApi,
          loadFirebaseIdToken: () async => 'visual-token',
          readAuthState: () => const AuthState(sessionId: 'visual-session'),
          logout: () async {},
        ),
      ),
    ],
    child: const MyApp(),
  );
}

/// Taps a shell destination by index; the destination widget is tapped rather
/// than its label because NavigationBar renders label text in two subtrees.
Future<void> _openShellDestination(WidgetTester tester, int index) async {
  await tester.tap(
    find
        .descendant(
          of: find.byKey(const Key('appNavigationBar')),
          matching: find.byType(NavigationDestination),
        )
        .at(index),
  );
  await tester.pumpAndSettle();
}

void main() {
  final binding = IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets('captures standalone surfaces in every direction/theme variant',
      (tester) async {
    await binding.convertFlutterSurfaceToImage();
    for (final direction in TextDirection.values) {
      final directionSuffix = direction == TextDirection.rtl ? 'rtl' : 'ltr';
      for (final brightness in Brightness.values) {
        final themeSuffix = brightness == Brightness.dark ? 'dark' : 'light';
        final suffix = '${directionSuffix}_$themeSuffix';

        await tester.pumpWidget(
          _scope(direction, const HomeScreen(), brightness: brightness),
        );
        await tester.pumpAndSettle();
        await binding.takeScreenshot('ux_home_$suffix');

        await tester.pumpWidget(
          _scope(direction, const CircleDiscoveryScreen(),
              brightness: brightness),
        );
        await tester.pumpAndSettle();
        await binding.takeScreenshot('ux_circles_$suffix');

        await tester.pumpWidget(
          _scope(direction, const ProfileScreen(), brightness: brightness),
        );
        await tester.pumpAndSettle();
        await binding.takeScreenshot('ux_profile_$suffix');

        await tester.pumpWidget(
          _scope(
            direction,
            CircleLoadError(
              failure: CircleJoinFailure.network,
              onRetry: () {},
            ),
            brightness: brightness,
          ),
        );
        await tester.pumpAndSettle();
        await binding.takeScreenshot('ux_error_$suffix');
      }
    }
  });

  testWidgets('captures the authenticated shell journeys in every variant',
      (tester) async {
    await binding.convertFlutterSurfaceToImage();
    for (final direction in TextDirection.values) {
      final rtl = direction == TextDirection.rtl;
      final directionSuffix = rtl ? 'rtl' : 'ltr';
      for (final brightness in Brightness.values) {
        final themeSuffix = brightness == Brightness.dark ? 'dark' : 'light';
        final suffix = '${directionSuffix}_$themeSuffix';
        tester.platformDispatcher.platformBrightnessTestValue = brightness;
        try {
          await tester.pumpWidget(
            _shellScope(rtl ? const Locale('ar') : const Locale('en')),
          );
          await tester.pumpAndSettle();
          await binding.takeScreenshot('ux_shell_home_$suffix');

          await _openShellDestination(tester, 1);
          await binding.takeScreenshot('ux_shell_circles_$suffix');

          await _openShellDestination(tester, 2);
          await binding.takeScreenshot('ux_shell_chats_$suffix');

          await _openShellDestination(tester, 3);
          await binding.takeScreenshot('ux_shell_profile_$suffix');
        } finally {
          tester.platformDispatcher.clearPlatformBrightnessTestValue();
        }
        await tester.pumpWidget(const SizedBox.shrink());
        await tester.pump();
      }
    }
  });
}
