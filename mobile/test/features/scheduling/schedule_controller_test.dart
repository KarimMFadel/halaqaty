import 'dart:async';

import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/scheduling/application/schedule_controller.dart';
import 'package:halaqaty_mobile/features/scheduling/data/schedule_api_client.dart';

void main() {
  group('ScheduleApiClient', () {
    test('listSchedules parses the data envelope and sends auth headers',
        () async {
      RequestOptions? sent;
      final dio = _stubDio(
        statusCode: 200,
        body: {
          'data': [_scheduleJson()],
        },
        onRequest: (options) => sent = options,
      );

      final schedules = await ScheduleApiClient(dio).listSchedules(
        token: 'firebase-token',
        sessionId: 'backend-session',
        circleId: 'circle-1',
      );

      expect(sent?.method, 'GET');
      expect(sent?.path, '/circles/circle-1/schedules');
      expect(sent?.headers['Authorization'], 'Bearer firebase-token');
      expect(sent?.headers['X-Halaqaty-Session-ID'], 'backend-session');
      expect(schedules.single.id, 'sched-1');
      expect(schedules.single.circleId, 'circle-1');
      expect(schedules.single.version, 3);
      expect(schedules.single.stoppedFromLocalDate, isNull);
      expect(schedules.single.plan.mode, ScheduleModes.weekdayPattern);
      expect(schedules.single.plan.weekCadence, 2);
      expect(schedules.single.plan.weekdays, [1, 3]);
      expect(schedules.single.plan.timezone, 'Africa/Cairo');
    });

    test(
        'createSchedule builds a biweekly weekday payload and sends the '
        'idempotency key', () async {
      RequestOptions? sent;
      final dio = _stubDio(
        statusCode: 201,
        body: _scheduleJson(),
        onRequest: (options) => sent = options,
      );

      await ScheduleApiClient(dio).createSchedule(
        token: 'firebase-token',
        sessionId: 'backend-session',
        circleId: 'circle-1',
        plan: _weekdayPlan(),
        idempotencyKey: 'key-1',
      );

      expect(sent?.method, 'POST');
      expect(sent?.path, '/circles/circle-1/schedules');
      expect(sent?.headers['Idempotency-Key'], 'key-1');
      final body = sent?.data as Map<String, dynamic>;
      expect(body, {
        'mode': 'weekday_pattern',
        'anchor_local_date': '2026-10-06',
        'local_start_time': '18:00',
        'local_end_time': '19:30',
        'duration_minutes': 90,
        'timezone': 'Africa/Cairo',
        'confirm_overlaps': false,
        'confirmed_warning_ids': <String>[],
        'week_cadence': 2,
        'weekdays': [1, 3],
      });
    });

    test(
        'createSchedule includes the optional title and end date only when '
        'set', () async {
      RequestOptions? sent;
      final dio = _stubDio(
        statusCode: 201,
        body: _scheduleJson(),
        onRequest: (options) => sent = options,
      );

      await ScheduleApiClient(dio).createSchedule(
        token: 'firebase-token',
        sessionId: 'backend-session',
        circleId: 'circle-1',
        plan: SchedulePlanInput(
          mode: ScheduleModes.weekdayPattern,
          anchorLocalDate: '2026-10-06',
          localStartTime: '18:00',
          localEndTime: '19:30',
          durationMinutes: 90,
          timezone: 'Africa/Cairo',
          title: 'Tajweed review',
          endLocalDate: '2026-12-30',
          weekCadence: 1,
          weekdays: const [1],
        ),
        idempotencyKey: 'key-2',
      );

      final body = sent?.data as Map<String, dynamic>;
      expect(body['title'], 'Tajweed review');
      expect(body['end_local_date'], '2026-12-30');
    });

    test('createSchedule builds an interval payload without weekday selectors',
        () async {
      RequestOptions? sent;
      final dio = _stubDio(
        statusCode: 201,
        body: _scheduleJson(),
        onRequest: (options) => sent = options,
      );

      await ScheduleApiClient(dio).createSchedule(
        token: 'firebase-token',
        sessionId: 'backend-session',
        circleId: 'circle-1',
        plan: const SchedulePlanInput(
          mode: ScheduleModes.interval,
          anchorLocalDate: '2026-10-06',
          localStartTime: '20:00',
          localEndTime: '21:00',
          durationMinutes: 60,
          timezone: 'UTC',
          intervalCount: 3,
          intervalUnit: 'day',
        ),
        idempotencyKey: 'key-3',
      );

      final body = sent?.data as Map<String, dynamic>;
      expect(body['mode'], 'interval');
      expect(body['interval_count'], 3);
      expect(body['interval_unit'], 'day');
      expect(body.containsKey('week_cadence'), isFalse);
      expect(body.containsKey('weekdays'), isFalse);
      expect(body.containsKey('selected_dates'), isFalse);
    });

    test('createSchedule builds a selected_dates payload', () async {
      RequestOptions? sent;
      final dio = _stubDio(
        statusCode: 201,
        body: _scheduleJson(),
        onRequest: (options) => sent = options,
      );

      await ScheduleApiClient(dio).createSchedule(
        token: 'firebase-token',
        sessionId: 'backend-session',
        circleId: 'circle-1',
        plan: const SchedulePlanInput(
          mode: ScheduleModes.selectedDates,
          anchorLocalDate: '2026-10-06',
          localStartTime: '20:00',
          localEndTime: '21:00',
          durationMinutes: 60,
          timezone: 'UTC',
          selectedDates: ['2026-10-06', '2026-10-20'],
        ),
        idempotencyKey: 'key-4',
      );

      final body = sent?.data as Map<String, dynamic>;
      expect(body['mode'], 'selected_dates');
      expect(body['selected_dates'], ['2026-10-06', '2026-10-20']);
      expect(body.containsKey('weekdays'), isFalse);
      expect(body.containsKey('interval_count'), isFalse);
    });

    test('changeSchedule sends expected version, effective date and plan',
        () async {
      RequestOptions? sent;
      final dio = _stubDio(
        statusCode: 200,
        body: _scheduleJson(version: 4),
        onRequest: (options) => sent = options,
      );

      final updated = await ScheduleApiClient(dio).changeSchedule(
        token: 'firebase-token',
        sessionId: 'backend-session',
        circleId: 'circle-1',
        scheduleId: 'sched-1',
        expectedVersion: 3,
        effectiveLocalDate: '2026-10-07',
        plan: _weekdayPlan(),
        idempotencyKey: 'key-5',
      );

      expect(sent?.method, 'PATCH');
      expect(sent?.path, '/circles/circle-1/schedules/sched-1');
      expect(sent?.headers['Idempotency-Key'], 'key-5');
      final body = sent?.data as Map<String, dynamic>;
      expect(body['expected_version'], 3);
      expect(body['effective_local_date'], '2026-10-07');
      expect(body['stop'], isFalse);
      expect(body['confirm_overlaps'], isFalse);
      expect(body['confirmed_warning_ids'], <String>[]);
      expect(body['plan'], isA<Map<String, dynamic>>());
      expect(updated.version, 4);
    });

    test('changeSchedule stop keeps the current plan and sets stop', () async {
      RequestOptions? sent;
      final dio = _stubDio(
        statusCode: 200,
        body: _scheduleJson(stoppedFrom: '2026-10-07'),
        onRequest: (options) => sent = options,
      );

      final updated = await ScheduleApiClient(dio).changeSchedule(
        token: 'firebase-token',
        sessionId: 'backend-session',
        circleId: 'circle-1',
        scheduleId: 'sched-1',
        expectedVersion: 3,
        effectiveLocalDate: '2026-10-07',
        plan: _weekdayPlan(),
        stop: true,
        idempotencyKey: 'key-6',
      );

      final body = sent?.data as Map<String, dynamic>;
      expect(body['stop'], isTrue);
      expect((body['plan'] as Map<String, dynamic>)['mode'], 'weekday_pattern');
      expect(updated.stoppedFromLocalDate, '2026-10-07');
    });

    test('changeOccurrence cancel sends both versions and cancelled flag',
        () async {
      RequestOptions? sent;
      final dio = _stubDio(
        statusCode: 200,
        body: _calendarItemJson(),
        onRequest: (options) => sent = options,
      );

      await ScheduleApiClient(dio).changeOccurrence(
        token: 'firebase-token',
        sessionId: 'backend-session',
        circleId: 'circle-1',
        scheduleId: 'sched-1',
        localDate: '2026-10-06',
        expectedSeriesVersion: 3,
        cancel: true,
        idempotencyKey: 'key-7',
      );

      expect(sent?.method, 'PATCH');
      expect(
        sent?.path,
        '/circles/circle-1/schedules/sched-1/occurrences/2026-10-06',
      );
      expect(sent?.headers['Idempotency-Key'], 'key-7');
      final body = sent?.data as Map<String, dynamic>;
      expect(body['expected_series_version'], 3);
      expect(body['expected_occurrence_version'], 0);
      expect(body['cancelled'], isTrue);
      expect(body['confirm_overlaps'], isFalse);
      expect(body['confirmed_warning_ids'], <String>[]);
    });

    test('changeOccurrence move sends the replacement date and times',
        () async {
      RequestOptions? sent;
      final dio = _stubDio(
        statusCode: 200,
        body: _calendarItemJson(),
        onRequest: (options) => sent = options,
      );

      await ScheduleApiClient(dio).changeOccurrence(
        token: 'firebase-token',
        sessionId: 'backend-session',
        circleId: 'circle-1',
        scheduleId: 'sched-1',
        localDate: '2026-10-06',
        expectedSeriesVersion: 3,
        replacementLocalDate: '2026-10-08',
        replacementLocalTime: '17:00',
        replacementEndLocalTime: '18:30',
        idempotencyKey: 'key-8',
      );

      final body = sent?.data as Map<String, dynamic>;
      expect(body['cancelled'], isFalse);
      expect(body['replacement_local_date'], '2026-10-08');
      expect(body['replacement_local_time'], '17:00');
      expect(body['replacement_end_local_time'], '18:30');
    });
  });

  group('ScheduleController', () {
    test('initial state is loading with an empty list', () {
      final controller = _controller(_FakeScheduleApi());
      addTearDown(controller.dispose);

      expect(controller.state.status, ScheduleListStatus.loading);
      expect(controller.state.schedules, isEmpty);
      expect(controller.state.failure, isNull);
    });

    test('load lists circle schedules with injected credentials', () async {
      final api = _FakeScheduleApi()..listResult = [_entry()];
      final controller = _controller(api);
      addTearDown(controller.dispose);

      await controller.load();

      expect(controller.state.status, ScheduleListStatus.ready);
      expect(controller.state.schedules.single.id, 'sched-1');
      expect(api.lastToken, 'firebase-token');
      expect(api.lastSessionId, 'backend-session');
      expect(api.lastCircleId, 'circle-1');
    });

    test('load failure keeps the last loaded list (offline, not blank)',
        () async {
      final api = _FakeScheduleApi()..listResult = [_entry()];
      final controller = _controller(api);
      addTearDown(controller.dispose);
      await controller.load();

      api.listError = DioException(
        requestOptions: RequestOptions(path: '/circles/circle-1/schedules'),
        type: DioExceptionType.connectionError,
      );
      await controller.load();

      expect(controller.state.status, ScheduleListStatus.error);
      expect(controller.state.failure, ScheduleFailure.network);
      expect(controller.state.schedules.single.id, 'sched-1');
    });

    test('a 403 load failure is reported as a permission failure', () async {
      final api = _FakeScheduleApi()..listError = _dioError(statusCode: 403);
      final controller = _controller(api);
      addTearDown(controller.dispose);

      await controller.load();

      expect(controller.state.status, ScheduleListStatus.error);
      expect(controller.state.failure, ScheduleFailure.permission);
    });

    test('a 404 load failure is reported as gone', () async {
      final api = _FakeScheduleApi()..listError = _dioError(statusCode: 404);
      final controller = _controller(api);
      addTearDown(controller.dispose);

      await controller.load();

      expect(controller.state.status, ScheduleListStatus.error);
      expect(controller.state.failure, ScheduleFailure.gone);
    });

    test('retry after a failure reloads successfully', () async {
      final api = _FakeScheduleApi()..listError = StateError('temporary');
      final controller = _controller(api);
      addTearDown(controller.dispose);
      await controller.load();
      expect(controller.state.status, ScheduleListStatus.error);
      expect(controller.state.failure, ScheduleFailure.unknown);

      api.listError = null;
      api.listResult = [_entry(id: 'sched-2')];
      await controller.load();

      expect(controller.state.status, ScheduleListStatus.ready);
      expect(controller.state.schedules.single.id, 'sched-2');
      expect(api.listCalls, 2);
    });

    test('create success flags the save and refreshes the list', () async {
      final api = _FakeScheduleApi()
        ..createResult = _entry()
        ..listResult = [_entry()];
      final controller = _controller(api);
      addTearDown(controller.dispose);

      final created = await controller.createSchedule(_weekdayPlan());

      expect(created, isTrue);
      expect(controller.state.saveSucceeded, isTrue);
      expect(controller.state.isSaving, isFalse);
      expect(api.createCalls, 1);
      expect(api.listCalls, 1);
      expect(controller.state.schedules.single.id, 'sched-1');
    });

    test('create exposes isSaving while the request is in flight', () async {
      final api = _FakeScheduleApi()
        ..createGate = Completer<void>()
        ..createResult = _entry();
      final controller = _controller(api);
      addTearDown(controller.dispose);

      final pending = controller.createSchedule(_weekdayPlan());
      await Future<void>.delayed(Duration.zero);
      expect(controller.state.isSaving, isTrue);

      api.createGate!.complete();
      expect(await pending, isTrue);
      expect(controller.state.isSaving, isFalse);
    });

    test('non-manager create is denied before any API call', () async {
      final api = _FakeScheduleApi();
      final controller = _controller(api, canManage: false);
      addTearDown(controller.dispose);

      final created = await controller.createSchedule(_weekdayPlan());

      expect(created, isFalse);
      expect(controller.state.saveError, ScheduleSaveError.permission);
      expect(api.createCalls, 0);
    });

    test('a past anchor date is rejected without an API call', () async {
      final api = _FakeScheduleApi();
      final controller = _controller(api, now: DateTime(2026, 10, 6));
      addTearDown(controller.dispose);

      final created = await controller.createSchedule(
        _weekdayPlan(anchorLocalDate: '2026-10-05'),
      );

      expect(created, isFalse);
      expect(controller.state.saveError, ScheduleSaveError.pastDate);
      expect(api.createCalls, 0);
    });

    test('a past selected date is rejected without an API call', () async {
      final api = _FakeScheduleApi();
      final controller = _controller(api, now: DateTime(2026, 10, 6));
      addTearDown(controller.dispose);

      final created = await controller.createSchedule(
        const SchedulePlanInput(
          mode: ScheduleModes.selectedDates,
          anchorLocalDate: '2026-10-06',
          localStartTime: '18:00',
          localEndTime: '19:00',
          durationMinutes: 60,
          timezone: 'UTC',
          selectedDates: ['2026-10-06', '2026-10-01'],
        ),
      );

      expect(created, isFalse);
      expect(controller.state.saveError, ScheduleSaveError.pastDate);
      expect(api.createCalls, 0);
    });

    test('today as the anchor date is accepted', () async {
      final api = _FakeScheduleApi()..createResult = _entry();
      final controller = _controller(api, now: DateTime(2026, 10, 6, 23, 59));
      addTearDown(controller.dispose);

      final created = await controller.createSchedule(
        _weekdayPlan(anchorLocalDate: '2026-10-06'),
      );

      expect(created, isTrue);
      expect(api.createCalls, 1);
    });

    test('a 422 save failure is surfaced as a validation error', () async {
      final api = _FakeScheduleApi()..createError = _dioError(statusCode: 422);
      final controller = _controller(api);
      addTearDown(controller.dispose);

      final created = await controller.createSchedule(_weekdayPlan());

      expect(created, isFalse);
      expect(controller.state.saveError, ScheduleSaveError.validation);
      expect(controller.state.saveSucceeded, isFalse);
    });

    test('a 409 series change is surfaced as a conflict', () async {
      final api = _FakeScheduleApi()..changeError = _dioError(statusCode: 409);
      final controller = _controller(api);
      addTearDown(controller.dispose);

      final changed = await controller.changeSeries(
        schedule: _entry(),
        plan: _weekdayPlan(),
      );

      expect(changed, isFalse);
      expect(controller.state.saveError, ScheduleSaveError.conflict);
    });

    test('a 401 save failure clears the local session via logout', () async {
      var logoutCalls = 0;
      final api = _FakeScheduleApi()..createError = _dioError(statusCode: 401);
      final controller = _controller(
        api,
        onLogout: () async => logoutCalls++,
      );
      addTearDown(controller.dispose);

      final created = await controller.createSchedule(_weekdayPlan());

      expect(created, isFalse);
      expect(logoutCalls, 1);
      expect(controller.state.saveError, ScheduleSaveError.permission);
    });

    test('a retried create reuses the idempotency key until success', () async {
      var keyCounter = 0;
      final api = _FakeScheduleApi()
        ..createError = DioException(
          requestOptions: RequestOptions(path: '/circles/circle-1/schedules'),
          type: DioExceptionType.connectionError,
        );
      final controller = _controller(
        api,
        nextKey: () => 'generated-${++keyCounter}',
      );
      addTearDown(controller.dispose);

      await controller.createSchedule(_weekdayPlan());
      await controller.createSchedule(_weekdayPlan());

      expect(api.createKeys, ['generated-1', 'generated-1']);

      api.createError = null;
      api.createResult = _entry();
      await controller.createSchedule(_weekdayPlan());
      expect(api.createKeys.last, 'generated-1');

      await controller.createSchedule(_weekdayPlan());
      expect(api.createKeys.last, 'generated-2');
    });

    test('changed create payload gets a new key after an uncertain failure',
        () async {
      var counter = 0;
      final api = _FakeScheduleApi()..createError = _dioError(statusCode: 503);
      final controller = _controller(api, nextKey: () => 'key-${++counter}');
      addTearDown(controller.dispose);
      await controller.createSchedule(_weekdayPlan());
      await controller.createSchedule(_weekdayPlan(weekCadence: 1));
      await controller.createSchedule(_weekdayPlan(weekCadence: 1));
      expect(api.createKeys, ['key-1', 'key-2', 'key-2']);
    });

    test('stopSeries sends stop with the current plan and version', () async {
      final api = _FakeScheduleApi()..changeResult = _entry();
      final controller = _controller(api, now: DateTime(2026, 10, 6));
      addTearDown(controller.dispose);

      final stopped = await controller.stopSeries(schedule: _entry());

      expect(stopped, isTrue);
      expect(api.changeCalls, 1);
      expect(api.lastChangeStop, isTrue);
      expect(api.lastChangeVersion, 3);
      expect(api.lastChangeEffectiveDate, '2026-10-06');
      expect(api.lastChangePlan?.mode, ScheduleModes.weekdayPattern);
    });

    test('changeSeries sends the new plan with the schedule version', () async {
      final api = _FakeScheduleApi()..changeResult = _entry(version: 4);
      final controller = _controller(api, now: DateTime(2026, 10, 6));
      addTearDown(controller.dispose);

      final changed = await controller.changeSeries(
        schedule: _entry(),
        plan: _weekdayPlan(weekCadence: 1, weekdays: const [1]),
      );

      expect(changed, isTrue);
      expect(api.lastChangeStop, isFalse);
      expect(api.lastChangeVersion, 3);
      expect(api.lastChangePlan?.weekCadence, 1);
      expect(controller.state.saveSucceeded, isTrue);
    });

    test('changeOccurrence cancel passes the occurrence identity and flag',
        () async {
      final api = _FakeScheduleApi();
      final controller = _controller(api);
      addTearDown(controller.dispose);

      final changed = await controller.changeOccurrence(
        schedule: _entry(),
        localDate: '2026-10-06',
        cancel: true,
      );

      expect(changed, isTrue);
      expect(api.occurrenceCalls, 1);
      expect(api.lastOccurrenceDate, '2026-10-06');
      expect(api.lastOccurrenceCancel, isTrue);
      expect(api.lastOccurrenceSeriesVersion, 3);
    });

    test('a past occurrence date is rejected without an API call', () async {
      final api = _FakeScheduleApi();
      final controller = _controller(api, now: DateTime(2026, 10, 6));
      addTearDown(controller.dispose);

      final changed = await controller.changeOccurrence(
        schedule: _entry(),
        localDate: '2026-10-05',
        cancel: true,
      );

      expect(changed, isFalse);
      expect(controller.state.saveError, ScheduleSaveError.pastDate);
      expect(api.occurrenceCalls, 0);
    });

    test('a failed save keeps the loaded list intact', () async {
      final api = _FakeScheduleApi()
        ..listResult = [_entry()]
        ..createError = DioException(
          requestOptions: RequestOptions(path: '/circles/circle-1/schedules'),
          type: DioExceptionType.connectionError,
        );
      final controller = _controller(api);
      addTearDown(controller.dispose);
      await controller.load();

      final created = await controller.createSchedule(_weekdayPlan());

      expect(created, isFalse);
      expect(controller.state.saveError, ScheduleSaveError.network);
      expect(controller.state.status, ScheduleListStatus.ready);
      expect(controller.state.schedules.single.id, 'sched-1');
    });

    test('a 409 exposes only the refreshed overlap warnings to confirmation UI',
        () async {
      final request = RequestOptions(path: '/circles/circle-1/schedules');
      final api = _FakeScheduleApi()
        ..createError = DioException(
          requestOptions: request,
          response: Response<Map<String, dynamic>>(
            requestOptions: request,
            statusCode: 409,
            data: {
              'error': {
                'warnings': [
                  {
                    'warning_id': 'new-warning',
                    'first_circle_name': 'Hifz',
                    'second_circle_name': 'Review',
                    'overlap_starts_at': '2026-10-06T15:30:00Z',
                    'overlap_ends_at': '2026-10-06T16:00:00Z',
                  }
                ],
              }
            },
          ),
        );
      final controller = _controller(api);
      addTearDown(controller.dispose);

      expect(await controller.createSchedule(_weekdayPlan()), isFalse);

      expect(controller.state.overlapWarnings.single.warningId, 'new-warning');
      expect(controller.state.saveError, ScheduleSaveError.conflict);
    });
  });
}

