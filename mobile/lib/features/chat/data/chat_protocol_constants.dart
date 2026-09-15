// F-004 chat protocol constants per `specs/004-real-time-chat/contracts/`.
// Mirrors the sessions feature's `session_protocol_constants.dart` pattern.

abstract final class ChatApiPaths {
  /// `GET/POST /circles/{circleId}/messages` (listCircleMessages /
  /// sendCircleMessage).
  static String circleMessages(String circleId) =>
      '/circles/$circleId/messages';

  static String searchCircleMessages(String circleId) =>
      '/circles/$circleId/messages/search';

  static String markCircleMessageRead(String circleId, String messageId) =>
      '/circles/$circleId/messages/$messageId/read';

  static String markDirectMessageRead(String userId, String messageId) =>
      '/dm/$userId/messages/$messageId/read';

  static String directMessages(String userId) => '/dm/$userId';

  static String deleteDirectMessage(String userId, String messageId) =>
      '/dm/$userId/messages/$messageId';

  static String pinCircleMessage(String circleId, String messageId) =>
      '/circles/$circleId/messages/$messageId/pin';

  static String pinnedCircleMessages(String circleId) =>
      '/circles/$circleId/messages/pinned';
}

abstract final class ChatMediaApiPaths {
  /// `POST /uploads/voice` (uploadVoice).
  static const uploadsVoice = '/uploads/voice';

  /// `POST /uploads/image` (uploadImage).
  static const uploadsImage = '/uploads/image';

  /// `POST /uploads/file` (uploadFile).
  static const uploadsFile = '/uploads/file';

  /// `POST /messages/{messageId}/media-url` (renewMessageMediaUrl).
  static String renewMediaUrl(String messageId) =>
      '/messages/$messageId/media-url';
}

abstract final class ChatJsonKeys {
  static const data = 'data';
  static const id = 'id';
  static const messageId = 'message_id';
  static const circleId = 'circle_id';
  static const dmPeerId = 'dm_peer_id';
  static const senderId = 'sender_id';
  static const userId = 'user_id';
  static const senderName = 'sender_name';
  static const messageType = 'message_type';
  static const content = 'content';
  static const sentAt = 'sent_at';
  static const deliveryStatus = 'delivery_status';
  static const hasMore = 'has_more';
  static const nextBefore = 'next_before';
  static const type = 'type';
  static const payload = 'payload';
  static const eventId = 'event_id';
  static const error = 'error';
  static const code = 'code';
  static const message = 'message';
  static const limit = 'limit';
  static const before = 'before';
  static const query = 'q';
  static const readerId = 'reader_id';
  static const readAt = 'read_at';
  static const isTyping = 'is_typing';

  /// Multipart `ChatUpload` field names and upload/media response keys.
  static const file = 'file';

  /// Client-declared voice duration (1–300 s) required by `/uploads/voice`.
  static const durationSeconds = 'duration_seconds';
  static const url = 'url';
  static const objectKey = 'object_key';
  static const uploadId = 'upload_id';
  static const urlExpiresAt = 'url_expires_at';
  static const expiresAt = 'expires_at';

  /// Contract `Message` media projection keys.
  static const mediaUrl = 'media_url';
  static const mediaUrlExpiresAt = 'media_url_expires_at';
  static const fileName = 'file_name';
  static const voiceDurationSeconds = 'voice_duration_seconds';
  static const replyToId = 'reply_to_id';
  static const replyPreview = 'reply_preview';
  static const pinnedAt = 'pinned_at';
  static const pinnedBy = 'pinned_by';
  static const deletedAt = 'deleted_at';
}

abstract final class ChatRealtimeTypes {
  /// Durable group/DM message projection (server to client).
  static const message = 'chat.message';
  static const messageRead = 'chat.message_read';
  static const messageDeleted = 'chat.message_deleted';
  static const typing = 'chat.typing';
  static const text = 'text';
}

abstract final class ChatHeaders {
  static const idempotencyKey = 'Idempotency-Key';

  /// Server-advised retry delay on `429`, honored up to 30 seconds (FR-008).
  static const retryAfter = 'Retry-After';
}

/// Ceiling applied to any server-advised `Retry-After` delay (spec
/// assumption: 429 honors Retry-After capped at 30 seconds).
const chatRetryAfterCap = Duration(seconds: 30);

abstract final class ChatLimits {
  /// Contract `SendMessageRequest.content.maxLength`.
  static const maxContentLength = 4000;

  /// Contract `/uploads/voice`: at most 300 seconds (FR-020).
  static const maxVoiceDurationSeconds = 300;

  /// Contract `/uploads/voice`: at most 20 MB (FR-020).
  static const maxVoiceBytes = 20 * 1024 * 1024;

  /// Contract `/uploads/image`: JPEG/PNG at most 5 MB (FR-021).
  static const maxImageBytes = 5 * 1024 * 1024;

  /// Contract `/uploads/file`: PDF at most 10 MB (FR-021).
  static const maxFileBytes = 10 * 1024 * 1024;
}

abstract final class ChatApiErrors {
  static const requestFailed = 'ERR_REQUEST_FAILED';
  static const requestFailedMessage = 'Chat request failed.';
  static const validationFailed = 'ERR_VALIDATION_FAILED';
  static const uploadTargetInvalid =
      'exactly one of circle_id or dm_peer_id is required';
}
