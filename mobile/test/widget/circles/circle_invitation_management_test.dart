import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_detail_controller.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_discovery_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_invitation_inbox_screen.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_invitation_management_screen.dart';

class _InvitationApi extends CircleApiClient {
  _InvitationApi() : super(Dio());

  String? invitedUserId;
  CircleRole? invitedRole;
  String? acceptedInvitationId;

  @override
  Future<List<CircleUser>> searchUsers({
    required String firebaseIdToken,
    required String sessionId,
    required String query,
  }) async =>
      [const CircleUser(id: 'user-1', displayName: 'Amina')];

  @override
  Future<CircleInvitation> createCircleInvitation({
    required String firebaseIdToken,
    required String sessionId,
    required String circleId,
    required String recipientUserId,
    required CircleRole role,
  }) async {
    invitedUserId = recipientUserId;
    invitedRole = role;
    return CircleInvitation(
      id: 'invitation-1',
      circleId: circleId,
      inviterUserId: 'manager-1',
      recipientUserId: recipientUserId,
      role: role,
      state: 'pending',
      createdAt: DateTime.utc(2026, 11, 22),
    );
  }

  @override
  Future<List<CircleInvitation>> listCircleInvitations({
    required String firebaseIdToken,
    required String sessionId,
    required String circleId,
  }) async =>
      const [];

  @override
  Future<CircleInvitation> acceptCircleInvitation({
    required String firebaseIdToken,
    required String sessionId,
    required String invitationId,
  }) async {
    acceptedInvitationId = invitationId;
    return CircleInvitation(
      id: invitationId,
      circleId: 'circle-1',
      inviterUserId: 'manager-1',
      recipientUserId: 'recipient-1',
      role: CircleRole.teacher,
      state: 'accepted',
      createdAt: DateTime.utc(2026, 11, 22),
    );
  }

  @override
  Future<List<CircleSummary>> listCircles({
    required String firebaseIdToken,
    required String sessionId,
  }) async =>
      const [];
}

void main() {
  testWidgets('manager can send a named teacher invitation', (tester) async {
    final api = _InvitationApi();
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          circleCredentialsProvider.overrideWith(
            (_) => Future.value((token: 'token', sessionId: 'session')),
          ),
          circleApiClientProvider.overrideWithValue(api),
        ],
        child: const MaterialApp(
          home: Directionality(
            textDirection: TextDirection.rtl,
            child: CircleInvitationManagementScreen(
              circleId: 'circle-1',
              canInviteTeacher: true,
              canRevokeInvitations: true,
            ),
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();

    await tester.enterText(
        find.byKey(const Key('circleInvitationSearch')), 'Am');
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('circleInvitationRole')));
    await tester.pumpAndSettle();
    await tester.tap(find.text('معلم').last);
    await tester.pumpAndSettle();
    await tester.tap(find.byTooltip('إرسال الدعوة'));
    await tester.pumpAndSettle();

    expect(api.invitedUserId, 'user-1');
    expect(api.invitedRole, CircleRole.teacher);
    expect(find.text('أُرسلت دعوة المعلم؛ يلزم قبولها'), findsOneWidget);
  });

  testWidgets('recipient accepts a named teacher invitation', (tester) async {
    final api = _InvitationApi();
    final invitation = CircleInvitation(
      id: 'invitation-1',
      circleId: 'circle-1',
      inviterUserId: 'manager-1',
      recipientUserId: 'recipient-1',
      role: CircleRole.teacher,
      state: 'pending',
      createdAt: DateTime.utc(2026, 11, 22),
    );
    final discoveryController = CircleDiscoveryController(
      apiClient: api,
      loadFirebaseIdToken: () async => 'token',
      readAuthState: () => const AuthState(sessionId: 'session'),
      logout: () async {},
    );
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          circleCredentialsProvider.overrideWith(
            (_) => Future.value((token: 'token', sessionId: 'session')),
          ),
          circleApiClientProvider.overrideWithValue(api),
          myCircleInvitationsProvider.overrideWith(
            (_) => Future.value([invitation]),
          ),
          circleDiscoveryControllerProvider.overrideWith(
            (_) => discoveryController,
          ),
        ],
        child: const MaterialApp(
          home: Directionality(
            textDirection: TextDirection.rtl,
            child: CircleInvitationInboxScreen(),
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('دعوة لتولي دور المعلم'), findsOneWidget);
    await tester.tap(find.byKey(const Key('acceptInvitation-invitation-1')));
    await tester.pumpAndSettle();

    expect(api.acceptedInvitationId, 'invitation-1');
    expect(find.text('تم قبول الدعوة'), findsOneWidget);
  });
}
