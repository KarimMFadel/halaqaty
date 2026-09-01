import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/auth/data/auth_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/domain/session_models.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_protocol_constants.dart';

export 'package:halaqaty_mobile/features/sessions/domain/session_models.dart';

class SessionApiClient {
  SessionApiClient(this._dio);
  final Dio _dio;

  Future<SessionModel> create(
      {required String token,
      required String sessionId,
      required String circleId}) async {
    final response = await _dio.post<Map<String, dynamic>>(
        '${SessionApiPaths.circles}/$circleId/sessions',
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
        '${SessionApiPaths.sessions}/$liveSessionId/$action',
        options: Options(headers: _headers(token, sessionId)));
    return SessionConnection.fromJson(response.data!);
  }

  Future<List<SessionModel>> list(
      {required String token,
      required String sessionId,
      required String circleId}) async {
    final response = await _dio.get<Map<String, dynamic>>(
        '${SessionApiPaths.circles}/$circleId/sessions',
        options: Options(headers: _headers(token, sessionId)));
    return (response.data?[SessionJsonKeys.data] as List<dynamic>? ?? const [])
        .whereType<Map<String, dynamic>>()
        .map(SessionModel.fromJson)
        .toList(growable: false);
  }

  /// Authoritative presence/hand snapshot (`GET /sessions/{id}/participants`).
  Future<List<SessionParticipant>> participants(
      {required String token,
      required String sessionId,
      required String liveSessionId}) async {
    final response = await _dio.get<Map<String, dynamic>>(
        '${SessionApiPaths.sessions}/$liveSessionId/participants',
        options: Options(headers: _headers(token, sessionId)));
    return (response.data?[SessionJsonKeys.data] as List<dynamic>? ?? const [])
        .whereType<Map<String, dynamic>>()
        .map(SessionParticipant.fromJson)
        .toList(growable: false);
  }

  Future<SessionModel> setLock(
      {required String token,
      required String sessionId,
      required String liveSessionId,
      required bool locked}) async {
    final response = await _dio.post<Map<String, dynamic>>(
        '${SessionApiPaths.sessions}/$liveSessionId/lock',
        data: {SessionJsonKeys.locked: locked},
        options: Options(headers: _headers(token, sessionId)));
    return SessionModel.fromJson(response.data!);
  }

  Future<SessionModel> end(
      {required String token,
      required String sessionId,
      required String liveSessionId}) async {
    final response = await _dio.post<Map<String, dynamic>>(
        '${SessionApiPaths.sessions}/$liveSessionId/end',
        options: Options(headers: _headers(token, sessionId)));
    return SessionModel.fromJson(response.data!);
  }

  Future<void> muteAll(
          {required String token,
          required String sessionId,
          required String liveSessionId}) =>
      _postWithoutContent(
          '${SessionApiPaths.sessions}/$liveSessionId/participants/mute-all',
          token,
          sessionId);

  Future<void> muteParticipant(
          {required String token,
          required String sessionId,
          required String liveSessionId,
          required String userId}) =>
      _postWithoutContent(
          '${SessionApiPaths.sessions}/$liveSessionId/participants/$userId/mute',
          token,
          sessionId);

  Future<void> unmuteParticipant(
          {required String token,
          required String sessionId,
          required String liveSessionId,
          required String userId}) =>
      _postWithoutContent(
          '${SessionApiPaths.sessions}/$liveSessionId/participants/$userId/unmute',
          token,
          sessionId);

  Future<void> removeParticipant(
          {required String token,
          required String sessionId,
          required String liveSessionId,
          required String userId}) =>
      _postWithoutContent(
          '${SessionApiPaths.sessions}/$liveSessionId/participants/$userId/remove',
          token,
          sessionId);

  Future<void> _postWithoutContent(
      String path, String token, String sessionId) async {
    await _dio.post<void>(path,
        options: Options(headers: _headers(token, sessionId)));
  }

  Map<String, String> _headers(String token, String sessionId) =>
      sessionRequestHeaders(token, sessionId);
}

/// Shared auth headers for session-scoped REST calls.
Map<String, String> sessionRequestHeaders(String token, String sessionId) => {
      SessionHeaders.authorization: 'Bearer $token',
      SessionHeaders.sessionId: sessionId
    };

final sessionApiClientProvider = Provider<SessionApiClient>(
    (ref) => SessionApiClient(ref.watch(dioProvider)));
