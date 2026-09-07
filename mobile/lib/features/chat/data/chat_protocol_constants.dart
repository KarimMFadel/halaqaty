// F-004 chat protocol constants per `specs/004-real-time-chat/contracts/`.
// Mirrors the sessions feature's `session_protocol_constants.dart` pattern.

abstract final class ChatApiPaths {
  /// `GET/POST /circles/{circleId}/messages` (listCircleMessages /
  /// sendCircleMessage).
  static String circleMessages(String circleId) =>
      '/circles/$circleId/messages';
}

abstract final class ChatJsonKeys {
  static const data = 'data';
  static const id = 'id';
  static const circleId = 'circle_id';
  static const senderId = 'sender_id';
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
}

abstract final class ChatRealtimeTypes {
  /// Durable group/DM message projection (server to client).
  static const message = 'chat.message';
  static const text = 'text';
}

abstract final class ChatHeaders {
  static const idempotencyKey = 'Idempotency-Key';
}

abstract final class ChatLimits {
  /// Contract `SendMessageRequest.content.maxLength`.
  static const maxContentLength = 4000;
}

abstract final class ChatApiErrors {
  static const requestFailed = 'ERR_REQUEST_FAILED';
  static const requestFailedMessage = 'Chat request failed.';
}
