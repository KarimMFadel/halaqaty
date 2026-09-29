import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/scheduling/application/calendar_controller.dart';
import 'package:halaqaty_mobile/features/scheduling/data/calendar_api_client.dart';
import 'package:halaqaty_mobile/features/scheduling/data/schedule_api_client.dart';

void main() {
  test('a refreshed overlap warning is sent back with explicit confirmation',
      () async {
    final bodies = <Map<String, dynamic>>[];
    var requests = 0;
    final dio = Dio(BaseOptions(baseUrl: 'http://localhost'));
    dio.interceptors.add(InterceptorsWrapper(
      onRequest: (options, handler) {
        requests++;
        bodies.add(options.data as Map<String, dynamic>);
        if (requests == 1) {
          handler.reject(DioException(
            requestOptions: options,
            response: Response<Map<String, dynamic>>(
              requestOptions: options,
              statusCode: 409,
              data: {
                'error': {
                  'code': 'ERR_CONFLICT',
                  'message': 'Review updated warnings.',
                  'warnings': [
                    {
                      'warning_id': 'fresh-one-off',
                      'first_circle_name': 'Hifz',
                      'second_circle_name': 'Tajweed',
                      'overlap_starts_at': '2026-10-06T15:30:00Z',
                      'overlap_ends_at': '2026-10-06T16:00:00Z',
                    }
                  ],
                }
              },
            ),
          ));
          return;
        }
        handler.resolve(Response<Map<String, dynamic>>(
          requestOptions: options,
          statusCode: 201,
          data: {
            'occurrence_key': 'one-off-1',
            'session_id': 'one-off-1',
            'circle_id': 'circle-1',
            'circle_name': 'Hifz',
            'title': 'Review',
            'starts_at': '2026-10-06T15:00:00Z',
            'ends_at': '2026-10-06T16:00:00Z',
            'planning_timezone': 'Africa/Cairo',
            'state': 'scheduled',
          },
        ));
      },
    ));
    final controller = OneOffController(
      CalendarApiClient(dio),
      () async => (token: 'token', sessionId: 'session'),
      'circle-1',
    );
    addTearDown(controller.dispose);
    const plan = SchedulePlanInput(
      mode: 'one_off',
      anchorLocalDate: '2026-10-06',
      localStartTime: '18:00',
      localEndTime: '19:00',
      durationMinutes: 60,
      timezone: 'Africa/Cairo',
      title: 'Review',
    );

    expect(await controller.create(plan), isFalse);
    expect(controller.state.overlapWarnings.single.warningId, 'fresh-one-off');
    expect(
      await controller.create(plan,
          confirmOverlaps: true, confirmedWarningIDs: const ['fresh-one-off']),
      isTrue,
    );
    expect(bodies.first['confirm_overlaps'], isFalse);
    expect(bodies.last['confirm_overlaps'], isTrue);
    expect(bodies.last['confirmed_warning_ids'], ['fresh-one-off']);
  });

  test('reuses the idempotency key when a one-off create is retried', () async {
    final keys = <String?>[];
    var requests = 0;
    final dio = Dio(BaseOptions(baseUrl: 'http://localhost'));
    dio.interceptors.add(InterceptorsWrapper(
      onRequest: (options, handler) {
        requests++;
        keys.add(options.headers['Idempotency-Key'] as String?);
        if (requests == 1) {
          handler.reject(DioException(
            requestOptions: options,
            type: DioExceptionType.connectionError,
          ));
          return;
        }
        handler.resolve(Response<Map<String, dynamic>>(
          requestOptions: options,
          statusCode: 201,
          data: {
            'occurrence_key': 'one-off-1',
            'session_id': 'one-off-1',
            'circle_id': 'circle-1',
            'circle_name': 'Tajweed Circle',
            'title': 'Review',
            'starts_at': '2026-10-06T15:00:00Z',
            'ends_at': '2026-10-06T16:00:00Z',
            'planning_timezone': 'Africa/Cairo',
            'state': 'scheduled',
          },
        ));
      },
    ));
    final controller = OneOffController(
      CalendarApiClient(dio),
      () async => (token: 'token', sessionId: 'session'),
      'circle-1',
    );
    addTearDown(controller.dispose);
    const plan = SchedulePlanInput(
      mode: 'one_off',
      anchorLocalDate: '2026-10-06',
      localStartTime: '18:00',
      localEndTime: '19:00',
      durationMinutes: 60,
      timezone: 'Africa/Cairo',
      title: 'Review',
    );

    expect(await controller.create(plan), isFalse);
    expect(await controller.create(plan), isTrue);

    expect(keys, hasLength(2));
    expect(keys.first, isNotEmpty);
    expect(keys[1], keys.first);
  });
}
