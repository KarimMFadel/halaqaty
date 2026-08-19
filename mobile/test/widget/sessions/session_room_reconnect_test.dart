import 'dart:async';

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/application/media_session.dart';
import 'package:halaqaty_mobile/features/sessions/application/session_room_controller.dart';
import 'package:halaqaty_mobile/features/sessions/data/realtime_session_client.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/presentation/session_room_screen.dart';
import 'package:halaqaty_mobile/features/sessions/presentation/session_ui_labels.dart';

void main() {
  testWidgets('reconnect refreshes the participant credential', (tester) async {
    final api = RecoverySessionApi();
    final media = RecordingMediaSession();
    await tester.pumpWidget(_app(api, media: media));

    await tester.tap(find.text('Join'));
    await _pumpAsync(tester);
    expect(media.connections, 1);
    expect(media.credentials, ['credential-1']);

    // A reconnect must use a fresh authenticated join response, not reuse the
    // short-lived credential from the first media connection.
    await tester.tap(find.text('Join'));
    await _pumpAsync(tester);

    expect(media.connections, 2);
    expect(media.credentials, ['credential-1', 'credential-2']);
    expect(api.joinCalls, 2);
  });

  testWidgets('recoverable reconnect rehydrates the authoritative snapshot',
      (tester) async {
    final api = RecoverySessionApi()
      ..participantsList = [_participant('student-1')];
    final media = RecordingMediaSession();
    final realtime = RecordingRealtimeClient();
    await tester.pumpWidget(_app(api, media: media, realtime: realtime));
    await tester.tap(find.text('Join'));
    await _pumpAsync(tester);

    api.participantsList = [_participant('student-2')];
    await tester.tap(find.text('Join'));
    await _pumpAsync(tester);

    expect(find.text('student-1'), findsNothing);
    expect(find.text('student-2'), findsOneWidget);
    expect(media.connections, 2);
  });

  testWidgets('ended event is terminal and stops the room state',
      (tester) async {
    final realtime = RecordingRealtimeClient();
    await tester.pumpWidget(_app(RecoverySessionApi(),
        realtime: realtime, direction: TextDirection.rtl));
    await tester.tap(find.text(SessionUiLabels.join));
    await _pumpAsync(tester);

    await tester.pump(const Duration(milliseconds: 1));
    realtime.emit(const SessionEndedEvent(
      sessionId: 'session-1',
      endReason: 'manual',
    ));
    await tester.pump();

    expect(find.text(SessionUiLabels.sessionEnded), findsOneWidget);
  });

  testWidgets('participant removal is terminal for that participant',
      (tester) async {
    final realtime = RecordingRealtimeClient();
    final api = RecoverySessionApi()
      ..participantsList = [_participant('student-1')];
    await tester.pumpWidget(_app(api, realtime: realtime));
    await tester.tap(find.text('Join'));
    await _pumpAsync(tester);
    expect(find.text('student-1'), findsOneWidget);

    realtime.emit(const ParticipantRemovedEvent(
      sessionId: 'session-1',
      userId: 'student-1',
    ));
    await tester.pump();

    expect(find.text('student-1'), findsNothing);
  });

  testWidgets('lock event updates the reconnect/join affordance in Arabic',
      (tester) async {
    final realtime = RecordingRealtimeClient();
    await tester.pumpWidget(_app(
      RecoverySessionApi(),
      realtime: realtime,
      direction: TextDirection.rtl,
      isModerator: true,
    ));
    await tester.tap(find.text(SessionUiLabels.join));
    await _pumpAsync(tester);

    realtime.emit(const LockChangedEvent(sessionId: 'session-1', locked: true));
    await tester.pump();

    expect(find.text(SessionUiLabels.unlockSession), findsOneWidget);
  });

  testWidgets('moderator end action reaches Arabic terminal state',
      (tester) async {
    await tester.pumpWidget(_app(
      RecoverySessionApi(),
      direction: TextDirection.rtl,
      isModerator: true,
      canStart: true,
    ));
    await tester.tap(find.text(SessionUiLabels.start));
    await _pumpAsync(tester);
    await tester.tap(find.text(SessionUiLabels.endSession));
    await tester.pumpAndSettle();

    expect(find.text(SessionUiLabels.sessionEnded), findsOneWidget);
  });

  testWidgets('connection failure uses safe Arabic error copy', (tester) async {
    await tester.pumpWidget(
        _app(FailingRecoverySessionApi(), direction: TextDirection.rtl));
    await tester.tap(find.text(SessionUiLabels.join));
    await tester.pumpAndSettle();

    expect(find.text(SessionUiLabels.unableToConnect), findsOneWidget);
    expect(find.textContaining('provider-secret'), findsNothing);
  });
}

