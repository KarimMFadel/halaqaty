import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';
import 'package:halaqaty_mobile/app/app_locale_controller.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_discovery_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/profile/application/profile_controller.dart';
import 'package:halaqaty_mobile/features/profile/data/profile_api_client.dart';
import 'package:halaqaty_mobile/main.dart';

/// Wave 0 (US1) visual evidence harness: boots the real `MyApp` shell with
/// faked auth/profile/circle providers and captures the splash, welcome,
/// Home states, and Chats tab in Arabic RTL / English LTR × light / dark.
class _Wave0AuthController extends StateNotifier<AuthState>
    implements AuthController {
  _Wave0AuthController(super.initialState);

  @override
  Future<void> logout() async {
    state = const AuthState(status: AuthStatus.unauthenticated);
  }

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

class _Wave0ProfileController extends StateNotifier<ProfileState>
    implements ProfileController {
  _Wave0ProfileController() : super(const ProfileState());

  @override
  Future<void> loadProfile() async {
    state = ProfileState(
      profile: ProfileUser(
        id: 'visual-user',
        firebaseUid: 'visual-firebase',
        fullName: 'Karim Fadel',
        displayName: 'Karim',
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

class _Wave0CircleApi extends CircleApiClient {
  _Wave0CircleApi() : super(Dio());

  static final circle = CircleSummary(
    id: 'visual-circle',
    name: 'حلقة الإتقان لاختبار العرض',
    description: 'Visual regression fixture',
    maxCapacity: 20,
    genderRestriction: 'mixed',
    language: 'ar',
    createdAt: DateTime.utc(2026, 1, 1),
  );

  List<CircleSummary> circles = const [];
  DioException? listCirclesError;

  @override
  Future<List<CircleSummary>> listCircles({
    required String firebaseIdToken,
    required String sessionId,
  }) async {
    // Keep failing while set: the shell issues overlapping loads per mount,
    // and a one-shot throw makes the final state racy.
    if (listCirclesError case final error?) {
      throw error;
    }
    return circles;
  }

  @override
  Future<CircleDiscoveryPage> discoverCircles({
    required String firebaseIdToken,
    required String sessionId,
    String? query,
    String? cursor,
  }) async =>
      const CircleDiscoveryPage(circles: []);
}

/// Pumps a fresh `MyApp` shell, detaching the old tree first: reparenting
/// the global `_homeNavigatorKey` into a second live GoRouter strands
/// navigation on the splash route.
Future<void> _pumpWave0App(
  WidgetTester tester, {
  required AuthState auth,
  required Locale platformLocale,
  required Brightness brightness,
  required _Wave0CircleApi circleApi,
}) async {
  await tester.pumpWidget(const SizedBox.shrink());
  await tester.pump();
  return tester.pumpWidget(
    MediaQuery(
      data: MediaQueryData(platformBrightness: brightness),
      child: ProviderScope(
        overrides: [
          platformLocaleProvider.overrideWithValue(platformLocale),
          authControllerProvider
              .overrideWith((_) => _Wave0AuthController(auth)),
          profileControllerProvider
              .overrideWith((_) => _Wave0ProfileController()),
          circleDiscoveryControllerProvider.overrideWith(
            (_) => CircleDiscoveryController(
              apiClient: circleApi,
              loadFirebaseIdToken: () async => 'visual-token',
              readAuthState: () => auth,
              logout: () async {},
            ),
          ),
        ],
        child: const MyApp(),
      ),
    ),
  );
}

void main() {
  final binding = IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  const authenticated = AuthState(
    status: AuthStatus.authenticated,
    sessionId: 'visual-session',
  );

  /// Pumps until [finder] stays visible for 3 consecutive frames: each
  /// `loadMyCircles` briefly clears `failure` at start, so a one-shot
  /// sentinel can capture a transient frame.
  Future<void> pumpUntilShown(
    WidgetTester tester,
    Finder sentinel, {
    String? debugName,
  }) async {
    var stableFrames = 0;
    for (var i = 0; i < 60; i++) {
      await tester.pump(const Duration(milliseconds: 200));
      if (sentinel.evaluate().isNotEmpty) {
        stableFrames++;
        if (stableFrames >= 3) {
          return;
        }
      } else {
        stableFrames = 0;
      }
    }
    if (debugName != null) {
      await binding.takeScreenshot('debug_$debugName');
    }
    throw StateError('Sentinel never appeared: $sentinel');
  }

  Future<void> captureMatrix(
    WidgetTester tester,
    IntegrationTestWidgetsFlutterBinding binding,
    Locale locale,
  ) async {
    final lang = locale.languageCode;
    for (final brightness in Brightness.values) {
      final theme = brightness == Brightness.light ? 'light' : 'dark';

      // Splash: authentication still initializing.
      await _pumpWave0App(
        tester,
        auth: const AuthState(status: AuthStatus.unknown),
        platformLocale: locale,
        brightness: brightness,
        circleApi: _Wave0CircleApi(),
      );
      await pumpUntilShown(tester, find.byKey(const Key('authInitializing')));
      await binding.takeScreenshot('wave0_splash_${lang}_$theme');

      // Welcome: unauthenticated entry.
      await _pumpWave0App(
        tester,
        auth: const AuthState(status: AuthStatus.unauthenticated),
        platformLocale: locale,
        brightness: brightness,
        circleApi: _Wave0CircleApi(),
      );
      await pumpUntilShown(
        tester,
        find.byKey(const Key('openLogin')),
        debugName: 'welcome_${lang}_$theme',
      );
      await binding.takeScreenshot('wave0_welcome_${lang}_$theme');

      // Home loaded with one circle.
      await _pumpWave0App(
        tester,
        auth: authenticated,
        platformLocale: locale,
        brightness: brightness,
        circleApi: _Wave0CircleApi()..circles = [_Wave0CircleApi.circle],
      );
      await pumpUntilShown(
        tester,
        find.byKey(const Key('homeCircle-visual-circle')),
      );
      await binding.takeScreenshot('wave0_home_loaded_${lang}_$theme');

      // Home empty after a successful load.
      await _pumpWave0App(
        tester,
        auth: authenticated,
        platformLocale: locale,
        brightness: brightness,
        circleApi: _Wave0CircleApi(),
      );
      await pumpUntilShown(tester, find.byKey(const Key('homeNoCircles')));
      await binding.takeScreenshot('wave0_home_empty_${lang}_$theme');

      // Home recoverable error (network failure, nothing cached).
      await _pumpWave0App(
        tester,
        auth: authenticated,
        platformLocale: locale,
        brightness: brightness,
        circleApi: _Wave0CircleApi()
          ..listCirclesError = DioException(
            requestOptions: RequestOptions(path: '/circles'),
            type: DioExceptionType.connectionError,
          ),
      );
      await pumpUntilShown(tester, find.byKey(const Key('circleLoadError')));
      await binding.takeScreenshot('wave0_home_error_${lang}_$theme');

      // Chats tab empty state inside the shell. The unselected destination
      // icon is a stable, language-independent tap target for this harness.
      await _pumpWave0App(
        tester,
        auth: authenticated,
        platformLocale: locale,
        brightness: brightness,
        circleApi: _Wave0CircleApi(),
      );
      await pumpUntilShown(tester, find.byIcon(Icons.forum_outlined));
      await tester.tap(find.byIcon(Icons.forum_outlined));
      await pumpUntilShown(tester, find.byKey(const Key('chatsEmpty')));
      await binding.takeScreenshot('wave0_chats_empty_${lang}_$theme');
    }
  }

  for (final locale in const [Locale('ar'), Locale('en')]) {
    testWidgets('wave0_shell_matrix_${locale.languageCode}', (tester) async {
      await binding.convertFlutterSurfaceToImage();
      await captureMatrix(tester, binding, locale);
    });
  }
}
