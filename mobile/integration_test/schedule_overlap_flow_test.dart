import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/scheduling/data/schedule_api_client.dart';
import 'package:integration_test/integration_test.dart';

/// T057: verify overlap preview, explicit manager confirmation, half-open
/// touching intervals and privacy across circles against the live API.
void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets('F-006 overlap preview and write confirmation', (tester) async {
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
    final runId = DateTime.now().toUtc().microsecondsSinceEpoch;
    final manager = await _disposableSession(
      dio,
      identity,
      email: 'f006-overlap-manager-$runId@example.com',
      displayName: 'F006 Overlap Manager $runId',
    );
    final privateManager = await _disposableSession(
      dio,
      identity,
      email: 'f006-overlap-private-$runId@example.com',
      displayName: 'F006 Private Manager $runId',
    );
    addTearDown(() => identity.deleteAccount(manager.token));
    addTearDown(() => identity.deleteAccount(privateManager.token));

    final circle = await circles.createCircle(
      firebaseIdToken: manager.token,
      sessionId: manager.sessionId,
      request: CreateCircleRequest(
        name: 'F006 overlap $runId',
        language: 'ar',
        maxCapacity: 10,
      ),
    );
    final privateCircle = await circles.createCircle(
      firebaseIdToken: privateManager.token,
      sessionId: privateManager.sessionId,
      request: CreateCircleRequest(
        name: 'F006 private $runId',
        language: 'ar',
        maxCapacity: 10,
      ),
    );
    addTearDown(() async {
      for (final value in [
        (circle: circle, session: manager),
        (circle: privateCircle, session: privateManager),
      ]) {
        try {
          await circles.archiveCircle(
            firebaseIdToken: value.session.token,
            sessionId: value.session.sessionId,
            circleId: value.circle.id,
          );
        } on DioException {
          // Cleanup is best effort after assertions complete.
        }
      }
    });

    final date = _dateOnly(DateTime.now().add(const Duration(days: 3)));
    final existingPlan = _selectedPlan(date, '10:00', '11:00');
    await schedules.createSchedule(
      token: manager.token,
      sessionId: manager.sessionId,
      circleId: circle.id,
      plan: existingPlan,
      idempotencyKey: 'f006-overlap-existing-$runId',
    );

    final overlapPlan = _selectedPlan(date, '10:30', '11:30');
    final preview = await _preview(
      dio,
      manager,
      circle.id,
      overlapPlan,
    );
    expect(preview, hasLength(1));
    expect(preview.single['first_circle_name'], 'F006 overlap $runId');
    final warningID = preview.single['warning_id'] as String;
    final savedOverlap = await schedules.createSchedule(
      token: manager.token,
      sessionId: manager.sessionId,
      circleId: circle.id,
      plan: overlapPlan,
      idempotencyKey: 'f006-overlap-confirmed-$runId',
      confirmOverlaps: true,
      confirmedWarningIDs: [warningID],
    );
    expect(savedOverlap.version, 1);

    final touchingPlan = _selectedPlan(date, '11:30', '12:30');
    expect(await _preview(dio, manager, circle.id, touchingPlan), isEmpty);
    await schedules.createSchedule(
      token: manager.token,
      sessionId: manager.sessionId,
      circleId: circle.id,
      plan: touchingPlan,
      idempotencyKey: 'f006-overlap-touching-$runId',
    );

    final privateDate = _dateOnly(DateTime.now().add(const Duration(days: 4)));
    await schedules.createSchedule(
      token: privateManager.token,
      sessionId: privateManager.sessionId,
      circleId: privateCircle.id,
      plan: _selectedPlan(privateDate, '10:00', '11:00'),
      idempotencyKey: 'f006-overlap-private-$runId',
    );
    final privatePreview = await _preview(
      dio,
      manager,
      circle.id,
      _selectedPlan(privateDate, '10:30', '11:30'),
    );
    expect(privatePreview, isEmpty);
    expect(privatePreview.toString(), isNot(contains('F006 private $runId')));
    await schedules.createSchedule(
      token: manager.token,
      sessionId: manager.sessionId,
      circleId: circle.id,
      plan: _selectedPlan(privateDate, '10:30', '11:30'),
      idempotencyKey: 'f006-overlap-private-safe-$runId',
    );
  });
}

Future<List<Map<String, dynamic>>> _preview(
  Dio dio,
  _Session session,
  String circleId,
  SchedulePlanInput plan,
) async {
  final response = await dio.post<Map<String, dynamic>>(
    '/circles/$circleId/planning-preview',
    data: {...plan.toJson(), 'confirm_overlaps': false},
    options: Options(headers: {
      'Authorization': 'Bearer ${session.token}',
      'X-Halaqaty-Session-ID': session.sessionId,
    }),
  );
  final warnings = response.data!['warnings'] as List<dynamic>;
  return warnings.whereType<Map<String, dynamic>>().toList(growable: false);
}

SchedulePlanInput _selectedPlan(String date, String start, String end) =>
    SchedulePlanInput(
      mode: ScheduleModes.selectedDates,
      anchorLocalDate: date,
      localStartTime: start,
      localEndTime: end,
      durationMinutes: _clockMinutes(end) - _clockMinutes(start),
      timezone: 'UTC',
      title: 'F006 overlap check',
      selectedDates: [date],
    );

int _clockMinutes(String value) {
  final parts = value.split(':').map(int.parse).toList();
  return parts[0] * 60 + parts[1];
}

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
  final response = await dio.post<Map<String, dynamic>>(
    '/auth/sessions',
    data: const {'device_name': 'f006-overlap-integration'},
    options: Options(headers: {'Authorization': 'Bearer $token'}),
  );
  final data = response.data!;
  return _Session(
    token: token,
    sessionId: data['session_id'] as String,
  );
}

String _dateOnly(DateTime date) => '${date.year.toString().padLeft(4, '0')}-'
    '${date.month.toString().padLeft(2, '0')}-'
    '${date.day.toString().padLeft(2, '0')}';

class _Session {
  const _Session({required this.token, required this.sessionId});

  final String token;
  final String sessionId;
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
        'password': 'F006!fixture${DateTime.now().microsecondsSinceEpoch}',
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
      // Fixture cleanup is best effort after the API assertions complete.
    }
  }
}
