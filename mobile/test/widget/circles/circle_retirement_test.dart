import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_detail_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_retirement_screen.dart';

class _ArchiveApiClient extends CircleApiClient {
  _ArchiveApiClient() : super(Dio());

  @override
  Future<void> archiveCircle({
    required String firebaseIdToken,
    required String sessionId,
    required String circleId,
  }) async =>
      throw DioException(
        requestOptions: RequestOptions(path: '/circles/$circleId'),
      );
}

Widget _build({
  CircleRole role = CircleRole.teacher,
  bool isArchived = false,
  CircleApiClient? apiClient,
  bool missingCredentials = false,
}) =>
    ProviderScope(
      overrides: [
        circleCredentialsProvider.overrideWith(
          (_) => missingCredentials
              ? Future.error(StateError('missing credentials'))
              : Future.value((token: 'token', sessionId: 'session')),
        ),
        if (apiClient != null)
          circleApiClientProvider.overrideWithValue(apiClient),
        circleDetailProvider('circle-1').overrideWith(
          (_) => Future.value(CircleResponse(
            id: 'circle-1',
            name: 'حلقة النور',
            inviteCode: 'HLQ-7X2K',
            inviteLink: 'https://halaqaty.app/join/HLQ-7X2K',
            createdAt: DateTime.utc(2026, 8, 1),
            isArchived: isArchived,
          )),
        ),
        circleMembersProvider('circle-1').overrideWith(
          (_) => Future.value([
            CircleMember(
              userId: 'current-user',
              displayName: 'مريم',
              role: role,
              joinedAt: DateTime.utc(2026, 8, 1),
            ),
          ]),
        ),
      ],
      child: const MaterialApp(
        home: CircleRetirementScreen(
          circleId: 'circle-1',
          currentUserId: 'current-user',
        ),
      ),
    );

void main() {
  testWidgets('CircleRetirementScreen confirms archive before retirement',
      (tester) async {
    await tester.pumpWidget(_build());
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('archiveCircleButton')));
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('confirmCircleArchive')), findsOneWidget);
  });

  testWidgets('CircleRetirementScreen hides archive for archived circles',
      (tester) async {
    await tester.pumpWidget(_build(isArchived: true));
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('archiveCircleButton')), findsNothing);
  });

  testWidgets('CircleRetirementScreen hides archive for non-teachers',
      (tester) async {
    await tester.pumpWidget(_build(role: CircleRole.supervisor));
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('archiveCircleButton')), findsNothing);
  });

  testWidgets('CircleRetirementScreen renders a safe archive failure',
      (tester) async {
    await tester.pumpWidget(_build(apiClient: _ArchiveApiClient()));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('archiveCircleButton')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('confirmCircleArchive')));
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('circleMutationError')), findsOneWidget);
    expect(find.textContaining('تعذر'), findsWidgets);
  });

  testWidgets('CircleRetirementScreen renders missing credentials safely',
      (tester) async {
    await tester.pumpWidget(_build(missingCredentials: true));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('archiveCircleButton')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('confirmCircleArchive')));
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('circleMutationError')), findsOneWidget);
  });
}
