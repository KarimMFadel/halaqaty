import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/sessions/application/media_session.dart';
import 'package:halaqaty_mobile/features/sessions/application/queue_controller.dart';
import 'package:halaqaty_mobile/features/sessions/data/realtime_session_client.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_api_client.dart';

enum SessionRoomStatus { idle, loading, connected, error, ended }

/// Recovery affordance exposed to the room UI after a failed connection.
enum SessionRoomRecovery { none, retryable, terminal }

class SessionRoomState {
  const SessionRoomState(
      {this.status = SessionRoomStatus.idle,
      this.connection,
      this.errorMessage,
      this.participants = const [],
      this.isLocked = false,
      this.isModerator = false,
      this.actionErrorMessage,
      this.recovery = SessionRoomRecovery.none,
      this.queueState});
  final SessionRoomStatus status;
  final SessionConnection? connection;
  final String? errorMessage;
  final List<SessionParticipant> participants;
  final bool isLocked;
  final bool isModerator;
  final String? actionErrorMessage;
  final SessionRoomRecovery recovery;
  final QueueControllerState? queueState;

  SessionRoomState copyWith(
          {SessionRoomStatus? status,
          SessionConnection? connection,
          String? errorMessage,
          bool clearError = false,
          List<SessionParticipant>? participants,
          bool? isLocked,
          bool? isModerator,
          String? actionErrorMessage,
          bool clearActionError = false,
          SessionRoomRecovery? recovery,
          QueueControllerState? queueState}) =>
      SessionRoomState(
        status: status ?? this.status,
        connection: connection ?? this.connection,
        errorMessage: clearError ? null : (errorMessage ?? this.errorMessage),
        participants: participants ?? this.participants,
        isLocked: isLocked ?? this.isLocked,
        isModerator: isModerator ?? this.isModerator,
        actionErrorMessage: clearActionError
            ? null
            : (actionErrorMessage ?? this.actionErrorMessage),
        recovery: recovery ?? this.recovery,
        queueState: queueState ?? this.queueState,
      );
}

class SessionRoomController extends StateNotifier<SessionRoomState> {
  SessionRoomController(
    this._api,
    this._credentials,
    this._mediaSession, {
    required RealtimeSessionClient realtime,
    bool isModerator = false,
    QueueController? queue,
  })  : _realtime = realtime,
        _queue = queue,
        super(SessionRoomState(
          isModerator: isModerator,
          queueState: queue?.state,
        )) {
    _queueListener = queue?.addListener(_applyQueueState);
  }
  final SessionApiClient _api;
  final Future<({String token, String sessionId})> Function() _credentials;
  final MediaSession _mediaSession;
  final RealtimeSessionClient _realtime;
  final QueueController? _queue;
  RemoveListener? _queueListener;
  StreamSubscription<RealtimeSessionEvent>? _subscription;
  String? _liveSessionId;
  String? _lastEventKey;

  Future<void> start(String sessionId) => _connect(sessionId, true);
  Future<void> join(String sessionId) => _connect(sessionId, false);

  /// Re-authorizes the current participant and obtains a fresh media ticket.
  /// The server still selects join versus eligible reconnect from durable state.
  Future<void> retry() async {
    final liveSessionId = _liveSessionId;
    if (liveSessionId == null ||
        state.recovery == SessionRoomRecovery.terminal) {
      return;
    }
    await _connect(liveSessionId, false);
  }

  /// Leaves the room without attempting another REST retry.
  Future<void> leave() async {
    await _queue?.leave();
    await _mediaSession.disconnect();
    await _subscription?.cancel();
    _subscription = null;
    _liveSessionId = null;
    state = state.copyWith(
        status: SessionRoomStatus.idle,
        clearError: true,
        recovery: SessionRoomRecovery.none);
  }

  Future<void> _connect(String liveSessionId, bool start) async {
    // Remember the attempted room so the Retry affordance works even when
    // the very first connect fails.
    _liveSessionId = liveSessionId;
    state = state.copyWith(
        status: SessionRoomStatus.loading,
        clearError: true,
        recovery: SessionRoomRecovery.none);
    try {
      final credentials = await _credentials();
      final connection = start
          ? await _api.start(
              token: credentials.token,
              sessionId: credentials.sessionId,
              liveSessionId: liveSessionId)
          : await _api.join(
              token: credentials.token,
              sessionId: credentials.sessionId,
              liveSessionId: liveSessionId);
      await _mediaSession.connect(connection.mediaConnection);
      // REST snapshot stays the source of truth; realtime events refine it.
      final participants = await _api.participants(
          token: credentials.token,
          sessionId: credentials.sessionId,
          liveSessionId: liveSessionId);
      _lastEventKey = null;
      final isModerator = state.isModerator || connection.isModerator;
      state = state.copyWith(
          status: SessionRoomStatus.connected,
          connection: connection,
          participants: participants,
          isLocked: connection.session.isLocked,
          isModerator: isModerator,
          recovery: SessionRoomRecovery.none,
          clearActionError: true);
      _queue?.setManager(isModerator);
      await _queue?.connect(liveSessionId, listenRealtime: false);
      unawaited(_subscription?.cancel());
      _subscription = _realtime
          .sessionEvents(liveSessionId,
              token: credentials.token, backendSessionId: credentials.sessionId)
          .listen(_applyEvent, onDone: _handleRealtimeDisconnect);
    } catch (error) {
      state = state.copyWith(
          status: SessionRoomStatus.error,
          errorMessage: error.toString(),
          recovery: _isTerminal(error)
              ? SessionRoomRecovery.terminal
              : SessionRoomRecovery.retryable);
    }
  }

