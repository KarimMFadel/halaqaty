import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/sessions/data/queue_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/presentation/queue/queue_manager_panel.dart';

void main() {
  testWidgets(
      'shows grade and routes completion/correction to the selected entry',
      (tester) async {
    final actions = <String>[];
    await tester.pumpWidget(
      MaterialApp(
        home: QueueManagerPanel(
          queue: _queue(),
          status: QueueManagerPanelStatus.ready,
          onPrepare: () {},
          onReorder: () {},
          onMove: () {},
          onAdvance: () {},
          onStart: () {},
          onSkip: () {},
          onComplete: () => actions.add('complete'),
          onCorrect: actions.add,
          onReset: () {},
          onEditPolicy: () {},
        ),
      ),
    );

    expect(find.bySemanticsLabel('Grade excellent'), findsOneWidget);
    await tester.tap(find.byTooltip('Correct grade'));
    expect(actions, ['entry-2']);
    await tester.tap(find.bySemanticsLabel('Complete turn'));
    expect(actions, ['entry-2', 'complete']);
  });

  testWidgets('disables grading controls for a finalized round',
      (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        home: QueueManagerPanel(
          queue: _queue(),
          status: QueueManagerPanelStatus.terminal,
          onPrepare: () {},
          onReorder: () {},
          onMove: () {},
          onAdvance: () {},
          onStart: () {},
          onSkip: () {},
          onComplete: () {},
          onCorrect: (_) {},
          onReset: () {},
          onEditPolicy: () {},
        ),
      ),
    );

    await tester.tap(find.bySemanticsLabel('Complete turn'));
    expect(find.byIcon(Icons.edit_outlined), findsOneWidget);
  });
}

QueueState _queue() => QueueState.fromJson({
      'session_id': 'session-1',
      'round_id': 'round-1',
      'round_number': 1,
      'round_type': 'revision',
      'lifecycle': 'active',
      'surah_id': 2,
      'from_ayah': 1,
      'to_ayah': 5,
      'grading_required': true,
      'selected_entry_id': 'entry-1',
      'version': 2,
      'policy': {
        'population': 'present_at_activation',
        'unfinished_finalization': 'mark_unfinished_skipped',
        'opt_out': 'approval_required',
        'grade_visibility': 'managers_and_student',
        'grade_correction': 'audited_any_time',
        'version': 1,
      },
      'preorder': const [],
      'entries': [
        {
          'id': 'entry-1',
          'student_id': 'student-1',
          'student_name': 'Mary',
          'position': 1,
          'status': 'reciting',
          'version': 1,
        },
        {
          'id': 'entry-2',
          'student_id': 'student-2',
          'student_name': 'Ahmed',
          'position': 2,
          'status': 'completed',
          'grade': 'excellent',
          'grade_notes': 'Clear',
          'version': 2,
        },
      ],
    });
