import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/scheduling/data/calendar_api_client.dart';
import 'package:halaqaty_mobile/features/scheduling/data/schedule_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_api_client.dart';
import 'package:integration_test/integration_test.dart';

/// T037 [US2]: real-backend planned-session calendar lifecycle and membership.
///
/// Supply `T024_FIREBASE_WEB_API_KEY` using the existing disposable-account
/// fixture pattern and optionally `T024_API_BASE_URL`. Each account is created
/// for this run and deleted during teardown; no fixture tokens are logged.
void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets('T037: calendar lifecycle, circles, and revoked membership',
      (tester) async {
    const webApiKey = String.fromEnvironment('T024_FIREBASE_WEB_API_KEY');
    if (webApiKey.isEmpty) {
      markTestSkipped(
        'T024_FIREBASE_WEB_API_KEY dart-define missing; provide the existing '
        'Firebase Identity Toolkit web API key fixture.',
      );
      return;
    }
    const baseUrl = String.fromEnvironment(
      'T024_API_BASE_URL',
      defaultValue: 'http://10.0.2.2:8080/api/v1',
    );
    final dio = Dio(BaseOptions(baseUrl: baseUrl));
    addTearDown(dio.close);
    final identity = _IdentityToolkitClient(webApiKey);
    final circles = CircleApiClient(dio);
    final schedules = ScheduleApiClient(dio);
    final calendar = CalendarApiClient(dio);
    final runId = DateTime.now().toUtc().microsecondsSinceEpoch;
    final teacher = await _disposableSession(
      dio,
      identity,
      email: 't037-teacher-$runId@example.com',
    );
    final student = await _disposableSession(
      dio,
      identity,
      email: 't037-student-$runId@example.com',
    );
    for (final account in [teacher, student]) {
      addTearDown(() => identity.deleteAccount(account.token));
    }

    final firstCircle = await circles.createCircle(
      firebaseIdToken: teacher.token,
      sessionId: teacher.sessionId,
      request: CreateCircleRequest(
        name: 'T037 Calendar A $runId',
        language: 'ar',
        maxCapacity: 10,
      ),
    );
    final secondCircle = await circles.createCircle(
      firebaseIdToken: teacher.token,
      sessionId: teacher.sessionId,
      request: CreateCircleRequest(
        name: 'T037 Calendar B $runId',
        language: 'ar',
        maxCapacity: 10,
      ),
    );
    for (final circle in [firstCircle, secondCircle]) {
      addTearDown(() async {
        try {
          await circles.archiveCircle(
            firebaseIdToken: teacher.token,
            sessionId: teacher.sessionId,
            circleId: circle.id,
          );
        } on DioException {
          // The journey assertions have completed; cleanup is best effort.
        }
      });
      await circles.joinCircleByInvite(
        firebaseIdToken: student.token,
        sessionId: student.sessionId,
        inviteCode: circle.inviteCode,
      );
    }

    final month = DateTime(DateTime.now().year, DateTime.now().month + 1);
    final today = _dateOnly(DateTime(month.year, month.month, 5));
    final tomorrow = _dateOnly(DateTime(month.year, month.month, 6));
    final dayAfter = _dateOnly(DateTime(month.year, month.month, 7));
    final weekday = DateTime.parse(today).weekday % 7;
    const planZone = 'Africa/Cairo';

    final startedSeries = await schedules.createSchedule(
      token: teacher.token,
      sessionId: teacher.sessionId,
      circleId: firstCircle.id,
      plan: SchedulePlanInput(
        mode: ScheduleModes.weekdayPattern,
        anchorLocalDate: today,
        localStartTime: '09:00',
        localEndTime: '10:00',
        durationMinutes: 60,
        timezone: planZone,
        title: 'T037 Started Recurrence',
        weekCadence: 1,
        weekdays: [weekday],
      ),
      idempotencyKey: 't037-start-series-$runId',
    );
    final cancelledSeries = await schedules.createSchedule(
      token: teacher.token,
      sessionId: teacher.sessionId,
      circleId: firstCircle.id,
      plan: SchedulePlanInput(
        mode: ScheduleModes.selectedDates,
        anchorLocalDate: tomorrow,
        localStartTime: '11:00',
        localEndTime: '12:00',
        durationMinutes: 60,
        timezone: planZone,
        title: 'T037 Cancelled Recurrence',
        selectedDates: [tomorrow],
      ),
      idempotencyKey: 't037-cancel-series-$runId',
    );
    final secondCircleSeries = await schedules.createSchedule(
      token: teacher.token,
      sessionId: teacher.sessionId,
      circleId: secondCircle.id,
      plan: SchedulePlanInput(
        mode: ScheduleModes.selectedDates,
        anchorLocalDate: dayAfter,
        localStartTime: '13:00',
        localEndTime: '14:00',
        durationMinutes: 60,
        timezone: planZone,
        title: 'T037 Other Circle Recurrence',
        selectedDates: [dayAfter],
      ),
      idempotencyKey: 't037-other-circle-$runId',
    );
    final oneOff = await calendar.createOneOff(
      token: teacher.token,
      sessionId: teacher.sessionId,
      circleId: firstCircle.id,
      plan: SchedulePlanInput(
        mode: 'one_off',
        anchorLocalDate: dayAfter,
        localStartTime: '15:00',
        localEndTime: '15:45',
        durationMinutes: 45,
        timezone: planZone,
        title: 'T037 One Off',
      ),
      idempotencyKey: 't037-create-one-off-$runId',
    );

    final beforeLifecycle = await calendar.getMonth(
      token: student.token,
      sessionId: student.sessionId,
      month: month,
    );
    expect(
        beforeLifecycle.items.map((item) => item.title),
        containsAll([
          'T037 Started Recurrence',
          'T037 Cancelled Recurrence',
          'T037 Other Circle Recurrence',
          'T037 One Off',
        ]));
    expect(
      beforeLifecycle.items
          .singleWhere((item) => item.occurrenceKey == oneOff.occurrenceKey)
          .state,
      'scheduled',
    );
    expect(
      beforeLifecycle.items
          .singleWhere((item) => item.occurrenceKey == oneOff.occurrenceKey)
          .endsAt
          .difference(beforeLifecycle.items
              .singleWhere((item) => item.occurrenceKey == oneOff.occurrenceKey)
              .startsAt)
          .inMinutes,
      45,
      reason: 'one-off start and end instants are projected to the calendar',
    );
    expect(
      beforeLifecycle.items
          .singleWhere((item) => item.title == 'T037 Other Circle Recurrence')
          .circleId,
      secondCircle.id,
      reason: 'calendar items preserve identity across circles',
    );

    await schedules.changeOccurrence(
      token: teacher.token,
      sessionId: teacher.sessionId,
      circleId: firstCircle.id,
      scheduleId: cancelledSeries.id,
      localDate: tomorrow,
      expectedSeriesVersion: cancelledSeries.version,
      cancel: true,
      idempotencyKey: 't037-cancel-occurrence-$runId',
    );
    await dio.patch<Map<String, dynamic>>(
      '/sessions/${oneOff.sessionId}/planned-details',
      data: {
        'expected_version': 1,
        'cancelled': true,
        'confirm_overlaps': false,
      },
      options: Options(headers: {
        'Authorization': 'Bearer ${teacher.token}',
        'X-Halaqaty-Session-ID': teacher.sessionId,
        'Idempotency-Key': 't037-cancel-one-off-$runId',
      }),
    );

    final afterLifecycle = await calendar.getMonth(
      token: student.token,
      sessionId: student.sessionId,
      month: month,
    );
    expect(
      _itemForOccurrence(afterLifecycle.items, startedSeries.id, today).state,
      'scheduled',
    );
    expect(
      _itemForOccurrence(afterLifecycle.items, cancelledSeries.id, tomorrow)
          .state,
      'cancelled',
    );
    expect(
      afterLifecycle.items
          .singleWhere((item) => item.occurrenceKey == oneOff.occurrenceKey)
          .state,
      'cancelled',
    );

    await circles.removeMember(
      firebaseIdToken: teacher.token,
      sessionId: teacher.sessionId,
      circleId: secondCircle.id,
      userId: student.userId,
    );
    final afterRevocation = await calendar.getMonth(
      token: student.token,
      sessionId: student.sessionId,
      month: month,
    );
    expect(
      afterRevocation.items.map((item) => item.circleId),
      isNot(contains(secondCircle.id)),
      reason: 'revoked membership removes that circle from personal history',
    );
    await _expectForbidden(
      () => schedules.listSchedules(
        token: student.token,
        sessionId: student.sessionId,
        circleId: secondCircle.id,
      ),
    );
    // Keep this variable referenced to make the recurrence fixture's identity
    // part of the assertions and to catch accidental fixture conflation.
    expect(secondCircleSeries.circleId, secondCircle.id);
  });

  testWidgets('T037: started recurrence displays completed history',
      (tester) async {
    const mediaEnabled = bool.fromEnvironment('T037_MEDIA_ENABLED');
    if (!mediaEnabled) {
      markTestSkipped(
        'T037_MEDIA_ENABLED dart-define absent; configure a LiveKit HTTPS/WSS '
        'endpoint before running start/end calendar assertions.',
      );
      return;
    }
    const webApiKey = String.fromEnvironment('T024_FIREBASE_WEB_API_KEY');
    if (webApiKey.isEmpty) {
      markTestSkipped(
        'T024_FIREBASE_WEB_API_KEY dart-define missing; provide the existing '
        'Firebase Identity Toolkit web API key fixture.',
      );
      return;
    }
    const baseUrl = String.fromEnvironment(
      'T024_API_BASE_URL',
      defaultValue: 'http://10.0.2.2:8080/api/v1',
    );
    final dio = Dio(BaseOptions(baseUrl: baseUrl));
    addTearDown(dio.close);
    final identity = _IdentityToolkitClient(webApiKey);
    final circles = CircleApiClient(dio);
    final schedules = ScheduleApiClient(dio);
    final calendar = CalendarApiClient(dio);
    final sessions = SessionApiClient(dio);
    final runId = DateTime.now().toUtc().microsecondsSinceEpoch;
    final teacher = await _disposableSession(
      dio,
      identity,
      email: 't037-lifecycle-$runId@example.com',
    );
    addTearDown(() => identity.deleteAccount(teacher.token));
    final circle = await circles.createCircle(
      firebaseIdToken: teacher.token,
      sessionId: teacher.sessionId,
      request: CreateCircleRequest(
        name: 'T037 Lifecycle $runId',
        language: 'ar',
        maxCapacity: 10,
      ),
    );
    addTearDown(() async {
      try {
        await circles.archiveCircle(
          firebaseIdToken: teacher.token,
          sessionId: teacher.sessionId,
          circleId: circle.id,
        );
      } on DioException {
        // Best-effort cleanup after assertions.
      }
    });
    final month = DateTime(DateTime.now().year, DateTime.now().month + 1);
    final date = _dateOnly(DateTime(month.year, month.month, 5));
    final series = await schedules.createSchedule(
      token: teacher.token,
      sessionId: teacher.sessionId,
      circleId: circle.id,
      plan: SchedulePlanInput(
        mode: ScheduleModes.selectedDates,
        anchorLocalDate: date,
        localStartTime: '10:00',
        localEndTime: '11:00',
        durationMinutes: 60,
        timezone: 'UTC',
        title: 'T037 Completed Recurrence',
        selectedDates: [date],
      ),
      idempotencyKey: 't037-lifecycle-series-$runId',
    );
    final startResponse = await dio.post<Map<String, dynamic>>(
      '/circles/${circle.id}/schedules/${series.id}/occurrences/$date/start',
      options: Options(headers: {
        'Authorization': 'Bearer ${teacher.token}',
        'X-Halaqaty-Session-ID': teacher.sessionId,
        'Idempotency-Key': 't037-lifecycle-start-$runId',
      }),
    );
    final sessionId = (startResponse.data!['session']
        as Map<String, dynamic>)['id'] as String;
    await sessions.end(
      token: teacher.token,
      sessionId: teacher.sessionId,
      liveSessionId: sessionId,
    );
    final result = await calendar.getMonth(
      token: teacher.token,
      sessionId: teacher.sessionId,
      month: month,
    );
    expect(
        _itemForOccurrence(result.items, series.id, date).state, 'completed');
  });
}

