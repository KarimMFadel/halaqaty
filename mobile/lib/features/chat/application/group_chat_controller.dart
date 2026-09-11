import 'dart:async';

import 'package:dio/dio.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/chat/application/chat_delivery_controller.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_realtime_client.dart';
import 'package:halaqaty_mobile/features/chat/data/pending_message_store.dart';

enum GroupChatStatus { idle, loading, ready, error, accessLost }

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
    this.terminalFailures = const {},
    this.readOnly = false,
  });

  final GroupChatStatus status;
  final List<ChatMessage> messages;
  final bool hasMore;
  final String? nextBefore;
  final String? errorMessage;
  final String? actionErrorMessage;
  final Map<String, String> terminalFailures;
  final bool readOnly;

  GroupChatControllerState copyWith({
    GroupChatStatus? status,
    List<ChatMessage>? messages,
    String? errorMessage,
    bool clearError = false,
    String? actionErrorMessage,
    bool clearActionError = false,
    Map<String, String>? terminalFailures,
    bool? readOnly,
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
        terminalFailures: terminalFailures ?? this.terminalFailures,
        readOnly: readOnly ?? this.readOnly,
      );
}

typedef ChatCredentials
    = Future<({String token, String sessionId, String userId})> Function();
typedef ChatRetryDelay = Future<void> Function(Duration delay);

/// Owns message-ID deduplication and authoritative-history reconciliation
/// (FR-011): REST pages are the source of truth, realtime `chat.message`
/// events merge in by id, and unknown events trigger a safe history refresh.
class GroupChatController extends StateNotifier<GroupChatControllerState> {
  GroupChatController(
    this._api,
    this._credentials, {
    required ChatRealtimeClient realtime,
    PendingMessageStore? pendingStore,
    ChatRetryDelay? retryDelay,
  })  : _realtime = realtime,
        _pendingStore = pendingStore,
        _retryDelay = retryDelay ?? Future<void>.delayed,
        super(const GroupChatControllerState());

  final ChatApiClient _api;
  final ChatCredentials _credentials;
  final ChatRealtimeClient _realtime;
  final PendingMessageStore? _pendingStore;
  final ChatRetryDelay _retryDelay;
  final Set<String> _activeRetries = {};
  StreamSubscription<ChatRealtimeEvent>? _subscription;
  String? _circleId;
  int _refreshGeneration = 0;
  bool _readOnly = false;

  /// Switches the projection to retained-history mode after circle archival.
  void setReadOnly(bool value) {
    _readOnly = value;
    if (!mounted) return;
    state = state.copyWith(readOnly: value);
  }

