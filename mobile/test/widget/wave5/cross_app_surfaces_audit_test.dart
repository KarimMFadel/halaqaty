import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/app/app_locale_controller.dart';
import 'package:halaqaty_mobile/core/theme/halaqaty_theme.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_detail_controller.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_discovery_controller.dart';
import 'package:halaqaty_mobile/features/circles/application/create_circle_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_detail_screen.dart';
import 'package:halaqaty_mobile/features/profile/application/profile_controller.dart';
import 'package:halaqaty_mobile/features/profile/data/profile_api_client.dart';
import 'package:halaqaty_mobile/features/profile/presentation/profile_screen.dart';
import 'package:halaqaty_mobile/features/sessions/application/circle_sessions_controller.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_api_client.dart';
import 'package:halaqaty_mobile/main.dart';

import '../../helpers/stub_auth_notifier.dart';

/// Wave 5 cross-app audit (T044): shell destination order, directional icons,
/// semantics spot-checks, long mixed-script/diacritic layout, and the
/// inventoried under-implementation notice actions (FR-032/FR-033).
/// The regression assertions preserve the T045 findings after their fixes.

class _TestAuthController extends StateNotifier<AuthState>
    implements AuthController {
  _TestAuthController(super.initialState);

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

class _TestProfileController extends StateNotifier<ProfileState>
    implements ProfileController {
  _TestProfileController() : super(const ProfileState());

  @override
  Future<void> loadProfile() async {}

  @override
  Future<bool> updateProfile({required UpdateProfileRequest request}) async =>
      false;
}

class _TestCircleApiClient extends CircleApiClient {
  _TestCircleApiClient() : super(Dio());

  List<CircleSummary> circles = const [];

  @override
  Future<List<CircleSummary>> listCircles({
    required String firebaseIdToken,
    required String sessionId,
  }) async =>
      circles;

  @override
  Future<CircleDiscoveryPage> discoverCircles({
    required String firebaseIdToken,
    required String sessionId,
    String? query,
    String? cursor,
  }) async =>
      const CircleDiscoveryPage(circles: []);
}

Future<void> _pumpShellApp(
  WidgetTester tester,
  _TestAuthController controller, {
  _TestCircleApiClient? circleApiClient,
  String? firebaseToken,
  Locale platformLocale = const Locale('en'),
}) {
  final apiClient = circleApiClient ?? _TestCircleApiClient();
  return tester.pumpWidget(
    ProviderScope(
      overrides: [
        platformLocaleProvider.overrideWithValue(platformLocale),
        authControllerProvider.overrideWith((_) => controller),
        profileControllerProvider.overrideWith((_) => _TestProfileController()),
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

/// Counts pushed routes so notice actions can prove zero navigation (FR-032).
class _RouteCounter extends NavigatorObserver {
  int pushes = 0;

  @override
  void didPush(Route<dynamic> route, Route<dynamic>? previousRoute) {
    pushes++;
  }
}

/// Records session-section invocations so notice actions can prove zero
/// controller/API side effects (FR-032/SC-009).
class _SpyCircleSessionsController extends StateNotifier<CircleSessionsState>
    implements CircleSessionsController {
  _SpyCircleSessionsController()
      : super(const CircleSessionsState(status: CircleSessionsStatus.ready));

  int loadCalls = 0;
  int createCalls = 0;

  @override
  String get circleId => 'circle-1';

  @override
  Future<void> load() async {
    loadCalls++;
  }

  @override
  Future<SessionModel?> create() async {
    createCalls++;
    return null;
  }
}

/// Profile stub that counts invocations for zero-side-effect assertions.
class _SpyProfileNotifier extends StateNotifier<ProfileState>
    implements ProfileController {
  _SpyProfileNotifier() : super(const ProfileState());

  int loadCalls = 0;
  bool updateCalled = false;

  @override
  Future<void> loadProfile() async {
    loadCalls++;
    state = ProfileState(
      profile: ProfileUser(
        id: 'user-1',
        firebaseUid: 'firebase-1',
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
  Future<bool> updateProfile({required UpdateProfileRequest request}) async {
    updateCalled = true;
    return false;
  }
}

class _RecordingAuthNotifier extends StubAuthNotifier {
  _RecordingAuthNotifier()
      : super(
          initialState: const AuthState(
            status: AuthStatus.authenticated,
            sessionId: 'session-1',
          ),
        );

  bool logoutCalled = false;

  @override
  Future<void> logout() async {
    logoutCalled = true;
  }
}

CircleSummary _circleSummary({String name = 'Circle', String? description}) =>
    CircleSummary(
      id: 'circle-1',
      name: name,
      description: description,
      maxCapacity: 10,
      genderRestriction: 'unspecified',
      language: 'en',
      createdAt: DateTime.utc(2026, 8, 1),
    );

CircleResponse _circleDetail() => CircleResponse(
      id: 'circle-1',
      name: 'Circle',
      inviteCode: 'HLQ-7X2K',
      inviteLink: 'https://halaqaty.app/join/HLQ-7X2K',
      isArchived: false,
      createdAt: DateTime.utc(2026, 8, 1),
    );

/// Pumps [CircleDetailScreen] standalone with stubbed detail/members data and
/// a spy sessions controller; [detailFetches] counts provider rebuilds.
Future<void> _pumpCircleDetail(
  WidgetTester tester, {
  required TextDirection direction,
  required _SpyCircleSessionsController sessionsSpy,
  required _RouteCounter routes,
  required void Function() onDetailFetch,
}) {
  return tester.pumpWidget(
    ProviderScope(
      overrides: [
        authControllerProvider.overrideWith((_) => StubAuthNotifier()),
        circleDetailProvider('circle-1').overrideWith((_) {
          onDetailFetch();
          return Future.value(_circleDetail());
        }),
        circleMembersProvider('circle-1').overrideWith(
          (_) => Future.value([
            CircleMember(
              userId: 'user-1',
              displayName: 'Student',
              role: CircleRole.student,
              joinedAt: DateTime.utc(2026, 8, 1),
            ),
          ]),
        ),
        circleSessionsControllerProvider('circle-1')
            .overrideWith((_) => sessionsSpy),
      ],
      child: MaterialApp(
        navigatorObservers: [routes],
        home: Directionality(
          textDirection: direction,
          child: const CircleDetailScreen(
            circleId: 'circle-1',
            currentUserId: 'user-1',
          ),
        ),
      ),
    ),
  );
}

Icon _trailingIcon(WidgetTester tester, Finder tileFinder) =>
    tester.widget<ListTile>(tileFinder).trailing! as Icon;

Finder _tileWith(String label) =>
    find.ancestor(of: find.text(label), matching: find.byType(ListTile)).first;

bool _isTrue(Object? flag) => flag == true || flag.toString() == 'isTrue';

void main() {
  group('shell destination order (FR-002/FR-010)', () {
    Finder destinations() => find.descendant(
          of: find.byKey(const Key('appNavigationBar')),
          matching: find.byType(NavigationDestination),
        );

    testWidgets('LTR order is Home, Circles, Chats, Profile left to right',
        (tester) async {
      await _pumpShellApp(
        tester,
        _TestAuthController(const AuthState(status: AuthStatus.authenticated)),
      );
      await tester.pumpAndSettle();

      final labels = tester
          .widgetList<NavigationDestination>(destinations())
          .map((destination) => destination.label)
          .toList();
      expect(labels, ['Home', 'Circles', 'Chats', 'Profile']);
      for (var i = 1; i < 4; i++) {
        expect(
          tester.getCenter(destinations().at(i)).dx,
          greaterThan(tester.getCenter(destinations().at(i - 1)).dx),
          reason: 'LTR destinations must flow left to right',
        );
      }
    });

    testWidgets(
        'RTL order is Home, Circles, Chats, Profile right to left at 48dp',
        (tester) async {
      await _pumpShellApp(
        tester,
        _TestAuthController(const AuthState(status: AuthStatus.authenticated)),
        platformLocale: const Locale('ar'),
      );
      await tester.pumpAndSettle();

      final labels = tester
          .widgetList<NavigationDestination>(destinations())
          .map((destination) => destination.label)
          .toList();
      expect(labels, ['الرئيسية', 'الحلقات', 'المحادثات', 'حسابي']);
      for (var i = 1; i < 4; i++) {
        expect(
          tester.getCenter(destinations().at(i)).dx,
          lessThan(tester.getCenter(destinations().at(i - 1)).dx),
          reason: 'RTL destinations must flow right to left',
        );
      }
      for (var i = 0; i < 4; i++) {
        expect(
          tester.getSize(destinations().at(i)).height,
          greaterThanOrEqualTo(48),
          reason: 'every shell destination is an interactive target (FR-013)',
        );
      }
    });
  });

  group('directional icons follow the active direction (FR-010)', () {
    // Material mirrors direction-aware icons (matchTextDirection: true) once
    // at paint time; surfaces must use one base forward glyph in both
    // directions. Selecting a pre-flipped glyph per direction double-mirrors
    // and points the affordance backwards in RTL.
    Future<IconData> homeChevron(
      WidgetTester tester, {
      required Locale platformLocale,
    }) async {
      await _pumpShellApp(
        tester,
        _TestAuthController(
          const AuthState(
            status: AuthStatus.authenticated,
            sessionId: 'session-1',
          ),
        ),
        circleApiClient: _TestCircleApiClient()..circles = [_circleSummary()],
        firebaseToken: 'firebase-token',
        platformLocale: platformLocale,
      );
      await tester.pumpAndSettle();
      return _trailingIcon(tester, find.byKey(const Key('homeCircle-circle-1')))
          .icon!;
    }

    Future<IconData> chatsChevron(
      WidgetTester tester, {
      required Locale platformLocale,
    }) async {
      await _pumpShellApp(
        tester,
        _TestAuthController(
          const AuthState(
            status: AuthStatus.authenticated,
            sessionId: 'session-1',
          ),
        ),
        circleApiClient: _TestCircleApiClient()..circles = [_circleSummary()],
        firebaseToken: 'firebase-token',
        platformLocale: platformLocale,
      );
      await tester.pumpAndSettle();
      // Tap the destination widget, not its label: NavigationBar renders
      // label text in more than one subtree, which makes a text tap
      // ambiguous.
      await tester.tap(
        find
            .descendant(
              of: find.byKey(const Key('appNavigationBar')),
              matching: find.byType(NavigationDestination),
            )
            .at(2),
      );
      await tester.pumpAndSettle();
      return _trailingIcon(tester, find.byKey(const Key('chatCircle-circle-1')))
          .icon!;
    }

    testWidgets('Home circle-card chevron is one direction-aware icon',
        (tester) async {
      final ltr = await homeChevron(tester, platformLocale: const Locale('en'));
      final rtl = await homeChevron(tester, platformLocale: const Locale('ar'));

      expect(ltr.matchTextDirection, isTrue);
      expect(
        rtl,
        ltr,
        reason: 'explicitly flipping to a mirrored glyph double-mirrors at '
            'paint time and points the affordance backwards in RTL (FR-010)',
      );
    });

    testWidgets('Chats circle-tile chevron is one direction-aware icon',
        (tester) async {
      final ltr =
          await chatsChevron(tester, platformLocale: const Locale('en'));
      final rtl =
          await chatsChevron(tester, platformLocale: const Locale('ar'));

      expect(ltr.matchTextDirection, isTrue);
      expect(
        rtl,
        ltr,
        reason: 'explicitly flipping to a mirrored glyph double-mirrors at '
            'paint time and points the affordance backwards in RTL (FR-010)',
      );
    });

    testWidgets(
        'circles discovery tiles use a direction-aware forward chevron in '
        'RTL', (tester) async {
      await _pumpShellApp(
        tester,
        _TestAuthController(
          const AuthState(
            status: AuthStatus.authenticated,
            sessionId: 'session-1',
          ),
        ),
        circleApiClient: _TestCircleApiClient()..circles = [_circleSummary()],
        firebaseToken: 'firebase-token',
        platformLocale: const Locale('ar'),
      );
      await tester.pumpAndSettle();

      await tester.tap(find.text('الحلقات'));
      await tester.pumpAndSettle();

      final icon =
          _trailingIcon(tester, find.byKey(const Key('openCircle-circle-1')));
      expect(
        icon.icon?.matchTextDirection,
        isTrue,
        reason: 'Material mirrors direction-aware icons in RTL (FR-010)',
      );
    });

    testWidgets(
        'circle detail members and chat tiles use direction-aware forward '
        'chevrons in RTL', (tester) async {
      final sessionsSpy = _SpyCircleSessionsController();
      await _pumpCircleDetail(
        tester,
        direction: TextDirection.rtl,
        sessionsSpy: sessionsSpy,
        routes: _RouteCounter(),
        onDetailFetch: () {},
      );
      await tester.pumpAndSettle();

      for (final label in ['الأعضاء', 'المحادثة']) {
        final icon = _trailingIcon(tester, _tileWith(label));
        expect(
          icon.icon?.matchTextDirection,
          isTrue,
          reason: '$label forward affordance must mirror in RTL (FR-010)',
        );
      }
    });

    testWidgets('profile notice tiles mirror their forward icon in RTL',
        (tester) async {
      await tester.pumpWidget(
        ProviderScope(
          overrides: [
            profileControllerProvider
                .overrideWith((_) => _SpyProfileNotifier()),
            authControllerProvider.overrideWith(
              (_) => StubAuthNotifier(
                initialState: const AuthState(
                  status: AuthStatus.authenticated,
                  sessionId: 'session-1',
                ),
              ),
            ),
          ],
          child: const MaterialApp(
            home: Directionality(
              textDirection: TextDirection.rtl,
              child: ProfileScreen(),
            ),
          ),
        ),
      );
      await tester.pump();

      final icon = _trailingIcon(tester, _tileWith('المظهر'));
      expect(
        icon.icon == Icons.chevron_left ||
            (icon.icon?.matchTextDirection ?? false),
        isTrue,
        reason: 'the forward affordance must mirror in RTL: use a '
            'direction-aware icon or an explicit left chevron (FR-010)',
      );
    });

    testWidgets('the AppBar back button uses a direction-aware icon',
        (tester) async {
      await _pumpShellApp(
        tester,
        _TestAuthController(
          const AuthState(
            status: AuthStatus.authenticated,
            sessionId: 'session-1',
          ),
        ),
        circleApiClient: _TestCircleApiClient()..circles = [_circleSummary()],
        firebaseToken: 'firebase-token',
        platformLocale: const Locale('ar'),
      );
      await tester.pumpAndSettle();

      await tester.tap(find.byKey(const Key('homeCircle-circle-1')));
      await tester.pumpAndSettle();

      final backButton = find.byType(BackButton);
      expect(backButton, findsOneWidget);
      final icon = tester.widget<Icon>(
        find.descendant(of: backButton, matching: find.byType(Icon)),
      );
      expect(icon.icon?.matchTextDirection, isTrue);
    });
  });

  group('semantics spot-checks (FR-014)', () {
    testWidgets('welcome actions expose meaningful labels and button roles',
        (tester) async {
      await _pumpShellApp(
        tester,
        _TestAuthController(
          const AuthState(status: AuthStatus.unauthenticated),
        ),
      );

      final login = tester.getSemantics(find.byKey(const Key('openLogin')));
      expect(login.label, 'Sign in');
      expect(_isTrue(login.flagsCollection.isButton), isTrue);
      final register =
          tester.getSemantics(find.byKey(const Key('openRegister')));
      expect(register.label, 'Register');
      expect(_isTrue(register.flagsCollection.isButton), isTrue);
    });

    testWidgets('welcome actions expose Arabic labels in RTL', (tester) async {
      await _pumpShellApp(
        tester,
        _TestAuthController(
          const AuthState(status: AuthStatus.unauthenticated),
        ),
        platformLocale: const Locale('ar'),
      );

      expect(
        tester.getSemantics(find.byKey(const Key('openLogin'))).label,
        'تسجيل الدخول',
      );
      expect(
        tester.getSemantics(find.byKey(const Key('openRegister'))).label,
        'إنشاء حساب',
      );
    });

    testWidgets('shell destinations expose selected state', (tester) async {
      await _pumpShellApp(
        tester,
        _TestAuthController(const AuthState(status: AuthStatus.authenticated)),
      );
      await tester.pumpAndSettle();

      // The NavigationBar wraps each destination in Semantics(role: tab,
      // selected: ...) under a MergeSemantics; assert the selected tab
      // exposes exactly one selected tab carrying the destination label.
      final navBar = find.byKey(const Key('appNavigationBar'));
      Finder selectedTab() => find.descendant(
            of: navBar,
            matching: find.byWidgetPredicate(
              (widget) =>
                  widget is Semantics && widget.properties.selected == true,
            ),
          );
      Finder labelIn(Finder tab, String label) =>
          find.descendant(of: tab, matching: find.text(label));

      expect(selectedTab(), findsOneWidget);
      expect(labelIn(selectedTab(), 'Home'), findsOneWidget);

      await tester.tap(find.text('Chats'));
      await tester.pumpAndSettle();

      expect(selectedTab(), findsOneWidget);
      expect(labelIn(selectedTab(), 'Chats'), findsOneWidget);
      expect(labelIn(selectedTab(), 'Home'), findsNothing);
    });
  });

  group('long mixed-script copy and diacritic layout (FR-015)', () {
    testWidgets(
        'home renders a long Arabic/Latin name and diacritic text at '
        '320dp and 200% text scale with the primary action reachable',
        (tester) async {
      tester.view.physicalSize = const Size(320, 800);
      tester.view.devicePixelRatio = 1.0;
      tester.platformDispatcher.textScaleFactorTestValue = 2.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      addTearDown(tester.platformDispatcher.clearTextScaleFactorTestValue);

      const name = 'حلقة تلاوة Quran Recitation Circle — جزء عمّ '
          'وأحكام التجويد للمبتدئين الجدد';
      const diacritics = 'بِسْمِ ٱللَّهِ ٱلرَّحْمَٰنِ ٱلرَّحِيمِ';
      final apiClient = _TestCircleApiClient()
        ..circles = [_circleSummary(name: name, description: diacritics)];

      await _pumpShellApp(
        tester,
        _TestAuthController(
          const AuthState(
            status: AuthStatus.authenticated,
            sessionId: 'session-1',
          ),
        ),
        circleApiClient: apiClient,
        firebaseToken: 'firebase-token',
        platformLocale: const Locale('ar'),
      );
      await tester.pumpAndSettle();

      // Any clipped diacritic or overflowing name throws in the test harness.
      final card = find.byKey(const Key('homeCircle-circle-1'));
      expect(card, findsOneWidget);
      final semantics = tester.widget<Semantics>(
        find.ancestor(
          of: card,
          matching: find.byWidgetPredicate(
            (widget) => widget is Semantics && widget.properties.label == name,
          ),
        ),
      );
      expect(semantics.properties.button, isTrue);

      // Primary action stays reachable at compact width and 200% text.
      await tester.tap(card);
      await tester.pumpAndSettle();
      expect(find.text('تفاصيل الحلقة'), findsOneWidget);
    });
  });

  group('under-implementation notices (FR-032/FR-033, SC-009)', () {
    testWidgets(
        'circle detail schedule action shows only the shared notice (LTR)',
        (tester) async {
      final sessionsSpy = _SpyCircleSessionsController();
      final routes = _RouteCounter();
      var detailFetches = 0;
      await _pumpCircleDetail(
        tester,
        direction: TextDirection.ltr,
        sessionsSpy: sessionsSpy,
        routes: routes,
        onDetailFetch: () => detailFetches++,
      );
      await tester.pumpAndSettle();
      final baselinePushes = routes.pushes;

      final schedule = find.text('Schedule');
      expect(
        schedule,
        findsOneWidget,
        reason: 'the approved design keeps the planned schedule action '
            'visible and tappable (compatibility inventory §6)',
      );

      await tester.ensureVisible(schedule);
      // Let the scroll finish before hit-testing (matches the management
      // tile pattern in circle_detail_screen_test.dart).
      await tester.pumpAndSettle();
      await tester.tap(schedule);
      await tester.pump();

      expect(
        find.byKey(const Key('halaqatyUnderImplementationSnackBar')),
        findsOneWidget,
      );
      expect(
        find.text(
          'This feature is under implementation and is not available yet.',
        ),
        findsOneWidget,
      );

      // Re-invocation replaces the prior instance instead of stacking.
      await tester.ensureVisible(schedule);
      await tester.pumpAndSettle();
      await tester.tap(schedule);
      await tester.pumpAndSettle();
      expect(
        find.byKey(const Key('halaqatyUnderImplementationSnackBar')),
        findsOneWidget,
      );

      // Dismissible via its action.
      await tester.tap(find.text('Dismiss'));
      await tester.pumpAndSettle();
      expect(
        find.byKey(const Key('halaqatyUnderImplementationSnackBar')),
        findsNothing,
      );

      // Zero navigation, controller/API calls, or state mutation.
      expect(find.byType(CircleDetailScreen), findsOneWidget);
      expect(routes.pushes, baselinePushes);
      expect(detailFetches, 1);
      expect(sessionsSpy.loadCalls, 1, reason: 'only the initial section load');
      expect(sessionsSpy.createCalls, 0);
    });

    testWidgets(
        'circle detail schedule action shows only the shared notice (RTL)',
        (tester) async {
      final sessionsSpy = _SpyCircleSessionsController();
      final routes = _RouteCounter();
      var detailFetches = 0;
      await _pumpCircleDetail(
        tester,
        direction: TextDirection.rtl,
        sessionsSpy: sessionsSpy,
        routes: routes,
        onDetailFetch: () => detailFetches++,
      );
      await tester.pumpAndSettle();

      final schedule = find.text('المواعيد');
      expect(
        schedule,
        findsOneWidget,
        reason: 'the approved design keeps the planned schedule action '
            'visible and tappable (compatibility inventory §6)',
      );

      await tester.ensureVisible(schedule);
      await tester.pumpAndSettle();
      await tester.tap(schedule);
      await tester.pump();

      expect(
        find.byKey(const Key('halaqatyUnderImplementationSnackBar')),
        findsOneWidget,
      );
      expect(
        find.text('هذه الميزة قيد التنفيذ وغير متاحة حالياً.'),
        findsOneWidget,
      );

      expect(find.byType(CircleDetailScreen), findsOneWidget);
      expect(detailFetches, 1);
      expect(sessionsSpy.loadCalls, 1);
      expect(sessionsSpy.createCalls, 0);
    });

    testWidgets(
        'profile notice tiles replace the notice and cause zero side '
        'effects in RTL', (tester) async {
      tester.view.physicalSize = const Size(800, 1600);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      final profile = _SpyProfileNotifier();
      final auth = _RecordingAuthNotifier();
      await tester.pumpWidget(
        ProviderScope(
          overrides: [
            profileControllerProvider.overrideWith((_) => profile),
            authControllerProvider.overrideWith((_) => auth),
          ],
          child: const MaterialApp(
            home: Directionality(
              textDirection: TextDirection.rtl,
              child: ProfileScreen(),
            ),
          ),
        ),
      );
      await tester.pump();
      expect(profile.loadCalls, 1);

      const noticeKey = Key('halaqatyUnderImplementationSnackBar');
      const arabicCopy = 'هذه الميزة قيد التنفيذ وغير متاحة حالياً.';

      await tester.ensureVisible(find.text('المظهر'));
      await tester.tap(find.text('المظهر'));
      await tester.pump();
      expect(find.byKey(noticeKey), findsOneWidget);
      expect(find.text(arabicCopy), findsOneWidget);

      // A second planned action replaces the prior notice (FR-033).
      await tester.ensureVisible(find.text('الإشعارات'));
      await tester.tap(find.text('الإشعارات'));
      await tester.pump();
      expect(find.byKey(noticeKey), findsOneWidget);

      // The notice is dismissible. Let the replaced instance finish its exit
      // animation first so the tap lands on the live notice's action.
      await tester.pump(const Duration(milliseconds: 500));
      await tester.tap(find.text('إغلاق'));
      await tester.pumpAndSettle();
      expect(find.byKey(noticeKey), findsNothing);

      for (final label in ['المساعدة والدعم', 'الخصوصية والأمان']) {
        await tester.ensureVisible(find.text(label));
        await tester.tap(find.text(label));
        await tester.pump();
        expect(
          find.byKey(noticeKey),
          findsOneWidget,
          reason: '$label must surface the shared notice',
        );
        expect(find.text(arabicCopy), findsOneWidget);
        // Let the notice expire before the next action.
        await tester.pump(const Duration(seconds: 5));
        await tester.pump();
      }

      // Zero navigation, controller/API calls, or state mutation (FR-032).
      expect(find.byType(ProfileScreen), findsOneWidget);
      expect(profile.loadCalls, 1, reason: 'only the initial profile load');
      expect(profile.updateCalled, isFalse);
      expect(auth.logoutCalled, isFalse);
    });
  });

  group('T047 review fixes (localized values, visible focus)', () {
    testWidgets(
        'circle detail shows a localized language name, never the '
        'raw code (LTR and RTL)', (tester) async {
      for (final (direction, expected) in [
        (TextDirection.ltr, 'Arabic'),
        (TextDirection.rtl, 'العربية'),
      ]) {
        await _pumpCircleDetail(
          tester,
          direction: direction,
          sessionsSpy: _SpyCircleSessionsController(),
          routes: _RouteCounter(),
          onDetailFetch: () {},
        );
        await tester.pumpAndSettle();
        expect(find.text(expected), findsOneWidget);
        expect(
          find.text('ar'),
          findsNothing,
          reason: 'the raw database language code must not leak into copy',
        );
      }
    });

    test('button themes expose a visible 2dp focus border in both schemes', () {
      final cases = [
        (halaqatyLightTheme(), HalaqatyColors.primaryDark),
        (halaqatyDarkTheme(), HalaqatyColors.primaryLight),
      ];
      for (final (theme, color) in cases) {
        for (final style in [
          theme.filledButtonTheme.style,
          theme.outlinedButtonTheme.style,
          theme.elevatedButtonTheme.style,
          theme.textButtonTheme.style,
        ]) {
          final side = style?.side?.resolve({WidgetState.focused});
          expect(
            side,
            isNotNull,
            reason: 'the 10% state-layer overlay alone is not a visible '
                'focus indicator (WCAG 2.4.7)',
          );
          expect(side!.width, 2);
          expect(side.color, color);
        }
      }
    });
  });
}
