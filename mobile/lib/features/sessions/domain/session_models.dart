import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';

class SessionModel {
  const SessionModel(
      {required this.id,
      required this.circleId,
      required this.status,
      required this.mediaMode,
      required this.participantCount,
      required this.isLocked,
      this.endReason,
      this.actualStart,
      this.actualEnd});
  final String id;
  final String circleId;
  final String status;
  final String mediaMode;
  final int participantCount;
  final bool isLocked;
  final String? endReason;
  final DateTime? actualStart;
  final DateTime? actualEnd;
  factory SessionModel.fromJson(Map<String, dynamic> json) => SessionModel(
        id: json['id'] as String,
        circleId: json['circle_id'] as String,
        status: json['status'] as String,
        mediaMode: json['media_mode'] as String,
        participantCount: json['participant_count'] as int? ?? 0,
        isLocked: json['is_locked'] as bool? ?? false,
        endReason: json['end_reason'] as String?,
        actualStart: _date(json['actual_start']),
        actualEnd: _date(json['actual_end']),
      );
  static DateTime? _date(Object? value) =>
      value is String ? DateTime.tryParse(value) : null;
}

/// A short-lived provider credential. It must never be persisted or logged.
class MediaConnection {
  const MediaConnection(
      {required this.endpoint,
      required this.credential,
      required this.expiresAt});
  final String endpoint;
  final String credential;
  final DateTime expiresAt;
  factory MediaConnection.fromJson(Map<String, dynamic> json) =>
      MediaConnection(
        endpoint: json['endpoint'] as String,
        credential: json['credential'] as String,
        expiresAt: DateTime.parse(json['expires_at'] as String),
      );
}

class SessionConnection {
  const SessionConnection(
      {required this.session,
      required this.mediaConnection,
      this.isModerator = false});
  final SessionModel session;
  final MediaConnection mediaConnection;
  final bool isModerator;
  factory SessionConnection.fromJson(Map<String, dynamic> json) =>
      SessionConnection(
        session: SessionModel.fromJson(json['session'] as Map<String, dynamic>),
        mediaConnection: MediaConnection.fromJson(
            json['media_connection'] as Map<String, dynamic>),
        isModerator: json['is_moderator'] as bool? ?? false,
      );
}

/// Presence and hand state for one session participant
/// (`SessionParticipant` in the canonical OpenAPI contract).
class SessionParticipant {
  const SessionParticipant(
      {required this.userId,
      required this.displayName,
      required this.role,
      required this.isCurrentlyPresent,
      this.handRaisedAt});
  final String userId;
  final String displayName;
  final CircleRole role;
  final bool isCurrentlyPresent;
  final DateTime? handRaisedAt;

  bool get isHandRaised => handRaisedAt != null;

  factory SessionParticipant.fromJson(Map<String, dynamic> json) =>
      SessionParticipant(
        userId: json['user_id'] as String,
        displayName: json['display_name'] as String,
        role: CircleRole.values.byName(json['role'] as String),
        isCurrentlyPresent: json['is_currently_present'] as bool? ?? false,
        handRaisedAt: _date(json['hand_raised_at']),
      );

  static DateTime? _date(Object? value) =>
      value is String ? DateTime.tryParse(value) : null;

  SessionParticipant copyWith(
          {bool? isCurrentlyPresent,
          DateTime? handRaisedAt,
          bool handLowered = false}) =>
      SessionParticipant(
        userId: userId,
        displayName: displayName,
        role: role,
        isCurrentlyPresent: isCurrentlyPresent ?? this.isCurrentlyPresent,
        handRaisedAt: handLowered ? null : (handRaisedAt ?? this.handRaisedAt),
      );
}
