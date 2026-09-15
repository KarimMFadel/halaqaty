import 'dart:math';

import 'package:halaqaty_mobile/features/chat/data/chat_protocol_constants.dart';

/// F-004 domain models for chat per `specs/004-real-time-chat/contracts/`.
///
/// Delivery statuses `pending`/`sent` are client-local only; the server is
/// authoritative and only ever emits `delivered`/`read`.
enum ChatDeliveryStatus { pending, sent, delivered, read }

/// Contract `message_type` enum (text, image, file, voice).
enum ChatMessageType { text, image, file, voice }

/// Local text validation outcome; presentation maps each value to localized
/// copy so the data layer stays free of user-visible strings.
enum ChatTextValidation { valid, empty, tooLong }

ChatMessageType _messageTypeFromName(String? name) {
  final type = ChatMessageType.values.asNameMap()[name];
  if (type == null) {
    throw FormatException('Unknown message_type: $name');
  }
  return type;
}

ChatDeliveryStatus _deliveryStatusFromName(String? name) {
  final status = ChatDeliveryStatus.values.asNameMap()[name];
  if (status == null ||
      status == ChatDeliveryStatus.pending ||
      status == ChatDeliveryStatus.sent) {
    throw FormatException('Unknown delivery_status: $name');
  }
  return status;
}

DateTime? _parseNullableDate(String? raw) =>
    raw == null ? null : DateTime.parse(raw);

class ChatReplyPreviewProjection {
  const ChatReplyPreviewProjection({
    required this.id,
    required this.senderName,
    required this.preview,
    required this.deleted,
  });

  factory ChatReplyPreviewProjection.fromJson(Map<String, dynamic> json) =>
      ChatReplyPreviewProjection(
        id: json['id'] as String,
        senderName: json['sender_name'] as String? ?? '',
        preview:
            json['deleted'] == true ? '' : json['preview'] as String? ?? '',
        deleted: json['deleted'] as bool? ?? false,
      );

  final String id;
  final String senderName;
  final String preview;
  final bool deleted;
}

/// Validates chat text before sending: non-empty after trimming and at most
/// [ChatLimits.maxContentLength] characters (contract `content.maxLength`).
ChatTextValidation validateChatText(String? content) {
  final trimmed = content?.trim() ?? '';
  if (trimmed.isEmpty) return ChatTextValidation.empty;
  if (trimmed.runes.length > ChatLimits.maxContentLength) {
    return ChatTextValidation.tooLong;
  }
  return ChatTextValidation.valid;
}

/// Creates a random (version 4) UUID used as the stable idempotency key of
/// one logical message send: every retry of that send MUST reuse it so the
/// server converges duplicates onto one durable message.
String newChatIdempotencyKey() {
  final bytes = List<int>.generate(16, (_) => Random.secure().nextInt(256));
  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;
  final hex = bytes.map((b) => b.toRadixString(16).padLeft(2, '0')).join();
  return '${hex.substring(0, 8)}-${hex.substring(8, 12)}-'
      '${hex.substring(12, 16)}-${hex.substring(16, 20)}-${hex.substring(20)}';
}

class ChatMessage {
  const ChatMessage({
    required this.id,
    required this.senderId,
    required this.circleId,
    this.dmPeerId,
    required this.content,
    required this.type,
    required this.sentAt,
    required this.deliveryStatus,
    this.senderName,
    this.mediaUrl,
    this.mediaUrlExpiresAt,
    this.fileName,
    this.voiceDurationSeconds,
    this.replyToId,
    this.replyPreview,
    this.pinnedAt,
    this.pinnedBy,
    this.deletedAt,
    this.readReceipts = const [],
  });

  factory ChatMessage.fromJson(Map<String, dynamic> json) => ChatMessage(
        id: json[ChatJsonKeys.id] as String,
        senderId: json[ChatJsonKeys.senderId] as String,
        circleId: json[ChatJsonKeys.circleId] as String?,
        dmPeerId: json[ChatJsonKeys.dmPeerId] as String?,
        content: json[ChatJsonKeys.content] as String? ?? '',
        type: _messageTypeFromName(json[ChatJsonKeys.messageType] as String?),
        sentAt: DateTime.parse(json[ChatJsonKeys.sentAt] as String),
        deliveryStatus: _deliveryStatusFromName(
            json[ChatJsonKeys.deliveryStatus] as String?),
        senderName: json[ChatJsonKeys.senderName] as String?,
        mediaUrl: json[ChatJsonKeys.mediaUrl] as String?,
        mediaUrlExpiresAt:
            _parseNullableDate(json[ChatJsonKeys.mediaUrlExpiresAt] as String?),
        fileName: json[ChatJsonKeys.fileName] as String?,
        voiceDurationSeconds: json[ChatJsonKeys.voiceDurationSeconds] as int?,
        replyToId: json[ChatJsonKeys.replyToId] as String?,
        replyPreview: switch (json[ChatJsonKeys.replyPreview]) {
          final Map<String, dynamic> value =>
            ChatReplyPreviewProjection.fromJson(value),
          _ => null,
        },
        pinnedAt: _parseNullableDate(json[ChatJsonKeys.pinnedAt] as String?),
        pinnedBy: json[ChatJsonKeys.pinnedBy] as String?,
        deletedAt: _parseNullableDate(json[ChatJsonKeys.deletedAt] as String?),
        readReceipts: (json['read_receipts'] as List<dynamic>? ?? const [])
            .whereType<Map<String, dynamic>>()
            .map(ChatReadReceipt.fromJson)
            .toList(growable: false),
      );

  final String id;
  final String senderId;
  final String? circleId;
  final String? dmPeerId;

