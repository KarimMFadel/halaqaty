import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/core/design/halaqaty_components.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_detail_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_detail_screen.dart';
import 'package:halaqaty_mobile/features/scheduling/application/schedule_controller.dart';
import 'package:halaqaty_mobile/features/scheduling/data/schedule_api_client.dart';
import 'package:halaqaty_mobile/features/scheduling/presentation/schedule_editor_screen.dart';
import 'package:halaqaty_mobile/features/sessions/application/circle_sessions_controller.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_api_client.dart';

import '../../helpers/stub_auth_notifier.dart';

class _StubScheduleController extends StateNotifier<ScheduleEditorState>
    implements ScheduleController {
  _StubScheduleController({
    ScheduleEditorState? initialState,
    bool manage = true,
  })  : _manage = manage,
        super(initialState ?? const ScheduleEditorState());

  final bool _manage;

  bool hangLoad = false;
  bool saveSucceeds = true;
  int loadCalls = 0;
  SchedulePlanInput? createdPlan;
  CircleScheduleEntry? changedSchedule;
  SchedulePlanInput? changedPlan;
  CircleScheduleEntry? stoppedSchedule;
  String? occurrenceDate;
  bool? occurrenceCancel;

  @override
  String get circleId => 'circle-1';

  @override
  bool get canManage => _manage;

  @override
  Future<void> load() async {
    loadCalls++;
    if (hangLoad) {
      state = state.copyWith(status: ScheduleListStatus.loading);
      return;
    }
  }

  @override
  Future<bool> createSchedule(SchedulePlanInput plan) async {
    createdPlan = plan;
    return _finishSave();
  }

  @override
  Future<bool> changeSeries({
    required CircleScheduleEntry schedule,
    required SchedulePlanInput plan,
  }) async {
    changedSchedule = schedule;
    changedPlan = plan;
    return _finishSave();
  }

  @override
  Future<bool> stopSeries({required CircleScheduleEntry schedule}) async {
    stoppedSchedule = schedule;
    return _finishSave();
  }

  @override
  Future<bool> changeOccurrence({
    required CircleScheduleEntry schedule,
    required String localDate,
    String? replacementLocalDate,
    String? replacementLocalTime,
    String? replacementEndLocalTime,
    int? durationMinutes,
    bool cancel = false,
  }) async {
    occurrenceDate = localDate;
    occurrenceCancel = cancel;
    return _finishSave();
  }

  Future<bool> _finishSave() async {
    if (!saveSucceeds) {
      state = state.copyWith(
        isSaving: false,
        saveError: ScheduleSaveError.network,
      );
      return false;
    }
    state = state.copyWith(isSaving: false, saveSucceeded: true);
    return true;
  }
}

/// Minimal sessions-section stub so circle detail tests need no Firebase.
class _SpyCircleSessionsController extends StateNotifier<CircleSessionsState>
    implements CircleSessionsController {
  _SpyCircleSessionsController()
      : super(const CircleSessionsState(status: CircleSessionsStatus.ready));

  @override
  String get circleId => 'circle-1';

  @override
  Future<void> load() async {}

  @override
  Future<SessionModel?> create() async => null;
}

const _scope = (circleId: 'circle-1', canManage: true);
const _viewerScope = (circleId: 'circle-1', canManage: false);

Widget _buildScreen(
  _StubScheduleController stub, {
  bool rtl = false,
  bool canManage = true,
  bool isArchived = false,
}) {
  return ProviderScope(
    overrides: [
      scheduleControllerProvider(canManage ? _scope : _viewerScope)
          .overrideWith((_) => stub),
    ],
    child: MaterialApp(
      home: Directionality(
        textDirection: rtl ? TextDirection.rtl : TextDirection.ltr,
        child: ScheduleEditorScreen(
          circleId: 'circle-1',
          circleName: 'Tajweed Circle',
          canManage: canManage,
          isArchived: isArchived,
        ),
      ),
    ),
  );
}

CircleScheduleEntry _entry({
  String id = 'sched-1',
  int version = 3,
  String? title,
  String? stoppedFrom,
}) =>
    CircleScheduleEntry.fromJson({
      'id': id,
      'circle_id': 'circle-1',
      'version': version,
      'stopped_from_local_date': stoppedFrom,
      'plan': {
        'mode': 'weekday_pattern',
        'title': title,
        'anchor_local_date': '2026-10-06',
        'local_start_time': '18:00',
        'local_end_time': '19:30',
        'duration_minutes': 90,
        'timezone': 'Africa/Cairo',
        'week_cadence': 2,
        'weekdays': [1, 3],
      },
    });