Dio _stubDio({
  required int statusCode,
  required Object body,
  void Function(RequestOptions)? onRequest,
}) {
  final dio = Dio(BaseOptions(baseUrl: 'http://localhost'));
  dio.interceptors.add(
    InterceptorsWrapper(
      onRequest: (options, handler) {
        onRequest?.call(options);
        handler.resolve(
          Response<Object>(
            requestOptions: options,
            statusCode: statusCode,
            data: body,
          ),
        );
      },
    ),
  );
  return dio;
}

DioException _dioError({required int statusCode}) => DioException(
      requestOptions: RequestOptions(path: '/circles/circle-1/schedules'),
      response: Response<Map<String, dynamic>>(
        requestOptions: RequestOptions(path: '/circles/circle-1/schedules'),
        statusCode: statusCode,
        data: const {
          'error': {'code': 'ERR', 'message': 'rejected'},
        },
      ),
    );

SchedulePlanInput _weekdayPlan({
  String anchorLocalDate = '2026-10-06',
  int weekCadence = 2,
  List<int> weekdays = const [1, 3],
}) =>
    SchedulePlanInput(
      mode: ScheduleModes.weekdayPattern,
      anchorLocalDate: anchorLocalDate,
      localStartTime: '18:00',
      localEndTime: '19:30',
      durationMinutes: 90,
      timezone: 'Africa/Cairo',
      weekCadence: weekCadence,
      weekdays: weekdays,
    );

