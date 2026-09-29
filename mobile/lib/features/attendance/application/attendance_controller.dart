import 'dart:convert';

import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/attendance/data/attendance_api_client.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';

enum AttendanceViewStatus { loading, ready, error }

enum AttendanceFailure { network, permission, unknown }

class AttendanceState {
  const AttendanceState({
    this.status = AttendanceViewStatus.loading,
    this.records = const [],
    this.failure,
    this.savingUserId,
  });

  final AttendanceViewStatus status;
  final List<AttendanceRecord> records;
  final AttendanceFailure? failure;
  final String? savingUserId;
}

typedef AttendanceCredentials = ({String token, String sessionId});

class AttendanceController extends StateNotifier<AttendanceState> {
  AttendanceController(this._api, this._credentials, this.sessionId,
      {required this.canCorrect,
      Future<void> Function()? logout,
      String Function()? idempotencyKeyGenerator})
      : _logout = logout ?? _noop,
        _newKey = idempotencyKeyGenerator ?? _generateKey,
        super(const AttendanceState());

  final AttendanceApiClient _api;
  final Future<AttendanceCredentials> Function() _credentials;
  final String sessionId;
  final bool canCorrect;
  final Future<void> Function() _logout;
  final String Function() _newKey;
  static int _keyCounter = 0;
  ({String payload, String key})? _pending;

  static Future<void> _noop() async {}
  static String _generateKey() =>
      'attendance-${DateTime.now().toUtc().microsecondsSinceEpoch}-${++_keyCounter}';

  Future<void> load() async {
    state = AttendanceState(
      status: AttendanceViewStatus.loading,
      records: state.records,
    );
    try {
      final credentials = await _credentials();
      final records = await _api.list(
        token: credentials.token,
        sessionId: credentials.sessionId,
        liveSessionId: sessionId,
      );
      state =
          AttendanceState(status: AttendanceViewStatus.ready, records: records);
    } catch (error) {
      await _handleUnauthorized(error);
      state = AttendanceState(
        status: AttendanceViewStatus.error,
        records: state.records,
        failure: _failure(error),
      );
    }
  }

  Future<bool> correct({
    required String userId,
    required AttendanceStatus status,
    required String reason,
  }) async {
    if (!canCorrect || reason.trim().isEmpty || state.savingUserId != null) {
      return false;
    }
    final payload = jsonEncode({
      'user_id': userId,
      'status': status.value,
      'reason': reason.trim(),
    });
    if (_pending?.payload != payload) {
      _pending = (payload: payload, key: _newKey());
    }
    final key = _pending!.key;
    state = AttendanceState(
      status: state.status,
      records: state.records,
      savingUserId: userId,
    );
    try {
      final credentials = await _credentials();
      await _api.correct(
        token: credentials.token,
        sessionId: credentials.sessionId,
        liveSessionId: sessionId,
        userId: userId,
        status: status,
        reason: reason.trim(),
        idempotencyKey: key,
      );
      _pending = null;
      await load();
      return true;
    } catch (error) {
      await _handleUnauthorized(error);
      state = AttendanceState(
        status: state.status,
        records: state.records,
        failure: _failure(error),
      );
      return false;
    }
  }

  Future<void> _handleUnauthorized(Object error) async {
    if (error is DioException && error.response?.statusCode == 401) {
      await _logout();
    }
  }

  AttendanceFailure _failure(Object error) {
    if (error is DioException) {
      final code = error.response?.statusCode;
      if (code == 401 || code == 403 || code == 404) {
        return AttendanceFailure.permission;
      }
      if (code == null) return AttendanceFailure.network;
    }
    return AttendanceFailure.unknown;
  }
}

final attendanceControllerProvider = StateNotifierProvider.autoDispose.family<
    AttendanceController,
    AttendanceState,
    ({String sessionId, bool canCorrect})>((ref, args) {
  return AttendanceController(
    ref.watch(attendanceApiClientProvider),
    () async {
      final user = ref.read(firebaseAuthProvider).currentUser;
      final sessionId = ref.read(authControllerProvider).sessionId;
      if (user == null || sessionId == null) {
        throw StateError('Attendance session is unavailable.');
      }
      final token = await user.getIdToken();
      if (token == null || token.isEmpty) {
        throw StateError('Attendance session is unavailable.');
      }
      return (token: token, sessionId: sessionId);
    },
    args.sessionId,
    canCorrect: args.canCorrect,
    logout: () => ref.read(authControllerProvider.notifier).logout(),
  );
});