class _Session {
  const _Session(this.token, this.sessionId, this.userId);
  final String token;
  final String sessionId;
  final String userId;
}

Future<_Session> _disposableSession(
  Dio dio,
  _IdentityToolkitClient identity, {
  required String email,
}) async {
  final token = await identity.signUp(email: email);
  await dio.post<void>(
    '/auth/register',
    data: {'display_name': email.split('@').first},
    options: Options(headers: {'Authorization': 'Bearer $token'}),
  );
  final response = await dio.post<Map<String, dynamic>>(
    '/auth/sessions',
    data: const {'device_name': 't037-calendar-integration'},
    options: Options(headers: {'Authorization': 'Bearer $token'}),
  );
  final data = response.data!;
  return _Session(
    token,
    data['session_id'] as String,
    (data['user'] as Map<String, dynamic>)['id'] as String,
  );
}

class _IdentityToolkitClient {
  _IdentityToolkitClient(this._webApiKey);
  final String _webApiKey;
  final Dio _dio = Dio(
    BaseOptions(baseUrl: 'https://identitytoolkit.googleapis.com'),
  );

  Future<String> signUp({required String email}) => _token(
        '/v1/accounts:signUp',
        {
          'email': email,
          'password': 'T037!fixture${DateTime.now().microsecondsSinceEpoch}',
          'returnSecureToken': true,
        },
      );

  Future<void> deleteAccount(String token) async {
    try {
      await _dio.post<void>(
        '/v1/accounts:delete?key=$_webApiKey',
        data: {'idToken': token},
      );
    } on DioException {
      // Disposable identity cleanup is best effort.
    }
  }

  Future<String> _token(String path, Map<String, dynamic> body) async {
    final response = await _dio.post<Map<String, dynamic>>(
      '$path?key=$_webApiKey',
      data: body,
    );
    return response.data!['idToken'] as String;
  }
}

Future<void> _expectForbidden(Future<Object> Function() action) async {
  try {
    await action();
    fail('Expected revoked circle membership to be denied');
  } on DioException catch (error) {
    expect(error.response?.statusCode, anyOf(403, 404));
  }
}

String _dateOnly(DateTime date) => '${date.year.toString().padLeft(4, '0')}-'
    '${date.month.toString().padLeft(2, '0')}-'
    '${date.day.toString().padLeft(2, '0')}';

CalendarItem _itemForOccurrence(
  List<CalendarItem> items,
  String scheduleId,
  String localDate,
) =>
    items.singleWhere((item) => item.occurrenceKey == '$scheduleId:$localDate');