Map<String, dynamic> _scheduleJson({
  String id = 'sched-1',
  int version = 3,
  String? stoppedFrom,
}) =>
    {
      'id': id,
      'circle_id': 'circle-1',
      'version': version,
      'stopped_from_local_date': stoppedFrom,
      'plan': {
        'mode': 'weekday_pattern',
        'anchor_local_date': '2026-10-06',
        'local_start_time': '18:00',
        'local_end_time': '19:30',
        'duration_minutes': 90,
        'timezone': 'Africa/Cairo',
        'week_cadence': 2,
        'weekdays': [1, 3],
      },
    };

Map<String, dynamic> _calendarItemJson() => {
      'occurrence_key': 'sched-1:2026-10-06',
      'session_id': null,
      'circle_id': 'circle-1',
      'circle_name': 'Tajweed Circle',
      'title': 'Circle Session',
      'starts_at': '2026-10-06T16:00:00Z',
      'ends_at': '2026-10-06T17:30:00Z',
      'planning_timezone': 'Africa/Cairo',
      'state': 'cancelled',
    };

CircleScheduleEntry _entry({String id = 'sched-1', int version = 3}) =>
    CircleScheduleEntry.fromJson(_scheduleJson(id: id, version: version));

ScheduleController _controller(
  _FakeScheduleApi api, {
  bool canManage = true,
  DateTime? now,
  Future<void> Function()? onLogout,
  String Function()? nextKey,
}) =>
    ScheduleController(
      api,
      () async => (token: 'firebase-token', sessionId: 'backend-session'),
      circleId: 'circle-1',
      canManage: canManage,
      logout: onLogout ?? () async {},
      now: () => now ?? DateTime(2026, 10, 6),
      idempotencyKeyGenerator: nextKey,
    );

