import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_detail_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_detail_screen.dart';
import 'package:integration_test/integration_test.dart';

const _circleID = 'circle-1';
const _teacherID = 'teacher-1';

class _RetirementFlowApiClient extends CircleApiClient {
  _RetirementFlowApiClient() : super(Dio());

  bool archived = false;

  @override
  Future<CircleResponse> getCircle({
    required String firebaseIdToken,
    required String sessionId,
    required String circleId,
  }) async =>
      CircleResponse(
        id: circleId,
        name: 'حلقة النور',
        inviteCode: 'HLQ-7X2K',
        inviteLink: 'https://halaqaty.app/join/HLQ-7X2K',
        createdAt: DateTime.utc(2026, 8, 1),
        isArchived: archived,
      );

  @override
  Future<List<CircleMember>> listCircleMembers({
    required String firebaseIdToken,
    required String sessionId,
    required String circleId,
  }) async =>
      [
        CircleMember(
          userId: _teacherID,
          displayName: 'المعلمة مريم',
          role: CircleRole.teacher,
          joinedAt: DateTime.utc(2026, 8, 1),
        ),
      ];

  @override
  Future<void> archiveCircle({
    required String firebaseIdToken,
    required String sessionId,
    required String circleId,
  }) async {
    archived = true;
  }
}

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets('Circle retirement flow: confirms archive then becomes read-only',
      (tester) async {
    final apiClient = _RetirementFlowApiClient();
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          circleApiClientProvider.overrideWithValue(apiClient),
          circleCredentialsProvider.overrideWith(
            (_) => Future.value((token: 'token', sessionId: 'session')),
          ),
        ],
        child: const MaterialApp(
          home: Directionality(
            textDirection: TextDirection.rtl,
            child: CircleDetailScreen(
              circleId: _circleID,
              currentUserId: _teacherID,
            ),
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('openCircleRetirement')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('archiveCircleButton')));
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('confirmCircleArchive')), findsOneWidget);

    await tester.tap(find.byKey(const Key('confirmCircleArchive')));
    await tester.pumpAndSettle();

    expect(find.text('الحلقة مؤرشفة ومتاحة للقراءة فقط'), findsOneWidget);
    expect(find.byKey(const Key('archiveCircleButton')), findsNothing);
  });
}
