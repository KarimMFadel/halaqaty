import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/auth/data/auth_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/domain/session_models.dart';

export 'package:halaqaty_mobile/features/sessions/domain/session_models.dart';

class SessionApiClient {
  SessionApiClient(this._dio);
  final Dio _dio;

  Future<SessionModel> create(
      {required String token,
      required String sessionId,
      required String circleId}) async {
    final response = await _dio.post<Map<String, dynamic>>(
        '/circles/$circleId/sessions',
        options: Options(headers: _headers(token, sessionId)));
    return SessionModel.fromJson(response.data!);
  }

  Future<SessionConnection> start(
          {required String token,
          required String sessionId,
          required String liveSessionId}) =>
      _connect(token, sessionId, liveSessionId, 'start');
  Future<SessionConnection> join(
          {required String token,
          required String sessionId,
          required String liveSessionId}) =>
      _connect(token, sessionId, liveSessionId, 'join');

  Future<SessionConnection> _connect(String token, String sessionId,
      String liveSessionId, String action) async {
    final response = await _dio.post<Map<String, dynamic>>(
        '/sessions/$liveSessionId/$action',
        options: Options(headers: _headers(token, sessionId)));
    return SessionConnection.fromJson(response.data!);
  }

  Future<List<SessionModel>> list(
      {required String token,
      required String sessionId,
      required String circleId}) async {
    final response = await _dio.get<Map<String, dynamic>>(
        '/circles/$circleId/sessions',
        options: Options(headers: _headers(token, sessionId)));
    return (response.data?['data'] as List<dynamic>? ?? const [])
        .whereType<Map<String, dynamic>>()
        .map(SessionModel.fromJson)
        .toList(growable: false);
  }

  Map<String, String> _headers(String token, String sessionId) =>
      {'Authorization': 'Bearer $token', 'X-Halaqaty-Session-ID': sessionId};
}

final sessionApiClientProvider = Provider<SessionApiClient>(
    (ref) => SessionApiClient(ref.watch(dioProvider)));
