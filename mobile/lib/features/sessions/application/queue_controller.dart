import 'dart:async';
import 'dart:math';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/sessions/data/queue_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/data/realtime_session_client.dart';

enum QueueControllerStatus { idle, loading, ready, error, ended }

enum QueueOptOutFeedback { pending, declined, approved, autoApproved }

class QueueControllerState {
  const QueueControllerState({
    this.status = QueueControllerStatus.idle,
    this.queue,
    this.errorMessage,
    this.actionErrorMessage,
    this.isManager = false,
    this.optOutFeedback,
  });

  final QueueControllerStatus status;
  final QueueState? queue;
  final String? errorMessage;
  final String? actionErrorMessage;
  final bool isManager;
  final QueueOptOutFeedback? optOutFeedback;

  QueueControllerState copyWith({
    QueueControllerStatus? status,
    QueueState? queue,
    String? errorMessage,
    bool clearError = false,
    String? actionErrorMessage,
    bool clearActionError = false,
    bool? isManager,
    QueueOptOutFeedback? optOutFeedback,
    bool clearOptOutFeedback = false,
  }) =>
      QueueControllerState(
        status: status ?? this.status,
        queue: queue ?? this.queue,
        errorMessage: clearError ? null : (errorMessage ?? this.errorMessage),
        actionErrorMessage: clearActionError
            ? null
            : (actionErrorMessage ?? this.actionErrorMessage),
        isManager: isManager ?? this.isManager,
        optOutFeedback: clearOptOutFeedback
            ? null
            : (optOutFeedback ?? this.optOutFeedback),
      );
}

typedef QueueCredentials = Future<({String token, String sessionId})>
    Function();

class QueueController extends StateNotifier<QueueControllerState> {
  QueueController(
    this._api,
    this._credentials, {
    required RealtimeSessionClient realtime,
    bool isManager = false,
  })  : _realtime = realtime,
        super(QueueControllerState(isManager: isManager));

  final QueueApiClient _api;
  final QueueCredentials _credentials;
  final RealtimeSessionClient _realtime;
  final Set<String> _eventIds = {};
  StreamSubscription<RealtimeSessionEvent>? _subscription;
  String? _liveSessionId;
  String? _pendingOptOutEntryId;
  int _refreshGeneration = 0;

  void setManager(bool isManager) {
    state = state.copyWith(isManager: isManager);
  }

  Future<void> connect(String liveSessionId,
      {bool listenRealtime = true}) async {
    await _subscription?.cancel();
    _subscription = null;
    _liveSessionId = liveSessionId;
    _pendingOptOutEntryId = null;
    _eventIds.clear();
    state = state.copyWith(
      status: QueueControllerStatus.loading,
      clearError: true,
      clearActionError: true,
      clearOptOutFeedback: true,
    );
    await _refresh();
    if (!listenRealtime || _liveSessionId != liveSessionId) return;

    try {
      final credentials = await _credentials();
      _subscription = _realtime
          .sessionEvents(
            liveSessionId,
            token: credentials.token,
            backendSessionId: credentials.sessionId,
          )
          .listen(handleRealtimeEvent);
    } catch (error) {
      state = state.copyWith(
        status: QueueControllerStatus.error,
        errorMessage: error.toString(),
      );
    }
  }

  Future<void> leave() async {
    await _subscription?.cancel();
    _subscription = null;
    _liveSessionId = null;
    _pendingOptOutEntryId = null;
    _eventIds.clear();
    state = QueueControllerState(
      isManager: state.isManager,
      optOutFeedback: null,
    );
  }

  Future<void> end() async {
    state = state.copyWith(status: QueueControllerStatus.ended);
    await _subscription?.cancel();
    _subscription = null;
  }

