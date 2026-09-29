import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/sessions/application/circle_sessions_controller.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/presentation/circle_sessions_section.dart';

void main() {
  testWidgets('archived circle keeps completed attendance history reachable',
      (tester) async {
    final controller = CircleSessionsController(
      _EndedSessionApi(),
      () async => (token: 'token', sessionId: 'auth-session'),
      circleId: 'circle-1',
    );
    await tester.pumpWidget(ProviderScope(
      overrides: [
        circleSessionsControllerProvider('circle-1')
            .overrideWith((_) => controller),
      ],
      child: const MaterialApp(
        home: Scaffold(
          body: SingleChildScrollView(
            child: CircleSessionsSection(
              circleId: 'circle-1',
              isManager: false,
              isArchived: true,
            ),
          ),
        ),
      ),
    ));
    await tester.pumpAndSettle();

    final tile = tester.widget<ListTile>(
      find.byKey(const Key('circleSession-session-1')),
    );
    expect(find.text('Completed'), findsOneWidget);
    expect(tile.enabled, isTrue);
    expect(tile.onTap, isNotNull);
  });
}

class _EndedSessionApi extends SessionApiClient {
  _EndedSessionApi() : super(Dio());

  @override
  Future<List<SessionModel>> list({
    required String token,
    required String sessionId,
    required String circleId,
  }) async =>
      const [
        SessionModel(
          id: 'session-1',
          circleId: 'circle-1',
          status: 'ended',
          mediaMode: 'audio',
          participantCount: 1,
          isLocked: false,
        ),
      ];
}
