import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/scheduling/application/calendar_controller.dart';
import 'package:halaqaty_mobile/features/scheduling/data/calendar_api_client.dart';
import 'package:halaqaty_mobile/features/scheduling/data/schedule_api_client.dart';

void main() {
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
