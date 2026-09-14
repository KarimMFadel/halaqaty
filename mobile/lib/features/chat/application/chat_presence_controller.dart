import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_realtime_client.dart';

/// Credentials required for a presence mutation.
typedef PresenceCredentials
    = Future<({String token, String sessionId, String userId})> Function();

/// Ephemeral typing state for one conversation.
class ChatTypingUser {
  const ChatTypingUser({
    required this.userId,
    required this.expiresAt,
    this.circleId,
    this.dmPeerId,
  });
  final String userId;
  final DateTime expiresAt;
  final String? circleId;
  final String? dmPeerId;
}

/// Read and typing projections kept separate from durable message history.
class ChatPresenceState {
  const ChatPresenceState(
      {this.readReceipts = const {}, this.typing = const {}});
  final Map<String, List<ChatReadReceipt>> readReceipts;
  final Map<String, ChatTypingUser> typing;

  ChatPresenceState copyWith({
    Map<String, List<ChatReadReceipt>>? readReceipts,
    Map<String, ChatTypingUser>? typing,
  }) =>
      ChatPresenceState(
        readReceipts: readReceipts ?? this.readReceipts,
        typing: typing ?? this.typing,
      );
}

/// Coordinates durable read submission and transient realtime presence.
class ChatPresenceController extends StateNotifier<ChatPresenceState> {
  ChatPresenceController(this._api, this._credentials)
      : super(const ChatPresenceState());

  final ChatApiClient _api;
  final PresenceCredentials _credentials;
  final Map<String, Timer> _expiryTimers = {};

  /// Current typing users for the active chat projection.
  List<String> get typingUserIds =>
      state.typing.values.map((user) => user.userId).toList(growable: false);

  /// Returns typing users only for one active group or direct conversation.
  List<String> typingUserIdsFor({String? circleId, String? dmPeerId}) {
    if ((circleId == null) == (dmPeerId == null)) return const [];
    return state.typing.values
        .where((user) => user.circleId == circleId && user.dmPeerId == dmPeerId)
        .map((user) => user.userId)
        .toList(growable: false);
  }

  /// Records a group read fact; callers should not invoke this for archived
  /// conversations or messages authored by the current user.
  Future<void> markGroupMessageRead(
      ChatMessage message, String circleId) async {
    final credentials = await _credentials();
    if (message.senderId == credentials.userId) return;
    await _api.markMessageRead(
      token: credentials.token,
      sessionId: credentials.sessionId,
      circleId: circleId,
      messageId: message.id,
      idempotencyKey: newChatIdempotencyKey(),
    );
  }

  /// Records a direct-message read fact unless the current user sent it.
  Future<void> markDirectMessageRead(ChatMessage message, String userId) async {
    final credentials = await _credentials();
    if (message.senderId == credentials.userId) return;
    await _api.markDirectMessageRead(
      token: credentials.token,
      sessionId: credentials.sessionId,
      userId: userId,
      messageId: message.id,
      idempotencyKey: newChatIdempotencyKey(),
    );
  }

  /// Applies one decoded realtime event. Read facts deduplicate by reader;
  /// typing is removed on stop or automatically at the server expiry.
  void handleRealtimeEvent(ChatRealtimeEvent event) {
    switch (event) {
      case final ChatMessageReadEvent read:
        final current = [...(state.readReceipts[read.messageId] ?? const [])];
        if (!current.any((receipt) => receipt.readerId == read.readerId)) {
          current.add(
              ChatReadReceipt(readerId: read.readerId, readAt: read.readAt));
          state = state.copyWith(readReceipts: {
            ...state.readReceipts,
            read.messageId: List.unmodifiable(current),
          });
        }
      case final ChatTypingEvent typing:
        final key = typing.circleId ?? typing.dmPeerId ?? '';
        final next = {...state.typing};
        final timerKey = '$key:${typing.userId}';
        _expiryTimers.remove(timerKey)?.cancel();
        if (!typing.isTyping || !typing.expiresAt.isAfter(DateTime.now())) {
          next.remove(timerKey);
        } else {
          next[timerKey] = ChatTypingUser(
            userId: typing.userId,
            expiresAt: typing.expiresAt,
            circleId: typing.circleId,
            dmPeerId: typing.dmPeerId,
          );
          _expiryTimers[timerKey] = Timer(
            typing.expiresAt.difference(DateTime.now()),
            () {
              if (!mounted) return;
              final updated = {...state.typing}..remove(timerKey);
              state = state.copyWith(typing: updated);
            },
          );
        }
        state = state.copyWith(typing: next);
      default:
        break;
    }
  }

  @override
  void dispose() {
    for (final timer in _expiryTimers.values) {
      timer.cancel();
    }
    super.dispose();
  }
}
