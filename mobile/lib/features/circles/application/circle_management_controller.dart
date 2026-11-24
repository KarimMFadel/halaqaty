import 'package:dio/dio.dart';
import 'package:firebase_auth/firebase_auth.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_detail_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';

class CircleManagementController {
  CircleManagementController(this._ref);

  final Ref _ref;

  Future<bool> assignRole(
    String circleId,
    String userId,
    CircleRole role,
  ) =>
      _mutate(circleId, (apiClient, credentials) async {
        await apiClient.assignMemberRole(
          firebaseIdToken: credentials.token,
          sessionId: credentials.sessionId,
          circleId: circleId,
          userId: userId,
          request: AssignCircleRoleRequest(role: role),
        );
      });

  Future<bool> inviteMember(
    String circleId,
    String userId,
    CircleRole role,
  ) =>
      _mutate(circleId, (apiClient, credentials) async {
        await apiClient.createCircleInvitation(
          firebaseIdToken: credentials.token,
          sessionId: credentials.sessionId,
          circleId: circleId,
          recipientUserId: userId,
          role: role,
        );
      });

  Future<List<CircleUser>> searchUsers(String query) async {
    try {
      final credentials = await _ref.read(circleCredentialsProvider.future);
      return await _ref.read(circleApiClientProvider).searchUsers(
            firebaseIdToken: credentials.token,
            sessionId: credentials.sessionId,
            query: query,
          );
    } on DioException catch (error) {
      if (error.response?.statusCode == 401) {
        await _ref.read(circleSessionLogoutProvider)();
      }
      return const [];
    } on FirebaseAuthException {
      return const [];
    } on StateError {
      return const [];
    }
  }

  Future<bool> revokeInvitation(String circleId, String invitationId) =>
      _mutate(circleId, (apiClient, credentials) async {
        await apiClient.revokeCircleInvitation(
          firebaseIdToken: credentials.token,
          sessionId: credentials.sessionId,
          circleId: circleId,
          invitationId: invitationId,
        );
      });

  Future<bool> acceptInvitation(String circleId, String invitationId) =>
      _mutate(circleId, (apiClient, credentials) async {
        await apiClient.acceptCircleInvitation(
          firebaseIdToken: credentials.token,
          sessionId: credentials.sessionId,
          invitationId: invitationId,
        );
      });

  Future<bool> removeMember(String circleId, String userId) =>
      _mutate(circleId, (apiClient, credentials) async {
        await apiClient.removeMember(
          firebaseIdToken: credentials.token,
          sessionId: credentials.sessionId,
          circleId: circleId,
          userId: userId,
        );
      });

  Future<bool> refreshInvite(String circleId) =>
      _mutate(circleId, (apiClient, credentials) async {
        await apiClient.refreshInviteCode(
          firebaseIdToken: credentials.token,
          sessionId: credentials.sessionId,
          circleId: circleId,
        );
      });

  Future<bool> _mutate(
    String circleId,
    Future<void> Function(CircleApiClient, CircleCredentials) request,
  ) async {
    try {
      final credentials = await _ref.read(circleCredentialsProvider.future);
      await request(_ref.read(circleApiClientProvider), credentials);
      _refresh(circleId);
      return true;
    } on DioException catch (error) {
      if (error.response?.statusCode == 401) {
        await _ref.read(circleSessionLogoutProvider)();
      }
      return false;
    } on FirebaseAuthException {
      return false;
    } on StateError {
      return false;
    }
  }

  void _refresh(String circleId) {
    _ref.invalidate(circleDetailProvider(circleId));
    _ref.invalidate(circleMembersProvider(circleId));
    _ref.invalidate(circleInvitationsProvider(circleId));
    _ref.invalidate(myCircleInvitationsProvider);
  }
}

final circleManagementControllerProvider = Provider(
  CircleManagementController.new,
);