  static bool _isTerminal(Object error) {
    final message = error.toString().toLowerCase();
    return message.contains('ended') ||
        message.contains('removed') ||
        message.contains('revoked') ||
        message.contains('unauthorized') ||
        message.contains('forbidden') ||
        message.contains('401') ||
        message.contains('403') ||
        message.contains('404');
  }

  void _handleRealtimeDisconnect() {
    if (!mounted || state.status != SessionRoomStatus.connected) return;
    _subscription = null;
    state = state.copyWith(
      status: SessionRoomStatus.error,
      errorMessage: 'Realtime connection closed',
      recovery: SessionRoomRecovery.retryable,
    );
  }

  /// Hand commands are WS-only per `ws_events.md` (`cmd.raise_hand`).
  Future<void> raiseHand() => _sendHandCommand(_realtime.raiseHand);
  Future<void> lowerHand() => _sendHandCommand(_realtime.lowerHand);

  Future<void> advanceQueue() => _queue?.advance() ?? Future.value();

  Future<void> prepareQueueRound({
    required String roundType,
    required int surahId,
    required int fromAyah,
    required int toAyah,
    required bool gradingRequired,
  }) =>
      _queue?.prepareRound(
        roundType: roundType,
        surahId: surahId,
        fromAyah: fromAyah,
        toAyah: toAyah,
        gradingRequired: gradingRequired,
      ) ??
      Future.value();

  Future<void> moveQueueEntry(String entryId, int newPosition) =>
      _queue?.moveEntry(entryId, newPosition) ?? Future.value();

  Future<void> resetQueueRound({
    required String roundType,
    required int surahId,
    required int fromAyah,
    required int toAyah,
    required bool gradingRequired,
  }) =>
      _queue?.reset(
        roundType: roundType,
        surahId: surahId,
        fromAyah: fromAyah,
        toAyah: toAyah,
        gradingRequired: gradingRequired,
      ) ??
      Future.value();

  Future<void> startSelectedQueueEntry() {
    final queue = state.queueState?.queue;
    final selectedEntryId = queue?.selectedEntryId;
    if (queue == null || selectedEntryId == null) return Future.value();
    return _queue?.startEntry(selectedEntryId) ?? Future.value();
  }

  Future<void> skipSelectedQueueEntry() {
    final queue = state.queueState?.queue;
    final selectedEntryId = queue?.selectedEntryId;
    if (queue == null || selectedEntryId == null) return Future.value();
    return _queue?.skipEntry(selectedEntryId) ?? Future.value();
  }

  void _applyQueueState(QueueControllerState queueState) {
    state = state.copyWith(queueState: queueState);
  }

  Future<void> _sendHandCommand(
      Future<void> Function(String liveSessionId) send) async {
    final liveSessionId = _liveSessionId;
    if (liveSessionId == null) return;
    try {
      await send(liveSessionId);
    } catch (error) {
      state = state.copyWith(actionErrorMessage: error.toString());
    }
  }

  /// Moderation actions are teacher/supervisor only. The UI hides the
  /// controls for members and the backend still enforces 403.
  Future<void> setLock(bool locked) => _runModeration(() async {
        final credentials = await _credentials();
        final session = await _api.setLock(
            token: credentials.token,
            sessionId: credentials.sessionId,
            liveSessionId: _liveSessionId!,
            locked: locked);
        state = state.copyWith(isLocked: session.isLocked);
      });

  Future<void> muteAll() => _runModeration(() async {
        final credentials = await _credentials();
        await _api.muteAll(
            token: credentials.token,
            sessionId: credentials.sessionId,
            liveSessionId: _liveSessionId!);
      });

  Future<void> muteParticipant(String userId) => _runModeration(() async {
        final credentials = await _credentials();
        await _api.muteParticipant(
            token: credentials.token,
            sessionId: credentials.sessionId,
            liveSessionId: _liveSessionId!,
            userId: userId);
      });

  Future<void> unmuteParticipant(String userId) => _runModeration(() async {
        final credentials = await _credentials();
        await _api.unmuteParticipant(
            token: credentials.token,
            sessionId: credentials.sessionId,
            liveSessionId: _liveSessionId!,
            userId: userId);
      });

  Future<void> removeParticipant(String userId) => _runModeration(() async {
        final credentials = await _credentials();
        await _api.removeParticipant(
            token: credentials.token,
            sessionId: credentials.sessionId,
            liveSessionId: _liveSessionId!,
            userId: userId);
      });

