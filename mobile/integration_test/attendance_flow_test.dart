import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/attendance/data/attendance_api_client.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/scheduling/data/schedule_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_api_client.dart';
import 'package:integration_test/integration_test.dart';

/// T048 [US3]: planned/ad-hoc attendance, role denial and archived history.
///
/// Supply the disposable Firebase fixture using `T024_FIREBASE_WEB_API_KEY`;
/// optionally override the local API with `T024_API_BASE_URL`.
void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets('T048: attendance lifecycle, correction, role and archive',
      (tester) async {
    const webApiKey = String.fromEnvironment('T024_FIREBASE_WEB_API_KEY');
    if (webApiKey.isEmpty) {
      markTestSkipped(
        'T024_FIREBASE_WEB_API_KEY dart-define missing; supply the existing '
        'Firebase Identity Toolkit fixture to run T048.',
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
    final sessions = SessionApiClient(dio);
    final schedules = ScheduleApiClient(dio);
    final attendance = AttendanceApiClient(dio);
    final runId = DateTime.now().toUtc().microsecondsSinceEpoch;
    final teacher = await _disposableSession(dio, identity, 'teacher', runId);
    final firstStudent =
        await _disposableSession(dio, identity, 'student-a', runId);
    final secondStudent =
        await _disposableSession(dio, identity, 'student-b', runId);
    for (final account in [teacher, firstStudent, secondStudent]) {
      addTearDown(() => identity.deleteAccount(account.token));
    }

    final circle = await circles.createCircle(
      firebaseIdToken: teacher.token,
      sessionId: teacher.sessionId,
      request: CreateCircleRequest(
        name: 'T048 Attendance $runId',
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
        // Cleanup is best effort after assertions finish.
      }
    });
    for (final student in [firstStudent, secondStudent]) {
      await circles.joinCircleByInvite(
        firebaseIdToken: student.token,
        sessionId: student.sessionId,
        inviteCode: circle.inviteCode,
      );
    }

    final adHoc = await sessions.create(
      token: teacher.token,
      sessionId: teacher.sessionId,
      circleId: circle.id,
    );
    await sessions.start(
      token: teacher.token,
      sessionId: teacher.sessionId,
      liveSessionId: adHoc.id,
    );
    await sessions.join(
      token: firstStudent.token,
      sessionId: firstStudent.sessionId,
      liveSessionId: adHoc.id,
    );
    await sessions.end(
      token: teacher.token,
      sessionId: teacher.sessionId,
      liveSessionId: adHoc.id,
    );

    final adHocRoster = await attendance.list(
      token: teacher.token,
      sessionId: teacher.sessionId,
      liveSessionId: adHoc.id,
    );
    expect(adHocRoster.map((record) => record.userId),
        containsAll([firstStudent.userId, secondStudent.userId]));
    expect(
      adHocRoster
          .singleWhere((record) => record.userId == secondStudent.userId)
          .status,
      AttendanceStatus.absent,
    );
    final correction = await attendance.correct(
      token: teacher.token,
      sessionId: teacher.sessionId,
      liveSessionId: adHoc.id,
      userId: firstStudent.userId,
      status: AttendanceStatus.excused,
      reason: 'T048 verified absence',
      idempotencyKey: 't048-correct-$runId',
    );
    expect(correction.status, AttendanceStatus.excused);
    expect(correction.source, 'manual');
    expect(correction.correction?.actorId, teacher.userId);
    expect(correction.correction?.reason, 'T048 verified absence');

    final studentRoster = await attendance.list(
      token: firstStudent.token,
      sessionId: firstStudent.sessionId,
      liveSessionId: adHoc.id,
    );
    expect(studentRoster.map((record) => record.userId), [firstStudent.userId]);
    await _expectDenied(() => attendance.correct(
          token: firstStudent.token,
          sessionId: firstStudent.sessionId,
          liveSessionId: adHoc.id,
          userId: secondStudent.userId,
          status: AttendanceStatus.present,
          reason: 'Unauthorized correction',
          idempotencyKey: 't048-denied-$runId',
        ));

    final plannedDate = _dateOnly(DateTime.now().add(const Duration(days: 1)));
    final schedule = await schedules.createSchedule(
      token: teacher.token,
      sessionId: teacher.sessionId,
      circleId: circle.id,
      plan: SchedulePlanInput(
        mode: ScheduleModes.selectedDates,
        anchorLocalDate: plannedDate,
        selectedDates: [plannedDate],
        localStartTime: '09:00',
        localEndTime: '10:00',
        durationMinutes: 60,
        timezone: 'Africa/Cairo',
        title: 'T048 Planned Attendance',
      ),
      idempotencyKey: 't048-schedule-$runId',
    );
    final started = await dio.post<Map<String, dynamic>>(
      '/circles/${circle.id}/schedules/${schedule.id}/occurrences/$plannedDate/start',
      options: Options(headers: {
        'Authorization': 'Bearer ${teacher.token}',
        'X-Halaqaty-Session-ID': teacher.sessionId,
        'Idempotency-Key': 't048-start-$runId',
      }),
    );
    final plannedSessionId =
        (started.data!['session'] as Map<String, dynamic>)['id'] as String;
    await sessions.join(
      token: firstStudent.token,
      sessionId: firstStudent.sessionId,
      liveSessionId: plannedSessionId,
    );
    await sessions.end(
      token: teacher.token,
      sessionId: teacher.sessionId,
      liveSessionId: plannedSessionId,
    );
    expect(
      (await attendance.list(
        token: teacher.token,
        sessionId: teacher.sessionId,
        liveSessionId: plannedSessionId,
      ))
          .singleWhere((record) => record.userId == firstStudent.userId)
          .status,
      AttendanceStatus.present,
    );

    await circles.archiveCircle(
      firebaseIdToken: teacher.token,
      sessionId: teacher.sessionId,
      circleId: circle.id,
    );
    expect(
      (await attendance.list(
        token: teacher.token,
        sessionId: teacher.sessionId,
        liveSessionId: adHoc.id,
      ))
          .map((record) => record.userId),
      contains(firstStudent.userId),
    );
    await _expectDenied(() => attendance.correct(
          token: teacher.token,
          sessionId: teacher.sessionId,
          liveSessionId: adHoc.id,
          userId: firstStudent.userId,
          status: AttendanceStatus.present,
          reason: 'Archived correction must be denied',
          idempotencyKey: 't048-archived-$runId',
        ));
  });
}

