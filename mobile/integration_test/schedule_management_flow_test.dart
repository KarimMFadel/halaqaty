import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/profile/data/profile_api_client.dart';
import 'package:halaqaty_mobile/features/scheduling/data/schedule_api_client.dart';
import 'package:integration_test/integration_test.dart';

/// T024 [US1]: teacher/supervisor end-to-end schedule create/edit/stop
/// journey against the real local backend, covering US1 acceptance
/// scenarios 1–5: multiple entries across recurrence modes, versioned
/// series change, single-occurrence edit/cancel, series stop, past-date
/// rejection, student read-only access, and local-time display data with
/// the viewer's stored profile timezone.
///
/// Fixtures arrive via `--dart-define` (the Android emulator cannot see
/// host environment variables):
///   T024_FIREBASE_WEB_API_KEY  Firebase Identity Toolkit web API key
///   T024_API_BASE_URL (optional; default http://10.0.2.2:8080/api/v1)
///
/// All three identities (teacher, supervisor, student) are disposable
/// Firebase accounts created by the test itself and deleted on teardown,
/// so shared fixture accounts' accumulated memberships cannot contaminate
/// the run (the active-membership budget is five circles per user).
void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets('T024: teacher/supervisor schedule management journey',
      (tester) async {
    const webApiKey = String.fromEnvironment('T024_FIREBASE_WEB_API_KEY');
    if (webApiKey.isEmpty) {
      markTestSkipped(
        'T024_FIREBASE_WEB_API_KEY dart-define missing; provide the '
        'Firebase Identity Toolkit web API key.',
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
    final profiles = ProfileApiClient(dio);

    // ── Bootstrap: three disposable identities, each provisioned through
    // the documented /auth/register + /auth/sessions fixture flow. ───────
    final runId = DateTime.now().toUtc().microsecondsSinceEpoch;
    final teacher = await _disposableSession(
      dio,
      identity,
      email: 't024-teacher-$runId@example.com',
      displayName: 'T024 Teacher $runId',
    );
    final supervisor = await _disposableSession(
      dio,
      identity,
      email: 't024-supervisor-$runId@example.com',
      displayName: 'T024 Supervisor $runId',
    );
    final student = await _disposableSession(
      dio,
      identity,
      email: 't024-student-$runId@example.com',
      displayName: 'T024 Student $runId',
    );
    for (final session in [teacher, supervisor, student]) {
      addTearDown(() => identity.deleteAccount(session.token));
    }

    // ── Circle fixture: teacher owns it, supervisor is the backup
    // supervisor, student joins by invite. ───────────────────────────────
    final circle = await circles.createCircle(
      firebaseIdToken: teacher.token,
      sessionId: teacher.sessionId,
      request: CreateCircleRequest(
        name: 'T024-schedule-$runId',
        language: 'ar',
        maxCapacity: 10,
        backupSupervisorUserId: supervisor.userId,
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
        // Best-effort cleanup; the assertions are already complete.
      }
    });
    await circles.joinCircleByInvite(
      firebaseIdToken: student.token,
      sessionId: student.sessionId,
      inviteCode: circle.inviteCode,
    );
    final members = await circles.listCircleMembers(
      firebaseIdToken: teacher.token,
      sessionId: teacher.sessionId,
      circleId: circle.id,
    );
    expect(
      members.singleWhere((m) => m.userId == supervisor.userId).role,
      CircleRole.supervisor,
      reason: 'backup supervisor must hold the supervisor role',
    );

    // Deterministic anchors: near-future dates derived only from "today".
    final today = _dateOnly(DateTime.now());
    final anchorA = _dateOnly(DateTime.now().add(const Duration(days: 1)));
    final anchorB = _dateOnly(DateTime.now().add(const Duration(days: 2)));
    final anchorD = _dateOnly(DateTime.now().add(const Duration(days: 3)));
    final selectedC = [
      _dateOnly(DateTime.now().add(const Duration(days: 4))),
      _dateOnly(DateTime.now().add(const Duration(days: 11))),
    ];
    final endB = _dateOnly(DateTime.now().add(const Duration(days: 30)));

    // ── Scenario 1 + 4 + 5: the teacher creates multiple entries across
    // recurrence modes, each retaining local times and planning zone. ────
    final weekly = await schedules.createSchedule(
      token: teacher.token,
      sessionId: teacher.sessionId,
      circleId: circle.id,
      plan: SchedulePlanInput(
        mode: ScheduleModes.weekdayPattern,
        anchorLocalDate: anchorA,
        localStartTime: '18:00',
        localEndTime: '19:00',
        durationMinutes: 60,
        timezone: 'Asia/Riyadh',
        title: 'T024 Weekly Hifz',
        weekCadence: 1,
        weekdays: [_dow(anchorA)],
      ),
      idempotencyKey: 't024-create-a-$runId',
    );
    expect(weekly.version, 1);
    expect(weekly.plan.mode, ScheduleModes.weekdayPattern);
    expect(weekly.plan.timezone, 'Asia/Riyadh');
    expect(weekly.plan.localStartTime, '18:00');

    final biweekly = await schedules.createSchedule(
      token: teacher.token,
      sessionId: teacher.sessionId,
      circleId: circle.id,
      plan: SchedulePlanInput(
        mode: ScheduleModes.weekdayPattern,
        anchorLocalDate: anchorB,
        localStartTime: '20:00',
        localEndTime: '21:30',
        durationMinutes: 90,
        timezone: 'Asia/Riyadh',
        title: 'T024 Biweekly Tajweed',
        weekCadence: 2,
        weekdays: [_dow(anchorB)],
        endLocalDate: endB,
      ),
      idempotencyKey: 't024-create-b-$runId',
    );
    expect(biweekly.plan.weekCadence, 2);
    expect(biweekly.plan.endLocalDate, endB);

    final selected = await schedules.createSchedule(
      token: teacher.token,
      sessionId: teacher.sessionId,
      circleId: circle.id,
      plan: SchedulePlanInput(
        mode: ScheduleModes.selectedDates,
        anchorLocalDate: selectedC.first,
        localStartTime: '17:00',
        localEndTime: '18:00',
        durationMinutes: 60,
        timezone: 'Africa/Cairo',
        title: 'T024 Selected Dates Revision',
        selectedDates: selectedC,
      ),
      idempotencyKey: 't024-create-c-$runId',
    );
    expect(selected.plan.selectedDates, selectedC);
    expect(selected.plan.timezone, 'Africa/Cairo');

    final interval = await schedules.createSchedule(
      token: teacher.token,
      sessionId: teacher.sessionId,
      circleId: circle.id,
      plan: SchedulePlanInput(
        mode: ScheduleModes.interval,
        anchorLocalDate: anchorD,
        localStartTime: '06:00',
        localEndTime: '07:00',
        durationMinutes: 60,
        timezone: 'UTC',
        title: 'T024 Interval Review',
        intervalCount: 10,
        intervalUnit: 'day',
      ),
      idempotencyKey: 't024-create-d-$runId',
    );
    expect(interval.plan.intervalCount, 10);
    expect(interval.plan.intervalUnit, 'day');

    final afterCreate = await schedules.listSchedules(
      token: teacher.token,
      sessionId: teacher.sessionId,
      circleId: circle.id,
    );
    expect(afterCreate, hasLength(4),
        reason: 'all four recurrence entries are listed for the circle');

    // ── FR-002: a past planned date is rejected as unprocessable. ───────
    final yesterday = _dateOnly(
      DateTime.now().subtract(const Duration(days: 1)),
    );
    await _expectStatus(
      () => schedules.createSchedule(
        token: teacher.token,
        sessionId: teacher.sessionId,
        circleId: circle.id,
        plan: SchedulePlanInput(
          mode: ScheduleModes.selectedDates,
          anchorLocalDate: yesterday,
          localStartTime: '10:00',
          localEndTime: '11:00',
          durationMinutes: 60,
          timezone: 'Asia/Riyadh',
          selectedDates: [yesterday],
        ),
        idempotencyKey: 't024-past-$runId',
      ),
      422,
    );

    // ── Scenario 2: the supervisor changes series A with a version check;
    // the other entries remain unchanged. ────────────────────────────────
    final revised = await schedules.changeSchedule(
      token: supervisor.token,
      sessionId: supervisor.sessionId,
      circleId: circle.id,
      scheduleId: weekly.id,
      expectedVersion: 1,
      effectiveLocalDate: today,
      plan: SchedulePlanInput(
        mode: ScheduleModes.weekdayPattern,
        anchorLocalDate: anchorA,
        localStartTime: '19:00',
        localEndTime: '20:00',
        durationMinutes: 60,
        timezone: 'Asia/Riyadh',
        title: 'T024 Weekly Hifz (revised)',
        weekCadence: 1,
        weekdays: [_dow(anchorA)],
      ),
      idempotencyKey: 't024-change-a-$runId',
    );
    expect(revised.version, 2);
    expect(revised.plan.localStartTime, '19:00');
    expect(revised.plan.title, 'T024 Weekly Hifz (revised)');

    // ── FR-013: edit one unstarted occurrence, then cancel another. ─────
    await schedules.changeOccurrence(
      token: supervisor.token,
      sessionId: supervisor.sessionId,
      circleId: circle.id,
      scheduleId: biweekly.id,
      localDate: anchorB,
      expectedSeriesVersion: biweekly.version,
      replacementLocalTime: '21:00',
      replacementEndLocalTime: '22:30',
      // The server accepts a clock replacement only as a full
      // start/end/duration triple.
      durationMinutes: 90,
      idempotencyKey: 't024-occ-b-$runId',
    );
    await schedules.changeOccurrence(
      token: supervisor.token,
      sessionId: supervisor.sessionId,
      circleId: circle.id,
      scheduleId: selected.id,
      localDate: selectedC.last,
      expectedSeriesVersion: selected.version,
      cancel: true,
      idempotencyKey: 't024-occ-c-$runId',
    );

    // ── Scenario 2 (stop): the teacher stops series A; the other active
    // entries remain unchanged. ──────────────────────────────────────────
    final stopped = await schedules.changeSchedule(
      token: teacher.token,
      sessionId: teacher.sessionId,
      circleId: circle.id,
      scheduleId: weekly.id,
      expectedVersion: revised.version,
      effectiveLocalDate: today,
      plan: revised.plan,
      stop: true,
      idempotencyKey: 't024-stop-a-$runId',
    );
    expect(stopped.stoppedFromLocalDate, today);

    final afterStop = await schedules.listSchedules(
      token: teacher.token,
      sessionId: teacher.sessionId,
      circleId: circle.id,
    );
    expect(afterStop, hasLength(4));
    final surviving = afterStop.singleWhere((s) => s.id == biweekly.id);
    expect(surviving.stoppedFromLocalDate, isNull);
    expect(surviving.plan.localStartTime, '20:00',
        reason: 'stopping one entry leaves the other entries unchanged');
    expect(surviving.version, biweekly.version);

    // ── Scenario 3 + FR-006/FR-012: the student reads the schedule with
    // their stored viewer timezone while the planning zone is preserved,
    // and every student mutation is denied. ──────────────────────────────
    // A fresh profile must complete full_name/country on its first update;
    // the timezone rides along with that first completion.
    final updatedProfile = await profiles.updateMe(
      firebaseIdToken: student.token,
      sessionId: student.sessionId,
      request: const UpdateProfileRequest(
        fullName: 'T024 Student',
        country: 'US',
        timezone: 'America/New_York',
      ),
    );
    expect(updatedProfile.timezone, 'America/New_York');
    final rereadProfile = await profiles.getMe(
      firebaseIdToken: student.token,
      sessionId: student.sessionId,
    );
    expect(rereadProfile.timezone, 'America/New_York',
        reason: 'the viewer timezone persists for local-time display');

    final studentView = await schedules.listSchedules(
      token: student.token,
      sessionId: student.sessionId,
      circleId: circle.id,
    );
    expect(studentView, hasLength(4));
    final studentWeekly = studentView.singleWhere((s) => s.id == weekly.id);
    expect(studentWeekly.stoppedFromLocalDate, today,
        reason: 'members see the stopped state of the revised series');
    expect(studentWeekly.plan.timezone, 'Asia/Riyadh');
    expect(studentWeekly.plan.localStartTime, '19:00',
        reason: 'local clock times and the planning zone are retained for '
            'display in the viewer context (FR-006)');
    final studentSelected = studentView.singleWhere((s) => s.id == selected.id);
    expect(studentSelected.plan.timezone, 'Africa/Cairo');

    await _expectStatus(
      () => schedules.createSchedule(
        token: student.token,
        sessionId: student.sessionId,
        circleId: circle.id,
        plan: SchedulePlanInput(
          mode: ScheduleModes.interval,
          anchorLocalDate: anchorD,
          localStartTime: '09:00',
          localEndTime: '10:00',
          durationMinutes: 60,
          timezone: 'UTC',
          intervalCount: 1,
          intervalUnit: 'week',
        ),
        idempotencyKey: 't024-student-create-$runId',
      ),
      403,
    );
    await _expectStatus(
      () => schedules.changeSchedule(
        token: student.token,
        sessionId: student.sessionId,
        circleId: circle.id,
        scheduleId: biweekly.id,
        expectedVersion: biweekly.version,
        effectiveLocalDate: today,
        plan: biweekly.plan,
        stop: true,
        idempotencyKey: 't024-student-stop-$runId',
      ),
      403,
    );

    // The denied student writes left the records unchanged (SC-005).
    final finalView = await schedules.listSchedules(
      token: teacher.token,
      sessionId: teacher.sessionId,
      circleId: circle.id,
    );
    expect(finalView, hasLength(4));
    expect(
      finalView.singleWhere((s) => s.id == biweekly.id).stoppedFromLocalDate,
      isNull,
    );
  });
}

