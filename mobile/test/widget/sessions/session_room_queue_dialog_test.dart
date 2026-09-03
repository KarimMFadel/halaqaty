import 'dart:async';

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter/semantics.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/sessions/application/media_session.dart';
import 'package:halaqaty_mobile/features/sessions/application/queue_controller.dart';
import 'package:halaqaty_mobile/features/sessions/application/session_room_controller.dart';
import 'package:halaqaty_mobile/features/sessions/data/queue_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/data/realtime_session_client.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/presentation/session_room_screen.dart';
import 'package:halaqaty_mobile/features/sessions/presentation/session_ui_labels.dart';

const _sessionId = 'queue-dialog-session';

void main() {
  testWidgets('prepare validates the round range with English feedback',
      (tester) async {
    final semantics = tester.ensureSemantics();
    final fixture = await _pumpManagerRoom(tester);
    addTearDown(fixture.queue.dispose);

    await tester.tap(find.bySemanticsLabel('Prepare round'));
    await tester.pumpAndSettle();

    final roundTypeSemantics =
        tester.getSemantics(find.byType(DropdownButton<String>));
    expect(roundTypeSemantics.getSemanticsData().hasAction(SemanticsAction.tap),
        isTrue);

    await tester.enterText(_editableWithin('Surah number'), '0');
    await tester.pump();
    expect(find.bySemanticsLabel('Surah number must be between 1 and 114'),
        findsOneWidget);
    _expectConfirmDisabled(tester, 'Confirm');

    await tester.enterText(_editableWithin('Surah number'), '115');
    await tester.pump();
    expect(find.bySemanticsLabel('Surah number must be between 1 and 114'),
        findsOneWidget);

    await tester.enterText(_editableWithin('Surah number'), '2');
    await tester.enterText(_editableWithin('From ayah'), '0');
    await tester.pump();
    expect(
        find.bySemanticsLabel('Ayah numbers must be positive'), findsOneWidget);
    _expectConfirmDisabled(tester, 'Confirm');

    await tester.enterText(_editableWithin('From ayah'), '6');
    await tester.enterText(_editableWithin('To ayah'), '5');
    await tester.pump();
    expect(find.bySemanticsLabel('From ayah must not exceed to ayah'),
        findsOneWidget);
    _expectConfirmDisabled(tester, 'Confirm');
    semantics.dispose();
  });

  testWidgets('reset announces Arabic round validation feedback',
      (tester) async {
    final semantics = tester.ensureSemantics();
    final fixture =
        await _pumpManagerRoom(tester, direction: TextDirection.rtl);
    addTearDown(fixture.queue.dispose);

    await tester.tap(find.bySemanticsLabel(SessionUiLabels.resetQueue));
    await tester.pumpAndSettle();
    await tester.enterText(_editableWithin(SessionUiLabels.toAyah), '0');
    await tester.pump();

    expect(
      find.bySemanticsLabel('رقم الآية يجب أن يكون رقمًا موجبًا'),
      findsOneWidget,
    );
    _expectConfirmDisabled(tester, SessionUiLabels.confirm);
    semantics.dispose();
  });

  testWidgets('move dialog offers waiting students and enforces round bounds',
      (tester) async {
    final semantics = tester.ensureSemantics();
    final fixture = await _pumpManagerRoom(tester);
    addTearDown(fixture.queue.dispose);

    await tester.tap(find.bySemanticsLabel('Move student'));
    await tester.pumpAndSettle();
    final studentSemantics =
        tester.getSemantics(find.byType(DropdownButton<String>));
    expect(studentSemantics.getSemanticsData().hasAction(SemanticsAction.tap),
        isTrue);
    expect(find.bySemanticsLabel(RegExp(r'^Student')), findsOneWidget);
    await tester.tap(find.byType(DropdownButton<String>));
    await tester.pumpAndSettle();

    expect(
      find.descendant(
        of: find.byType(DropdownMenuItem<String>),
        matching: find.text('Reciting student'),
      ),
      findsNothing,
    );
    expect(
      find.descendant(
        of: find.byType(DropdownMenuItem<String>),
        matching: find.text('Waiting student'),
      ),
      findsAtLeastNWidgets(1),
    );
    await tester.tap(find.text('Waiting student').last);
    await tester.pump();

    await tester.enterText(_editableWithin('New position'), '0');
    await tester.pump();
    _expectConfirmDisabled(tester, 'Confirm');

    await tester.enterText(_editableWithin('New position'), '4');
    await tester.pump();
    _expectConfirmDisabled(tester, 'Confirm');

    await tester.enterText(_editableWithin('New position'), '3');
    await tester.pump();
    final confirm = find.descendant(
      of: find.bySemanticsLabel('Confirm'),
      matching: find.byType(FilledButton),
    );
    expect(tester.widget<FilledButton>(confirm).onPressed, isNotNull);
    semantics.dispose();
  });

  testWidgets('completion dialog offers only contract grades', (tester) async {
    final fixture = await _pumpManagerRoom(tester);
    addTearDown(fixture.queue.dispose);

    await tester.tap(find.bySemanticsLabel('Complete turn'));
    await tester.pumpAndSettle();
    await tester.tap(find.byType(DropdownButton<String>));
    await tester.pumpAndSettle();

    expect(find.text('acceptable'), findsOneWidget);
    expect(find.text('not_assessed'), findsNothing);
  });

  testWidgets('end-session control stays available when the round finalizes',
      (tester) async {
    final semantics = tester.ensureSemantics();
    final fixture = await _pumpManagerRoom(tester);
    addTearDown(fixture.queue.dispose);
    expect(find.bySemanticsLabel('End session'), findsOneWidget);

    fixture.realtime.emit(QueueStateEvent(
      sessionId: _sessionId,
      eventId: 'finalized-state',
      queue: QueueState.fromJson({
        'session_id': _sessionId,
        'round_id': 'round-1',
        'round_number': 1,
        'round_type': 'revision',
        'lifecycle': 'finalized',
        'surah_id': 2,
        'from_ayah': 1,
        'to_ayah': 5,
        'grading_required': false,
        'selected_entry_id': null,
        'version': 2,
        'policy': const {
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
            'id': 'reciting-entry',
            'student_id': 'student-1',
            'student_name': 'Reciting student',
            'position': 1,
            'status': 'completed',
            'grade': 'good',
            'version': 2,
          },
        ],
      }),
    ));
    await tester.pump();
    await tester.pump();

    // The terminal queue state is rendered read-only...
    expect(find.text('Round finalized; grading is read-only'), findsOneWidget);
    // ...and the manager can still end the session.
    final endSession = find.descendant(
      of: find.bySemanticsLabel('End session'),
      matching: find.byType(FilledButton),
    );
    expect(tester.widget<FilledButton>(endSession).onPressed, isNotNull);
    semantics.dispose();
  });
}

