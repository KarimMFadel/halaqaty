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
    final realtime = RecordingRealtimeClient();
    await tester.pumpWidget(_app(api, media: media, realtime: realtime));

    await tester.tap(find.text('Join'));
    await _pumpAsync(tester);
    expect(media.connections, 1);
    expect(media.credentials, ['credential-1']);

    // A reconnect must use a fresh authenticated join response, not reuse the
    // short-lived credential from the first media connection. The connected
    // room offers no re-join action; the retry affordance owns recovery.
    await realtime.close();
    await tester.pump();
    await tester.tap(find.byKey(const Key('sessionRoomRetry')));
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
    await realtime.close();
    await tester.pump();
    await tester.tap(find.byKey(const Key('sessionRoomRetry')));
    await _pumpAsync(tester);

    expect(find.text('student-1'), findsNothing);
    expect(find.text('student-2'), findsOneWidget);
    expect(media.connections, 2);
  });

  testWidgets('a retry after a prior connection announces reconnecting',
      (tester) async {
    final realtime = RecordingRealtimeClient();
    final api = RecoverySessionApi();
    await tester.pumpWidget(_app(api, realtime: realtime));
    await tester.tap(find.text('Join'));
    await _pumpAsync(tester);

    await realtime.close();
    await tester.pump();
    // Dropping the focused dominant action on connect defers the rebuild one
    // frame, so the retry affordance needs a second pump before it is tappable.
    await tester.pump();
    api.joinDelay = const Duration(milliseconds: 100);
    await tester.tap(find.byKey(const Key('sessionRoomRetry')));
    await tester.pump();
    await tester.pump();

    // A prior connection turns the wait into a reconnect, matching the queue
    // panel's reconnecting copy.
    expect(find.text('Reconnecting...'), findsOneWidget);
    expect(find.text('Loading participants...'), findsNothing);

    await _pumpAsync(tester);
    // Dropping the focused dominant action on reconnect defers the rebuild one
    // frame, so the connected copy needs a second pump.
    await tester.pump();
    expect(find.text('Connected. Audio is ready.'), findsOneWidget);
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
    // Dropping the focused dominant action on connect defers the rebuild one
    // frame, so the post-event state needs a second pump.
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
    // Dropping the focused dominant action on connect defers the rebuild one
    // frame, so the post-event state needs a second pump.
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
    // Dropping the focused dominant action on connect defers the rebuild one
    // frame, so the post-event state needs a second pump.
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

  testWidgets('realtime interruption keeps moderator end control available',
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

    await realtime.close();
    await tester.pump();

    expect(find.text(SessionUiLabels.retry), findsOneWidget);
    expect(find.text(SessionUiLabels.endSession), findsOneWidget);
  });

  testWidgets('connection failure uses safe Arabic error copy', (tester) async {
    await tester.pumpWidget(
        _app(FailingRecoverySessionApi(), direction: TextDirection.rtl));
    await tester.tap(find.text(SessionUiLabels.join));
    await tester.pumpAndSettle();

    expect(find.text(SessionUiLabels.unableToConnect), findsOneWidget);
    expect(find.textContaining('provider-secret'), findsNothing);
  });

  testWidgets('retryable failure keeps retry and leave affordances at 48dp',
      (tester) async {
    await tester.pumpWidget(
        _app(FailingRecoverySessionApi(), direction: TextDirection.ltr));
    await tester.tap(find.text('Join'));
    await tester.pumpAndSettle();

    final retry = find.byKey(const Key('sessionRoomRetry'));
    final leave = find.byKey(const Key('sessionRoomLeave'));
    expect(retry, findsOneWidget);
    expect(leave, findsOneWidget);
    expect(tester.getSize(retry).height, greaterThanOrEqualTo(48));
    expect(tester.getSize(leave).height, greaterThanOrEqualTo(48));
    expect(find.text('Session access has ended'), findsNothing);
  });

  testWidgets(
      'terminal failure states retry is unavailable and offers a safe exit',
      (tester) async {
    await tester.pumpWidget(
        _app(TerminalRecoverySessionApi(), direction: TextDirection.ltr));
    await tester.tap(find.text('Join'));
    await tester.pumpAndSettle();

    expect(find.text('Session access has ended'), findsOneWidget);
    expect(
        find.text('Retry is unavailable. Leave the session.'), findsOneWidget);
    expect(find.byKey(const Key('sessionRoomRetry')), findsNothing);
    final leave = find.byKey(const Key('sessionRoomLeave'));
    expect(leave, findsOneWidget);
    expect(tester.getSize(leave).height, greaterThanOrEqualTo(48));
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
  StreamController<RealtimeSessionEvent> _events =
      StreamController<RealtimeSessionEvent>.broadcast(sync: true);

  void emit(RealtimeSessionEvent event) {
    scheduleMicrotask(() => _events.add(event));
  }

  Future<void> close() => _events.close();

  @override
  Stream<RealtimeSessionEvent> sessionEvents(String liveSessionId,
      {required String token, required String backendSessionId}) {
    // A retry reopens the channel, like a fresh WebSocket connection;
    // reusing the closed controller would replay an immediate done event.
    if (_events.isClosed) {
      _events = StreamController<RealtimeSessionEvent>.broadcast(sync: true);
    }
    return _events.stream;
  }

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

  /// Optional artificial join latency so tests can observe the loading state.
  Duration? joinDelay;

  @override
  Future<SessionConnection> join({
    required String token,
    required String sessionId,
    required String liveSessionId,
  }) async {
    joinCalls++;
    final delay = joinDelay;
    if (delay != null) await Future<void>.delayed(delay);
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

/// Terminal per `SessionRoomController._isTerminal` ('ended' / '403').
class TerminalRecoverySessionApi extends RecoverySessionApi {
  @override
  Future<SessionConnection> join({
    required String token,
    required String sessionId,
    required String liveSessionId,
  }) =>
      Future.error(StateError('session access ended: 403'));
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
