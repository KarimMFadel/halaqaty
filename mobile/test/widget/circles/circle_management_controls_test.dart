import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_detail_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_management_screen.dart';

const _circleID = 'circle-1';
const _teacherID = 'teacher-1';
const _studentID = 'student-1';

class _StubCircleApiClient extends CircleApiClient {
  _StubCircleApiClient() : super(Dio());

  Never _fail() => throw DioException(
        requestOptions: RequestOptions(path: '/circle-mutation'),
      );

  @override
  Future<CircleRoleAssignmentResponse> assignMemberRole({
    required String firebaseIdToken,
    required String sessionId,
    required String circleId,
    required String userId,
    required AssignCircleRoleRequest request,
  }) async =>
      _fail();

  @override
  Future<void> removeMember({
    required String firebaseIdToken,
    required String sessionId,
    required String circleId,
    required String userId,
  }) async =>
      _fail();

  @override
  Future<CircleInviteResponse> refreshInviteCode({
    required String firebaseIdToken,
    required String sessionId,
    required String circleId,
  }) async =>
      _fail();
}

Widget _build({
  required String currentUserID,
  CircleRole currentRole = CircleRole.teacher,
  bool isArchived = false,
  CircleApiClient? apiClient,
  bool missingCredentials = false,
}) {
  return ProviderScope(
    overrides: [
      circleCredentialsProvider.overrideWith(
        (_) => missingCredentials
            ? Future.error(StateError('missing credentials'))
            : Future.value((token: 'token', sessionId: 'session')),
      ),
      if (apiClient != null)
        circleApiClientProvider.overrideWithValue(apiClient),
      circleDetailProvider(_circleID).overrideWith(
        (_) => Future.value(
          CircleResponse(
            id: _circleID,
            name: 'حلقة النور',
            inviteCode: 'HLQ-7X2K',
            inviteLink: 'https://halaqaty.app/join/HLQ-7X2K',
            createdAt: DateTime.utc(2026, 8, 1),
            isArchived: isArchived,
          ),
        ),
      ),
      circleMembersProvider(_circleID).overrideWith(
        (_) => Future.value([
          CircleMember(
            userId: _teacherID,
            displayName: 'المعلمة مريم',
            role:
                currentUserID == _teacherID ? currentRole : CircleRole.teacher,
            joinedAt: DateTime.utc(2026, 8, 1),
          ),
          CircleMember(
            userId: _studentID,
            displayName: 'أحمد',
            role: CircleRole.student,
            joinedAt: DateTime.utc(2026, 8, 2),
          ),
        ]),
      ),
    ],
    child: MaterialApp(
      home: Directionality(
        textDirection: TextDirection.rtl,
        child: CircleManagementScreen(
          circleId: _circleID,
          currentUserId: currentUserID,
        ),
      ),
    ),
  );
}

void main() {
  testWidgets(
      'CircleManagementScreen: teacher sees role and invite controls for another member',
      (tester) async {
    await tester.pumpWidget(_build(currentUserID: _teacherID));
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('manageRole-student-1')), findsOneWidget);
    expect(find.byKey(const Key('circleInviteLink')), findsOneWidget);
    expect(find.byKey(const Key('shareCircleInvite')), findsOneWidget);
    expect(find.byKey(const Key('refreshCircleInvite')), findsOneWidget);
    expect(find.byKey(const Key('removeMember-student-1')), findsOneWidget);
    expect(find.byKey(const Key('manageRole-teacher-1')), findsNothing);
  });

  testWidgets(
      'CircleManagementScreen: requires confirmation before changing another member role',
      (tester) async {
    await tester.pumpWidget(_build(currentUserID: _teacherID));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('manageRole-student-1')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('circleRole-supervisor')));
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('confirmCircleRoleChange')), findsOneWidget);
    expect(find.textContaining('أحمد'), findsWidgets);
    expect(find.textContaining('مشرف'), findsWidgets);
  });

  testWidgets(
      'CircleManagementScreen: shows the current invite link and asks before refreshing it',
      (tester) async {
    await tester.pumpWidget(_build(currentUserID: _teacherID));
    await tester.pumpAndSettle();

    expect(find.text('https://halaqaty.app/join/HLQ-7X2K'), findsOneWidget);
    await tester.tap(find.byKey(const Key('refreshCircleInvite')));
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('confirmCircleInviteRefresh')), findsOneWidget);
  });

  testWidgets('CircleManagementScreen: asks before removing another member',
      (tester) async {
    await tester.pumpWidget(_build(currentUserID: _teacherID));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('removeMember-student-1')));
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('confirmCircleMemberRemoval')), findsOneWidget);
    expect(find.textContaining('أحمد'), findsWidgets);
  });

  testWidgets(
      'CircleManagementScreen: student does not see management or invite-mutation controls',
      (tester) async {
    await tester.pumpWidget(_build(currentUserID: _studentID));
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('manageRole-teacher-1')), findsNothing);
    expect(find.byKey(const Key('refreshCircleInvite')), findsNothing);
    expect(find.byKey(const Key('shareCircleInvite')), findsNothing);
    expect(find.byKey(const Key('circleManagementDenied')), findsOneWidget);
  });

  testWidgets('CircleManagementScreen: supervisor cannot remove members',
      (tester) async {
    await tester.pumpWidget(_build(
      currentUserID: _teacherID,
      currentRole: CircleRole.supervisor,
    ));
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('manageRole-student-1')), findsOneWidget);
    expect(find.byKey(const Key('removeMember-student-1')), findsNothing);
    expect(find.byKey(const Key('circleInviteLink')), findsNothing);
    expect(find.byKey(const Key('shareCircleInvite')), findsNothing);
    expect(find.byKey(const Key('refreshCircleInvite')), findsNothing);
  });

  testWidgets('CircleManagementScreen: renders a safe role mutation failure',
      (tester) async {
    final apiClient = _StubCircleApiClient();
    await tester.pumpWidget(_build(
      currentUserID: _teacherID,
      apiClient: apiClient,
    ));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('manageRole-student-1')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('circleRole-supervisor')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('confirmCircleRoleChange')));
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('circleMutationError')), findsOneWidget);
    expect(find.textContaining('تعذر'), findsWidgets);
  });

  testWidgets('CircleManagementScreen: renders a safe invite refresh failure',
      (tester) async {
    final apiClient = _StubCircleApiClient();
    await tester.pumpWidget(_build(
      currentUserID: _teacherID,
      apiClient: apiClient,
    ));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('refreshCircleInvite')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('confirmCircleInviteRefresh')));
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('circleMutationError')), findsOneWidget);
  });

  testWidgets('CircleManagementScreen: renders missing credentials safely',
      (tester) async {
    await tester.pumpWidget(_build(
      currentUserID: _teacherID,
      missingCredentials: true,
    ));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('refreshCircleInvite')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('confirmCircleInviteRefresh')));
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('circleMutationError')), findsOneWidget);
  });
}
