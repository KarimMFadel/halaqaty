import 'dart:async';
import 'dart:io';

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/auth/data/auth_api_client.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/application/media_session.dart';
import 'package:halaqaty_mobile/features/sessions/application/queue_controller.dart';
import 'package:halaqaty_mobile/features/sessions/application/session_room_controller.dart';
import 'package:halaqaty_mobile/features/sessions/data/queue_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/data/realtime_session_client.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_protocol_constants.dart';
import 'package:halaqaty_mobile/features/sessions/presentation/session_room_screen.dart';
import 'package:integration_test/integration_test.dart';

const _poll = Duration(milliseconds: 100);

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets(
    'T048: late join, opt-out approval flow, duplicate events, reconnect, '
    'and auto-approve (RTL)',
    (tester) async {
      final env = Platform.environment;
      final apiBaseUrl =
          env['T048_API_BASE_URL'] ?? 'http://host.docker.internal:8080/api/v1';
      final teacher = _UserCredentials.fromEnv(env, 'TEACHER');
      final student = _UserCredentials.fromEnv(env, 'STUDENT');

      if (teacher == null || student == null) {
        markTestSkipped(
          'T048_* env vars missing; real-backend integration needs '
          'pre-provisioned Firebase tokens and backend sessions.',
        );
        return;
      }

      final dio = Dio(BaseOptions(baseUrl: apiBaseUrl));
      addTearDown(dio.close);

      final circles = CircleApiClient(dio);
      final sessions = SessionApiClient(dio);
      final queue = QueueApiClient(dio);

      // ── World setup via REST (teacher = queue manager) ────────────────────
      final circle = await circles.createCircle(
        firebaseIdToken: teacher.token,
        sessionId: teacher.sessionId,
        request: const CreateCircleRequest(
          name: 'T048-queue-late-join',
          language: 'ar',
          maxCapacity: 10,
        ),
      );
      addTearDown(() async {
        try {
          await circles.archiveCircle(
            firebaseIdToken: teacher.token,
            sessionId: teacher.sessionId,
            circleId: circle.id,
          );
        } on DioException {
          // Best-effort cleanup.
        }
      });

      await circles.joinCircleByInvite(
        firebaseIdToken: student.token,
        sessionId: student.sessionId,
        inviteCode: circle.inviteCode,
      );

      final liveSession = await sessions.create(
        token: teacher.token,
        sessionId: teacher.sessionId,
        circleId: circle.id,
      );
      final liveSessionId = liveSession.id;
      addTearDown(() async {
        try {
          await sessions.end(
            token: teacher.token,
            sessionId: teacher.sessionId,
            liveSessionId: liveSessionId,
          );
        } on DioException {
          // Best-effort cleanup.
        }
      });

      await sessions.start(
        token: teacher.token,
        sessionId: teacher.sessionId,
        liveSessionId: liveSessionId,
      );

      // Prepare while only the teacher is present; the round activates empty.
      final initialQueue = await queue.prepareRound(
        token: teacher.token,
        sessionId: teacher.sessionId,
        liveSessionId: liveSessionId,
        roundType: 'revision',
        surahId: 1,
        fromAyah: 1,
        toAyah: 7,
        gradingRequired: false,
      );
      expect(initialQueue.lifecycle, 'active');
      expect(initialQueue.entries, isEmpty);

      // Manager joins live so they are authorized on the realtime topic.
      await sessions.join(
        token: teacher.token,
        sessionId: teacher.sessionId,
        liveSessionId: liveSessionId,
      );

      // Student joins late → F-003 appends one waiting entry at the end.
      await sessions.join(
        token: student.token,
        sessionId: student.sessionId,
        liveSessionId: liveSessionId,
      );

      // ── Student UI: durable late-join position ────────────────────────────
      final studentRealtime = _TestRealtimeClient(dio);
      final studentControllers = _StudentRoomBinding();
      addTearDown(studentControllers.dispose);

      await tester.pumpWidget(_studentApp(
        liveSessionId: liveSessionId,
        dio: dio,
        student: student,
        realtime: studentRealtime,
        binding: studentControllers,
      ));
      await tester.pumpAndSettle();

      await tester.tap(find.text('انضمام'));
      await _waitFor(
        () =>
            studentControllers.room?.state.status ==
            SessionRoomStatus.connected,
        timeout: const Duration(seconds: 10),
      );
      await tester.pumpAndSettle();

      final studentQueue = studentControllers.queue!;
      expect(studentQueue.state.queue?.entries, hasLength(1));
      final studentEntry = studentQueue.state.queue!.entries.single;
      expect(studentEntry.studentId, student.userId);
      expect(studentEntry.status, 'waiting');
      expect(find.bySemanticsLabel('موضعك: 1'), findsOneWidget);

      // ── Manager realtime spy for targeted delivery ────────────────────────
      final managerRealtime = _TestRealtimeClient(dio);
      addTearDown(managerRealtime.dispose);
      final managerEvents = <RealtimeSessionEvent>[];
      final managerSub = managerRealtime
          .sessionEvents(
            liveSessionId,
            token: teacher.token,
            backendSessionId: teacher.sessionId,
          )
          .listen(managerEvents.add);
      addTearDown(managerSub.cancel);
      // Wait for the initial queue.state after subscription.
      await tester.runAsync(() => _waitFor(
            () => managerEvents.whereType<QueueStateEvent>().isNotEmpty,
            timeout: const Duration(seconds: 5),
          ));

      // Also spy on the student realtime stream for targeted-delivery checks.
      final studentEvents = <RealtimeSessionEvent>[];
      final studentSpySub = studentRealtime.stream.listen(studentEvents.add);
      addTearDown(studentSpySub.cancel);

      // ── Approval-required opt-out: pending → declined ─────────────────────
      await tester.tap(find.bySemanticsLabel('الاعتذار عن الدور'));
      await tester.pumpAndSettle();
      await _waitFor(
        () => find.text('بانتظار موافقة المعلم').evaluate().isNotEmpty,
        timeout: const Duration(seconds: 5),
      );
      expect(find.text('بانتظار موافقة المعلم'), findsOneWidget);

      // The manager (and only the manager) receives queue.opt_out_requested.
      await _waitFor(
        () => managerEvents
            .whereType<QueueChangeEvent>()
            .where((e) => e.type == 'queue.opt_out_requested')
            .isNotEmpty,
        timeout: const Duration(seconds: 5),
      );
      final requestedEvents = managerEvents
          .whereType<QueueChangeEvent>()
          .where((e) => e.type == 'queue.opt_out_requested')
          .toList();
      expect(requestedEvents, hasLength(1));
      expect(
        studentEvents
            .whereType<QueueChangeEvent>()
            .where((e) => e.type == 'queue.opt_out_requested'),
        isEmpty,
        reason: 'Students must never receive queue.opt_out_requested',
      );

      // Resolve the pending request id from the REST surface (idempotent).
      final firstOptOutResult = await queue.optOut(
        token: student.token,
        sessionId: student.sessionId,
        liveSessionId: liveSessionId,
      );
      await _decideOptOut(
        dio: dio,
        credentials: teacher,
        liveSessionId: liveSessionId,
        requestId: firstOptOutResult.request.id,
        decision: 'declined',
        expectedEntryVersion: firstOptOutResult.entry.version,
      );

      await _waitFor(
        () => find.text('يبقى دورك محفوظًا لك').evaluate().isNotEmpty,
        timeout: const Duration(seconds: 5),
      );
      expect(find.text('يبقى دورك محفوظًا لك'), findsOneWidget);
      expect(studentQueue.state.queue?.entries.single.status, 'waiting');

      // ── Approval-required opt-out: approved ───────────────────────────────
      await tester.tap(find.bySemanticsLabel('الاعتذار عن الدور'));
      await tester.pumpAndSettle();
      await _waitFor(
        () => find.text('بانتظار موافقة المعلم').evaluate().isNotEmpty,
        timeout: const Duration(seconds: 5),
      );

      // Confirm the second manager-only event arrived, then resolve the
      // request id from the REST surface (idempotent replay).
      final requestedEvents2 = managerEvents
          .whereType<QueueChangeEvent>()
          .where((e) => e.type == 'queue.opt_out_requested')
          .toList();
      expect(requestedEvents2, hasLength(2));

      final secondOptOutResult = await queue.optOut(
        token: student.token,
        sessionId: student.sessionId,
        liveSessionId: liveSessionId,
      );
      await _decideOptOut(
        dio: dio,
        credentials: teacher,
        liveSessionId: liveSessionId,
        requestId: secondOptOutResult.request.id,
        decision: 'approved',
        expectedEntryVersion: secondOptOutResult.entry.version,
      );

      await _waitFor(
        () => find.text('تم اعتماد الاعتذار').evaluate().isNotEmpty,
        timeout: const Duration(seconds: 5),
      );
      expect(find.text('تم اعتماد الاعتذار'), findsOneWidget);
      expect(studentQueue.state.queue?.entries.single.status, 'opted_out');

      // ── Duplicate queue.entry_updated replay is deduplicated ──────────────
      final entryUpdatedEvents = managerEvents
          .whereType<QueueChangeEvent>()
          .where((e) => e.type == 'queue.entry_updated')
          .toList();
      expect(entryUpdatedEvents, isNotEmpty);
      final approvalEvent = entryUpdatedEvents.last;
      // Replaying the exact same event must not crash or revert state.
      studentRealtime.inject(approvalEvent);
      await tester.pumpAndSettle(const Duration(milliseconds: 500));
      expect(studentQueue.state.queue?.entries.single.status, 'opted_out');

      // ── Reconnect re-fetches the authoritative GET snapshot ───────────────
      // Break the realtime transport from the client side.
      await studentRealtime.dispose();
      await _waitFor(
        () => studentControllers.room?.state.status == SessionRoomStatus.error,
        timeout: const Duration(seconds: 5),
      );

      // While disconnected, the manager changes the opt-out policy.
      await queue.updatePolicy(
        token: teacher.token,
        sessionId: teacher.sessionId,
        liveSessionId: liveSessionId,
        expectedVersion: studentQueue.state.queue!.policy.version,
        optOut: 'auto_approve',
      );

      // Reconnect: the room rejoins and the queue controller re-fetches.
      await studentControllers.room!.retry();
      await _waitFor(
        () =>
            studentControllers.room?.state.status ==
            SessionRoomStatus.connected,
        timeout: const Duration(seconds: 10),
      );
      await _waitFor(
        () => studentQueue.state.queue?.policy.optOut == 'auto_approve',
        timeout: const Duration(seconds: 5),
      );
      expect(studentQueue.state.queue?.policy.optOut, 'auto_approve');
      expect(find.text('تم اعتماد الاعتذار'), findsOneWidget);

      // ── Auto-approve: fresh round, opt-out approved immediately ───────────
      // Reset creates a new active round; the student gets a new waiting entry.
      await queue.reset(
        token: teacher.token,
        sessionId: teacher.sessionId,
        liveSessionId: liveSessionId,
        roundType: 'revision',
        surahId: 1,
        fromAyah: 1,
        toAyah: 7,
        gradingRequired: false,
        expectedVersion: studentQueue.state.queue!.version,
      );
      await _waitFor(
        () => studentQueue.state.queue?.entries.isNotEmpty ?? false,
        timeout: const Duration(seconds: 5),
      );
      expect(studentQueue.state.queue?.entries.single.status, 'waiting');
      await tester.pumpAndSettle();

      final optOutRequestedCountBefore = managerEvents
          .whereType<QueueChangeEvent>()
          .where((e) => e.type == 'queue.opt_out_requested')
          .length;

      await tester.tap(find.bySemanticsLabel('الاعتذار عن الدور'));
      await tester.pumpAndSettle();

      await _waitFor(
        () => find.text('تم اعتماد الاعتذار تلقائيًا').evaluate().isNotEmpty,
        timeout: const Duration(seconds: 5),
      );
      expect(find.text('تم اعتماد الاعتذار تلقائيًا'), findsOneWidget);
      expect(studentQueue.state.queue?.entries.single.status, 'opted_out');

      // No additional manager-only opt-out-requested event was emitted.
      final optOutRequestedCountAfter = managerEvents
          .whereType<QueueChangeEvent>()
          .where((e) => e.type == 'queue.opt_out_requested')
          .length;
      expect(optOutRequestedCountAfter, optOutRequestedCountBefore);
    },
  );
}

