import 'dart:async';

import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/sessions/application/media_session.dart';
import 'package:halaqaty_mobile/features/sessions/application/queue_controller.dart';
import 'package:halaqaty_mobile/features/sessions/application/session_room_controller.dart';
import 'package:halaqaty_mobile/features/sessions/data/queue_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/data/realtime_session_client.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_api_client.dart';

const _liveSessionId = 'live-session-1';

void main() {
  test('connect loads the authoritative queue snapshot', () async {
    final queueApi = _FakeQueueApiClient([_queueState(version: 1)]);
    final controller = _queueController(queueApi, _FakeRealtimeClient());
    addTearDown(controller.dispose);

    await controller.connect(_liveSessionId);

    expect(controller.state.status, QueueControllerStatus.ready);
    expect(controller.state.queue?.version, 1);
    expect(controller.state.queue?.entries.single.status, 'waiting');
    expect(queueApi.getQueueCalls, 1);
  });

  test('manager command failure is action-local and retains the snapshot',
      () async {
    final queueApi = _FakeQueueApiClient([_queueState(version: 1)])
      ..advanceFailure = StateError('advance failed');
    final controller = _queueController(queueApi, _FakeRealtimeClient());
    addTearDown(controller.dispose);
    await controller.connect(_liveSessionId);

    await controller.advance();

    expect(queueApi.advanceCalls, 1);
    expect(controller.state.status, QueueControllerStatus.ready);
    expect(controller.state.queue?.version, 1);
    expect(controller.state.actionErrorMessage, contains('advance failed'));
  });

  test('deduplicates duplicate queue events by event_id', () async {
    final realtime = _FakeRealtimeClient();
    final controller = _queueController(
      _FakeQueueApiClient([_queueState(version: 1)]),
      realtime,
    );
    addTearDown(controller.dispose);
    await controller.connect(_liveSessionId);

    realtime.emit(QueueStateEvent(
      sessionId: _liveSessionId,
      eventId: 'event-2',
      queue: _queueState(version: 2),
    ));
    realtime.emit(QueueStateEvent(
      sessionId: _liveSessionId,
      eventId: 'event-2',
      queue: _queueState(version: 3),
    ));

    expect(controller.state.queue?.version, 2);
  });

  test('ignores a stale queue event with a new event_id', () async {
    final realtime = _FakeRealtimeClient();
    final controller = _queueController(
      _FakeQueueApiClient([_queueState(version: 2)]),
      realtime,
    );
    addTearDown(controller.dispose);
    await controller.connect(_liveSessionId);

    realtime.emit(QueueStateEvent(
      sessionId: _liveSessionId,
      eventId: 'stale-event',
      queue: _queueState(version: 1),
    ));

    expect(controller.state.queue?.version, 2);
  });

  test('re-fetches the current queue after a version gap', () async {
    final realtime = _FakeRealtimeClient();
    final queueApi = _FakeQueueApiClient([
      _queueState(version: 1),
      _queueState(version: 3),
    ]);
    final controller = _queueController(queueApi, realtime);
    addTearDown(controller.dispose);
    await controller.connect(_liveSessionId);

    realtime.emit(QueueStateEvent(
      sessionId: _liveSessionId,
      eventId: 'gap-event',
      queue: _queueState(version: 3),
    ));
    await Future<void>.delayed(Duration.zero);

    expect(queueApi.getQueueCalls, 2);
    expect(controller.state.queue?.version, 3);
  });

  test('re-fetches the current queue after queue.policy_changed', () async {
    final realtime = _FakeRealtimeClient();
    final queueApi = _FakeQueueApiClient([
      _queueState(version: 1, policyVersion: 1),
      _queueState(version: 2, policyVersion: 2),
    ]);
    final controller = _queueController(queueApi, realtime);
    addTearDown(controller.dispose);
    await controller.connect(_liveSessionId);

    realtime.emit(const QueuePolicyChangedEvent(
      sessionId: _liveSessionId,
      eventId: 'policy-event',
      policyVersion: 2,
    ));
    await Future<void>.delayed(Duration.zero);

    expect(queueApi.getQueueCalls, 2);
    expect(controller.state.queue?.version, 2);
    expect(controller.state.queue?.policy.version, 2);
  });

  test('F-005 session end remains usable after a queue command failure',
      () async {
    final realtime = _FakeRealtimeClient();
    final queueApi = _FakeQueueApiClient([_queueState(version: 1)])
      ..advanceFailure = StateError('advance failed');
    final queueController = _queueController(queueApi, realtime);
    final sessionApi = _FakeSessionApi();
    final roomController = SessionRoomController(
      sessionApi,
      () async => (token: 'token', sessionId: 'backend-session'),
      _NoopMediaSession(),
      realtime: realtime,
      isModerator: true,
      queue: queueController,
    );
    addTearDown(queueController.dispose);
    addTearDown(roomController.dispose);
    await roomController.join(_liveSessionId);

    await queueController.advance();
    await roomController.endSession();

    expect(
        queueController.state.actionErrorMessage, contains('advance failed'));
    expect(sessionApi.endCalls, 1);
    expect(roomController.state.status, SessionRoomStatus.ended);
    expect(queueController.state.status, QueueControllerStatus.ended);

    realtime.emit(QueueStateEvent(
      sessionId: _liveSessionId,
      eventId: 'late-queue-state',
      queue: _queueState(version: 2),
    ));
    expect(queueController.state.status, QueueControllerStatus.ended);
  });

  test('session lifecycle refreshes queue without a second realtime stream',
      () async {
    final realtime = _FakeRealtimeClient();
    final queueApi = _FakeQueueApiClient([
      _queueState(version: 1),
      _queueState(version: 2),
    ]);
    final queueController = _queueController(queueApi, realtime);
    final roomController = SessionRoomController(
      _FakeSessionApi(),
      () async => (token: 'token', sessionId: 'backend-session'),
      _NoopMediaSession(),
      realtime: realtime,
      isModerator: true,
      queue: queueController,
    );
    addTearDown(queueController.dispose);
    addTearDown(roomController.dispose);

    await roomController.join(_liveSessionId);

    expect(queueController.state.queue?.version, 1);
    expect(realtime.sessionEventsCalls, 1);

    await roomController.retry();

    expect(queueController.state.queue?.version, 2);
    expect(realtime.sessionEventsCalls, 2);

    await roomController.leave();
    expect(queueController.state.status, QueueControllerStatus.idle);
  });
}

