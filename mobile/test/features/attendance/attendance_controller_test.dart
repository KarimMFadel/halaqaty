import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/attendance/application/attendance_controller.dart';
import 'package:halaqaty_mobile/features/attendance/data/attendance_api_client.dart';

void main() {
  group('AttendanceApiClient', () {
    test('loads the role-filtered session roster', () async {
      RequestOptions? sent;
      final client = AttendanceApiClient(_dio((options, handler) {
        sent = options;
        handler.resolve(Response<List<dynamic>>(
          requestOptions: options,
          statusCode: 200,
          data: [_recordJson()],
        ));
      }));

      final records = await client.list(
        token: 'token',
        sessionId: 'backend-session',
        liveSessionId: 'session-1',
      );

      expect(sent?.method, 'GET');
      expect(sent?.path, '/sessions/session-1/attendance');
      expect(sent?.headers['Authorization'], 'Bearer token');
      expect(records.single.userId, 'student-1');
      expect(records.single.status, AttendanceStatus.present);
    });

    test('sends correction status, reason, and stable retry key', () async {
      RequestOptions? sent;
      final client = AttendanceApiClient(_dio((options, handler) {
        sent = options;
        handler.resolve(Response<Map<String, dynamic>>(
          requestOptions: options,
          statusCode: 200,
          data: _recordJson(status: 'excused', source: 'manual'),
        ));
      }));

      await client.correct(
        token: 'token',
        sessionId: 'backend-session',
        liveSessionId: 'session-1',
        userId: 'student-1',
        status: AttendanceStatus.excused,
        reason: 'Approved by teacher',
        idempotencyKey: 'attendance-1',
      );

      expect(sent?.method, 'PATCH');
      expect(sent?.path, '/sessions/session-1/attendance/student-1');
      expect(sent?.headers['Idempotency-Key'], 'attendance-1');
      expect(
          sent?.data, {'status': 'excused', 'reason': 'Approved by teacher'});
    });
  });

  group('AttendanceController', () {
    test('loads attendance and refreshes the effective status after correction',
        () async {
      var getCount = 0;
      final client = AttendanceApiClient(_dio((options, handler) {
        if (options.method == 'GET') {
          getCount++;
          handler.resolve(Response<List<dynamic>>(
            requestOptions: options,
            statusCode: 200,
            data: [
              _recordJson(
                  status: getCount == 1 ? 'absent' : 'excused',
                  source: getCount == 1 ? 'automatic' : 'manual')
            ],
          ));
        } else {
          handler.resolve(Response<Map<String, dynamic>>(
            requestOptions: options,
            statusCode: 200,
            data: _recordJson(status: 'excused', source: 'manual'),
          ));
        }
      }));
      final controller = AttendanceController(
        client,
        () async => (token: 'token', sessionId: 'session'),
        'session-1',
        canCorrect: true,
        idempotencyKeyGenerator: () => 'attendance-retry',
      );
      addTearDown(controller.dispose);

      await controller.load();
      expect(controller.state.records.single.status, AttendanceStatus.absent);
      expect(
          await controller.correct(
            userId: 'student-1',
            status: AttendanceStatus.excused,
            reason: 'Approved by teacher',
          ),
          isTrue);

      expect(controller.state.records.single.status, AttendanceStatus.excused);
      expect(controller.state.records.single.source, 'manual');
    });

    test('student cannot submit a correction', () async {
      var requests = 0;
      final controller = AttendanceController(
        AttendanceApiClient(_dio((options, handler) {
          requests++;
          handler.resolve(Response<List<dynamic>>(
            requestOptions: options,
            statusCode: 200,
            data: [_recordJson()],
          ));
        })),
        () async => (token: 'token', sessionId: 'session'),
        'session-1',
        canCorrect: false,
      );
      addTearDown(controller.dispose);

      expect(
          await controller.correct(
            userId: 'student-1',
            status: AttendanceStatus.excused,
            reason: 'Approved',
          ),
          isFalse);
      expect(requests, 0);
    });

    test('keeps a correction retry key after network failure', () async {
      var keys = 0;
      final sentKeys = <String>[];
      final client = AttendanceApiClient(_dio((options, handler) {
        if (options.method == 'GET') {
          handler.resolve(Response<List<dynamic>>(
            requestOptions: options,
            statusCode: 200,
            data: [_recordJson()],
          ));
          return;
        }
        sentKeys.add(options.headers['Idempotency-Key'] as String);
        if (sentKeys.length == 1) {
          handler.reject(DioException(
            requestOptions: options,
            type: DioExceptionType.connectionError,
          ));
        } else {
          handler.resolve(Response<Map<String, dynamic>>(
            requestOptions: options,
            statusCode: 200,
            data: _recordJson(status: 'excused', source: 'manual'),
          ));
        }
      }));
      final controller = AttendanceController(
        client,
        () async => (token: 'token', sessionId: 'session'),
        'session-1',
        canCorrect: true,
        idempotencyKeyGenerator: () => 'key-${++keys}',
      );
      addTearDown(controller.dispose);
      await controller.load();
      for (var attempt = 0; attempt < 2; attempt++) {
        await controller.correct(
          userId: 'student-1',
          status: AttendanceStatus.excused,
          reason: 'Approved',
        );
      }

      expect(sentKeys, ['key-1', 'key-1']);
    });
  });
}

Map<String, dynamic> _recordJson({
  String status = 'present',
  String source = 'automatic',
}) =>
    {
      'session_id': 'session-1',
      'user_id': 'student-1',
      'status': status,
      'source': source,
      'first_presence_at': '2026-09-29T10:05:00Z',
      'correction_id': source == 'manual' ? 'correction-1' : null,
      'correction': source == 'manual'
          ? {
              'actor_id': 'teacher-1',
              'at': '2026-09-29T11:00:00Z',
              'reason': 'Approved by teacher',
              'previous_status': 'absent',
              'new_status': status,
            }
          : null,
    };

Dio _dio(void Function(RequestOptions, RequestInterceptorHandler) onRequest) {
  final dio = Dio(BaseOptions(baseUrl: 'http://localhost'));
  dio.interceptors.add(InterceptorsWrapper(onRequest: onRequest));
  return dio;
}