  /// Plain text passthrough: never HTML-interpreted here; escaping happens in
  /// the presentation layer, which renders text widgets without markup.
  final String content;
  final ChatMessageType type;

  /// UTC instant as delivered by the API; local display conversion is a
  /// presentation concern.
  final DateTime sentAt;
  final ChatDeliveryStatus deliveryStatus;
  final String? senderName;

  /// Short-lived media projection (FR-024); presigned for 7 days and never
  /// authoritative — access controllers renew before playback/download.
  final String? mediaUrl;
  final DateTime? mediaUrlExpiresAt;
  final String? fileName;
  final int? voiceDurationSeconds;
  final String? replyToId;
  final ChatReplyPreviewProjection? replyPreview;
  final DateTime? pinnedAt;
  final String? pinnedBy;
  final DateTime? deletedAt;
  final List<ChatReadReceipt> readReceipts;

  ChatMessage copyWith({
    String? content,
    String? mediaUrl,
    DateTime? mediaUrlExpiresAt,
    String? fileName,
    int? voiceDurationSeconds,
    DateTime? pinnedAt,
    String? pinnedBy,
    DateTime? deletedAt,
    ChatReplyPreviewProjection? replyPreview,
  }) =>
      ChatMessage(
        id: id,
        senderId: senderId,
        circleId: circleId,
        dmPeerId: dmPeerId,
        content: content ?? this.content,
        type: type,
        sentAt: sentAt,
        deliveryStatus: deliveryStatus,
        senderName: senderName,
        mediaUrl: mediaUrl,
        mediaUrlExpiresAt: mediaUrlExpiresAt,
        fileName: fileName,
        voiceDurationSeconds: voiceDurationSeconds,
        replyToId: replyToId,
        replyPreview: replyPreview ?? this.replyPreview,
        pinnedAt: pinnedAt ?? this.pinnedAt,
        pinnedBy: pinnedBy ?? this.pinnedBy,
        deletedAt: deletedAt ?? this.deletedAt,
        readReceipts: readReceipts,
      );
}

/// One sender-visible, currently authorized read fact.
class ChatReadReceipt {
  const ChatReadReceipt({required this.readerId, required this.readAt});

  factory ChatReadReceipt.fromJson(Map<String, dynamic> json) =>
      ChatReadReceipt(
        readerId: json[ChatJsonKeys.readerId] as String,
        readAt: DateTime.parse(json[ChatJsonKeys.readAt] as String),
      );

  final String readerId;
  final DateTime readAt;
}

/// One page of cursor-paginated history. [messages] arrives newest-first;
/// [nextBefore] is the `before` cursor for the next older page.
class ChatMessagePage {
  const ChatMessagePage({
    required this.messages,
    required this.hasMore,
    this.nextBefore,
  });

  factory ChatMessagePage.fromJson(Map<String, dynamic> json) =>
      ChatMessagePage(
        messages: (json[ChatJsonKeys.data] as List<dynamic>? ?? const [])
            .whereType<Map<String, dynamic>>()
            .map(ChatMessage.fromJson)
            .toList(growable: false),
        hasMore: json[ChatJsonKeys.hasMore] as bool? ?? false,
        nextBefore: json[ChatJsonKeys.nextBefore] as String?,
      );

  final List<ChatMessage> messages;
  final bool hasMore;
  final String? nextBefore;
}

/// Merges two message lists by id: incoming entries win (server-authoritative
/// replacement), duplicates collapse to one, and the result keeps the
/// deterministic `(sent_at, id)` descending order used by history pages.
List<ChatMessage> mergeChatMessages(
  Iterable<ChatMessage> existing,
  Iterable<ChatMessage> incoming,
) {
  final byId = {for (final message in existing) message.id: message};
  byId.addAll({for (final message in incoming) message.id: message});
  final merged = byId.values.toList()
    ..sort((a, b) {
      final byTime = b.sentAt.compareTo(a.sentAt);
      if (byTime != 0) return byTime;
      return b.id.compareTo(a.id);
    });
  return merged;
}

/// Whether a message is a client-local projection (optimistic or queued).
/// The server only ever emits `delivered`/`read`, so these never collide
/// with authoritative rows.
bool _isLocal(ChatMessage message) =>
    message.deliveryStatus == ChatDeliveryStatus.pending ||
    message.deliveryStatus == ChatDeliveryStatus.sent;

/// Reconciles the projection against an authoritative first page. Within the
/// page's `(sent_at, id)` coverage window, absence means the server deleted
/// or never committed the message, so it is removed; messages outside the
/// window (older pages, post-snapshot commits) and local pending/sent items
/// are kept. An empty page establishes no window, so nothing is removed
/// (FR-011 reconciliation; deletion convergence for FR-031).
List<ChatMessage> reconcileChatMessages(
  Iterable<ChatMessage> existing,
  Iterable<ChatMessage> page,
) {
  final merged = mergeChatMessages(existing, page);
  final pageList = page.toList(growable: false);
  if (pageList.length < 2) return merged;
  final newest = pageList.first;
  final oldest = pageList.last;
  final pageIds = {for (final message in pageList) message.id};
  return merged.where((message) {
    if (pageIds.contains(message.id) || _isLocal(message)) return true;
    // Keep anything strictly outside [oldest, newest]; only in-window
    // absentees are provably deleted.
    final newerThanNewest = message.sentAt.isAfter(newest.sentAt) ||
        (message.sentAt.isAtSameMomentAs(newest.sentAt) &&
            message.id.compareTo(newest.id) > 0);
    final olderThanOldest = message.sentAt.isBefore(oldest.sentAt) ||
        (message.sentAt.isAtSameMomentAs(oldest.sentAt) &&
            message.id.compareTo(oldest.id) < 0);
    return newerThanNewest || olderThanOldest;
  }).toList(growable: false);
}
