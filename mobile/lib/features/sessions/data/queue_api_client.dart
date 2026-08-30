import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/auth/data/auth_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_protocol_constants.dart';

class QueuePreorderItem {
  const QueuePreorderItem({
    required this.studentId,
    required this.studentName,
    required this.position,
  });

  final String studentId;
  final String studentName;
  final int position;

  factory QueuePreorderItem.fromJson(Map<String, dynamic> json) =>
      QueuePreorderItem(
        studentId: json['student_id'] as String,
        studentName: json['student_name'] as String,
        position: json['position'] as int,
      );
}

class QueueEntry {
  const QueueEntry({
    required this.id,
    required this.studentId,
    required this.studentName,
    required this.position,
    required this.status,
    required this.version,
    this.grade,
    this.gradeNotes,
  });

  final String id;
  final String studentId;
  final String studentName;
  final int position;
  final String status;
  final String? grade;
  final String? gradeNotes;
  final int version;

  factory QueueEntry.fromJson(Map<String, dynamic> json) => QueueEntry(
        id: json['id'] as String,
        studentId: json['student_id'] as String,
        studentName: json['student_name'] as String,
        position: json['position'] as int,
        status: json['status'] as String,
        grade: json['grade'] as String?,
        gradeNotes: json['grade_notes'] as String?,
        version: json['version'] as int,
      );
}

class QueuePolicy {
  const QueuePolicy({
    required this.population,
    required this.unfinishedFinalization,
    required this.optOut,
    required this.gradeVisibility,
    required this.gradeCorrection,
    required this.version,
  });

  final String population;
  final String unfinishedFinalization;
  final String optOut;
  final String gradeVisibility;
  final String gradeCorrection;
  final int version;

  factory QueuePolicy.fromJson(Map<String, dynamic> json) => QueuePolicy(
        population: json['population'] as String,
        unfinishedFinalization: json['unfinished_finalization'] as String,
        optOut: json['opt_out'] as String,
        gradeVisibility: json['grade_visibility'] as String,
        gradeCorrection: json['grade_correction'] as String,
        version: json['version'] as int,
      );
}

class QueueState {
  const QueueState({
    required this.sessionId,
    required this.roundId,
    required this.roundNumber,
    required this.roundType,
    required this.lifecycle,
    required this.surahId,
    required this.fromAyah,
    required this.toAyah,
    required this.gradingRequired,
    required this.version,
    required this.policy,
    required this.preorder,
    required this.entries,
    this.selectedEntryId,
  });

  final String sessionId;
  final String roundId;
  final int roundNumber;
  final String roundType;
  final String lifecycle;
  final int surahId;
  final int fromAyah;
  final int toAyah;
  final bool gradingRequired;
  final String? selectedEntryId;
  final int version;
  final QueuePolicy policy;
  final List<QueuePreorderItem> preorder;
  final List<QueueEntry> entries;

  factory QueueState.fromJson(Map<String, dynamic> json) => QueueState(
        sessionId: json['session_id'] as String,
        roundId: json['round_id'] as String,
        roundNumber: json['round_number'] as int,
        roundType: json['round_type'] as String,
        lifecycle: json['lifecycle'] as String,
        surahId: json['surah_id'] as int,
        fromAyah: json['from_ayah'] as int,
        toAyah: json['to_ayah'] as int,
        gradingRequired: json['grading_required'] as bool,
        selectedEntryId: json['selected_entry_id'] as String?,
        version: json['version'] as int,
        policy: QueuePolicy.fromJson(json['policy'] as Map<String, dynamic>),
        preorder:
            _items(json['preorder']).map(QueuePreorderItem.fromJson).toList(
                  growable: false,
                ),
        entries: _items(json['entries']).map(QueueEntry.fromJson).toList(
              growable: false,
            ),
      );

  static List<Map<String, dynamic>> _items(Object? value) =>
      (value as List<dynamic>? ?? const [])
          .whereType<Map<String, dynamic>>()
          .toList(growable: false);
}

class OptOutRequest {
  const OptOutRequest({
    required this.id,
    required this.queueEntryId,
    required this.status,
    required this.requestedAt,
    this.decidedAt,
  });

  final String id;
  final String queueEntryId;
  final String status;
  final DateTime requestedAt;
  final DateTime? decidedAt;

