/// F-008 REST contract models (docs/contracts/openapi.yaml — Notifications).
///
/// Shapes follow the contract exactly: `NotificationItem` puts required
/// `id, type, title, body, source_ref, created_at, is_read`; `NotificationPage`
/// carries authoritative `unread_count` plus opaque `next_cursor`; the
/// allowlisted types are mirrored from the OpenAPI enum.

/// `NotificationSourceRef` — where the authorized source lives.
class NotificationSourceRef {
  const NotificationSourceRef({required this.type, required this.id});

  final String type;
  final String id;

  factory NotificationSourceRef.fromJson(Map<String, dynamic> json) =>
      NotificationSourceRef(
        type: json['type'] as String,
        id: json['id'] as String,
      );

  Map<String, dynamic> toJson() => {'type': type, 'id': id};
}

/// One history row. `title`/`body` are generic, lock-screen-safe fallbacks;
/// display copy is resolved Arabic-first in the presentation resolver.
class NotificationItem {
  const NotificationItem({
    required this.id,
    required this.type,
    required this.title,
    required this.body,
    required this.source,
    required this.createdAt,
    required this.isRead,
  });

  final String id;
  final String type;
  final String title;
  final String body;
  final NotificationSourceRef source;
  final DateTime createdAt;
  final bool isRead;

  factory NotificationItem.fromJson(Map<String, dynamic> json) =>
      NotificationItem(
        id: json['id'] as String,
        type: json['type'] as String,
        title: json['title'] as String,
        body: json['body'] as String,
        source: NotificationSourceRef.fromJson(
            json['source_ref'] as Map<String, dynamic>),
        createdAt: DateTime.parse(json['created_at'] as String),
        isRead: json['is_read'] as bool,
      );
}

/// `GET /notifications` page projection.
class NotificationPage {
  const NotificationPage({
    required this.items,
    required this.nextCursor,
    required this.unreadCount,
  });

  /// `null` means no more pages (contract: nullable opaque cursor).
  final List<NotificationItem> items;
  final String? nextCursor;

  /// Exact, server-authoritative unread count. Never derived or
  /// incremented client-side (realtime.md: replace and reconcile).
  final int unreadCount;

  factory NotificationPage.fromJson(Map<String, dynamic> json) =>
      NotificationPage(
        items: (json['items'] as List<dynamic>? ?? const [])
            .whereType<Map<String, dynamic>>()
            .map(NotificationItem.fromJson)
            .toList(growable: false),
        nextCursor: json['next_cursor'] as String?,
        unreadCount: json['unread_count'] as int,
      );
}

/// Per-type preference row from `GET/PUT /notifications/preferences`.
class NotificationTypePreference {
  const NotificationTypePreference({
    required this.type,
    required this.enabled,
    required this.configurable,
    required this.defaultEnabled,
  });

  final String type;
  final bool enabled;
  final bool configurable;
  final bool defaultEnabled;

  factory NotificationTypePreference.fromJson(Map<String, dynamic> json) =>
      NotificationTypePreference(
        type: json['type'] as String,
        enabled: json['enabled'] as bool,
        configurable: json['configurable'] as bool,
        defaultEnabled: json['default_enabled'] as bool,
      );
}

/// Quiet-hours settings; `timezone` is read-only (profile IANA zone).
class NotificationQuietHours {
  const NotificationQuietHours({
    required this.enabled,
    required this.startLocal,
    required this.endLocal,
    required this.timezone,
  });

  final bool enabled;
  final String startLocal;
  final String endLocal;
  final String timezone;

  factory NotificationQuietHours.fromJson(Map<String, dynamic> json) =>
      NotificationQuietHours(
        enabled: json['enabled'] as bool,
        startLocal: json['start_local'] as String,
        endLocal: json['end_local'] as String,
        timezone: json['timezone'] as String,
      );

  Map<String, dynamic> toRequestJson() => {
        'enabled': enabled,
        'start_local': startLocal,
        'end_local': endLocal,
      };
}

/// `NotificationPreferences` response.
class NotificationPreferences {
  const NotificationPreferences({
    required this.types,
    required this.quietHours,
  });

  final List<NotificationTypePreference> types;
  final NotificationQuietHours quietHours;

  factory NotificationPreferences.fromJson(Map<String, dynamic> json) =>
      NotificationPreferences(
        types: (json['types'] as List<dynamic>? ?? const [])
            .whereType<Map<String, dynamic>>()
            .map(NotificationTypePreference.fromJson)
            .toList(growable: false),
        quietHours: NotificationQuietHours.fromJson(
          json['quiet_hours'] as Map<String, dynamic>,
        ),
      );
}

/// `PUT /notifications/preferences` request body (types may be omitted).
class UpdateNotificationPreferencesRequest {
  const UpdateNotificationPreferencesRequest({this.types, this.quietHours});

  final List<({String type, bool enabled})>? types;
  final NotificationQuietHours? quietHours;

  Map<String, dynamic> toJson() => {
        if (types != null)
          'types': types!
              .map((t) => {'type': t.type, 'enabled': t.enabled})
              .toList(growable: false),
        if (quietHours != null) 'quiet_hours': quietHours!.toRequestJson(),
      };
}

/// Authenticated credentials for notification calls (F-001 pattern: Firebase
/// bearer + backend session).
class NotificationCredentials {
  const NotificationCredentials({
    required this.firebaseIdToken,
    required this.sessionId,
  });

  final String firebaseIdToken;
  final String sessionId;
}

/// Resolves the current session's credentials; throws when the session is
/// unavailable (logged out, restore pending, Firebase state lost).
abstract interface class NotificationCredentialsSource {
  Future<NotificationCredentials> load();
}

/// One item of the `notification.updated` envelope plus the client-owned
/// reconnect signal.
///
/// The realtime contract (specs/008-notification-system/contracts/realtime.md)
/// carries no title/body/URL: creation/read hints expose `notification_id`
/// and the authoritative `unread_count` only, and delivery may be duplicated
/// or missed. The reconnect signal is a local transport transition (the
/// contract's REST reconciliation trigger), not a server value.
class NotificationRealtimeEvent {
  const NotificationRealtimeEvent(
      {required this.unreadCount, this.notificationId});

  const NotificationRealtimeEvent.reconnected()
      : notificationId = null,
        unreadCount = null;

  /// Authoritative unread count after the committed transition (null only
  /// for the reconnect signal).
  final int? unreadCount;

  /// Present for single-notification creation/read updates; omitted for
  /// bulk read-all/count invalidation.
  final String? notificationId;

  bool get isReconnectSignal => unreadCount == null;
}
