import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/auth/data/auth_api_client.dart';

/// Recurrence mode wire values of the F-006 `PlanInput` contract
/// (`weekday_pattern` with cadence 1/2 + weekdays, `interval` with a positive
/// count of days/weeks, `selected_dates`). `one_off` belongs to the planned
/// session surface (US2) and is intentionally not produced here.
abstract final class ScheduleModes {
  static const weekdayPattern = 'weekday_pattern';
  static const interval = 'interval';
  static const selectedDates = 'selected_dates';
}

/// The F-006 `PlanInput` request/response shape: local clock times and the
/// IANA planning timezone are retained as entered; the server resolves UTC
/// instants and DST rules.
class SchedulePlanInput {
  const SchedulePlanInput({
    required this.mode,
    required this.anchorLocalDate,
    required this.localStartTime,
    required this.localEndTime,
    required this.durationMinutes,
    required this.timezone,
    this.title,
    this.endLocalDate,
    this.weekCadence,
    this.weekdays,
    this.intervalCount,
    this.intervalUnit,
    this.selectedDates,
  });

  final String mode;
  final String anchorLocalDate;
  final String localStartTime;
  final String localEndTime;
  final int durationMinutes;
  final String timezone;
  final String? title;
  final String? endLocalDate;
  final int? weekCadence;
  final List<int>? weekdays;
  final int? intervalCount;
  final String? intervalUnit;
  final List<String>? selectedDates;

  Map<String, dynamic> toJson() => {
        'mode': mode,
        'anchor_local_date': anchorLocalDate,
        'local_start_time': localStartTime,
        'local_end_time': localEndTime,
        'duration_minutes': durationMinutes,
        'timezone': timezone,
        if (title != null) 'title': title,
        if (endLocalDate != null) 'end_local_date': endLocalDate,
        if (weekCadence != null) 'week_cadence': weekCadence,
        if (weekdays != null) 'weekdays': weekdays,
        if (intervalCount != null) 'interval_count': intervalCount,
        if (intervalUnit != null) 'interval_unit': intervalUnit,
        if (selectedDates != null) 'selected_dates': selectedDates,
      };

  factory SchedulePlanInput.fromJson(Map<String, dynamic> json) =>
      SchedulePlanInput(
        mode: json['mode'] as String,
        anchorLocalDate: json['anchor_local_date'] as String,
        localStartTime: json['local_start_time'] as String,
        localEndTime: json['local_end_time'] as String,
        durationMinutes: json['duration_minutes'] as int,
        timezone: json['timezone'] as String,
        title: json['title'] as String?,
        endLocalDate: json['end_local_date'] as String?,
        weekCadence: json['week_cadence'] as int?,
        weekdays: (json['weekdays'] as List<dynamic>?)
            ?.map((day) => day as int)
            .toList(growable: false),
        intervalCount: json['interval_count'] as int?,
        intervalUnit: json['interval_unit'] as String?,
        selectedDates: (json['selected_dates'] as List<dynamic>?)
            ?.map((date) => date as String)
            .toList(growable: false),
      );
}

/// The F-006 `Schedule` response: identity, optimistic-concurrency version
/// and the current plan revision.
class CircleScheduleEntry {
  const CircleScheduleEntry({
    required this.id,
    required this.circleId,
    required this.version,
    this.occurrenceVersions = const {},
    this.stoppedFromLocalDate,
    required this.plan,
  });

  final String id;
  final String circleId;
  final int version;

  /// Current exception versions by original local date. An absent date is a
  /// virtual occurrence and starts at version zero.
  final Map<String, int> occurrenceVersions;
  final String? stoppedFromLocalDate;
  final SchedulePlanInput plan;

  factory CircleScheduleEntry.fromJson(Map<String, dynamic> json) =>
      CircleScheduleEntry(
        id: json['id'] as String,
        circleId: json['circle_id'] as String,
        version: json['version'] as int,
        occurrenceVersions:
            (json['occurrence_versions'] as Map<String, dynamic>? ??
                    const <String, dynamic>{})
                .map((date, version) => MapEntry(date, version as int)),
        stoppedFromLocalDate: json['stopped_from_local_date'] as String?,
        plan: SchedulePlanInput.fromJson(json['plan'] as Map<String, dynamic>),
      );
}

/// Wraps the F-006 circle schedule REST endpoints. Every mutation carries a
/// caller-supplied `Idempotency-Key` so a retry never duplicates a write.
class ScheduleApiClient {
  ScheduleApiClient(this._dio);

  final Dio _dio;

  static const _idempotencyHeader = 'Idempotency-Key';

