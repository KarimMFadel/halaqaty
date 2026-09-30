import 'dart:convert';

import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/scheduling/data/calendar_api_client.dart';
import 'package:halaqaty_mobile/features/scheduling/data/schedule_api_client.dart';

enum ScheduleListStatus { loading, ready, error }

/// Why the last list load failed, for the screen's error copy.
enum ScheduleFailure { network, permission, gone, unknown }

/// Why the last save was rejected, mapped to safe localized copy in the UI
/// (never the raw server body).
enum ScheduleSaveError {
  pastDate,
  validation,
  conflict,
  permission,
  network,
  unknown
}

/// Load/save state for one circle's schedule list. The last loaded list is
/// retained across failures so an offline reload shows stale data with a
/// label instead of blanking the screen.
class ScheduleEditorState {
  const ScheduleEditorState({
    this.status = ScheduleListStatus.loading,
    this.schedules = const <CircleScheduleEntry>[],
    this.failure,
    this.isSaving = false,
    this.saveSucceeded = false,
    this.saveError,
    this.overlapWarnings = const [],
  });

  final ScheduleListStatus status;
  final List<CircleScheduleEntry> schedules;
  final ScheduleFailure? failure;
  final bool isSaving;
  final bool saveSucceeded;
  final ScheduleSaveError? saveError;
  final List<CalendarWarning> overlapWarnings;

  ScheduleEditorState copyWith({
    ScheduleListStatus? status,
    List<CircleScheduleEntry>? schedules,
    ScheduleFailure? failure,
    bool clearFailure = false,
    bool? isSaving,
    bool? saveSucceeded,
    ScheduleSaveError? saveError,
    bool clearSaveError = false,
    List<CalendarWarning>? overlapWarnings,
    bool clearOverlapWarnings = false,
  }) =>
      ScheduleEditorState(
        status: status ?? this.status,
        schedules: schedules ?? this.schedules,
        failure: clearFailure ? null : (failure ?? this.failure),
        isSaving: isSaving ?? this.isSaving,
        saveSucceeded: saveSucceeded ?? this.saveSucceeded,
        saveError: clearSaveError ? null : (saveError ?? this.saveError),
        overlapWarnings: clearOverlapWarnings
            ? const []
            : (overlapWarnings ?? this.overlapWarnings),
      );
}

typedef ScheduleCredentials = ({String token, String sessionId});

/// Family key: the schedule list plus the viewer's manage rights, which gate
/// every mutation before any network call.
typedef ScheduleScope = ({String circleId, bool canManage});

/// Presentation controller for the F-006 US1 schedule manager: list, create,
/// series change/stop and single-occurrence edit/cancel over
/// [ScheduleApiClient]. Manager rights gate mutations locally (the server
/// stays authoritative); idempotency keys survive retries of the same
/// operation so a repeated submit cannot duplicate a write.
class ScheduleController extends StateNotifier<ScheduleEditorState> {
  ScheduleController(
    this._api,
    this._credentials, {
    required this.circleId,
    required this.canManage,
    required Future<void> Function() logout,
    DateTime Function()? now,
    String Function()? idempotencyKeyGenerator,
  })  : _logout = logout,
        _now = now ?? DateTime.now,
        _nextKey = idempotencyKeyGenerator ?? _defaultKey,
        super(const ScheduleEditorState());

  final ScheduleApiClient _api;
  final Future<ScheduleCredentials> Function() _credentials;
  final String circleId;
  final bool canManage;
  final Future<void> Function() _logout;
  final DateTime Function() _now;
  final String Function() _nextKey;

  /// One pending key per operation; reused across retries, cleared on success.
  final Map<String, String> _pendingKeys = {};

  static int _keyCounter = 0;

  static String _defaultKey() =>
      'schedule-${DateTime.now().toUtc().microsecondsSinceEpoch}-'
      '${++_keyCounter}';

  Future<void> load() async {
    state = state.copyWith(
      status: ScheduleListStatus.loading,
      clearFailure: true,
    );
    try {
      final credentials = await _credentials();
      final schedules = await _api.listSchedules(
        token: credentials.token,
        sessionId: credentials.sessionId,
        circleId: circleId,
      );
      state = state.copyWith(
        status: ScheduleListStatus.ready,
        schedules: schedules,
        clearFailure: true,
      );
    } catch (error) {
      await _logoutIfUnauthorized(error);
      state = state.copyWith(
        status: ScheduleListStatus.error,
        failure: _classifyLoad(error),
      );
    }
  }

