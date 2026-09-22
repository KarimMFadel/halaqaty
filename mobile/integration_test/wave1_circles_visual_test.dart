import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';
import 'package:halaqaty_mobile/core/theme/halaqaty_theme.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_detail_controller.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_discovery_controller.dart';
import 'package:halaqaty_mobile/features/circles/application/create_circle_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_detail_screen.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_discovery_screen.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_join_screen.dart';
import 'package:halaqaty_mobile/features/circles/presentation/create_circle_screen.dart';

import '../test/helpers/stub_auth_notifier.dart';

/// Wave 1 (US2) visual evidence harness: renders the circles surfaces with
/// stubbed providers and captures discovery (loaded/empty/error), circle
/// detail (student/manager/archived), and the create/join forms in Arabic
/// RTL / English LTR x light / dark.
class _Wave1CircleApi extends CircleApiClient {
  _Wave1CircleApi() : super(Dio());

  static CircleSummary memberCircle(Locale locale) => CircleSummary(
        id: 'circle-member',
        name: locale.languageCode == 'ar' ? 'حلقة الإتقان' : 'Mastery Circle',
        description: locale.languageCode == 'ar'
            ? 'تلاوة وحفظ'
            : 'Recitation and memorization',
        maxCapacity: 20,
        genderRestriction: 'mixed',
        language: locale.languageCode,
        createdAt: DateTime.utc(2026, 1, 1),
      );

  static CircleSummary publicCircle(Locale locale) => CircleSummary(
        id: 'circle-public',
        name: locale.languageCode == 'ar' ? 'حلقة النور' : 'Light Circle',
        description:
            locale.languageCode == 'ar' ? 'حفظ القرآن' : 'Quran memorization',
        maxCapacity: 50,
        genderRestriction: 'female',
        language: locale.languageCode,
        createdAt: DateTime.utc(2026, 1, 1),
      );

  static CircleResponse detail(Locale locale, {bool archived = false}) =>
      CircleResponse(
        id: 'circle-member',
        name: locale.languageCode == 'ar' ? 'حلقة الإتقان' : 'Mastery Circle',
        inviteCode: 'HLQ-7X2K',
        inviteLink: 'https://halaqaty.app/join/HLQ-7X2K',
        maxCapacity: 20,
        language: locale.languageCode,
        isArchived: archived,
        createdAt: DateTime.utc(2026, 1, 1),
      );

  List<CircleSummary> memberships = const [];
  List<CircleSummary> discovered = const [];
  bool failLoads = false;

  @override
  Future<List<CircleSummary>> listCircles({
    required String firebaseIdToken,
    required String sessionId,
  }) {
    if (failLoads) return _throwNetwork();
    return Future.value(memberships);
  }

  @override
  Future<CircleDiscoveryPage> discoverCircles({
    required String firebaseIdToken,
    required String sessionId,
    String? query,
    String? cursor,
  }) {
    if (failLoads) return _throwNetwork();
    return Future.value(CircleDiscoveryPage(circles: discovered));
  }

  Future<T> _throwNetwork<T>() => throw DioException(
        requestOptions: RequestOptions(path: '/circles'),
        type: DioExceptionType.connectionError,
      );
}

CircleDiscoveryController _discoveryController(_Wave1CircleApi api) {
  return CircleDiscoveryController(
    apiClient: api,
    loadFirebaseIdToken: () async => 'visual-token',
    readAuthState: () =>
        const AuthState(status: AuthStatus.authenticated, sessionId: 'visual'),
    logout: () async {},
  );
}

CircleMember _member(String userId, CircleRole role) => CircleMember(
      userId: userId,
      displayName: 'Karim',
      role: role,
      joinedAt: DateTime.utc(2026, 1, 1),
    );