class _UserCredentials {
  const _UserCredentials({
    required this.token,
    required this.sessionId,
    required this.userId,
  });

  final String token;
  final String sessionId;
  final String userId;

  static _UserCredentials? fromEnv(Map<String, String> env, String role) {
    final token = env['T048_${role}_TOKEN'];
    final sessionId = env['T048_${role}_SESSION'];
    final userId = env['T048_${role}_USER_ID'];
    if (token == null || sessionId == null || userId == null) return null;
    return _UserCredentials(
      token: token,
      sessionId: sessionId,
      userId: userId,
    );
  }
}

class _StudentRoomBinding {
  SessionRoomController? room;
  QueueController? queue;

  void attach(SessionRoomController room, QueueController queue) {
    this.room = room;
    this.queue = queue;
  }

  void dispose() {
    room?.dispose();
    queue?.dispose();
  }
}

/// Builds the student session-room app with real REST/WebSocket clients but
/// test-supplied credentials so no Firebase SDK configuration is required.
Widget _studentApp({
  required String liveSessionId,
  required Dio dio,
  required _UserCredentials student,
  required _TestRealtimeClient realtime,
  required _StudentRoomBinding binding,
}) {
  final sessionApi = SessionApiClient(dio);
  final queueApi = QueueApiClient(dio);
  final credentials = () async => (
        token: student.token,
        sessionId: student.sessionId,
      );
  final queue = QueueController(
    queueApi,
    credentials,
    realtime: realtime,
    isManager: false,
  );
  final room = SessionRoomController(
    sessionApi,
    credentials,
    const _NoopMediaSession(),
    realtime: realtime,
    isModerator: false,
    queue: queue,
    currentUserId: student.userId,
  );
  binding.attach(room, queue);

  return ProviderScope(
    overrides: [
      dioProvider.overrideWithValue(dio),
      sessionRoomControllerProvider(liveSessionId).overrideWith((_) => room),
      queueControllerProvider(liveSessionId).overrideWith((_) => queue),
    ],
    child: MaterialApp(
      home: Directionality(
        textDirection: TextDirection.rtl,
        child: SessionRoomScreen(
          sessionId: liveSessionId,
          canStart: false,
        ),
      ),
    ),
  );
}

