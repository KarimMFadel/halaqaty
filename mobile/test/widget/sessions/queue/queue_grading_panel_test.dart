import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/sessions/data/queue_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/presentation/queue/queue_grading_panel.dart';

void main() {
  testWidgets('shows Arabic grading controls and submits a grade with a note',
      (tester) async {
    final calls = <String>[];
    final semantics = tester.ensureSemantics();
    await tester.pumpWidget(_panel(
      direction: TextDirection.rtl,
      onComplete: (grade, note) => calls.add('$grade:$note'),
      onCorrect: (grade, note, clear) {},
    ));

    expect(find.bySemanticsLabel('تقييم التلاوة'), findsOneWidget);
    expect(find.bySemanticsLabel('ممتاز'), findsOneWidget);
    expect(find.bySemanticsLabel('ملاحظات المعلم'), findsOneWidget);
    await tester.tap(find.bySemanticsLabel('جيد'));
    await tester.enterText(find.bySemanticsLabel('ملاحظات المعلم'), 'أحسنت');
    await tester.tap(find.bySemanticsLabel('حفظ التقييم'));

    expect(calls, ['good:أحسنت']);
    semantics.dispose();
  });

  testWidgets(
      'shows LTR correction controls and preserves the terminal read-only state',
      (tester) async {
    final corrections = <String>[];
    await tester.pumpWidget(_panel(
      direction: TextDirection.ltr,
      lifecycle: 'active',
      entryStatus: 'completed',
      onComplete: (_, __) {},
      onCorrect: (grade, note, clear) => corrections.add('$grade:$note:$clear'),
    ));

    expect(find.bySemanticsLabel('Correct grade or note'), findsOneWidget);
    expect(find.text('Current grade: Good'), findsOneWidget);
    await tester.tap(find.bySemanticsLabel('Excellent'));
    await tester.tap(find.bySemanticsLabel('Save correction'));
    expect(corrections, ['excellent:null:false']);

    await tester.pumpWidget(_panel(
      direction: TextDirection.ltr,
      lifecycle: 'finalized',
      entryStatus: 'completed',
      onComplete: (_, __) {},
      onCorrect: (_, __, ___) {},
    ));
    expect(find.text('Round finalized; grading is read-only'), findsOneWidget);
    expect(find.bySemanticsLabel('Save correction'), findsNothing);
  });

  testWidgets('correction allows a note without a grade', (tester) async {
    final corrections = <String>[];
    await tester.pumpWidget(_panel(
      direction: TextDirection.ltr,
      entryStatus: 'completed',
      entryGrade: null,
      gradingRequired: false,
      onComplete: (_, __) {},
      onCorrect: (grade, note, clear) => corrections.add('$grade:$note:$clear'),
    ));

    await tester.enterText(
        find.bySemanticsLabel('Teacher notes'), 'Needs another listen');
    await tester.pump();
    await tester.tap(find.bySemanticsLabel('Save correction'));

    expect(corrections, ['null:Needs another listen:false']);
  });

  testWidgets('correction sends explicit note clearing', (tester) async {
    final corrections = <String>[];
    await tester.pumpWidget(_panel(
      direction: TextDirection.ltr,
      entryStatus: 'completed',
      entryNotes: 'Old note',
      onComplete: (_, __) {},
      onCorrect: (grade, note, clear) => corrections.add('$grade:$note:$clear'),
    ));

    await tester.enterText(find.bySemanticsLabel('Teacher notes'), '');
    await tester.pump();
    await tester.tap(find.bySemanticsLabel('Save correction'));

    expect(corrections, ['good:null:true']);
  });

  testWidgets('caps the teacher note at 500 characters', (tester) async {
    final semantics = tester.ensureSemantics();
    await tester.pumpWidget(_panel(
      direction: TextDirection.rtl,
      onComplete: (_, __) {},
      onCorrect: (_, __, ___) {},
    ));

    await tester.enterText(find.bySemanticsLabel('ملاحظات المعلم'), 'م' * 501);
    await tester.pump();

    final notes =
        tester.widget<EditableText>(find.byType(EditableText)).controller;
    expect(notes.text.length, 500);
    expect(find.text('500/500'), findsOneWidget);
    semantics.dispose();
  });

  testWidgets(
      'renders only the grade fields the payload carries '
      '(visibility-filtered)', (tester) async {
    final semantics = tester.ensureSemantics();
    // Redacted payload (e.g. managers_only projection of another student):
    // correction controls render with no current-grade or note line.
    await tester.pumpWidget(_panel(
      direction: TextDirection.ltr,
      entryStatus: 'completed',
      entryGrade: null,
      entryNotes: null,
      onComplete: (_, __) {},
      onCorrect: (_, __, ___) {},
    ));

    expect(find.bySemanticsLabel('Correct grade or note'), findsOneWidget);
    expect(find.textContaining('Current grade'), findsNothing);
    expect(find.textContaining('Current teacher note'), findsNothing);

    // A full payload shows the current grade and note (Arabic rendering).
    await tester.pumpWidget(_panel(
      direction: TextDirection.rtl,
      entryStatus: 'completed',
      entryGrade: 'good',
      entryNotes: 'أحسنت',
      onComplete: (_, __) {},
      onCorrect: (_, __, ___) {},
    ));

    expect(find.text('التقييم الحالي: جيد'), findsOneWidget);
    expect(find.text('ملاحظة المعلم الحالية: أحسنت'), findsOneWidget);
    semantics.dispose();
  });
}

Widget _panel({
  required TextDirection direction,
  required void Function(String? grade, String? notes) onComplete,
  required void Function(String? grade, String? notes, bool clearNotes)
      onCorrect,
  String lifecycle = 'active',
  String entryStatus = 'reciting',
  String? entryGrade = 'good',
  String? entryNotes,
  bool gradingRequired = true,
}) =>
    MaterialApp(
      home: Directionality(
        textDirection: direction,
        child: Scaffold(
          body: QueueGradingPanel(
            entry: QueueEntry.fromJson({
              'id': 'entry-1',
              'student_id': 'student-1',
              'student_name': 'Maryam',
              'position': 1,
              'status': entryStatus,
              'grade': entryStatus == 'completed' ? entryGrade : null,
              'grade_notes': entryStatus == 'completed' ? entryNotes : null,
              'version': 2,
            }),
            gradingRequired: gradingRequired,
            lifecycle: lifecycle,
            onComplete: onComplete,
            onCorrect: onCorrect,
          ),
        ),
      ),
    );
