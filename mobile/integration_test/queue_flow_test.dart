import 'dart:async';

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/application/media_session.dart';
import 'package:halaqaty_mobile/features/sessions/application/queue_controller.dart';
import 'package:halaqaty_mobile/features/sessions/application/session_room_controller.dart';
import 'package:halaqaty_mobile/features/sessions/data/queue_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/data/realtime_session_client.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/presentation/session_room_screen.dart';
import 'package:integration_test/integration_test.dart';

const _liveSessionId = 'live-session-1';

/// Bounded wait for dialog/dropdown open-close transitions to finish.
const _dialogSettle = Duration(milliseconds: 300);

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets(
    'Queue flow: manager mutations keep the separate student microphone untouched',
    (tester) async {
      final backend = _QueueFlowBackend();
      final managerMedia = _RecordingMediaSession();
      final realtime = _StreamingRealtimeClient();
      final sessionApi = _QueueFlowSessionApi();
      final queue = QueueController(
        backend,
        () async => (token: 'token', sessionId: 'backend-session'),
        realtime: realtime,
        isManager: true,
      );
      final room = SessionRoomController(
        sessionApi,
        () async => (token: 'token', sessionId: 'backend-session'),
        managerMedia,
        realtime: realtime,
        isModerator: true,
        queue: queue,
      );
      addTearDown(queue.dispose);
      final student = _StudentRoomFixture();
      addTearDown(student.dispose);
      await student.room.join(_liveSessionId);
      expect(student.room.state.status, SessionRoomStatus.connected);
      expect(
          student.media.connectedRoom?.endpoint, 'wss://student-media.example');

      await tester.pumpWidget(
        ProviderScope(
          overrides: [
            sessionRoomControllerProvider(_liveSessionId).overrideWith(
              (_) => room,
            ),
          ],
          child: MaterialApp(
            initialRoute: '/session',
            routes: {
              '/session': (_) => const SessionRoomScreen(
                    sessionId: _liveSessionId,
                  ),
            },
          ),
        ),
      );

      // The manager connects independently from the student media room.
      await tester.tap(find.text('Join'));
      await _pumpUntil(
          tester, () => room.state.status == SessionRoomStatus.connected);
      expect(room.state.status, SessionRoomStatus.connected);

      // Prepare goes through the round-details dialog, not the controller.
      await tester.tap(find.bySemanticsLabel('Prepare round'));
      await tester.pump(_dialogSettle);
      await tester.enterText(_editableWithin('Surah number'), '2');
      await tester.enterText(_editableWithin('To ayah'), '5');
      await tester.pump();
      await tester.tap(find.bySemanticsLabel('Confirm'));
      await _pumpUntil(tester, () => queue.state.queue?.roundNumber == 1);
      expect(queue.state.queue?.lifecycle, 'active');

      await tester.tap(find.bySemanticsLabel('Select next'));
      await tester.pump();
      expect(queue.state.queue?.selectedEntryId, 'entry-1');

      await tester.tap(find.bySemanticsLabel('Start recitation'));
      await tester.pump();
      expect(_entry(queue, 'entry-1').status, 'reciting');
      // Manager queue actions never touch the separate student microphone.
      expect(student.media.microphoneCommands, isZero);

      // Move entry-2 to position 1 through the move dialog while entry-1
      // keeps reciting.
      await tester.tap(find.bySemanticsLabel('Move student'));
      await tester.pump(_dialogSettle);
      expect(find.text('Student B'), findsWidgets);
      await tester.enterText(_editableWithin('New position'), '1');
      await tester.pump();
      await tester.tap(find.bySemanticsLabel('Confirm'));
      await _pumpUntil(tester, () => _entry(queue, 'entry-2').position == 1);
      expect(_entry(queue, 'entry-1').status, 'reciting');

      await tester.tap(find.bySemanticsLabel('Skip turn'));
      await tester.pump();
      expect(_entry(queue, 'entry-1').status, 'skipped');

      // Reset goes through the round-details dialog prefilled from the queue.
      await tester.tap(find.bySemanticsLabel('Reset round'));
      await tester.pump(_dialogSettle);
      await tester.tap(find.bySemanticsLabel('Confirm'));
      await _pumpUntil(tester, () => queue.state.queue?.roundNumber == 2);
      expect(queue.state.queue?.lifecycle, 'active');

      // A realtime transport closure, not a participants REST failure, makes
      // the room retryable. The reconnect then replaces the queue from GET.
      final queueFetchesBeforeReconnect = backend.queueFetches;
      final participantsBeforeDisconnect = sessionApi.participantsCalls;
      backend.replaceAuthoritativeSnapshot(roundNumber: 9);
      await realtime.disconnect();
      await _pumpUntil(
          tester, () => room.state.status == SessionRoomStatus.error);
      expect(sessionApi.participantsCalls, participantsBeforeDisconnect);
      expect(realtime.sessionEventsCalls, 1);
      await tester.tap(find.text('Retry'));
      await _pumpUntil(
          tester, () => room.state.status == SessionRoomStatus.connected);
      expect(backend.queueFetches, queueFetchesBeforeReconnect + 1);
      expect(queue.state.queue?.roundNumber, 9);
      expect(realtime.sessionEventsCalls, 2);

      expect(student.media.microphoneEnabled, isTrue);
      expect(student.media.microphoneCommands, isZero);
    },
  );

  testWidgets('T062: complete, correct, reset, and end keeps session usable',
      (tester) async {
    final backend = _QueueFlowBackend();
    final realtime = _StreamingRealtimeClient();
    final queueController = QueueController(
      backend,
      () async => (token: 'token', sessionId: 'backend-session'),
      realtime: realtime,
      isManager: true,
    );
    final room = SessionRoomController(
      _QueueFlowSessionApi(),
      () async => (token: 'token', sessionId: 'backend-session'),
      _RecordingMediaSession(),
      realtime: realtime,
      isModerator: true,
      queue: queueController,
    );
    addTearDown(room.dispose);

    await room.join(_liveSessionId);
    await queueController.prepareRound(
      roundType: 'revision',
      surahId: 2,
      fromAyah: 1,
      toAyah: 5,
      gradingRequired: false,
    );
    await queueController.advance();
    await queueController.startEntry('entry-1');
    await queueController.completeEntry(entryId: 'entry-1');
    expect(queueController.state.queue!.entries.single.status, 'completed');

    await queueController.correctGrade(
        entryId: 'entry-1', grade: 'good', notes: 'Review tajweed');
    expect(backend.correctedGrade, 'good');
    expect(backend.correctedNotes, 'Review tajweed');

    await queueController.reset(
      roundType: 'revision',
      surahId: 2,
      fromAyah: 1,
      toAyah: 5,
      gradingRequired: false,
    );
    expect(queueController.state.queue!.roundNumber, 2);
    await room.endSession();
    expect(room.state.status, SessionRoomStatus.ended);
  });
}

