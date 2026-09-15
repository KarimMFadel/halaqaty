import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/auth/data/auth_api_client.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_protocol_constants.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_api_client.dart';

/// The safe, server-projected portion of a message used when replying.
class ChatReplyPreview {
  const ChatReplyPreview({
    required this.id,
    required this.senderName,
    required this.preview,
    required this.deleted,
  });

  final String id;
  final String senderName;
  final String preview;
  final bool deleted;

  factory ChatReplyPreview.fromMessage(ChatMessage message) {
    final projection = message.replyPreview;
    return ChatReplyPreview(
      id: message.replyToId ?? projection?.id ?? message.id,
      senderName: projection?.senderName ?? message.senderName ?? '',
      preview: projection?.preview ?? '',
      deleted: projection?.deleted ?? false,
    ).redactedIfDeleted();
  }

  /// Deleted content is never retained in a local reply projection.
  ChatReplyPreview redactedIfDeleted() => deleted
      ? ChatReplyPreview(
          id: id,
          senderName: senderName,
          preview: '',
          deleted: true,
        )
      : this;
}

/// REST operations needed by chat discovery. Keeping this small makes the
/// controller usable with an in-memory API in tests and with Dio in production.
abstract interface class ChatDiscoveryApi {
  Future<ChatMessage> sendReply({
    required String token,
    required String sessionId,
    required String circleId,
    required String content,
    required String? replyToId,
  }) =>
      throw UnimplementedError();

  Future<ChatMessagePage> searchMessages({
    required String token,
    required String sessionId,
    required String circleId,
    required String query,
  });

  Future<List<ChatMessage>> listPinnedMessages({
    required String token,
    required String sessionId,
    required String circleId,
  });

  Future<ChatMessage> pinMessage({
    required String token,
    required String sessionId,
    required String circleId,
    required String messageId,
  });

  Future<void> unpinMessage({
    required String token,
    required String sessionId,
    required String circleId,
    required String messageId,
  });
}

typedef ChatDiscoveryCredentials
    = Future<({String token, String sessionId, String userId})> Function();

enum ChatDiscoveryStatus { idle, loading, ready, error }

enum ChatDiscoveryFailure { unauthorized, conflict, unavailable, unknown }

class ChatDiscoveryState {
  const ChatDiscoveryState({
    this.status = ChatDiscoveryStatus.idle,
    this.replyTarget,
    this.searchResults = const [],
    this.pinnedMessages = const [],
    this.pinLimitReached = false,
    this.readOnly = false,
    this.errorMessage,
    this.failure,
    this.draft = '',
  });

  final ChatDiscoveryStatus status;
  final ChatReplyPreview? replyTarget;
  final List<ChatMessage> searchResults;
  final List<ChatMessage> pinnedMessages;
  final bool pinLimitReached;
  final bool readOnly;
  final String? errorMessage;
  final ChatDiscoveryFailure? failure;
  final String draft;

  ChatDiscoveryState copyWith({
    ChatDiscoveryStatus? status,
    ChatReplyPreview? replyTarget,
    bool clearReplyTarget = false,
    List<ChatMessage>? searchResults,
    List<ChatMessage>? pinnedMessages,
    bool? pinLimitReached,
    bool? readOnly,
    String? errorMessage,
    bool clearError = false,
    ChatDiscoveryFailure? failure,
    bool clearFailure = false,
    String? draft,
  }) =>
      ChatDiscoveryState(
        status: status ?? this.status,
        replyTarget:
            clearReplyTarget ? null : (replyTarget ?? this.replyTarget),
        searchResults: searchResults ?? this.searchResults,
        pinnedMessages: pinnedMessages ?? this.pinnedMessages,
        pinLimitReached: pinLimitReached ?? this.pinLimitReached,
        readOnly: readOnly ?? this.readOnly,
        errorMessage: clearError ? null : (errorMessage ?? this.errorMessage),
        failure: clearFailure ? null : (failure ?? this.failure),
        draft: draft ?? this.draft,
      );
}

class ChatDiscoveryController extends StateNotifier<ChatDiscoveryState> {
  ChatDiscoveryController(this._api, this._credentials)
      : super(const ChatDiscoveryState());

  final ChatDiscoveryApi _api;
  final ChatDiscoveryCredentials _credentials;

  void setReadOnly(bool value) {
    if (!mounted) return;
    state = state.copyWith(readOnly: value);
  }

  /// Selects a reply target while ensuring deleted content is not kept.
  void selectReplyTarget(ChatReplyPreview target) {
    state = state.copyWith(replyTarget: target.redactedIfDeleted());
  }

  void clearReplyTarget() {
    state = state.copyWith(clearReplyTarget: true);
  }

