import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:dio/dio.dart';
import 'package:go_router/go_router.dart';
import 'package:halaqaty_mobile/app/app_locale_controller.dart';
import 'package:halaqaty_mobile/app/chats_screen.dart';
import 'package:halaqaty_mobile/app/home_screen.dart';
import 'package:halaqaty_mobile/app/router.dart';
import 'package:halaqaty_mobile/app/welcome_screen.dart';
import 'package:halaqaty_mobile/core/design/halaqaty_components.dart';
import 'package:halaqaty_mobile/core/theme/halaqaty_theme.dart';
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

  int signInCalls = 0;
  int registerCalls = 0;
  String? lastPreferredLanguage;

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
  }) async {
    registerCalls++;
    lastPreferredLanguage = preferredLanguage;
  }

  @override
  Future<void> signIn({
    required String email,
    required String password,
  }) async {
    signInCalls++;
  }
}

class _TestProfileController extends StateNotifier<ProfileState>
    implements ProfileController {
  _TestProfileController() : super(const ProfileState());

  bool isDisposed = false;
  ProfileUser? profileToLoad;
  bool updateResult = false;

  @override
  void dispose() {
    isDisposed = true;
    super.dispose();
  }

  @override
  Future<void> loadProfile() async {
    final profile = profileToLoad;
    if (profile != null) {
      state = ProfileState(profile: profile);
    }
  }

  @override
  Future<bool> updateProfile({required UpdateProfileRequest request}) async =>
      updateResult;
}

class _TestCircleApiClient extends CircleApiClient {
  _TestCircleApiClient() : super(Dio());

  List<CircleSummary> circles = const [];
  DioException? listCirclesError;
  int listCirclesCalls = 0;
  Completer<List<CircleSummary>>? listCirclesCompleter;

  @override
  Future<List<CircleSummary>> listCircles({
    required String firebaseIdToken,
    required String sessionId,
  }) async =>
      _listCircles();

