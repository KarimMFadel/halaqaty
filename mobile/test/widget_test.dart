import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:dio/dio.dart';
import 'package:halaqaty_mobile/app/implemented_features_app.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_discovery_controller.dart';
import 'package:halaqaty_mobile/features/circles/application/create_circle_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/auth/presentation/auth_screens.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_discovery_screen.dart';
import 'package:halaqaty_mobile/features/circles/presentation/create_circle_screen.dart';
import 'package:halaqaty_mobile/features/profile/application/profile_controller.dart';
import 'package:halaqaty_mobile/features/profile/data/profile_api_client.dart';
import 'package:halaqaty_mobile/features/profile/presentation/profile_screen.dart';
import 'package:halaqaty_mobile/main.dart';

class _TestAuthController extends StateNotifier<AuthState>
    implements AuthController {
  _TestAuthController(super.initialState);

  @override
  Future<void> logout() async {
    state = const AuthState(status: AuthStatus.unauthenticated);
  }

  void becomeUnauthenticated() {
    state = const AuthState(status: AuthStatus.unauthenticated);
  }

  void becomeAuthenticated(String sessionId) {
    state = AuthState(
      status: AuthStatus.authenticated,
      sessionId: sessionId,
    );
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

class _TestProfileController extends StateNotifier<ProfileState>
    implements ProfileController {
  _TestProfileController() : super(const ProfileState());

  bool isDisposed = false;

  @override
  void dispose() {
    isDisposed = true;
    super.dispose();
  }

  @override
  Future<void> loadProfile() async {}

  @override
  Future<bool> updateProfile({required UpdateProfileRequest request}) async =>
      false;
}

class _TestCircleApiClient extends CircleApiClient {
  _TestCircleApiClient() : super(Dio());

  @override
  Future<List<CircleSummary>> listCircles({
    required String firebaseIdToken,
    required String sessionId,
  }) async =>
      const [];

  @override
  Future<CircleDiscoveryPage> discoverCircles({
    required String firebaseIdToken,
    required String sessionId,
    String? query,
    String? cursor,
  }) async =>
      const CircleDiscoveryPage(circles: []);
}

Future<void> _pumpApp(
  WidgetTester tester,
  _TestAuthController controller,
  {List<_TestProfileController>? profileControllers},
) {
  return tester.pumpWidget(
    ProviderScope(
      overrides: [
        authControllerProvider.overrideWith((_) => controller),
        profileControllerProvider.overrideWith((_) {
          final profileController = _TestProfileController();
          profileControllers?.add(profileController);
          return profileController;
        }),
        circleDiscoveryControllerProvider.overrideWith(
          (_) => CircleDiscoveryController(
            apiClient: _TestCircleApiClient(),
            loadFirebaseIdToken: () async => 'firebase-token',
            readAuthState: () => controller.state,
            logout: controller.logout,
          ),
        ),
        createCircleControllerProvider.overrideWith(
          (_) => CreateCircleController(
            apiClient: _TestCircleApiClient(),
            loadFirebaseIdToken: () async => 'firebase-token',
            readAuthState: () => controller.state,
            logout: controller.logout,
          ),
        ),
      ],
      child: const MyApp(),
    ),
  );
}

void main() {
  testWidgets('shows startup progress while authentication initializes',
      (WidgetTester tester) async {
    await _pumpApp(
      tester,
      _TestAuthController(const AuthState(status: AuthStatus.unknown)),
    );

    expect(find.byKey(const Key('authInitializing')), findsOneWidget);
  });

  testWidgets('shows authentication entry actions when unauthenticated',
      (WidgetTester tester) async {
    await _pumpApp(
      tester,
      _TestAuthController(const AuthState(status: AuthStatus.unauthenticated)),
    );

    expect(find.byKey(const Key('openLogin')), findsOneWidget);
    expect(find.byKey(const Key('openRegister')), findsOneWidget);
  });

  testWidgets('opens the existing login form', (WidgetTester tester) async {
    await _pumpApp(
      tester,
      _TestAuthController(const AuthState(status: AuthStatus.unauthenticated)),
    );

    await tester.tap(find.byKey(const Key('openLogin')));
    await tester.pumpAndSettle();

    expect(find.byType(LoginScreen), findsOneWidget);
  });

  testWidgets('opens the existing registration form',
      (WidgetTester tester) async {
    await _pumpApp(
      tester,
      _TestAuthController(const AuthState(status: AuthStatus.unauthenticated)),
    );

    await tester.tap(find.byKey(const Key('openRegister')));
    await tester.pumpAndSettle();

    expect(find.byType(RegisterScreen), findsOneWidget);
  });

  testWidgets('shows implemented feature entries when authenticated',
      (WidgetTester tester) async {
    await _pumpApp(
      tester,
      _TestAuthController(const AuthState(status: AuthStatus.authenticated)),
    );

    expect(find.byKey(const Key('openProfile')), findsOneWidget);
    expect(find.byKey(const Key('openCircleDiscovery')), findsOneWidget);
    expect(find.byKey(const Key('openCreateCircle')), findsOneWidget);
    expect(find.byKey(const Key('logoutButton')), findsOneWidget);
    expect(find.text('Implemented features'), findsOneWidget);
    expect(find.byIcon(Icons.chevron_right), findsNWidgets(3));
  });

  testWidgets('uses Arabic labels and left-facing chevrons in RTL',
      (WidgetTester tester) async {
    final controller = _TestAuthController(
      const AuthState(status: AuthStatus.authenticated),
    );
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          authControllerProvider.overrideWith((_) => controller),
        ],
        child: const MaterialApp(
          home: Directionality(
            textDirection: TextDirection.rtl,
            child: ImplementedFeaturesScreen(),
          ),
        ),
      ),
    );

    expect(find.text('الميزات المتاحة'), findsOneWidget);
    expect(find.text('الملف الشخصي'), findsOneWidget);
    expect(find.text('اكتشاف الحلقات'), findsOneWidget);
    expect(find.text('إنشاء حلقة'), findsOneWidget);
    expect(find.byIcon(Icons.chevron_left), findsNWidgets(3));
  });

  testWidgets('opens the profile screen', (WidgetTester tester) async {
    await _pumpApp(
      tester,
      _TestAuthController(const AuthState(status: AuthStatus.authenticated)),
    );

    await tester.tap(find.byKey(const Key('openProfile')));
    await tester.pumpAndSettle();

    expect(find.byType(ProfileScreen), findsOneWidget);
  });

  testWidgets('opens the circle discovery screen', (WidgetTester tester) async {
    await _pumpApp(
      tester,
      _TestAuthController(const AuthState(status: AuthStatus.authenticated)),
    );

    await tester.tap(find.byKey(const Key('openCircleDiscovery')));
    await tester.pumpAndSettle();

    expect(find.byType(CircleDiscoveryScreen), findsOneWidget);
  });

  testWidgets('opens the create circle screen', (WidgetTester tester) async {
    await _pumpApp(
      tester,
      _TestAuthController(const AuthState(status: AuthStatus.authenticated)),
    );

    await tester.tap(find.byKey(const Key('openCreateCircle')));
    await tester.pumpAndSettle();

    expect(find.byType(CreateCircleScreen), findsOneWidget);
  });

  testWidgets('returns to authentication entry after logout',
      (WidgetTester tester) async {
    final controller = _TestAuthController(
      const AuthState(status: AuthStatus.authenticated),
    );
    await _pumpApp(tester, controller);

    await tester.tap(find.byKey(const Key('logoutButton')));
    await tester.pump();

    expect(find.byKey(const Key('openLogin')), findsOneWidget);
  });

  testWidgets('auth loss clears the protected route stack',
      (WidgetTester tester) async {
    final controller = _TestAuthController(
      const AuthState(status: AuthStatus.authenticated),
    );
    await _pumpApp(tester, controller);

    await tester.tap(find.byKey(const Key('openProfile')));
    await tester.pumpAndSettle();
    expect(find.byType(ProfileScreen), findsOneWidget);

    controller.becomeUnauthenticated();
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('openLogin')), findsOneWidget);
    expect(find.byType(ProfileScreen), findsNothing);
  });

  testWidgets('system back pops the protected route stack',
      (WidgetTester tester) async {
    await _pumpApp(
      tester,
      _TestAuthController(const AuthState(status: AuthStatus.authenticated)),
    );

    await tester.tap(find.byKey(const Key('openProfile')));
    await tester.pumpAndSettle();
    expect(find.byType(ProfileScreen), findsOneWidget);

    await tester.binding.handlePopRoute();
    await tester.pumpAndSettle();

    expect(find.text('Implemented features'), findsOneWidget);
    expect(find.byType(ProfileScreen), findsNothing);
  });

  testWidgets('auth transition resets profile state for the next session',
      (WidgetTester tester) async {
    final profileControllers = <_TestProfileController>[];
    final controller = _TestAuthController(
      const AuthState(
        status: AuthStatus.authenticated,
        sessionId: 'session-a',
      ),
    );
    await _pumpApp(
      tester,
      controller,
      profileControllers: profileControllers,
    );

    await tester.tap(find.byKey(const Key('openProfile')));
    await tester.pumpAndSettle();
    final profileControllerForSessionA = profileControllers.single;

    controller.becomeUnauthenticated();
    await tester.pumpAndSettle();

    expect(profileControllerForSessionA.isDisposed, isTrue);

    controller.becomeAuthenticated('session-b');
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('openProfile')));
    await tester.pumpAndSettle();

    expect(profileControllers, hasLength(2));
    expect(profileControllers.last, isNot(same(profileControllerForSessionA)));
  });
}