Future<void> _pumpUntil(
  WidgetTester tester,
  bool Function() condition, {
  Duration timeout = const Duration(seconds: 5),
}) async {
  final deadline = DateTime.now().add(timeout);
  while (!condition() && DateTime.now().isBefore(deadline)) {
    await tester.pump(const Duration(milliseconds: 50));
  }
  await tester.pump();
  expect(condition(), isTrue, reason: 'Timed out waiting for the session room');
}

/// Locates the editable text of a field labeled via its Semantics wrapper.
Finder _editableWithin(String label) => find.descendant(
      of: find.bySemanticsLabel(label),
      matching: find.byType(EditableText),
    );

QueueEntry _entry(QueueController controller, String entryId) =>
    controller.state.queue!.entries.singleWhere((entry) => entry.id == entryId);

class _QueueFlowBackend extends QueueApiClient {
  _QueueFlowBackend() : super(Dio());

  int queueFetches = 0;
  int _roundNumber = 0;
  int _queueVersion = 0;
  String? _selectedEntryId;
  List<_BackendEntry> _entries = const [];
  String? correctedGrade;
  String? correctedNotes;

  void replaceAuthoritativeSnapshot({required int roundNumber}) {
    _roundNumber = roundNumber;
    _queueVersion += 10;
  }

  @override
  Future<QueueState> getQueue({
    required String token,
    required String sessionId,
    required String liveSessionId,
  }) async {
    queueFetches++;
    return _snapshot();
  }

  @override
  Future<QueueState> prepareRound({
    required String token,
    required String sessionId,
    required String liveSessionId,
    required String roundType,
    required int surahId,
    required int fromAyah,
    required int toAyah,
    required bool gradingRequired,
    List<String>? studentOrder,
    String? idempotencyKey,
  }) async {
    _roundNumber++;
    _queueVersion++;
    _selectedEntryId = null;
    _entries = const [
      _BackendEntry('entry-1', 'student-1', 'Student A', 1, 'waiting', 1),
      _BackendEntry('entry-2', 'student-2', 'Student B', 2, 'waiting', 1),
    ];
    return _snapshot();
  }

