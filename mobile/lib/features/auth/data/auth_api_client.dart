import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

class BackendUser {
  const BackendUser({
    required this.id,
    required this.firebaseUid,
    this.fullName,
    this.displayName,
    this.bio,
    this.country,
    this.avatarUrl,
    required this.preferredLanguage,
    required this.createdAt,
  });

  final String id;
  final String firebaseUid;
  final String? fullName;
  final String? displayName;
  final String? bio;
  final String? country;
  final String? avatarUrl;
  final String preferredLanguage;
  final DateTime createdAt;

  factory BackendUser.fromJson(Map<String, dynamic> json) => BackendUser(
        id: json['id'] as String,
        firebaseUid: json['firebase_uid'] as String,
        fullName: json['full_name'] as String?,
        displayName: json['display_name'] as String?,
        bio: json['bio'] as String?,
        country: json['country'] as String?,
        avatarUrl: json['avatar_url'] as String?,
        preferredLanguage: json['preferred_language'] as String,
        createdAt: DateTime.parse(json['created_at'] as String),
      );
}

class BackendSessionResponse {
  const BackendSessionResponse({
    required this.sessionId,
    required this.expiresAt,
    required this.user,
  });

  final String sessionId;
  final DateTime expiresAt;
  final BackendUser user;

  factory BackendSessionResponse.fromJson(Map<String, dynamic> json) =>
      BackendSessionResponse(
        sessionId: json['session_id'] as String,
        expiresAt: DateTime.parse(json['expires_at'] as String),
        user: BackendUser.fromJson(json['user'] as Map<String, dynamic>),
      );
}

class RegisterRequest {
  const RegisterRequest({
    required this.displayName,
    required this.preferredLanguage,
  });

  final String displayName;
  final String preferredLanguage;

  Map<String, dynamic> toJson() => {
        'display_name': displayName,
        'preferred_language': preferredLanguage,
      };
}

class CreateSessionRequest {
  const CreateSessionRequest({this.deviceName});

  final String? deviceName;

  Map<String, dynamic> toJson() => {
        if (deviceName != null) 'device_name': deviceName,
      };
}

/// Wraps HTTP calls to backend auth endpoints.
class AuthApiClient {
  AuthApiClient(this._dio);

  final Dio _dio;

  Future<BackendSessionResponse> register({
    required String firebaseIdToken,
    required RegisterRequest request,
  }) async {
    final response = await _dio.post<Map<String, dynamic>>(
      '/auth/register',
      data: request.toJson(),
      options: Options(headers: _bearerHeader(firebaseIdToken)),
    );
    return BackendSessionResponse.fromJson(
      response.data as Map<String, dynamic>,
    );
  }

  Future<BackendSessionResponse> createSession({
    required String firebaseIdToken,
    CreateSessionRequest? request,
  }) async {
    final response = await _dio.post<Map<String, dynamic>>(
      '/auth/sessions',
      data: (request ?? const CreateSessionRequest()).toJson(),
      options: Options(headers: _bearerHeader(firebaseIdToken)),
    );
    return BackendSessionResponse.fromJson(
      response.data as Map<String, dynamic>,
    );
  }

  Future<void> logout({
    required String firebaseIdToken,
    required String sessionId,
  }) async {
    await _dio.post<void>(
      '/auth/logout',
      options: Options(
        headers: {
          ..._bearerHeader(firebaseIdToken),
          'X-Halaqaty-Session-ID': sessionId,
        },
      ),
    );
  }

  Map<String, String> _bearerHeader(String token) =>
      {'Authorization': 'Bearer $token'};
}

final dioProvider = Provider<Dio>((ref) {
  return Dio(
    BaseOptions(
      baseUrl: const String.fromEnvironment(
        'API_BASE_URL',
        defaultValue: 'http://localhost:8080',
      ),
      connectTimeout: const Duration(seconds: 10),
      receiveTimeout: const Duration(seconds: 15),
    ),
  );
});

final authApiClientProvider = Provider<AuthApiClient>((ref) {
  return AuthApiClient(ref.watch(dioProvider));
});
