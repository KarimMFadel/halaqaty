import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/auth/data/auth_api_client.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_protocol_constants.dart';
import 'package:halaqaty_mobile/features/chat/domain/chat_models.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_api_client.dart';

export 'package:halaqaty_mobile/features/chat/domain/chat_models.dart';

/// Contract error envelope (`{error: {code, message}}`) surfaced as a typed
/// exception; presentation maps it to localized copy, never raw strings.
class ChatApiException implements Exception {
  const ChatApiException({
    required this.statusCode,
    required this.code,
    required this.message,
    this.retryAfterSeconds,
  });

  final int? statusCode;
  final String code;
  final String message;

  /// Parsed `Retry-After` header from a `429`, in seconds, when present.
  final int? retryAfterSeconds;

  @override
  String toString() => '$code: $message';
}

/// Maps a [DioException] (contract error envelope or transport failure) to
/// the typed [ChatApiException]; shared by all F-004 chat API clients.
ChatApiException mapChatApiException(DioException error) {
  final body = error.response?.data;
  final envelope =
      body is Map<String, dynamic> ? body[ChatJsonKeys.error] : null;
  final details = envelope is Map<String, dynamic> ? envelope : const {};
  final retryAfterRaw = error.response?.headers.value(ChatHeaders.retryAfter);
  return ChatApiException(
    statusCode: error.response?.statusCode,
    code: details[ChatJsonKeys.code] as String? ?? ChatApiErrors.requestFailed,
    message: details[ChatJsonKeys.message] as String? ??
        ChatApiErrors.requestFailedMessage,
    retryAfterSeconds:
        retryAfterRaw == null ? null : int.tryParse(retryAfterRaw),
  );
}

/// Dio client for the F-004 group chat REST surface. Reuses the shared
/// authenticated `dioProvider` and session headers; it owns no transport.
class ChatApiClient {
  ChatApiClient(this._dio);

  final Dio _dio;

  /// Lists the current eligible unordered-pair conversation.
  Future<ChatMessagePage> listDirectMessages({
    required String token,
    required String sessionId,
    required String userId,
    int? limit,
    String? before,
  }) async {
    try {
      final response = await _dio.get<Map<String, dynamic>>(
        ChatApiPaths.directMessages(userId),
        queryParameters: {
          if (limit != null) ChatJsonKeys.limit: limit,
          if (before != null) ChatJsonKeys.before: before,
        },
        options: Options(headers: sessionRequestHeaders(token, sessionId)),
      );
      return ChatMessagePage.fromJson(response.data!);
    } on DioException catch (error) {
      throw mapChatApiException(error);
    }
  }

  /// Sends one idempotent direct text message.
  Future<ChatMessage> sendDirectTextMessage({
    required String token,
    required String sessionId,
    required String userId,
    required String content,
    required String idempotencyKey,
  }) async {
    try {
      final response = await _dio.post<Map<String, dynamic>>(
        ChatApiPaths.directMessages(userId),
        data: {
          ChatJsonKeys.messageType: ChatRealtimeTypes.text,
          ChatJsonKeys.content: content,
        },
        options: Options(headers: {
          ...sessionRequestHeaders(token, sessionId),
          ChatHeaders.idempotencyKey: idempotencyKey,
        }),
      );
      return ChatMessage.fromJson(response.data!);
    } on DioException catch (error) {
      throw mapChatApiException(error);
    }
  }

  /// Authoritative newest-first history page (`listCircleMessages`).
  Future<ChatMessagePage> listMessages({
    required String token,
    required String sessionId,
    required String circleId,
    int? limit,
    String? before,
  }) async {
    try {
      final response = await _dio.get<Map<String, dynamic>>(
        ChatApiPaths.circleMessages(circleId),
        queryParameters: {
          if (limit != null) ChatJsonKeys.limit: limit,
          if (before != null) ChatJsonKeys.before: before,
        },
        options: Options(headers: sessionRequestHeaders(token, sessionId)),
      );
      return ChatMessagePage.fromJson(response.data!);
    } on DioException catch (error) {
      throw mapChatApiException(error);
    }
  }