  @override
  Future<QueueState> advance({
    required String token,
    required String sessionId,
    required String liveSessionId,
    required int expectedVersion,
    String? idempotencyKey,
  }) async {
    _queueVersion++;
    _selectedEntryId =
        _entries.firstWhere((entry) => entry.status == 'waiting').id;
    return _snapshot();
  }

  @override
  Future<QueueState> moveEntry({
    required String token,
    required String sessionId,
    required String liveSessionId,
    required String entryId,
    required int newPosition,
    required int expectedVersion,
    String? idempotencyKey,
  }) async {
    _queueVersion++;
    final moved = _entries.singleWhere((entry) => entry.id == entryId);
    _entries = [
      moved.copyWith(position: newPosition, version: moved.version + 1),
      for (final entry in _entries)
        if (entry.id != entryId && entry.position >= newPosition)
          entry.copyWith(position: entry.position + 1)
        else if (entry.id != entryId)
          entry,
    ];
    return _snapshot();
  }

  @override
  Future<QueueState> updateEntryStatus({
    required String token,
    required String sessionId,
    required String liveSessionId,
    required String entryId,
    required String status,
    required int expectedEntryVersion,
    String? idempotencyKey,
  }) async {
    _queueVersion++;
    _entries = _entries
        .map((entry) => entry.id == entryId
            ? entry.copyWith(
                status: status == 'start'
                    ? 'reciting'
                    : status == 'completed'
                        ? 'completed'
                        : 'skipped',
                version: entry.version + 1,
              )
            : entry)
        .toList(growable: false);
    if (status == 'skip') _selectedEntryId = null;
    return _snapshot();
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
  }) =>
      updateEntryStatus(
        token: token,
        sessionId: sessionId,
        liveSessionId: liveSessionId,
        entryId: entryId,
        status: 'completed',
        expectedEntryVersion: expectedEntryVersion,
        idempotencyKey: idempotencyKey,
      );

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
    correctedGrade = grade;
    correctedNotes = notes;
    final entry = _entries.singleWhere((entry) => entry.id == entryId);
    return QueueEntry.fromJson({
      'id': entry.id,
      'student_id': entry.studentId,
      'student_name': entry.studentName,
      'position': entry.position,
      'status': entry.status,
      'grade': grade,
      'grade_notes': notes,
      'version': entry.version + 1,
    });
  }

  @override
  Future<QueueState> reset({
    required String token,
    required String sessionId,
    required String liveSessionId,
    required String roundType,
    required int surahId,
    required int fromAyah,
    required int toAyah,
    required bool gradingRequired,
    required int expectedVersion,
    List<String>? studentOrder,
    String? idempotencyKey,
  }) async =>
      prepareRound(
        token: token,
        sessionId: sessionId,
        liveSessionId: liveSessionId,
        roundType: roundType,
        surahId: surahId,
        fromAyah: fromAyah,
        toAyah: toAyah,
        gradingRequired: gradingRequired,
        studentOrder: studentOrder,
        idempotencyKey: idempotencyKey,
      );

  QueueState _snapshot() => QueueState.fromJson({
        'session_id': _liveSessionId,
        'round_id': 'round-$_roundNumber',
        'round_number': _roundNumber,
        'round_type': 'revision',
        'lifecycle': 'active',
        'surah_id': 2,
        'from_ayah': 1,
        'to_ayah': 5,
        'grading_required': false,
        'selected_entry_id': _selectedEntryId,
        'version': _queueVersion,
        'policy': {
          'population': 'present_at_activation',
          'unfinished_finalization': 'mark_unfinished_skipped',
          'opt_out': 'approval_required',
          'grade_visibility': 'managers_and_student',
          'grade_correction': 'audited_any_time',
          'version': 1,
        },
        'preorder': const [],
        'entries': _entries
            .map((entry) => {
                  'id': entry.id,
                  'student_id': entry.studentId,
                  'student_name': entry.studentName,
                  'position': entry.position,
                  'status': entry.status,
                  'grade': entry.grade,
                  'grade_notes': entry.gradeNotes,
                  'version': entry.version,
                })
            .toList(growable: false),
      });
}

class _BackendEntry {
  const _BackendEntry(
    this.id,
    this.studentId,
    this.studentName,
    this.position,
    this.status,
    this.version, {
    this.grade,
    this.gradeNotes,
  });

  final String id;
  final String studentId;
  final String studentName;
  final int position;
  final String status;
  final int version;
  final String? grade;
  final String? gradeNotes;