void main() {
  final binding = IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  /// Pumps until [finder] stays visible for 3 consecutive frames.
  Future<void> pumpUntilStable(WidgetTester tester, Finder sentinel) async {
    var stable = 0;
    for (var i = 0; i < 60 && stable < 3; i++) {
      await tester.pump(const Duration(milliseconds: 200));
      stable = sentinel.evaluate().isNotEmpty ? stable + 1 : 0;
    }
    if (stable < 3) throw StateError('Sentinel never stabilized: $sentinel');
  }

  Future<void> pumpScreen(
    WidgetTester tester,
    Widget screen,
    Locale locale,
    Brightness brightness,
    List<Override> overrides,
  ) async {
    await tester.pumpWidget(const SizedBox.shrink());
    await tester.pump();
    await tester.pumpWidget(
      ProviderScope(
        overrides: overrides,
        child: MaterialApp(
          debugShowCheckedModeBanner: false,
          theme: halaqatyLightTheme(),
          darkTheme: halaqatyDarkTheme(),
          themeMode:
              brightness == Brightness.dark ? ThemeMode.dark : ThemeMode.light,
          home: Directionality(
            textDirection: locale.languageCode == 'ar'
                ? TextDirection.rtl
                : TextDirection.ltr,
            child: screen,
          ),
        ),
      ),
    );
  }

  Future<void> capture(
    WidgetTester tester,
    String state,
    Widget screen,
    Locale locale,
    Brightness brightness,
    List<Override> overrides,
    Finder sentinel,
  ) async {
    await pumpScreen(tester, screen, locale, brightness, overrides);
    await pumpUntilStable(tester, sentinel);
    await binding.takeScreenshot(
      'wave1_${state}_${locale.languageCode}_'
      '${brightness == Brightness.dark ? 'dark' : 'light'}',
    );
  }

  for (final locale in const [Locale('ar'), Locale('en')]) {
    testWidgets('wave1_circles_matrix_${locale.languageCode}', (tester) async {
      await binding.convertFlutterSurfaceToImage();
      for (final brightness in Brightness.values) {
        await capture(
          tester,
          'discovery_loaded',
          const CircleDiscoveryScreen(),
          locale,
          brightness,
          [
            authControllerProvider.overrideWith((_) => StubAuthNotifier()),
            circleDiscoveryControllerProvider.overrideWith(
              (_) => _discoveryController(
                _Wave1CircleApi()
                  ..memberships = [_Wave1CircleApi.memberCircle(locale)]
                  ..discovered = [_Wave1CircleApi.publicCircle(locale)],
              ),
            ),
          ],
          find.byKey(const Key('joinCircle-circle-public')),
        );

        await capture(
          tester,
          'discovery_empty',
          const CircleDiscoveryScreen(),
          locale,
          brightness,
          [
            authControllerProvider.overrideWith((_) => StubAuthNotifier()),
            circleDiscoveryControllerProvider.overrideWith(
              (_) => _discoveryController(_Wave1CircleApi()),
            ),
          ],
          find.text(
            locale.languageCode == 'ar'
                ? 'لا توجد حلقات عامة متاحة'
                : 'No public circles',
          ),
        );

        await capture(
          tester,
          'discovery_error',
          const CircleDiscoveryScreen(),
          locale,
          brightness,
          [
            authControllerProvider.overrideWith((_) => StubAuthNotifier()),
            circleDiscoveryControllerProvider.overrideWith(
              (_) => _discoveryController(_Wave1CircleApi()..failLoads = true),
            ),
          ],
          find.byKey(const Key('circleLoadError')),
        );

        for (final variant in [
          ('detail_student', CircleRole.student, false),
          ('detail_manager', CircleRole.teacher, false),
          ('detail_archived', CircleRole.teacher, true),
        ]) {
          await capture(
            tester,
            variant.$1,
            const CircleDetailScreen(
              circleId: 'circle-member',
              currentUserId: 'user-1',
            ),
            locale,
            brightness,
            [
              circleDetailProvider('circle-member').overrideWith(
                (_) => Future.value(
                  _Wave1CircleApi.detail(locale, archived: variant.$3),
                ),
              ),
              circleMembersProvider('circle-member').overrideWith(
                (_) => Future.value([_member('user-1', variant.$2)]),
              ),
            ],
            find.byKey(const Key('openCircleChat')),
          );
        }

        await capture(
          tester,
          'create_form',
          const CreateCircleScreen(),
          locale,
          brightness,
          [
            authControllerProvider.overrideWith((_) => StubAuthNotifier()),
            createCircleControllerProvider.overrideWith(
              (_) => CreateCircleController(
                apiClient: _Wave1CircleApi(),
                loadFirebaseIdToken: () async => 'visual-token',
                readAuthState: () => const AuthState(
                  status: AuthStatus.authenticated,
                  sessionId: 'visual',
                ),
                logout: () async {},
              ),
            ),
          ],
          find.byKey(const Key('createCircleNameField')),
        );

        await capture(
          tester,
          'join_form',
          const CircleJoinScreen(),
          locale,
          brightness,
          [
            authControllerProvider.overrideWith((_) => StubAuthNotifier()),
            circleDiscoveryControllerProvider.overrideWith(
              (_) => _discoveryController(_Wave1CircleApi()),
            ),
          ],
          find.byKey(const Key('circleInviteField')),
        );
      }
    });
  }
}
