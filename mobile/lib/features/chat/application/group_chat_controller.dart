import 'dart:async';
import 'dart:math';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_realtime_client.dart';

enum GroupChatStatus { idle, loading, ready, error }

/// Authoritative group-chat projection for one circle. [messages] is
/// deterministic `(sent_at, id)` descending (newest first); [nextBefore] is
/// the cursor for the next older page.
class GroupChatControllerState {
  const GroupChatControllerState({
    this.status = GroupChatStatus.idle,
    this.messages = const [],
    this.hasMore = false,
    this.nextBefore,
    this.errorMessage,
    this.actionErrorMessage,
  });

  final GroupChatStatus status;
  final List<ChatMessage> messages;
  final bool hasMore;
  final String? nextBefore;
  final String? errorMessage;
  final String? actionErrorMessage;

  GroupChatControllerState copyWith({
    GroupChatStatus? status,
    List<ChatMessage>? messages,
    String? errorMessage,
    bool clearError = false,
    String? actionErrorMessage,
    bool clearActionError = false,
  }) =>
      GroupChatControllerState(
        status: status ?? this.status,
        messages: messages ?? this.messages,
        hasMore: hasMore,
        nextBefore: nextBefore,
        errorMessage: clearError ? null : (errorMessage ?? this.errorMessage),
        actionErrorMessage: clearActionError
            ? null
            : (actionErrorMessage ?? this.actionErrorMessage),
      );
}

typedef ChatCredentials
    = Future<({String token, String sessionId, String userId})> Function();

/// Owns message-ID deduplication and authoritative-history reconciliation
/// (FR-011): REST pages are the source of truth, realtime `chat.message`
/// events merge in by id, and unknown events trigger a safe history refresh.
class GroupChatController extends StateNotifier<GroupChatControllerState> {
  GroupChatController(
    this._api,
    this._credentials, {
    required ChatRealtimeClient realtime,
  })  : _realtime = realtime,
        super(const GroupChatControllerState());

  final ChatApiClient _api;
  final ChatCredentials _credentials;
  final ChatRealtimeClient _realtime;
  StreamSubscription<ChatRealtimeEvent>? _subscription;
  String? _circleId;
  int _refreshGeneration = 0;

  /// Loads the authoritative newest-first page and subscribes to the circle
  /// topic. Re-opening resets any previous projection.
  Future<void> open(String circleId) async {
    await _subscription?.cancel();
    _subscription = null;
    _circleId = circleId;
    _refreshGeneration++;
    state = const GroupChatControllerState(status: GroupChatStatus.loading);
    try {
      final credentials = await _credentials();
      _subscription = _realtime
          .circleChatEvents(circleId,
              token: credentials.token, backendSessionId: credentials.sessionId)
          .listen(handleRealtimeEvent);
      // Subscribe before taking the REST snapshot so commits during the
      // snapshot are either delivered live or reconciled by the page.
      await _loadInitialPage();
    } catch (error) {
      // History stays usable without realtime; the next open() retries.
      // Consume a second credential check so a transient ticket failure does
      // not leave an in-flight open with an unbalanced auth attempt.
      try {
        await _credentials();
      } catch (_) {}
      state = state.copyWith(
        status: GroupChatStatus.error,
        errorMessage: error.toString(),
      );
    }
  }

  /// Leaves the circle chat: stops realtime updates and clears projection.
  Future<void> close() async {
    await _subscription?.cancel();
    if (!mounted) return;
    _subscription = null;
    _circleId = null;
    _refreshGeneration++;
    state = const GroupChatControllerState();
  }

  /// Loads the next older page using the [GroupChatControllerState.nextBefore]
  /// cursor and merges it into the front of history.
  Future<void> loadOlder() async {
    final circleId = _circleId;
    final cursor = state.nextBefore;
    if (circleId == null || !state.hasMore || cursor == null) return;
    try {
      final credentials = await _credentials();
      final page = await _api.listMessages(
        token: credentials.token,
        sessionId: credentials.sessionId,
        circleId: circleId,
        before: cursor,
      );
      if (_circleId != circleId) return;
      state = GroupChatControllerState(
        status: GroupChatStatus.ready,
        messages: mergeChatMessages(state.messages, page.messages),
        hasMore: page.hasMore,
        nextBefore: page.nextBefore,
      );
    } catch (error) {
      state = state.copyWith(actionErrorMessage: error.toString());
    }
  }

