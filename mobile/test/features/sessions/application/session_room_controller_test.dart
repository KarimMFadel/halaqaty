import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/sessions/application/media_session.dart';
import 'package:halaqaty_mobile/features/sessions/application/session_room_controller.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_api_client.dart';

void main() {
  test(
      'start connects through the neutral media boundary without storing credentials outside state',
      () async {
    final api = FakeSessionApi();
    final media = FakeMediaSession();
    final controller = SessionRoomController(
        api, () async => (token: 'token', sessionId: 'backend-session'), media);
    addTearDown(controller.dispose);

    await controller.start('live-session-1');

    expect(controller.state.status, SessionRoomStatus.connected);
    expect(media.connections, 1);
    expect(media.lastConnection?.credential, 'short-lived-credential');
    expect(api.lastToken, 'token');
    expect(api.lastSessionId, 'backend-session');
  });

  test('join exposes an error state and does not connect media when API fails',
      () async {
    final media = FakeMediaSession();
    final controller = SessionRoomController(FailingSessionApi(),
        () async => (token: 'token', sessionId: 'session'), media);
    addTearDown(controller.dispose);

    await controller.join('live-session-1');

    expect(controller.state.status, SessionRoomStatus.error);
    expect(controller.state.errorMessage, contains('join failed'));
    expect(media.connections, 0);
  });
}

class FakeMediaSession implements MediaSession {
  int connections = 0;
  MediaConnection? lastConnection;

  @override
  Future<void> connect(MediaConnection connection) async {
    connections++;
    lastConnection = connection;
  }

  @override
  Future<void> disconnect() async {}

  @override
  Future<void> setMicrophoneEnabled(bool enabled) async {}
}

class FakeSessionApi extends SessionApiClient {
  FakeSessionApi() : super(Dio());
  String? lastToken;
  String? lastSessionId;

  @override
  Future<SessionConnection> start(
      {required String token,
      required String sessionId,
      required String liveSessionId}) async {
    lastToken = token;
    lastSessionId = sessionId;
    return _connection();
  }

  @override
  Future<SessionConnection> join(
          {required String token,
          required String sessionId,
          required String liveSessionId}) async =>
      _connection();
}

class FailingSessionApi extends SessionApiClient {
  FailingSessionApi() : super(Dio());

  @override
  Future<SessionConnection> join(
          {required String token,
          required String sessionId,
          required String liveSessionId}) =>
      Future.error(StateError('join failed'));
}

SessionConnection _connection() => SessionConnection(
      session: const SessionModel(
          id: 'live-session-1',
          circleId: 'circle-1',
          status: 'active',
          mediaMode: 'audio',
          participantCount: 1,
          isLocked: false),
      mediaConnection: MediaConnection(
          endpoint: 'wss://media.example',
          credential: 'short-lived-credential',
          expiresAt: DateTime.now().add(const Duration(minutes: 1))),
    );
