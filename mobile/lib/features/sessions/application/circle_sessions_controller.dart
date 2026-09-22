import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_api_client.dart';

enum CircleSessionsStatus { loading, ready, error }

/// Why the last list/create call failed, for the section's error copy.
enum CircleSessionsFailure { network, permission, unknown }

class CircleSessionsState {
  const CircleSessionsState({
    this.status = CircleSessionsStatus.loading,
    this.sessions = const <SessionModel>[],
    this.failure,
    this.isCreating = false,
  });

  final CircleSessionsStatus status;

  /// Last known sessions; retained across failures so an offline reload does
  /// not blank the section.
  final List<SessionModel> sessions;
  final CircleSessionsFailure? failure;
  final bool isCreating;

  CircleSessionsState copyWith({
    CircleSessionsStatus? status,
    List<SessionModel>? sessions,
    CircleSessionsFailure? failure,
    bool clearFailure = false,
    bool? isCreating,
  }) =>
      CircleSessionsState(
        status: status ?? this.status,
        sessions: sessions ?? this.sessions,
        failure: clearFailure ? null : (failure ?? this.failure),
        isCreating: isCreating ?? this.isCreating,
      );
}

/// Minimal presentation controller for the circle detail sessions section:
/// list + create over the existing F-005 [SessionApiClient], nothing more.
class CircleSessionsController extends StateNotifier<CircleSessionsState> {
  CircleSessionsController(
    this._api,
    this._credentials, {
    required this.circleId,
  }) : super(const CircleSessionsState());

  final SessionApiClient _api;
  final Future<({String token, String sessionId})> Function() _credentials;
  final String circleId;

  Future<void> load() async {
    state = state.copyWith(
        status: CircleSessionsStatus.loading, clearFailure: true);
    try {
      final credentials = await _credentials();
      final sessions = await _api.list(
        token: credentials.token,
        sessionId: credentials.sessionId,
        circleId: circleId,
      );
      state = state.copyWith(
        status: CircleSessionsStatus.ready,
        sessions: sessions,
        clearFailure: true,
      );
    } catch (error) {
      state = state.copyWith(
        status: CircleSessionsStatus.error,
        failure: _classify(error),
      );
    }
  }

  /// Creates an ad-hoc session; returns it on success so the caller can
  /// navigate straight into the room for the explicit start transition.
  Future<SessionModel?> create() async {
    if (state.isCreating) return null;
    state = state.copyWith(isCreating: true);
    try {
      final credentials = await _credentials();
      final created = await _api.create(
        token: credentials.token,
        sessionId: credentials.sessionId,
        circleId: circleId,
      );
      state = state.copyWith(
        isCreating: false,
        sessions: [created, ...state.sessions],
      );
      return created;
    } catch (error) {
      // Keep the previous list status: the section reports the create failure
      // via snackbar, so flipping to the load-error copy would be misleading.
      state = state.copyWith(
        isCreating: false,
        failure: _classify(error),
      );
      return null;
    }
  }

  static CircleSessionsFailure _classify(Object error) {
    if (error is DioException) {
      final code = error.response?.statusCode;
      if (code == 401 || code == 403) return CircleSessionsFailure.permission;
      if (error.type == DioExceptionType.connectionError ||
          error.type == DioExceptionType.connectionTimeout) {
        return CircleSessionsFailure.network;
      }
    }
    return CircleSessionsFailure.unknown;
  }
}

final circleSessionsControllerProvider = StateNotifierProvider.family<
    CircleSessionsController, CircleSessionsState, String>((ref, circleId) {
  return CircleSessionsController(
    ref.watch(sessionApiClientProvider),
    () async {
      // Read lazily per call: watching auth eagerly would build the real
      // Firebase-backed providers for any consumer of this section.
      final auth = ref.read(authControllerProvider);
      final user = ref.read(firebaseAuthProvider).currentUser;
      final sessionId = auth.sessionId;
      final token = await user?.getIdToken();
      if (token == null ||
          token.isEmpty ||
          sessionId == null ||
          sessionId.isEmpty) {
        throw StateError('User not authenticated');
      }
      return (token: token, sessionId: sessionId);
    },
    circleId: circleId,
  );
});
