import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/sessions/data/queue_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/presentation/queue/queue_manager_panel.dart';

void main() {
  testWidgets(
      'RTL manager panel exposes Arabic queue controls, status, and turn announcement',
      (tester) async {
    final semantics = tester.ensureSemantics();
    final actions = <String>[];
    await tester.pumpWidget(_panel(
      direction: TextDirection.rtl,
      queue: _queueState(entryStatus: 'reciting'),
      actions: actions,
    ));

    for (final label in [
      'إعداد الجولة',
      'إعادة ترتيب الدور',
      'نقل الطالب',
      'اختيار التالي',
      'بدء التلاوة',
      'تخطي الدور',
      'سياسة القائمة',
    ]) {
      expect(find.bySemanticsLabel(label), findsOneWidget);
    }
    expect(find.bySemanticsLabel('الموضع 1'), findsOneWidget);
    expect(find.bySemanticsLabel('يتلو الآن'), findsOneWidget);
    expect(find.bySemanticsLabel('دور التلاوة الحالي: مريم'), findsOneWidget);

    final announcement =
        tester.getSemantics(find.bySemanticsLabel('دور التلاوة الحالي: مريم'));
    expect(
      announcement.flagsCollection.isLiveRegion,
      isTrue,
    );

    for (final label in ['إعداد الجولة', 'بدء التلاوة', 'تخطي الدور']) {
      final size = tester.getSize(find.bySemanticsLabel(label));
      expect(size.width, greaterThanOrEqualTo(48));
      expect(size.height, greaterThanOrEqualTo(48));
    }

    await tester.tap(find.bySemanticsLabel('إعداد الجولة'));
    await tester.tap(find.bySemanticsLabel('إعادة ترتيب الدور'));
    await tester.tap(find.bySemanticsLabel('نقل الطالب'));
    await tester.tap(find.bySemanticsLabel('اختيار التالي'));
    await tester.tap(find.bySemanticsLabel('بدء التلاوة'));
    await tester.tap(find.bySemanticsLabel('تخطي الدور'));
    await tester.tap(find.bySemanticsLabel('سياسة القائمة'));

    expect(actions, [
      'prepare',
      'reorder',
      'move',
      'advance',
      'start',
      'skip',
      'policy',
    ]);
    semantics.dispose();
  });

  testWidgets('LTR manager panel retains the same queue controls',
      (tester) async {
    final semantics = tester.ensureSemantics();
    final actions = <String>[];
    await tester.pumpWidget(_panel(
      direction: TextDirection.ltr,
      queue: _queueState(entryStatus: 'waiting'),
      actions: actions,
    ));

    for (final label in [
      'Prepare round',
      'Reorder queue',
      'Move student',
      'Select next',
      'Start recitation',
      'Skip turn',
      'Queue policy',
    ]) {
      expect(find.bySemanticsLabel(label), findsOneWidget);
    }
    expect(find.bySemanticsLabel('Position 1'), findsOneWidget);
    expect(find.bySemanticsLabel('Waiting'), findsOneWidget);

    await tester.tap(find.bySemanticsLabel('Select next'));
    await tester.tap(find.bySemanticsLabel('Start recitation'));
    expect(actions, ['advance', 'start']);
    semantics.dispose();
  });

  testWidgets(
      'renders loading, empty, reconnecting, recoverable, and terminal states',
      (tester) async {
    final semantics = tester.ensureSemantics();
    final cases = [
      (
        status: QueueManagerPanelStatus.loading,
        queue: null,
        message: 'جارٍ تحميل قائمة التلاوة...',
      ),
      (
        status: QueueManagerPanelStatus.empty,
        queue: null,
        message: 'لا توجد جولة تلاوة حالية',
      ),
      (
        status: QueueManagerPanelStatus.reconnecting,
        queue: _queueState(entryStatus: 'waiting'),
        message: 'جارٍ إعادة الاتصال بقائمة التلاوة...',
      ),
      (
        status: QueueManagerPanelStatus.recoverableError,
        queue: _queueState(entryStatus: 'waiting'),
        message: 'تعذر تحديث قائمة التلاوة',
      ),
      (
        status: QueueManagerPanelStatus.terminal,
        queue: _queueState(entryStatus: 'waiting'),
        message: 'انتهت جولة التلاوة',
      ),
    ];

    for (final testCase in cases) {
      await tester.pumpWidget(_panel(
        direction: TextDirection.rtl,
        queue: testCase.queue,
        status: testCase.status,
        actions: <String>[],
      ));

      expect(find.text(testCase.message), findsOneWidget);
    }

    await tester.pumpWidget(_panel(
      direction: TextDirection.rtl,
      queue: _queueState(entryStatus: 'waiting'),
      status: QueueManagerPanelStatus.recoverableError,
      actions: <String>[],
    ));
    expect(find.bySemanticsLabel('تخطي الدور'), findsOneWidget);
    expect(find.bySemanticsLabel('سياسة القائمة'), findsOneWidget);
    semantics.dispose();
  });
}

Widget _panel({
  required TextDirection direction,
  required QueueState? queue,
  required List<String> actions,
  QueueManagerPanelStatus status = QueueManagerPanelStatus.ready,
}) =>
    MaterialApp(
      home: Directionality(
        textDirection: direction,
        child: Scaffold(
          body: QueueManagerPanel(
            queue: queue,
            status: status,
            onPrepare: () => actions.add('prepare'),
            onReorder: () => actions.add('reorder'),
            onMove: () => actions.add('move'),
            onAdvance: () => actions.add('advance'),
            onStart: () => actions.add('start'),
            onSkip: () => actions.add('skip'),
            onEditPolicy: () => actions.add('policy'),
          ),
        ),
      ),
    );

QueueState _queueState({required String entryStatus}) => QueueState.fromJson({
      'session_id': 'session-1',
      'round_id': 'round-1',
      'round_number': 1,
      'round_type': 'revision',
      'lifecycle': 'active',
      'surah_id': 2,
      'from_ayah': 1,
      'to_ayah': 5,
      'grading_required': false,
      'selected_entry_id': entryStatus == 'reciting' ? 'entry-1' : null,
      'version': 1,
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
          'student_name': 'مريم',
          'position': 1,
          'status': entryStatus,
          'version': 1,
        },
      ],
    });
