import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/sessions/data/queue_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/presentation/queue/queue_student_panel.dart';

void main() {
  testWidgets(
      'shows the durable student position and hides manager controls (RTL + LTR)',
      (tester) async {
    final semantics = tester.ensureSemantics();
    for (final direction in TextDirection.values) {
      final labels = _StudentPanelLabels(direction == TextDirection.rtl);
      await tester.pumpWidget(_panel(direction: direction));

      expect(find.bySemanticsLabel(labels.yourPosition(2)), findsOneWidget);
      expect(find.bySemanticsLabel(labels.waiting), findsOneWidget);
      expect(find.bySemanticsLabel(labels.position(1)), findsOneWidget);
      expect(find.bySemanticsLabel(labels.reciting), findsOneWidget);
      for (final control in labels.managerControls) {
        expect(find.bySemanticsLabel(control), findsNothing);
      }
    }
    semantics.dispose();
  });

  testWidgets('renders every humane opt-out request state (RTL + LTR)',
      (tester) async {
    final semantics = tester.ensureSemantics();
    for (final direction in TextDirection.values) {
      final labels = _StudentPanelLabels(direction == TextDirection.rtl);
      final actions = <String>[];

      // Not requested: the student can ask to opt out.
      await tester.pumpWidget(_panel(
        direction: direction,
        actions: actions,
        optOutStatus: StudentOptOutStatus.notRequested,
      ));
      expect(find.bySemanticsLabel(labels.optOutAction), findsOneWidget);
      await tester.tap(find.bySemanticsLabel(labels.optOutAction));
      expect(actions, ['opt-out']);
      actions.clear();

      // Requesting: in flight, no second command.
      await tester.pumpWidget(_panel(
        direction: direction,
        actions: actions,
        optOutStatus: StudentOptOutStatus.requesting,
      ));
      expect(find.bySemanticsLabel(labels.optOutRequesting), findsOneWidget);
      await tester.tap(
        find.bySemanticsLabel(labels.optOutRequesting),
        warnIfMissed: false,
      );
      expect(actions, isEmpty);

      // Pending: awaiting the manager decision, no repeat request.
      await tester.pumpWidget(_panel(
        direction: direction,
        optOutStatus: StudentOptOutStatus.pending,
      ));
      expect(find.bySemanticsLabel(labels.optOutPending), findsOneWidget);
      expect(find.bySemanticsLabel(labels.optOutAction), findsNothing);

      // Declined: humane feedback and the entry remains waiting.
      await tester.pumpWidget(_panel(
        direction: direction,
        optOutStatus: StudentOptOutStatus.declined,
      ));
      expect(find.bySemanticsLabel(labels.optOutDeclined), findsOneWidget);
      expect(find.bySemanticsLabel(labels.yourPosition(2)), findsOneWidget);
      expect(find.bySemanticsLabel(labels.waiting), findsOneWidget);

      // Approved / auto-approved: opted out, no turn expectations left.
      for (final (status, message) in [
        (StudentOptOutStatus.approved, labels.optOutApproved),
        (StudentOptOutStatus.autoApproved, labels.optOutAutoApproved),
      ]) {
        await tester.pumpWidget(_panel(
          direction: direction,
          optOutStatus: status,
          queue: _queueState(myStatus: 'opted_out'),
          myEntry: _myEntry(status: 'opted_out'),
        ));
        expect(find.bySemanticsLabel(message), findsOneWidget);
        expect(find.bySemanticsLabel(labels.optedOut), findsOneWidget);
        expect(find.bySemanticsLabel(labels.waiting), findsNothing);
      }
    }
    semantics.dispose();
  });

  testWidgets(
      'shows the reconnecting banner then the authoritative snapshot (RTL + LTR)',
      (tester) async {
    final semantics = tester.ensureSemantics();
    for (final direction in TextDirection.values) {
      final labels = _StudentPanelLabels(direction == TextDirection.rtl);

      await tester.pumpWidget(_panel(
        direction: direction,
        status: QueueStudentPanelStatus.reconnecting,
      ));
      expect(find.text(labels.reconnecting), findsOneWidget);

      // Connectivity restored: the authoritative snapshot replaces the state.
      await tester.pumpWidget(_panel(
        direction: direction,
        queue: _queueState(myPosition: 3),
        myEntry: _myEntry(position: 3),
      ));
      expect(find.text(labels.reconnecting), findsNothing);
      expect(find.bySemanticsLabel(labels.yourPosition(3)), findsOneWidget);
      expect(find.bySemanticsLabel(labels.yourPosition(2)), findsNothing);
    }
    semantics.dispose();
  });

  testWidgets(
      'shows Arabic empty-state guidance with no opt-out action (RTL + LTR)',
      (tester) async {
    final semantics = tester.ensureSemantics();
    for (final direction in TextDirection.values) {
      final labels = _StudentPanelLabels(direction == TextDirection.rtl);
      await tester.pumpWidget(_panel(
        direction: direction,
        queue: null,
        myEntry: null,
        status: QueueStudentPanelStatus.empty,
      ));

      expect(find.text(labels.emptyGuidance), findsOneWidget);
      expect(find.bySemanticsLabel(labels.optOutAction), findsNothing);
    }
    semantics.dispose();
  });

  testWidgets(
      'meets accessibility: 48dp targets and live-region feedback (RTL + LTR)',
      (tester) async {
    final semantics = tester.ensureSemantics();
    for (final direction in TextDirection.values) {
      final labels = _StudentPanelLabels(direction == TextDirection.rtl);

      await tester.pumpWidget(_panel(direction: direction));
      final target = tester.getSize(find.bySemanticsLabel(labels.optOutAction));
      expect(target.width, greaterThanOrEqualTo(48));
      expect(target.height, greaterThanOrEqualTo(48));

      await tester.pumpWidget(_panel(
        direction: direction,
        optOutStatus: StudentOptOutStatus.pending,
      ));
      final announcement =
          tester.getSemantics(find.bySemanticsLabel(labels.optOutPending));
      expect(announcement.flagsCollection.isLiveRegion, isTrue);
    }
    semantics.dispose();
  });
}