  _BackendEntry copyWith({
    String? status,
    int? position,
    int? version,
    String? grade,
    String? gradeNotes,
  }) =>
      _BackendEntry(
        id,
        studentId,
        studentName,
        position ?? this.position,
        status ?? this.status,
        version ?? this.version,
        grade: grade ?? this.grade,
        gradeNotes: gradeNotes ?? this.gradeNotes,
      );
}

class _QueueFlowSessionApi extends SessionApiClient {
  _QueueFlowSessionApi() : super(Dio());

  int participantsCalls = 0;

  @override
  Future<SessionConnection> join({
    required String token,
    required String sessionId,
    required String liveSessionId,
  }) async =>
      _connection;

  @override
  Future<List<SessionParticipant>> participants({
    required String token,
    required String sessionId,
    required String liveSessionId,
  }) async {
    participantsCalls++;
    return const [
      SessionParticipant(
        userId: 'student-1',
        displayName: 'Student A',
        role: CircleRole.student,
        isCurrentlyPresent: true,
      ),
    ];
  }

  @override
  Future<SessionModel> end({
    required String token,
    required String sessionId,
    required String liveSessionId,
  }) async =>
      const SessionModel(
        id: _liveSessionId,
        circleId: 'circle-1',
        status: 'ended',
        mediaMode: 'audio',
        participantCount: 0,
        isLocked: false,
      );
}

final _connection = SessionConnection(
  session: const SessionModel(
    id: _liveSessionId,
    circleId: 'circle-1',
    status: 'active',
    mediaMode: 'audio',
    participantCount: 2,
    isLocked: false,
  ),
  mediaConnection: MediaConnection(
    endpoint: 'wss://media.example',
    credential: 'short-lived-credential',
    expiresAt: DateTime.utc(2026, 9, 1),
  ),
  isModerator: true,
);

class _RecordingMediaSession implements MediaSession {
  MediaConnection? connectedRoom;

  @override
  Future<void> connect(MediaConnection connection) async {
    connectedRoom = connection;
  }

  @override
  Future<void> disconnect() async {}

  @override
  Future<void> setMicrophoneEnabled(bool enabled) async {}
}

class _StudentMediaSession implements MediaSession {
  bool microphoneEnabled = true;
  int microphoneCommands = 0;
  MediaConnection? connectedRoom;

  @override
  Future<void> connect(MediaConnection connection) async {
    connectedRoom = connection;
  }

  @override
  Future<void> disconnect() async {}

  @override
  Future<void> setMicrophoneEnabled(bool enabled) async {
    microphoneCommands++;
    microphoneEnabled = enabled;
  }
}

class _StreamingRealtimeClient implements RealtimeSessionClient {
  StreamController<RealtimeSessionEvent>? _events;
  int sessionEventsCalls = 0;

  Future<void> disconnect() => _events?.close() ?? Future.value();

  @override
  Stream<RealtimeSessionEvent> sessionEvents(
    String liveSessionId, {
    required String token,
    required String backendSessionId,
  }) {
    sessionEventsCalls++;
    _events = StreamController<RealtimeSessionEvent>.broadcast(sync: true);
    return _events!.stream;
  }

  @override
  Future<void> dispose() => disconnect();

  @override
  Future<void> lowerHand(String liveSessionId) async {}

  @override
  Future<void> raiseHand(String liveSessionId) async {}
}

class _StudentRoomFixture {
  _StudentRoomFixture()
      : media = _StudentMediaSession(),
        realtime = _StreamingRealtimeClient() {
    // The room must use the same dedicated media and realtime instances that
    // this fixture observes; queue actions are not injected into it.
    room = SessionRoomController(
      _StudentSessionApi(),
      () async => (token: 'student-token', sessionId: 'student-session'),
      media,
      realtime: realtime,
    );
  }

  final _StudentMediaSession media;
  final _StreamingRealtimeClient realtime;
  late final SessionRoomController room;

  void dispose() {
    room.dispose();
  }
}

class _StudentSessionApi extends SessionApiClient {
  _StudentSessionApi() : super(Dio());

  @override
  Future<SessionConnection> join({
    required String token,
    required String sessionId,
    required String liveSessionId,
  }) async =>
      SessionConnection(
        session: const SessionModel(
          id: _liveSessionId,
          circleId: 'circle-1',
          status: 'active',
          mediaMode: 'audio',
          participantCount: 1,
          isLocked: false,
        ),
        mediaConnection: MediaConnection(
          endpoint: 'wss://student-media.example',
          credential: 'student-short-lived-credential',
          expiresAt: DateTime.utc(2026, 9, 1),
        ),
      );

  @override
  Future<List<SessionParticipant>> participants({
    required String token,
    required String sessionId,
    required String liveSessionId,
  }) async =>
      const [];
}