class _NoopMediaSession implements MediaSession {
  const _NoopMediaSession();

  @override
  Future<void> connect(MediaConnection connection) async {}

  @override
  Future<void> disconnect() async {}

  @override
  Future<void> setMicrophoneEnabled(bool enabled) async {}
}

/// Real WebSocket client with a test-only injection point for duplicate events
/// and a [dispose] hook that forces a transport-level reconnect.
class _TestRealtimeClient extends WebSocketRealtimeSessionClient {
  _TestRealtimeClient(super.dio);

  StreamController<RealtimeSessionEvent>? _merged;
  final _injections =
      StreamController<RealtimeSessionEvent>.broadcast(sync: true);

  /// Exposes the same stream the controller listens to, for test spies.
  Stream<RealtimeSessionEvent> get stream =>
      _merged?.stream ?? const Stream.empty();

  @override
  Stream<RealtimeSessionEvent> sessionEvents(
    String liveSessionId, {
    required String token,
    required String backendSessionId,
  }) {
    final real = super.sessionEvents(
      liveSessionId,
      token: token,
      backendSessionId: backendSessionId,
    );
    final merged = StreamController<RealtimeSessionEvent>.broadcast();
    _merged = merged;

    StreamSubscription<RealtimeSessionEvent>? realSub;
    StreamSubscription<RealtimeSessionEvent>? injectSub;

    void detach() {
      realSub?.cancel();
      injectSub?.cancel();
      if (_merged == merged) _merged = null;
    }

    realSub = real.listen(
      merged.add,
      onError: merged.addError,
      onDone: () {
        merged.close();
        detach();
      },
    );
    injectSub = _injections.stream.listen(merged.add, onError: merged.addError);
    merged.onCancel = detach;

    return merged.stream;
  }

