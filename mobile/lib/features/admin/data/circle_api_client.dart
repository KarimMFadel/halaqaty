import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/auth/data/auth_api_client.dart';

enum CircleRole { student, supervisor, teacher }

class CreateCircleRequest {
  const CreateCircleRequest({
    required this.name,
    this.teacherUserIds = const [],
    this.backupSupervisorUserId,
  });

  final String name;
  final List<String> teacherUserIds;
  final String? backupSupervisorUserId;

  Map<String, dynamic> toJson() => {
        'name': name,
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
    required this.createdAt,
  });

  final String id;
  final String name;
  final String inviteCode;
  final DateTime createdAt;

  factory CircleResponse.fromJson(Map<String, dynamic> json) => CircleResponse(
        id: json['id'] as String,
        name: json['name'] as String,
        inviteCode: json['invite_code'] as String,
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

  Map<String, String> _bearerHeader(String token) =>
      {'Authorization': 'Bearer $token'};
}

final circleApiClientProvider = Provider<CircleApiClient>((ref) {
  return CircleApiClient(ref.watch(dioProvider));
});
