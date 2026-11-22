import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/auth/data/auth_api_client.dart';
import 'package:halaqaty_mobile/features/notifications/domain/notification_models.dart';

/// Notification REST boundary (`docs/contracts/openapi.yaml` — Notifications).
abstract interface class NotificationRepository {
  /// Lists the authenticated recipient's newest-first 30-day history with
  /// the authoritative unread count. [limit] is clamped to the contract
  /// range 1..50 (out-of-range would be a 400).
  Future<NotificationPage> list({
    required String firebaseIdToken,
    required String sessionId,
    String? cursor,
    int? limit,
  });

  /// Marks one notification read (idempotent). Returns the authoritative
  /// unread count after the transition.
  Future<int> markRead({
    required String firebaseIdToken,
    required String sessionId,
    required String notificationId,
  });

  /// Marks all retained unread notifications read (idempotent). Returns the
  /// authoritative unread count after the transition.
  Future<int> markAllRead({
    required String firebaseIdToken,
    required String sessionId,
  });

  /// Reads effective per-type preferences and quiet hours.
  Future<NotificationPreferences> getPreferences({
    required String firebaseIdToken,
    required String sessionId,
  });

  /// Updates configurable preferences; immutable always-on types and the
  /// profile timezone are enforced server-side. Returns effective values.
  Future<NotificationPreferences> updatePreferences({
    required String firebaseIdToken,
    required String sessionId,
    required UpdateNotificationPreferencesRequest request,
  });
}

class DioNotificationRepository implements NotificationRepository {
  DioNotificationRepository(this._dio);

  final Dio _dio;

  static const int _defaultLimit = 20;
  static const int _maxLimit = 50;

  Map<String, String> _headers(String token, String sessionId) => {
        'Authorization': 'Bearer $token',
        'X-Halaqaty-Session-ID': sessionId,
      };

  @override
  Future<NotificationPage> list({
    required String firebaseIdToken,
    required String sessionId,
    String? cursor,
    int? limit,
  }) async {
    final clamped = (limit ?? _defaultLimit).clamp(1, _maxLimit);
    final response = await _dio.get<Map<String, dynamic>>(
      '/notifications',
      queryParameters: {
        'limit': clamped,
        if (cursor != null && cursor.isNotEmpty) 'cursor': cursor,
      },
      options: Options(headers: _headers(firebaseIdToken, sessionId)),
    );
    return NotificationPage.fromJson(response.data as Map<String, dynamic>);
  }

  @override
  Future<int> markRead({
    required String firebaseIdToken,
    required String sessionId,
    required String notificationId,
  }) async {
    final response = await _dio.put<Map<String, dynamic>>(
      '/notifications/$notificationId/read',
      options: Options(headers: _headers(firebaseIdToken, sessionId)),
    );
    final count = (response.data as Map<String, dynamic>)['unread_count'];
    return count as int;
  }

  @override
  Future<int> markAllRead({
    required String firebaseIdToken,
    required String sessionId,
  }) async {
    final response = await _dio.put<Map<String, dynamic>>(
      '/notifications/read-all',
      options: Options(headers: _headers(firebaseIdToken, sessionId)),
    );
    final count = (response.data as Map<String, dynamic>)['unread_count'];
    return count as int;
  }

  @override
  Future<NotificationPreferences> getPreferences({
    required String firebaseIdToken,
    required String sessionId,
  }) async {
    final response = await _dio.get<Map<String, dynamic>>(
      '/notifications/preferences',
      options: Options(headers: _headers(firebaseIdToken, sessionId)),
    );
    return NotificationPreferences.fromJson(
      response.data as Map<String, dynamic>,
    );
  }

  @override
  Future<NotificationPreferences> updatePreferences({
    required String firebaseIdToken,
    required String sessionId,
    required UpdateNotificationPreferencesRequest request,
  }) async {
    final response = await _dio.put<Map<String, dynamic>>(
      '/notifications/preferences',
      data: request.toJson(),
      options: Options(headers: _headers(firebaseIdToken, sessionId)),
    );
    return NotificationPreferences.fromJson(
      response.data as Map<String, dynamic>,
    );
  }
}

final notificationRepositoryProvider = Provider<NotificationRepository>(
    (ref) => DioNotificationRepository(ref.watch(dioProvider)));