  Future<bool> sendReply(String circleId, String content) async {
    final target = state.replyTarget;
    if (target == null ||
        state.readOnly ||
        validateChatText(content) != ChatTextValidation.valid) {
      return false;
    }
    state = state.copyWith(draft: content, clearError: true);
    try {
      final credentials = await _credentials();
      await _api.sendReply(
        token: credentials.token,
        sessionId: credentials.sessionId,
        circleId: circleId,
        content: content.trim(),
        replyToId: target.id,
      );
      if (!mounted) return false;
      state = state.copyWith(
          draft: '',
          clearReplyTarget: true,
          clearError: true,
          clearFailure: true);
      return true;
    } catch (error) {
      if (!mounted) return false;
      state = state.copyWith(
        errorMessage: _safeErrorMessage(error),
        failure: _failureFor(error),
      );
      return false;
    }
  }

  /// Searches the server's normalized retained-history index. The query is
  /// intentionally passed through unchanged so Arabic and Latin normalization
  /// stays identical to the backend contract.
  Future<void> search(String circleId, String query) async {
    final trimmed = query.trim();
    if (trimmed.runes.length < 2) {
      state = state.copyWith(
        status: ChatDiscoveryStatus.ready,
        searchResults: const [],
        clearError: true,
      );
      return;
    }
    state =
        state.copyWith(status: ChatDiscoveryStatus.loading, clearError: true);
    try {
      final credentials = await _credentials();
      final page = await _api.searchMessages(
        token: credentials.token,
        sessionId: credentials.sessionId,
        circleId: circleId,
        query: trimmed,
      );
      if (!mounted) return;
      state = state.copyWith(
        status: ChatDiscoveryStatus.ready,
        searchResults: page.messages,
        clearFailure: true,
      );
    } catch (error) {
      if (!mounted) return;
      state = state.copyWith(
        status: ChatDiscoveryStatus.error,
        errorMessage: _safeErrorMessage(error),
        failure: _failureFor(error),
      );
    }
  }

  Future<void> loadPinned(String circleId) async {
    state =
        state.copyWith(status: ChatDiscoveryStatus.loading, clearError: true);
    try {
      final credentials = await _credentials();
      final messages = await _api.listPinnedMessages(
        token: credentials.token,
        sessionId: credentials.sessionId,
        circleId: circleId,
      );
      if (!mounted) return;
      // The API's newest-first order is authoritative; don't sort locally.
      state = state.copyWith(
        status: ChatDiscoveryStatus.ready,
        pinnedMessages: List.unmodifiable(messages),
        pinLimitReached: false,
        clearFailure: true,
      );
    } catch (error) {
      if (!mounted) return;
      state = state.copyWith(
        status: ChatDiscoveryStatus.error,
        errorMessage: _safeErrorMessage(error),
        failure: _failureFor(error),
      );
    }
  }

  Future<void> pin(String circleId, String messageId) async {
    if (state.readOnly) return;
    try {
      final credentials = await _credentials();
      final message = await _api.pinMessage(
        token: credentials.token,
        sessionId: credentials.sessionId,
        circleId: circleId,
        messageId: messageId,
      );
      if (!mounted) return;
      final messages = [
        message,
        ...state.pinnedMessages.where((item) => item.id != message.id),
      ];
      state = state.copyWith(
        status: ChatDiscoveryStatus.ready,
        pinnedMessages: List.unmodifiable(messages.take(5)),
        pinLimitReached: false,
        clearError: true,
        clearFailure: true,
      );
    } catch (error) {
      if (!mounted) return;
      state = state.copyWith(
        pinLimitReached: _isPinConflict(error),
        errorMessage: _safeErrorMessage(error),
        failure: _failureFor(error),
      );
    }
  }

  Future<void> unpin(String circleId, String messageId) async {
    if (state.readOnly) return;
    try {
      final credentials = await _credentials();
      await _api.unpinMessage(
        token: credentials.token,
        sessionId: credentials.sessionId,
        circleId: circleId,
        messageId: messageId,
      );
      if (!mounted) return;
      state = state.copyWith(
        status: ChatDiscoveryStatus.ready,
        pinnedMessages: state.pinnedMessages
            .where((message) => message.id != messageId)
            .toList(growable: false),
        clearError: true,
        clearFailure: true,
      );
    } catch (error) {
      if (!mounted) return;
      state = state.copyWith(
        errorMessage: _safeErrorMessage(error),
        failure: _failureFor(error),
      );
    }
  }

  static bool _isPinConflict(Object error) =>
      error is ChatApiException && error.code == 'ERR_CONFLICT';

  static ChatDiscoveryFailure _failureFor(Object error) {
    if (error is ChatApiException) {
      if (error.code == 'ERR_CONFLICT') return ChatDiscoveryFailure.conflict;
      if (error.statusCode == 401 || error.statusCode == 403) {
        return ChatDiscoveryFailure.unauthorized;
      }
      if (error.statusCode != null && error.statusCode! >= 500) {
        return ChatDiscoveryFailure.unavailable;
      }
    }
    return ChatDiscoveryFailure.unknown;
  }

