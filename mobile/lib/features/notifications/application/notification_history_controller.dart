import 'dart:async';

import 'package:dio/dio.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/notifications/data/notification_realtime_client.dart';
import 'package:halaqaty_mobile/features/notifications/data/notification_repository.dart';
import 'package:halaqaty_mobile/features/notifications/domain/notification_models.dart';

/// Passive bell/history state space. `unavailable` means "no usable session
/// data" (logged out, restore pending); it renders no badge rather than an
/// error. The exact server count is the only unread truth (realtime.md).
enum NotificationHistoryStatus {
  ready,
  loading,
  error,
  unavailable,
}

enum NotificationHistoryFailure { network, unknown }

class NotificationHistoryState {
  const NotificationHistoryState({
    this.items = const [],
    this.nextCursor,
    this.unreadCount,
    this.status = NotificationHistoryStatus.ready,
    this.failure,
  });

  final List<NotificationItem> items;

  /// Opaque next-page cursor; `null` exhausts the (30-day) history.
  final String? nextCursor;

  /// Exact, server-authoritative unread count. `null` is unknown (not
  /// loaded or signed out). Never derived client-side: REST responses and
  /// `notification.updated` values REPLACE it, never increment it.
  final int? unreadCount;

  final NotificationHistoryStatus status;
  final NotificationHistoryFailure? failure;

  bool get hasMore => nextCursor != null;

  NotificationHistoryState copyWith({
    List<NotificationItem>? items,
    String? nextCursor,
    bool clearNextCursor = false,
    int? unreadCount,
    bool clearUnreadCount = false,
    NotificationHistoryStatus? status,
    NotificationHistoryFailure? failure,
    bool clearFailure = false,
  }) {
    return NotificationHistoryState(
      items: items ?? this.items,
      nextCursor: clearNextCursor ? null : (nextCursor ?? this.nextCursor),
      unreadCount: clearUnreadCount ? null : (unreadCount ?? this.unreadCount),
      status: status ?? this.status,
      failure: clearFailure ? null : (failure ?? this.failure),
    );
  }
}

/// Owns history paging, the exact-server unread count, and reconciliation.
///
/// Reconciliation (specs/008-notification-system/contracts/realtime.md): the
/// user-targeted `notification.updated` WebSocket event REPLACES the
/// displayed count; app resume, transport reconnect, and account switch
/// (provider rebuild on the auth session id) reconcile through
/// `GET /notifications`. Events carry no history content, so a count event
/// alone never triggers extra REST calls; the list reconciles on entry,
/// resume, reconnect, and pull-to-refresh.
class NotificationHistoryController
    extends StateNotifier<NotificationHistoryState> {
  NotificationHistoryController({
    required this.repository,
    required NotificationCredentialsSource credentials,
    required this.connectRealtime,
    this.logout,
    Stream<AppLifecycleState>? lifecycle,
  })  : _credentials = credentials,
        super(const NotificationHistoryState()) {
    _lifecycleSubscription = lifecycle?.listen((stateValue) {
      if (stateValue == AppLifecycleState.resumed) {
        // Resume always reconciles against the server record (realtime.md).
        unawaited(load());
      }
    });
  }

  final NotificationRepository repository;

  /// Opens the user-targeted `notification.updated` transport with the
  /// session's credentials; the opened stream is consumed here.
  final Stream<NotificationRealtimeEvent> Function(NotificationCredentials)
      connectRealtime;

  /// 401 stale-auth rule: clear the local session through the shared
  /// auth controller.
  final Future<void> Function()? logout;

  StreamSubscription<AppLifecycleState>? _lifecycleSubscription;
  StreamSubscription<NotificationRealtimeEvent>? _realtimeSubscription;

  final NotificationCredentialsSource _credentials;
  bool _busy = false;

  /// Reconciles the whole state from the first page. Fires on screen entry,
  /// pull-to-refresh, app resume, and transport reconnect (and, through the
  /// provider rebuild, on login/account switch). Keeps loaded items visible
  /// during a failing refresh so a transient blip never blanks the page.
  Future<void> load() async {
    if (_busy) return;
    _busy = true;
    try {
      if (state.items.isEmpty) {
        state = state.copyWith(
          status: NotificationHistoryStatus.loading,
          clearFailure: true,
        );
      }
      final credentials = await _loadCredentials();
      if (credentials == null) {
        _onUnavailable();
        return;
      }
      final page = await repository.list(
        firebaseIdToken: credentials.firebaseIdToken,
        sessionId: credentials.sessionId,
      );
      state = NotificationHistoryState(
        items: page.items,
        nextCursor: page.nextCursor,
        unreadCount: page.unreadCount,
        status: NotificationHistoryStatus.ready,
      );
      _ensureRealtime(credentials);
    } on StateError {
      _onUnavailable();
    } on DioException catch (error) {
      await _applyFailure(error);
    } catch (error) {
      _applyError(NotificationHistoryFailure.unknown);
    } finally {
      _busy = false;
    }
  }

  /// Follows the opaque cursor and appends the next page; the
  /// authoritative count is replaced from the newest response. Exhausted
  /// history is a no-op.
  Future<void> loadMore() async {
    final cursor = state.nextCursor;
    if (cursor == null || _busy) return;
    _busy = true;
    try {
      final credentials = await _loadCredentials();
      if (credentials == null) {
        _onUnavailable();
        return;
      }
      final page = await repository.list(
        firebaseIdToken: credentials.firebaseIdToken,
        sessionId: credentials.sessionId,
        cursor: cursor,
      );
      state = state.copyWith(
        items: [...state.items, ...page.items],
        nextCursor: page.nextCursor,
        clearNextCursor: page.nextCursor == null,
        unreadCount: page.unreadCount,
        status: NotificationHistoryStatus.ready,
        clearFailure: true,
      );
    } on StateError {
      _onUnavailable();
    } on DioException catch (error) {
      await _applyFailure(error);
    } catch (error) {
      _applyError(NotificationHistoryFailure.unknown);
    } finally {
      _busy = false;
    }
  }

  /// Applies one `notification.updated` hint or reconnect signal.
  void onRealtimeEvent(NotificationRealtimeEvent event) {
    if (event.isReconnectSignal) {
      unawaited(load());
      return;
    }
    // The event is a state hint only: replace the displayed count with the
    // authoritative value; derive nothing (count is never incremented).
    final unreadCount = event.unreadCount;
    if (unreadCount == null) return;
    state = state.copyWith(
      unreadCount: unreadCount,
      clearFailure: true,
    );
  }

  void _ensureRealtime(NotificationCredentials credentials) {
    if (_realtimeSubscription != null) return;
    _realtimeSubscription =
        connectRealtime(credentials).listen(onRealtimeEvent);
  }

  Future<NotificationCredentials?> _loadCredentials() async {
    try {
      return await _credentials.load();
    } on StateError {
      return null;
    }
  }

  void _onUnavailable() {
    state = const NotificationHistoryState(
      status: NotificationHistoryStatus.unavailable,
    );
    _closeRealtime();
  }

  void _closeRealtime() {
    unawaited(_realtimeSubscription?.cancel());
    _realtimeSubscription = null;
  }

  Future<void> _applyFailure(DioException error) async {
    if (error.response?.statusCode == 401) {
      _onUnavailable();
      await logout?.call();
      return;
    }
    _applyError(_failureOf(error));
  }

  void _applyError(NotificationHistoryFailure failure) {
    final hasItems = state.items.isNotEmpty;
    state = state.copyWith(
      status: hasItems
          ? NotificationHistoryStatus.ready
          : NotificationHistoryStatus.error,
      failure: failure,
    );
  }

  NotificationHistoryFailure _failureOf(DioException error) {
    return error.response?.statusCode == null
        ? NotificationHistoryFailure.network
        : NotificationHistoryFailure.unknown;
  }

  @override
  void dispose() {
    unawaited(_lifecycleSubscription?.cancel());
    _closeRealtime();
    super.dispose();
  }
}