Future<void> _pumpAsync(WidgetTester tester) async {
  await tester.pump();
  await tester.pump(const Duration(milliseconds: 100));
}

Widget _app(
  SessionApiClient api, {
  RecordingMediaSession? media,
  RecordingRealtimeClient? realtime,
  TextDirection direction = TextDirection.ltr,
  bool isModerator = false,
  bool canStart = false,
}) {
  return ProviderScope(
    overrides: [
      sessionRoomControllerProvider('session-1').overrideWith(
        (ref) => SessionRoomController(
          api,
          () async => (token: 'token', sessionId: 'backend-session'),
          media ?? RecordingMediaSession(),
          realtime: realtime ?? RecordingRealtimeClient(),
          isModerator: isModerator,
        ),
      ),
    ],
    child: MaterialApp(
      home: Directionality(
        textDirection: direction,
        child: SessionRoomScreen(
          sessionId: 'session-1',
          canStart: canStart,
        ),
      ),
    ),
  );
}

class RecordingMediaSession implements MediaSession {
  int connections = 0;
  final List<String> credentials = [];

  @override
  Future<void> connect(MediaConnection connection) async {
    connections++;
    credentials.add(connection.credential);
  }

  @override
  Future<void> disconnect() async {}

  @override
  Future<void> setMicrophoneEnabled(bool enabled) async {}
}

class RecordingRealtimeClient implements RealtimeSessionClient {
  final StreamController<RealtimeSessionEvent> _events =
      StreamController<RealtimeSessionEvent>.broadcast(sync: true);

  void emit(RealtimeSessionEvent event) {
    scheduleMicrotask(() => _events.add(event));
  }

  @override
  Stream<RealtimeSessionEvent> sessionEvents(String liveSessionId,
          {required String token, required String backendSessionId}) =>
      _events.stream;

  @override
  Future<void> raiseHand(String liveSessionId) async {}

  @override
  Future<void> lowerHand(String liveSessionId) async {}

  @override
  Future<void> dispose() => _events.close();
}

class RecoverySessionApi extends SessionApiClient {
  RecoverySessionApi() : super(Dio());
  int joinCalls = 0;
  List<SessionParticipant> participantsList = const [];

  @override
  Future<SessionConnection> join({
    required String token,
    required String sessionId,
    required String liveSessionId,
  }) async {
    joinCalls++;
    return _connection(credential: 'credential-$joinCalls');
  }

  @override
  Future<SessionConnection> start({
    required String token,
    required String sessionId,
    required String liveSessionId,
  }) async =>
      _connection(credential: 'start-credential');

  @override
  Future<List<SessionParticipant>> participants({
    required String token,
    required String sessionId,
    required String liveSessionId,
  }) async =>
      participantsList;

  @override
  Future<SessionModel> setLock({
    required String token,
    required String sessionId,
    required String liveSessionId,
    required bool locked,
  }) async =>
      _session(locked: locked);

  @override
  Future<SessionModel> end({
    required String token,
    required String sessionId,
    required String liveSessionId,
  }) async =>
      _session(status: 'ended');
}

class FailingRecoverySessionApi extends RecoverySessionApi {
  @override
  Future<SessionConnection> join({
    required String token,
    required String sessionId,
    required String liveSessionId,
  }) =>
      Future.error(StateError('provider-secret: media unavailable'));
}

SessionParticipant _participant(String userId) => SessionParticipant(
      userId: userId,
      displayName: userId,
      role: CircleRole.student,
      isCurrentlyPresent: true,
    );

SessionConnection _connection({required String credential}) =>
    SessionConnection(
      session: _session(),
      mediaConnection: MediaConnection(
        endpoint: 'wss://media.example',
        credential: credential,
        expiresAt: DateTime.now().add(const Duration(minutes: 1)),
      ),
    );

SessionModel _session({String status = 'active', bool locked = false}) =>
    SessionModel(
      id: 'session-1',
      circleId: 'circle-1',
      status: status,
      mediaMode: 'audio',
      participantCount: 1,
      isLocked: locked,
    );
