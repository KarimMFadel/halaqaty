import 'package:dio/dio.dart';
import 'package:firebase_auth/firebase_auth.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_detail_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';

class CircleRetirementController {
  CircleRetirementController(this._ref);

  final Ref _ref;

  Future<bool> archive(String circleId) async {
    try {
      final credentials = await _ref.read(circleCredentialsProvider.future);
      await _ref.read(circleApiClientProvider).archiveCircle(
            firebaseIdToken: credentials.token,
            sessionId: credentials.sessionId,
            circleId: circleId,
          );
      _ref.invalidate(circleDetailProvider(circleId));
      _ref.invalidate(circleMembersProvider(circleId));
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
}

final circleRetirementControllerProvider = Provider(
  CircleRetirementController.new,
);