  Future<void> endSession() => _runModeration(() async {
        final credentials = await _credentials();
        await _api.end(
            token: credentials.token,
            sessionId: credentials.sessionId,
            liveSessionId: _liveSessionId!);
        await _queue?.end();
        state = state.copyWith(status: SessionRoomStatus.ended);
      });

  Future<void> _runModeration(Future<void> Function() action) async {
    final liveSessionId = _liveSessionId;
    if (!state.isModerator || liveSessionId == null) return;
    try {
      await action();
      state = state.copyWith(clearActionError: true);
    } catch (error) {
      // The room stays connected; only the failed action is reported.
      state = state.copyWith(actionErrorMessage: error.toString());
    }
  }

  void _applyEvent(RealtimeSessionEvent event) {
    _queue?.handleRealtimeEvent(event);
    switch (event) {
      // Snapshots are authoritative and never deduplicated.
      case SessionSnapshotEvent():
        _lastEventKey = null;
        state = state.copyWith(
            participants: event.participants, isLocked: event.isLocked);
      case ParticipantJoinedEvent():
        if (_skipDuplicate('joined:${event.userId}')) return;
        final participants = [...state.participants];
        final index = participants.indexWhere((p) => p.userId == event.userId);
        final joined = SessionParticipant(
            userId: event.userId,
            displayName: event.displayName,
            role: event.role,
            isCurrentlyPresent: true);
        if (index == -1) {
          participants.add(joined);
        } else {
          participants[index] = joined;
        }
        state = state.copyWith(participants: participants);
      case ParticipantLeftEvent():
        if (_skipDuplicate('left:${event.userId}')) return;
        state = state.copyWith(
            participants: _markAbsent(state.participants, event.userId));
      case ParticipantRemovedEvent():
        if (_skipDuplicate('removed:${event.userId}')) return;
        state = state.copyWith(
            participants: _markAbsent(state.participants, event.userId));
      case HandRaisedEvent():
        if (_skipDuplicate('raised:${event.participantId}')) return;
        state = state.copyWith(
            participants: _withHand(state.participants, event.participantId,
                event.at ?? DateTime.fromMillisecondsSinceEpoch(0)));
      case HandLoweredEvent():
        if (_skipDuplicate('lowered:${event.participantId}')) return;
        state = state.copyWith(
            participants: _lowerHand(state.participants, event.participantId));
      case LockChangedEvent():
        if (_skipDuplicate('lock:${event.locked}')) return;
        state = state.copyWith(isLocked: event.locked);
      case SessionEndedEvent():
        if (_skipDuplicate('ended:${event.endReason ?? ''}')) return;
        state = state.copyWith(
            status: SessionRoomStatus.ended,
            recovery: SessionRoomRecovery.terminal);
    }
  }

  /// At-least-once delivery: skip an identical consecutive event
  /// (type + affected participant) per `ws_events.md` dedup guidance.
  bool _skipDuplicate(String key) {
    if (key == _lastEventKey) return true;
    _lastEventKey = key;
    return false;
  }

  static List<SessionParticipant> _markAbsent(
      List<SessionParticipant> participants, String userId) {
    return participants
        .map((p) =>
            p.userId == userId ? p.copyWith(isCurrentlyPresent: false) : p)
        .toList(growable: false);
  }

  /// Hand events for participants outside the current snapshot are ignored;
  /// the next authoritative snapshot reconciles them.
  static List<SessionParticipant> _withHand(
      List<SessionParticipant> participants,
      String participantId,
      DateTime raisedAt) {
    return participants
        .map((p) =>
            p.userId == participantId ? p.copyWith(handRaisedAt: raisedAt) : p)
        .toList(growable: false);
  }

  static List<SessionParticipant> _lowerHand(
      List<SessionParticipant> participants, String participantId) {
    return participants
        .map((p) =>
            p.userId == participantId ? p.copyWith(handLowered: true) : p)
        .toList(growable: false);
  }

  @override
  void dispose() {
    _queueListener?.call();
    _subscription?.cancel();
    unawaited(_realtime.dispose());
    _mediaSession.disconnect();
    super.dispose();
  }
}

final sessionRoomControllerProvider = StateNotifierProvider.family<
    SessionRoomController, SessionRoomState, String>((ref, liveSessionId) {
  final auth = ref.watch(authControllerProvider);
  return SessionRoomController(ref.watch(sessionApiClientProvider), () async {
    final user = ref.read(firebaseAuthProvider).currentUser;
    final sessionId = auth.sessionId;
    final token = await user?.getIdToken();
    if (token == null ||
        token.isEmpty ||
        sessionId == null ||
        sessionId.isEmpty) throw StateError('User not authenticated');
    final tokenValue = token;
    final sessionValue = sessionId;
    return (token: tokenValue, sessionId: sessionValue);
  }, ref.watch(mediaSessionProvider),
      realtime: ref.watch(realtimeSessionClientProvider),
      queue: ref.watch(queueControllerProvider(liveSessionId).notifier));
});
