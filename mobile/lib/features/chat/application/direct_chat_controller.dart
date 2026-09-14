import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';
import 'package:halaqaty_mobile/features/chat/application/chat_presence_controller.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_realtime_client.dart';

enum DirectChatStatus { idle, loading, ready, error, accessLost }

typedef DirectChatCredentials
    = Future<({String token, String sessionId, String userId})> Function();

class DirectChatControllerState {
  const DirectChatControllerState({
    this.status = DirectChatStatus.idle,
    this.messages = const [],
    this.hasMore = false,
    this.nextBefore,
    this.errorMessage,
  });

  final DirectChatStatus status;
  final List<ChatMessage> messages;
  final bool hasMore;
  final String? nextBefore;
  final String? errorMessage;

  DirectChatControllerState copyWith({
    DirectChatStatus? status,
    List<ChatMessage>? messages,
    bool? hasMore,
    String? nextBefore,
    String? errorMessage,
  }) =>
      DirectChatControllerState(
        status: status ?? this.status,
        messages: messages ?? this.messages,
        hasMore: hasMore ?? this.hasMore,
        nextBefore: nextBefore ?? this.nextBefore,
        errorMessage: errorMessage ?? this.errorMessage,
      );
}

class DirectChatController extends StateNotifier<DirectChatControllerState> {
  DirectChatController(this._api, this._credentials,
      {this.presence, ChatRealtimePresenceClient? realtime})
      : _realtime = realtime,
        super(const DirectChatControllerState());

  final ChatApiClient _api;
  final DirectChatCredentials _credentials;
  final ChatPresenceController? presence;
  final ChatRealtimePresenceClient? _realtime;
  StreamSubscription<ChatRealtimeEvent>? _subscription;
  String? _peerId;

  Future<void> open(String peerId) async {
    _peerId = peerId;
    state = const DirectChatControllerState(status: DirectChatStatus.loading);
    try {
      final credentials = await _credentials();
      final realtime = _realtime;
      if (realtime != null) {
        await _subscription?.cancel();
        _subscription = realtime
            .directChatEvents(peerId,
                token: credentials.token,
                backendSessionId: credentials.sessionId)
            .listen(handleRealtimeEvent);
      }
      final page = await _api.listDirectMessages(
        token: credentials.token,
        sessionId: credentials.sessionId,
        userId: peerId,
      );
      state = DirectChatControllerState(
        status: DirectChatStatus.ready,
        messages: page.messages,
        hasMore: page.hasMore,
        nextBefore: page.nextBefore,
      );
      for (final message in page.messages) {
        await presence?.markDirectMessageRead(message, peerId);
      }
    } catch (error) {
      state = DirectChatControllerState(
        status: _isAccessLost(error)
            ? DirectChatStatus.accessLost
            : DirectChatStatus.error,
        errorMessage: _safeError(error),
      );
    }
  }

  /// Sends an ephemeral typing command through the already authenticated DM
  /// connection. The server remains the authorization authority.
  Future<void> setTyping(bool isTyping) async {
    final peerId = _peerId;
    if (peerId == null || state.status != DirectChatStatus.ready) return;
    await _realtime?.sendTyping(dmPeerId: peerId, isTyping: isTyping);
  }

  /// Projects a shared authenticated realtime event while this DM is open.
  void handleRealtimeEvent(ChatRealtimeEvent event) {
    final peerId = _peerId;
    if (peerId == null) return;
    switch (event) {
      case final ChatMessageEvent message
          when message.message.dmPeerId == peerId:
        presence?.handleRealtimeEvent(event);
        state = state.copyWith(
            messages: mergeChatMessages(state.messages, [message.message]));
        unawaited(presence?.markDirectMessageRead(message.message, peerId));
      case final ChatMessageReadEvent read
          when state.messages.any((message) => message.id == read.messageId):
        presence?.handleRealtimeEvent(event);
        unawaited(_reconcile(peerId));
      case final ChatTypingEvent typing when typing.dmPeerId == peerId:
        presence?.handleRealtimeEvent(event);
      default:
        break;
    }
  }

