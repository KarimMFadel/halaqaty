import 'dart:async';

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/notifications/application/notification_history_controller.dart';
import 'package:halaqaty_mobile/features/notifications/data/notification_realtime_client.dart';
import 'package:halaqaty_mobile/features/notifications/data/notification_repository.dart';
import 'package:halaqaty_mobile/features/notifications/domain/notification_models.dart';

/// Deterministic notification REST boundary: pages are served in order.
class _FakeNotificationRepository implements NotificationRepository {
  _FakeNotificationRepository({this.pages = const [], this.error});

  final List<NotificationPage> pages;

  /// When set, [list] always throws it.
  Object? error;
  int listCalls = 0;
  final List<String?> cursors = [];
  int _next = 0;

  @override
  Future<NotificationPage> list({
    required String firebaseIdToken,
    required String sessionId,
    String? cursor,
    int? limit,
  }) async {
    listCalls++;
    cursors.add(cursor);
    final failure = error;
    if (failure != null) throw failure;
    final page = pages[_next.clamp(0, pages.length - 1)];
    if (_next < pages.length - 1) _next++;
    return page;
  }

  @override
  Future<int> markRead({
    required String firebaseIdToken,
    required String sessionId,
    required String notificationId,
  }) async {
    throw UnimplementedError('read actions are T028 scope');
  }

  @override
  Future<int> markAllRead({
    required String firebaseIdToken,
    required String sessionId,
  }) async {
    throw UnimplementedError('read actions are T028 scope');
  }

  @override
  Future<NotificationPreferences> getPreferences({
    required String firebaseIdToken,
    required String sessionId,
  }) async {
    throw UnimplementedError('preferences UI is T033 scope');
  }

  @override
  Future<NotificationPreferences> updatePreferences({
    required String firebaseIdToken,
    required String sessionId,
    required UpdateNotificationPreferencesRequest request,
  }) async {
    throw UnimplementedError('preferences UI is T033 scope');
  }
}

NotificationPage _page({
  int unread = 0,
  String? nextCursor,
  List<NotificationItem> items = const [],
}) =>
    NotificationPage(
      items: items,
      nextCursor: nextCursor,
      unreadCount: unread,
    );

NotificationItem _item(String id, {bool read = false}) => NotificationItem(
      id: id,
      type: 'session_reminder',
      title: 'Session reminder',
      body: 'A session starts soon.',
      source: const NotificationSourceRef(
        type: 'session',
        id: '22222222-2222-2222-2222-222222222222',
      ),
      createdAt: DateTime.utc(2026, 10, 25),
      isRead: read,
    );

/// Fed realtime transport: records the credentials it was opened with.
class _RealtimeHarness {
  final StreamController<NotificationRealtimeEvent> input =
      StreamController<NotificationRealtimeEvent>.broadcast();
  final List<NotificationCredentials> openedWith = [];

  Stream<NotificationRealtimeEvent> connect(NotificationCredentials c) {
    openedWith.add(c);
    return input.stream;
  }

  void emit(NotificationRealtimeEvent event) => input.add(event);
}

class _FakeRealtimeClient implements NotificationRealtimeClient {
  _FakeRealtimeClient(this._connect);

  final Stream<NotificationRealtimeEvent> Function(NotificationCredentials)
      _connect;

  @override
  Stream<NotificationRealtimeEvent> events(
          NotificationCredentials credentials) =>
      _connect(credentials);

  @override
  Future<void> dispose() async {}
}

class _CredentialsSource implements NotificationCredentialsSource {
  _CredentialsSource([this.fixedCredentials]);

  NotificationCredentials? fixedCredentials;
  Object? error;

  @override
  Future<NotificationCredentials> load() async {
    final failure = error;
    if (failure != null) throw failure;
    final value = fixedCredentials;
    if (value == null) {
      throw StateError('Notification session is unavailable.');
    }
    return value;
  }
}

const _creds = NotificationCredentials(
  firebaseIdToken: 'token-1',
  sessionId: 'session-1',
);

NotificationHistoryController _directController(
  _FakeNotificationRepository repo,
  _RealtimeHarness harness, {
  NotificationCredentialsSource? credentials,
  Future<void> Function()? logout,
  Stream<AppLifecycleState>? lifecycle,
}) =>
    NotificationHistoryController(
      repository: repo,
      credentials: credentials ?? _CredentialsSource(_creds),
      connectRealtime: harness.connect,
      logout: logout,
      lifecycle: lifecycle ?? const Stream.empty(),
    );