  Future<List<CircleSummary>> _listCircles() async {
    listCirclesCalls++;
    if (listCirclesError case final error?) {
      listCirclesError = null;
      throw error;
    }
    final pending = listCirclesCompleter;
    if (pending != null) {
      return pending.future;
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

Future<void> _pumpApp(
  WidgetTester tester,
  _TestAuthController controller, {
  List<_TestProfileController>? profileControllers,
  _TestCircleApiClient? circleApiClient,
  String? firebaseToken,
  ProfileUser? profileToLoad,
  bool profileUpdateResult = false,
  Locale platformLocale = const Locale('en'),
}) {
  final apiClient = circleApiClient ?? _TestCircleApiClient();
  return tester.pumpWidget(
    ProviderScope(
      overrides: [
        platformLocaleProvider.overrideWithValue(platformLocale),
        authControllerProvider.overrideWith((_) => controller),
        profileControllerProvider.overrideWith((_) {
          final profileController = _TestProfileController()
            ..profileToLoad = profileToLoad
            ..updateResult = profileUpdateResult;
          profileControllers?.add(profileController);
          return profileController;
        }),
        circleDiscoveryControllerProvider.overrideWith(
          (_) => CircleDiscoveryController(
            apiClient: apiClient,
            loadFirebaseIdToken: () async => firebaseToken,
            readAuthState: () => controller.state,
            logout: controller.logout,
          ),
        ),
        createCircleControllerProvider.overrideWith(
          (_) => CreateCircleController(
            apiClient: apiClient,
            loadFirebaseIdToken: () async => firebaseToken,
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

    final theme = Theme.of(tester.element(find.byKey(
      const Key('appNavigationBar'),
    )));
    expect(
      theme.navigationBarTheme.indicatorColor,
      theme.colorScheme.secondaryContainer,
    );
  });

  testWidgets('shows a retryable offline state when Home cannot load circles',
      (WidgetTester tester) async {
    final apiClient = _TestCircleApiClient()
      ..listCirclesError = DioException(
        requestOptions: RequestOptions(path: '/circles'),
        type: DioExceptionType.connectionError,
      );
    await _pumpApp(
      tester,
      _TestAuthController(
        const AuthState(
          status: AuthStatus.authenticated,
          sessionId: 'session-1',
        ),
      ),
      circleApiClient: apiClient,
      firebaseToken: 'firebase-token',
    );
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('circleLoadError')), findsOneWidget);
    expect(find.text('We cannot reach the server right now'), findsOneWidget);

    await tester.tap(find.byKey(const Key('circleLoadRetry')));
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('circleLoadError')), findsNothing);
    expect(apiClient.listCirclesCalls, 2);
  });

  testWidgets('switches to the chats tab', (WidgetTester tester) async {
    await _pumpApp(
      tester,
      _TestAuthController(
        const AuthState(
          status: AuthStatus.authenticated,
          sessionId: 'session-1',
        ),
      ),
      firebaseToken: 'firebase-token',
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
    expect(
      tester
          .getSemantics(find.text('Circles'))
          .flagsCollection
          .isSelected
          .toString(),
      endsWith('isTrue'),
    );
  });

  testWidgets('switching back to Home resets a pushed detail screen',
      (WidgetTester tester) async {
    final apiClient = _TestCircleApiClient()
      ..circles = [
        CircleSummary(
          id: 'circle-1',
          name: 'Circle',
          description: null,
          maxCapacity: 10,
          genderRestriction: 'unspecified',
          language: 'en',
          createdAt: DateTime.utc(2026, 8, 1),
        ),
      ];
    await _pumpApp(
      tester,
      _TestAuthController(
        const AuthState(
          status: AuthStatus.authenticated,
          sessionId: 'session-1',
        ),
      ),
      circleApiClient: apiClient,
      firebaseToken: 'firebase-token',
    );
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('homeCircle-circle-1')));
    await tester.pumpAndSettle();
    expect(find.text('Circle details'), findsOneWidget);

    await tester.tap(find.text('Profile'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Home'));
    await tester.pumpAndSettle();

    expect(find.byType(HomeScreen), findsOneWidget);
    expect(find.text('Circle details'), findsNothing);
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

  group('wave 4 auth tap budgets (US5)', () {
    testWidgets('complete sign-in from welcome within the 2-action budget',
        (WidgetTester tester) async {
      final controller = _TestAuthController(
        const AuthState(status: AuthStatus.unauthenticated),
      );
      await _pumpApp(tester, controller);

      // Action 1 of 2: open the login form.
      await tester.tap(find.byKey(const Key('openLogin')));
      await tester.pumpAndSettle();
      expect(find.byType(LoginScreen), findsOneWidget);

      // Typing does not count against the budget; submission is action 2.
      await tester.enterText(find.byKey(const Key('emailField')), 'a@b.com');
      await tester.enterText(find.byKey(const Key('passwordField')), 'secret1');
      await tester.tap(find.byKey(const Key('submitButton')));
      await tester.pump();

      expect(controller.signInCalls, 1);
    });

    testWidgets(
        'complete registration from welcome within the 2-action '
        'budget', (WidgetTester tester) async {
      final controller = _TestAuthController(
        const AuthState(status: AuthStatus.unauthenticated),
      );
      await _pumpApp(tester, controller);

      // Action 1 of 2: open the registration form.
      await tester.tap(find.byKey(const Key('openRegister')));
      await tester.pumpAndSettle();
      expect(find.byType(RegisterScreen), findsOneWidget);

      await tester.enterText(
          find.byKey(const Key('displayNameField')), 'Ahmad');
      await tester.enterText(find.byKey(const Key('emailField')), 'a@b.com');
      await tester.enterText(
          find.byKey(const Key('passwordField')), 'password123');
      await tester.tap(find.byKey(const Key('submitButton')));
      await tester.pump();

      expect(controller.registerCalls, 1);
      expect(controller.lastPreferredLanguage, 'ar');
    });
  });

  group('app locale and direction (US1)', () {
    Locale appLocaleOf(WidgetTester tester) => ProviderScope.containerOf(
          tester.element(find.byType(MaterialApp)),
        ).read(appLocaleControllerProvider);

    TextDirection directionOf(WidgetTester tester, Finder finder) =>
        Directionality.of(tester.element(finder));

    testWidgets('unsupported platform locale falls back to Arabic RTL',
        (WidgetTester tester) async {
      await _pumpApp(
        tester,
        _TestAuthController(
          const AuthState(status: AuthStatus.unauthenticated),
        ),
        platformLocale: const Locale('fr'),
      );

      expect(appLocaleOf(tester), const Locale('ar'));
      expect(
        directionOf(tester, find.byType(WelcomeScreen)),
        TextDirection.rtl,
      );
      expect(find.text('حلقاتي'), findsOneWidget);
    });

    testWidgets('Arabic platform locale selects Arabic RTL',
        (WidgetTester tester) async {
      await _pumpApp(
        tester,
        _TestAuthController(
          const AuthState(status: AuthStatus.unauthenticated),
        ),
        platformLocale: const Locale('ar'),
      );

      expect(appLocaleOf(tester), const Locale('ar'));
      expect(
        directionOf(tester, find.byType(WelcomeScreen)),
        TextDirection.rtl,
      );
    });

    testWidgets('English platform locale selects English LTR',
        (WidgetTester tester) async {
      await _pumpApp(
        tester,
        _TestAuthController(
          const AuthState(status: AuthStatus.unauthenticated),
        ),
      );

      expect(appLocaleOf(tester), const Locale('en'));
      expect(
        directionOf(tester, find.byType(WelcomeScreen)),
        TextDirection.ltr,
      );
      expect(find.text('Halaqaty'), findsOneWidget);
    });

    testWidgets('registration language selection updates locale immediately',
        (WidgetTester tester) async {
      await _pumpApp(
        tester,
        _TestAuthController(
          const AuthState(status: AuthStatus.unauthenticated),
        ),
        platformLocale: const Locale('ar'),
      );
      await tester.tap(find.byKey(const Key('openRegister')));
      await tester.pumpAndSettle();

      await tester.tap(find.byKey(const Key('languageDropdown')));
      await tester.pumpAndSettle();
      await tester.tap(find.text('الإنجليزية').last);
      await tester.pumpAndSettle();

      expect(appLocaleOf(tester), const Locale('en'));
      expect(
        directionOf(tester, find.byType(RegisterScreen)),
        TextDirection.ltr,
      );
    });

    testWidgets('successful profile load applies preferred language',
        (WidgetTester tester) async {
      await _pumpApp(
        tester,
        _TestAuthController(
          const AuthState(
            status: AuthStatus.authenticated,
            sessionId: 'session-1',
          ),
        ),
        platformLocale: const Locale('ar'),
        profileToLoad: ProfileUser(
          id: 'user-1',
          firebaseUid: 'firebase-1',
          fullName: 'Karim Fadel',
          displayName: 'Karim',
          bio: null,
          country: 'EG',
          preferredLanguage: 'en',
          avatarUrl: null,
          phone: null,
          createdAt: DateTime.utc(2026, 1, 1),
        ),
      );
      await tester.pumpAndSettle();

      await tester.tap(find.text('حسابي'));
      await tester.pumpAndSettle();

      expect(appLocaleOf(tester), const Locale('en'));
      expect(
        directionOf(tester, find.byType(ProfileScreen)),
        TextDirection.ltr,
      );
    });

    testWidgets('profile save updates locale and rebuilds the app router',
        (WidgetTester tester) async {
      tester.view.physicalSize = const Size(800, 1600);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      await _pumpApp(
        tester,
        _TestAuthController(
          const AuthState(
            status: AuthStatus.authenticated,
            sessionId: 'session-1',
          ),
        ),
        platformLocale: const Locale('ar'),
        profileToLoad: ProfileUser(
          id: 'user-1',
          firebaseUid: 'firebase-1',
          fullName: 'Karim Fadel',
          displayName: 'Karim',
          bio: null,
          country: 'EG',
          preferredLanguage: 'ar',
          avatarUrl: null,
          phone: null,
          createdAt: DateTime.utc(2026, 1, 1),
        ),
        profileUpdateResult: true,
      );
      await tester.pumpAndSettle();
      await tester.tap(find.text('حسابي'));
      await tester.pumpAndSettle();

      await tester.tap(find.byKey(const Key('profileLanguageDropdown')));
      await tester.pumpAndSettle();
      await tester.tap(find.text('الإنجليزية').last);
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const Key('profileSaveButton')));
      await tester.pumpAndSettle();

      expect(appLocaleOf(tester), const Locale('en'));
      expect(
        directionOf(tester, find.byType(ProfileScreen)),
        TextDirection.ltr,
      );
    });

    testWidgets('logout restores the platform locale fallback',
        (WidgetTester tester) async {
      tester.view.physicalSize = const Size(800, 1600);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      await _pumpApp(
        tester,
        _TestAuthController(
          const AuthState(
            status: AuthStatus.authenticated,
            sessionId: 'session-1',
          ),
        ),
        platformLocale: const Locale('en'),
        profileToLoad: ProfileUser(
          id: 'user-1',
          firebaseUid: 'firebase-1',
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
      await tester.pumpAndSettle();
      await tester.tap(find.text('Profile'));
      await tester.pumpAndSettle();
      expect(appLocaleOf(tester), const Locale('ar'));

      await tester.tap(find.byKey(const Key('logoutButton')));
      await tester.pumpAndSettle();

      expect(appLocaleOf(tester), const Locale('en'));
      expect(
        directionOf(tester, find.byType(WelcomeScreen)),
        TextDirection.ltr,
      );
    });

    testWidgets('protected redirect keeps unauthenticated users off the shell',
        (WidgetTester tester) async {
      await _pumpApp(
        tester,
        _TestAuthController(
          const AuthState(status: AuthStatus.unauthenticated),
        ),
      );
      await tester.pumpAndSettle();

      final router = tester
          .widget<MaterialApp>(find.byType(MaterialApp))
          .routerConfig! as GoRouter;
      router.go('/home');
      await tester.pumpAndSettle();

      expect(find.byType(WelcomeScreen), findsOneWidget);
      expect(find.byKey(const Key('appNavigationBar')), findsNothing);
    });
  });

  group('shell quality gates (US1)', () {
    testWidgets('navigation destinations and welcome actions meet 48dp',
        (WidgetTester tester) async {
      await _pumpApp(
        tester,
        _TestAuthController(
          const AuthState(status: AuthStatus.authenticated),
        ),
      );
      await tester.pumpAndSettle();

      for (final label in ['Home', 'Circles', 'Chats', 'Profile']) {
        expect(
          find.descendant(
            of: find.byKey(const Key('appNavigationBar')),
            matching: find.text(label),
          ),
          findsOneWidget,
        );
      }
      final destinations = find.descendant(
        of: find.byKey(const Key('appNavigationBar')),
        matching: find.byType(NavigationDestination),
      );
      expect(destinations, findsNWidgets(4));
      for (var i = 0; i < 4; i++) {
        expect(
          tester.getSize(destinations.at(i)).height,
          greaterThanOrEqualTo(48),
        );
      }
    });

    testWidgets('welcome actions meet 48dp targets',
        (WidgetTester tester) async {
      await _pumpApp(
        tester,
        _TestAuthController(
          const AuthState(status: AuthStatus.unauthenticated),
        ),
      );
      await tester.pumpAndSettle();

      expect(find.byKey(const Key('openLogin')), findsOneWidget);
      expect(
        tester.getSize(find.byKey(const Key('openLogin'))).height,
        greaterThanOrEqualTo(48),
      );
      expect(find.byKey(const Key('openRegister')), findsOneWidget);
      expect(
        tester.getSize(find.byKey(const Key('openRegister'))).height,
        greaterThanOrEqualTo(48),
      );
    });

    testWidgets('splash loading is branded and semantically labelled',
        (WidgetTester tester) async {
      await _pumpApp(
        tester,
        _TestAuthController(const AuthState(status: AuthStatus.unknown)),
      );

      expect(find.byType(HalaqatyLogo), findsOneWidget);
      final semantics = tester.getSemantics(
        find.byKey(const Key('authInitializing')),
      );
      expect(semantics.label, isNotEmpty);
    });

    testWidgets('selected tab changes icon shape, not color alone',
        (WidgetTester tester) async {
      await _pumpApp(
        tester,
        _TestAuthController(
          const AuthState(status: AuthStatus.authenticated),
        ),
      );
      await tester.pumpAndSettle();

      expect(find.byIcon(Icons.home), findsOneWidget);
      expect(find.byIcon(Icons.groups_outlined), findsOneWidget);

      await tester.tap(find.text('Circles'));
      await tester.pumpAndSettle();

      expect(find.byIcon(Icons.home_outlined), findsOneWidget);
      expect(find.byIcon(Icons.groups), findsOneWidget);
    });

    testWidgets('dark theme keeps selected-state meaning and font mapping',
        (WidgetTester tester) async {
      final dark = halaqatyDarkTheme();
      expect(
        dark.navigationBarTheme.indicatorColor,
        dark.colorScheme.secondaryContainer,
      );
      final labelStyle =
          dark.navigationBarTheme.labelTextStyle?.resolve(const {});
      expect(labelStyle?.fontWeight, FontWeight.w600);

      // Frozen bundled-font mapping: title/label/button request 600 (Poppins
      // SemiBold / Cairo Bold fallback); display/headline/body request 400.
      expect(dark.textTheme.titleLarge?.fontWeight, FontWeight.w600);
      expect(dark.textTheme.titleMedium?.fontWeight, FontWeight.w600);
      expect(dark.textTheme.labelLarge?.fontWeight, FontWeight.w600);
      expect(dark.textTheme.bodyLarge?.fontWeight, FontWeight.w400);
      expect(dark.textTheme.headlineSmall?.fontWeight, FontWeight.w400);
      // Arabic/Quranic lines keep height at least 1.5 in both themes.
      expect(dark.textTheme.bodyLarge?.height, greaterThanOrEqualTo(1.5));

      final light = halaqatyLightTheme();
      expect(light.textTheme.titleLarge?.fontWeight, FontWeight.w600);
      expect(light.textTheme.bodyLarge?.fontWeight, FontWeight.w400);
      expect(light.textTheme.bodyLarge?.height, greaterThanOrEqualTo(1.5));
    });
  });

  group('home and chats states (US1)', () {
    CircleSummary circle(String id) => CircleSummary(
          id: id,
          name: 'Circle $id',
          description: null,
          maxCapacity: 10,
          genderRestriction: 'unspecified',
          language: 'en',
          createdAt: DateTime.utc(2026, 8, 1),
        );

    DioException connectionError() => DioException(
          requestOptions: RequestOptions(path: '/circles'),
          type: DioExceptionType.connectionError,
        );

    testWidgets('home shows branded loading while circles load',
        (WidgetTester tester) async {
      final apiClient = _TestCircleApiClient()
        ..listCirclesCompleter = Completer<List<CircleSummary>>();
      await _pumpApp(
        tester,
        _TestAuthController(
          const AuthState(
            status: AuthStatus.authenticated,
            sessionId: 'session-1',
          ),
        ),
        circleApiClient: apiClient,
        firebaseToken: 'firebase-token',
      );
      await tester.pump();

      expect(find.byKey(const Key('homeLoading')), findsOneWidget);
      expect(find.byType(HalaqatyLogo), findsWidgets);

      apiClient.listCirclesCompleter!.complete(const []);
      await tester.pumpAndSettle();
      expect(find.byKey(const Key('homeNoCircles')), findsOneWidget);
    });

    testWidgets(
        'home keeps circles visible with a stale notice when refresh '
        'fails', (WidgetTester tester) async {
      final apiClient = _TestCircleApiClient()..circles = [circle('circle-1')];
      await _pumpApp(
        tester,
        _TestAuthController(
          const AuthState(
            status: AuthStatus.authenticated,
            sessionId: 'session-1',
          ),
        ),
        circleApiClient: apiClient,
        firebaseToken: 'firebase-token',
      );
      await tester.pumpAndSettle();
      expect(find.byKey(const Key('homeCircle-circle-1')), findsOneWidget);

      apiClient.listCirclesError = connectionError();
      final container = ProviderScope.containerOf(
        tester.element(find.byType(HomeScreen)),
      );
      await container
          .read(circleDiscoveryControllerProvider.notifier)
          .loadMyCircles();
      await tester.pump();

      expect(find.byKey(const Key('homeCircle-circle-1')), findsOneWidget);
      expect(find.byKey(const Key('circleLoadError')), findsOneWidget);

      await tester.tap(find.byKey(const Key('circleLoadRetry')));
      await tester.pumpAndSettle();
      expect(find.byKey(const Key('circleLoadError')), findsNothing);
      expect(find.byKey(const Key('homeCircle-circle-1')), findsOneWidget);
    });

    testWidgets(
        'chats shows a retryable error instead of empty when load '
        'fails', (WidgetTester tester) async {
      // The error is armed after Home's initial load so it hits the Chats
      // tab's own load (the fake consumes each armed error once).
      final apiClient = _TestCircleApiClient();
      await _pumpApp(
        tester,
        _TestAuthController(
          const AuthState(
            status: AuthStatus.authenticated,
            sessionId: 'session-1',
          ),
        ),
        circleApiClient: apiClient,
        firebaseToken: 'firebase-token',
      );
      await tester.pumpAndSettle();

      apiClient.listCirclesError = connectionError();
      await tester.tap(find.text('Chats'));
      await tester.pumpAndSettle();

      expect(find.byKey(const Key('circleLoadError')), findsOneWidget);
      expect(find.byKey(const Key('chatsEmpty')), findsNothing);
    });

    testWidgets(
        'chats keeps conversations visible with a stale notice when '
        'refresh fails', (WidgetTester tester) async {
      final apiClient = _TestCircleApiClient()..circles = [circle('circle-1')];
      await _pumpApp(
        tester,
        _TestAuthController(
          const AuthState(
            status: AuthStatus.authenticated,
            sessionId: 'session-1',
          ),
        ),
        circleApiClient: apiClient,
        firebaseToken: 'firebase-token',
      );
      await tester.pumpAndSettle();
      await tester.tap(find.text('Chats'));
      await tester.pumpAndSettle();
      expect(find.byKey(const Key('chatCircle-circle-1')), findsOneWidget);

      apiClient.listCirclesError = connectionError();
      final container = ProviderScope.containerOf(
        tester.element(find.byType(ChatsScreen)),
      );
      await container
          .read(circleDiscoveryControllerProvider.notifier)
          .loadMyCircles();
      await tester.pump();

      expect(find.byKey(const Key('chatCircle-circle-1')), findsOneWidget);
      expect(find.byKey(const Key('circleLoadError')), findsOneWidget);
    });
  });
}