/// Resolves the authenticated session like the calendar providers (F-001
/// pattern): Firebase bearer + backend session id; missing either leaves the
/// bell without data rather than failing hard.
final notificationCredentialsProvider = Provider<NotificationCredentialsSource>(
    (ref) => NotificationAuthProviderCredentials(ref));

class NotificationAuthProviderCredentials
    implements NotificationCredentialsSource {
  NotificationAuthProviderCredentials(this._ref);

  final Ref _ref;

  @override
  Future<NotificationCredentials> load() async {
    final firebaseUser = _ref.read(firebaseAuthProvider).currentUser;
    final sessionId = _ref.read(authControllerProvider).sessionId;
    if (firebaseUser == null || sessionId == null) {
      throw StateError('Notification session is unavailable.');
    }
    final token = await firebaseUser.getIdToken();
    if (token == null || token.isEmpty) {
      throw StateError('Notification session is unavailable.');
    }
    return NotificationCredentials(
      firebaseIdToken: token,
      sessionId: sessionId,
    );
  }
}

/// Emits app-lifecycle transitions so the controller can reconcile on
/// resume (realtime.md); injected as a stream to stay test-friendly.
final appLifecycleStreamProvider = Provider<Stream<AppLifecycleState>>((ref) {
  final binding = WidgetsBinding.instance;
  final controller = StreamController<AppLifecycleState>.broadcast();
  final observer = _LifecycleObserver(controller);
  binding.addObserver(observer);
  ref.onDispose(() {
    binding.removeObserver(observer);
    unawaited(controller.close());
  });
  return controller.stream;
});

class _LifecycleObserver with WidgetsBindingObserver {
  _LifecycleObserver(this._pending);

  final StreamController<AppLifecycleState> _pending;

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    _pending.add(state);
  }
}

/// App-wide bell/history authority. Non-autoDispose: the bell is always
/// visible inside the shell while authenticated. Rebuilt by the auth
/// session id only — login, restore, account switch (new id) and logout
/// (null id) recreate the controller, resetting any stale data. After
/// creation with a live session it immediately reconciles the exact count
/// and opens the `notification.updated` transport.
final notificationHistoryControllerProvider = StateNotifierProvider<
    NotificationHistoryController, NotificationHistoryState>((ref) {
  final sessionId =
      ref.watch(authControllerProvider.select((s) => s.sessionId));
  final controller = NotificationHistoryController(
    repository: ref.watch(notificationRepositoryProvider),
    credentials: ref.watch(notificationCredentialsProvider),
    connectRealtime: (credentials) =>
        ref.watch(notificationRealtimeClientProvider).events(credentials),
    logout: () => ref.read(authControllerProvider.notifier).logout(),
    lifecycle: ref.watch(appLifecycleStreamProvider),
  );
  if (sessionId != null) {
    unawaited(controller.load());
  }
  return controller;
});