  factory OptOutRequest.fromJson(Map<String, dynamic> json) => OptOutRequest(
        id: json['id'] as String,
        queueEntryId: json['queue_entry_id'] as String,
        status: json['status'] as String,
        requestedAt: DateTime.parse(json['requested_at'] as String),
        decidedAt: json['decided_at'] == null
            ? null
            : DateTime.parse(json['decided_at'] as String),
      );
}

class OptOutResult {
  const OptOutResult({
    required this.request,
    required this.entry,
  });

  final OptOutRequest request;
  final QueueEntry entry;

  factory OptOutResult.fromJson(Map<String, dynamic> json) => OptOutResult(
        request:
            OptOutRequest.fromJson(json['request'] as Map<String, dynamic>),
        entry: QueueEntry.fromJson(json['entry'] as Map<String, dynamic>),
      );
}

class QueueApiException implements Exception {
  const QueueApiException({
    required this.statusCode,
    required this.code,
    required this.message,
    this.fields,
  });

  final int? statusCode;
  final String code;
  final String message;
  final Map<String, String>? fields;

  @override
  String toString() => '$code: $message';
}

class QueueApiClient {
  QueueApiClient(this._dio);

  final Dio _dio;

  Future<QueueState> getQueue({
    required String token,
    required String sessionId,
    required String liveSessionId,
  }) =>
      _state(() => _dio.get<Map<String, dynamic>>(
            _queuePath(liveSessionId),
            options: Options(headers: _headers(token, sessionId)),
          ));

  Future<QueueState> prepareRound({
    required String token,
    required String sessionId,
    required String liveSessionId,
    required String roundType,
    required int surahId,
    required int fromAyah,
    required int toAyah,
    required bool gradingRequired,
    List<String>? studentOrder,
    String? idempotencyKey,
  }) =>
      _state(() => _dio.post<Map<String, dynamic>>(
            '${_queuePath(liveSessionId)}/rounds',
            data: {
              'round_type': roundType,
              'surah_id': surahId,
              'from_ayah': fromAyah,
              'to_ayah': toAyah,
              'grading_required': gradingRequired,
              if (studentOrder != null) 'student_order': studentOrder,
            },
            options: Options(
              headers: _headers(token, sessionId, idempotencyKey),
            ),
          ));

  Future<QueueState> advance({
    required String token,
    required String sessionId,
    required String liveSessionId,
    required int expectedVersion,
    String? idempotencyKey,
  }) =>
      _state(() => _dio.post<Map<String, dynamic>>(
            '${_queuePath(liveSessionId)}/advance',
            data: {'expected_version': expectedVersion},
            options: Options(
              headers: _headers(token, sessionId, idempotencyKey),
            ),
          ));

  Future<QueueState> reorder({
    required String token,
    required String sessionId,
    required String liveSessionId,
    required List<String> orderedIds,
    required int expectedVersion,
    String? idempotencyKey,
  }) =>
      _state(() => _dio.put<Map<String, dynamic>>(
            '${_queuePath(liveSessionId)}/order',
            data: {
              'ordered_ids': orderedIds,
              'expected_version': expectedVersion,
            },
            options: Options(
              headers: _headers(token, sessionId, idempotencyKey),
            ),
          ));

  Future<QueueState> moveEntry({
    required String token,
    required String sessionId,
    required String liveSessionId,
    required String entryId,
    required int newPosition,
    required int expectedVersion,
    String? idempotencyKey,
  }) =>
      _state(() => _dio.post<Map<String, dynamic>>(
            '${_queuePath(liveSessionId)}/entries/$entryId/move',
            data: {
              'new_position': newPosition,
              'expected_version': expectedVersion,
            },
            options: Options(
              headers: _headers(token, sessionId, idempotencyKey),
            ),
          ));

  Future<QueueState> updateEntryStatus({
    required String token,
    required String sessionId,
    required String liveSessionId,
    required String entryId,
    required String status,
    required int expectedEntryVersion,
    String? idempotencyKey,
  }) =>
      _state(() => _dio.put<Map<String, dynamic>>(
            '${_queuePath(liveSessionId)}/entries/$entryId/status',
            data: {
              'status': status,
              'expected_entry_version': expectedEntryVersion,
            },
            options: Options(
              headers: _headers(token, sessionId, idempotencyKey),
            ),
          ));

