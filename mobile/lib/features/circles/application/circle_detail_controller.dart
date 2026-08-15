import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';

typedef CircleCredentials = ({String token, String sessionId});

Future<CircleCredentials> _loadCircleCredentials(Ref ref) async {
  final authState = ref.watch(authControllerProvider);
  final sessionId = authState.sessionId;
  if (!authState.isAuthenticated || sessionId == null || sessionId.isEmpty) {
    throw StateError('User not authenticated');
  }

  final firebaseAuth = ref.watch(firebaseAuthProvider);
  final token = await firebaseAuth.currentUser?.getIdToken();
  if (token == null || token.isEmpty) {
    throw StateError('No auth token available');
  }

  return (token: token, sessionId: sessionId);
}

final circleCredentialsProvider = FutureProvider<CircleCredentials>(
  _loadCircleCredentials,
);

final circleSessionLogoutProvider = Provider<Future<void> Function()>((ref) {
  return ref.read(authControllerProvider.notifier).logout;
});

Future<T> _loadCircleResource<T>(
  Ref ref,
  Future<T> Function(CircleApiClient, CircleCredentials) request,
) async {
  final credentials = await ref.watch(circleCredentialsProvider.future);
  try {
    return await request(ref.watch(circleApiClientProvider), credentials);
  } on DioException catch (error) {
    if (error.response?.statusCode == 401) {
      await ref.read(circleSessionLogoutProvider)();
    }
    rethrow;
  }
}

final circleDetailProvider =
    FutureProvider.family<CircleResponse, String>((ref, circleId) async {
  return _loadCircleResource(
    ref,
    (apiClient, credentials) => apiClient.getCircle(
      firebaseIdToken: credentials.token,
      sessionId: credentials.sessionId,
      circleId: circleId,
    ),
  );
});

final circleMembersProvider =
    FutureProvider.family<List<CircleMember>, String>((ref, circleId) async {
  return _loadCircleResource(
    ref,
    (apiClient, credentials) => apiClient.listCircleMembers(
      firebaseIdToken: credentials.token,
      sessionId: credentials.sessionId,
      circleId: circleId,
    ),
  );
});
