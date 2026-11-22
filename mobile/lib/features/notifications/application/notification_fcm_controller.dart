import 'dart:async';

import 'package:firebase_core/firebase_core.dart';
import 'package:firebase_messaging/firebase_messaging.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

/// How the user arrived from a notification tap.
enum NotificationOpenIntentKind { coldStart, inAppTap }

/// A recorded notification-open event.
///
/// Intentionally carries no payload content: FCM push payloads carry no
/// source data (tasks.md T026 alignment note 2026-10-25), so resolving the
/// authorized destination happens only server-side via `GET /notifications`
/// (T028), never from push fields.
class NotificationOpenIntent {
  const NotificationOpenIntent({required this.kind, required this.receivedAt});

  final NotificationOpenIntentKind kind;
  final DateTime receivedAt;
}

/// App-facet state of FCM message arrival (T026).
///
/// `lastForegroundMessage` is the presentation seam: no in-app display
/// exists yet. T027 will plug its history/bell reconciliation here and T032
/// will gate the heads-up presentation on the type preference (preference
/// OFF suppresses the overlay, never the `notification.updated` WS
/// reconciliation). iOS foreground presentation stays at plugin defaults so
/// no heads-up can bypass the preference rule once it lands.
class NotificationFcmState {
  const NotificationFcmState({this.lastForegroundMessage, this.pendingOpen});

  final RemoteMessage? lastForegroundMessage;

  /// Latest notification-open event; the app routes its decision point in
  /// main.dart (T027 seam: a future notification-center route replaces the
  /// `/home` landing).
  final NotificationOpenIntent? pendingOpen;

  NotificationFcmState copyWith({
    RemoteMessage? lastForegroundMessage,
    NotificationOpenIntent? pendingOpen,
  }) {
    return NotificationFcmState(
      lastForegroundMessage:
          lastForegroundMessage ?? this.lastForegroundMessage,
      pendingOpen: pendingOpen ?? this.pendingOpen,
    );
  }
}

/// Subscribes the app to the injected FCM streams (foreground messages,
/// open taps, cold-start resolution). Streams are injected because
/// `FirebaseMessaging.onMessage` / `onMessageOpenedApp` are static streams.
class NotificationFcmController extends StateNotifier<NotificationFcmState> {
  NotificationFcmController({
    required Stream<RemoteMessage> foregroundMessages,
    required Stream<RemoteMessage> openTaps,
    required Future<RemoteMessage?> Function() initialMessage,
  }) : super(const NotificationFcmState()) {
    _foregroundSubscription = foregroundMessages.listen(
      (message) => state = state.copyWith(lastForegroundMessage: message),
    );
    _openTapSubscription = openTaps.listen(
      (_) => _recordOpen(NotificationOpenIntentKind.inAppTap),
    );
    unawaited(_resolveColdStart(initialMessage));
  }

  StreamSubscription<RemoteMessage>? _foregroundSubscription;
  StreamSubscription<RemoteMessage>? _openTapSubscription;

  Future<void> _resolveColdStart(
    Future<RemoteMessage?> Function() initialMessage,
  ) async {
    try {
      if (await initialMessage() != null) {
        await _recordOpen(NotificationOpenIntentKind.coldStart);
      }
    } on FirebaseException {
      // Cold-start resolution is unavailable; the user still lands on the
      // sanctioned shell and the next open recording reconciles.
    }
  }

  Future<void> _recordOpen(NotificationOpenIntentKind kind) {
    state = state.copyWith(
      pendingOpen: NotificationOpenIntent(
        kind: kind,
        receivedAt: DateTime.now().toUtc(),
      ),
    );
    return Future.value();
  }

  @override
  void dispose() {
    unawaited(_foregroundSubscription?.cancel());
    unawaited(_openTapSubscription?.cancel());
    super.dispose();
  }
}

/// Top-level handler for messages arriving while the app is terminated
/// (required by the FCM plugin; registered from `main.dart`).
///
/// The server persists history/unread before dispatch and the payload is
/// positional only, so this handler performs no work: cold-start/open
/// routing resolves server state via `GET /notifications`, never push
/// fields (tasks.md T026 alignment note 2026-10-25).
@pragma('vm:entry-point')
Future<void> firebaseMessagingBackgroundHandler(RemoteMessage message) async {}

final notificationFcmControllerProvider =
    StateNotifierProvider<NotificationFcmController, NotificationFcmState>(
        (ref) {
  return NotificationFcmController(
    foregroundMessages: FirebaseMessaging.onMessage,
    openTaps: FirebaseMessaging.onMessageOpenedApp,
    initialMessage: () => FirebaseMessaging.instance.getInitialMessage(),
  );
});