  Future<bool> createSchedule(SchedulePlanInput plan,
      {bool confirmOverlaps = false,
      List<String> confirmedWarningIDs = const []}) async {
    if (_hasPastPlannedDate(plan)) return _rejectPastDate();
    // The pending idempotency key is scoped to the exact payload: a retry of
    // the same write replays the key, but a changed payload is a new write
    // and must never reuse it.
    return _mutate(
        'create:${jsonEncode([
              plan.toJson(),
              confirmOverlaps,
              confirmedWarningIDs
            ])}', (credentials, key) async {
      await _api.createSchedule(
        token: credentials.token,
        sessionId: credentials.sessionId,
        circleId: circleId,
        plan: plan,
        idempotencyKey: key,
        confirmOverlaps: confirmOverlaps,
        confirmedWarningIDs: confirmedWarningIDs,
      );
    });
  }

  /// Replaces the series plan from today's local date forward.
  Future<bool> changeSeries({
    required CircleScheduleEntry schedule,
    required SchedulePlanInput plan,
    bool confirmOverlaps = false,
    List<String> confirmedWarningIDs = const [],
  }) async {
    final effectiveLocalDate = _todayLocalDate();
    return _mutate(
        'series:${schedule.id}:${schedule.version}:${jsonEncode([
              plan.toJson(),
              effectiveLocalDate,
              confirmOverlaps,
              confirmedWarningIDs
            ])}', (credentials, key) async {
      await _api.changeSchedule(
        token: credentials.token,
        sessionId: credentials.sessionId,
        circleId: circleId,
        scheduleId: schedule.id,
        expectedVersion: schedule.version,
        effectiveLocalDate: effectiveLocalDate,
        plan: plan,
        idempotencyKey: key,
        confirmOverlaps: confirmOverlaps,
        confirmedWarningIDs: confirmedWarningIDs,
      );
    });
  }

  /// Stops future occurrences from today; started/completed history and the
  /// stored plan are retained.
  Future<bool> stopSeries({required CircleScheduleEntry schedule}) {
    final effectiveLocalDate = _todayLocalDate();
    return _mutate(
        'stop:${schedule.id}:${schedule.version}:$effectiveLocalDate',
        (credentials, key) async {
      await _api.changeSchedule(
        token: credentials.token,
        sessionId: credentials.sessionId,
        circleId: circleId,
        scheduleId: schedule.id,
        expectedVersion: schedule.version,
        effectiveLocalDate: effectiveLocalDate,
        plan: schedule.plan,
        stop: true,
        idempotencyKey: key,
      );
    });
  }

  /// Edits or cancels one unstarted occurrence, identified by its original
  /// local date. A clock replacement needs the full triple, so
  /// [durationMinutes] accompanies the replacement start/end times.
  Future<bool> changeOccurrence({
    required CircleScheduleEntry schedule,
    required String localDate,
    String? replacementLocalDate,
    String? replacementLocalTime,
    String? replacementEndLocalTime,
    int? durationMinutes,
    bool cancel = false,
    bool confirmOverlaps = false,
    List<String> confirmedWarningIDs = const [],
  }) async {
    if (localDate.compareTo(_todayLocalDate()) < 0) return _rejectPastDate();
    return _mutate(
      'occurrence:${schedule.id}:$localDate:${schedule.version}:${schedule.occurrenceVersions[localDate] ?? 0}:${jsonEncode([
            replacementLocalDate,
            replacementLocalTime,
            replacementEndLocalTime,
            durationMinutes,
            cancel,
            confirmOverlaps,
            confirmedWarningIDs
          ])}',
      (credentials, key) async {
        await _api.changeOccurrence(
          token: credentials.token,
          sessionId: credentials.sessionId,
          circleId: circleId,
          scheduleId: schedule.id,
          localDate: localDate,
          expectedSeriesVersion: schedule.version,
          expectedOccurrenceVersion:
              schedule.occurrenceVersions[localDate] ?? 0,
          replacementLocalDate: replacementLocalDate,
          replacementLocalTime: replacementLocalTime,
          replacementEndLocalTime: replacementEndLocalTime,
          durationMinutes: durationMinutes,
          cancel: cancel,
          idempotencyKey: key,
          confirmOverlaps: confirmOverlaps,
          confirmedWarningIDs: confirmedWarningIDs,
        );
      },
    );
  }

  Future<bool> _mutate(
    String operation,
    Future<void> Function(ScheduleCredentials credentials, String key) call,
  ) async {
    if (!canManage) {
      state = state.copyWith(
        saveError: ScheduleSaveError.permission,
        saveSucceeded: false,
      );
      return false;
    }
    if (state.isSaving) return false;
    state = state.copyWith(
      isSaving: true,
      clearSaveError: true,
      clearOverlapWarnings: true,
      saveSucceeded: false,
    );
    ScheduleCredentials credentials;
    try {
      credentials = await _credentials();
      await call(credentials, _pendingKeys[operation] ??= _nextKey());
      _pendingKeys.remove(operation);
    } catch (error) {
      await _logoutIfUnauthorized(error);
      state = state.copyWith(
        isSaving: false,
        saveSucceeded: false,
        saveError: _classifySave(error),
        overlapWarnings:
            error is DioException && error.response?.statusCode == 409
                ? calendarWarningsFromConflict(error.response?.data)
                : const [],
      );
      return false;
    }
    // The write succeeded; a failed refresh below only degrades the list.
    state = state.copyWith(isSaving: false, saveSucceeded: true);
    await _refresh(credentials);
    return true;
  }

  Future<void> _refresh(ScheduleCredentials credentials) async {
    try {
      final schedules = await _api.listSchedules(
        token: credentials.token,
        sessionId: credentials.sessionId,
        circleId: circleId,
      );
      state = state.copyWith(
        status: ScheduleListStatus.ready,
        schedules: schedules,
        clearFailure: true,
      );
    } catch (error) {
      await _logoutIfUnauthorized(error);
      state = state.copyWith(
        status: ScheduleListStatus.error,
        failure: _classifyLoad(error),
      );
    }
  }

  bool _rejectPastDate() {
    state = state.copyWith(
      saveError: ScheduleSaveError.pastDate,
      saveSucceeded: false,
    );
    return false;
  }

  bool _hasPastPlannedDate(SchedulePlanInput plan) {
    final today = _todayLocalDate();
    if (plan.anchorLocalDate.compareTo(today) < 0) {
      return true;
    }
    final selectedDates = plan.selectedDates;
    if (selectedDates != null) {
      for (final date in selectedDates) {
        if (date.compareTo(today) < 0) return true;
      }
    }
    return false;
  }

  String _todayLocalDate() {
    final now = _now();
    return '${now.year.toString().padLeft(4, '0')}-'
        '${now.month.toString().padLeft(2, '0')}-'
        '${now.day.toString().padLeft(2, '0')}';
  }

  Future<void> _logoutIfUnauthorized(Object error) async {
    if (error is DioException && error.response?.statusCode == 401) {
      await _logout();
    }
  }

  static ScheduleFailure _classifyLoad(Object error) {
    if (error is DioException) {
      final code = error.response?.statusCode;
      if (code == 401 || code == 403) return ScheduleFailure.permission;
      if (code == 404) return ScheduleFailure.gone;
      if (error.type == DioExceptionType.connectionError ||
          error.type == DioExceptionType.connectionTimeout) {
        return ScheduleFailure.network;
      }
    }
    return ScheduleFailure.unknown;
  }

  static ScheduleSaveError _classifySave(Object error) {
    if (error is DioException) {
      final code = error.response?.statusCode;
      if (code == 401 || code == 403) return ScheduleSaveError.permission;
      if (code == 409) return ScheduleSaveError.conflict;
      if (code == 400 || code == 422) return ScheduleSaveError.validation;
      if (error.type == DioExceptionType.connectionError ||
          error.type == DioExceptionType.connectionTimeout) {
        return ScheduleSaveError.network;
      }
    }
    return ScheduleSaveError.unknown;
  }
}

final scheduleControllerProvider = StateNotifierProvider.family<
    ScheduleController, ScheduleEditorState, ScheduleScope>((ref, scope) {
  return ScheduleController(
    ref.watch(scheduleApiClientProvider),
    () async {
      // Read lazily per call: watching auth eagerly would build the real
      // Firebase-backed providers for any consumer of this screen.
      final auth = ref.read(authControllerProvider);
      final user = ref.read(firebaseAuthProvider).currentUser;
      final sessionId = auth.sessionId;
      final token = await user?.getIdToken();
      if (token == null ||
          token.isEmpty ||
          sessionId == null ||
          sessionId.isEmpty) {
        throw StateError('User not authenticated');
      }
      return (token: token, sessionId: sessionId);
    },
    circleId: scope.circleId,
    canManage: scope.canManage,
    logout: () => ref.read(authControllerProvider.notifier).logout(),
  );
});
