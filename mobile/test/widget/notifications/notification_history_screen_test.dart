import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_router/go_router.dart';
import 'package:halaqaty_mobile/features/notifications/application/notification_history_controller.dart';
import 'package:halaqaty_mobile/features/notifications/data/notification_repository.dart';
import 'package:halaqaty_mobile/features/notifications/domain/notification_models.dart';
import 'package:halaqaty_mobile/features/notifications/presentation/notification_bell.dart';
import 'package:halaqaty_mobile/features/notifications/presentation/notification_history_screen.dart';

NotificationItem _item({
  required String id,
  required String type,
  bool isRead = false,
}) =>
    NotificationItem(
      id: id,
      type: type,
      // Server copy: the generic fallback; the Arabic-first resolver users
      // never sees these unless the type is unknown.
      title: 'server title $id',
      body: 'server body $id',
      source: const NotificationSourceRef(type: 'session', id: 'session-1'),
      createdAt: DateTime.utc(2026, 10, 25, 18, 30),
      isRead: isRead,
    );

/// Seeded hub controller: records load/loadMore calls, no network, no
/// realtime (test-boundary fake per test-guard: this is an application
/// controller seam, not a freezed data class).
class _SeededHubController extends NotificationHistoryController {
  _SeededHubController(NotificationHistoryState seed)
      : _seedLoadCalls = 0,
        _loadMoreCalls = 0,
        super(
          repository: _ThrowingRepository(),
          credentials: _ThrowingCredentials(),
          connectRealtime: (_) =>
              const Stream<NotificationRealtimeEvent>.empty(),
        ) {
    state = seed;
  }

  int _seedLoadCalls;
  int _loadMoreCalls;

  int get loadCalls => _seedLoadCalls;
  int get loadMoreCalls => _loadMoreCalls;

  @override
  Future<void> load() async {
    _seedLoadCalls++;
  }

  @override
  Future<void> loadMore() async {
    _loadMoreCalls++;
  }
}

class _ThrowingRepository implements NotificationRepository {
  @override
  dynamic noSuchMethod(Invocation invocation) =>
      throw UnimplementedError('seeded hub never calls the repository');
}

class _ThrowingCredentials implements NotificationCredentialsSource {
  @override
  dynamic noSuchMethod(Invocation invocation) =>
      throw UnimplementedError('seeded hub never loads credentials');
}

NotificationHistoryState _seed({
  List<NotificationItem>? items,
  String? nextCursor,
  int? unreadCount,
  NotificationHistoryStatus status = NotificationHistoryStatus.ready,
}) =>
    NotificationHistoryState(
      items: items ?? const [],
      nextCursor: nextCursor,
      unreadCount: unreadCount,
      status: status,
    );

/// Pumps the history screen inside the RTL app direction with the seeded
/// hub provider override.
Future<void> _pumpScreen(
  WidgetTester tester,
  _SeededHubController hub,
) async {
  await tester.pumpWidget(
    ProviderScope(
      overrides: [
        notificationHistoryControllerProvider.overrideWith((ref) => hub),
      ],
      child: MaterialApp(
        builder: (context, child) =>
            Directionality(textDirection: TextDirection.rtl, child: child!),
        home: NotificationHistoryScreen(),
      ),
    ),
  );
  // initState reconciliation lands post-frame.
  await tester.pump();
  await tester.pump();
}

Future<void> _pumpBellWithRouter(
  WidgetTester tester,
  NotificationHistoryState seed,
) async {
  final router = GoRouter(
    initialLocation: '/home',
    routes: [
      GoRoute(
        path: '/home',
        builder: (context, state) =>
            Scaffold(appBar: AppBar(actions: [const NotificationBell()])),
      ),
      GoRoute(
        path: '/notifications',
        builder: (context, state) => const Scaffold(
          body: Center(child: FlutterLogo(key: Key('NC-SCREEN'))),
        ),
      ),
    ],
  );
  final hub = _SeededHubController(seed);
  await tester.pumpWidget(
    ProviderScope(
      overrides: [
        notificationHistoryControllerProvider.overrideWith((ref) => hub),
      ],
      child: MaterialApp.router(routerConfig: router),
    ),
  );
  await tester.pumpAndSettle();
}

