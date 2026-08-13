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
  }
}

final circleManagementControllerProvider = Provider(
  CircleManagementController.new,
);
