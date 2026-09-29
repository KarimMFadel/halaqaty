import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/scheduling/application/calendar_controller.dart';
import 'package:halaqaty_mobile/features/scheduling/data/calendar_api_client.dart';

void main() {
  group('CalendarApiClient', () {
    test('requests the selected month with the authenticated session',
        () async {
      RequestOptions? sent;
      final api = CalendarApiClient(_stubDio(
        body: {
          'items': [_itemJson()],
          'warnings': {'warnings': []}
        },
        onRequest: (options) => sent = options,
      ));

      final month = await api.getMonth(
        token: 'firebase-token',
        sessionId: 'backend-session',
        month: DateTime(2026, 10),
      );

      expect(sent?.method, 'GET');
      expect(sent?.path, '/calendar/me');
      expect(sent?.queryParameters['month'], '2026-10');
      expect(sent?.headers['Authorization'], 'Bearer firebase-token');
      expect(sent?.headers['X-Halaqaty-Session-ID'], 'backend-session');
      expect(month.items.single.occurrenceKey, 'sched-1:2026-10-06');
      expect(month.items.single.planningTimezone, 'Africa/Cairo');
    });
  });

  group('CalendarController', () {
    test('opens the current month in the stored profile timezone', () async {
      final requestedMonths = <String>[];
      final controller = CalendarController(
        CalendarApiClient(_stubDio(
          body: {
            'items': [_itemJson()],
            'warnings': {'warnings': []}
          },
          onRequest: (options) {
            requestedMonths.add(options.queryParameters['month'] as String);
          },
        )),
        () async => (token: 'token', sessionId: 'session'),
        timezone: 'Africa/Cairo',
        now: () => DateTime.utc(2026, 9, 30, 22, 30),
      );
      addTearDown(controller.dispose);

      await controller.load();

      expect(requestedMonths, ['2026-10']);
      expect(controller.state.status, CalendarStatus.ready);
    });

    test('navigates to earlier and later months', () async {
      final requestedMonths = <String>[];
      final controller = CalendarController(
        CalendarApiClient(_stubDio(
          body: {
            'items': [_itemJson()],
            'warnings': {'warnings': []}
          },
          onRequest: (options) {
            requestedMonths.add(options.queryParameters['month'] as String);
          },
        )),
        () async => (token: 'token', sessionId: 'session'),
        timezone: 'UTC',
        now: () => DateTime.utc(2026, 10, 15),
      );
      addTearDown(controller.dispose);

      await controller.load();
      await controller.previousMonth();
      await controller.nextMonth();
      await controller.nextMonth();

      expect(requestedMonths, ['2026-10', '2026-09', '2026-10', '2026-11']);
    });

    test('shows an offline load failure as an error state', () async {
      final controller = CalendarController(
        CalendarApiClient(_stubDio(
          body: {
            'items': [],
            'warnings': {'warnings': []}
          },
          onRequest: (options) => throw DioException(
            requestOptions: options,
            type: DioExceptionType.connectionError,
          ),
        )),
        () async => (token: 'token', sessionId: 'session'),
        timezone: 'UTC',
        now: () => DateTime.utc(2026, 10, 15),
      );
      addTearDown(controller.dispose);

      await controller.load();

      expect(controller.state.status, CalendarStatus.error);
    });

    test('does not show the prior month items under a failed new month',
        () async {
      var requests = 0;
      final controller = CalendarController(
        CalendarApiClient(_stubDio(
          body: {
            'items': [_itemJson()],
            'warnings': {'warnings': []}
          },
          onRequest: (options) {
            requests++;
            if (requests == 2) {
              throw DioException(
                requestOptions: options,
                type: DioExceptionType.connectionError,
              );
            }
          },
        )),
        () async => (token: 'token', sessionId: 'session'),
        timezone: 'UTC',
        now: () => DateTime.utc(2026, 10, 15),
      );
      addTearDown(controller.dispose);

      await controller.load();
      expect(controller.state.items, hasLength(1));
      await controller.nextMonth();

      expect(controller.state.month, DateTime(2026, 11));
      expect(controller.state.status, CalendarStatus.error);
      expect(controller.state.items, isEmpty);
    });
  });
}

Map<String, dynamic> _itemJson({
  String key = 'sched-1:2026-10-06',
  String circleId = 'circle-1',
  String circleName = 'Tajweed Circle',
  String state = 'scheduled',
}) =>
    {
      'occurrence_key': key,
      'session_id': null,
      'circle_id': circleId,
      'circle_name': circleName,
      'title': 'Circle Session',
      'starts_at': '2026-10-06T15:00:00Z',
      'ends_at': '2026-10-06T16:00:00Z',
      'planning_timezone': 'Africa/Cairo',
      'state': state,
    };

Dio _stubDio({
  required Map<String, dynamic> body,
  void Function(RequestOptions)? onRequest,
}) {
  final dio = Dio(BaseOptions(baseUrl: 'http://localhost'));
  dio.interceptors.add(InterceptorsWrapper(
    onRequest: (options, handler) {
      onRequest?.call(options);
      handler.resolve(Response<Map<String, dynamic>>(
        requestOptions: options,
        statusCode: 200,
        data: body,
      ));
    },
  ));
  return dio;
}
