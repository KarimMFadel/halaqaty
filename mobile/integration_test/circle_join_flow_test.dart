import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_discovery_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_discovery_screen.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_join_screen.dart';
import 'package:integration_test/integration_test.dart';

final _publicCircle = CircleSummary(
  id: 'public-circle',
  name: 'حلقة عامة',
  description: 'للحفظ اليومي',
  maxCapacity: 50,
  genderRestriction: 'mixed',
  language: 'ar',
  createdAt: DateTime.utc(2026, 8, 1),
);

class _FlowApiClient extends CircleApiClient {
  _FlowApiClient() : super(Dio());

  @override
  Future<CircleDiscoveryPage> discoverCircles({
    required String firebaseIdToken,
    required String sessionId,
    String? query,
    String? cursor,
  }) async =>
      CircleDiscoveryPage(circles: [_publicCircle]);

  @override
  Future<CircleResponse> joinPublicCircle({
    required String firebaseIdToken,
    required String sessionId,
    required String circleId,
  }) async =>
      _response(circleId, _publicCircle.name);

  @override
  Future<CircleResponse> joinCircleByInvite({
    required String firebaseIdToken,
    required String sessionId,
    required String inviteCode,
  }) async {
    if (inviteCode == 'HLQ-DUP2') {
      throw _conflict('user is already a circle member');
    }
    if (inviteCode == 'HLQ-LMT5') {
      throw _conflict('user has reached the maximum of 5 circles');
    }
    return _response('private-circle', 'حلقة خاصة');
  }
}

CircleResponse _response(String id, String name) => CircleResponse(
      id: id,
      name: name,
      inviteCode: 'HLQ-7X2K',
      inviteLink: 'https://halaqaty.app/join/HLQ-7X2K',
      createdAt: DateTime.utc(2026, 8, 1),
    );

DioException _conflict(String message) => DioException(
      requestOptions: RequestOptions(path: '/circles/join'),
      response: Response<Map<String, dynamic>>(
        requestOptions: RequestOptions(path: '/circles/join'),
        statusCode: 409,
        data: {
          'error': {'code': 'ERR_CONFLICT', 'message': message},
        },
      ),
    );

CircleDiscoveryController _controller() => CircleDiscoveryController(
      apiClient: _FlowApiClient(),
      loadFirebaseIdToken: () async => 'firebase-token',
      readAuthState: () => const AuthState(sessionId: 'session-id'),
      logout: () async {},
    );

Widget _app(CircleDiscoveryController controller, Widget home) => ProviderScope(
      overrides: [
        circleDiscoveryControllerProvider.overrideWith((_) => controller),
      ],
      child: MaterialApp(
        builder: (context, child) => Directionality(
          textDirection: TextDirection.rtl,
          child: child!,
        ),
        home: home,
      ),
    );

Future<void> _joinInvite(WidgetTester tester, String code) async {
  await tester.enterText(find.byKey(const Key('circleInviteField')), code);
  await tester.tap(find.byKey(const Key('circleInviteSubmitButton')));
  await tester.pumpAndSettle();
  await tester.tap(find.byKey(const Key('confirmCircleJoinButton')));
  await tester.pumpAndSettle();
}

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets('Circle join flow: supports public and private joins',
      (tester) async {
    final controller = _controller();

    await tester.pumpWidget(_app(controller, const CircleDiscoveryScreen()));
    await tester.pumpAndSettle();
    expect(find.text('حلقة عامة'), findsOneWidget);

    await tester.tap(find.byKey(const Key('joinCircle-public-circle')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('confirmCircleJoinButton')));
    await tester.pumpAndSettle();
    expect(controller.state.myCircles.single.id, 'public-circle');

    await tester.tap(find.byKey(const Key('openInviteJoinButton')));
    await tester.pumpAndSettle();
    await _joinInvite(tester, 'https://halaqaty.app/join/HLQ-7X2K');
    expect(controller.state.myCircles.last.id, 'private-circle');
  });

  for (final errorCase in <(String, CircleJoinFailure)>[
    ('HLQ-DUP2', CircleJoinFailure.alreadyMember),
    ('HLQ-LMT5', CircleJoinFailure.membershipLimit),
  ]) {
    testWidgets('Circle join flow: exposes ${errorCase.$2.name}',
        (tester) async {
      final controller = _controller();
      await tester.pumpWidget(_app(controller, const CircleJoinScreen()));

      await _joinInvite(tester, errorCase.$1);

      expect(controller.state.failure, errorCase.$2);
      expect(find.byKey(const Key('circleJoinError')), findsOneWidget);
    });
  }
}
