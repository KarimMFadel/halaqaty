import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';

typedef _CircleCredentials = ({String token, String sessionId});

Future<_CircleCredentials> _loadCircleCredentials(Ref ref) async {
  final authState = ref.watch(authControllerProvider);
  if (!authState.isAuthenticated || authState.sessionId == null) {
    throw Exception('User not authenticated');
  }

  final firebaseAuth = ref.watch(firebaseAuthProvider);
  final token = await firebaseAuth.currentUser?.getIdToken();
  if (token == null) {
    throw Exception('No auth token available');
  }

  return (token: token, sessionId: authState.sessionId!);
}

final circleDetailProvider =
    FutureProvider.family<CircleResponse, String>((ref, circleId) async {
  final credentials = await _loadCircleCredentials(ref);
  final apiClient = ref.watch(circleApiClientProvider);
  return apiClient.getCircle(
    firebaseIdToken: credentials.token,
    sessionId: credentials.sessionId,
    circleId: circleId,
  );
});

final circleMembersProvider =
    FutureProvider.family<List<CircleMember>, String>((ref, circleId) async {
  final credentials = await _loadCircleCredentials(ref);
  final apiClient = ref.watch(circleApiClientProvider);
  return apiClient.listCircleMembers(
    firebaseIdToken: credentials.token,
    sessionId: credentials.sessionId,
    circleId: circleId,
  );
});
