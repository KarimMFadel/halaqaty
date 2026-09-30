import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/auth/data/auth_api_client.dart';

enum AttendanceStatus { present, late, absent, excused }

extension AttendanceStatusWire on AttendanceStatus {
  String get value => name;

  static AttendanceStatus parse(String value) =>
      AttendanceStatus.values.firstWhere((status) => status.name == value);
}

class AttendanceCorrection {
  const AttendanceCorrection({
    required this.actorId,
    required this.at,
    required this.reason,
    required this.previousStatus,
    required this.newStatus,
  });

  final String actorId;
  final DateTime at;
  final String reason;
  final AttendanceStatus previousStatus;
  final AttendanceStatus newStatus;

  factory AttendanceCorrection.fromJson(Map<String, dynamic> json) =>
      AttendanceCorrection(
        actorId: json['actor_id'] as String,
        at: DateTime.parse(json['at'] as String),
        reason: json['reason'] as String,
        previousStatus:
            AttendanceStatusWire.parse(json['previous_status'] as String),
        newStatus: AttendanceStatusWire.parse(json['new_status'] as String),
      );
}

class AttendanceRecord {
  const AttendanceRecord({
    required this.sessionId,
    required this.userId,
    required this.status,
    required this.source,
    this.firstPresenceAt,
    this.correctionId,
    this.correction,
  });

  final String sessionId;
  final String userId;
  final AttendanceStatus status;
  final String source;
  final DateTime? firstPresenceAt;
  final String? correctionId;
  final AttendanceCorrection? correction;

  factory AttendanceRecord.fromJson(Map<String, dynamic> json) {
    final correction = json['correction'] as Map<String, dynamic>?;
    return AttendanceRecord(
      sessionId: json['session_id'] as String,
      userId: json['user_id'] as String,
      status: AttendanceStatusWire.parse(json['status'] as String),
      source: json['source'] as String,
      firstPresenceAt: json['first_presence_at'] == null
          ? null
          : DateTime.parse(json['first_presence_at'] as String),
      correctionId: json['correction_id'] as String?,
      correction:
          correction == null ? null : AttendanceCorrection.fromJson(correction),
    );
  }
}

class AttendanceApiClient {
  AttendanceApiClient(this._dio);

  final Dio _dio;

  Future<List<AttendanceRecord>> list({
    required String token,
    required String sessionId,
    required String liveSessionId,
  }) async {
    final response = await _dio.get<List<dynamic>>(
      '/sessions/$liveSessionId/attendance',
      options: Options(headers: _headers(token, sessionId)),
    );
    return (response.data ?? const <dynamic>[])
        .whereType<Map<String, dynamic>>()
        .map(AttendanceRecord.fromJson)
        .toList(growable: false);
  }

  Future<AttendanceRecord> correct({
    required String token,
    required String sessionId,
    required String liveSessionId,
    required String userId,
    required AttendanceStatus status,
    required String reason,
    required String idempotencyKey,
  }) async {
    final response = await _dio.patch<Map<String, dynamic>>(
      '/sessions/$liveSessionId/attendance/$userId',
      data: {'status': status.value, 'reason': reason},
      options: Options(headers: {
        ..._headers(token, sessionId),
        'Idempotency-Key': idempotencyKey,
      }),
    );
    return AttendanceRecord.fromJson(response.data!);
  }

  Map<String, String> _headers(String token, String sessionId) => {
        'Authorization': 'Bearer $token',
        'X-Halaqaty-Session-ID': sessionId,
      };
}

final attendanceApiClientProvider = Provider<AttendanceApiClient>(
  (ref) => AttendanceApiClient(ref.watch(dioProvider)),
);