QueueController _queueController(
  QueueApiClient api,
  RealtimeSessionClient realtime,
) =>
    QueueController(
      api,
      () async => (token: 'token', sessionId: 'backend-session'),
      realtime: realtime,
      isManager: true,
    );

class _FakeQueueApiClient extends QueueApiClient {
  _FakeQueueApiClient(this.snapshots) : super(Dio());

  final List<QueueState> snapshots;
  int getQueueCalls = 0;
  int advanceCalls = 0;
  Object? advanceFailure;

  @override
  Future<QueueState> getQueue({
    required String token,
    required String sessionId,
    required String liveSessionId,
  }) async {
    final snapshotIndex =
        getQueueCalls < snapshots.length ? getQueueCalls : snapshots.length - 1;
    getQueueCalls++;
    return snapshots[snapshotIndex];
  }

  @override
  Future<QueueState> advance({
    required String token,
    required String sessionId,
    required String liveSessionId,
    required int expectedVersion,
    String? idempotencyKey,
  }) async {
    advanceCalls++;
    final failure = advanceFailure;
    if (failure != null) throw failure;
    return snapshots.last;
  }
}

class _FakeRealtimeClient implements RealtimeSessionClient {
  final StreamController<RealtimeSessionEvent> _events =
      StreamController<RealtimeSessionEvent>.broadcast(sync: true);

  void emit(RealtimeSessionEvent event) => _events.add(event);
  int sessionEventsCalls = 0;

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
  }) {
    sessionEventsCalls++;
    return _events.stream;
  }
}

class _NoopMediaSession implements MediaSession {
  @override
  Future<void> connect(MediaConnection connection) async {}

  @override
  Future<void> disconnect() async {}

  @override
  Future<void> setMicrophoneEnabled(bool enabled) async {}
}

class _FakeSessionApi extends SessionApiClient {
  _FakeSessionApi() : super(Dio());

  int endCalls = 0;

  @override
  Future<SessionConnection> join({
    required String token,
    required String sessionId,
    required String liveSessionId,
  }) async =>
      _sessionConnection();

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

QueueState _queueState({required int version, int policyVersion = 1}) =>
    QueueState.fromJson({
      'session_id': _liveSessionId,
      'round_id': 'round-1',
      'round_number': 1,
      'round_type': 'revision',
      'lifecycle': 'active',
      'surah_id': 2,
      'from_ayah': 1,
      'to_ayah': 5,
      'grading_required': false,
      'selected_entry_id': null,
      'version': version,
      'policy': {
        'population': 'present_at_activation',
        'unfinished_finalization': 'mark_unfinished_skipped',
        'opt_out': 'approval_required',
        'grade_visibility': 'managers_and_student',
        'grade_correction': 'audited_any_time',
        'version': policyVersion,
      },
      'preorder': const [],
      'entries': [
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

SessionConnection _sessionConnection() => SessionConnection(
      session: _session(),
      mediaConnection: MediaConnection(
        endpoint: 'wss://media.example',
        credential: 'short-lived',
        expiresAt: DateTime.utc(2026, 1, 2),
      ),
    );

SessionModel _session({String status = 'active'}) => SessionModel(
      id: _liveSessionId,
      circleId: 'circle-1',
      status: status,
      mediaMode: 'audio_only',
      participantCount: 0,
      isLocked: false,
    );