  void handleRealtimeEvent(RealtimeSessionEvent event) {
    if (event is SessionEndedEvent) {
      unawaited(end());
      return;
    }
    if (state.status == QueueControllerStatus.ended) return;
    if (event is! QueueRealtimeEvent || !_eventIds.add(event.eventId)) return;

    final current = state.queue;
    if (event case QueueStateEvent()) {
      if (current != null && event.queue.roundId == current.roundId) {
        if (event.queue.version <= current.version) return;
        if (event.queue.version > current.version + 1) {
          unawaited(_refresh());
          return;
        }
      }
      state = state.copyWith(
        status: QueueControllerStatus.ready,
        queue: event.queue,
        clearError: true,
      );
      return;
    }
    if (event is QueuePolicyChangedEvent || event is QueueVersionGapEvent) {
      unawaited(_refresh());
      return;
    }
    if (event.version == null ||
        (current != null && event.version! < current.version)) {
      return;
    }
    unawaited(_refresh());
  }

  Future<void> prepareRound({
    required String roundType,
    required int surahId,
    required int fromAyah,
    required int toAyah,
    required bool gradingRequired,
    List<String>? studentOrder,
  }) =>
      _runManager((credentials, _) => _api.prepareRound(
            token: credentials.token,
            sessionId: credentials.sessionId,
            liveSessionId: _liveSessionId!,
            roundType: roundType,
            surahId: surahId,
            fromAyah: fromAyah,
            toAyah: toAyah,
            gradingRequired: gradingRequired,
            studentOrder: studentOrder,
          ));

  Future<void> advance() => _runManager((credentials, queue) => _api.advance(
        token: credentials.token,
        sessionId: credentials.sessionId,
        liveSessionId: _liveSessionId!,
        expectedVersion: queue.version,
      ));

  Future<void> requestOptOut() async {
    final queue = state.queue;
    if (state.isManager || queue == null || _liveSessionId == null) return;
    try {
      final credentials = await _credentials();
      final result = await _api.optOut(
        token: credentials.token,
        sessionId: credentials.sessionId,
        liveSessionId: _liveSessionId!,
        idempotencyKey: _newIdempotencyKey(),
      );
      final feedback = _mapOptOutFeedback(result, queue.policy.optOut);
      _pendingOptOutEntryId =
          feedback == QueueOptOutFeedback.pending ? result.entry.id : null;
      state = state.copyWith(
        optOutFeedback: feedback,
        clearActionError: true,
      );
      await _refresh();
    } catch (error) {
      state = state.copyWith(actionErrorMessage: error.toString());
    }
  }

  Future<void> reorder(List<String> orderedIds) =>
      _runManager((credentials, queue) => _api.reorder(
            token: credentials.token,
            sessionId: credentials.sessionId,
            liveSessionId: _liveSessionId!,
            orderedIds: orderedIds,
            expectedVersion: queue.version,
          ));

  Future<void> moveEntry(String entryId, int newPosition) =>
      _runManager((credentials, queue) => _api.moveEntry(
            token: credentials.token,
            sessionId: credentials.sessionId,
            liveSessionId: _liveSessionId!,
            entryId: entryId,
            newPosition: newPosition,
            expectedVersion: queue.version,
          ));

  Future<void> startEntry(String entryId) =>
      _updateEntryStatus(entryId, 'start');

  Future<void> skipEntry(String entryId) => _updateEntryStatus(entryId, 'skip');

  Future<void> reset({
    required String roundType,
    required int surahId,
    required int fromAyah,
    required int toAyah,
    required bool gradingRequired,
    List<String>? studentOrder,
  }) =>
      _runManager((credentials, queue) => _api.reset(
            token: credentials.token,
            sessionId: credentials.sessionId,
            liveSessionId: _liveSessionId!,
            roundType: roundType,
            surahId: surahId,
            fromAyah: fromAyah,
            toAyah: toAyah,
            gradingRequired: gradingRequired,
            expectedVersion: queue.version,
            studentOrder: studentOrder,
          ));