  Future<OptOutResult> optOut({
    required String token,
    required String sessionId,
    required String liveSessionId,
    String? idempotencyKey,
  }) async {
    try {
      final response = await _dio.post<Map<String, dynamic>>(
        '${_queuePath(liveSessionId)}/opt-out',
        options: Options(headers: _headers(token, sessionId, idempotencyKey)),
      );
      return OptOutResult.fromJson(response.data!);
    } on DioException catch (error) {
      throw _exception(error);
    }
  }

  Future<QueueState> reset({
    required String token,
    required String sessionId,
    required String liveSessionId,
    required String roundType,
    required int surahId,
    required int fromAyah,
    required int toAyah,
    required bool gradingRequired,
    required int expectedVersion,
    List<String>? studentOrder,
    String? idempotencyKey,
  }) =>
      _state(() => _dio.post<Map<String, dynamic>>(
            '${_queuePath(liveSessionId)}/reset',
            data: {
              'round_type': roundType,
              'surah_id': surahId,
              'from_ayah': fromAyah,
              'to_ayah': toAyah,
              'grading_required': gradingRequired,
              if (studentOrder != null) 'student_order': studentOrder,
              'expected_version': expectedVersion,
            },
            options: Options(
              headers: _headers(token, sessionId, idempotencyKey),
            ),
          ));

  Future<QueuePolicy> updatePolicy({
    required String token,
    required String sessionId,
    required String liveSessionId,
    required int expectedVersion,
    String? population,
    String? unfinishedFinalization,
    String? optOut,
    String? gradeVisibility,
    String? gradeCorrection,
    String? idempotencyKey,
  }) =>
      _policy(() => _dio.patch<Map<String, dynamic>>(
            '${_queuePath(liveSessionId)}/policy',
            data: {
              'expected_version': expectedVersion,
              if (population != null) 'population': population,
              if (unfinishedFinalization != null)
                'unfinished_finalization': unfinishedFinalization,
              if (optOut != null) 'opt_out': optOut,
              if (gradeVisibility != null) 'grade_visibility': gradeVisibility,
              if (gradeCorrection != null) 'grade_correction': gradeCorrection,
            },
            options: Options(
              headers: _headers(token, sessionId, idempotencyKey),
            ),
          ));

  Future<QueueState> _state(
    Future<Response<Map<String, dynamic>>> Function() request,
  ) async =>
      QueueState.fromJson(await _data(request));

  Future<QueuePolicy> _policy(
    Future<Response<Map<String, dynamic>>> Function() request,
  ) async =>
      QueuePolicy.fromJson(await _data(request));

  Future<Map<String, dynamic>> _data(
    Future<Response<Map<String, dynamic>>> Function() request,
  ) async {
    try {
      return (await request()).data!;
    } on DioException catch (error) {
      throw _exception(error);
    }
  }

  QueueApiException _exception(DioException error) {
    final response = error.response;
    final body = response?.data;
    final envelope = body is Map<String, dynamic> ? body['error'] : null;
    final details = envelope is Map<String, dynamic> ? envelope : const {};
    final rawFields = details['fields'];
    return QueueApiException(
      statusCode: response?.statusCode,
      code: details['code'] as String? ?? 'ERR_REQUEST_FAILED',
      message: details['message'] as String? ?? 'Queue request failed.',
      fields: rawFields is Map
          ? Map<String, String>.fromEntries(
              rawFields.entries
                  .where(
                      (entry) => entry.key is String && entry.value is String)
                  .map(
                    (entry) =>
                        MapEntry(entry.key as String, entry.value as String),
                  ),
            )
          : null,
    );
  }

  Map<String, String> _headers(String token, String sessionId,
          [String? idempotencyKey]) =>
      {
        ...sessionRequestHeaders(token, sessionId),
        if (idempotencyKey != null) 'Idempotency-Key': idempotencyKey,
      };

  String _queuePath(String liveSessionId) =>
      '${SessionApiPaths.sessions}/$liveSessionId/queue';
}

final queueApiClientProvider = Provider<QueueApiClient>(
  (ref) => QueueApiClient(ref.watch(dioProvider)),
);
