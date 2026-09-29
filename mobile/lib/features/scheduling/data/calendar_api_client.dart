import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/auth/data/auth_api_client.dart';
import 'package:halaqaty_mobile/features/scheduling/data/schedule_api_client.dart';

class CalendarItem {
  const CalendarItem({
    required this.occurrenceKey,
    required this.circleId,
    required this.circleName,
    required this.title,
    required this.startsAt,
    required this.endsAt,
    required this.planningTimezone,
    required this.state,
    this.sessionId,
    this.version,
  });

  final String occurrenceKey;
  final String? sessionId;
  final String circleId;
  final String circleName;
  final String title;
  final DateTime startsAt;
  final DateTime endsAt;
  final String planningTimezone;
  final String state;

  /// One-off planned-session version for `expected_version` edits; null for
  /// recurring occurrences.
  final int? version;

  factory CalendarItem.fromJson(Map<String, dynamic> json) => CalendarItem(
        occurrenceKey: json['occurrence_key'] as String,
        sessionId: json['session_id'] as String?,
        circleId: json['circle_id'] as String,
        circleName: json['circle_name'] as String,
        title: json['title'] as String,
        startsAt: DateTime.parse(json['starts_at'] as String).toUtc(),
        endsAt: DateTime.parse(json['ends_at'] as String).toUtc(),
        planningTimezone: json['planning_timezone'] as String,
        state: json['state'] as String,
        version: json['version'] as int?,
      );
}

class CalendarWarning {
  const CalendarWarning({
    required this.warningId,
    required this.firstCircleName,
    required this.secondCircleName,
  });

  final String warningId;
  final String firstCircleName;
  final String secondCircleName;

  factory CalendarWarning.fromJson(Map<String, dynamic> json) =>
      CalendarWarning(
        warningId: json['warning_id'] as String,
        firstCircleName: json['first_circle_name'] as String,
        secondCircleName: json['second_circle_name'] as String,
      );
}

class CalendarMonth {
  const CalendarMonth({required this.items, required this.warnings});

  final List<CalendarItem> items;
  final List<CalendarWarning> warnings;
}

class CalendarApiClient {
  CalendarApiClient(this._dio);

  final Dio _dio;

  Future<CalendarMonth> getMonth({
    required String token,
    required String sessionId,
    required DateTime month,
  }) async {
    final response = await _dio.get<Map<String, dynamic>>(
      '/calendar/me',
      queryParameters: {'month': _monthKey(month)},
      options: Options(headers: _authHeaders(token, sessionId)),
    );
    final body = response.data!;
    final warningsResult = body['warnings'] as Map<String, dynamic>;
    return CalendarMonth(
      items: (body['items'] as List<dynamic>)
          .whereType<Map<String, dynamic>>()
          .map(CalendarItem.fromJson)
          .toList(growable: false),
      warnings: (warningsResult['warnings'] as List<dynamic>)
          .whereType<Map<String, dynamic>>()
          .map(CalendarWarning.fromJson)
          .toList(growable: false),
    );
  }

  Future<CalendarItem> createOneOff({
    required String token,
    required String sessionId,
    required String circleId,
    required SchedulePlanInput plan,
    required String idempotencyKey,
  }) async {
    final response = await _dio.post<Map<String, dynamic>>(
      '/circles/$circleId/planned-sessions',
      data: plan.toJson(),
      options: Options(headers: {
        ..._authHeaders(token, sessionId),
        'Idempotency-Key': idempotencyKey,
      }),
    );
    return CalendarItem.fromJson(response.data!);
  }

  Map<String, String> _authHeaders(String token, String sessionId) => {
        'Authorization': 'Bearer $token',
        'X-Halaqaty-Session-ID': sessionId,
      };

  String _monthKey(DateTime month) =>
      '${month.year.toString().padLeft(4, '0')}-'
      '${month.month.toString().padLeft(2, '0')}';
}

final calendarApiClientProvider = Provider<CalendarApiClient>(
  (ref) => CalendarApiClient(ref.watch(dioProvider)),
);