  Future<List<CircleScheduleEntry>> listSchedules({
    required String token,
    required String sessionId,
    required String circleId,
  }) async {
    final response = await _dio.get<Map<String, dynamic>>(
      '/circles/$circleId/schedules',
      options: Options(headers: _authHeaders(token, sessionId)),
    );
    return (response.data?['data'] as List<dynamic>? ?? const [])
        .whereType<Map<String, dynamic>>()
        .map(CircleScheduleEntry.fromJson)
        .toList(growable: false);
  }

  Future<CircleScheduleEntry> createSchedule({
    required String token,
    required String sessionId,
    required String circleId,
    required SchedulePlanInput plan,
    required String idempotencyKey,
    bool confirmOverlaps = false,
    List<String> confirmedWarningIDs = const [],
  }) async {
    final response = await _dio.post<Map<String, dynamic>>(
      '/circles/$circleId/schedules',
      data: {
        ...plan.toJson(),
        'confirm_overlaps': confirmOverlaps,
        'confirmed_warning_ids': confirmedWarningIDs,
      },
      options: Options(headers: {
        ..._authHeaders(token, sessionId),
        _idempotencyHeader: idempotencyKey,
      }),
    );
    return CircleScheduleEntry.fromJson(response.data!);
  }

  /// Changes or stops future occurrences of a series; `stop` retains the
  /// current plan in the body because the contract always requires it.
  Future<CircleScheduleEntry> changeSchedule({
    required String token,
    required String sessionId,
    required String circleId,
    required String scheduleId,
    required int expectedVersion,
    required String effectiveLocalDate,
    required SchedulePlanInput plan,
    bool stop = false,
    required String idempotencyKey,
    bool confirmOverlaps = false,
    List<String> confirmedWarningIDs = const [],
  }) async {
    final response = await _dio.patch<Map<String, dynamic>>(
      '/circles/$circleId/schedules/$scheduleId',
      data: {
        'expected_version': expectedVersion,
        'effective_local_date': effectiveLocalDate,
        'plan': plan.toJson(),
        'stop': stop,
        'confirm_overlaps': confirmOverlaps,
        'confirmed_warning_ids': confirmedWarningIDs,
      },
      options: Options(headers: {
        ..._authHeaders(token, sessionId),
        _idempotencyHeader: idempotencyKey,
      }),
    );
    return CircleScheduleEntry.fromJson(response.data!);
  }

  /// Edits or cancels one unstarted occurrence, identified by its original
  /// local date. The updated calendar item is not consumed yet (the month
  /// calendar lands with US2), so the response is acknowledged only.
  ///
  /// A clock replacement must send the full triple — start, end and
  /// [durationMinutes] — because the server rejects a partial clock patch.
  Future<void> changeOccurrence({
    required String token,
    required String sessionId,
    required String circleId,
    required String scheduleId,
    required String localDate,
    required int expectedSeriesVersion,
    int expectedOccurrenceVersion = 0,
    String? replacementLocalDate,
    String? replacementLocalTime,
    String? replacementEndLocalTime,
    int? durationMinutes,
    String? title,
    bool cancel = false,
    required String idempotencyKey,
    bool confirmOverlaps = false,
    List<String> confirmedWarningIDs = const [],
  }) async {
    await _dio.patch<Map<String, dynamic>>(
      '/circles/$circleId/schedules/$scheduleId/occurrences/$localDate',
      data: {
        'expected_series_version': expectedSeriesVersion,
        'expected_occurrence_version': expectedOccurrenceVersion,
        'cancelled': cancel,
        'confirm_overlaps': confirmOverlaps,
        'confirmed_warning_ids': confirmedWarningIDs,
        if (replacementLocalDate != null)
          'replacement_local_date': replacementLocalDate,
        if (replacementLocalTime != null)
          'replacement_local_time': replacementLocalTime,
        if (replacementEndLocalTime != null)
          'replacement_end_local_time': replacementEndLocalTime,
        if (durationMinutes != null) 'duration_minutes': durationMinutes,
        if (title != null) 'title': title,
      },
      options: Options(headers: {
        ..._authHeaders(token, sessionId),
        _idempotencyHeader: idempotencyKey,
      }),
    );
  }

  Map<String, String> _authHeaders(String token, String sessionId) => {
        'Authorization': 'Bearer $token',
        'X-Halaqaty-Session-ID': sessionId,
      };
}

final scheduleApiClientProvider = Provider<ScheduleApiClient>((ref) {
  return ScheduleApiClient(ref.watch(dioProvider));
});
