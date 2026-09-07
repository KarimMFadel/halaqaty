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
  });

  final int? statusCode;
  final String code;
  final String message;

  @override
  String toString() => '$code: $message';
}

/// Dio client for the F-004 group chat REST surface. Reuses the shared
/// authenticated `dioProvider` and session headers; it owns no transport.
class ChatApiClient {
  ChatApiClient(this._dio);

  final Dio _dio;

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
      throw _exception(error);
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
      throw _exception(error);
    }
  }

  ChatApiException _exception(DioException error) {
    final body = error.response?.data;
    final envelope =
        body is Map<String, dynamic> ? body[ChatJsonKeys.error] : null;
    final details = envelope is Map<String, dynamic> ? envelope : const {};
    return ChatApiException(
      statusCode: error.response?.statusCode,
      code:
          details[ChatJsonKeys.code] as String? ?? ChatApiErrors.requestFailed,
      message: details[ChatJsonKeys.message] as String? ??
          ChatApiErrors.requestFailedMessage,
    );
  }
}

final chatApiClientProvider = Provider<ChatApiClient>(
  (ref) => ChatApiClient(ref.watch(dioProvider)),
);
