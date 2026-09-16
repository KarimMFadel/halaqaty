import 'package:halaqaty_mobile/features/sessions/data/session_protocol_constants.dart';

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
        endpoint: json[SessionJsonKeys.endpoint] as String,
        credential: json[SessionJsonKeys.credential] as String,
        expiresAt: DateTime.parse(json[SessionJsonKeys.expiresAt] as String),
      );
}
