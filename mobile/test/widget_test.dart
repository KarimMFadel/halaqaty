import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:dio/dio.dart';
import 'package:halaqaty_mobile/app/chats_screen.dart';
import 'package:halaqaty_mobile/app/home_screen.dart';
import 'package:halaqaty_mobile/app/router.dart';
import 'package:halaqaty_mobile/app/welcome_screen.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_discovery_controller.dart';
import 'package:halaqaty_mobile/features/circles/application/create_circle_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/auth/presentation/auth_screens.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_discovery_screen.dart';
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
  _TestAuthController controller, {
  List<_TestProfileController>? profileControllers,
}) {
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
  testWidgets('shows branded splash while authentication initializes',
      (WidgetTester tester) async {
    await _pumpApp(
      tester,
      _TestAuthController(const AuthState(status: AuthStatus.unknown)),
    );

    expect(find.byKey(const Key('authInitializing')), findsOneWidget);
    expect(find.byType(HalaqatySplash), findsOneWidget);
  });

  testWidgets('shows welcome actions when unauthenticated',
      (WidgetTester tester) async {
    await _pumpApp(
      tester,
      _TestAuthController(const AuthState(status: AuthStatus.unauthenticated)),
    );

    expect(find.byType(WelcomeScreen), findsOneWidget);
    expect(find.byKey(const Key('openLogin')), findsOneWidget);
    expect(find.byKey(const Key('openRegister')), findsOneWidget);
  });

  testWidgets('opens the login form', (WidgetTester tester) async {
    await _pumpApp(
      tester,
      _TestAuthController(const AuthState(status: AuthStatus.unauthenticated)),
    );

    await tester.tap(find.byKey(const Key('openLogin')));
    await tester.pumpAndSettle();

    expect(find.byType(LoginScreen), findsOneWidget);
  });

  testWidgets('opens the registration form', (WidgetTester tester) async {
    await _pumpApp(
      tester,
      _TestAuthController(const AuthState(status: AuthStatus.unauthenticated)),
    );

    await tester.tap(find.byKey(const Key('openRegister')));
    await tester.pumpAndSettle();

    expect(find.byType(RegisterScreen), findsOneWidget);
  });

  testWidgets('shows the four-tab shell when authenticated',
      (WidgetTester tester) async {
    await _pumpApp(
      tester,
      _TestAuthController(const AuthState(status: AuthStatus.authenticated)),
    );
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('appNavigationBar')), findsOneWidget);
    expect(find.byType(HomeScreen), findsOneWidget);
    expect(find.byKey(const Key('homeNoCircles')), findsOneWidget);
    expect(find.text('Circles'), findsOneWidget);
    expect(find.text('Chats'), findsOneWidget);
    expect(find.text('Profile'), findsOneWidget);
  });

  testWidgets('switches to the chats tab', (WidgetTester tester) async {
    await _pumpApp(
      tester,
      _TestAuthController(const AuthState(status: AuthStatus.authenticated)),
    );
    await tester.pumpAndSettle();

    await tester.tap(find.text('Chats'));
    await tester.pumpAndSettle();

    expect(find.byType(ChatsScreen), findsOneWidget);
    expect(find.byKey(const Key('chatsEmpty')), findsOneWidget);
  });

  testWidgets('switches to the circles tab', (WidgetTester tester) async {
    await _pumpApp(
      tester,
      _TestAuthController(const AuthState(status: AuthStatus.authenticated)),
    );
    await tester.pumpAndSettle();

    await tester.tap(find.text('Circles'));
    await tester.pumpAndSettle();

    expect(find.byType(CircleDiscoveryScreen), findsOneWidget);
  });

  testWidgets('switches to the profile tab and logs out',
      (WidgetTester tester) async {
    // Tall viewport keeps the logout button on-screen without scrolling —
    // deterministic across Flutter versions and the IndexedStack branches.
    tester.view.physicalSize = const Size(800, 1600);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    await _pumpApp(
      tester,
      _TestAuthController(const AuthState(status: AuthStatus.authenticated)),
    );
    await tester.pumpAndSettle();

    await tester.tap(find.text('Profile'));
    await tester.pumpAndSettle();
    expect(find.byType(ProfileScreen), findsOneWidget);

    await tester.tap(find.byKey(const Key('logoutButton')));
    await tester.pump();

    expect(find.byKey(const Key('openLogin')), findsOneWidget);
    expect(find.byType(ProfileScreen), findsNothing);
  });

  testWidgets('auth loss clears the protected route stack',
      (WidgetTester tester) async {
    final controller = _TestAuthController(
      const AuthState(status: AuthStatus.authenticated),
    );
    await _pumpApp(tester, controller);
    await tester.pumpAndSettle();

    controller.becomeUnauthenticated();
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('openLogin')), findsOneWidget);
    expect(find.byKey(const Key('appNavigationBar')), findsNothing);
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
    await tester.pumpAndSettle();

    await tester.tap(find.text('Profile'));
    await tester.pumpAndSettle();
    final profileControllerForSessionA = profileControllers.single;

    controller.becomeUnauthenticated();
    await tester.pumpAndSettle();

    expect(profileControllerForSessionA.isDisposed, isTrue);

    controller.becomeAuthenticated('session-b');
    await tester.pumpAndSettle();
    await tester.tap(find.text('Profile'));
    await tester.pumpAndSettle();

    expect(profileControllers, hasLength(2));
    expect(profileControllers.last, isNot(same(profileControllerForSessionA)));
  });

  testWidgets('welcome screen uses Arabic labels in RTL',
      (WidgetTester tester) async {
    await tester.pumpWidget(
      const ProviderScope(
        child: MaterialApp(
          home: Directionality(
            textDirection: TextDirection.rtl,
            child: WelcomeScreen(),
          ),
        ),
      ),
    );

    expect(find.text('حلقاتي'), findsOneWidget);
    expect(find.text('تسجيل الدخول'), findsOneWidget);
    expect(find.text('إنشاء حساب'), findsOneWidget);
  });

  testWidgets('router builds with halaqaty light theme by default',
      (WidgetTester tester) async {
    await _pumpApp(
      tester,
      _TestAuthController(const AuthState(status: AuthStatus.unauthenticated)),
    );

    final materialApp = tester.widget<MaterialApp>(find.byType(MaterialApp));
    expect(materialApp.theme?.colorScheme.primary, const Color(0xFF1B7E3C));
  });
}
