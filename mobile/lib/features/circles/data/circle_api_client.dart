import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/auth/data/auth_api_client.dart';

enum CircleRole { student, supervisor, teacher }

class CircleUser {
  const CircleUser({required this.id, required this.displayName});

  final String id;
  final String displayName;

  factory CircleUser.fromJson(Map<String, dynamic> json) => CircleUser(
        id: json['id'] as String,
        displayName: json['display_name'] as String,
      );
}

class CreateCircleRequest {
  const CreateCircleRequest({
    required this.name,
    this.description,
    this.rules,
    this.maxCapacity = 50,
    this.isPrivate = false,
    this.genderRestriction = 'unspecified',
    this.language = 'ar',
    this.teacherUserIds = const [],
    this.backupSupervisorUserId,
  });

  final String name;
  final String? description;
  final String? rules;
  final int maxCapacity;
  final bool isPrivate;
  final String genderRestriction;
  final String language;
  final List<String> teacherUserIds;
  final String? backupSupervisorUserId;

  Map<String, dynamic> toJson() => {
        'name': name,
        if (description != null) 'description': description,
        if (rules != null) 'rules': rules,
        'max_capacity': maxCapacity,
        'is_private': isPrivate,
        'gender_restriction': genderRestriction,
        'language': language,
        'teacher_user_ids': teacherUserIds,
        if (backupSupervisorUserId != null)
          'backup_supervisor_user_id': backupSupervisorUserId,
      };
}

class CircleResponse {
  const CircleResponse({
    required this.id,
    required this.name,
    required this.inviteCode,
    required this.inviteLink,
    this.description,
    this.rules,
    this.maxCapacity = 50,
    this.isPrivate = false,
    this.genderRestriction = 'unspecified',
    this.language = 'ar',
    required this.createdAt,
  });

  final String id;
  final String name;
  final String inviteCode;
  final String inviteLink;
  final String? description;
  final String? rules;
  final int maxCapacity;
  final bool isPrivate;
  final String genderRestriction;
  final String language;
  final DateTime createdAt;

  factory CircleResponse.fromJson(Map<String, dynamic> json) => CircleResponse(
        id: json['id'] as String,
        name: json['name'] as String,
        inviteCode: json['invite_code'] as String,
        inviteLink: json['invite_link'] as String,
        description: json['description'] as String?,
        rules: json['rules'] as String?,
        maxCapacity: json['max_capacity'] as int? ?? 50,
        isPrivate: json['is_private'] as bool? ?? false,
        genderRestriction:
            json['gender_restriction'] as String? ?? 'unspecified',
        language: json['language'] as String? ?? 'ar',
        createdAt: DateTime.parse(json['created_at'] as String),
      );
}

class AssignCircleRoleRequest {
  const AssignCircleRoleRequest({required this.role});

  final CircleRole role;

  Map<String, dynamic> toJson() => {'role': role.name};
}

class CircleRoleAssignmentResponse {
  const CircleRoleAssignmentResponse({
    required this.circleId,
    required this.userId,
    required this.role,
  });

  final String circleId;
  final String userId;
  final CircleRole role;

  factory CircleRoleAssignmentResponse.fromJson(Map<String, dynamic> json) =>
      CircleRoleAssignmentResponse(
        circleId: json['circle_id'] as String,
        userId: json['user_id'] as String,
        role: CircleRole.values.byName(json['role'] as String),
      );
}

class CircleApiClient {
  CircleApiClient(this._dio);

  final Dio _dio;

  Future<CircleResponse> createCircle({
    required String firebaseIdToken,
    required String sessionId,
    required CreateCircleRequest request,
  }) async {
    final response = await _dio.post<Map<String, dynamic>>(
      '/circles',
      data: request.toJson(),
      options: Options(
        headers: {
          ..._bearerHeader(firebaseIdToken),
          'X-Halaqaty-Session-ID': sessionId,
        },
      ),
    );
    return CircleResponse.fromJson(response.data as Map<String, dynamic>);
  }

  Future<CircleRoleAssignmentResponse> assignMemberRole({
    required String firebaseIdToken,
    required String sessionId,
    required String circleId,
    required String userId,
    required AssignCircleRoleRequest request,
  }) async {
    final response = await _dio.put<Map<String, dynamic>>(
      '/circles/$circleId/members/$userId/role',
      data: request.toJson(),
      options: Options(
        headers: {
          ..._bearerHeader(firebaseIdToken),
          'X-Halaqaty-Session-ID': sessionId,
        },
      ),
    );
    return CircleRoleAssignmentResponse.fromJson(
      response.data as Map<String, dynamic>,
    );
  }

  Future<List<CircleUser>> searchUsers({
    required String firebaseIdToken,
    required String sessionId,
    required String query,
  }) async {
    final response = await _dio.get<Map<String, dynamic>>(
      '/users/search',
      queryParameters: {'q': query},
      options: Options(headers: {
        ..._bearerHeader(firebaseIdToken),
        'X-Halaqaty-Session-ID': sessionId,
      }),
    );
    final data = (response.data?['data'] as List<dynamic>? ?? const []);
    return data
        .whereType<Map<String, dynamic>>()
        .map(CircleUser.fromJson)
        .toList(growable: false);
  }

  Map<String, String> _bearerHeader(String token) =>
      {'Authorization': 'Bearer $token'};
}

final circleApiClientProvider = Provider<CircleApiClient>((ref) {
  return CircleApiClient(ref.watch(dioProvider));
});