Finder _editableWithin(String label) => find.descendant(
      of: find.bySemanticsLabel(label),
      matching: find.byType(EditableText),
    );

void _expectConfirmDisabled(WidgetTester tester, String label) {
  final confirm = find.descendant(
    of: find.bySemanticsLabel(label),
    matching: find.byType(FilledButton),
  );
  expect(tester.widget<FilledButton>(confirm).onPressed, isNull);
}

Future<_ManagerRoomFixture> _pumpManagerRoom(
  WidgetTester tester, {
  TextDirection direction = TextDirection.ltr,
}) async {
  await tester.binding.setSurfaceSize(const Size(1000, 1000));
  addTearDown(() => tester.binding.setSurfaceSize(null));
  final realtime = _EmptyRealtimeClient();
  final queue = QueueController(
    _DialogQueueApi(),
    () async => (token: 'token', sessionId: 'backend-session'),
    realtime: realtime,
    isManager: true,
  );
  final room = SessionRoomController(
    _DialogSessionApi(),
    () async => (token: 'token', sessionId: 'backend-session'),
    _NoopMediaSession(),
    realtime: realtime,
    isModerator: true,
    queue: queue,
  );
  await tester.pumpWidget(
    ProviderScope(
      overrides: [
        sessionRoomControllerProvider(_sessionId).overrideWith((_) => room),
      ],
      child: MaterialApp(
        home: Directionality(
          textDirection: direction,
          child: const SessionRoomScreen(sessionId: _sessionId),
        ),
      ),
    ),
  );
  await room.join(_sessionId);
  await tester.pump();
  return _ManagerRoomFixture(room, queue, realtime);
}

class _ManagerRoomFixture {
  const _ManagerRoomFixture(this.room, this.queue, this.realtime);

  final SessionRoomController room;
  final QueueController queue;
  final _EmptyRealtimeClient realtime;
}

class _DialogQueueApi extends QueueApiClient {
  _DialogQueueApi() : super(Dio());

  @override
  Future<QueueState> getQueue({
    required String token,
    required String sessionId,
    required String liveSessionId,
  }) async =>
      QueueState.fromJson({
        'session_id': liveSessionId,
        'round_id': 'round-1',
        'round_number': 1,
        'round_type': 'revision',
        'lifecycle': 'active',
        'surah_id': 2,
        'from_ayah': 1,
        'to_ayah': 5,
        'grading_required': false,
        'selected_entry_id': 'reciting-entry',
        'version': 1,
        'policy': const {
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
            'id': 'reciting-entry',
            'student_id': 'student-1',
            'student_name': 'Reciting student',
            'position': 1,
            'status': 'reciting',
            'version': 1,
          },
          {
            'id': 'waiting-entry',
            'student_id': 'student-2',
            'student_name': 'Waiting student',
            'position': 2,
            'status': 'waiting',
            'version': 1,
          },
          {
            'id': 'skipped-entry',
            'student_id': 'student-3',
            'student_name': 'Skipped student',
            'position': 3,
            'status': 'skipped',
            'version': 1,
          },
        ],
      });
}

class _DialogSessionApi extends SessionApiClient {
  _DialogSessionApi() : super(Dio());

  @override
  Future<SessionConnection> join({
    required String token,
    required String sessionId,
    required String liveSessionId,
  }) async =>
      SessionConnection(
        session: const SessionModel(
          id: _sessionId,
          circleId: 'circle-1',
          status: 'active',
          mediaMode: 'audio',
          participantCount: 3,
          isLocked: false,
        ),
        mediaConnection: MediaConnection(
          endpoint: 'wss://media.example',
          credential: 'manager-credential',
          expiresAt: DateTime.utc(2026, 9, 1),
        ),
        isModerator: true,
      );

  @override
  Future<List<SessionParticipant>> participants({
    required String token,
    required String sessionId,
    required String liveSessionId,
  }) async =>
      const [];
}

class _NoopMediaSession implements MediaSession {
  @override
  Future<void> connect(MediaConnection connection) async {}

  @override
  Future<void> disconnect() async {}

  @override
  Future<void> setMicrophoneEnabled(bool enabled) async {}
}

class _EmptyRealtimeClient implements RealtimeSessionClient {
  final StreamController<RealtimeSessionEvent> _events =
      StreamController<RealtimeSessionEvent>.broadcast();

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
