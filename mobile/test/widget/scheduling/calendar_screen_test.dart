import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/scheduling/application/calendar_controller.dart';
import 'package:halaqaty_mobile/features/scheduling/data/calendar_api_client.dart';
import 'package:halaqaty_mobile/features/scheduling/presentation/calendar_screen.dart';

void main() {
  testWidgets('shows informational overlap details without hiding sessions',
      (tester) async {
    final dio = _calendarDio(
      [_item()],
      warnings: [
        {
          'warning_id': 'w-1',
          'first_circle_name': 'Tajweed Circle',
          'second_circle_name': 'Review Circle',
          'overlap_starts_at': '2026-10-06T15:30:00Z',
          'overlap_ends_at': '2026-10-06T16:00:00Z',
        }
      ],
    );
    await tester.pumpWidget(_buildScreen(dio));
    await tester.pumpAndSettle();

    expect(find.text('Schedule overlap'), findsOneWidget);
    expect(find.textContaining('Tajweed Circle'), findsWidgets);
    expect(find.textContaining('Review Circle'), findsOneWidget);
    expect(find.textContaining('18:30–19:00'), findsOneWidget);
    expect(find.byIcon(Icons.warning_amber_rounded), findsOneWidget);
    expect(find.byKey(const Key('calendarItem-sched-1:2026-10-06')),
        findsOneWidget);
  });

  testWidgets('shows an accessible overlap cue in Arabic RTL', (tester) async {
    final dio = _calendarDio(
      [_item()],
      warnings: [
        {
          'warning_id': 'w-1',
          'first_circle_name': 'حلقة التجويد',
          'second_circle_name': 'حلقة المراجعة',
          'overlap_starts_at': '2026-10-06T15:30:00Z',
          'overlap_ends_at': '2026-10-06T16:00:00Z',
        }
      ],
    );
    await tester.pumpWidget(_buildScreen(dio, rtl: true));
    await tester.pumpAndSettle();

    expect(find.text('تنبيه: تداخل في المواعيد'), findsOneWidget);
    expect(find.textContaining('حلقة التجويد'), findsOneWidget);
    expect(find.byIcon(Icons.warning_amber_rounded), findsOneWidget);
  });

  testWidgets('shows sessions from multiple circles with accessible identity',
      (tester) async {
    await tester.pumpWidget(_buildScreen(_calendarDio([
      _item(circleId: 'circle-1', circleName: 'Tajweed Circle'),
      _item(
        key: 'sched-2:2026-10-08',
        circleId: 'circle-2',
        circleName: 'Hifz Circle',
      ),
    ])));
    await tester.pumpAndSettle();

    expect(find.text('Tajweed Circle'), findsOneWidget);
    expect(find.text('Hifz Circle'), findsOneWidget);
    expect(find.bySemanticsLabel('Circle: Tajweed Circle'), findsOneWidget);
    expect(find.bySemanticsLabel('Circle: Hifz Circle'), findsOneWidget);
  });

  testWidgets('labels completed and cancelled sessions distinctly',
      (tester) async {
    await tester.pumpWidget(_buildScreen(_calendarDio([
      _item(state: 'completed', title: 'Completed review'),
      _item(
        key: 'one-off-1',
        state: 'cancelled',
        title: 'Cancelled review',
      ),
    ])));
    await tester.pumpAndSettle();

    expect(find.text('Completed review'), findsOneWidget);
    expect(find.text('Completed'), findsOneWidget);
    expect(find.text('Cancelled review'), findsOneWidget);
    expect(find.text('Cancelled'), findsOneWidget);
  });

  for (final rtl in [false, true]) {
    testWidgets('respects ${rtl ? 'RTL' : 'LTR'} layout direction',
        (tester) async {
      await tester.pumpWidget(_buildScreen(_calendarDio([_item()]), rtl: rtl));
      await tester.pumpAndSettle();

      expect(
        Directionality.of(tester.element(find.byType(CalendarScreen))),
        rtl ? TextDirection.rtl : TextDirection.ltr,
      );
      expect(find.text('Tajweed Circle'), findsOneWidget);
    });
  }

  testWidgets('shows retry after an offline error and recovers',
      (tester) async {
    var requests = 0;
    final dio = _calendarDio([_item()], onRequest: (options, handler) {
      requests++;
      if (requests == 1) {
        handler.reject(DioException(
          requestOptions: options,
          type: DioExceptionType.connectionError,
        ));
      } else {
        handler.resolve(Response<Map<String, dynamic>>(
          requestOptions: options,
          statusCode: 200,
          data: {
            'items': [_item()],
            'warnings': {'warnings': []}
          },
        ));
      }
    });
    await tester.pumpWidget(_buildScreen(dio));
    await tester.pumpAndSettle();

    expect(find.text('Try again'), findsOneWidget);
    await tester.tap(find.text('Try again'));
    await tester.pumpAndSettle();

    expect(requests, 2);
    expect(find.text('Tajweed Circle'), findsOneWidget);
  });

  testWidgets('navigates to earlier and later months', (tester) async {
    final requestedMonths = <String>[];
    final dio = _calendarDio([_item()], onRequest: (options, handler) {
      requestedMonths.add(options.queryParameters['month'] as String);
      handler.resolve(Response<Map<String, dynamic>>(
        requestOptions: options,
        statusCode: 200,
        data: {
          'items': [_item()],
          'warnings': {'warnings': []}
        },
      ));
    });
    await tester.pumpWidget(_buildScreen(dio));
    await tester.pumpAndSettle();

    expect(find.text('October 2026'), findsOneWidget);
    await tester.tap(find.byTooltip('Previous month'));
    await tester.pumpAndSettle();
    await tester.tap(find.byTooltip('Next month'));
    await tester.pumpAndSettle();

    expect(requestedMonths, ['2026-10', '2026-09', '2026-10']);
  });
}

