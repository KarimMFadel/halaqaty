import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/attendance/application/attendance_controller.dart';
import 'package:halaqaty_mobile/features/attendance/data/attendance_api_client.dart';
import 'package:halaqaty_mobile/features/attendance/presentation/attendance_screen.dart';

void main() {
  testWidgets('student reads own attendance without correction controls',
      (tester) async {
    final controller = _controller(canCorrect: false);
    await tester.pumpWidget(_app(controller, canCorrect: false));
    await tester.pumpAndSettle();

    expect(find.text('student-1'), findsOneWidget);
    expect(find.text('Present'), findsOneWidget);
    expect(find.byKey(const Key('attendanceCorrect-student-1')), findsNothing);
  });

  testWidgets('teacher can open an audited correction dialog', (tester) async {
    final controller = _controller(canCorrect: true);
    await tester.pumpWidget(_app(controller, canCorrect: true));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('attendanceCorrect-student-1')));
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('attendanceReason')), findsOneWidget);
    expect(find.byKey(const Key('attendanceSaveCorrection')), findsOneWidget);
    expect(
        tester
            .widget<FilledButton>(
              find.byKey(const Key('attendanceSaveCorrection')),
            )
            .onPressed,
        isNull);
    await tester.enterText(
        find.byKey(const Key('attendanceReason')), 'Verified');
    await tester.pump();
    expect(
        tester
            .widget<FilledButton>(
              find.byKey(const Key('attendanceSaveCorrection')),
            )
            .onPressed,
        isNotNull);
  });

  testWidgets('attendance screen renders in Arabic RTL', (tester) async {
    final controller = _controller(canCorrect: false);
    await tester.pumpWidget(_app(controller, canCorrect: false, rtl: true));
    await tester.pumpAndSettle();
    expect(find.text('الحضور'), findsOneWidget);
    expect(find.text('حاضر'), findsOneWidget);
  });

  testWidgets('empty attendance has explicit empty state', (tester) async {
    final controller = _controller(canCorrect: false, records: const []);
    await tester.pumpWidget(_app(controller, canCorrect: false));
    await tester.pumpAndSettle();
    expect(find.text('No attendance records'), findsOneWidget);
  });

  testWidgets('failed attendance load offers retry', (tester) async {
    final controller = _controller(canCorrect: false, fail: true);
    await tester.pumpWidget(_app(controller, canCorrect: false));
    await tester.pumpAndSettle();
    expect(find.text('Unable to load attendance'), findsOneWidget);
    expect(find.byKey(const Key('attendanceRetry')), findsOneWidget);
  });
}

Widget _app(AttendanceController controller,
        {required bool canCorrect, bool rtl = false}) =>
    ProviderScope(
      overrides: [
        attendanceControllerProvider((
          sessionId: 'session-1',
          canCorrect: canCorrect,
        )).overrideWith((_) => controller),
      ],
      child: MaterialApp(
        home: Directionality(
          textDirection: rtl ? TextDirection.rtl : TextDirection.ltr,
          child: AttendanceScreen(
            sessionId: 'session-1',
            canCorrect: canCorrect,
          ),
        ),
      ),
    );

AttendanceController _controller({
  required bool canCorrect,
  List<Map<String, dynamic>>? records,
  bool fail = false,
}) {
  final dio = Dio(BaseOptions(baseUrl: 'http://localhost'))
    ..interceptors.add(InterceptorsWrapper(onRequest: (options, handler) {
      if (fail) {
        handler.reject(DioException(
          requestOptions: options,
          response: Response<dynamic>(requestOptions: options, statusCode: 500),
        ));
      } else {
        handler.resolve(Response<List<dynamic>>(
          requestOptions: options,
          statusCode: 200,
          data: options.method == 'GET'
              ? (records ?? [_recordJson])
              : [_recordJson],
        ));
      }
    }));
  return AttendanceController(
    AttendanceApiClient(dio),
    () async => (token: 'token', sessionId: 'auth-session'),
    'session-1',
    canCorrect: canCorrect,
    idempotencyKeyGenerator: () => 'correction-key',
  );
}

final _recordJson = <String, dynamic>{
  'session_id': 'session-1',
  'user_id': 'student-1',
  'status': 'present',
  'source': 'automatic',
  'first_presence_at': null,
  'correction_id': null,
  'correction': null,
};