Future<void> _settle() => Future<void>.delayed(Duration.zero);

/// Mutable auth authority for provider-wiring tests: the hub provider
/// watches the session id and must rebuild on transitions.
class _MutableAuthController extends StateNotifier<AuthState>
    implements AuthController {
  _MutableAuthController(super.initial);

  @override
  void updateTimezone(String timezone) {}

  @override
  Future<void> logout() async {
    state = const AuthState(status: AuthStatus.unauthenticated);
  }

  void becomeAuthenticated(String sessionId) {
    state = AuthState(
      status: AuthStatus.authenticated,
      sessionId: sessionId,
    );
  }

  void becomeUnauthenticated() {
    state = const AuthState(status: AuthStatus.unauthenticated);
  }

  @override
  dynamic noSuchMethod(Invocation invocation) => null;
}

void main() {
  // The real appLifecycle stream provider consults WidgetsBinding.instance.
  WidgetsFlutterBinding.ensureInitialized();
  test('load() displays the exact server unread count and page', () async {
    final repo = _FakeNotificationRepository(pages: [
      _page(unread: 5, nextCursor: 'opaque', items: [
        _item('a'),
        _item('b', read: true),
      ]),
    ]);
    final harness = _RealtimeHarness();
    final controller = _directController(repo, harness);
    await controller.load();
    await _settle();

    expect(controller.state.unreadCount, 5,
        reason: 'the count comes only from the server response');
    expect(controller.state.items.map((n) => n.id), ['a', 'b']);
    expect(controller.state.status, NotificationHistoryStatus.ready);
    expect(controller.state.hasMore, isTrue);
    expect(harness.openedWith.single.sessionId, 'session-1',
        reason: 'the user-targeted transport opens with session credentials');
  });

  test(
    'a notification.updated WS event REPLACES the displayed count, never '
    'increments',
    () async {
      final repo = _FakeNotificationRepository(pages: [_page(unread: 5)]);
      final harness = _RealtimeHarness();
      final controller = _directController(repo, harness);
      await controller.load();

      harness.emit(const NotificationRealtimeEvent(unreadCount: 9));
      await _settle();
      expect(controller.state.unreadCount, 9);
      expect(controller.state.items, isEmpty,
          reason: 'the event carries no history content');
      expect(repo.listCalls, 1,
          reason: 'a count event alone never triggers extra REST calls');

      // Missed-event recovery: the authoritative lower value also wins.
      harness.emit(const NotificationRealtimeEvent(unreadCount: 2));
      await _settle();
      expect(controller.state.unreadCount, 2);
    },
  );

  test('the realtime reconnect signal reconciles via REST', () async {
    final repo = _FakeNotificationRepository(pages: [
      _page(unread: 2),
      _page(unread: 0),
    ]);
    final harness = _RealtimeHarness();
    final controller = _directController(repo, harness);
    await controller.load();
    expect(controller.state.unreadCount, 2);

    harness.emit(const NotificationRealtimeEvent.reconnected());
    await _settle();
    expect(controller.state.unreadCount, 0);
    expect(repo.listCalls, 2);
  });

  test('pages reconcile again on a second full load (reconnect/resume joint)',
      () async {
    final repo = _FakeNotificationRepository(pages: [
      _page(unread: 1),
      _page(unread: 0),
    ]);
    final controller = _directController(repo, _RealtimeHarness());
    await controller.load();
    expect(controller.state.unreadCount, 1);

    await controller.load();
    expect(controller.state.unreadCount, 0);
    expect(repo.listCalls, 2);
  });

  test(
    'loadMore() follows the cursor, appends, and replaces the count from '
    'the response',
    () async {
      final repo = _FakeNotificationRepository(pages: [
        _page(unread: 5, nextCursor: 'opaque', items: [_item('a')]),
        _page(unread: 4, items: [_item('b')]),
      ]);
      final controller = _directController(repo, _RealtimeHarness());
      await controller.load();
      await controller.loadMore();

      expect(controller.state.items.map((n) => n.id), ['a', 'b']);
      expect(controller.state.unreadCount, 4);
      expect(repo.cursors, [null, 'opaque']);
      expect(controller.state.hasMore, isFalse);
    },
  );

  test('loadMore() without a cursor is exhausted and calls nothing', () async {
    final repo = _FakeNotificationRepository(pages: [_page(nextCursor: null)]);
    final controller = _directController(repo, _RealtimeHarness());
    await controller.load();
    final before = repo.listCalls;
    await controller.loadMore();

    expect(repo.listCalls, before);
    expect(controller.state.hasMore, isFalse);
  });

  test('app resume reconciles the count via REST', () async {
    final repo = _FakeNotificationRepository(pages: [
      _page(unread: 1),
      _page(unread: 0),
    ]);
    final lifecycle = StreamController<AppLifecycleState>();
    final harness = _RealtimeHarness();
    final controller =
        _directController(repo, harness, lifecycle: lifecycle.stream);
    await controller.load();
    expect(controller.state.unreadCount, 1);

    lifecycle.add(AppLifecycleState.resumed);
    await _settle();
    expect(controller.state.unreadCount, 0);
    expect(repo.listCalls, 2);
  });

  test(
    'an unavailable session shows no data and never calls the network',
    () async {
      final repo = _FakeNotificationRepository(pages: [_page(unread: 5)]);
      final controller = _directController(
        repo,
        _RealtimeHarness(),
        credentials: _CredentialsSource(),
      );
      await controller.load();

      expect(controller.state.items, isEmpty);
      expect(controller.state.unreadCount, isNull);
      expect(controller.state.status, NotificationHistoryStatus.unavailable);
      expect(repo.listCalls, isZero);
    },
  );

  test('REST 401 clears the local session through the shared controller',
      () async {
    final repo = _FakeNotificationRepository(
      error: DioException(
        requestOptions: RequestOptions(path: '/notifications'),
        response: Response(
          requestOptions: RequestOptions(path: '/notifications'),
          statusCode: 401,
        ),
      ),
    );
    final logouts = <int>[];
    final controller = _directController(
      repo,
      _RealtimeHarness(),
      logout: () async => logouts.add(1),
    );
    await controller.load();

    expect(logouts, [1]);
    expect(controller.state.status, NotificationHistoryStatus.unavailable);
  });

  test('provider wiring: account switch reconciles the new account', () async {
    final repo = _FakeNotificationRepository(pages: [
      _page(unread: 5, items: [_item('a')]),
      _page(unread: 1, items: []),
    ]);
    final harness = _RealtimeHarness();
    final auth = _MutableAuthController(
      const AuthState(status: AuthStatus.authenticated, sessionId: 'session-1'),
    );

    final container = ProviderContainer(overrides: [
      authControllerProvider.overrideWith((_) => auth),
      notificationRepositoryProvider.overrideWithValue(repo),
      notificationCredentialsProvider
          .overrideWithValue(_CredentialsSource(_creds)),
      notificationRealtimeClientProvider
          .overrideWithValue(_FakeRealtimeClient(harness.connect)),
    ]);
    addTearDown(container.dispose);
    container.listen(
        notificationHistoryControllerProvider, (previous, next) {});
    await _settle();

    expect(
        container.read(notificationHistoryControllerProvider).unreadCount, 5);

    // Account switch: the provider rebuilds on the auth session-id change,
    // resetting stale data and reconciling the new account immediately.
    auth.becomeAuthenticated('session-9');
    await _settle();
    await _settle();
    final switched = container.read(notificationHistoryControllerProvider);
    expect(switched.unreadCount, 1);
    expect(switched.items, isEmpty,
        reason: 'the new controller starts from the fresh response');
  });

  test('provider wiring: sign-out clears the bell state', () async {
    final repo = _FakeNotificationRepository(pages: [
      _page(unread: 5, items: [_item('a')]),
    ]);
    final auth = _MutableAuthController(
      const AuthState(status: AuthStatus.authenticated, sessionId: 'session-1'),
    );

    final container = ProviderContainer(overrides: [
      authControllerProvider.overrideWith((_) => auth),
      notificationRepositoryProvider.overrideWithValue(repo),
      notificationCredentialsProvider
          .overrideWithValue(_CredentialsSource(_creds)),
    ]);
    addTearDown(container.dispose);
    container.listen(
        notificationHistoryControllerProvider, (previous, next) {});
    await _settle();
    expect(
        container.read(notificationHistoryControllerProvider).unreadCount, 5);

    auth.becomeUnauthenticated();
    await _settle();
    final cleared = container.read(notificationHistoryControllerProvider);
    expect(cleared.unreadCount, isNull);
    expect(cleared.items, isEmpty);
  });
}
