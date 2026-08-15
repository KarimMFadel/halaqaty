import 'dart:async';

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_detail_controller.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_discovery_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_detail_screen.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_discovery_screen.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_join_screen.dart';

import '../../helpers/stub_auth_notifier.dart';

final _publicCircle = CircleSummary(
  id: 'circle-public',
  name: 'حلقة النور',
  description: 'حفظ القرآن',
  maxCapacity: 20,
  genderRestriction: 'female',
  language: 'ar',
  createdAt: DateTime.utc(2026, 8, 1),
);

final _memberCircle = CircleSummary(
  id: 'circle-member',
  name: 'حلقتي',
  description: 'حلقة مسجل بها',
  maxCapacity: 20,
  genderRestriction: 'unspecified',
  language: 'ar',
  createdAt: DateTime.utc(2026, 8, 1),
);

class _StubCircleApiClient extends CircleApiClient {
  _StubCircleApiClient({this.discoveryCompleter}) : super(Dio());

  final Completer<CircleDiscoveryPage>? discoveryCompleter;
  List<CircleSummary> discovered = [_publicCircle];
  List<CircleSummary> memberships = const [];
  DioException? joinError;
  String? joinedCircleId;
  String? joinedInviteCode;

  @override
  Future<List<CircleSummary>> listCircles({
    required String firebaseIdToken,
    required String sessionId,
  }) async =>
      memberships;

  @override
  Future<CircleDiscoveryPage> discoverCircles({
    required String firebaseIdToken,
    required String sessionId,
    String? query,
    String? cursor,
  }) async {
    if (discoveryCompleter case final completer?) return completer.future;
    return CircleDiscoveryPage(circles: discovered);
  }

  @override
  Future<CircleResponse> joinPublicCircle({
    required String firebaseIdToken,
    required String sessionId,
    required String circleId,
  }) async {
    joinedCircleId = circleId;
    if (joinError case final error?) throw error;
    return _joinedCircle(circleId, _publicCircle.name);
  }

  @override
  Future<CircleResponse> joinCircleByInvite({
    required String firebaseIdToken,
    required String sessionId,
    required String inviteCode,
  }) async {
    joinedInviteCode = inviteCode;
    if (joinError case final error?) throw error;
    return _joinedCircle('circle-private', 'حلقة خاصة');
  }
}

CircleResponse _joinedCircle(String id, String name) => CircleResponse(
      id: id,
      name: name,
      inviteCode: 'HLQ-7X2K',
      inviteLink: 'https://halaqaty.app/join/HLQ-7X2K',
      createdAt: DateTime.utc(2026, 8, 1),
    );

CircleDiscoveryController _controller(_StubCircleApiClient apiClient) {
  return CircleDiscoveryController(
    apiClient: apiClient,
    loadFirebaseIdToken: () async => 'firebase-token',
    readAuthState: () => const AuthState(sessionId: 'session-id'),
    logout: () async {},
  );
}

Widget _build(Widget child, CircleDiscoveryController controller) {
  return ProviderScope(
    overrides: [
      authControllerProvider.overrideWith((_) => StubAuthNotifier()),
      circleDiscoveryControllerProvider.overrideWith((_) => controller),
      circleDetailProvider('circle-member').overrideWith(
        (_) => Future.value(_joinedCircle('circle-member', 'حلقتي')),
      ),
    ],
    child: MaterialApp(
      home: Directionality(textDirection: TextDirection.rtl, child: child),
    ),
  );
}

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

Future<void> _submitInvite(WidgetTester tester, String input) async {
  await tester.enterText(find.byKey(const Key('circleInviteField')), input);
  await tester.tap(find.byKey(const Key('circleInviteSubmitButton')));
  await tester.pumpAndSettle();
  await tester.tap(find.byKey(const Key('confirmCircleJoinButton')));
  await tester.pumpAndSettle();
}

