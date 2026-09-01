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

  test('complete sends grade and notes and replaces the snapshot', () async {
    final queueApi = _FakeQueueApiClient([
      _queueState(version: 1),
      _queueState(
          version: 2, entries: [_entryJson('entry-1', status: 'completed')]),
    ]);
    final controller = _queueController(queueApi, _FakeRealtimeClient());
    addTearDown(controller.dispose);
    await controller.connect(_liveSessionId);

    await controller.completeEntry(
        entryId: 'entry-1', grade: 'good', notes: 'Well done');

    expect(queueApi.completeCalls, 1);
    expect(queueApi.lastGrade, 'good');
    expect(queueApi.lastNotes, 'Well done');
    expect(controller.state.queue?.entries.single.status, 'completed');
  });

  test('correction refreshes the authoritative queue snapshot', () async {
    final queueApi = _FakeQueueApiClient([
      _queueState(
          version: 1, entries: [_entryJson('entry-1', status: 'completed')]),
      _queueState(
          version: 2, entries: [_entryJson('entry-1', status: 'completed')]),
    ]);
    final controller = _queueController(queueApi, _FakeRealtimeClient());
    addTearDown(controller.dispose);
    await controller.connect(_liveSessionId);

    await controller.correctGrade(entryId: 'entry-1', grade: 'excellent');

    expect(queueApi.correctCalls, 1);
    expect(queueApi.lastGrade, 'excellent');
    expect(controller.state.status, QueueControllerStatus.ready);
    expect(queueApi.getQueueCalls, 2);
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

  group('T045 (US2): duplicate, out-of-order, and unknown events', () {
    test('applies a duplicated queue change event only once', () async {
      final realtime = _FakeRealtimeClient();
      final queueApi = _FakeQueueApiClient([
        _queueState(version: 1),
        _queueState(version: 2),
      ]);
      final controller = _queueController(queueApi, realtime);
      addTearDown(controller.dispose);
      await controller.connect(_liveSessionId);

      realtime.emit(const QueueChangeEvent(
        sessionId: _liveSessionId,
        eventId: 'duplicate-advanced',
        version: 2,
        type: 'queue.advanced',
      ));
      realtime.emit(const QueueChangeEvent(
        sessionId: _liveSessionId,
        eventId: 'duplicate-advanced',
        version: 2,
        type: 'queue.advanced',
      ));
      await Future<void>.delayed(Duration.zero);

      expect(queueApi.getQueueCalls, 2); // connect + one refresh, never two
      expect(controller.state.queue?.version, 2);
    });

    test('ignores an out-of-order queue change event with a stale version',
        () async {
      final realtime = _FakeRealtimeClient();
      final queueApi = _FakeQueueApiClient([_queueState(version: 2)]);
      final controller = _queueController(queueApi, realtime);
      addTearDown(controller.dispose);
      await controller.connect(_liveSessionId);

      realtime.emit(const QueueChangeEvent(
        sessionId: _liveSessionId,
        eventId: 'stale-entry-update',
        version: 1,
        type: 'queue.entry_updated',
      ));
      await Future<void>.delayed(Duration.zero);

      expect(queueApi.getQueueCalls, 1);
      expect(controller.state.queue?.version, 2);
      expect(controller.state.status, QueueControllerStatus.ready);
    });

    test('ignores an unknown event type without crashing or corrupting state',
        () async {
      final realtime = _FakeRealtimeClient();
      final queueApi = _FakeQueueApiClient([_queueState(version: 1)]);
      final controller = _queueController(queueApi, realtime);
      addTearDown(controller.dispose);
      await controller.connect(_liveSessionId);

      realtime.emit(const _UnknownRealtimeEvent());
      await Future<void>.delayed(Duration.zero);

      expect(queueApi.getQueueCalls, 1);
      expect(controller.state.status, QueueControllerStatus.ready);
      expect(controller.state.queue?.version, 1);
    });

    test('re-fetches on an unrecognized queue event type with a fresh version',
        () async {
      final realtime = _FakeRealtimeClient();
      final queueApi = _FakeQueueApiClient([
        _queueState(version: 1),
        _queueState(version: 2),
      ]);
      final controller = _queueController(queueApi, realtime);
      addTearDown(controller.dispose);
      await controller.connect(_liveSessionId);

      realtime.emit(const QueueChangeEvent(
        sessionId: _liveSessionId,
        eventId: 'future-event',
        version: 2,
        type: 'queue.some_future_event',
      ));
      await Future<void>.delayed(Duration.zero);

      expect(queueApi.getQueueCalls, 2);
      expect(controller.state.queue?.version, 2);
      expect(controller.state.status, QueueControllerStatus.ready);
    });

    test('reconnect replaces projected state with the authoritative snapshot',
        () async {
      final realtime = _FakeRealtimeClient();
      final queueApi = _FakeQueueApiClient([
        _queueState(
          version: 1,
          entries: [_entryJson('entry-1', status: 'waiting')],
        ),
        _queueState(
          version: 3,
          entries: [_entryJson('entry-3', status: 'opted_out')],
        ),
      ]);
      final controller = _queueController(queueApi, realtime);
      addTearDown(controller.dispose);
      await controller.connect(_liveSessionId);

      realtime.emit(QueueStateEvent(
        sessionId: _liveSessionId,
        eventId: 'projected-2',
        queue: _queueState(version: 2, entries: [
          _entryJson('entry-1', status: 'waiting'),
          _entryJson('entry-2', status: 'waiting'),
        ]),
      ));
      expect(controller.state.queue?.entries.length, 2);

      await controller.connect(_liveSessionId);

      // PostgreSQL truth replaces the projected state wholesale.
      expect(queueApi.getQueueCalls, 2);
      expect(controller.state.queue?.version, 3);
      expect(
        controller.state.queue?.entries.map((entry) => entry.id).toList(),
        ['entry-3'],
      );

      // Dedup cache resets on reconnect: the replayed event id applies again.
      realtime.emit(QueueStateEvent(
        sessionId: _liveSessionId,
        eventId: 'projected-2',
        queue: _queueState(
          version: 4,
          entries: [_entryJson('entry-3', status: 'opted_out')],
        ),
      ));
      expect(controller.state.queue?.version, 4);
    });

    test('a new-round snapshot replaces a higher-version previous round',
        () async {
      final controller = _queueController(
        _FakeQueueApiClient([_queueState(version: 5)]),
        _FakeRealtimeClient(),
      );
      addTearDown(controller.dispose);
      await controller.connect(_liveSessionId);

      controller.handleRealtimeEvent(QueueStateEvent(
        sessionId: _liveSessionId,
        eventId: 'new-round-state',
        queue: _queueState(version: 1, roundId: 'round-2'),
      ));

      expect(controller.state.queue?.roundId, 'round-2');
      expect(controller.state.queue?.version, 1);
    });
  });

  group('T045 (US2): student opt-out command', () {
    test(
        'approval_required submits a pending request and keeps the entry '
        'waiting', () async {
      final queueApi = _FakeQueueApiClient([_queueState(version: 1)])
        ..optOutResult = _optOutResult(
          requestStatus: 'pending',
          entryStatus: 'waiting',
        );
      final controller =
          _queueController(queueApi, _FakeRealtimeClient(), isManager: false);
      addTearDown(controller.dispose);
      await controller.connect(_liveSessionId);

      await controller.requestOptOut();

      expect(queueApi.optOutCalls, 1);
      expect(controller.state.optOutFeedback, QueueOptOutFeedback.pending);
      expect(controller.state.queue?.entries.single.status, 'waiting');
      expect(controller.state.actionErrorMessage, isNull);
    });

    test('approved realtime update resolves pending student feedback',
        () async {
      final realtime = _FakeRealtimeClient();
      final queueApi = _FakeQueueApiClient([
        _queueState(version: 1),
        _queueState(version: 2),
        _queueState(
          version: 2,
          entries: [_entryJson('entry-1', status: 'opted_out', version: 2)],
        ),
      ])
        ..optOutResult = _optOutResult(
          requestStatus: 'pending',
          entryStatus: 'waiting',
        );
      final controller = _queueController(queueApi, realtime, isManager: false);
      addTearDown(controller.dispose);
      await controller.connect(_liveSessionId);
      await controller.requestOptOut();

      realtime.emit(const QueueChangeEvent(
        sessionId: _liveSessionId,
        eventId: 'opt-out-approved',
        version: 2,
        type: 'queue.entry_updated',
      ));
      await Future<void>.delayed(Duration.zero);

      expect(controller.state.queue?.entries.single.status, 'opted_out');
      expect(controller.state.optOutFeedback, QueueOptOutFeedback.approved);
    });

    test('an older request refresh cannot overwrite realtime approval',
        () async {
      final realtime = _FakeRealtimeClient();
      final blockedRefresh = Completer<void>();
      final queueApi = _FakeQueueApiClient([
        _queueState(version: 1),
        _queueState(version: 2),
        _queueState(
          version: 2,
          entries: [_entryJson('entry-1', status: 'opted_out', version: 2)],
        ),
      ])
        ..optOutResult = _optOutResult(
          requestStatus: 'pending',
          entryStatus: 'waiting',
        )
        ..blockedGetQueueCall = 2
        ..getQueueBlock = blockedRefresh;
      final controller = _queueController(queueApi, realtime, isManager: false);
      addTearDown(controller.dispose);
      await controller.connect(_liveSessionId);

      final request = controller.requestOptOut();
      while (queueApi.getQueueCalls < 2) {
        await Future<void>.delayed(Duration.zero);
      }
      realtime.emit(const QueueChangeEvent(
        sessionId: _liveSessionId,
        eventId: 'approval-during-refresh',
        version: 2,
        type: 'queue.entry_updated',
      ));
      await Future<void>.delayed(Duration.zero);
      blockedRefresh.complete();
      await request;

      expect(controller.state.queue?.entries.single.status, 'opted_out');
      expect(controller.state.optOutFeedback, QueueOptOutFeedback.approved);
    });

    test('a declined request keeps the entry waiting', () async {
      final queueApi = _FakeQueueApiClient([_queueState(version: 1)])
        ..optOutResult = _optOutResult(
          requestStatus: 'declined',
          entryStatus: 'waiting',
        );
      final controller =
          _queueController(queueApi, _FakeRealtimeClient(), isManager: false);
      addTearDown(controller.dispose);
      await controller.connect(_liveSessionId);

      await controller.requestOptOut();

      expect(queueApi.optOutCalls, 1);
      expect(controller.state.optOutFeedback, QueueOptOutFeedback.declined);
      expect(controller.state.queue?.entries.single.status, 'waiting');
    });

    test('an approved request opts the entry out of the round', () async {
      final queueApi = _FakeQueueApiClient([
        _queueState(version: 1),
        _queueState(
          version: 2,
          entries: [_entryJson('entry-1', status: 'opted_out')],
        ),
      ])
        ..optOutResult = _optOutResult(
          requestStatus: 'approved',
          entryStatus: 'opted_out',
        );
      final controller =
          _queueController(queueApi, _FakeRealtimeClient(), isManager: false);
      addTearDown(controller.dispose);
      await controller.connect(_liveSessionId);

      await controller.requestOptOut();

      expect(queueApi.optOutCalls, 1);
      expect(controller.state.optOutFeedback, QueueOptOutFeedback.approved);
      expect(controller.state.queue?.entries.single.status, 'opted_out');
    });

    test('auto_approve applies immediately without a pending state', () async {
      final queueApi = _FakeQueueApiClient([
        _queueState(version: 1, optOutPolicy: 'auto_approve'),
        _queueState(
          version: 2,
          optOutPolicy: 'auto_approve',
          entries: [_entryJson('entry-1', status: 'opted_out')],
        ),
      ])
        ..optOutResult = _optOutResult(
          requestStatus: 'approved',
          entryStatus: 'opted_out',
        );
      final controller =
          _queueController(queueApi, _FakeRealtimeClient(), isManager: false);
      addTearDown(controller.dispose);
      await controller.connect(_liveSessionId);

      await controller.requestOptOut();

      expect(queueApi.optOutCalls, 1);
      expect(
        controller.state.optOutFeedback,
        QueueOptOutFeedback.autoApproved,
      );
      expect(controller.state.queue?.entries.single.status, 'opted_out');
      expect(queueApi.getQueueCalls, 2);
    });

    test('manager controllers never issue the student-only opt-out command',
        () async {
      final queueApi = _FakeQueueApiClient([_queueState(version: 1)])
        ..optOutResult = _optOutResult(
          requestStatus: 'pending',
          entryStatus: 'waiting',
        );
      final controller = _queueController(queueApi, _FakeRealtimeClient());
      addTearDown(controller.dispose);
      await controller.connect(_liveSessionId);

      await controller.requestOptOut();

      expect(queueApi.optOutCalls, 0);
      expect(controller.state.optOutFeedback, isNull);
    });

    test('opt-out failure is action-local and retains the snapshot', () async {
      final queueApi = _FakeQueueApiClient([_queueState(version: 1)])
        ..optOutFailure = StateError('opt-out failed');
      final controller =
          _queueController(queueApi, _FakeRealtimeClient(), isManager: false);
      addTearDown(controller.dispose);
      await controller.connect(_liveSessionId);

      await controller.requestOptOut();

      expect(controller.state.optOutFeedback, isNull);
      expect(controller.state.actionErrorMessage, contains('opt-out failed'));
      expect(controller.state.queue?.version, 1);
      expect(controller.state.status, QueueControllerStatus.ready);
    });
  });
}

QueueController _queueController(
  QueueApiClient api,
  RealtimeSessionClient realtime, {
  bool isManager = true,
}) =>
    QueueController(
      api,
      () async => (token: 'token', sessionId: 'backend-session'),
      realtime: realtime,
      isManager: isManager,
    );

class _UnknownRealtimeEvent extends RealtimeSessionEvent {
  const _UnknownRealtimeEvent() : super(sessionId: _liveSessionId);
}

class _FakeQueueApiClient extends QueueApiClient {
  _FakeQueueApiClient(this.snapshots) : super(Dio());

  final List<QueueState> snapshots;
  int getQueueCalls = 0;
  int? blockedGetQueueCall;
  Completer<void>? getQueueBlock;
  int advanceCalls = 0;
  Object? advanceFailure;
  int optOutCalls = 0;
  OptOutResult? optOutResult;
  Object? optOutFailure;
  int completeCalls = 0;
  int correctCalls = 0;
  String? lastGrade;
  String? lastNotes;

  @override
  Future<QueueState> getQueue({
    required String token,
    required String sessionId,
    required String liveSessionId,
  }) async {
    getQueueCalls++;
    final call = getQueueCalls;
    if (call == blockedGetQueueCall) await getQueueBlock?.future;
    final snapshotIndex =
        call <= snapshots.length ? call - 1 : snapshots.length - 1;
    return snapshots[snapshotIndex];
  }

  @override
  Future<OptOutResult> optOut({
    required String token,
    required String sessionId,
    required String liveSessionId,
    String? idempotencyKey,
  }) async {
    optOutCalls++;
    final failure = optOutFailure;
    if (failure != null) throw failure;
    return optOutResult!;
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

  @override
  Future<QueueState> completeEntry({
    required String token,
    required String sessionId,
    required String liveSessionId,
    required String entryId,
    required int expectedEntryVersion,
    String? grade,
    String? notes,
    String? idempotencyKey,
  }) async {
    completeCalls++;
    lastGrade = grade;
    lastNotes = notes;
    return snapshots.last;
  }

  @override
  Future<QueueEntry> correctGrade({
    required String token,
    required String sessionId,
    required String liveSessionId,
    required String entryId,
    required int expectedEntryVersion,
    String? grade,
    String? notes,
    bool clearNotes = false,
    String? idempotencyKey,
  }) async {
    correctCalls++;
    lastGrade = grade;
    lastNotes = notes;
    return snapshots.last.entries.single;
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

QueueState _queueState({
  required int version,
  String roundId = 'round-1',
  int policyVersion = 1,
  String optOutPolicy = 'approval_required',
  List<Map<String, dynamic>>? entries,
}) =>
    QueueState.fromJson({
      'session_id': _liveSessionId,
      'round_id': roundId,
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
        'opt_out': optOutPolicy,
        'grade_visibility': 'managers_and_student',
        'grade_correction': 'audited_any_time',
        'version': policyVersion,
      },
      'preorder': const [],
      'entries': entries ??
          [
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

Map<String, dynamic> _entryJson(
  String id, {
  String studentId = 'student-1',
  String studentName = 'مريم',
  int position = 1,
  String status = 'waiting',
  int version = 1,
}) =>
    {
      'id': id,
      'student_id': studentId,
      'student_name': studentName,
      'position': position,
      'status': status,
      'version': version,
    };

OptOutResult _optOutResult({
  required String requestStatus,
  required String entryStatus,
}) =>
    OptOutResult.fromJson({
      'request': {
        'id': 'request-1',
        'queue_entry_id': 'entry-1',
        'status': requestStatus,
        'requested_at': '2026-08-30T09:00:00Z',
        'decided_at':
            requestStatus == 'pending' ? null : '2026-08-30T09:01:00Z',
      },
      'entry': _entryJson('entry-1', status: entryStatus, version: 2),
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