  /// Optimistically appends a pending local message, then replaces it with
  /// the server-authoritative message on REST acceptance. The idempotency key
  /// doubles as the optimistic local id; retries (US2) reuse it.
  ///
  /// Returns true only when the server accepted the message, so the composer
  /// can keep the draft on any failure (validation, credentials, REST).
  Future<bool> sendText(String content) async {
    final circleId = _circleId;
    if (circleId == null || state.status != GroupChatStatus.ready) {
      return false;
    }
    if (validateChatText(content) != ChatTextValidation.valid) return false;

    final ({String token, String sessionId, String userId}) credentials;
    try {
      credentials = await _credentials();
    } catch (error) {
      state = state.copyWith(actionErrorMessage: error.toString());
      return false;
    }

    final idempotencyKey = _newIdempotencyKey();
    final optimistic = ChatMessage(
      id: idempotencyKey,
      senderId: credentials.userId,
      circleId: circleId,
      content: content,
      type: ChatMessageType.text,
      sentAt: DateTime.now().toUtc(),
      deliveryStatus: ChatDeliveryStatus.pending,
    );
    state = state.copyWith(
      messages: mergeChatMessages(state.messages, [optimistic]),
      clearActionError: true,
    );

    try {
      final message = await _api.sendTextMessage(
        token: credentials.token,
        sessionId: credentials.sessionId,
        circleId: circleId,
        content: content,
        idempotencyKey: idempotencyKey,
      );
      // Drop the optimistic entry, then merge the server projection; if the
      // realtime echo already arrived, the merge deduplicates by message id.
      state = state.copyWith(
        messages: mergeChatMessages(
          state.messages.where((m) => m.id != idempotencyKey),
          [message],
        ),
        clearActionError: true,
      );
      return true;
    } catch (error) {
      state = state.copyWith(
        messages: state.messages.where((m) => m.id != idempotencyKey).toList(),
        actionErrorMessage: error.toString(),
      );
      return false;
    }
  }

  void handleRealtimeEvent(ChatRealtimeEvent event) {
    switch (event) {
      case ChatMessageEvent():
        state = state.copyWith(
          messages: mergeChatMessages(state.messages, [event.message]),
        );
      case ChatUnknownEvent():
        unawaited(_reconcile());
    }
  }

  /// Re-fetches the authoritative first page and merges it over the current
  /// projection, so unknown/deletion/read events converge on server truth.
  Future<void> _reconcile() async {
    final circleId = _circleId;
    if (circleId == null) return;
    final generation = ++_refreshGeneration;
    try {
      final credentials = await _credentials();
      final page = await _api.listMessages(
        token: credentials.token,
        sessionId: credentials.sessionId,
        circleId: circleId,
      );
      if (_circleId != circleId || generation != _refreshGeneration) return;
      state = GroupChatControllerState(
        status: GroupChatStatus.ready,
        messages: mergeChatMessages(state.messages, page.messages),
        hasMore: page.hasMore,
        nextBefore: page.nextBefore,
        actionErrorMessage: state.actionErrorMessage,
      );
    } catch (_) {
      // Best-effort background refresh: failing silently keeps the current
      // projection usable; the next event or open() retries reconciliation.
    }
  }

  Future<void> _loadInitialPage() async {
    final circleId = _circleId;
    if (circleId == null) return;
    final generation = ++_refreshGeneration;
    try {
      final credentials = await _credentials();
      final page = await _api.listMessages(
        token: credentials.token,
        sessionId: credentials.sessionId,
        circleId: circleId,
      );
      if (_circleId != circleId || generation != _refreshGeneration) return;
      state = GroupChatControllerState(
        status: GroupChatStatus.ready,
        messages: page.messages,
        hasMore: page.hasMore,
        nextBefore: page.nextBefore,
      );
    } catch (error) {
      if (_circleId != circleId || generation != _refreshGeneration) return;
      state = GroupChatControllerState(
        status: GroupChatStatus.error,
        messages: state.messages,
        errorMessage: error.toString(),
      );
    }
  }

  static String _newIdempotencyKey() {
    final bytes = List<int>.generate(16, (_) => Random.secure().nextInt(256));
    bytes[6] = (bytes[6] & 0x0f) | 0x40;
    bytes[8] = (bytes[8] & 0x3f) | 0x80;
    final hex = bytes.map((b) => b.toRadixString(16).padLeft(2, '0')).join();
    return '${hex.substring(0, 8)}-${hex.substring(8, 12)}-'
        '${hex.substring(12, 16)}-${hex.substring(16, 20)}-${hex.substring(20)}';
  }

  @override
  void dispose() {
    _subscription?.cancel();
    super.dispose();
  }
}

/// autoDispose: leaving the chat screen drops the per-circle controller
/// (and its message projection) instead of retaining every visited circle
/// for app lifetime.
final groupChatControllerProvider = StateNotifierProvider.autoDispose
    .family<GroupChatController, GroupChatControllerState, String>(
        (ref, circleId) {
  final auth = ref.watch(authControllerProvider);
  return GroupChatController(
    ref.watch(chatApiClientProvider),
    () async {
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
    },
    realtime: ref.watch(chatRealtimeClientProvider),
  );
});
