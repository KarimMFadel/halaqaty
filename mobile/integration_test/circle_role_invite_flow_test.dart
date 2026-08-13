import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_detail_controller.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_discovery_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_join_screen.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_management_screen.dart';
import 'package:integration_test/integration_test.dart';

const _circleID = 'circle-1';
const _teacherID = 'teacher-1';
const _studentID = 'student-1';
const _oldInviteCode = 'HLQ-7X2K';
const _newInviteCode = 'HLQ-9Y3M';

class _RoleInviteFlowApiClient extends CircleApiClient {
  _RoleInviteFlowApiClient() : super(Dio());

  String activeInviteCode = _oldInviteCode;
  CircleRole studentRole = CircleRole.student;

  CircleResponse get circle => CircleResponse(
        id: _circleID,
        name: 'حلقة النور',
        inviteCode: activeInviteCode,
        inviteLink: 'https://halaqaty.app/join/$activeInviteCode',
        createdAt: DateTime.utc(2026, 8, 1),
      );

  @override
  Future<CircleResponse> getCircle({
    required String firebaseIdToken,
    required String sessionId,
    required String circleId,
  }) async =>
      circle;

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
        CircleMember(
          userId: _studentID,
          displayName: 'أحمد',
          role: studentRole,
          joinedAt: DateTime.utc(2026, 8, 2),
        ),
      ];

  @override
  Future<CircleRoleAssignmentResponse> assignMemberRole({
    required String firebaseIdToken,
    required String sessionId,
    required String circleId,
    required String userId,
    required AssignCircleRoleRequest request,
  }) async {
    studentRole = request.role;
    return CircleRoleAssignmentResponse(
      circleId: circleId,
      userId: userId,
      role: request.role,
    );
  }

  @override
  Future<CircleInviteResponse> refreshInviteCode({
    required String firebaseIdToken,
    required String sessionId,
    required String circleId,
  }) async {
    activeInviteCode = _newInviteCode;
    return CircleInviteResponse(
      inviteCode: activeInviteCode,
      inviteLink: 'https://halaqaty.app/join/$activeInviteCode',
    );
  }

  @override
  Future<CircleResponse> joinCircleByInvite({
    required String firebaseIdToken,
    required String sessionId,
    required String inviteCode,
  }) async {
    if (inviteCode != activeInviteCode) {
      throw DioException(
        requestOptions: RequestOptions(path: '/circles/join'),
        response: Response<Map<String, dynamic>>(
          requestOptions: RequestOptions(path: '/circles/join'),
          statusCode: 404,
          data: const {
            'error': {
              'code': 'ERR_CIRCLE_NOT_FOUND',
              'message': 'circle not found',
            },
          },
        ),
      );
    }
    return circle;
  }
}

Widget _managementApp(_RoleInviteFlowApiClient apiClient) => ProviderScope(
      overrides: [
        circleApiClientProvider.overrideWithValue(apiClient),
        circleCredentialsProvider.overrideWith(
          (_) => Future.value((token: 'token', sessionId: 'session')),
        ),
      ],
      child: const MaterialApp(
        home: Directionality(
          textDirection: TextDirection.rtl,
          child: CircleManagementScreen(
            circleId: _circleID,
            currentUserId: _teacherID,
          ),
        ),
      ),
    );

Widget _joinApp(_RoleInviteFlowApiClient apiClient) {
  final controller = CircleDiscoveryController(
    apiClient: apiClient,
    loadFirebaseIdToken: () async => 'token',
    readAuthState: () => const AuthState(sessionId: 'session'),
    logout: () async {},
  );
  return ProviderScope(
    overrides: [
      circleDiscoveryControllerProvider.overrideWith((_) => controller),
    ],
    child: const MaterialApp(
      home: Directionality(
        textDirection: TextDirection.rtl,
        child: CircleJoinScreen(),
      ),
    ),
  );
}

Future<void> _confirmInviteJoin(WidgetTester tester, String inviteCode) async {
  await tester.enterText(
    find.byKey(const Key('circleInviteField')),
    inviteCode,
  );
  await tester.tap(find.byKey(const Key('circleInviteSubmitButton')));
  await tester.pumpAndSettle();
  await tester.tap(find.byKey(const Key('confirmCircleJoinButton')));
  await tester.pumpAndSettle();
}

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets(
      'Circle role and invite flow: changes role, rotates invite, and rejects old link',
      (tester) async {
    final apiClient = _RoleInviteFlowApiClient();
    await tester.pumpWidget(_managementApp(apiClient));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('manageRole-student-1')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('circleRole-supervisor')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('confirmCircleRoleChange')));
    await tester.pumpAndSettle();
    expect(find.text('مشرف'), findsOneWidget);

    await tester.tap(find.byKey(const Key('shareCircleInvite')));
    await tester.pumpAndSettle();
    final clipboard = await Clipboard.getData(Clipboard.kTextPlain);
    expect(clipboard?.text, 'https://halaqaty.app/join/$_oldInviteCode');

    await tester.tap(find.byKey(const Key('refreshCircleInvite')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('confirmCircleInviteRefresh')));
    await tester.pumpAndSettle();
    expect(find.text('https://halaqaty.app/join/$_newInviteCode'), findsOneWidget);

    await tester.pumpWidget(_joinApp(apiClient));
    await tester.pumpAndSettle();
    await _confirmInviteJoin(tester, _oldInviteCode);
    expect(find.byKey(const Key('circleJoinError')), findsOneWidget);
  });
}