void main() {
  testWidgets('CircleDiscoveryScreen: renders redacted RTL public cards',
      (tester) async {
    final apiClient = _StubCircleApiClient();
    final semantics = tester.ensureSemantics();

    await tester.pumpWidget(
      _build(const CircleDiscoveryScreen(), _controller(apiClient)),
    );
    await tester.pumpAndSettle();

    expect(find.text('حلقة النور'), findsOneWidget);
    expect(find.text('حفظ القرآن'), findsOneWidget);
    expect(find.text('السعة: 20'), findsOneWidget);
    expect(find.bySemanticsLabel('حلقة عامة: حلقة النور'), findsOneWidget);
    expect(find.textContaining('HLQ-'), findsNothing);
    expect(find.textContaining('خاصة'), findsNothing);
    expect(find.textContaining('عضو'), findsNothing);
    semantics.dispose();
  });

  testWidgets('CircleDiscoveryScreen: shows loading and join confirmation',
      (tester) async {
    final completer = Completer<CircleDiscoveryPage>();
    final apiClient = _StubCircleApiClient(discoveryCompleter: completer);
    await tester.pumpWidget(
      _build(const CircleDiscoveryScreen(), _controller(apiClient)),
    );
    await tester.pump();

    expect(find.byKey(const Key('circleDiscoveryLoading')), findsOneWidget);
    completer.complete(CircleDiscoveryPage(circles: [_publicCircle]));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('joinCircle-circle-public')));
    await tester.pumpAndSettle();
    expect(find.text('هل تريد الانضمام إلى حلقة النور؟'), findsOneWidget);

    await tester.tap(find.byKey(const Key('confirmCircleJoinButton')));
    await tester.pumpAndSettle();
    expect(apiClient.joinedCircleId, 'circle-public');
    expect(find.text('تم الانضمام إلى الحلقة'), findsOneWidget);
  });

  testWidgets('CircleDiscoveryScreen: opens an authenticated member circle',
      (tester) async {
    final apiClient = _StubCircleApiClient()..memberships = [_memberCircle];
    await tester.pumpWidget(
      _build(const CircleDiscoveryScreen(), _controller(apiClient)),
    );
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('openCircle-circle-member')));
    await tester.pumpAndSettle();

    expect(find.byType(CircleDetailScreen), findsOneWidget);
  });

  testWidgets('CircleJoinScreen: rejects invalid invite before the API call',
      (tester) async {
    final apiClient = _StubCircleApiClient();
    await tester.pumpWidget(
      _build(const CircleJoinScreen(), _controller(apiClient)),
    );

    await tester.enterText(
      find.byKey(const Key('circleInviteField')),
      'https://halaqaty.app/join/not-valid',
    );
    await tester.tap(find.byKey(const Key('circleInviteSubmitButton')));
    await tester.pump();

    expect(find.text('رابط الدعوة غير صالح'), findsOneWidget);
    expect(apiClient.joinedInviteCode, isNull);
  });

  for (final errorCase in <(String, String)>[
    ('user is already a circle member', 'أنت عضو في هذه الحلقة بالفعل'),
    ('circle has reached its maximum capacity', 'الحلقة مكتملة السعة'),
    ('circle is archived', 'هذه الحلقة مؤرشفة ومتاحة للقراءة فقط'),
    (
      'user has reached the maximum of 5 circles',
      'لا يمكنك الانضمام إلى أكثر من 5 حلقات',
    ),
  ]) {
    testWidgets('CircleJoinScreen: shows the ${errorCase.$1} state',
        (tester) async {
      final apiClient = _StubCircleApiClient()
        ..joinError = _conflict(errorCase.$1);
      await tester.pumpWidget(
        _build(const CircleJoinScreen(), _controller(apiClient)),
      );

      await _submitInvite(tester, 'HLQ-7X2K');

      expect(find.text(errorCase.$2), findsOneWidget);
    });
  }
}
