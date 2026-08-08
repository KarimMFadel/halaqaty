import 'package:dio/dio.dart';
import 'package:firebase_auth/firebase_auth.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';

typedef ReadCircleAuthState = AuthState Function();
typedef CircleLogout = Future<void> Function();

class CreateCircleState {
  const CreateCircleState({
    this.circle,
    this.isSaving = false,
    this.errorMessage,
    this.fieldErrors = const {},
  });

  final CircleResponse? circle;
  final bool isSaving;
  final String? errorMessage;
  final Map<String, String> fieldErrors;

  CreateCircleState copyWith({
    CircleResponse? circle,
    bool? isSaving,
    String? errorMessage,
    Map<String, String>? fieldErrors,
    bool clearError = false,
  }) {
    return CreateCircleState(
      circle: circle ?? this.circle,
      isSaving: isSaving ?? this.isSaving,
      errorMessage: clearError ? null : (errorMessage ?? this.errorMessage),
      fieldErrors: fieldErrors ?? this.fieldErrors,
    );
  }
}

class CreateCircleController extends StateNotifier<CreateCircleState> {
  CreateCircleController({
    required CircleApiClient apiClient,
    required Future<String?> Function() loadFirebaseIdToken,
    required ReadCircleAuthState readAuthState,
    required CircleLogout logout,
  })  : _apiClient = apiClient,
        _loadFirebaseIdToken = loadFirebaseIdToken,
        _readAuthState = readAuthState,
        _logout = logout,
        super(const CreateCircleState());

  final CircleApiClient _apiClient;
  final Future<String?> Function() _loadFirebaseIdToken;
  final ReadCircleAuthState _readAuthState;
  final CircleLogout _logout;

  Future<bool> create(CreateCircleRequest request) async {
    state = const CreateCircleState(isSaving: true);
    try {
      final sessionId = _readAuthState().sessionId;
      final token = await _loadFirebaseIdToken();
      if (token == null || token.isEmpty || sessionId == null) {
        state = state.copyWith(
          isSaving: false,
          errorMessage: 'Session is missing. Please sign in again.',
        );
        return false;
      }
      final circle = await _apiClient.createCircle(
        firebaseIdToken: token,
        sessionId: sessionId,
        request: request,
      );
      state = state.copyWith(circle: circle, isSaving: false, clearError: true);
      return true;
    } on FirebaseAuthException catch (e) {
      state = state.copyWith(isSaving: false, errorMessage: e.message);
      return false;
    } on DioException catch (e) {
      final data = e.response?.data;
      final error = data is Map<String, dynamic> ? data['error'] : null;
      final fields = error is Map ? error['fields'] : null;
      final mapped = <String, String>{};
      if (fields is Map) {
        for (final entry in fields.entries) {
          if (entry.key is String && entry.value is String) {
            mapped[entry.key as String] = entry.value as String;
          }
        }
      }
      if (e.response?.statusCode == 401) {
        await _logout();
      }
      state = state.copyWith(
        isSaving: false,
        errorMessage: error is Map ? error['message'] as String? : e.message,
        fieldErrors: mapped,
      );
      return false;
    } finally {
      if (state.isSaving) {
        state = state.copyWith(isSaving: false);
      }
    }
  }

  Future<List<CircleUser>> searchUsers(String query) async {
    try {
      final sessionId = _readAuthState().sessionId;
      final token = await _loadFirebaseIdToken();
      if (token == null || token.isEmpty || sessionId == null) {
        state = state.copyWith(
          errorMessage: 'Session is missing. Please sign in again.',
        );
        return const [];
      }
      return await _apiClient.searchUsers(
        firebaseIdToken: token,
        sessionId: sessionId,
        query: query,
      );
    } on FirebaseAuthException catch (e) {
      state = state.copyWith(errorMessage: e.message);
      return const [];
    } on DioException catch (e) {
      if (e.response?.statusCode == 401) await _logout();
      state = state.copyWith(errorMessage: e.message);
      return const [];
    }
  }
}

final createCircleControllerProvider =
    StateNotifierProvider<CreateCircleController, CreateCircleState>((ref) {
  return CreateCircleController(
    apiClient: ref.watch(circleApiClientProvider),
    loadFirebaseIdToken: () =>
        ref.read(firebaseAuthProvider).currentUser?.getIdToken(),
    readAuthState: () => ref.read(authControllerProvider),
    logout: ref.read(authControllerProvider.notifier).logout,
  );
});