void main() {
  testWidgets('resolves Arabic-first copy for known types and keys each row',
      (tester) async {
    final hub = _SeededHubController(_seed(
      items: [
        _item(id: 'n-1', type: 'session_started'),
        _item(id: 'n-2', type: 'future_event'),
      ],
      unreadCount: 3,
    ));
    await _pumpScreen(tester, hub);

    expect(find.byKey(const Key('notificationItem-n-1')), findsOneWidget);
    expect(find.byKey(const Key('notificationItem-n-2')), findsOneWidget);
    // Known type: Arabic display copy, never the server fallback.
    expect(find.text('بدأت الجلسة'), findsOneWidget);
    expect(find.text('بدأت جلسة الحلقة الآن؛ يمكنك الانضمام.'), findsOneWidget);
    expect(find.text('server title n-1'), findsNothing);
    // Unknown/future type: falls back to the server copy verbatim.
    expect(find.text('server title n-2'), findsOneWidget);
    expect(find.text('server body n-2'), findsOneWidget);
    // Entry reconciliation fired exactly once (post-frame).
    expect(hub.loadCalls, 1);
  });

  testWidgets('renders unread dots only for unread rows and the date label',
      (tester) async {
    final hub = _SeededHubController(_seed(
      items: [
        _item(id: 'n-1', type: 'session_started'),
        _item(id: 'n-2', type: 'session_started', isRead: true),
      ],
      unreadCount: 1,
    ));
    await _pumpScreen(tester, hub);

    expect(find.byIcon(Icons.fiber_manual_record), findsOneWidget);
    expect(
      find.descendant(
        of: find.byKey(const Key('notificationItem-n-2')),
        matching: find.byIcon(Icons.fiber_manual_record),
      ),
      findsNothing,
    );
    expect(find.text('25/10/2026'), findsNWidgets(2));
  });

  testWidgets('empty state shows the branded empty card without retry',
      (tester) async {
    final hub = _SeededHubController(_seed(unreadCount: 0));
    await _pumpScreen(tester, hub);

    expect(find.byKey(const Key('notificationHistoryEmpty')), findsOneWidget);
    expect(find.text('لا توجد إشعارات بعد'), findsOneWidget);
    expect(find.text('إعادة المحاولة'), findsNothing);
  });

  testWidgets('error state maps to safe copy with the retry affordance',
      (tester) async {
    final hub = _SeededHubController(_seed(
      status: NotificationHistoryStatus.error,
    ));
    await _pumpScreen(tester, hub);

    expect(find.byKey(const Key('notificationHistoryRetry')), findsOneWidget);
    expect(find.text('إعادة المحاولة'), findsOneWidget);
    expect(hub.loadCalls, 1);

    await tester.tap(find.text('إعادة المحاولة'));
    await tester.pump();
    expect(hub.loadCalls, 2);
  });

  testWidgets('load more appears only with a cursor and records the call',
      (tester) async {
    final hub = _SeededHubController(_seed(
      items: [_item(id: 'n-1', type: 'session_started')],
      nextCursor: 'opaque',
      unreadCount: 1,
    ));
    await _pumpScreen(tester, hub);

    final button = find.byKey(const Key('notificationHistoryLoadMore'));
    expect(button, findsOneWidget);
    expect(hub.loadMoreCalls, 0);
    await tester.tap(button);
    await tester.pump();
    expect(hub.loadMoreCalls, 1);
  });

  testWidgets('load more is absent when the history is exhausted',
      (tester) async {
    final hub = _SeededHubController(_seed(
      items: [_item(id: 'n-1', type: 'session_started')],
      unreadCount: 1,
    ));
    await _pumpScreen(tester, hub);

    expect(find.byKey(const Key('notificationHistoryLoadMore')), findsNothing);
  });

  test('badge label caps at 99+', () {
    expect(NotificationBell.badgeLabel(7), '7');
    expect(NotificationBell.badgeLabel(99), '99');
    expect(NotificationBell.badgeLabel(120), '99+');
  });

  testWidgets('bell shows the exact server count and opens the center',
      (tester) async {
    await _pumpBellWithRouter(
      tester,
      _seed(unreadCount: 2),
    );
    expect(find.text('2'), findsOneWidget);
    await tester.tap(find.byKey(const Key('homeNotificationsAction')));
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('NC-SCREEN')), findsOneWidget);
  });

  testWidgets('bell hides the badge when the count is unknown (no session)',
      (tester) async {
    await _pumpBellWithRouter(tester, _seed());
    expect(find.text('0'), findsNothing);
    expect(find.text('2'), findsNothing);
    expect(find.byKey(const Key('homeNotificationsAction')), findsOneWidget);
  });
}