class _FakeScheduleApi extends ScheduleApiClient {
  _FakeScheduleApi() : super(Dio());

  List<CircleScheduleEntry> listResult = const [];
  Object? listError;
  CircleScheduleEntry? createResult;
  Object? createError;
  Completer<void>? createGate;
  CircleScheduleEntry? changeResult;
  Object? changeError;
  Object? occurrenceError;

  int listCalls = 0;
  int createCalls = 0;
  int changeCalls = 0;
  int occurrenceCalls = 0;
  String? lastToken;
  String? lastSessionId;
  String? lastCircleId;
  bool? lastChangeStop;
  int? lastChangeVersion;
  String? lastChangeEffectiveDate;
  SchedulePlanInput? lastChangePlan;
  String? lastOccurrenceDate;
  bool? lastOccurrenceCancel;
  int? lastOccurrenceSeriesVersion;
  final List<String> createKeys = [];

  @override
  Future<List<CircleScheduleEntry>> listSchedules({
    required String token,
    required String sessionId,
    required String circleId,
  }) async {
    listCalls++;
    lastToken = token;
    lastSessionId = sessionId;
    lastCircleId = circleId;
    final error = listError;
    if (error != null) throw error;
    return listResult;
  }

  @override
  Future<CircleScheduleEntry> createSchedule({
    required String token,
    required String sessionId,
    required String circleId,
    required SchedulePlanInput plan,
    required String idempotencyKey,
    bool confirmOverlaps = false,
    List<String> confirmedWarningIDs = const [],
  }) async {
    createCalls++;
    createKeys.add(idempotencyKey);
    final gate = createGate;
    if (gate != null) await gate.future;
    final error = createError;
    if (error != null) throw error;
    return createResult!;
  }