class _Account {
  const _Account(this.token, this.sessionId, this.userId);
  final String token;
  final String sessionId;
  final String userId;
}

Future<_Account> _disposableSession(
  Dio dio,
  _IdentityToolkitClient identity,
  String label,
  int runId,
) async {
  final email = 't048-$label-$runId@example.com';
  final token = await identity.signUp(email: email);
  await dio.post<void>(
    '/auth/register',
    data: {'display_name': email.split('@').first},
    options: Options(headers: {'Authorization': 'Bearer $token'}),
  );
  final response = await dio.post<Map<String, dynamic>>(
    '/auth/sessions',
    data: const {'device_name': 't048-attendance-integration'},
    options: Options(headers: {'Authorization': 'Bearer $token'}),
  );
  final body = response.data!;
  return _Account(
    token,
    body['session_id'] as String,
    (body['user'] as Map<String, dynamic>)['id'] as String,
  );
}

class _IdentityToolkitClient {
  _IdentityToolkitClient(this._webApiKey);
  final String _webApiKey;
  final Dio _dio = Dio(
    BaseOptions(baseUrl: 'https://identitytoolkit.googleapis.com'),
  );

  Future<String> signUp({required String email}) async {
    final response = await _dio.post<Map<String, dynamic>>(
      '/v1/accounts:signUp?key=$_webApiKey',
      data: {
        'email': email,
        'password': 'T048!fixture${DateTime.now().microsecondsSinceEpoch}',
        'returnSecureToken': true,
      },
    );
    return response.data!['idToken'] as String;
  }

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
}

Future<void> _expectDenied(Future<Object> Function() action) async {
  try {
    await action();
    fail('Expected attendance mutation to be denied');
  } on DioException catch (error) {
    expect(error.response?.statusCode, anyOf(403, 404));
  }
}

String _dateOnly(DateTime date) => '${date.year.toString().padLeft(4, '0')}-'
    '${date.month.toString().padLeft(2, '0')}-'
    '${date.day.toString().padLeft(2, '0')}';