  Future<void> updatePolicy({
    String? population,
    String? unfinishedFinalization,
    String? optOut,
    String? gradeVisibility,
    String? gradeCorrection,
  }) =>
      _runManager((credentials, queue) async {
        await _api.updatePolicy(
          token: credentials.token,
          sessionId: credentials.sessionId,
          liveSessionId: _liveSessionId!,
          expectedVersion: queue.policy.version,
          population: population,
          unfinishedFinalization: unfinishedFinalization,
          optOut: optOut,
          gradeVisibility: gradeVisibility,
          gradeCorrection: gradeCorrection,
        );
        return _queue(credentials);
      });

  Future<void> _updateEntryStatus(String entryId, String status) =>
      _runManager((credentials, queue) {
        final entry = queue.entries.where((entry) => entry.id == entryId);
        if (entry.isEmpty) throw StateError('Queue entry is unavailable');
        return _api.updateEntryStatus(
          token: credentials.token,
          sessionId: credentials.sessionId,
          liveSessionId: _liveSessionId!,
          entryId: entryId,
          status: status,
          expectedEntryVersion: entry.single.version,
        );
      });

  Future<void> _runManager(
    Future<QueueState> Function(
      ({String token, String sessionId}) credentials,
      QueueState queue,
    ) mutation,
  ) async {
    final queue = state.queue;
    if (!state.isManager || queue == null || _liveSessionId == null) return;
    try {
      final updated = await mutation(await _credentials(), queue);
      state = state.copyWith(
        status: QueueControllerStatus.ready,
        queue: updated,
        clearError: true,
        clearActionError: true,
      );
    } catch (error) {
      state = state.copyWith(actionErrorMessage: error.toString());
    }
  }

  Future<void> _refresh() async {
    final liveSessionId = _liveSessionId;
    if (liveSessionId == null) return;
    final generation = ++_refreshGeneration;
    try {
      final queue = await _queue(await _credentials());
      if (_liveSessionId != liveSessionId || generation != _refreshGeneration) {
        return;
      }
      var optOutFeedback = state.optOutFeedback;
      final pendingEntryID = _pendingOptOutEntryId;
      if (optOutFeedback == QueueOptOutFeedback.pending &&
          pendingEntryID != null &&
          queue.entries.any((entry) =>
              entry.id == pendingEntryID && entry.status == 'opted_out')) {
        optOutFeedback = QueueOptOutFeedback.approved;
        _pendingOptOutEntryId = null;
      }
      state = state.copyWith(
        status: QueueControllerStatus.ready,
        queue: queue,
        optOutFeedback: optOutFeedback,
        clearError: true,
      );
    } catch (error) {
      if (_liveSessionId != liveSessionId || generation != _refreshGeneration) {
        return;
      }
      state = state.copyWith(
        status: QueueControllerStatus.error,
        errorMessage: error.toString(),
      );
    }
  }

  Future<QueueState> _queue(
    ({String token, String sessionId}) credentials,
  ) =>
      _api.getQueue(
        token: credentials.token,
        sessionId: credentials.sessionId,
        liveSessionId: _liveSessionId!,
      );

  static QueueOptOutFeedback _mapOptOutFeedback(
    OptOutResult result,
    String optOutPolicy,
  ) =>
      switch (result.request.status) {
        'pending' => QueueOptOutFeedback.pending,
        'declined' => QueueOptOutFeedback.declined,
        'approved' when optOutPolicy == 'auto_approve' =>
          QueueOptOutFeedback.autoApproved,
        _ => QueueOptOutFeedback.approved,
      };

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

final queueControllerProvider =
    StateNotifierProvider.family<QueueController, QueueControllerState, String>(
        (ref, _) {
  final auth = ref.watch(authControllerProvider);
  return QueueController(
    ref.watch(queueApiClientProvider),
    () async {
      final user = ref.read(firebaseAuthProvider).currentUser;
      final sessionId = auth.sessionId;
      final token = await user?.getIdToken();
      if (token == null ||
          token.isEmpty ||
          sessionId == null ||
          sessionId.isEmpty) {
        throw StateError('User not authenticated');
      }
      return (token: token, sessionId: sessionId);
    },
    realtime: ref.watch(realtimeSessionClientProvider),
  );
});