  /// Searches retained group history using the server's normalized index.
  Future<ChatMessagePage> searchMessages({
    required String token,
    required String sessionId,
    required String circleId,
    required String query,
    int? limit,
    String? before,
  }) async {
    try {
      final response = await _dio.get<Map<String, dynamic>>(
        ChatApiPaths.searchCircleMessages(circleId),
        queryParameters: {
          ChatJsonKeys.query: query,
          if (limit != null) ChatJsonKeys.limit: limit,
          if (before != null) ChatJsonKeys.before: before,
        },
        options: Options(headers: sessionRequestHeaders(token, sessionId)),
      );
      return ChatMessagePage.fromJson(response.data!);
    } on DioException catch (error) {
      throw mapChatApiException(error);
    }
  }

  /// Records a durable read fact for an active-circle message.
  Future<void> markMessageRead({
    required String token,
    required String sessionId,
    required String circleId,
    required String messageId,
    required String idempotencyKey,
  }) async {
    try {
      await _dio.post<void>(
        ChatApiPaths.markCircleMessageRead(circleId, messageId),
        options: Options(headers: {
          ...sessionRequestHeaders(token, sessionId),
          ChatHeaders.idempotencyKey: idempotencyKey,
        }),
      );
    } on DioException catch (error) {
      throw mapChatApiException(error);
    }
  }

  /// Records a durable read fact for an eligible direct message.
  Future<void> markDirectMessageRead({
    required String token,
    required String sessionId,
    required String userId,
    required String messageId,
    required String idempotencyKey,
  }) async {
    try {
      await _dio.post<void>(
        ChatApiPaths.markDirectMessageRead(userId, messageId),
        options: Options(headers: {
          ...sessionRequestHeaders(token, sessionId),
          ChatHeaders.idempotencyKey: idempotencyKey,
        }),
      );
    } on DioException catch (error) {
      throw mapChatApiException(error);
    }
  }

  /// Sends one idempotent text message (`sendCircleMessage`). The
  /// idempotency key is stable per logical send so retries converge on one
  /// durable message.
  Future<ChatMessage> sendTextMessage({
    required String token,
    required String sessionId,
    required String circleId,
    required String content,
    required String idempotencyKey,
  }) async {
    try {
      final response = await _dio.post<Map<String, dynamic>>(
        ChatApiPaths.circleMessages(circleId),
        data: {
          ChatJsonKeys.messageType: ChatRealtimeTypes.text,
          ChatJsonKeys.content: content,
        },
        options: Options(headers: {
          ...sessionRequestHeaders(token, sessionId),
          ChatHeaders.idempotencyKey: idempotencyKey,
        }),
      );
      return ChatMessage.fromJson(response.data!);
    } on DioException catch (error) {
      throw mapChatApiException(error);
    }
  }

  /// Attaches one staged upload as a media message (`sendCircleMessage`
  /// with `upload_id`). Retries MUST reuse [idempotencyKey]: the upload
  /// attaches exactly once and a fresh key re-attaching a consumed upload
  /// is rejected with `409` by the server.
  Future<ChatMessage> sendMediaMessage({
    required String token,
    required String sessionId,
    required String circleId,
    required ChatMessageType type,
    required String uploadId,
    required String idempotencyKey,
  }) async {
    try {
      final response = await _dio.post<Map<String, dynamic>>(
        ChatApiPaths.circleMessages(circleId),
        data: {
          ChatJsonKeys.messageType: type.name,
          ChatJsonKeys.uploadId: uploadId,
        },
        options: Options(headers: {
          ...sessionRequestHeaders(token, sessionId),
          ChatHeaders.idempotencyKey: idempotencyKey,
        }),
      );
      return ChatMessage.fromJson(response.data!);
    } on DioException catch (error) {
      throw mapChatApiException(error);
    }
  }
}

final chatApiClientProvider = Provider<ChatApiClient>(
  (ref) => ChatApiClient(ref.watch(dioProvider)),
);
