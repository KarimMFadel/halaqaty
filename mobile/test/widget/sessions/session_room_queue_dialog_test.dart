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

  testWidgets('completion dialog offers only contract grades, localized',
      (tester) async {
    final fixture = await _pumpManagerRoom(tester);
    addTearDown(fixture.queue.dispose);

    await tester.tap(find.bySemanticsLabel('Complete turn'));
    await tester.pumpAndSettle();
    await tester.tap(find.byType(DropdownButton<String>));
    await tester.pumpAndSettle();

    expect(find.text('Acceptable'), findsWidgets);
    expect(find.text('Needs review'), findsWidgets);
    // Never the raw contract values (FR-028).
    expect(find.text('acceptable'), findsNothing);
    expect(find.text('needs_review'), findsNothing);
    expect(find.text('not_assessed'), findsNothing);
  });

  testWidgets('manager can review queue policy and cancel without changing it',
      (tester) async {
    final fixture = await _pumpManagerRoom(tester);
    addTearDown(fixture.queue.dispose);

    await tester.tap(find.bySemanticsLabel('Queue policy'));
    await tester.pumpAndSettle();
    expect(find.byType(AlertDialog), findsOneWidget);
    expect(find.text('Present at activation'), findsWidgets);
    expect(find.text('Approval required'), findsWidgets);
    final save = find.widgetWithText(FilledButton, 'Save');
    expect(tester.widget<FilledButton>(save).onPressed, isNull);

    await tester.tap(find.text('Cancel'));
    await tester.pumpAndSettle();
    expect(find.byType(AlertDialog), findsNothing);
    expect(fixture.queue.state.queue!.policy.optOut, 'approval_required');
  });

  testWidgets('manager policy edit updates the authoritative queue snapshot',
      (tester) async {
    final fixture = await _pumpManagerRoom(tester);
    addTearDown(fixture.queue.dispose);

    await tester.tap(find.bySemanticsLabel('Queue policy'));
    await tester.pumpAndSettle();
    final approvedValues = <String, List<String>>{
      'Population': ['present_at_activation', 'all_active_students'],
      'Finalization': ['mark_unfinished_skipped', 'preserve_last_state'],
      'OptOut': ['approval_required', 'auto_approve'],
      'Visibility': [
        'managers_and_student',
        'managers_only',
        'all_participants',
      ],
      'Correction': [
        'audited_any_time',
        'before_round_finalization',
        'immutable',
      ],
    };
    for (final (field, label, choices) in [
      (
        'Population',
        'All active students',
        ['Present at activation', 'All active students']
      ),
      (
        'Finalization',
        'Preserve last state',
        ['Skip unfinished turns', 'Preserve last state']
      ),
      ('OptOut', 'Auto approve', ['Approval required', 'Auto approve']),
      (
        'Visibility',
        'Managers only',
        ['Managers and student', 'Managers only', 'All participants']
      ),
      (
        'Correction',
        'Immutable',
        ['Audited any time', 'Before round finalization', 'Immutable']
      ),
    ]) {
      final control = find.byKey(Key('queuePolicy$field'));
      final dropdown = tester.widget<DropdownButton<String>>(
        find.descendant(
          of: control,
          matching: find.byType(DropdownButton<String>),
        ),
      );
      expect(dropdown.items!.map((item) => item.value), approvedValues[field]);
      await tester.ensureVisible(control);
      await tester.tap(control);
      await tester.pumpAndSettle();
      for (final choice in choices) {
        expect(find.text(choice), findsWidgets);
      }
      await tester.tap(find.text(label).last);
      await tester.pumpAndSettle();
    }
    await tester.tap(find.text('Save'));
    await tester.pumpAndSettle();

    expect(find.byType(AlertDialog), findsNothing);
    final policy = fixture.queue.state.queue!.policy;
    expect(policy.population, 'all_active_students');
    expect(policy.unfinishedFinalization, 'preserve_last_state');
    expect(policy.optOut, 'auto_approve');
    expect(policy.gradeVisibility, 'managers_only');
    expect(policy.gradeCorrection, 'immutable');
    expect(fixture.queue.state.actionErrorMessage, isNull);
  });

  testWidgets('Arabic policy editor saves an approved choice', (tester) async {
    final fixture =
        await _pumpManagerRoom(tester, direction: TextDirection.rtl);
    addTearDown(fixture.queue.dispose);

    await tester.tap(find.bySemanticsLabel(SessionUiLabels.queuePolicy));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('queuePolicyOptOut')));
    await tester.pumpAndSettle();
    await tester.tap(find.text('موافقة تلقائية').last);
    await tester.pumpAndSettle();
    await tester.tap(find.text('حفظ'));
    await tester.pumpAndSettle();

    expect(fixture.queue.state.queue!.policy.optOut, 'auto_approve');
  });

  testWidgets('stale policy editor keeps the newer policy', (tester) async {
    final fixture = await _pumpManagerRoom(tester);
    addTearDown(fixture.queue.dispose);

    await tester.tap(find.bySemanticsLabel('Queue policy'));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('queuePolicyOptOut')));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Auto approve').last);
    await tester.pumpAndSettle();
    fixture.api.policyVersion = 2;
    fixture.api.queueVersion = 2;
    fixture.realtime.emit(QueueStateEvent(
      sessionId: _sessionId,
      eventId: 'newer-policy',
      queue: await fixture.api.getQueue(
        token: 'token',
        sessionId: 'backend-session',
        liveSessionId: _sessionId,
      ),
    ));
    await tester.pump();
    await tester.tap(find.text('Save'));
    await tester.pumpAndSettle();

    expect(fixture.queue.state.queue!.policy.optOut, 'approval_required');
    expect(fixture.queue.state.queue!.policy.version, 2);
    expect(find.text('Unable to update queue'), findsOneWidget);
  });

  testWidgets('rejected policy edit leaves the old policy visible',
      (tester) async {
    final fixture = await _pumpManagerRoom(tester);
    addTearDown(fixture.queue.dispose);
    fixture.api.rejectPolicy = true;

    await tester.tap(find.bySemanticsLabel('Queue policy'));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('queuePolicyOptOut')));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Auto approve').last);
    await tester.pumpAndSettle();
    await tester.tap(find.text('Save'));
    await tester.pumpAndSettle();

    expect(fixture.queue.state.queue!.policy.optOut, 'approval_required');
    expect(find.text('Unable to update queue'), findsOneWidget);
  });

  testWidgets('prepared queue reorder persists the full student order',
      (tester) async {
    final fixture = await _pumpManagerRoom(tester, prepared: true);
    addTearDown(fixture.queue.dispose);

    expect(find.text('No recitation round'), findsNothing);

    await tester.tap(find.bySemanticsLabel('Reorder queue'));
    await tester.pumpAndSettle();
    expect(find.byType(AlertDialog), findsOneWidget);
    expect(
        tester
            .widget<FilledButton>(find.widgetWithText(FilledButton, 'Save'))
            .onPressed,
        isNull);
    expect(find.byTooltip('Move First student down to position 2'),
        findsOneWidget);
    await tester.tap(find.byKey(const Key('queueMoveDown-student-1')));
    await tester.pump();
    await tester.tap(find.text('Save'));
    await tester.pumpAndSettle();

    expect(fixture.queue.state.queue!.preorder.map((item) => item.studentId),
        ['student-2', 'student-1']);
    expect(fixture.queue.state.actionErrorMessage, isNull);
  });

  testWidgets('Arabic reorder can be cancelled without mutation',
      (tester) async {
    final fixture = await _pumpManagerRoom(tester,
        direction: TextDirection.rtl, prepared: true);
    addTearDown(fixture.queue.dispose);

    await tester.tap(find.bySemanticsLabel(SessionUiLabels.reorderQueue));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('queueMoveDown-student-1')));
    await tester.pump();
    await tester.tap(find.text(SessionUiLabels.cancel));
    await tester.pumpAndSettle();

    expect(fixture.api.preorderOrder, ['student-1', 'student-2']);
    expect(find.byType(AlertDialog), findsNothing);
  });

  testWidgets('rejected reorder keeps the server order and shows an error',
      (tester) async {
    final fixture = await _pumpManagerRoom(tester, prepared: true);
    addTearDown(fixture.queue.dispose);
    fixture.api.rejectReorder = true;

    await tester.tap(find.bySemanticsLabel('Reorder queue'));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('queueMoveDown-student-1')));
    await tester.pump();
    await tester.tap(find.text('Save'));
    await tester.pumpAndSettle();

    expect(fixture.api.preorderOrder, ['student-1', 'student-2']);
    expect(fixture.queue.state.queue!.preorder.map((item) => item.studentId),
        ['student-1', 'student-2']);
    expect(fixture.queue.state.queue!.version, 2);
    expect(find.text('Unable to update queue'), findsOneWidget);
  });

  testWidgets('stale reorder dialog cannot overwrite a newer queue',
      (tester) async {
    final fixture = await _pumpManagerRoom(tester, prepared: true);
    addTearDown(fixture.queue.dispose);

    await tester.tap(find.bySemanticsLabel('Reorder queue'));
    await tester.pumpAndSettle();
    fixture.api.queueVersion = 2;
    fixture.realtime.emit(QueueStateEvent(
      sessionId: _sessionId,
      eventId: 'newer-queue',
      queue: await fixture.api.getQueue(
        token: 'token',
        sessionId: 'backend-session',
        liveSessionId: _sessionId,
      ),
    ));
    await tester.pump();
    await tester.tap(find.byKey(const Key('queueMoveDown-student-1')));
    await tester.pump();
    await tester.tap(find.text('Save'));
    await tester.pumpAndSettle();

    expect(fixture.api.preorderOrder, ['student-1', 'student-2']);
    expect(fixture.queue.state.queue!.version, 2);
    expect(find.text('Unable to update queue'), findsOneWidget);
  });

  testWidgets('active round does not offer full-list reorder', (tester) async {
    final fixture = await _pumpManagerRoom(tester);
    addTearDown(fixture.queue.dispose);

    final reorder = find.descendant(
      of: find.bySemanticsLabel('Reorder queue'),
      matching: find.byType(OutlinedButton),
    );
    expect(tester.widget<OutlinedButton>(reorder).onPressed, isNull);
  });

  testWidgets('student and ended rooms cannot edit queue policy',
      (tester) async {
    final student = await _pumpManagerRoom(tester, isManager: false);
    addTearDown(student.queue.dispose);
    expect(find.bySemanticsLabel('Queue policy'), findsNothing);

    final manager = await _pumpManagerRoom(tester);
    addTearDown(manager.queue.dispose);
    manager.realtime.emit(const SessionEndedEvent(sessionId: _sessionId));
    await tester.pump();
    final policy = find.descendant(
      of: find.bySemanticsLabel('Queue policy'),
      matching: find.byType(OutlinedButton),
    );
    if (policy.evaluate().isNotEmpty) {
      expect(tester.widget<OutlinedButton>(policy).onPressed, isNull);
    } else {
      expect(find.bySemanticsLabel('Queue policy'), findsNothing);
    }
  });

  testWidgets('reset dialog warns that current round progress is discarded',
      (tester) async {
    final fixture =
        await _pumpManagerRoom(tester, direction: TextDirection.rtl);
    addTearDown(fixture.queue.dispose);

    await tester.tap(find.bySemanticsLabel(SessionUiLabels.resetQueue));
    await tester.pumpAndSettle();

    expect(find.text(SessionUiLabels.resetConsequence), findsOneWidget);
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
    final endSession = find.widgetWithText(FilledButton, 'End session');
    expect(tester.widget<FilledButton>(endSession).onPressed, isNotNull);
    final policy = find.descendant(
      of: find.bySemanticsLabel('Queue policy'),
      matching: find.byType(OutlinedButton),
    );
    expect(tester.widget<OutlinedButton>(policy).onPressed, isNull);
    semantics.dispose();
  });

  testWidgets('manager queue room scrolls at a short viewport without overflow',
      (tester) async {
    final fixture = await _pumpManagerRoom(
      tester,
      surfaceSize: const Size(800, 600),
    );
    addTearDown(fixture.queue.dispose);

    expect(find.text('Recitation queue'), findsOneWidget);
    expect(tester.takeException(), isNull);
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
  Size surfaceSize = const Size(1000, 1000),
  bool prepared = false,
  bool isManager = true,
}) async {
  await tester.binding.setSurfaceSize(surfaceSize);
  addTearDown(() => tester.binding.setSurfaceSize(null));
  final realtime = _EmptyRealtimeClient();
  final api = _DialogQueueApi(prepared: prepared);
  final queue = QueueController(
    api,
    () async => (token: 'token', sessionId: 'backend-session'),
    realtime: realtime,
    isManager: isManager,
  );
  final room = SessionRoomController(
    _DialogSessionApi(isModerator: isManager),
    () async => (token: 'token', sessionId: 'backend-session'),
    _NoopMediaSession(),
    realtime: realtime,
    isModerator: isManager,
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
  return _ManagerRoomFixture(room, queue, realtime, api);
}

class _ManagerRoomFixture {
  const _ManagerRoomFixture(this.room, this.queue, this.realtime, this.api);

  final SessionRoomController room;
  final QueueController queue;
  final _EmptyRealtimeClient realtime;
  final _DialogQueueApi api;
}

class _DialogQueueApi extends QueueApiClient {
  _DialogQueueApi({this.prepared = false}) : super(Dio());

  final bool prepared;
  List<String> preorderOrder = ['student-1', 'student-2'];
  bool rejectReorder = false;
  bool rejectPolicy = false;
  int queueVersion = 1;
  int policyVersion = 1;

  @override
  Future<QueueState> reorder({
    required String token,
    required String sessionId,
    required String liveSessionId,
    required List<String> orderedIds,
    required int expectedVersion,
    String? idempotencyKey,
  }) async {
    if (rejectReorder || expectedVersion != queueVersion) {
      queueVersion = 2;
      throw const QueueApiException(
        statusCode: 409,
        code: 'QUEUE_VERSION_CONFLICT',
        message: 'Queue changed on server',
      );
    }
    preorderOrder = List.of(orderedIds);
    return getQueue(
      token: token,
      sessionId: sessionId,
      liveSessionId: liveSessionId,
    );
  }

  String populationPolicy = 'present_at_activation';
  String finalizationPolicy = 'mark_unfinished_skipped';
  String optOutPolicy = 'approval_required';
  String visibilityPolicy = 'managers_and_student';
  String correctionPolicy = 'audited_any_time';

  @override
  Future<QueuePolicy> updatePolicy({
    required String token,
    required String sessionId,
    required String liveSessionId,
    required int expectedVersion,
    String? population,
    String? unfinishedFinalization,
    String? optOut,
    String? gradeVisibility,
    String? gradeCorrection,
    String? idempotencyKey,
  }) async {
    if (rejectPolicy) {
      throw const QueueApiException(
        statusCode: 422,
        code: 'QUEUE_POLICY_INVALID',
        message: 'Policy rejected on server',
      );
    }
    if (expectedVersion != policyVersion) {
      throw const QueueApiException(
        statusCode: 409,
        code: 'QUEUE_POLICY_VERSION_CONFLICT',
        message: 'Policy changed on server',
      );
    }
    populationPolicy = population ?? populationPolicy;
    finalizationPolicy = unfinishedFinalization ?? finalizationPolicy;
    optOutPolicy = optOut ?? optOutPolicy;
    visibilityPolicy = gradeVisibility ?? visibilityPolicy;
    correctionPolicy = gradeCorrection ?? correctionPolicy;
    policyVersion++;
    return QueuePolicy.fromJson({
      'population': populationPolicy,
      'unfinished_finalization': finalizationPolicy,
      'opt_out': optOutPolicy,
      'grade_visibility': visibilityPolicy,
      'grade_correction': correctionPolicy,
      'version': policyVersion,
    });
  }

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
        'lifecycle': prepared ? 'prepared' : 'active',
        'surah_id': 2,
        'from_ayah': 1,
        'to_ayah': 5,
        'grading_required': false,
        'selected_entry_id': prepared ? null : 'reciting-entry',
        'version': queueVersion,
        'policy': {
          'population': populationPolicy,
          'unfinished_finalization': finalizationPolicy,
          'opt_out': optOutPolicy,
          'grade_visibility': visibilityPolicy,
          'grade_correction': correctionPolicy,
          'version': policyVersion,
        },
        'preorder': prepared
            ? [
                for (var index = 0; index < preorderOrder.length; index++)
                  {
                    'student_id': preorderOrder[index],
                    'student_name': preorderOrder[index] == 'student-1'
                        ? 'First student'
                        : 'Second student',
                    'position': index + 1,
                  },
              ]
            : const [],
        'entries': prepared
            ? const []
            : const [
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
  _DialogSessionApi({this.isModerator = true}) : super(Dio());

  final bool isModerator;

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
        isModerator: isModerator,
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