  Future<void> _reconcile(String peerId) async {
    try {
      final credentials = await _credentials();
      final page = await _api.listDirectMessages(
        token: credentials.token,
        sessionId: credentials.sessionId,
        userId: peerId,
      );
      if (!mounted || _peerId != peerId) return;
      state = state.copyWith(
        messages: reconcileChatMessages(state.messages, page.messages),
        hasMore: page.hasMore,
        nextBefore: page.nextBefore,
      );
    } catch (_) {
      // A read receipt is only a projection hint; later events or reopening
      // still reconcile the durable conversation without clearing history.
    }
  }

  Future<void> loadOlder() async {
    final peerId = _peerId;
    if (peerId == null || !state.hasMore || state.nextBefore == null) return;
    try {
      final credentials = await _credentials();
      final page = await _api.listDirectMessages(
        token: credentials.token,
        sessionId: credentials.sessionId,
        userId: peerId,
        before: state.nextBefore,
      );
      state = state.copyWith(
        status: DirectChatStatus.ready,
        messages: mergeChatMessages(state.messages, page.messages),
        hasMore: page.hasMore,
        nextBefore: page.nextBefore,
      );
      for (final message in page.messages) {
        await presence?.markDirectMessageRead(message, peerId);
      }
    } catch (error) {
      state = state.copyWith(errorMessage: _safeError(error));
    }
  }

  @override
  void dispose() {
    unawaited(_subscription?.cancel());
    super.dispose();
  }

  Future<bool> sendText(String content) async {
    final peerId = _peerId;
    if (peerId == null || state.status != DirectChatStatus.ready) return false;
    if (validateChatText(content) != ChatTextValidation.valid) return false;
    try {
      final credentials = await _credentials();
      final message = await _api.sendDirectTextMessage(
        token: credentials.token,
        sessionId: credentials.sessionId,
        userId: peerId,
        content: content,
        idempotencyKey: newChatIdempotencyKey(),
      );
      state = state.copyWith(
          messages: mergeChatMessages(state.messages, [message]));
      return true;
    } catch (error) {
      if (_isAccessLost(error)) {
        state = DirectChatControllerState(
          status: DirectChatStatus.accessLost,
          errorMessage: _safeError(error),
        );
      } else {
        state = state.copyWith(errorMessage: _safeError(error));
      }
      return false;
    }
  }

  static bool _isAccessLost(Object error) =>
      error is ChatApiException &&
      (error.statusCode == 401 ||
          error.statusCode == 403 ||
          error.statusCode == 404);

  static String _safeError(Object error) {
    if (error is ChatApiException &&
        (error.statusCode == 401 ||
            error.statusCode == 403 ||
            error.statusCode == 404)) {
      return 'This conversation is unavailable';
    }
    return 'Chat request failed.';
  }
}

final directChatControllerProvider = StateNotifierProvider.autoDispose
    .family<DirectChatController, DirectChatControllerState, String>(
        (ref, peerId) {
  final auth = ref.watch(authControllerProvider);
  Future<({String token, String sessionId, String userId})>
      credentials() async {
    final user = ref.read(firebaseAuthProvider).currentUser;
    final sessionId = auth.sessionId;
    final token = await user?.getIdToken();
    final userId = auth.user?.id;
    if (token == null ||
        token.isEmpty ||
        sessionId == null ||
        sessionId.isEmpty ||
        userId == null) {
      throw StateError('User not authenticated');
    }
    return (token: token, sessionId: sessionId, userId: userId);
  }

  final presence =
      ChatPresenceController(ref.watch(chatApiClientProvider), credentials);
  ref.onDispose(presence.dispose);
  return DirectChatController(ref.watch(chatApiClientProvider), credentials,
      presence: presence,
      realtime:
          ref.watch(chatRealtimeClientProvider) as ChatRealtimePresenceClient);
});