class _Session {
  const _Session({
    required this.token,
    required this.sessionId,
    required this.userId,
  });

  final String token;
  final String sessionId;
  final String userId;
}

/// Creates a disposable Firebase identity, provisions it in Halaqaty, and
/// opens a backend session.
Future<_Session> _disposableSession(
  Dio dio,
  _IdentityToolkitClient identity, {
  required String email,
  required String displayName,
}) async {
  final token = await identity.signUp(email: email);
  await dio.post<void>(
    '/auth/register',
    data: {'display_name': displayName},
    options: Options(headers: {'Authorization': 'Bearer $token'}),
  );
  return _openBackendSession(dio, token);
}

Future<_Session> _openBackendSession(Dio dio, String token) async {
  final response = await dio.post<Map<String, dynamic>>(
    '/auth/sessions',
    data: const {'device_name': 't024-integration'},
    options: Options(headers: {'Authorization': 'Bearer $token'}),
  );
  final data = response.data!;
  return _Session(
    token: token,
    sessionId: data['session_id'] as String,
    userId: (data['user'] as Map<String, dynamic>)['id'] as String,
  );
}

/// Minimal Identity Toolkit REST client for test fixtures only.
class _IdentityToolkitClient {
  _IdentityToolkitClient(this._webApiKey);