Widget _buildScreen(Dio dio, {bool rtl = false}) => ProviderScope(
      overrides: [
        calendarApiClientProvider.overrideWithValue(CalendarApiClient(dio)),
        calendarControllerProvider.overrideWith((ref) => CalendarController(
              ref.watch(calendarApiClientProvider),
              () async => (token: 'firebase-token', sessionId: 'session-1'),
              timezone: 'Africa/Cairo',
              now: () => DateTime.utc(2026, 9, 30, 22, 30),
            )),
      ],
      child: MaterialApp(
        home: Directionality(
          textDirection: rtl ? TextDirection.rtl : TextDirection.ltr,
          child: const CalendarScreen(),
        ),
      ),
    );

Dio _calendarDio(
  List<Map<String, dynamic>> items, {
  List<Map<String, dynamic>> warnings = const [],
  void Function(RequestOptions, RequestInterceptorHandler)? onRequest,
}) {
  final dio = Dio(BaseOptions(baseUrl: 'http://localhost'));
  dio.interceptors.add(InterceptorsWrapper(
    onRequest: (options, handler) {
      if (onRequest != null) {
        onRequest(options, handler);
      } else {
        handler.resolve(Response<Map<String, dynamic>>(
          requestOptions: options,
          statusCode: 200,
          data: {
            'items': items,
            'warnings': {'warnings': warnings}
          },
        ));
      }
    },
  ));
  return dio;
}

Map<String, dynamic> _item({
  String key = 'sched-1:2026-10-06',
  String circleId = 'circle-1',
  String circleName = 'Tajweed Circle',
  String title = 'Circle Session',
  String state = 'scheduled',
}) =>
    {
      'occurrence_key': key,
      'session_id': null,
      'circle_id': circleId,
      'circle_name': circleName,
      'title': title,
      'starts_at': '2026-10-06T15:00:00Z',
      'ends_at': '2026-10-06T16:00:00Z',
      'planning_timezone': 'Africa/Cairo',
      'state': state,
    };