  @override
  Future<CircleScheduleEntry> changeSchedule({
    required String token,
    required String sessionId,
    required String circleId,
    required String scheduleId,
    required int expectedVersion,
    required String effectiveLocalDate,
    required SchedulePlanInput plan,
    bool stop = false,
    required String idempotencyKey,
    bool confirmOverlaps = false,
    List<String> confirmedWarningIDs = const [],
  }) async {
    changeCalls++;
    lastChangeStop = stop;
    lastChangeVersion = expectedVersion;
    lastChangeEffectiveDate = effectiveLocalDate;
    lastChangePlan = plan;
    final error = changeError;
    if (error != null) throw error;
    return changeResult!;
  }

  @override
  Future<void> changeOccurrence({
    required String token,
    required String sessionId,
    required String circleId,
    required String scheduleId,
    required String localDate,
    required int expectedSeriesVersion,
    int expectedOccurrenceVersion = 0,
    String? replacementLocalDate,
    String? replacementLocalTime,
    String? replacementEndLocalTime,
    int? durationMinutes,
    String? title,
    bool cancel = false,
    required String idempotencyKey,
    bool confirmOverlaps = false,
    List<String> confirmedWarningIDs = const [],
  }) async {
    occurrenceCalls++;
    lastOccurrenceDate = localDate;
    lastOccurrenceCancel = cancel;
    lastOccurrenceSeriesVersion = expectedSeriesVersion;
    final error = occurrenceError;
    if (error != null) throw error;
  }
}
