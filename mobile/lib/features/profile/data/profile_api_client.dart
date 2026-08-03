import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/auth/data/auth_api_client.dart';

class ProfileUser {
  const ProfileUser({
    required this.id,
    required this.firebaseUid,
    this.fullName,
    required this.displayName,
    this.bio,
    this.country,
    required this.preferredLanguage,
    this.avatarUrl,
    this.phone,
    required this.createdAt,
  });

  final String id;
  final String firebaseUid;
  final String? fullName;
  final String displayName;
  final String? bio;
  final String? country;
  final String preferredLanguage;
  final String? avatarUrl;
  final String? phone;
  final DateTime createdAt;

  factory ProfileUser.fromJson(Map<String, dynamic> json) => ProfileUser(
        id: json['id'] as String,
        firebaseUid: json['firebase_uid'] as String,
        fullName: json['full_name'] as String?,
        displayName: json['display_name'] as String? ?? '',
        bio: json['bio'] as String?,
        country: json['country'] as String?,
        preferredLanguage: json['preferred_language'] as String? ?? 'ar',
        avatarUrl: json['avatar_url'] as String?,
        phone: json['phone'] as String?,
        createdAt: DateTime.parse(json['created_at'] as String),
      );
}

class UpdateProfileRequest {
  const UpdateProfileRequest({
    this.fullName,
    this.displayName,
    this.bio,
    this.country,
    this.preferredLanguage,
    this.avatarUrl,
    this.phone,
  });

  final String? fullName;
  final String? displayName;
  final String? bio;
  final String? country;
  final String? preferredLanguage;
  final String? avatarUrl;
  final String? phone;

  Map<String, dynamic> toJson() => {
        if (fullName != null) 'full_name': fullName,
        if (displayName != null) 'display_name': displayName,
        if (bio != null) 'bio': bio,
        if (country != null) 'country': country,
        if (preferredLanguage != null) 'preferred_language': preferredLanguage,
        if (avatarUrl != null) 'avatar_url': avatarUrl,
        if (phone != null) 'phone': phone,
      };
}

class ProfileApiClient {
  ProfileApiClient(this._dio);

  final Dio _dio;

  Future<ProfileUser> getMe({
    required String firebaseIdToken,
    required String sessionId,
  }) async {
    final response = await _dio.get<Map<String, dynamic>>(
      '/auth/me',
      options: Options(
        headers: {
          ..._bearerHeader(firebaseIdToken),
          'X-Halaqaty-Session-ID': sessionId,
        },
      ),
    );
    return ProfileUser.fromJson(response.data as Map<String, dynamic>);
  }

  Future<ProfileUser> updateMe({
    required String firebaseIdToken,
    required String sessionId,
    required UpdateProfileRequest request,
  }) async {
    final response = await _dio.put<Map<String, dynamic>>(
      '/auth/me',
      data: request.toJson(),
      options: Options(
        headers: {
          ..._bearerHeader(firebaseIdToken),
          'X-Halaqaty-Session-ID': sessionId,
        },
      ),
    );
    return ProfileUser.fromJson(response.data as Map<String, dynamic>);
  }

  Map<String, String> _bearerHeader(String token) =>
      {'Authorization': 'Bearer $token'};
}

final profileApiClientProvider = Provider<ProfileApiClient>((ref) {
  return ProfileApiClient(ref.watch(dioProvider));
});