  /// Loads the authoritative newest-first page and subscribes to the circle
  /// topic. Re-opening resets any previous projection.
  Future<void> open(String circleId) async {
    await _subscription?.cancel();
    _subscription = null;
    _circleId = circleId;
    _refreshGeneration++;
    state = GroupChatControllerState(
      status: GroupChatStatus.loading,
      readOnly: _readOnly,
    );
    try {
      final credentials = await _credentials();
      _subscription = _realtime
          .circleChatEvents(circleId,
              token: credentials.token, backendSessionId: credentials.sessionId)
          .listen(handleRealtimeEvent);
      // Subscribe before taking the REST snapshot so commits during the
      // snapshot are either delivered live or reconciled by the page.
      await _loadInitialPage();
      await _restorePending(circleId);
      unawaited(retryPending());
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
        readOnly: _readOnly,
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
    if (circleId == null || state.status != GroupChatStatus.ready || state.readOnly) {
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
    final envelope = PendingMessageEnvelope(
      idempotencyKey: idempotencyKey,
      circleId: circleId,
      content: content,
      updatedAt: DateTime.now().toUtc(),
    );
    await _pendingStore?.save(envelope);

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
      await _pendingStore?.discard(idempotencyKey);
      return true;
    } catch (error) {
      state = state.copyWith(
        messages: _pendingStore == null
            ? state.messages.where((m) => m.id != idempotencyKey).toList()
            : state.messages,
        actionErrorMessage: error.toString(),
      );
      if (_isRetryable(error)) {
        unawaited(_retry(envelope));
      } else {
        _markTerminal(envelope.idempotencyKey, error);
      }
      return false;
    }
  }

  /// Retries durable pending envelopes using their original idempotency keys.
  /// Call this after reconnect or an explicit user retry.
  Future<void> retryPending() async {
    final circleId = _circleId;
    final store = _pendingStore;
    if (circleId == null || store == null) return;
    for (final envelope in await store.loadAll()) {
      if (envelope.circleId != circleId) continue;
      await _retry(envelope);
    }
  }

  /// Replaces a terminal pending draft while preserving its idempotency key.
  Future<void> editPending(String idempotencyKey, String content) async {
    if (validateChatText(content) != ChatTextValidation.valid) return;
    await _pendingStore?.edit(idempotencyKey, content);
    final failures = Map<String, String>.of(state.terminalFailures)
      ..remove(idempotencyKey);
    state = state.copyWith(
      messages: state.messages
          .map((message) => message.id == idempotencyKey
              ? ChatMessage(
                  id: message.id,
                  senderId: message.senderId,
                  circleId: message.circleId,
                  content: content,
                  type: message.type,
                  sentAt: message.sentAt,
                  deliveryStatus: ChatDeliveryStatus.pending,
                )
              : message)
          .toList(growable: false),
      terminalFailures: failures,
      clearActionError: true,
    );
  }

  /// Discards a terminal pending draft and its durable local envelope.
  Future<void> discardPending(String idempotencyKey) async {
    await _pendingStore?.discard(idempotencyKey);
    final failures = Map<String, String>.of(state.terminalFailures)
      ..remove(idempotencyKey);
    state = state.copyWith(
      messages: state.messages
          .where((message) => message.id != idempotencyKey)
          .toList(),
      terminalFailures: failures,
      clearActionError: true,
    );
  }

  Future<void> _restorePending(String circleId) async {
    final store = _pendingStore;
    if (store == null) return;
    final pending = (await store.loadAll())
        .where((envelope) => envelope.circleId == circleId)
        .map((envelope) => ChatMessage(
              id: envelope.idempotencyKey,
              senderId: '',
              circleId: circleId,
              content: envelope.content,
              type: ChatMessageType.text,
              sentAt: envelope.updatedAt ?? DateTime.now().toUtc(),
              deliveryStatus: ChatDeliveryStatus.pending,
            ));
    state =
        state.copyWith(messages: mergeChatMessages(state.messages, pending));
  }

  Future<void> _sendPending(PendingMessageEnvelope envelope) async {
    final credentials = await _credentials();
    final message = await _api.sendTextMessage(
      token: credentials.token,
      sessionId: credentials.sessionId,
      circleId: envelope.circleId,
      content: envelope.content,
      idempotencyKey: envelope.idempotencyKey,
    );
    state = state.copyWith(
        messages: mergeChatMessages(
            state.messages.where((m) => m.id != envelope.idempotencyKey),
            [message]));
    await _pendingStore?.discard(envelope.idempotencyKey);
  }

  Future<void> _retry(PendingMessageEnvelope envelope) async {
    if (!_activeRetries.add(envelope.idempotencyKey)) return;
    try {
      for (final delay in chatRetryDelays) {
        await _retryDelay(delay);
        if (_circleId != envelope.circleId) return;
        try {
          await _sendPending(envelope);
          return;
        } catch (error) {
          if (!_isRetryable(error)) {
            _markTerminal(envelope.idempotencyKey, error);
            return;
          }
        }
      }
      _markTerminal(envelope.idempotencyKey, 'Retry required');
    } finally {
      _activeRetries.remove(envelope.idempotencyKey);
    }
  }

  void _markTerminal(String idempotencyKey, Object error) {
    state = state.copyWith(
      actionErrorMessage: error.toString(),
      terminalFailures: Map<String, String>.of(state.terminalFailures)
        ..[idempotencyKey] = error.toString(),
    );
  }

  static bool _isRetryable(Object error) {
    if (error is ChatApiException) {
      final status = error.statusCode;
      return status == null || status == 429 || status >= 500;
    }
    if (error is! DioException) return false;
    final status = error.response?.statusCode;
    return status == null || status == 429 || status >= 500;
  }

  void handleRealtimeEvent(ChatRealtimeEvent event) {
    switch (event) {
      case ChatMessageEvent():
        if (state.status == GroupChatStatus.accessLost) return;
        state = state.copyWith(
          messages: mergeChatMessages(state.messages, [event.message]),
        );
      case ChatUnknownEvent():
        unawaited(_reconcile());
      case ChatReconnectedEvent():
        unawaited(_recoverAfterReconnect());
    }
  }

  Future<void> _recoverAfterReconnect() async {
    if (state.status == GroupChatStatus.accessLost) return;
    await _reconcile();
    await retryPending();
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
    } catch (error) {
      if (error is ChatApiException &&
          (error.statusCode == 401 || error.statusCode == 403) &&
          _circleId == circleId) {
        state =
            const GroupChatControllerState(status: GroupChatStatus.accessLost);
      }
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
        status: error is ChatApiException &&
                (error.statusCode == 401 || error.statusCode == 403)
            ? GroupChatStatus.accessLost
            : GroupChatStatus.error,
        messages: error is ChatApiException &&
                (error.statusCode == 401 || error.statusCode == 403)
            ? const []
            : state.messages,
        errorMessage: error.toString(),
        readOnly: _readOnly,
      );
    }
  }

  static String _newIdempotencyKey() => newChatIdempotencyKey();

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
    pendingStore: PendingMessageStore(const FlutterSecureStorage()),
  );
});
