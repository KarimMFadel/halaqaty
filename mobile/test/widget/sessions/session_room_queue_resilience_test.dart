import 'dart:async';

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/sessions/application/media_session.dart';
import 'package:halaqaty_mobile/features/sessions/application/queue_controller.dart';
import 'package:halaqaty_mobile/features/sessions/application/session_room_controller.dart';
import 'package:halaqaty_mobile/features/sessions/data/queue_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/data/realtime_session_client.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/presentation/queue/queue_student_panel.dart';
import 'package:halaqaty_mobile/features/sessions/presentation/session_ui_labels.dart';

const _sessionID = 'session-resilience';

void main() {
  test('queue resilience: deduplicates events and replaces state on reconnect',
      () async {
    final realtime = _ResilienceRealtime();
    final api = _ResilienceQueueApi([_queue(1), _queue(3)]);
    final controller = _queueController(api, realtime);
    addTearDown(controller.dispose);

    await controller.connect(_sessionID);
    realtime.emit(QueueStateEvent(
      sessionId: _sessionID,
      eventId: 'duplicate',
      queue: _queue(2),
    ));
    realtime.emit(QueueStateEvent(
      sessionId: _sessionID,
      eventId: 'duplicate',
      queue: _queue(99),
    ));
    expect(controller.state.queue?.version, 2);

    await controller.connect(_sessionID);
    expect(api.getQueueCalls, 2);
    expect(controller.state.queue?.version, 3);
  });

  test('queue failure remains isolated from ending the live session', () async {
    final realtime = _ResilienceRealtime();
    final queueApi = _ResilienceQueueApi([_queue(1)])
      ..advanceFailure = StateError('queue unavailable');
    final queue = _queueController(queueApi, realtime);
    final sessionApi = _ResilienceSessionApi();
    final room = SessionRoomController(
      sessionApi,
      () async => (token: 'token', sessionId: 'backend-session'),
      _NoopMedia(),
      realtime: realtime,
      isModerator: true,
      queue: queue,
    );
    addTearDown(queue.dispose);
    addTearDown(room.dispose);

    await room.join(_sessionID);
    await queue.advance();
    await room.endSession();

    expect(queue.state.actionErrorMessage, contains('queue unavailable'));
    expect(sessionApi.endCalls, 1);
    expect(room.state.status, SessionRoomStatus.ended);
  });

  testWidgets('student queue controls stay role-safe in RTL and LTR',
      (tester) async {
    for (final direction in TextDirection.values) {
      await tester.pumpWidget(MaterialApp(
        home: Directionality(
          textDirection: direction,
          child: QueueStudentPanel(
            queue: _queue(1),
            myEntry: _queue(1).entries.single,
            status: QueueStudentPanelStatus.ready,
            optOutStatus: StudentOptOutStatus.notRequested,
            onRequestOptOut: () {},
          ),
        ),
      ));

      expect(find.text(SessionUiLabels.reorderQueue), findsNothing);
      expect(find.text(SessionUiLabels.resetQueue), findsNothing);
      expect(
        find.bySemanticsLabel(
          direction == TextDirection.rtl
              ? SessionUiLabels.optOutAction
              : 'Opt out of turn',
        ),
        findsOneWidget,
      );
    }
  });
}

QueueController _queueController(
        QueueApiClient api, RealtimeSessionClient realtime) =>
    QueueController(
      api,
      () async => (token: 'token', sessionId: 'backend-session'),
      realtime: realtime,
      isManager: true,
    );

class _ResilienceQueueApi extends QueueApiClient {
  _ResilienceQueueApi(this.snapshots) : super(Dio());

  final List<QueueState> snapshots;
  int getQueueCalls = 0;
  Object? advanceFailure;

  @override
  Future<QueueState> getQueue({
    required String token,
    required String sessionId,
    required String liveSessionId,
  }) async =>
      snapshots[(getQueueCalls++).clamp(0, snapshots.length - 1)];

  @override
  Future<QueueState> advance({
    required String token,
    required String sessionId,
    required String liveSessionId,
    required int expectedVersion,
    String? idempotencyKey,
  }) async {
    if (advanceFailure != null) throw advanceFailure!;
    return snapshots.last;
  }
}

class _ResilienceRealtime implements RealtimeSessionClient {
  final _events = StreamController<RealtimeSessionEvent>.broadcast(sync: true);

  void emit(RealtimeSessionEvent event) => _events.add(event);

  @override
  Future<void> dispose() => _events.close();

  @override
  Future<void> lowerHand(String liveSessionId) async {}

  @override
  Future<void> raiseHand(String liveSessionId) async {}

  @override
  Stream<RealtimeSessionEvent> sessionEvents(
    String liveSessionId, {
    required String token,
    required String backendSessionId,
  }) =>
      _events.stream;
}

class _ResilienceSessionApi extends SessionApiClient {
  _ResilienceSessionApi() : super(Dio());

  int endCalls = 0;

  @override
  Future<SessionConnection> join({
    required String token,
    required String sessionId,
    required String liveSessionId,
  }) async =>
      _connection();

  @override
  Future<List<SessionParticipant>> participants({
    required String token,
    required String sessionId,
    required String liveSessionId,
  }) async =>
      const [];

  @override
  Future<SessionModel> end({
    required String token,
    required String sessionId,
    required String liveSessionId,
  }) async {
    endCalls++;
    return _session(status: 'ended');
  }
}

class _NoopMedia implements MediaSession {
  @override
  Future<void> connect(MediaConnection connection) async {}

  @override
  Future<void> disconnect() async {}

  @override
  Future<void> setMicrophoneEnabled(bool enabled) async {}
}

QueueState _queue(int version) => QueueState.fromJson({
      'session_id': _sessionID,
      'round_id': 'round-1',
      'round_number': 1,
      'round_type': 'revision',
      'lifecycle': 'active',
      'surah_id': 1,
      'from_ayah': 1,
      'to_ayah': 7,
      'grading_required': false,
      'selected_entry_id': null,
      'version': version,
      'policy': {
        'population': 'present_at_activation',
        'unfinished_finalization': 'mark_unfinished_skipped',
        'opt_out': 'approval_required',
        'grade_visibility': 'managers_and_student',
        'grade_correction': 'audited_any_time',
        'version': 1,
      },
      'preorder': const [],
      'entries': const [
        {
          'id': 'entry-1',
          'student_id': 'student-1',
          'student_name': 'مريم',
          'position': 1,
          'status': 'waiting',
          'version': 1,
        },
      ],
    });

SessionConnection _connection() => SessionConnection(
      session: _session(),
      mediaConnection: MediaConnection(
        endpoint: 'wss://media.example',
        credential: 'credential',
        expiresAt: DateTime.utc(2026, 1, 1),
      ),
    );

SessionModel _session({String status = 'active'}) => SessionModel(
      id: _sessionID,
      circleId: 'circle-1',
      status: status,
      mediaMode: 'audio',
      participantCount: 1,
      isLocked: false,
    );