ScheduleEditorState _readyWith(List<CircleScheduleEntry> entries) =>
    ScheduleEditorState(status: ScheduleListStatus.ready, schedules: entries);

TextField _field(WidgetTester tester, Key key) => tester.widget<TextField>(
      find.descendant(of: find.byKey(key), matching: find.byType(TextField)),
    );

/// Opens the create form and pumps it to its final size.
Future<void> _openCreateForm(WidgetTester tester) async {
  // The form sheet is taller than the default 600px test surface; a tall
  // viewport keeps every field built and tappable without scroll timing.
  tester.view.physicalSize = const Size(800, 1600);
  tester.view.devicePixelRatio = 1.0;
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(tester.view.resetDevicePixelRatio);

  await tester.tap(find.byKey(const Key('scheduleAddButton')));
  await tester.pumpAndSettle();
}

/// Taps the weekday chip matching tomorrow (the default anchor date).
Future<void> _selectAnchorWeekday(WidgetTester tester) async {
  final tomorrow = DateTime.now().add(const Duration(days: 1));
  final dow = tomorrow.weekday % DateTime.daysPerWeek;
  final chip = find.byKey(Key('scheduleWeekday-$dow'));
  await tester.ensureVisible(chip);
  await tester.tap(chip);
  await tester.pump();
}