  final String _webApiKey;
  final Dio _dio = Dio(
    BaseOptions(baseUrl: 'https://identitytoolkit.googleapis.com'),
  );

  Future<String> signUp({required String email}) =>
      _token('/v1/accounts:signUp', {
        'email': email,
        // Fixture password; the account is deleted on teardown.
        'password': 'T024!fixture${DateTime.now().microsecondsSinceEpoch}',
        'returnSecureToken': true,
      });

  Future<void> deleteAccount(String idToken) async {
    try {
      await _dio.post<void>(
        '/v1/accounts:delete?key=$_webApiKey',
        data: {'idToken': idToken},
      );
    } on DioException {
      // Best-effort cleanup of the disposable identity.
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

Future<void> _expectStatus(Future<Object> Function() action, int status) async {
  try {
    await action();
    fail('Expected the request to be rejected with HTTP $status');
  } on DioException catch (error) {
    expect(error.response?.statusCode, status);
  }
}

String _dateOnly(DateTime date) => '${date.year.toString().padLeft(4, '0')}-'
    '${date.month.toString().padLeft(2, '0')}-'
    '${date.day.toString().padLeft(2, '0')}';

/// Postgres DOW numbering used by the contract: 0 is Sunday … 6 Saturday.
int _dow(String localDate) => DateTime.parse(localDate).weekday % 7;
