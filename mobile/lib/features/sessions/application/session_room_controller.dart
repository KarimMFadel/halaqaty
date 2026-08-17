import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/sessions/application/media_session.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_api_client.dart';

enum SessionRoomStatus { idle, loading, connected, error }

class SessionRoomState {
  const SessionRoomState(
      {this.status = SessionRoomStatus.idle,
      this.connection,
      this.errorMessage});
  final SessionRoomStatus status;
  final SessionConnection? connection;
  final String? errorMessage;

  SessionRoomState copyWith(
          {SessionRoomStatus? status,
          SessionConnection? connection,
          String? errorMessage,
          bool clearError = false}) =>
      SessionRoomState(
        status: status ?? this.status,
        connection: connection ?? this.connection,
        errorMessage: clearError ? null : (errorMessage ?? this.errorMessage),
      );
}

class SessionRoomController extends StateNotifier<SessionRoomState> {
  SessionRoomController(this._api, this._credentials, this._mediaSession)
      : super(const SessionRoomState());
  final SessionApiClient _api;
  final Future<({String token, String sessionId})> Function() _credentials;
  final MediaSession _mediaSession;

  Future<void> start(String sessionId) => _connect(sessionId, true);
  Future<void> join(String sessionId) => _connect(sessionId, false);

  Future<void> _connect(String liveSessionId, bool start) async {
    state = state.copyWith(status: SessionRoomStatus.loading, clearError: true);
    try {
      final credentials = await _credentials();
      final connection = start
          ? await _api.start(
              token: credentials.token,
              sessionId: credentials.sessionId,
              liveSessionId: liveSessionId)
          : await _api.join(
              token: credentials.token,
              sessionId: credentials.sessionId,
              liveSessionId: liveSessionId);
      await _mediaSession.connect(connection.mediaConnection);
      state = state.copyWith(
          status: SessionRoomStatus.connected, connection: connection);
    } catch (error) {
      state = state.copyWith(
          status: SessionRoomStatus.error, errorMessage: error.toString());
    }
  }

  @override
  void dispose() {
    _mediaSession.disconnect();
    super.dispose();
  }
}

final sessionRoomControllerProvider = StateNotifierProvider.family<
    SessionRoomController, SessionRoomState, String>((ref, _) {
  final auth = ref.watch(authControllerProvider);
  return SessionRoomController(ref.watch(sessionApiClientProvider), () async {
    final user = ref.read(firebaseAuthProvider).currentUser;
    final sessionId = auth.sessionId;
    final token = await user?.getIdToken();
    if (token == null ||
        token.isEmpty ||
        sessionId == null ||
        sessionId.isEmpty) throw StateError('User not authenticated');
    final tokenValue = token;
    final sessionValue = sessionId;
    return (token: tokenValue, sessionId: sessionValue);
  }, ref.watch(mediaSessionProvider));
});