void main() {
  group('ScheduleEditorScreen states', () {
    testWidgets('shows branded loading while the list loads', (tester) async {
      final stub = _StubScheduleController(
        initialState: const ScheduleEditorState(),
      );
      await tester.pumpWidget(_buildScreen(stub));
      await tester.pump();

      expect(find.byType(HalaqatyLoading), findsOneWidget);
    });

    testWidgets('empty list shows the empty state with a manager hint',
        (tester) async {
      final stub = _StubScheduleController(initialState: _readyWith(const []));
      await tester.pumpWidget(_buildScreen(stub));
      await tester.pump();

      expect(find.byKey(const Key('schedulesEmpty')), findsOneWidget);
      expect(find.text('No schedule entries yet'), findsOneWidget);
      expect(
        find.text('Add the first recurring time for this circle'),
        findsOneWidget,
      );
    });

    testWidgets('load failure explains the error and retries', (tester) async {
      final stub = _StubScheduleController(
        initialState: const ScheduleEditorState(
          status: ScheduleListStatus.error,
          failure: ScheduleFailure.unknown,
        ),
      );
      await tester.pumpWidget(_buildScreen(stub));
      await tester.pump();

      expect(find.text('Could not load the schedule'), findsOneWidget);
      expect(stub.loadCalls, 1);

      await tester.tap(find.byKey(const Key('scheduleRetryButton')));
      await tester.pump();

      expect(stub.loadCalls, 2);
    });

    testWidgets('a permission failure has no retry', (tester) async {
      final stub = _StubScheduleController(
        initialState: const ScheduleEditorState(
          status: ScheduleListStatus.error,
          failure: ScheduleFailure.permission,
        ),
      );
      await tester.pumpWidget(_buildScreen(stub));
      await tester.pump();

      expect(
        find.text('You do not have access to this schedule'),
        findsOneWidget,
      );
      expect(find.byKey(const Key('scheduleRetryButton')), findsNothing);
    });

    testWidgets(
        'offline keeps the last list with a stale label and pauses '
        'mutations', (tester) async {
      final stub = _StubScheduleController(
        initialState: ScheduleEditorState(
          status: ScheduleListStatus.error,
          failure: ScheduleFailure.network,
          schedules: [_entry()],
        ),
      );
      await tester.pumpWidget(_buildScreen(stub));
      await tester.pump();

      expect(find.byKey(const Key('scheduleStaleBanner')), findsOneWidget);
      expect(
        find.text('Offline — showing the last loaded schedule'),
        findsOneWidget,
      );
      expect(find.byKey(const Key('scheduleCard-sched-1')), findsOneWidget);
      expect(find.byKey(const Key('scheduleAddButton')), findsNothing);
      expect(find.byKey(const Key('scheduleEdit-sched-1')), findsNothing);
      expect(find.byKey(const Key('scheduleRetryButton')), findsOneWidget);
    });

    testWidgets(
        'a loaded entry shows title, times, zone, circle identity and '
        'a non-color cue', (tester) async {
      final stub = _StubScheduleController(
        initialState: _readyWith([
          _entry(title: 'Tajweed review'),
          _entry(id: 'sched-2', stoppedFrom: '2026-10-07'),
        ]),
      );
      await tester.pumpWidget(_buildScreen(stub));
      await tester.pump();

      expect(find.text('Tajweed review'), findsOneWidget);
      expect(find.text('18:00–19:30 · Africa/Cairo'), findsNWidgets(2));
      // Circle identity by text plus an icon cue, never color alone.
      expect(find.text('Tajweed Circle'), findsNWidgets(2));
      expect(find.byIcon(Icons.event_repeat), findsNWidgets(2));
      expect(find.text('Stopped from 2026-10-07'), findsOneWidget);
      // Untitled entries fall back to the contract default title.
      expect(find.text('Circle Session'), findsOneWidget);
    });
  });

  group('ScheduleEditorScreen RTL/LTR', () {
    testWidgets('renders English copy in LTR', (tester) async {
      final stub = _StubScheduleController(initialState: _readyWith(const []));
      await tester.pumpWidget(_buildScreen(stub));
      await tester.pump();

      expect(
        Directionality.of(tester.element(find.byType(ScheduleEditorScreen))),
        TextDirection.ltr,
      );
      expect(find.text('Circle schedule'), findsOneWidget);
      expect(find.text('Add schedule'), findsOneWidget);
    });

    testWidgets('renders Arabic copy in RTL', (tester) async {
      final stub = _StubScheduleController(initialState: _readyWith(const []));
      await tester.pumpWidget(_buildScreen(stub, rtl: true));
      await tester.pump();

      expect(
        Directionality.of(tester.element(find.byType(ScheduleEditorScreen))),
        TextDirection.rtl,
      );
      expect(find.text('مواعيد الحلقة'), findsOneWidget);
      expect(find.text('إضافة موعد'), findsOneWidget);
    });
  });

  group('ScheduleEditorScreen rights', () {
    testWidgets('a viewer sees the list without any mutation affordance',
        (tester) async {
      final stub = _StubScheduleController(
        initialState: _readyWith([_entry()]),
        manage: false,
      );
      await tester.pumpWidget(_buildScreen(stub, canManage: false));
      await tester.pump();

      expect(find.byKey(const Key('scheduleCard-sched-1')), findsOneWidget);
      expect(find.byKey(const Key('scheduleAddButton')), findsNothing);
      expect(find.byKey(const Key('scheduleEdit-sched-1')), findsNothing);
      expect(find.byKey(const Key('scheduleStop-sched-1')), findsNothing);
      expect(find.byKey(const Key('scheduleOccurrence-sched-1')), findsNothing);
    });

    testWidgets('an archived circle is read-only even for managers',
        (tester) async {
      final stub = _StubScheduleController(
        initialState: _readyWith([_entry()]),
      );
      await tester.pumpWidget(_buildScreen(stub, isArchived: true));
      await tester.pump();

      expect(find.byKey(const Key('scheduleCard-sched-1')), findsOneWidget);
      expect(find.byKey(const Key('scheduleAddButton')), findsNothing);
      expect(find.byKey(const Key('scheduleEdit-sched-1')), findsNothing);
    });
  });

  group('ScheduleEditorScreen create form', () {
    testWidgets('weekday mode requires at least one selected day',
        (tester) async {
      final stub = _StubScheduleController(initialState: _readyWith(const []));
      await tester.pumpWidget(_buildScreen(stub));
      await tester.pump();

      await _openCreateForm(tester);
      await tester.ensureVisible(find.byKey(const Key('scheduleSubmitButton')));
      await tester.tap(find.byKey(const Key('scheduleSubmitButton')));
      await tester.pump();

      expect(find.text('Select at least one day'), findsOneWidget);
      expect(stub.createdPlan, isNull);
    });

    testWidgets(
        'creating a weekly entry sends the selected weekday payload '
        'and shows a retained success', (tester) async {
      final stub = _StubScheduleController(initialState: _readyWith(const []));
      await tester.pumpWidget(_buildScreen(stub));
      await tester.pump();

      await _openCreateForm(tester);
      await _selectAnchorWeekday(tester);
      await tester.ensureVisible(find.byKey(const Key('scheduleSubmitButton')));
      await tester.tap(find.byKey(const Key('scheduleSubmitButton')));
      await tester.pumpAndSettle();

      final plan = stub.createdPlan;
      expect(plan, isNotNull);
      expect(plan!.mode, ScheduleModes.weekdayPattern);
      expect(plan.weekCadence, 1);
      final tomorrow = DateTime.now().add(const Duration(days: 1));
      expect(plan.weekdays, [tomorrow.weekday % DateTime.daysPerWeek]);
      expect(plan.localStartTime, '18:00');
      expect(plan.localEndTime, '19:00');
      expect(plan.durationMinutes, 60);
      expect(plan.timezone, 'UTC');
      expect(plan.title, isNull);
      expect(find.byKey(const Key('scheduleSaveSuccess')), findsOneWidget);
      expect(find.text('Schedule saved'), findsOneWidget);
    });

    testWidgets('an optional title is included only when entered',
        (tester) async {
      final stub = _StubScheduleController(initialState: _readyWith(const []));
      await tester.pumpWidget(_buildScreen(stub));
      await tester.pump();

      await _openCreateForm(tester);
      await tester.enterText(
        find.byKey(const Key('scheduleTitleField')),
        'Tafsir night',
      );
      await _selectAnchorWeekday(tester);
      await tester.ensureVisible(find.byKey(const Key('scheduleSubmitButton')));
      await tester.tap(find.byKey(const Key('scheduleSubmitButton')));
      await tester.pumpAndSettle();

      expect(stub.createdPlan?.title, 'Tafsir night');
    });

    testWidgets('interval mode validates the count and sends day intervals',
        (tester) async {
      final stub = _StubScheduleController(initialState: _readyWith(const []));
      await tester.pumpWidget(_buildScreen(stub));
      await tester.pump();

      await _openCreateForm(tester);
      await tester.tap(find.text('Interval'));
      await tester.pump();
      expect(
          find.byKey(const Key('scheduleIntervalCountField')), findsOneWidget);

      await tester.enterText(
        find.byKey(const Key('scheduleIntervalCountField')),
        '0',
      );
      await tester.ensureVisible(find.byKey(const Key('scheduleSubmitButton')));
      await tester.tap(find.byKey(const Key('scheduleSubmitButton')));
      await tester.pump();

      expect(find.text('Enter a positive whole number'), findsOneWidget);
      expect(stub.createdPlan, isNull);

      await tester.enterText(
        find.byKey(const Key('scheduleIntervalCountField')),
        '3',
      );
      await tester.ensureVisible(find.byKey(const Key('scheduleSubmitButton')));
      await tester.tap(find.byKey(const Key('scheduleSubmitButton')));
      await tester.pumpAndSettle();

      expect(stub.createdPlan?.mode, ScheduleModes.interval);
      expect(stub.createdPlan?.intervalCount, 3);
      expect(stub.createdPlan?.intervalUnit, 'day');
      expect(stub.createdPlan?.weekdays, isNull);
    });

    testWidgets(
        'selected-dates mode requires at least one date and collects '
        'picked dates', (tester) async {
      final stub = _StubScheduleController(initialState: _readyWith(const []));
      await tester.pumpWidget(_buildScreen(stub));
      await tester.pump();

      await _openCreateForm(tester);
      await tester.tap(find.text('Selected dates'));
      await tester.pump();

      await tester.ensureVisible(find.byKey(const Key('scheduleSubmitButton')));
      await tester.tap(find.byKey(const Key('scheduleSubmitButton')));
      await tester.pump();
      expect(find.text('Add at least one date'), findsOneWidget);
      expect(stub.createdPlan, isNull);

      await tester
          .ensureVisible(find.byKey(const Key('scheduleAddDateButton')));
      await tester.tap(find.byKey(const Key('scheduleAddDateButton')));
      await tester.pumpAndSettle();
      await tester.tap(find.text('OK'));
      await tester.pumpAndSettle();

      await tester.ensureVisible(find.byKey(const Key('scheduleSubmitButton')));
      await tester.tap(find.byKey(const Key('scheduleSubmitButton')));
      await tester.pumpAndSettle();

      final plan = stub.createdPlan;
      expect(plan?.mode, ScheduleModes.selectedDates);
      expect(plan?.selectedDates, hasLength(1));
      expect(plan?.anchorLocalDate, plan?.selectedDates?.single);
    });

    testWidgets('an invalid timezone name is rejected with guidance',
        (tester) async {
      final stub = _StubScheduleController(initialState: _readyWith(const []));
      await tester.pumpWidget(_buildScreen(stub));
      await tester.pump();

      await _openCreateForm(tester);
      await tester.enterText(
        find.byKey(const Key('scheduleTimezoneField')),
        'nowhere',
      );
      await _selectAnchorWeekday(tester);
      await tester.ensureVisible(find.byKey(const Key('scheduleSubmitButton')));
      await tester.tap(find.byKey(const Key('scheduleSubmitButton')));
      await tester.pump();

      expect(
        find.text('Enter a valid time zone, e.g. Africa/Cairo'),
        findsOneWidget,
      );
      expect(stub.createdPlan, isNull);
    });

    testWidgets('the end time must match start plus the entered duration',
        (tester) async {
      final stub = _StubScheduleController(initialState: _readyWith(const []));
      await tester.pumpWidget(_buildScreen(stub));
      await tester.pump();

      await _openCreateForm(tester);
      await _selectAnchorWeekday(tester);
      await tester.enterText(
        find.byKey(const Key('scheduleDurationField')),
        '45',
      );
      await tester.ensureVisible(find.byKey(const Key('scheduleSubmitButton')));
      await tester.tap(find.byKey(const Key('scheduleSubmitButton')));
      await tester.pump();

      expect(
        find.text('End time must match start time plus duration'),
        findsOneWidget,
      );
      expect(stub.createdPlan, isNull);
    });

    testWidgets('a failed save keeps the sheet open with an error copy',
        (tester) async {
      final stub = _StubScheduleController(initialState: _readyWith(const []))
        ..saveSucceeds = false;
      await tester.pumpWidget(_buildScreen(stub));
      await tester.pump();

      await _openCreateForm(tester);
      await _selectAnchorWeekday(tester);
      await tester.ensureVisible(find.byKey(const Key('scheduleSubmitButton')));
      await tester.tap(find.byKey(const Key('scheduleSubmitButton')));
      await tester.pump();

      expect(
        find.text('No connection. Your change was not saved.'),
        findsOneWidget,
      );
      expect(find.byKey(const Key('scheduleSubmitButton')), findsOneWidget);
    });
  });

  group('ScheduleEditorScreen edit, stop and occurrence flows', () {
    testWidgets('title-only edit preserves an interval anchor', (tester) async {
      final entry = CircleScheduleEntry.fromJson({
        'id': 'sched-1',
        'circle_id': 'circle-1',
        'version': 1,
        'plan': {
          'mode': 'interval',
          'anchor_local_date': '2090-10-06',
          'local_start_time': '18:00',
          'local_end_time': '19:00',
          'duration_minutes': 60,
          'timezone': 'UTC',
          'interval_count': 3,
          'interval_unit': 'week',
        },
      });
      final stub = _StubScheduleController(initialState: _readyWith([entry]));
      await tester.pumpWidget(_buildScreen(stub));
      await tester.pump();
      await tester.tap(find.byKey(const Key('scheduleEdit-sched-1')));
      await tester.pumpAndSettle();
      await tester.enterText(
          find.byKey(const Key('scheduleTitleField')), 'New title');
      await tester.ensureVisible(find.byKey(const Key('scheduleSubmitButton')));
      await tester.tap(find.byKey(const Key('scheduleSubmitButton')));
      await tester.pumpAndSettle();
      expect(stub.changedPlan?.anchorLocalDate, '2090-10-06');
      expect(stub.changedPlan?.intervalCount, 3);
    });

    testWidgets('failed stop displays a safe error on the list',
        (tester) async {
      final stub = _StubScheduleController(initialState: _readyWith([_entry()]))
        ..saveSucceeds = false;
      await tester.pumpWidget(_buildScreen(stub));
      await tester.pump();
      await tester.tap(find.byKey(const Key('scheduleStop-sched-1')));
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const Key('scheduleStopConfirm')));
      await tester.pumpAndSettle();
      expect(find.text('No connection. Your change was not saved.'),
          findsOneWidget);
    });

    testWidgets('edit opens a prefilled form and submits a series change',
        (tester) async {
      final stub = _StubScheduleController(
        initialState: _readyWith([_entry(title: 'Tajweed review')]),
      );
      await tester.pumpWidget(_buildScreen(stub));
      await tester.pump();

      await tester.tap(find.byKey(const Key('scheduleEdit-sched-1')));
      await tester.pumpAndSettle();

      expect(
        _field(tester, const Key('scheduleTitleField')).controller?.text,
        'Tajweed review',
      );
      expect(
        _field(tester, const Key('scheduleTimezoneField')).controller?.text,
        'Africa/Cairo',
      );

      await _selectAnchorWeekday(tester);
      await tester.ensureVisible(find.byKey(const Key('scheduleSubmitButton')));
      await tester.tap(find.byKey(const Key('scheduleSubmitButton')));
      await tester.pumpAndSettle();

      expect(stub.changedSchedule?.id, 'sched-1');
      expect(stub.changedPlan?.mode, ScheduleModes.weekdayPattern);
      expect(find.byKey(const Key('scheduleSaveSuccess')), findsOneWidget);
    });

    testWidgets('stop asks for confirmation before stopping the series',
        (tester) async {
      final stub = _StubScheduleController(
        initialState: _readyWith([_entry()]),
      );
      await tester.pumpWidget(_buildScreen(stub));
      await tester.pump();

      await tester.tap(find.byKey(const Key('scheduleStop-sched-1')));
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const Key('scheduleStopConfirm')));
      await tester.pumpAndSettle();

      expect(stub.stoppedSchedule?.id, 'sched-1');
    });

    testWidgets('occurrence cancel submits the stable original date',
        (tester) async {
      final stub = _StubScheduleController(
        initialState: _readyWith([_entry()]),
      );
      await tester.pumpWidget(_buildScreen(stub));
      await tester.pump();

      await tester.tap(find.byKey(const Key('scheduleOccurrence-sched-1')));
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const Key('occurrenceCancelSwitch')));
      await tester.pump();
      await tester.tap(find.byKey(const Key('occurrenceSubmitButton')));
      await tester.pumpAndSettle();

      expect(stub.occurrenceCancel, isTrue);
      expect(stub.occurrenceDate, isNotNull);
    });
  });

  group('circle detail entry point', () {
    testWidgets('a teacher opens the schedule manager from circle details',
        (tester) async {
      tester.view.physicalSize = const Size(800, 1600);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      final stub = _StubScheduleController(
        initialState: _readyWith(const []),
      );
      await tester.pumpWidget(
        ProviderScope(
          overrides: [
            authControllerProvider.overrideWith((_) => StubAuthNotifier()),
            circleDetailProvider('circle-1').overrideWith(
              (_) => Future.value(
                CircleResponse(
                  id: 'circle-1',
                  name: 'Circle',
                  inviteCode: 'HLQ-7X2K',
                  inviteLink: 'https://halaqaty.app/join/HLQ-7X2K',
                  createdAt: DateTime.utc(2026, 8, 1),
                ),
              ),
            ),
            circleMembersProvider('circle-1').overrideWith(
              (_) => Future.value([
                CircleMember(
                  userId: 'teacher-1',
                  displayName: 'Teacher',
                  role: CircleRole.teacher,
                  joinedAt: DateTime.utc(2026, 8, 1),
                ),
              ]),
            ),
            circleSessionsControllerProvider('circle-1')
                .overrideWith((_) => _SpyCircleSessionsController()),
            scheduleControllerProvider(_scope).overrideWith((_) => stub),
          ],
          child: const MaterialApp(
            home: CircleDetailScreen(
              circleId: 'circle-1',
              currentUserId: 'teacher-1',
            ),
          ),
        ),
      );
      await tester.pumpAndSettle();

      final scheduleTile = find.byKey(const Key('openCircleSchedule'));
      await tester.ensureVisible(scheduleTile);
      await tester.pumpAndSettle();
      await tester.tap(scheduleTile);
      await tester.pumpAndSettle();

      expect(find.byType(ScheduleEditorScreen), findsOneWidget);
      expect(stub.loadCalls, greaterThanOrEqualTo(1));
    });
  });
}
