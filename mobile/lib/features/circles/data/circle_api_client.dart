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
    this.isArchived = false,
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
  final bool isArchived;

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
        isArchived: json['is_archived'] as bool? ?? false,
      );

  CircleSummary toSummary() => CircleSummary(
        id: id,
        name: name,
        description: description,
        maxCapacity: maxCapacity,
        genderRestriction: genderRestriction,
        language: language,
        createdAt: createdAt,
      );
}

class CircleMember {
  const CircleMember({
    required this.userId,
    required this.displayName,
    required this.role,
    required this.joinedAt,
  });

  final String userId;
  final String displayName;
  final CircleRole role;
  final DateTime joinedAt;

  factory CircleMember.fromJson(Map<String, dynamic> json) => CircleMember(
        userId: json['user_id'] as String,
        displayName: json['display_name'] as String,
        role: CircleRole.values.byName(json['role'] as String),
        joinedAt: DateTime.parse(json['joined_at'] as String),
      );
}

class CircleSummary {
  const CircleSummary({
    required this.id,
    required this.name,
    required this.description,
    required this.maxCapacity,
    required this.genderRestriction,
    required this.language,
    required this.createdAt,
  });

  final String id;
  final String name;
  final String? description;
  final int maxCapacity;
  final String genderRestriction;
  final String language;
  final DateTime createdAt;

  factory CircleSummary.fromJson(Map<String, dynamic> json) => CircleSummary(
        id: json['id'] as String,
        name: json['name'] as String,
        description: json['description'] as String?,
        maxCapacity: json['max_capacity'] as int,
        genderRestriction: json['gender_restriction'] as String,
        language: json['language'] as String,
        createdAt: DateTime.parse(json['created_at'] as String),
      );
}

class CircleDiscoveryPage {
  const CircleDiscoveryPage({required this.circles, this.nextCursor});

  final List<CircleSummary> circles;
  final String? nextCursor;
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

class CircleInviteResponse {
  const CircleInviteResponse(
      {required this.inviteCode, required this.inviteLink});

  final String inviteCode;
  final String inviteLink;

  factory CircleInviteResponse.fromJson(Map<String, dynamic> json) =>
      CircleInviteResponse(
        inviteCode: json['invite_code'] as String,
        inviteLink: json['invite_link'] as String,
      );
}

class CircleApiClient {
  CircleApiClient(this._dio);

  final Dio _dio;

  Future<CircleResponse> getCircle({
    required String firebaseIdToken,
    required String sessionId,
    required String circleId,
  }) async {
    final response = await _dio.get<Map<String, dynamic>>(
      '/circles/$circleId',
      options: Options(headers: _authHeaders(firebaseIdToken, sessionId)),
    );
    return CircleResponse.fromJson(response.data!);
  }

  Future<List<CircleMember>> listCircleMembers({
    required String firebaseIdToken,
    required String sessionId,
    required String circleId,
  }) async {
    final response = await _dio.get<Map<String, dynamic>>(
      '/circles/$circleId/members',
      options: Options(headers: _authHeaders(firebaseIdToken, sessionId)),
    );
    final data = (response.data?['data'] as List<dynamic>? ?? const []);
    return data
        .whereType<Map<String, dynamic>>()
        .map(CircleMember.fromJson)
        .toList(growable: false);
  }

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

  Future<List<CircleSummary>> listCircles({
    required String firebaseIdToken,
    required String sessionId,
  }) async {
    final response = await _dio.get<Map<String, dynamic>>(
      '/circles',
      options: Options(headers: _authHeaders(firebaseIdToken, sessionId)),
    );
    return _summaries(response.data?['data']);
  }

  Future<CircleDiscoveryPage> discoverCircles({
    required String firebaseIdToken,
    required String sessionId,
    String? query,
    String? cursor,
  }) async {
    final response = await _dio.get<Map<String, dynamic>>(
      '/circles/discover',
      queryParameters: {
        if (query != null && query.isNotEmpty) 'query': query,
        if (cursor != null && cursor.isNotEmpty) 'cursor': cursor,
      },
      options: Options(headers: _authHeaders(firebaseIdToken, sessionId)),
    );
    return CircleDiscoveryPage(
      circles: _summaries(response.data?['data']),
      nextCursor: response.data?['next_cursor'] as String?,
    );
  }

  Future<CircleResponse> joinPublicCircle({
    required String firebaseIdToken,
    required String sessionId,
    required String circleId,
  }) async {
    final response = await _dio.post<Map<String, dynamic>>(
      '/circles/$circleId/join',
      options: Options(headers: _authHeaders(firebaseIdToken, sessionId)),
    );
    return CircleResponse.fromJson(response.data as Map<String, dynamic>);
  }

  Future<CircleResponse> joinCircleByInvite({
    required String firebaseIdToken,
    required String sessionId,
    required String inviteCode,
  }) async {
    final response = await _dio.post<Map<String, dynamic>>(
      '/circles/join',
      data: {'invite_code': inviteCode},
      options: Options(headers: _authHeaders(firebaseIdToken, sessionId)),
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

  Future<void> removeMember({
    required String firebaseIdToken,
    required String sessionId,
    required String circleId,
    required String userId,
  }) =>
      _dio.delete<void>(
        '/circles/$circleId/members/$userId',
        options: Options(headers: _authHeaders(firebaseIdToken, sessionId)),
      );

  Future<CircleInviteResponse> refreshInviteCode({
    required String firebaseIdToken,
    required String sessionId,
    required String circleId,
  }) async {
    final response = await _dio.post<Map<String, dynamic>>(
      '/circles/$circleId/invite-code/refresh',
      options: Options(headers: _authHeaders(firebaseIdToken, sessionId)),
    );
    return CircleInviteResponse.fromJson(response.data!);
  }

  Future<void> archiveCircle({
    required String firebaseIdToken,
    required String sessionId,
    required String circleId,
  }) =>
      _dio.delete<void>(
        '/circles/$circleId',
        options: Options(headers: _authHeaders(firebaseIdToken, sessionId)),
      );

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

  Map<String, String> _authHeaders(String token, String sessionId) => {
        ..._bearerHeader(token),
        'X-Halaqaty-Session-ID': sessionId,
      };

  List<CircleSummary> _summaries(dynamic payload) =>
      (payload as List<dynamic>? ?? const [])
          .whereType<Map<String, dynamic>>()
          .map(CircleSummary.fromJson)
          .toList(growable: false);
}

final circleApiClientProvider = Provider<CircleApiClient>((ref) {
  return CircleApiClient(ref.watch(dioProvider));
});