  void inject(RealtimeSessionEvent event) => _injections.add(event);

  @override
  Future<void> dispose() async {
    await _merged?.close();
    _merged = null;
    await super.dispose();
  }
}

Future<void> _decideOptOut({
  required Dio dio,
  required _UserCredentials credentials,
  required String liveSessionId,
  required String requestId,
  required String decision,
  required int expectedEntryVersion,
}) async {
  final response = await dio.post<Map<String, dynamic>>(
    '/sessions/$liveSessionId/queue/opt-out-requests/$requestId/decision',
    data: {
      'decision': decision,
      'expected_entry_version': expectedEntryVersion,
    },
    options: Options(
      headers: {
        SessionHeaders.authorization: 'Bearer ${credentials.token}',
        SessionHeaders.sessionId: credentials.sessionId,
      },
    ),
  );
  if (response.statusCode != 200) {
    throw StateError('Decide opt-out failed: ${response.statusCode}');
  }
}

Future<void> _waitFor(
  bool Function() condition, {
  required Duration timeout,
}) async {
  final deadline = DateTime.now().add(timeout);
  while (!condition() && DateTime.now().isBefore(deadline)) {
    await Future<void>.delayed(_poll);
  }
  if (!condition()) {
    throw TimeoutException('Condition not met within $timeout', timeout);
  }
}