  static String _safeErrorMessage(Object error) => switch (_failureFor(error)) {
        ChatDiscoveryFailure.unauthorized => 'Chat access is unavailable.',
        ChatDiscoveryFailure.conflict =>
          'Chat action conflicts with current state.',
        ChatDiscoveryFailure.unavailable =>
          'Chat service is temporarily unavailable.',
        ChatDiscoveryFailure.unknown => 'Chat request failed.',
      };
}

/// Dio implementation of the discovery subset of the canonical REST client.
class DioChatDiscoveryApi implements ChatDiscoveryApi {
  DioChatDiscoveryApi(this._dio);

  final Dio _dio;

  @override
  Future<ChatMessagePage> searchMessages({
    required String token,
    required String sessionId,
    required String circleId,
    required String query,
  }) async {
    try {
      final response = await _dio.get<Map<String, dynamic>>(
        ChatApiPaths.searchCircleMessages(circleId),
        queryParameters: {ChatJsonKeys.query: query},
        options: Options(headers: sessionRequestHeaders(token, sessionId)),
      );
      return ChatMessagePage.fromJson(response.data!);
    } on DioException catch (error) {
      throw mapChatApiException(error);
    }
  }

  @override
  Future<ChatMessage> sendReply({
    required String token,
    required String sessionId,
    required String circleId,
    required String content,
    required String? replyToId,
  }) async {
    try {
      final response = await _dio.post<Map<String, dynamic>>(
        ChatApiPaths.circleMessages(circleId),
        data: {
          ChatJsonKeys.messageType: ChatRealtimeTypes.text,
          ChatJsonKeys.content: content,
          if (replyToId != null) ChatJsonKeys.replyToId: replyToId,
        },
        options: Options(headers: {
          ...sessionRequestHeaders(token, sessionId),
          ChatHeaders.idempotencyKey: newChatIdempotencyKey(),
        }),
      );
      return ChatMessage.fromJson(response.data!);
    } on DioException catch (error) {
      throw mapChatApiException(error);
    }
  }

  @override
  Future<List<ChatMessage>> listPinnedMessages({
    required String token,
    required String sessionId,
    required String circleId,
  }) async {
    try {
      final response = await _dio.get<Map<String, dynamic>>(
        ChatApiPaths.pinnedCircleMessages(circleId),
        options: Options(headers: sessionRequestHeaders(token, sessionId)),
      );
      final data = response.data?[ChatJsonKeys.data];
      return (data is List ? data : const <dynamic>[])
          .whereType<Map<String, dynamic>>()
          .map(ChatMessage.fromJson)
          .toList(growable: false);
    } on DioException catch (error) {
      throw mapChatApiException(error);
    }
  }

  @override
  Future<ChatMessage> pinMessage({
    required String token,
    required String sessionId,
    required String circleId,
    required String messageId,
  }) =>
      _mutateMessage(
        path: ChatApiPaths.pinCircleMessage(circleId, messageId),
        token: token,
        sessionId: sessionId,
      );

  @override
  Future<void> unpinMessage({
    required String token,
    required String sessionId,
    required String circleId,
    required String messageId,
  }) async {
    try {
      await _dio.delete<void>(
        ChatApiPaths.pinCircleMessage(circleId, messageId),
        options: Options(headers: {
          ...sessionRequestHeaders(token, sessionId),
          ChatHeaders.idempotencyKey: newChatIdempotencyKey(),
        }),
      );
    } on DioException catch (error) {
      throw mapChatApiException(error);
    }
  }

  Future<ChatMessage> _mutateMessage({
    required String path,
    required String token,
    required String sessionId,
  }) async {
    try {
      final response = await _dio.post<Map<String, dynamic>>(
        path,
        options: Options(headers: {
          ...sessionRequestHeaders(token, sessionId),
          ChatHeaders.idempotencyKey: newChatIdempotencyKey(),
        }),
      );
      return ChatMessage.fromJson(response.data!);
    } on DioException catch (error) {
      throw mapChatApiException(error);
    }
  }
}

final chatDiscoveryControllerProvider = StateNotifierProvider.autoDispose
    .family<ChatDiscoveryController, ChatDiscoveryState, String>(
  (ref, _) {
    final auth = ref.watch(authControllerProvider);
    Future<({String token, String sessionId, String userId})>
        credentials() async {
      final firebaseUser = ref.read(firebaseAuthProvider).currentUser;
      final token = await firebaseUser?.getIdToken();
      final sessionId = auth.sessionId;
      final userId = auth.user?.id;
      if (token == null ||
          token.isEmpty ||
          sessionId == null ||
          userId == null) {
        throw StateError('User not authenticated');
      }
      return (token: token, sessionId: sessionId, userId: userId);
    }

    return ChatDiscoveryController(
      DioChatDiscoveryApi(ref.watch(dioProvider)),
      credentials,
    );
  },
);