Widget _panel({
  required TextDirection direction,
  List<String> actions = const [],
  QueueState? queue,
  QueueEntry? myEntry,
  QueueStudentPanelStatus status = QueueStudentPanelStatus.ready,
  StudentOptOutStatus optOutStatus = StudentOptOutStatus.notRequested,
}) {
  queue ??= _queueState();
  myEntry ??= _myEntry();
  return MaterialApp(
    home: Directionality(
      textDirection: direction,
      child: Scaffold(
        body: QueueStudentPanel(
          queue: queue,
          myEntry: myEntry,
          status: status,
          optOutStatus: optOutStatus,
          onRequestOptOut: () => actions.add('opt-out'),
        ),
      ),
    ),
  );
}

QueueState _queueState({String myStatus = 'waiting', int myPosition = 2}) =>
    QueueState.fromJson({
      'session_id': 'session-1',
      'round_id': 'round-1',
      'round_number': 1,
      'round_type': 'revision',
      'lifecycle': 'active',
      'surah_id': 2,
      'from_ayah': 1,
      'to_ayah': 5,
      'grading_required': false,
      'selected_entry_id': 'entry-1',
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
        _entryJson(
          'entry-1',
          studentId: 'student-1',
          name: 'مريم',
          position: 1,
          status: 'reciting',
        ),
        _entryJson(
          'entry-2',
          studentId: 'student-me',
          name: 'خالد',
          position: myPosition,
          status: myStatus,
        ),
      ],
    });

QueueEntry _myEntry({String status = 'waiting', int position = 2}) =>
    QueueEntry.fromJson(_entryJson(
      'entry-2',
      studentId: 'student-me',
      name: 'خالد',
      position: position,
      status: status,
    ));

Map<String, dynamic> _entryJson(
  String id, {
  required String studentId,
  required String name,
  required int position,
  required String status,
}) =>
    {
      'id': id,
      'student_id': studentId,
      'student_name': name,
      'position': position,
      'status': status,
      'version': 1,
    };

/// Arabic-first labels pinned for the T047 student panel; the LTR column
/// mirrors the bilingual pattern of `queue_manager_panel.dart`.
class _StudentPanelLabels {
  const _StudentPanelLabels(this.rtl);

  final bool rtl;

  String get optOutAction => rtl ? 'الاعتذار عن الدور' : 'Opt out of turn';
  String get optOutRequesting =>
      rtl ? 'جارٍ إرسال الاعتذار...' : 'Sending opt-out...';
  String get optOutPending =>
      rtl ? 'بانتظار موافقة المعلم' : 'Awaiting teacher approval';
  String get optOutDeclined =>
      rtl ? 'يبقى دورك محفوظًا لك' : 'Your turn stays saved for you';
  String get optOutApproved => rtl ? 'تم اعتماد الاعتذار' : 'Opt-out approved';
  String get optOutAutoApproved =>
      rtl ? 'تم اعتماد الاعتذار تلقائيًا' : 'Opt-out approved automatically';
  String get waiting => rtl ? 'بانتظار الدور' : 'Waiting';
  String get reciting => rtl ? 'يتلو الآن' : 'Reciting';
  String get optedOut => rtl ? 'معتذر' : 'Opted out';
  String get reconnecting =>
      rtl ? 'جارٍ إعادة الاتصال بقائمة التلاوة...' : 'Reconnecting to queue...';
  String get emptyGuidance => rtl
      ? 'لم تبدأ جولة التلاوة بعد؛ سيظهر دورك هنا'
      : 'No recitation round yet; your turn will appear here';

  String yourPosition(int position) =>
      rtl ? 'موضعك: $position' : 'Your position: $position';
  String position(int position) =>
      rtl ? 'الموضع $position' : 'Position $position';

  List<String> get managerControls => rtl
      ? const [
          'إعداد الجولة',
          'إعادة ترتيب الدور',
          'نقل الطالب',
          'اختيار التالي',
          'بدء التلاوة',
          'تخطي الدور',
          'إعادة تعيين الجولة',
          'سياسة القائمة',
        ]
      : const [
          'Prepare round',
          'Reorder queue',
          'Move student',
          'Select next',
          'Start recitation',
          'Skip turn',
          'Reset round',
          'Queue policy',
        ];
}
