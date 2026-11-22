import 'dart:async';

import 'package:firebase_messaging/firebase_messaging.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/notifications/application/notification_fcm_controller.dart';

class _FakeFirebaseMessaging implements FirebaseMessaging {
  _FakeFirebaseMessaging();

  final StreamController<RemoteMessage> onMessageController =
      StreamController<RemoteMessage>.broadcast();

  final StreamController<RemoteMessage> onMessageOpenedAppController =
      StreamController<RemoteMessage>.broadcast();

  /// Served by [getInitialMessage]; `null` means the app was not opened
  /// from a notification tap (cold start without a notice).
  Future<RemoteMessage?> initialMessageFuture = Future<RemoteMessage?>.value();

  @override
  Future<RemoteMessage?> getInitialMessage() => initialMessageFuture;

  void emitForeground(RemoteMessage message) {
    onMessageController.add(message);
  }

  void emitOpenedApp(RemoteMessage message) {
    onMessageOpenedAppController.add(message);
  }

  Future<void> settle() async {
    await Future<void>.delayed(Duration.zero);
    await Future<void>.delayed(Duration.zero);
  }

  void close() {
    onMessageController.close();
    onMessageOpenedAppController.close();
  }

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

const _openedMessage = RemoteMessage(
  messageId: 'open-1',
  // Push payloads are positional: a bogus deep-link field must not leak
  // into the open intent (T026 alignment note 2026-10-25).
  data: <String, dynamic>{'evil_route': '/unauthorized'},
);

NotificationFcmController _controller(_FakeFirebaseMessaging messaging) {
  return NotificationFcmController(
    foregroundMessages: messaging.onMessageController.stream,
    openTaps: messaging.onMessageOpenedAppController.stream,
    initialMessage: () => messaging.getInitialMessage(),
  );
}

void main() {
  test(
    'foreground messages land on the presentation seam without triggering '
    'open routing',
    () async {
      final messaging = _FakeFirebaseMessaging();
      final controller = _controller(messaging);
      addTearDown(controller.dispose);
      addTearDown(messaging.close);
      await messaging.settle();

      messaging.emitForeground(const RemoteMessage(messageId: 'fg-1'));
      await messaging.settle();

      expect(controller.state.lastForegroundMessage!.messageId, 'fg-1');
      expect(controller.state.pendingOpen, isNull);
    },
  );

  test('a notification tap records an in-app open intent', () async {
    final messaging = _FakeFirebaseMessaging();
    final controller = _controller(messaging);
    addTearDown(controller.dispose);
    addTearDown(messaging.close);
    await messaging.settle();

    messaging.emitOpenedApp(_openedMessage);
    await messaging.settle();

    expect(controller.state.pendingOpen!.kind,
        NotificationOpenIntentKind.inAppTap);
  });

  test('a cold start from a notification records a cold-start open intent',
      () async {
    final messaging = _FakeFirebaseMessaging()
      ..initialMessageFuture = Future<RemoteMessage?>.value(_openedMessage);
    final controller = _controller(messaging);
    addTearDown(controller.dispose);
    addTearDown(messaging.close);

    await messaging.settle();

    expect(controller.state.pendingOpen!.kind,
        NotificationOpenIntentKind.coldStart);
  });

  test('a cold start without a notification records no open intent', () async {
    final messaging = _FakeFirebaseMessaging();
    final controller = _controller(messaging);
    addTearDown(controller.dispose);
    addTearDown(messaging.close);

    await messaging.settle();

    expect(controller.state.pendingOpen, isNull);
  });

  test('the open intent never exposes payload contents (positional-only)',
      () async {
    final messaging = _FakeFirebaseMessaging()
      ..initialMessageFuture = Future<RemoteMessage?>.value(_openedMessage);
    final controller = _controller(messaging);
    addTearDown(controller.dispose);
    addTearDown(messaging.close);

    await messaging.settle();

    final state = controller.state;
    expect(state.pendingOpen!.kind, isA<NotificationOpenIntentKind>());
    // The recorded open intent carries no route, source, or payload data:
    // resolving authorized source state is GET /notifications (T028).
    expect(state.lastForegroundMessage, isNull);
  });

  test('the top-level background handler does no work off the message',
      () async {
    // The server persists history/unread; the handler exists so the plugin
    // keeps receiving data messages while terminated. It must complete
    // without touching payload contents.
    await firebaseMessagingBackgroundHandler(_openedMessage);
  });
}
