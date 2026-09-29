import 'dart:convert';

import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/scheduling/data/calendar_api_client.dart';
import 'package:halaqaty_mobile/features/scheduling/data/schedule_api_client.dart';
import 'package:timezone/data/latest_all.dart' as tz_data;
import 'package:timezone/timezone.dart' as tz;

enum CalendarStatus { loading, ready, error }

class CalendarState {
  const CalendarState({
    this.status = CalendarStatus.loading,
    this.month,
    this.items = const [],
    this.warnings = const [],
    this.error,
  });

  final CalendarStatus status;
  final DateTime? month;
  final List<CalendarItem> items;
  final List<CalendarWarning> warnings;
  final CalendarFailure? error;

  CalendarState copyWith({
    CalendarStatus? status,
    DateTime? month,
    List<CalendarItem>? items,
    List<CalendarWarning>? warnings,
    CalendarFailure? error,
    bool clearError = false,
  }) =>
      CalendarState(
        status: status ?? this.status,
        month: month ?? this.month,
        items: items ?? this.items,
        warnings: warnings ?? this.warnings,
        error: clearError ? null : (error ?? this.error),
      );
}

enum CalendarFailure { network, permission, unknown }

typedef CalendarCredentials = ({String token, String sessionId});

bool _timeZonesInitialized = false;

void initializeCalendarTimeZones() {
  if (_timeZonesInitialized) return;
  tz_data.initializeTimeZones();
  _timeZonesInitialized = true;
}

tz.TZDateTime calendarTimeInZone(DateTime instant, String timezone) {
  initializeCalendarTimeZones();
  return tz.TZDateTime.from(instant.toUtc(), tz.getLocation(timezone));
}

class CalendarController extends StateNotifier<CalendarState> {
  CalendarController(
    this._api,
    this._credentials, {
    required this.timezone,
    DateTime Function()? now,
    Future<void> Function()? logout,
  })  : _now = now ?? DateTime.now,
        _logout = logout ?? _noop,
        super(const CalendarState());

  final CalendarApiClient _api;
  final Future<CalendarCredentials> Function() _credentials;
  final String timezone;
  final DateTime Function() _now;
  final Future<void> Function() _logout;

  static Future<void> _noop() async {}

  Future<void> load() async {
    final month = state.month ?? _currentMonth();
    await _loadMonth(month);
  }

  Future<void> previousMonth() async {
    final month = state.month ?? _currentMonth();
    await _loadMonth(DateTime(month.year, month.month - 1));
  }

  Future<void> nextMonth() async {
    final month = state.month ?? _currentMonth();
    await _loadMonth(DateTime(month.year, month.month + 1));
  }

  DateTime _currentMonth() {
    final localNow = calendarTimeInZone(_now(), timezone);
    return DateTime(localNow.year, localNow.month);
  }

  Future<void> _loadMonth(DateTime month) async {
    final requestedMonth = DateTime(month.year, month.month);
    final sameMonth =
        state.month?.year == month.year && state.month?.month == month.month;
    state = state.copyWith(
      status: CalendarStatus.loading,
      month: requestedMonth,
      items: sameMonth ? state.items : const [],
      warnings: sameMonth ? state.warnings : const [],
      clearError: true,
    );
    try {
      final credentials = await _credentials();
      final result = await _api.getMonth(
        token: credentials.token,
        sessionId: credentials.sessionId,
        month: month,
      );
      state = state.copyWith(
        status: CalendarStatus.ready,
        items: result.items,
        warnings: result.warnings,
        clearError: true,
      );
    } catch (error) {
      if (error is DioException && error.response?.statusCode == 401) {
        await _logout();
      }
      state = state.copyWith(
        status: CalendarStatus.error,
        error: _failure(error),
      );
    }
  }

  CalendarFailure _failure(Object error) {
    if (error is DioException) {
      final code = error.response?.statusCode;
      if (code == 401 || code == 403 || code == 404) {
        return CalendarFailure.permission;
      }
      if (code == null) return CalendarFailure.network;
    }
    return CalendarFailure.unknown;
  }
}

final calendarControllerProvider =
    StateNotifierProvider.autoDispose<CalendarController, CalendarState>((ref) {
  final auth = ref.watch(authControllerProvider);
  return CalendarController(
    ref.watch(calendarApiClientProvider),
    () async {
      final firebaseUser = ref.read(firebaseAuthProvider).currentUser;
      final sessionId = ref.read(authControllerProvider).sessionId;
      if (firebaseUser == null || sessionId == null) {
        throw StateError('Calendar session is unavailable.');
      }
      final token = await firebaseUser.getIdToken();
      if (token == null || token.isEmpty) {
        throw StateError('Calendar session is unavailable.');
      }
      return (token: token, sessionId: sessionId);
    },
    timezone: auth.user?.timezone ?? 'UTC',
    logout: () => ref.read(authControllerProvider.notifier).logout(),
  );
});

class OneOffState {
  const OneOffState({this.isSaving = false, this.failure});

  final bool isSaving;
  final CalendarFailure? failure;
}

class OneOffController extends StateNotifier<OneOffState> {
  OneOffController(this._api, this._credentials, this.circleId,
      {Future<void> Function()? logout})
      : _logout = logout ?? CalendarController._noop,
        super(const OneOffState());

  final CalendarApiClient _api;
  final Future<CalendarCredentials> Function() _credentials;
  final String circleId;
  final Future<void> Function() _logout;
  String? _pendingPayload;
  String? _pendingKey;
  static int _keyCounter = 0;

  Future<bool> create(SchedulePlanInput plan) async {
    state = const OneOffState(isSaving: true);
    try {
      final credentials = await _credentials();
      final payload = jsonEncode(plan.toJson());
      if (_pendingPayload != payload) {
        _pendingPayload = payload;
        _pendingKey =
            'one-off-${DateTime.now().toUtc().microsecondsSinceEpoch}-${++_keyCounter}';
      }
      await _api.createOneOff(
        token: credentials.token,
        sessionId: credentials.sessionId,
        circleId: circleId,
        plan: plan,
        idempotencyKey: _pendingKey!,
      );
      _pendingPayload = null;
      _pendingKey = null;
      state = const OneOffState();
      return true;
    } catch (error) {
      if (error is DioException && error.response?.statusCode == 401) {
        await _logout();
      }
      final failure = error is DioException && error.response?.statusCode == 403
          ? CalendarFailure.permission
          : error is DioException && error.response?.statusCode == null
              ? CalendarFailure.network
              : CalendarFailure.unknown;
      state = OneOffState(failure: failure);
      return false;
    }
  }
}

final oneOffControllerProvider = StateNotifierProvider.autoDispose
    .family<OneOffController, OneOffState, String>((ref, circleId) {
  return OneOffController(
    ref.watch(calendarApiClientProvider),
    () async {
      final firebaseUser = ref.read(firebaseAuthProvider).currentUser;
      final sessionId = ref.read(authControllerProvider).sessionId;
      if (firebaseUser == null || sessionId == null) {
        throw StateError('Calendar session is unavailable.');
      }
      final token = await firebaseUser.getIdToken();
      if (token == null || token.isEmpty) {
        throw StateError('Calendar session is unavailable.');
      }
      return (token: token, sessionId: sessionId);
    },
    circleId,
    logout: () => ref.read(authControllerProvider.notifier).logout(),
  );
});
