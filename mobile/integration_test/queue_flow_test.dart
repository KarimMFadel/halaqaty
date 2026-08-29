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

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets(
    'Queue flow: manager mutations keep student microphone publishing enabled',
    (tester) async {
      final backend = _QueueFlowBackend();
      final media = _OpenStudentMicrophone();
      final realtime = _EmptyRealtimeClient();
      final queue = QueueController(
        backend,
        () async => (token: 'token', sessionId: 'backend-session'),
        realtime: realtime,
        isManager: true,
      );
      final room = SessionRoomController(
        _QueueFlowSessionApi(),
        () async => (token: 'token', sessionId: 'backend-session'),
        media,
        realtime: realtime,
        isModerator: true,
        queue: queue,
      );
      addTearDown(queue.dispose);

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

      await tester.tap(find.text('Join'));
      await _pumpUntil(
          tester, () => room.state.status == SessionRoomStatus.connected);
      expect(room.state.status, SessionRoomStatus.connected);

      await queue.prepareRound(
        roundType: 'revision',
        surahId: 2,
        fromAyah: 1,
        toAyah: 5,
        gradingRequired: false,
      );
      await tester.pump();
      expect(queue.state.queue?.lifecycle, 'active');
      expect(queue.state.queue?.roundNumber, 1);

      await tester.tap(find.bySemanticsLabel('Select next'));
      await tester.pump();
      expect(queue.state.queue?.selectedEntryId, 'entry-1');

      await tester.tap(find.bySemanticsLabel('Start recitation'));
      await tester.pump();
      expect(_entry(queue, 'entry-1').status, 'reciting');

      await queue.moveEntry('entry-2', 1);
      await tester.pump();
      expect(_entry(queue, 'entry-1').status, 'reciting');
      expect(_entry(queue, 'entry-2').position, 1);

      await tester.tap(find.bySemanticsLabel('Skip turn'));
      await tester.pump();
      expect(_entry(queue, 'entry-1').status, 'skipped');

      await queue.reset(
        roundType: 'revision',
        surahId: 2,
        fromAyah: 1,
        toAyah: 5,
        gradingRequired: false,
      );
      await tester.pump();
      expect(queue.state.queue?.roundNumber, 2);
      expect(queue.state.queue?.lifecycle, 'active');

      final queueFetchesBeforeReconnect = backend.queueFetches;
      await room.retry();
      await _pumpUntil(
          tester, () => room.state.status == SessionRoomStatus.connected);
      expect(backend.queueFetches, queueFetchesBeforeReconnect + 1);
      expect(queue.state.queue?.roundNumber, 2);

      expect(media.microphoneEnabled, isTrue);
      expect(media.microphoneCommands, isZero);
    },
  );
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

QueueEntry _entry(QueueController controller, String entryId) =>
    controller.state.queue!.entries.singleWhere((entry) => entry.id == entryId);

class _QueueFlowBackend extends QueueApiClient {
  _QueueFlowBackend() : super(Dio());

  int queueFetches = 0;
  int _roundNumber = 0;
  int _queueVersion = 0;
  String? _selectedEntryId;
  List<_BackendEntry> _entries = const [];

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
                status: status == 'start' ? 'reciting' : 'skipped',
                version: entry.version + 1,
              )
            : entry)
        .toList(growable: false);
    if (status == 'skip') _selectedEntryId = null;
    return _snapshot();
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
    this.version,
  );

  final String id;
  final String studentId;
  final String studentName;
  final int position;
  final String status;
  final int version;

  _BackendEntry copyWith({String? status, int? position, int? version}) =>
      _BackendEntry(
        id,
        studentId,
        studentName,
        position ?? this.position,
        status ?? this.status,
        version ?? this.version,
      );
}

class _QueueFlowSessionApi extends SessionApiClient {
  _QueueFlowSessionApi() : super(Dio());

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
  }) async =>
      const [
        SessionParticipant(
          userId: 'student-1',
          displayName: 'Student A',
          role: CircleRole.student,
          isCurrentlyPresent: true,
        ),
      ];
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

class _OpenStudentMicrophone implements MediaSession {
  bool microphoneEnabled = true;
  int microphoneCommands = 0;

  @override
  Future<void> connect(MediaConnection connection) async {}

  @override
  Future<void> disconnect() async {}

  @override
  Future<void> setMicrophoneEnabled(bool enabled) async {
    microphoneCommands++;
    microphoneEnabled = enabled;
  }
}

class _EmptyRealtimeClient implements RealtimeSessionClient {
  @override
  Stream<RealtimeSessionEvent> sessionEvents(
    String liveSessionId, {
    required String token,
    required String backendSessionId,
  }) =>
      const Stream.empty();

  @override
  Future<void> dispose() async {}

  @override
  Future<void> lowerHand(String liveSessionId) async {}

  @override
  Future<void> raiseHand(String liveSessionId) async {}
}
